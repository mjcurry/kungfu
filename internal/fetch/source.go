// Package fetch implements remote skill installation from GitHub: parsing
// the user-facing source string, talking to the GitHub API to resolve a
// ref, downloading the gzipped tar archive, caching it, and extracting it
// to a scratch directory.
//
// Network access is confined to internal/fetch.github.go; the rest of the
// package is pure logic that tests can drive against fixture tarballs and
// an httptest-backed Client.
package fetch

import (
	"errors"
	"fmt"
	"strings"
)

// HostGitHub is the only host fetch supports in v1.
const HostGitHub = "github.com"

// ErrNotRemote is returned by Parse when the input looks like a local path.
// Callers (the install command) handle this by falling through to the
// local-install flow.
var ErrNotRemote = errors.New("fetch: source looks like a local path")

// Source identifies a remote skill location.
type Source struct {
	// Host is always "github.com" in v1.
	Host string
	// Owner is the GitHub user or organisation.
	Owner string
	// Repo is the repository name.
	Repo string
	// Subpath is an optional slash-separated path inside the repo. Empty
	// means "install the repo root".
	Subpath string
	// Ref is the user-supplied tag, branch, or commit reference. Empty
	// means "use the repo's default branch"; the github client resolves
	// it.
	Ref string
}

// String returns the canonical user-facing representation:
//
//	github.com/owner/repo[/subpath][@ref]
func (s *Source) String() string {
	out := fmt.Sprintf("%s/%s/%s", s.Host, s.Owner, s.Repo)
	if s.Subpath != "" {
		out += "/" + s.Subpath
	}
	if s.Ref != "" {
		out += "@" + s.Ref
	}
	return out
}

// Parse turns a user-facing source string into a Source. Accepted forms:
//
//	user/repo
//	user/repo@ref
//	user/repo/subpath
//	user/repo/subpath@ref
//	github.com/user/repo[/subpath][@ref]
//	https://github.com/user/repo[/subpath][@ref]
//
// Strings that are clearly local paths (start with ./, ../, /, contain a
// backslash, or equal "."/"..") yield ErrNotRemote so the caller can use the
// local-install path. Strings that look remote-shaped but reference an
// unsupported host return a regular error.
func Parse(s string) (*Source, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("fetch: empty source")
	}
	if looksLocal(s) {
		return nil, ErrNotRemote
	}

	rest := s
	switch {
	case strings.HasPrefix(rest, "https://"):
		rest = strings.TrimPrefix(rest, "https://")
	case strings.HasPrefix(rest, "http://"):
		return nil, errors.New("fetch: http:// is not supported; use https://")
	}

	// If the leading segment looks like a host (contains a dot before the
	// next slash) it must be github.com.
	if i := strings.IndexByte(rest, '/'); i > 0 {
		head := rest[:i]
		if strings.ContainsRune(head, '.') {
			if head != HostGitHub {
				return nil, fmt.Errorf(
					"fetch: v1 supports github.com only; see roadmap for other hosts (got %q)", head)
			}
			rest = rest[i+1:]
		}
	}

	// Split off the optional ref. Use the *last* @ so a branch name with an
	// @ (rare but legal) at the start does not eat the owner.
	ref := ""
	if at := strings.LastIndex(rest, "@"); at >= 0 {
		ref = strings.TrimSpace(rest[at+1:])
		rest = rest[:at]
	}
	if rest == "" {
		return nil, fmt.Errorf("fetch: missing owner/repo in %q", s)
	}

	parts := strings.SplitN(rest, "/", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("fetch: missing repo in %q", s)
	}
	owner, repo := parts[0], parts[1]
	repo = strings.TrimSuffix(repo, ".git")
	if owner == "" || repo == "" {
		return nil, fmt.Errorf("fetch: invalid owner/repo in %q", s)
	}
	if strings.ContainsAny(owner, "@:") || strings.ContainsAny(repo, "@:") {
		return nil, fmt.Errorf("fetch: invalid owner/repo in %q", s)
	}

	subpath := ""
	if len(parts) == 3 {
		subpath = strings.Trim(parts[2], "/")
	}

	return &Source{
		Host:    HostGitHub,
		Owner:   owner,
		Repo:    repo,
		Subpath: subpath,
		Ref:     ref,
	}, nil
}

// TarballURL returns the codeload.github.com URL for src's gzipped tar
// archive at resolvedRef. codeload is GitHub's canonical archive endpoint;
// it serves the tar without a redirect, which keeps the streaming download
// path simple.
func (s *Source) TarballURL(resolvedRef string) string {
	base := defaultArchiveBase
	return fmt.Sprintf("%s/%s/%s/tar.gz/%s", base, s.Owner, s.Repo, resolvedRef)
}

// defaultArchiveBase is the canonical archive host. Overridden in tests by
// the Client.Archive setting.
const defaultArchiveBase = "https://codeload.github.com"

func looksLocal(s string) bool {
	if s == "." || s == ".." {
		return true
	}
	if strings.ContainsRune(s, '\\') {
		return true
	}
	if strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") || strings.HasPrefix(s, "/") {
		return true
	}
	return false
}
