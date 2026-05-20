package fetch

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// DefaultCacheTTL is how long a cached tarball is considered fresh before
// the client refetches.
const DefaultCacheTTL = 7 * 24 * time.Hour

// Cache stores tarballs on disk keyed by source + resolved SHA.
type Cache struct {
	// Dir is the root cache directory (e.g. ~/.cache/kungfu/tarballs).
	Dir string
	// TTL controls when a cached file is considered stale. Zero means
	// "never expire". DefaultCacheTTL is used by DefaultCache.
	TTL time.Duration
}

// DefaultCache returns a Cache rooted at $XDG_CACHE_HOME/kungfu/tarballs,
// with a 7-day TTL.
func DefaultCache() *Cache {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".cache")
		}
	}
	return &Cache{
		Dir: filepath.Join(base, "kungfu", "tarballs"),
		TTL: DefaultCacheTTL,
	}
}

// Path returns the on-disk location where a tarball for src@sha would be
// stored. It does not check whether the file exists.
func (c *Cache) Path(src *Source, sha string) string {
	return filepath.Join(c.Dir, src.Host, src.Owner, src.Repo, sha+".tar.gz")
}

// Get returns the cached tarball path if it exists and is within TTL. The
// second return value is false on either condition.
func (c *Cache) Get(src *Source, sha string) (string, bool) {
	p := c.Path(src, sha)
	info, err := os.Stat(p)
	if err != nil {
		return "", false
	}
	if c.TTL > 0 && time.Since(info.ModTime()) > c.TTL {
		return "", false
	}
	return p, true
}

// Put writes the bytes from r into the cache at the canonical path for
// src@sha and returns the path. The write is atomic: the data goes through
// a sibling temp file and is renamed into place on success.
func (c *Cache) Put(src *Source, sha string, r io.Reader) (string, error) {
	p := c.Path(src, sha)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", fmt.Errorf("fetch: cache: creating %s: %w", filepath.Dir(p), err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), filepath.Base(p)+".tmp-*")
	if err != nil {
		return "", fmt.Errorf("fetch: cache: temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("fetch: cache: writing %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("fetch: cache: closing %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		return "", fmt.Errorf("fetch: cache: rename %s -> %s: %w", tmpName, p, err)
	}
	cleanup = false
	return p, nil
}
