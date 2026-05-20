package fetch

import (
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name      string
		in        string
		wantOwner string
		wantRepo  string
		wantSub   string
		wantRef   string
		wantErr   error  // sentinel via errors.Is, or nil
		wantSub2  string // substring required in non-sentinel error
	}{
		{name: "bare owner/repo", in: "user/repo", wantOwner: "user", wantRepo: "repo"},
		{name: "with ref", in: "user/repo@v1.0.0", wantOwner: "user", wantRepo: "repo", wantRef: "v1.0.0"},
		{name: "branch ref", in: "user/repo@main", wantOwner: "user", wantRepo: "repo", wantRef: "main"},
		{name: "short sha", in: "user/repo@abc1234", wantOwner: "user", wantRepo: "repo", wantRef: "abc1234"},
		{name: "subpath", in: "user/repo/path/to/skill", wantOwner: "user", wantRepo: "repo", wantSub: "path/to/skill"},
		{name: "subpath + ref", in: "user/repo/path/to/skill@v1", wantOwner: "user", wantRepo: "repo", wantSub: "path/to/skill", wantRef: "v1"},
		{name: "explicit host", in: "github.com/user/repo", wantOwner: "user", wantRepo: "repo"},
		{name: "explicit host + ref", in: "github.com/user/repo@v1.0.0", wantOwner: "user", wantRepo: "repo", wantRef: "v1.0.0"},
		{name: "https URL", in: "https://github.com/user/repo", wantOwner: "user", wantRepo: "repo"},
		{name: "https URL + ref", in: "https://github.com/user/repo@v1.0.0", wantOwner: "user", wantRepo: "repo", wantRef: "v1.0.0"},
		{name: "strips .git", in: "user/repo.git", wantOwner: "user", wantRepo: "repo"},
		{name: "branch with slash", in: "user/repo@feature/foo", wantOwner: "user", wantRepo: "repo", wantRef: "feature/foo"},

		// Negative cases.
		{name: "empty", in: "", wantSub2: "empty"},
		{name: "absolute path", in: "/usr/local/bin", wantErr: ErrNotRemote},
		{name: "dot-slash", in: "./local-skill", wantErr: ErrNotRemote},
		{name: "dot-dot-slash", in: "../sibling", wantErr: ErrNotRemote},
		{name: "lone dot", in: ".", wantErr: ErrNotRemote},
		{name: "windows path", in: `C:\Users\me\skill`, wantErr: ErrNotRemote},
		{name: "http scheme", in: "http://github.com/user/repo", wantSub2: "https"},
		{name: "wrong host", in: "gitlab.com/user/repo", wantSub2: "github.com only"},
		{name: "missing repo", in: "user", wantSub2: "missing repo"},
		{name: "empty owner", in: "/repo", wantErr: ErrNotRemote}, // looks local first
		{name: "empty repo", in: "user/", wantSub2: "owner/repo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want errors.Is %v", err, tc.wantErr)
				}
				return
			case tc.wantSub2 != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantSub2) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantSub2)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Owner != tc.wantOwner {
				t.Errorf("Owner = %q, want %q", got.Owner, tc.wantOwner)
			}
			if got.Repo != tc.wantRepo {
				t.Errorf("Repo = %q, want %q", got.Repo, tc.wantRepo)
			}
			if got.Subpath != tc.wantSub {
				t.Errorf("Subpath = %q, want %q", got.Subpath, tc.wantSub)
			}
			if got.Ref != tc.wantRef {
				t.Errorf("Ref = %q, want %q", got.Ref, tc.wantRef)
			}
			if got.Host != HostGitHub {
				t.Errorf("Host = %q, want %q", got.Host, HostGitHub)
			}
		})
	}
}

func TestSourceString(t *testing.T) {
	cases := []struct {
		src  Source
		want string
	}{
		{Source{Host: "github.com", Owner: "u", Repo: "r"}, "github.com/u/r"},
		{Source{Host: "github.com", Owner: "u", Repo: "r", Ref: "v1"}, "github.com/u/r@v1"},
		{Source{Host: "github.com", Owner: "u", Repo: "r", Subpath: "a/b"}, "github.com/u/r/a/b"},
		{Source{Host: "github.com", Owner: "u", Repo: "r", Subpath: "a/b", Ref: "v1"}, "github.com/u/r/a/b@v1"},
	}
	for _, tc := range cases {
		if got := tc.src.String(); got != tc.want {
			t.Errorf("String() = %q, want %q", got, tc.want)
		}
	}
}

func TestTarballURL(t *testing.T) {
	src := &Source{Host: "github.com", Owner: "u", Repo: "r"}
	want := "https://codeload.github.com/u/r/tar.gz/abc123"
	if got := src.TarballURL("abc123"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
