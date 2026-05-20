package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildReleaseTarball returns the bytes of a goreleaser-shaped release
// archive: <binaryName> + LICENSE + README.md at the root, no top-level
// directory prefix. The binary content is just the bytes you pass in so
// tests can assert "this binary ended up on disk".
func buildReleaseTarball(t *testing.T, binaryName string, binaryContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	write := func(name string, body []byte, mode int64) {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     mode,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	write(binaryName, binaryContent, 0o755)
	write("LICENSE", []byte("MIT\n"), 0o644)
	write("README.md", []byte("# kungfu\n"), 0o644)
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// releaseFakeServer serves goreleaser-style release downloads at
// /<tag>/<filename>, plus a "latest" API endpoint at /api/latest that
// reports the configured tag.
type releaseFakeServer struct {
	srv *httptest.Server
	tag string
	// fileName -> contents; the test populates this map.
	files map[string][]byte
}

func newReleaseFakeServer(t *testing.T, tag string) *releaseFakeServer {
	t.Helper()
	rfs := &releaseFakeServer{tag: tag, files: map[string][]byte{}}
	rfs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/latest":
			fmt.Fprintf(w, `{"tag_name":%q}`, rfs.tag)
		case strings.HasPrefix(r.URL.Path, "/download/"+rfs.tag+"/"):
			name := strings.TrimPrefix(r.URL.Path, "/download/"+rfs.tag+"/")
			body, ok := rfs.files[name]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rfs.srv.Close)
	return rfs
}

func (r *releaseFakeServer) apiURL() string  { return r.srv.URL + "/api/latest" }
func (r *releaseFakeServer) baseURL() string { return r.srv.URL + "/download" }

func TestFetchLatestTag(t *testing.T) {
	rfs := newReleaseFakeServer(t, "v9.9.9")
	t.Setenv(EnvReleaseAPIURL, rfs.apiURL())
	got, err := fetchLatestTag()
	if err != nil {
		t.Fatal(err)
	}
	if got != "v9.9.9" {
		t.Errorf("got %q, want v9.9.9", got)
	}
}

func TestExtractBinaryFromTarball(t *testing.T) {
	tar := buildReleaseTarball(t, "kungfu", []byte("KUNGFU_BINARY_CONTENT"))
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := os.WriteFile(archive, tar, 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "extracted")
	if err := extractBinaryFromTarball(archive, "kungfu", out); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "KUNGFU_BINARY_CONTENT" {
		t.Errorf("extracted bytes = %q", got)
	}
}

func TestExtractBinaryFromTarball_MissingBinary(t *testing.T) {
	tar := buildReleaseTarball(t, "kungfu", []byte("x"))
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := os.WriteFile(archive, tar, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractBinaryFromTarball(archive, "other-name", filepath.Join(dir, "out")); err == nil {
		t.Fatal("expected error when the named binary is absent from the tarball")
	}
}

func TestVerifySha256(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "kungfu_0.1.0_linux_amd64.tar.gz")
	body := []byte("hello world")
	if err := os.WriteFile(archive, body, 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := filepath.Join(dir, "kungfu_0.1.0_checksums.txt")
	checksumContents := sha256Hex(body) + "  " + filepath.Base(archive) + "\n" +
		"deadbeef  some-other-file.zip\n"
	if err := os.WriteFile(checksum, []byte(checksumContents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySha256(archive, checksum); err != nil {
		t.Fatalf("matching checksum should pass, got %v", err)
	}
}

func TestVerifySha256_Mismatch(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "a.tar.gz")
	if err := os.WriteFile(archive, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	checksum := filepath.Join(dir, "checksums.txt")
	wrong := sha256Hex([]byte("different bytes"))
	if err := os.WriteFile(checksum, []byte(wrong+"  a.tar.gz\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifySha256(archive, checksum); err == nil {
		t.Fatal("checksum mismatch should error")
	}
}

func TestReplaceBinary(t *testing.T) {
	// Replace a stand-in target with a new file; the running test process
	// is not affected because we point at tempdir paths.
	dir := t.TempDir()
	target := filepath.Join(dir, "kungfu")
	if err := os.WriteFile(target, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	newBin := filepath.Join(dir, "new-kungfu")
	if err := os.WriteFile(newBin, []byte("NEW"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(target, newBin); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "NEW" {
		t.Errorf("target content = %q, want NEW", got)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(target)
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("replaced binary lost +x: %v", info.Mode())
		}
	}
}

func TestReadChecksumFor(t *testing.T) {
	dir := t.TempDir()
	checksum := filepath.Join(dir, "k.txt")
	body := "aaaa  kungfu_0.1.0_linux_amd64.tar.gz\n" +
		"bbbb  kungfu_0.1.0_darwin_arm64.tar.gz\n"
	if err := os.WriteFile(checksum, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readChecksumFor("kungfu_0.1.0_darwin_arm64.tar.gz", checksum)
	if err != nil {
		t.Fatal(err)
	}
	if got != "bbbb" {
		t.Errorf("got %q", got)
	}
	if _, err := readChecksumFor("missing.zip", checksum); err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestEnsureWritable(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "kungfu")
	if err := os.WriteFile(target, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureWritable(target); err != nil {
		t.Errorf("writable dir should pass: %v", err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		if err := ensureWritable(target); err == nil {
			t.Errorf("read-only dir should error")
		}
	}
}

// TestReleaseBaseURL_OverrideRoundTrip is an end-to-end test of the
// download+verify+extract+swap stack against a httptest server, leaving
// only the smoke-test step out (we cannot run a freshly-extracted "kungfu"
// stub during a test). The smoke test is exercised separately above via
// extractBinaryFromTarball.
func TestSelfUpdate_DownloadVerifyExtractReplace(t *testing.T) {
	target := "v0.0.99"
	binName := "kungfu"
	if runtime.GOOS == "windows" {
		binName = "kungfu.exe"
	}
	archiveName := fmt.Sprintf("kungfu_%s_%s_%s.tar.gz",
		strings.TrimPrefix(target, "v"), runtime.GOOS, runtime.GOARCH)
	checksumName := fmt.Sprintf("kungfu_%s_checksums.txt", strings.TrimPrefix(target, "v"))

	tarball := buildReleaseTarball(t, binName, []byte("NEW_BINARY"))
	rfs := newReleaseFakeServer(t, target)
	rfs.files[archiveName] = tarball
	rfs.files[checksumName] = []byte(sha256Hex(tarball) + "  " + archiveName + "\n")

	t.Setenv(EnvReleaseAPIURL, rfs.apiURL())
	t.Setenv(EnvReleaseBaseURL, rfs.baseURL())

	// Drive the lower-level helpers (download → verify → extract →
	// replace) directly. We can't invoke the full self-update command
	// because it would try to overwrite the running test binary.
	dir := t.TempDir()
	archivePath := filepath.Join(dir, archiveName)
	if err := downloadFile(rfs.baseURL()+"/"+target+"/"+archiveName, archivePath); err != nil {
		t.Fatal(err)
	}
	checksumPath := filepath.Join(dir, checksumName)
	if err := downloadFile(rfs.baseURL()+"/"+target+"/"+checksumName, checksumPath); err != nil {
		t.Fatal(err)
	}
	if err := verifySha256(archivePath, checksumPath); err != nil {
		t.Fatal(err)
	}
	stagedBin := filepath.Join(dir, "new-kungfu")
	if err := extractBinaryFromTarball(archivePath, binName, stagedBin); err != nil {
		t.Fatal(err)
	}
	targetBin := filepath.Join(dir, "kungfu-target")
	if err := os.WriteFile(targetBin, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary(targetBin, stagedBin); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(targetBin)
	if string(got) != "NEW_BINARY" {
		t.Errorf("post-replace content = %q, want NEW_BINARY", got)
	}
}
