package fetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// entry describes a single tar entry the test wants to include. Symlinks
// and other typeflags can be specified via TypeflagOverride.
type entry struct {
	Name             string
	Body             string
	Typeflag         byte // tar.TypeReg by default
	TypeflagOverride byte
	LinkName         string
	Size             int64 // override for stress tests
}

// buildTarball returns gzipped tar bytes for the given entries, prefixed
// with topDir to match GitHub's "<repo>-<sha>/" wrapper. Any entry with
// Typeflag 0 is treated as a regular file.
func buildTarball(t *testing.T, topDir string, entries []entry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{
		Name:     topDir + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
		ModTime:  time.Now(),
	})
	for _, e := range entries {
		typeflag := e.Typeflag
		if e.TypeflagOverride != 0 {
			typeflag = e.TypeflagOverride
		}
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     topDir + "/" + e.Name,
			Mode:     0o644,
			Typeflag: typeflag,
			Linkname: e.LinkName,
			ModTime:  time.Now(),
		}
		if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
			hdr.Size = int64(len(e.Body))
			if e.Size != 0 {
				hdr.Size = e.Size
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Size > 0 && len(e.Body) > 0 {
			if _, err := io.WriteString(tw, e.Body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeTarball(t *testing.T, dir string, b []byte) string {
	t.Helper()
	path := filepath.Join(dir, "archive.tar.gz")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestExtract_HappyPath(t *testing.T) {
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "SKILL.md", Body: "---\nname: r\n---\n"},
		{Name: "scripts/run.sh", Body: "#!/bin/sh\n"},
		{Name: "docs/readme.md", Body: "# docs\n"},
	})
	src := writeTarball(t, t.TempDir(), b)
	dst := t.TempDir()
	if err := Extract(src, dst, ""); err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	for _, p := range []string{"SKILL.md", "scripts/run.sh", "docs/readme.md"} {
		if _, err := os.Stat(filepath.Join(dst, p)); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(filepath.Join(dst, "scripts", "run.sh"))
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("scripts/run.sh not executable: %v", info.Mode())
		}
		si, _ := os.Stat(filepath.Join(dst, "SKILL.md"))
		if si.Mode().Perm()&0o111 != 0 {
			t.Errorf("SKILL.md unexpectedly executable: %v", si.Mode())
		}
	}
}

func TestExtract_WithSubpath(t *testing.T) {
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "skills/foo/SKILL.md", Body: "---\nname: foo\n---\n"},
		{Name: "skills/foo/scripts/run.sh", Body: "x"},
		{Name: "skills/bar/SKILL.md", Body: "..."},
		{Name: "README.md", Body: "..."},
	})
	src := writeTarball(t, t.TempDir(), b)
	dst := t.TempDir()
	if err := Extract(src, dst, "skills/foo"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("subpath SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "scripts", "run.sh")); err != nil {
		t.Errorf("subpath scripts/run.sh missing: %v", err)
	}
	// Files outside the subpath must NOT be present.
	if _, err := os.Stat(filepath.Join(dst, "README.md")); err == nil {
		t.Errorf("README.md should not have been extracted")
	}
	if _, err := os.Stat(filepath.Join(dst, "skills")); err == nil {
		t.Errorf("skills/ directory should not have been extracted under subpath stripping")
	}
}

func TestExtract_RejectsPathTraversal(t *testing.T) {
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "../escape.txt", Body: "x"},
	})
	src := writeTarball(t, t.TempDir(), b)
	if err := Extract(src, t.TempDir(), ""); err == nil {
		t.Fatal("expected error for path traversal entry")
	}
}

func TestExtract_SkipsSymlinks(t *testing.T) {
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "SKILL.md", Body: "---\nname: r\n---\n"},
		{Name: "link", TypeflagOverride: tar.TypeSymlink, LinkName: "../etc/passwd"},
	})
	src := writeTarball(t, t.TempDir(), b)
	dst := t.TempDir()
	if err := Extract(src, dst, ""); err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dst, "link")); err == nil {
		t.Errorf("symlink was created; expected skip")
	}
}

func TestExtract_RejectsOversizedTotal(t *testing.T) {
	// Two large legitimate-looking entries that together exceed the cap.
	huge := strings.Repeat("a", 60*1024*1024)
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "a.bin", Body: huge},
		{Name: "b.bin", Body: huge},
	})
	src := writeTarball(t, t.TempDir(), b)
	if err := Extract(src, t.TempDir(), ""); err == nil {
		t.Fatal("expected oversize error")
	}
}

func TestExtract_EmptyArchive(t *testing.T) {
	b := buildTarball(t, "repo-abc", nil)
	src := writeTarball(t, t.TempDir(), b)
	if err := Extract(src, t.TempDir(), ""); err == nil {
		t.Fatal("expected error for empty archive")
	}
}

func TestExtract_UnmatchedSubpath(t *testing.T) {
	b := buildTarball(t, "repo-abc", []entry{
		{Name: "SKILL.md", Body: "x"},
	})
	src := writeTarball(t, t.TempDir(), b)
	if err := Extract(src, t.TempDir(), "no/such/path"); err == nil {
		t.Fatal("expected error when subpath matches nothing")
	}
}
