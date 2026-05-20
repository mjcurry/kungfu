package fetch

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/mjcurry/kungfu/internal/skill"
)

// MaxExtractedSize caps total extracted bytes. Matches the tarball cap so a
// zip-bomb-style archive cannot blow up local disk regardless of compression.
const MaxExtractedSize int64 = 100 * 1024 * 1024

// Extract decompresses and unpacks the tarball at tarPath into destDir. If
// subpath is non-empty only entries beneath that subpath are extracted, and
// the subpath prefix is stripped so destDir holds the skill at its root.
//
// GitHub tarballs always have a single top-level "<repo>-<sha>/" directory;
// that prefix is stripped regardless of subpath.
//
// Safety guarantees:
//
//   - entries that escape destDir (path traversal) are rejected;
//   - symlinks, device files, FIFOs, and sockets are skipped;
//   - total extracted size is capped at MaxExtractedSize;
//   - regular file permissions follow skill.FileMode rather than the
//     archive's bits, matching the template and local install behaviour.
func Extract(tarPath, destDir, subpath string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("fetch: extract: opening %s: %w", tarPath, err)
	}
	defer f.Close()
	return ExtractReader(f, destDir, subpath)
}

// ExtractReader is the streaming form of Extract; r should serve gzipped tar
// bytes.
func ExtractReader(r io.Reader, destDir, subpath string) error {
	abs, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("fetch: extract: resolving %s: %w", destDir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return fmt.Errorf("fetch: extract: creating %s: %w", abs, err)
	}

	gz, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("fetch: extract: gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	subpath = strings.Trim(subpath, "/")

	var total int64
	sawAny := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("fetch: extract: tar: %w", err)
		}
		// Skip non-regular entries we never want to materialise.
		switch hdr.Typeflag {
		case tar.TypeSymlink, tar.TypeLink,
			tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
			continue
		}

		rel, ok := stripTopLevel(hdr.Name)
		if !ok || rel == "" {
			continue
		}
		if subpath != "" {
			if !hasSubpathPrefix(rel, subpath) {
				continue
			}
			rel = strings.TrimPrefix(rel, subpath)
			rel = strings.TrimPrefix(rel, "/")
			if rel == "" && hdr.Typeflag != tar.TypeDir {
				// The subpath itself addresses a single file. Rare; treat
				// it as the SKILL.md placement under destDir.
				rel = filepath.Base(hdr.Name)
			}
			if rel == "" {
				continue
			}
		}

		target := filepath.Join(abs, filepath.FromSlash(rel))
		if !strings.HasPrefix(target+string(filepath.Separator), abs+string(filepath.Separator)) &&
			target != abs {
			return fmt.Errorf("fetch: extract: refused path traversal entry %q", hdr.Name)
		}

		sawAny = true
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("fetch: extract: mkdir %s: %w", target, err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if hdr.Size > MaxExtractedSize {
				return fmt.Errorf("fetch: extract: file %s too large (%d bytes)", hdr.Name, hdr.Size)
			}
			total += hdr.Size
			if total > MaxExtractedSize {
				return fmt.Errorf("fetch: extract: total size exceeded %d bytes", MaxExtractedSize)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("fetch: extract: mkdir parent of %s: %w", target, err)
			}
			if err := writeRegular(target, tr, skill.FileMode(rel)); err != nil {
				return err
			}
		}
	}
	if !sawAny {
		if subpath != "" {
			return fmt.Errorf("fetch: extract: subpath %q matched no entries", subpath)
		}
		return fmt.Errorf("fetch: extract: empty archive")
	}
	return nil
}

func writeRegular(target string, r io.Reader, mode os.FileMode) error {
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("fetch: extract: opening %s: %w", target, err)
	}
	if _, err := io.Copy(out, r); err != nil {
		_ = out.Close()
		return fmt.Errorf("fetch: extract: writing %s: %w", target, err)
	}
	return out.Close()
}

// stripTopLevel removes the "<repo>-<sha>/" prefix GitHub adds to every
// entry in an archive. Returns (rel, true) if the entry was under that
// prefix or (entry, true) if there is no prefix to strip. The second
// return is false for unexpectedly-shaped entries that should be skipped.
func stripTopLevel(name string) (string, bool) {
	name = path.Clean(name)
	if name == "." || name == "/" {
		return "", false
	}
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return name[i+1:], true
	}
	// A bare top-level directory entry (e.g. "repo-sha"); nothing to
	// extract from it directly.
	return "", false
}

// hasSubpathPrefix reports whether rel sits under subpath. Match is on path
// components, not bytes, so subpath="docs" does not match rel="docs2/foo".
func hasSubpathPrefix(rel, subpath string) bool {
	if rel == subpath {
		return true
	}
	return strings.HasPrefix(rel, subpath+"/")
}
