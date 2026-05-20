package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// MaxTarballSize caps tarball downloads. Skills are tiny; anything bigger
// than this is almost certainly the wrong thing and worth refusing.
const MaxTarballSize int64 = 100 * 1024 * 1024

// DefaultUserAgent is the User-Agent header sent on every API + archive
// request. GitHub returns 403 to requests without a UA.
const DefaultUserAgent = "kungfu"

// DefaultAPIBase is the canonical GitHub REST API host.
const DefaultAPIBase = "https://api.github.com"

// DefaultArchiveBase is the canonical GitHub archive host.
const DefaultArchiveBase = "https://codeload.github.com"

// Client talks to GitHub for ref resolution and tarball download. The
// BaseURL / Archive fields are injectable so tests can point the client at
// an httptest.Server.
type Client struct {
	HTTP      *http.Client
	BaseURL   string // REST API base (no trailing slash)
	Archive   string // tarball download base (no trailing slash)
	UserAgent string
	Token     string // optional GITHUB_TOKEN; empty means anonymous
	Cache     *Cache // optional tarball cache; nil writes to a temp file
}

// EnvAPIBase and EnvArchiveBase override the GitHub endpoint URLs. They
// exist for tests that want to point kungfu at an httptest.Server without
// the production code growing a test-only knob. Production callers leave
// them unset and get the real GitHub.
const (
	EnvAPIBase     = "KUNGFU_GITHUB_API_BASE"
	EnvArchiveBase = "KUNGFU_GITHUB_ARCHIVE_BASE"
)

// NewClient returns a Client wired with production defaults: api.github.com,
// codeload.github.com, a 60s HTTP timeout, and GITHUB_TOKEN if present. The
// GitHub endpoints may be overridden via the EnvAPIBase / EnvArchiveBase
// environment variables (used by tests).
func NewClient() *Client {
	api := DefaultAPIBase
	if v := os.Getenv(EnvAPIBase); v != "" {
		api = v
	}
	archive := DefaultArchiveBase
	if v := os.Getenv(EnvArchiveBase); v != "" {
		archive = v
	}
	return &Client{
		HTTP:      &http.Client{Timeout: 60 * time.Second},
		BaseURL:   api,
		Archive:   archive,
		UserAgent: DefaultUserAgent,
		Token:     os.Getenv("GITHUB_TOKEN"),
		Cache:     DefaultCache(),
	}
}

// ResolveRef returns the canonical 40-character commit SHA for src.Ref and a
// short label describing how it was resolved ("sha", "branch", "ref",
// "default-branch"). When src.Ref is empty, ResolveRef looks up the repo's
// default branch and resolves that.
func (c *Client) ResolveRef(ctx context.Context, src *Source) (string, string, error) {
	if src.Ref == "" {
		branch, err := c.fetchDefaultBranch(ctx, src)
		if err != nil {
			return "", "", err
		}
		sha, _, err := c.resolveSHA(ctx, src, branch)
		if err != nil {
			return "", "", err
		}
		return sha, "default-branch", nil
	}
	if isFullSHA(src.Ref) {
		return strings.ToLower(src.Ref), "sha", nil
	}
	sha, label, err := c.resolveSHA(ctx, src, src.Ref)
	if err != nil {
		return "", "", err
	}
	return sha, label, nil
}

// FetchTarball downloads or returns from cache the gzipped tar archive for
// src at sha. The returned path points at an on-disk file the caller can
// stream to Extract.
func (c *Client) FetchTarball(ctx context.Context, src *Source, sha string) (string, error) {
	if c.Cache != nil {
		if p, ok := c.Cache.Get(src, sha); ok {
			return p, nil
		}
	}
	url := fmt.Sprintf("%s/%s/%s/tar.gz/%s",
		strings.TrimRight(c.Archive, "/"), src.Owner, src.Repo, sha)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("fetch: building request: %w", err)
	}
	c.addAuthHeaders(req, "")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: tarball request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch: tarball download for %s/%s@%s: %s",
			src.Owner, src.Repo, sha, resp.Status)
	}
	if resp.ContentLength > MaxTarballSize {
		return "", fmt.Errorf("fetch: tarball is %d bytes; max %d",
			resp.ContentLength, MaxTarballSize)
	}
	body := &cappedReader{R: resp.Body, Max: MaxTarballSize}

	if c.Cache != nil {
		path, err := c.Cache.Put(src, sha, body)
		if err != nil {
			return "", err
		}
		if body.exceeded {
			_ = os.Remove(path)
			return "", fmt.Errorf("fetch: tarball exceeded %d bytes", MaxTarballSize)
		}
		return path, nil
	}

	tmp, err := os.CreateTemp("", "kungfu-tarball-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("fetch: temp file: %w", err)
	}
	if _, err := io.Copy(tmp, body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("fetch: streaming tarball: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return "", err
	}
	if body.exceeded {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("fetch: tarball exceeded %d bytes", MaxTarballSize)
	}
	return tmp.Name(), nil
}

func (c *Client) fetchDefaultBranch(ctx context.Context, src *Source) (string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s",
		strings.TrimRight(c.BaseURL, "/"), src.Owner, src.Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("fetch: building default-branch request: %w", err)
	}
	c.addAuthHeaders(req, "application/vnd.github+json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch: default-branch request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch: GET /repos/%s/%s: %s",
			src.Owner, src.Repo, resp.Status)
	}
	var data struct {
		DefaultBranch string `json:"default_branch"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&data); err != nil {
		return "", fmt.Errorf("fetch: decoding default-branch response: %w", err)
	}
	if data.DefaultBranch == "" {
		return "", fmt.Errorf("fetch: %s/%s reported empty default_branch", src.Owner, src.Repo)
	}
	return data.DefaultBranch, nil
}

func (c *Client) resolveSHA(ctx context.Context, src *Source, ref string) (string, string, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/commits/%s",
		strings.TrimRight(c.BaseURL, "/"), src.Owner, src.Repo, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", fmt.Errorf("fetch: building ref-resolve request: %w", err)
	}
	c.addAuthHeaders(req, "application/vnd.github.sha")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("fetch: ref-resolve request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("fetch: GET commits/%s: %s", ref, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512))
	if err != nil {
		return "", "", fmt.Errorf("fetch: reading SHA response: %w", err)
	}
	sha := strings.TrimSpace(string(body))
	if !isFullSHA(sha) {
		return "", "", fmt.Errorf("fetch: unexpected SHA response %q", sha)
	}
	return strings.ToLower(sha), "ref", nil
}

func (c *Client) addAuthHeaders(req *http.Request, accept string) {
	ua := c.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
}

// isFullSHA reports whether s is exactly 40 lowercase or uppercase hex
// characters — the GitHub commit SHA shape.
func isFullSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'f':
		case r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

// cappedReader is an io.Reader that errors after Max bytes. Used to enforce
// the tarball size cap without buffering.
type cappedReader struct {
	R        io.Reader
	Max      int64
	n        int64
	exceeded bool
}

func (r *cappedReader) Read(p []byte) (int, error) {
	if r.n >= r.Max {
		r.exceeded = true
		return 0, io.EOF
	}
	remain := r.Max - r.n
	if int64(len(p)) > remain+1 {
		p = p[:remain+1]
	}
	n, err := r.R.Read(p)
	r.n += int64(n)
	if r.n > r.Max {
		r.exceeded = true
		// Trim the overflow so we don't return more than Max bytes.
		n -= int(r.n - r.Max)
		r.n = r.Max
		return n, io.EOF
	}
	return n, err
}
