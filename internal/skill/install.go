package skill

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ErrSkillExists is returned by Install when the destination directory
// already exists and the caller did not request force-overwrite.
var ErrSkillExists = errors.New("skill: destination already exists")

// InstallResult reports non-fatal warnings from a successful install.
// A non-zero Result with a nil error means the install committed but the
// caller should surface the warning to the user.
type InstallResult struct {
	// BackupLeftover is the path of a `.bak-<rand>` directory that was
	// created when force=true overwrote an existing skill and could not
	// be removed after the new copy was committed. The install itself
	// succeeded; the user should `rm -rf` the path manually.
	BackupLeftover string
}

// Install copies the skill tree at src into dst. The directory at src must
// contain a SKILL.md.
//
// The copy is staged through a sibling temporary directory and committed
// with an atomic rename, so an interrupted install never leaves a partial
// skill behind. When force is true and dst already exists, the existing
// directory is first renamed to a sibling backup, then removed after the
// new copy is committed. Backup removal is best-effort: if it fails, the
// install still succeeded (the atomic rename to dst has already happened)
// and the leftover path is surfaced via Result.BackupLeftover.
//
// File modes are preserved (executable scripts stay executable). Symlinks
// are recreated as symlinks pointing at the same target.
func Install(src, dst string, force bool) (InstallResult, error) {
	var res InstallResult
	srcInfo, err := os.Stat(src)
	if err != nil {
		return res, fmt.Errorf("skill: install: stat source: %w", err)
	}
	if !srcInfo.IsDir() {
		return res, fmt.Errorf("skill: install: source %s is not a directory", src)
	}
	if _, err := os.Stat(filepath.Join(src, FileName)); err != nil {
		return res, fmt.Errorf("skill: install: source %s has no %s", src, FileName)
	}

	if info, err := os.Stat(dst); err == nil {
		if !info.IsDir() {
			return res, fmt.Errorf("skill: install: destination %s exists and is not a directory", dst)
		}
		if !force {
			return res, fmt.Errorf("%w: %s", ErrSkillExists, dst)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return res, fmt.Errorf("skill: install: stat destination: %w", err)
	}

	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return res, fmt.Errorf("skill: install: creating %s: %w", parent, err)
	}

	suffix, err := randomSuffix()
	if err != nil {
		return res, fmt.Errorf("skill: install: random suffix: %w", err)
	}
	staging := filepath.Join(parent, filepath.Base(dst)+".tmp-"+suffix)

	_ = os.RemoveAll(staging)
	if err := copyTree(src, staging); err != nil {
		_ = os.RemoveAll(staging)
		return res, fmt.Errorf("skill: install: copying tree: %w", err)
	}

	backup := ""
	if _, err := os.Stat(dst); err == nil {
		backup = dst + ".bak-" + suffix
		if err := os.Rename(dst, backup); err != nil {
			_ = os.RemoveAll(staging)
			return res, fmt.Errorf("skill: install: moving existing %s aside: %w", dst, err)
		}
	}

	if err := os.Rename(staging, dst); err != nil {
		if backup != "" {
			if rerr := os.Rename(backup, dst); rerr != nil {
				return res, fmt.Errorf("skill: install: rename failed (%w) and could not restore backup %s (%v)",
					err, backup, rerr)
			}
		}
		_ = os.RemoveAll(staging)
		return res, fmt.Errorf("skill: install: committing rename: %w", err)
	}

	// The commit-rename above already succeeded — the new skill is live.
	// Backup removal is best-effort: a failure here does not undo the
	// install, so surface it as a non-fatal warning instead of returning
	// an error that would mislead the caller into thinking the install
	// failed (and trigger retries that pile up more backups).
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			res.BackupLeftover = backup
		}
	}
	return res, nil
}

// copyTree replicates src into dst, creating dst itself in the process.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			return copyRegularFile(path, target, info.Mode().Perm())
		}
	})
}

func copyRegularFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func randomSuffix() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
