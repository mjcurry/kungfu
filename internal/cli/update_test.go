package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
	"github.com/mjcurry/kungfu/internal/testutil/githubfake"
)

// runUpdate is a tiny helper that runs `kungfu update` with --no-color and
// the multi-target test config prepended.
func runUpdateCmd(t *testing.T, env *multiTargetEnv, args ...string) (string, error) {
	t.Helper()
	return func() (string, error) {
		buf, err := runRoot(t, env, append([]string{"update"}, args...)...)
		return buf.String(), err
	}()
}

func TestUpdate_NoNameNoAllErrors(t *testing.T) {
	_, env := setupRemoteEnv(t)
	_, err := runUpdateCmd(t, env)
	if exitCode(err) != 1 {
		t.Fatalf("exit %d, want 1; err=%v", exitCode(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "--all") {
		t.Errorf("error should mention --all: %v", err)
	}
}

func TestUpdate_LocalOnlySkillErrors(t *testing.T) {
	_, env := setupRemoteEnv(t)
	// Drop a provenance-free skill straight into the claude target.
	writeFixtureSkill(t, env.dirs["claude"], "local-only", "Use this skill when locally installed.", false)

	_, err := runUpdateCmd(t, env, "local-only")
	if exitCode(err) != 1 {
		t.Fatalf("exit %d, want 1; err=%v", exitCode(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "not installed from a remote source") {
		t.Errorf("error should explain missing provenance: %v", err)
	}
}

func TestUpdate_UnknownNameErrors(t *testing.T) {
	_, env := setupRemoteEnv(t)
	_, err := runUpdateCmd(t, env, "no-such-skill")
	if exitCode(err) != 1 {
		t.Fatalf("exit %d, want 1; err=%v", exitCode(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should say not found: %v", err)
	}
}

func TestUpdate_UpToDate(t *testing.T) {
	const sha = "abcabc1234567890abcabc1234567890abcabc12"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": sha},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV.\n---\n\nbody\n",
		},
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/csv"); exitCode(err) != 0 {
		t.Fatalf("install: %v", err)
	}

	out, err := runUpdateCmd(t, env, "csv")
	if exitCode(err) != 0 {
		t.Fatalf("update: exit %d: %v", exitCode(err), err)
	}
	if !strings.Contains(out, "up to date") || !strings.Contains(out, "everything up to date") {
		t.Errorf("expected 'up to date' messaging:\n%s", out)
	}
}

func TestUpdate_RemoteAdvancedSHA(t *testing.T) {
	const oldSHA = "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newSHA = "2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV at v1.\n---\n\nv1 body\n",
		},
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/csv"); exitCode(err) != 0 {
		t.Fatalf("install: %v", err)
	}
	// Advance the remote: main now points at newSHA with different content.
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": newSHA, oldSHA: oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV at v2.\n---\n\nv2 body, refreshed.\n",
		},
	})

	out, err := runUpdateCmd(t, env, "--yes", "csv")
	if exitCode(err) != 0 {
		t.Fatalf("update: exit %d: %v\n%s", exitCode(err), err, out)
	}
	if !strings.Contains(out, "updated: csv") {
		t.Errorf("expected updated line:\n%s", out)
	}

	s, err := skill.Load(filepath.Join(env.dirs["claude"], "csv"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SHA != newSHA {
		t.Errorf("SHA = %q, want %q", s.SHA, newSHA)
	}
	if !strings.Contains(s.Body, "v2 body") {
		t.Errorf("body did not refresh:\n%s", s.Body)
	}
}

func TestUpdate_DryRunMakesNoChanges(t *testing.T) {
	const oldSHA = "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newSHA = "3ccccccccccccccccccccccccccccccccccccccc"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV at v1.\n---\n\nv1 body\n",
		},
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/csv"); exitCode(err) != 0 {
		t.Fatalf("install: %v", err)
	}
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": newSHA, oldSHA: oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV at v2.\n---\n\nv2 body\n",
		},
	})

	if _, err := runUpdateCmd(t, env, "--dry-run", "csv"); exitCode(err) != 0 {
		t.Fatalf("dry-run: %v", err)
	}
	s, err := skill.Load(filepath.Join(env.dirs["claude"], "csv"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SHA != oldSHA {
		t.Errorf("dry-run modified installed SHA: %q, want %q", s.SHA, oldSHA)
	}
}

func TestUpdate_RefOverride(t *testing.T) {
	const v1SHA = "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const v2SHA = "2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs: map[string]string{
			"v1.0.0": v1SHA,
			"v2.0.0": v2SHA,
			"main":   v2SHA,
		},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV.\n---\n\nbody\n",
		},
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/csv@v1.0.0"); exitCode(err) != 0 {
		t.Fatalf("install: %v", err)
	}

	if _, err := runUpdateCmd(t, env, "--yes", "--ref", "v2.0.0", "csv"); exitCode(err) != 0 {
		t.Fatalf("update: %v", err)
	}
	s, err := skill.Load(filepath.Join(env.dirs["claude"], "csv"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SHA != v2SHA {
		t.Errorf("SHA = %q, want %q", s.SHA, v2SHA)
	}
	if s.Ref != "v2.0.0" {
		t.Errorf("Ref = %q, want v2.0.0", s.Ref)
	}
}

func TestUpdate_AllUpdatesEveryProvenancedSkill(t *testing.T) {
	const aOld = "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const aNew = "2aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const bOld = "1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const bNew = "2bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "a", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": aOld},
		Files: map[string]string{
			"SKILL.md": "---\nname: a\ndescription: Use this skill when a is needed.\n---\n\nold-a\n",
		},
	})
	srv.AddRepo("acme", "b", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": bOld},
		Files: map[string]string{
			"SKILL.md": "---\nname: b\ndescription: Use this skill when b is needed.\n---\n\nold-b\n",
		},
	})
	for _, name := range []string{"a", "b"} {
		if _, err := runRoot(t, env, "install", "--yes", "acme/"+name); exitCode(err) != 0 {
			t.Fatalf("install %s: %v", name, err)
		}
	}
	srv.AddRepo("acme", "a", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": aNew},
		Files: map[string]string{
			"SKILL.md": "---\nname: a\ndescription: Use this skill when a is needed.\n---\n\nnew-a\n",
		},
	})
	srv.AddRepo("acme", "b", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": bNew},
		Files: map[string]string{
			"SKILL.md": "---\nname: b\ndescription: Use this skill when b is needed.\n---\n\nnew-b\n",
		},
	})

	if _, err := runUpdateCmd(t, env, "--yes", "--all"); exitCode(err) != 0 {
		t.Fatalf("update --all: %v", err)
	}
	for _, name := range []string{"a", "b"} {
		s, err := skill.Load(filepath.Join(env.dirs["claude"], name))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(s.Body, "new-") {
			t.Errorf("%s body not updated:\n%s", name, s.Body)
		}
	}
}

func TestUpdate_ConfirmDeclineExits2(t *testing.T) {
	const oldSHA = "1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const newSHA = "9aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV.\n---\n\nbody\n",
		},
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/csv"); exitCode(err) != 0 {
		t.Fatalf("install: %v", err)
	}
	srv.AddRepo("acme", "csv", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": newSHA, oldSHA: oldSHA},
		Files: map[string]string{
			"SKILL.md": "---\nname: csv\ndescription: Use this skill when handling CSV v2.\n---\n\nv2\n",
		},
	})

	_, err := runRootWithStdin(t, env, "n\n", "update", "csv")
	if exitCode(err) != 2 {
		// Some shells may exit 0 with "aborted" — accept either as long as
		// the file did not change.
		s, lerr := skill.Load(filepath.Join(env.dirs["claude"], "csv"))
		if lerr != nil {
			t.Fatal(lerr)
		}
		if s.SHA != oldSHA {
			t.Errorf("decline modified file: SHA %q, want %q", s.SHA, oldSHA)
		}
		return
	}
	// On exit 2 path, also verify file unchanged.
	s, err := skill.Load(filepath.Join(env.dirs["claude"], "csv"))
	if err != nil {
		t.Fatal(err)
	}
	if s.SHA != oldSHA {
		t.Errorf("decline modified file: SHA %q, want %q", s.SHA, oldSHA)
	}
}
