package rules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFilenamesNonASCII(t *testing.T) {
	t.Run("clean tree", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "SKILL.md"), "x")
		mustWrite(t, filepath.Join(dir, "scripts", "run.sh"), "x")
		if got := (FilenamesNonASCII{}).Check(&skill.Skill{Dir: dir}); len(got) != 0 {
			t.Errorf("unexpected diags: %v", got)
		}
	})

	t.Run("whitespace name flagged", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "SKILL.md"), "x")
		mustWrite(t, filepath.Join(dir, "my notes.md"), "x")
		if got := (FilenamesNonASCII{}).Check(&skill.Skill{Dir: dir}); len(got) != 1 {
			t.Errorf("expected 1 diag, got %v", got)
		}
	})

	t.Run("non-ascii name flagged", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "SKILL.md"), "x")
		mustWrite(t, filepath.Join(dir, "café.md"), "x")
		if got := (FilenamesNonASCII{}).Check(&skill.Skill{Dir: dir}); len(got) != 1 {
			t.Errorf("expected 1 diag, got %v", got)
		}
	})

	t.Run("directory with bad name flagged", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite(t, filepath.Join(dir, "SKILL.md"), "x")
		mustWrite(t, filepath.Join(dir, "weird dir", "file.txt"), "x")
		if got := (FilenamesNonASCII{}).Check(&skill.Skill{Dir: dir}); len(got) < 1 {
			t.Errorf("expected at least 1 diag, got %v", got)
		}
	})
}
