package fetch

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/testutil/githubfake"
)

func newTestClient(t *testing.T, srv *githubfake.Server) *Client {
	t.Helper()
	return &Client{
		HTTP:      http.DefaultClient,
		BaseURL:   srv.APIBase(),
		Archive:   srv.ArchiveBase(),
		UserAgent: "kungfu-test",
		Cache:     &Cache{Dir: t.TempDir()},
	}
}

func TestResolveRef_BranchAndTag(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	srv.AddRepo("u", "r", githubfake.Repo{
		DefaultBranch: "main",
		Refs: map[string]string{
			"main":   "1111111111111111111111111111111111111111",
			"v1.0.0": "2222222222222222222222222222222222222222",
		},
		Files: map[string]string{"SKILL.md": "---\nname: r\ndescription: ...\n---\n"},
	})
	c := newTestClient(t, srv)

	t.Run("branch resolves", func(t *testing.T) {
		sha, label, err := c.ResolveRef(context.Background(), &Source{Owner: "u", Repo: "r", Ref: "main"})
		if err != nil {
			t.Fatal(err)
		}
		if sha != "1111111111111111111111111111111111111111" {
			t.Errorf("sha = %q", sha)
		}
		if label != "ref" {
			t.Errorf("label = %q", label)
		}
	})
	t.Run("tag resolves", func(t *testing.T) {
		sha, _, err := c.ResolveRef(context.Background(), &Source{Owner: "u", Repo: "r", Ref: "v1.0.0"})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(sha, "2222222") {
			t.Errorf("sha = %q", sha)
		}
	})
}

func TestResolveRef_EmptyUsesDefaultBranch(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	srv.AddRepo("u", "r", githubfake.Repo{
		DefaultBranch: "trunk",
		Refs:          map[string]string{"trunk": "3333333333333333333333333333333333333333"},
		Files:         map[string]string{"SKILL.md": "x"},
	})
	c := newTestClient(t, srv)
	sha, label, err := c.ResolveRef(context.Background(), &Source{Owner: "u", Repo: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sha, "3333333") {
		t.Errorf("sha = %q", sha)
	}
	if label != "default-branch" {
		t.Errorf("label = %q", label)
	}
}

func TestResolveRef_FullSHAPassesThrough(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	c := newTestClient(t, srv)
	sha := "abcdef0123456789abcdef0123456789abcdef01"
	got, label, err := c.ResolveRef(context.Background(), &Source{Owner: "u", Repo: "r", Ref: sha})
	if err != nil {
		t.Fatal(err)
	}
	if got != sha {
		t.Errorf("sha = %q, want %q", got, sha)
	}
	if label != "sha" {
		t.Errorf("label = %q", label)
	}
}

func TestResolveRef_NotFound(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	srv.AddRepo("u", "r", githubfake.Repo{DefaultBranch: "main"})
	c := newTestClient(t, srv)
	_, _, err := c.ResolveRef(context.Background(), &Source{Owner: "u", Repo: "r", Ref: "nope"})
	if err == nil {
		t.Fatal("expected error for unknown ref")
	}
}

func TestFetchTarball_CachesAndReuses(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	srv.AddRepo("u", "r", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "4444444444444444444444444444444444444444"},
		Files:         map[string]string{"SKILL.md": "---\nname: r\ndescription: y\n---\n"},
	})
	c := newTestClient(t, srv)

	path1, err := c.FetchTarball(context.Background(),
		&Source{Owner: "u", Repo: "r", Host: HostGitHub},
		"4444444444444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
	st1, err := os.Stat(path1)
	if err != nil {
		t.Fatal(err)
	}

	// Second call should be served from cache: path identical, file
	// untouched.
	path2, err := c.FetchTarball(context.Background(),
		&Source{Owner: "u", Repo: "r", Host: HostGitHub},
		"4444444444444444444444444444444444444444")
	if err != nil {
		t.Fatal(err)
	}
	if path1 != path2 {
		t.Errorf("expected identical cache paths, got %q and %q", path1, path2)
	}
	st2, _ := os.Stat(path2)
	if st1.ModTime() != st2.ModTime() {
		t.Errorf("cache file was rewritten on second call")
	}
}

func TestFetchTarball_UnknownSHAErrors(t *testing.T) {
	srv := githubfake.NewServer()
	defer srv.Close()
	srv.AddRepo("u", "r", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "5555555555555555555555555555555555555555"},
	})
	c := newTestClient(t, srv)
	_, err := c.FetchTarball(context.Background(),
		&Source{Owner: "u", Repo: "r", Host: HostGitHub},
		"deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if err == nil {
		t.Fatal("expected error for unknown SHA")
	}
}

func TestIsFullSHA(t *testing.T) {
	good := "abcdef0123456789abcdef0123456789abcdef01"
	if !isFullSHA(good) {
		t.Errorf("good SHA not recognised")
	}
	for _, bad := range []string{"", "abc", strings.Repeat("g", 40), strings.Repeat("a", 39)} {
		if isFullSHA(bad) {
			t.Errorf("bad SHA %q recognised as good", bad)
		}
	}
}
