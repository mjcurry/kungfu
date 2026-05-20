// Package githubfake provides an httptest-backed stand-in for the subset
// of GitHub's API and archive endpoints kungfu's fetch package consumes.
// Tests register a repository's files and refs once and the server then
// answers ref-resolution and tarball requests deterministically.
package githubfake

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"sync"
	"time"
)

// Repo is a fake repository: a default branch, a name -> SHA map for tags
// and branches, and a path -> content tree the server packs into tarballs.
type Repo struct {
	// DefaultBranch is returned from GET /repos/<owner>/<name>.
	DefaultBranch string
	// Refs maps branch / tag / short-SHA names to a canonical 40-char SHA.
	// The fake also accepts the SHAs themselves (any value listed) as a
	// lookup key.
	Refs map[string]string
	// Files maps repo-relative paths to file contents. Paths must use
	// forward slashes.
	Files map[string]string
}

// Server is an httptest.Server backed by an in-memory repository set. The
// API root is APIBase(), the archive root is ArchiveBase(); both point at
// the same underlying listener but use different path prefixes so the
// fetch.Client can be pointed at them via BaseURL / Archive.
type Server struct {
	HTTP  *httptest.Server
	repos map[string]Repo
	mu    sync.Mutex
}

// NewServer starts a Server with no repos registered.
func NewServer() *Server {
	s := &Server{repos: map[string]Repo{}}
	s.HTTP = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// Close stops the underlying httptest.Server.
func (s *Server) Close() { s.HTTP.Close() }

// APIBase returns the URL fetch.Client should use as BaseURL.
func (s *Server) APIBase() string { return s.HTTP.URL + "/api" }

// ArchiveBase returns the URL fetch.Client should use as Archive.
func (s *Server) ArchiveBase() string { return s.HTTP.URL + "/archive" }

// AddRepo registers (or replaces) the named repo's contents.
func (s *Server) AddRepo(owner, name string, repo Repo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.repos[owner+"/"+name] = repo
}

// handle is the shared HTTP handler. The path prefix selects whether to
// serve API or archive responses.
func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasPrefix(r.URL.Path, "/api/repos/"):
		s.serveAPI(w, r, strings.TrimPrefix(r.URL.Path, "/api/repos/"))
	case strings.HasPrefix(r.URL.Path, "/archive/"):
		s.serveArchive(w, r, strings.TrimPrefix(r.URL.Path, "/archive/"))
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	owner, name := parts[0], parts[1]
	s.mu.Lock()
	repo, ok := s.repos[owner+"/"+name]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 2 {
		// GET /repos/<owner>/<name>
		fmt.Fprintf(w, `{"default_branch":%q}`, repo.DefaultBranch)
		return
	}
	if parts[2] == "commits" && len(parts) == 4 {
		ref := parts[3]
		sha, ok := repo.Refs[ref]
		if !ok {
			// Maybe ref is itself a SHA we know.
			for _, knownSHA := range repo.Refs {
				if knownSHA == ref {
					sha = ref
					ok = true
					break
				}
			}
		}
		if !ok {
			http.NotFound(w, r)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "vnd.github.sha") {
			http.Error(w, "expected vnd.github.sha Accept header", http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, sha)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) serveArchive(w http.ResponseWriter, r *http.Request, rest string) {
	// /archive/<owner>/<name>/tar.gz/<sha>
	parts := strings.SplitN(rest, "/", 4)
	if len(parts) != 4 || parts[2] != "tar.gz" {
		http.NotFound(w, r)
		return
	}
	owner, name, sha := parts[0], parts[1], parts[3]
	s.mu.Lock()
	repo, ok := s.repos[owner+"/"+name]
	s.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	known := false
	for _, v := range repo.Refs {
		if v == sha {
			known = true
			break
		}
	}
	if !known {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	prefix := name + "-" + sha
	if err := tw.WriteHeader(&tar.Header{
		Name:     prefix + "/",
		Mode:     0o755,
		Typeflag: tar.TypeDir,
		ModTime:  time.Now(),
	}); err != nil {
		return
	}
	// Emit files in sorted-ish order: directories first via WriteHeader as
	// needed. tar archives don't require explicit directories; writers
	// auto-create on extract via filepath.MkdirAll. We rely on that.
	for relPath, content := range repo.Files {
		entry := prefix + "/" + strings.TrimPrefix(path.Clean(relPath), "/")
		if err := tw.WriteHeader(&tar.Header{
			Name:     entry,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			ModTime:  time.Now(),
		}); err != nil {
			return
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return
		}
	}
}
