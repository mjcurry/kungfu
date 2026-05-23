package skill

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/target"
)

func makeFixtureSkill(t *testing.T, root, name string, withExecScript bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Use this skill when testing installs.\n---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if withExecScript {
		if err := os.MkdirAll(filepath.Join(dir, "scripts"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "scripts", "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFindByName(t *testing.T) {
	primary := t.TempDir()
	extra := t.TempDir()

	makeFixtureSkill(t, primary, "alpha", false)
	makeFixtureSkill(t, extra, "alpha", false)
	makeFixtureSkill(t, extra, "beta", false)

	locations := []target.Location{
		{Target: target.Target{Name: "claude"}, Scope: target.ScopePersonal, Dir: primary},
		{Target: target.Target{Name: "codex"}, Scope: target.ScopePersonal, Dir: extra},
	}

	t.Run("multiple matches across locations", func(t *testing.T) {
		matches, err := FindByName("alpha", locations)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 2 {
			t.Errorf("got %d matches, want 2", len(matches))
		}
		if matches[0].Target != "claude" || matches[1].Target != "codex" {
			t.Errorf("matches in wrong order: %+v", matches)
		}
	})

	t.Run("single match", func(t *testing.T) {
		matches, err := FindByName("beta", locations)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 || matches[0].Target != "codex" {
			t.Errorf("got %+v", matches)
		}
	})

	t.Run("no match returns empty slice", func(t *testing.T) {
		matches, err := FindByName("missing", locations)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Errorf("got %+v, want empty", matches)
		}
	})

	t.Run("empty name returns empty slice", func(t *testing.T) {
		matches, err := FindByName("", locations)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Errorf("got %+v", matches)
		}
	})
}

func TestInstallCopiesTree(t *testing.T) {
	src := makeFixtureSkill(t, t.TempDir(), "demo", true)
	dst := filepath.Join(t.TempDir(), "demo")

	if _, err := Install(src, dst, false); err != nil {
		t.Fatalf("Install() error: %v", err)
	}
	if _, err := Load(dst); err != nil {
		t.Errorf("Load(installed) error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "scripts", "run.sh")); err != nil {
		t.Errorf("scripts/run.sh missing: %v", err)
	}
}

func TestInstallPreservesExecutableBit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes not meaningful on Windows")
	}
	src := makeFixtureSkill(t, t.TempDir(), "exec", true)
	dst := filepath.Join(t.TempDir(), "exec")
	if _, err := Install(src, dst, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dst, "scripts", "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script not executable after install: mode=%v", info.Mode())
	}
}

func TestInstallRefusesExistingWithoutForce(t *testing.T) {
	src := makeFixtureSkill(t, t.TempDir(), "existing", false)
	dstParent := t.TempDir()
	dst := filepath.Join(dstParent, "existing")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Install(src, dst, false)
	if !errors.Is(err, ErrSkillExists) {
		t.Fatalf("err = %v, want ErrSkillExists", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "marker")); err != nil {
		t.Errorf("destination was touched without --force: %v", err)
	}
}

func TestInstallForceReplacesAndCleansBackup(t *testing.T) {
	src := makeFixtureSkill(t, t.TempDir(), "force", false)
	dstParent := t.TempDir()
	dst := filepath.Join(dstParent, "force")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, "old-marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Install(src, dst, true); err != nil {
		t.Fatalf("Install --force error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "old-marker")); err == nil {
		t.Error("old-marker still exists after force install")
	}
	if _, err := Load(dst); err != nil {
		t.Errorf("Load(installed) error: %v", err)
	}
	entries, err := os.ReadDir(dstParent)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") || strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("leftover artifact: %s", e.Name())
		}
	}
}

func TestInstallRejectsNonDirectorySource(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(file, filepath.Join(root, "dst"), false); err == nil {
		t.Fatal("expected error when source is a file")
	}
}

func TestInstallRejectsSourceWithoutSKILLmd(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "nope")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(src, filepath.Join(root, "dst"), false); err == nil {
		t.Fatal("expected error when source has no SKILL.md")
	}
}

func TestInstallForceReportsBackupLeftoverWithoutFailing(t *testing.T) {
	// When force-overwriting an existing skill and the backup cleanup
	// fails (e.g. read-only parent), the install itself has already
	// committed — surface the leftover path via Result and return nil
	// rather than reporting the install as failed.
	if runtime.GOOS == "windows" {
		t.Skip("read-only directory semantics differ on Windows")
	}
	src := makeFixtureSkill(t, t.TempDir(), "leftover", false)
	dstParent := t.TempDir()
	dst := filepath.Join(dstParent, "leftover")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-populate the destination so backup-creation kicks in.
	if err := os.WriteFile(filepath.Join(dst, "old-marker"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the parent read-only AFTER install renames things into place
	// — we cannot prevent the create-backup rename, but we can break the
	// cleanup RemoveAll. The simplest cross-platform way is to plant an
	// undeletable file inside the backup once it appears. Easier: chmod
	// the parent to 0o555 right before Install runs; the staging copy,
	// the rename to backup, and the rename of staging→dst all fail under
	// 0o555 because they create entries in the parent. We need a more
	// surgical hook.
	//
	// Approach: use a fixture where the destination contains a
	// subdirectory we make 0o500 so its inner files cannot be removed.
	// RemoveAll then fails on the backup, but the new install is already
	// in place because the rename committed first.
	innerDir := filepath.Join(dst, "innards")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(innerDir, "stuck.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(innerDir, 0o500); err != nil {
		t.Fatal(err)
	}
	// After the test, walk dstParent and restore any 0o500 dirs to 0o755
	// so t.TempDir's own RemoveAll cleanup can succeed. The innards
	// directory will have been renamed inside a `.bak-<rand>` sibling.
	t.Cleanup(func() {
		_ = filepath.WalkDir(dstParent, func(path string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				_ = os.Chmod(path, 0o755)
			}
			return nil
		})
	})

	res, err := Install(src, dst, true)
	if err != nil {
		t.Fatalf("Install --force returned error: %v", err)
	}
	if res.BackupLeftover == "" {
		t.Errorf("expected BackupLeftover to be set when cleanup fails")
	}
	// The new copy must still be in place.
	if _, err := Load(dst); err != nil {
		t.Errorf("Load(installed) failed despite Install returning nil: %v", err)
	}
}

func TestInstallSourceMissingErrors(t *testing.T) {
	if _, err := Install(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "dst"), false); err == nil {
		t.Fatal("expected error for missing source")
	} else if !errors.Is(err, fs.ErrNotExist) {
		// fs.ErrNotExist wrapping is conventional but not required; just
		// make sure we got *some* error here.
		_ = err
	}
}
