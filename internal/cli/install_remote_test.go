package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/skill"
	"github.com/mjcurry/kungfu/internal/testutil/githubfake"
)

// setupRemoteEnv starts a fake GitHub server and wires the fetch client's
// env-var seams + a private tarball cache at it. Returns the server and
// the multiTargetEnv so callers can compose with the existing library
// test helpers.
func setupRemoteEnv(t *testing.T) (*githubfake.Server, *multiTargetEnv) {
	t.Helper()
	srv := githubfake.NewServer()
	t.Cleanup(srv.Close)
	t.Setenv(fetch.EnvAPIBase, srv.APIBase())
	t.Setenv(fetch.EnvArchiveBase, srv.ArchiveBase())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return srv, setupMultiTargetEnv(t)
}

// minimalRepoFiles is the simplest lint-clean SKILL.md content the fake
// repos serve. The skill name matches the directory name on extraction.
func minimalRepoFiles(name string) map[string]string {
	return map[string]string{
		"SKILL.md": "---\nname: " + name +
			"\ndescription: Use this skill when validating remote installs.\n---\n\n" +
			"# " + name + "\n\nBody.\n",
	}
}

func TestRemoteInstall_DefaultBranch(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv-formatter", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "1111111111111111111111111111111111111111"},
		Files:         minimalRepoFiles("csv-formatter"),
	})

	out, err := runRoot(t, env, "install", "--yes", "acme/csv-formatter")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}

	dst := filepath.Join(env.dirs["claude"], "csv-formatter")
	data, err := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "kungfu_source: github.com/acme/csv-formatter") {
		t.Errorf("kungfu_source missing:\n%s", body)
	}
	// yaml.v3 may emit the SHA quoted or unquoted depending on its scalar
	// heuristics; assert key + value separately.
	if !strings.Contains(body, "kungfu_sha:") || !strings.Contains(body, "1111111111111111111111111111111111111111") {
		t.Errorf("kungfu_sha missing:\n%s", body)
	}
	if !strings.Contains(body, "kungfu_installed_at:") {
		t.Errorf("kungfu_installed_at missing:\n%s", body)
	}
}

func TestRemoteInstall_ExplicitRef(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "csv-formatter", githubfake.Repo{
		DefaultBranch: "main",
		Refs: map[string]string{
			"main":   "0000000000000000000000000000000000000000",
			"v1.0.0": "2222222222222222222222222222222222222222",
		},
		Files: minimalRepoFiles("csv-formatter"),
	})

	if _, err := runRoot(t, env, "install", "--yes", "acme/csv-formatter@v1.0.0"); exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	data, _ := os.ReadFile(filepath.Join(env.dirs["claude"], "csv-formatter", "SKILL.md"))
	body := string(data)
	if !strings.Contains(body, "kungfu_ref: v1.0.0") {
		t.Errorf("expected kungfu_ref: v1.0.0:\n%s", body)
	}
	// yaml.v3 sometimes quotes 40-digit-looking strings; accept either form.
	if !strings.Contains(body, "kungfu_sha:") || !strings.Contains(body, "2222222222222222222222222222222222222222") {
		t.Errorf("expected kungfu_sha matching v1.0.0 ref:\n%s", body)
	}
}

func TestRemoteInstall_Subpath(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "monorepo", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "3333333333333333333333333333333333333333"},
		Files: map[string]string{
			"README.md":                 "top-level readme",
			"skills/csv/SKILL.md":       "---\nname: csv\ndescription: Use this skill when extracting CSV.\n---\n\nBody.\n",
			"skills/csv/scripts/run.sh": "#!/bin/sh\necho\n",
			"skills/other/SKILL.md":     "unused",
		},
	})

	if _, err := runRoot(t, env, "install", "--yes", "acme/monorepo/skills/csv"); exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	dst := filepath.Join(env.dirs["claude"], "csv")
	for _, want := range []string{"SKILL.md", "scripts/run.sh"} {
		if _, err := os.Stat(filepath.Join(dst, want)); err != nil {
			t.Errorf("missing %s: %v", want, err)
		}
	}
	// Files outside the subpath must NOT be present.
	for _, no := range []string{"README.md", "skills"} {
		if _, err := os.Stat(filepath.Join(dst, no)); err == nil {
			t.Errorf("%s leaked out of subpath", no)
		}
	}
	// Provenance includes the subpath.
	data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if !strings.Contains(string(data), "kungfu_source: github.com/acme/monorepo/skills/csv") {
		t.Errorf("subpath missing from kungfu_source:\n%s", data)
	}
}

func TestRemoteInstall_CacheHitOnSecondInstall(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "demo", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "4444444444444444444444444444444444444444"},
		Files:         minimalRepoFiles("demo"),
	})
	// First install: populates cache.
	if _, err := runRoot(t, env, "install", "--yes", "acme/demo"); exitCode(err) != 0 {
		t.Fatalf("first install: exit %d: %v", exitCode(err), err)
	}
	cachedPath := filepath.Join(
		os.Getenv("XDG_CACHE_HOME"), "kungfu", "tarballs",
		"github.com", "acme", "demo",
		"4444444444444444444444444444444444444444.tar.gz",
	)
	stat1, err := os.Stat(cachedPath)
	if err != nil {
		t.Fatalf("cache file missing: %v", err)
	}

	// Second install with --force: should reuse cache (mtime unchanged).
	if _, err := runRoot(t, env, "install", "--yes", "--force", "acme/demo"); exitCode(err) != 0 {
		t.Fatalf("second install: exit %d: %v", exitCode(err), err)
	}
	stat2, _ := os.Stat(cachedPath)
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("cache file was rewritten on second install (mtime changed)")
	}
}

func TestRemoteInstall_NoCacheRefetches(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "demo", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "5555555555555555555555555555555555555555"},
		Files:         minimalRepoFiles("demo"),
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/demo"); exitCode(err) != 0 {
		t.Fatalf("first install: exit %d: %v", exitCode(err), err)
	}
	cachedPath := filepath.Join(
		os.Getenv("XDG_CACHE_HOME"), "kungfu", "tarballs",
		"github.com", "acme", "demo",
		"5555555555555555555555555555555555555555.tar.gz",
	)
	stat1, _ := os.Stat(cachedPath)
	// --no-cache bypasses the cache entirely (no read, no write). The
	// cache file from the first install must stay byte-identical after the
	// second install.
	if _, err := runRoot(t, env, "install", "--yes", "--no-cache", "--force", "acme/demo"); exitCode(err) != 0 {
		t.Fatalf("second install: exit %d: %v", exitCode(err), err)
	}
	stat2, _ := os.Stat(cachedPath)
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Errorf("--no-cache should not write to cache (file mtime changed)")
	}
}

func TestRemoteInstall_LintFailureBlocks(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	// description-missing is a content-based lint error that the rename-
	// to-skill-name machinery cannot mask (unlike name-mismatch).
	srv.AddRepo("acme", "no-desc", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "6666666666666666666666666666666666666666"},
		Files: map[string]string{
			"SKILL.md": "---\nname: no-desc\n---\n\nBody.\n",
		},
	})

	_, err := runRoot(t, env, "install", "--yes", "acme/no-desc")
	if exitCode(err) != 1 {
		t.Fatalf("exit %d, want 1; err=%v", exitCode(err), err)
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "no-desc")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("lint-failing skill leaked through to dest: %v", err)
	}
}

func TestRemoteInstall_AutoDiscoversNestedSkill(t *testing.T) {
	// Skill collections store one or more SKILL.md files in nested
	// directories rather than at the repo root. With exactly one nested
	// match, `kungfu install user/repo` should auto-pick it and bake the
	// discovered subpath into the kungfu_source provenance value.
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "single-nested", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "9deadbeefdeadbeefdeadbeefdeadbeefdeadbee"},
		Files: map[string]string{
			"README.md":               "top-level readme",
			"skills/csv/SKILL.md":     "---\nname: csv\ndescription: Use this skill when extracting CSV.\n---\n\nBody.\n",
			"skills/csv/scripts/x.sh": "#!/bin/sh\n",
		},
	})

	out, err := runRoot(t, env, "install", "--yes", "acme/single-nested")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}
	dst := filepath.Join(env.dirs["claude"], "csv")
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing at %s: %v", dst, err)
	}
	// The provenance must include the auto-discovered subpath so a later
	// `kungfu update` re-fetches the same nested skill.
	data, _ := os.ReadFile(filepath.Join(dst, "SKILL.md"))
	if !strings.Contains(string(data), "kungfu_source: github.com/acme/single-nested/skills/csv") {
		t.Errorf("kungfu_source missing auto-discovered subpath:\n%s", data)
	}
	if !strings.Contains(out.String(), "discovered skill") {
		t.Errorf("expected 'discovered skill' notice in output:\n%s", out.String())
	}
}

func TestRemoteInstall_MultipleSkillsRequireDisambiguation(t *testing.T) {
	// A repo with several SKILL.md files in different subdirectories
	// (e.g. anthropics/skills) is ambiguous. Auto-discovery refuses to
	// pick one and exits with a list pointing at the subpath syntax.
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "collection", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "cdeadbeefdeadbeefdeadbeefdeadbeefdeadbee"},
		Files: map[string]string{
			"skills/csv/SKILL.md":  "---\nname: csv\ndescription: Use this skill when CSV.\n---\n\nBody.\n",
			"skills/json/SKILL.md": "---\nname: json\ndescription: Use this skill when JSON.\n---\n\nBody.\n",
			"skills/yaml/SKILL.md": "---\nname: yaml\ndescription: Use this skill when YAML.\n---\n\nBody.\n",
		},
	})

	_, err := runRoot(t, env, "install", "--yes", "acme/collection")
	if exitCode(err) != 7 {
		t.Fatalf("exit %d, want 7; err=%v", exitCode(err), err)
	}
	if err == nil || !strings.Contains(err.Error(), "this repo contains 3 skills") {
		t.Errorf("expected disambiguation message naming 3 skills: %v", err)
	}
	for _, want := range []string{"skills/csv", "skills/json", "skills/yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should list %q: %v", want, err)
		}
	}
	// Nothing should have landed in any target.
	for _, name := range []string{"csv", "json", "yaml"} {
		if _, err := os.Stat(filepath.Join(env.dirs["claude"], name)); err == nil {
			t.Errorf("%s leaked through to claude despite ambiguous source", name)
		}
	}
}

func TestRemoteInstall_DisambiguationViaSubpathStillWorks(t *testing.T) {
	// Following the disambiguation hint, the user can pick one skill
	// explicitly via the subpath syntax.
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "collection", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "edeadbeefdeadbeefdeadbeefdeadbeefdeadbee"},
		Files: map[string]string{
			"skills/csv/SKILL.md":  "---\nname: csv\ndescription: Use this skill when CSV.\n---\n\nBody.\n",
			"skills/json/SKILL.md": "---\nname: json\ndescription: Use this skill when JSON.\n---\n\nBody.\n",
		},
	})

	if _, err := runRoot(t, env, "install", "--yes", "acme/collection/skills/json"); exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "json", "SKILL.md")); err != nil {
		t.Errorf("explicit-subpath skill not installed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "csv")); err == nil {
		t.Errorf("csv leaked through when only json was requested")
	}
}

func TestRemoteInstall_UnknownSourceExits6(t *testing.T) {
	_, env := setupRemoteEnv(t)
	// "not-a-source-string!" is neither a local path nor a valid GitHub
	// source (the trailing !). Parse should error and we exit 6.
	_, err := runRoot(t, env, "install", "--yes", "gitlab.com/user/repo")
	if exitCode(err) != 6 {
		t.Fatalf("exit %d, want 6; err=%v", exitCode(err), err)
	}
}

func TestRemoteInstall_DryRun(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "demo", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "7777777777777777777777777777777777777777"},
		Files:         minimalRepoFiles("demo"),
	})
	out, err := runRoot(t, env, "--target", "claude,codex", "install", "--yes", "--dry-run", "acme/demo")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	body := out.String()
	if !strings.Contains(body, "would install: demo → claude") || !strings.Contains(body, "would install: demo → codex") {
		t.Errorf("expected dry-run plan for both targets:\n%s", body)
	}
	// Nothing should actually be installed.
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "demo")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry-run created claude destination")
	}
}

func TestRemoteInstall_JSONListShowsProvenance(t *testing.T) {
	srv, env := setupRemoteEnv(t)
	srv.AddRepo("acme", "demo", githubfake.Repo{
		DefaultBranch: "main",
		Refs:          map[string]string{"main": "8888888888888888888888888888888888888888"},
		Files:         minimalRepoFiles("demo"),
	})
	if _, err := runRoot(t, env, "install", "--yes", "acme/demo"); exitCode(err) != 0 {
		t.Fatalf("install: exit %d: %v", exitCode(err), err)
	}
	// Loading the installed skill back via the skill package should expose
	// the provenance fields.
	s, err := skill.Load(filepath.Join(env.dirs["claude"], "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != "github.com/acme/demo" {
		t.Errorf("Source = %q", s.Source)
	}
	if s.SHA != "8888888888888888888888888888888888888888" {
		t.Errorf("SHA = %q", s.SHA)
	}
	if s.InstalledAt == "" {
		t.Errorf("InstalledAt is empty")
	}
	// And `list --json` includes the installed skill.
	out, _ := runRoot(t, env, "list", "--json")
	var items []map[string]any
	if err := json.Unmarshal([]byte(out.String()), &items); err != nil {
		t.Fatalf("invalid list --json: %v\n%s", err, out.String())
	}
	if len(items) != 1 || items[0]["name"] != "demo" {
		t.Errorf("list output: %v", items)
	}
}
