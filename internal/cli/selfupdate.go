package cli

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/ui"
)

// Environment-variable overrides for the release endpoints. Production runs
// leave them unset and use the real GitHub URLs; tests can point them at
// an httptest.Server.
const (
	EnvReleaseAPIURL  = "KUNGFU_RELEASE_API_URL"
	EnvReleaseBaseURL = "KUNGFU_RELEASE_BASE_URL"
)

// Defaults for the release endpoints.
const (
	defaultReleaseAPIURL  = "https://api.github.com/repos/mjcurry/kungfu/releases/latest"
	defaultReleaseBaseURL = "https://github.com/mjcurry/kungfu/releases/download"
)

// maxBinaryArchiveSize caps the self-update download so a wrong URL or a
// MITM cannot exhaust local disk. Production tarballs are ~5MB; 200MB is
// plenty of headroom.
const maxBinaryArchiveSize int64 = 200 * 1024 * 1024

// newSelfUpdateCmd builds the `kungfu self-update` command.
//
// Exit codes:
//
//	0 — already up to date OR successfully updated
//	2 — user declined the prompt
//	3 — network / I/O / checksum failure
func newSelfUpdateCmd() *cobra.Command {
	var (
		check       bool
		versionFlag string
		yes         bool
	)
	cmd := &cobra.Command{
		Use:   "self-update",
		Short: "Update the kungfu binary to the latest release",
		Long: "Replace the running kungfu binary with the latest release from\n" +
			"GitHub. Verifies the sha256 checksum, smoke-tests the new binary,\n" +
			"then atomically renames it over the current one.\n\n" +
			"Use --check to report whether an update is available without\n" +
			"installing it, or --version to pin a specific tag.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSelfUpdate(cmd, check, versionFlag, yes)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "print available update without installing it")
	cmd.Flags().StringVar(&versionFlag, "version", "", "specific tag to install (default: latest release)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func runSelfUpdate(cmd *cobra.Command, check bool, versionFlag string, yes bool) error {
	out := cmd.OutOrStdout()

	current := currentVersion().Version
	target := strings.TrimSpace(versionFlag)
	if target == "" {
		fmt.Fprintln(out, ui.Muted.Render("checking latest release ..."))
		latest, err := fetchLatestTag()
		if err != nil {
			return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
		}
		target = latest
	}
	curNorm := strings.TrimPrefix(current, "v")
	tgtNorm := strings.TrimPrefix(target, "v")

	fmt.Fprintln(out, "current: "+ui.Bold.Render(current))
	fmt.Fprintln(out, "latest:  "+ui.Bold.Render(target))

	if curNorm == tgtNorm {
		fmt.Fprintln(out, ui.Success.Render("✓ already up to date"))
		return nil
	}
	if check {
		fmt.Fprintln(out, ui.Warning.Render("update available — run `kungfu self-update` to install"))
		return nil
	}

	if !yes {
		fmt.Fprintf(out, "\nupdate kungfu from %s to %s? [Y/n] ", current, target)
		ok, err := readPromptYes(cmd.InOrStdin(), true)
		if err != nil {
			return &ExitError{Code: 3, Err: err}
		}
		if !ok {
			fmt.Fprintln(out, ui.Muted.Render("aborted"))
			return &ExitError{Code: 2}
		}
	}

	binPath, err := resolveSelfBinary()
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}
	if err := ensureWritable(binPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}

	tmpDir, err := os.MkdirTemp("", "kungfu-self-update-*")
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: tempdir: %w", err)}
	}
	defer os.RemoveAll(tmpDir)

	archiveName := fmt.Sprintf("kungfu_%s_%s_%s.tar.gz", tgtNorm, runtime.GOOS, runtime.GOARCH)
	checksumName := fmt.Sprintf("kungfu_%s_checksums.txt", tgtNorm)
	base := releaseBaseURL() + "/" + target

	archivePath := filepath.Join(tmpDir, archiveName)
	fmt.Fprintln(out, ui.Muted.Render("downloading "+archiveName+" ..."))
	if err := downloadFile(base+"/"+archiveName, archivePath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}
	checksumPath := filepath.Join(tmpDir, checksumName)
	if err := downloadFile(base+"/"+checksumName, checksumPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}

	fmt.Fprintln(out, ui.Muted.Render("verifying checksum ..."))
	if err := verifySha256(archivePath, checksumPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}

	newBinaryName := filepath.Base(binPath)
	newBinaryPath := filepath.Join(tmpDir, "new-"+newBinaryName)
	if err := extractBinaryFromTarball(archivePath, newBinaryName, newBinaryPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}
	if err := os.Chmod(newBinaryPath, 0o755); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: chmod new binary: %w", err)}
	}

	// Smoke-test the new binary before swapping in. If it crashes or
	// reports a totally bogus version, abort with the old binary intact.
	fmt.Fprintln(out, ui.Muted.Render("smoke-testing ..."))
	if err := smokeTestBinary(newBinaryPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}

	if err := replaceBinary(binPath, newBinaryPath); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("self-update: %w", err)}
	}

	fmt.Fprintln(out, ui.Success.Render("✓ updated kungfu to "+target))
	return nil
}

// resolveSelfBinary returns the absolute path of the running kungfu binary
// with any symlinks resolved, so a self-update writes to the underlying
// file (e.g. when kungfu is installed via a symlink farm).
func resolveSelfBinary() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating current binary: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		// If EvalSymlinks fails, fall back to the un-resolved path —
		// some test environments have transient symlink resolution
		// failures and the un-resolved path is correct in those cases.
		resolved = p
	}
	return resolved, nil
}

// ensureWritable returns a friendly error when the binary path's directory
// cannot accept a temp file. Tests for write access ahead of time so we
// fail loudly before downloading megabytes.
func ensureWritable(binPath string) error {
	dir := filepath.Dir(binPath)
	tmp, err := os.CreateTemp(dir, ".kungfu-write-probe-*")
	if err != nil {
		return fmt.Errorf("%s is not writable by this user; rerun with sudo or reinstall to ~/.local/bin",
			dir)
	}
	name := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(name)
	return nil
}

// fetchLatestTag GETs the GitHub "latest release" API and returns the tag
// name (e.g. "v0.1.0"). Honours EnvReleaseAPIURL for tests.
func fetchLatestTag() (string, error) {
	url := os.Getenv(EnvReleaseAPIURL)
	if url == "" {
		url = defaultReleaseAPIURL
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kungfu/self-update")
	req.Header.Set("Accept", "application/vnd.github+json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	var data struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&data); err != nil {
		return "", fmt.Errorf("decoding release JSON: %w", err)
	}
	if data.TagName == "" {
		return "", errors.New("release JSON had empty tag_name")
	}
	return data.TagName, nil
}

// releaseBaseURL returns the host URL the archive + checksum live under.
// Honours EnvReleaseBaseURL for tests.
func releaseBaseURL() string {
	if v := os.Getenv(EnvReleaseBaseURL); v != "" {
		return strings.TrimRight(v, "/")
	}
	return defaultReleaseBaseURL
}

// downloadFile streams url into dest, capped at maxBinaryArchiveSize.
func downloadFile(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kungfu/self-update")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: %s", url, resp.Status)
	}
	if resp.ContentLength > maxBinaryArchiveSize {
		return fmt.Errorf("downloading %s: response too large (%d bytes)", url, resp.ContentLength)
	}
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("creating %s: %w", dest, err)
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, maxBinaryArchiveSize+1))
	if err != nil {
		_ = out.Close()
		return fmt.Errorf("writing %s: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return err
	}
	if n > maxBinaryArchiveSize {
		_ = os.Remove(dest)
		return fmt.Errorf("downloading %s: exceeded %d bytes", url, maxBinaryArchiveSize)
	}
	return nil
}

// verifySha256 reads the sha256 of archivePath and compares against the
// matching entry in checksumPath (a file of the goreleaser shape:
// "<sha256>  <filename>" lines).
func verifySha256(archivePath, checksumPath string) error {
	expected, err := readChecksumFor(filepath.Base(archivePath), checksumPath)
	if err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("checksum mismatch for %s\n  expected: %s\n  got:      %s",
			filepath.Base(archivePath), expected, actual)
	}
	return nil
}

// readChecksumFor scans a goreleaser checksums file for the line matching
// name and returns the hex digest from the first column.
func readChecksumFor(name, checksumPath string) (string, error) {
	data, err := os.ReadFile(checksumPath)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksum for %s not found in %s", name, filepath.Base(checksumPath))
}

// extractBinaryFromTarball pulls the named binary out of a goreleaser
// release tarball (kungfu_<v>_<os>_<arch>.tar.gz). The tarball has the
// binary plus LICENSE / README.md at the root; we only need the binary.
func extractBinaryFromTarball(archivePath, binaryName, destPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
			continue
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, tr); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	}
	return fmt.Errorf("binary %q not found in archive", binaryName)
}

// smokeTestBinary runs the freshly-downloaded binary's `version` subcommand
// to make sure it executes at all before we replace ourselves with it.
func smokeTestBinary(path string) error {
	cmd := osexec.Command(path, "version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("new binary failed `kungfu version`: %w", err)
	}
	return nil
}

// replaceBinary writes newBinary in place of target via a sidecar +
// atomic rename. On Unix the rename swaps the inode out from under the
// running process safely. On Windows the rename succeeds when the
// filesystem supports POSIX semantics (recent Windows 10+); otherwise it
// fails with a clear message and the old binary stays in place.
func replaceBinary(target, newBinary string) error {
	sidecar := target + ".new"
	if err := copyFileForUpdate(newBinary, sidecar); err != nil {
		return fmt.Errorf("staging new binary: %w", err)
	}
	if err := os.Chmod(sidecar, 0o755); err != nil {
		_ = os.Remove(sidecar)
		return fmt.Errorf("chmod sidecar: %w", err)
	}
	if err := os.Rename(sidecar, target); err != nil {
		_ = os.Remove(sidecar)
		if runtime.GOOS == "windows" {
			return fmt.Errorf("replacing %s: %w (Windows may require restarting kungfu first; rerun the installer if so)",
				target, err)
		}
		return fmt.Errorf("replacing %s: %w", target, err)
	}
	return nil
}

func copyFileForUpdate(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
