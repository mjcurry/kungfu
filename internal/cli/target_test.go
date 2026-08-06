package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetList_ShowsBuiltinsAndDefaults(t *testing.T) {
	env := setupMultiTargetEnv(t)
	out, err := runRoot(t, env, "target", "list")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}
	body := out.String()
	for _, name := range []string{"claude", "codex", "cursor", "copilot", "devin", "antigravity"} {
		if !strings.Contains(body, name) {
			t.Errorf("target list missing %q:\n%s", name, body)
		}
	}
	// claude is the default target in the test config; the row should be
	// marked and every builtin row labeled builtin.
	if !strings.Contains(body, "builtin") {
		t.Errorf("expected 'builtin' source column:\n%s", body)
	}
}

func TestTargetAdd_AppendsSectionAndRoundTrips(t *testing.T) {
	env := setupMultiTargetEnv(t)
	out, err := runRoot(t, env, "target", "add", "aider",
		"--personal-dir", "~/.aider/skills", "--project-dir", ".aider/skills")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}
	raw, readErr := os.ReadFile(env.configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(raw), "[targets.aider]") {
		t.Errorf("config missing [targets.aider]:\n%s", raw)
	}
	// The new target must be visible to a subsequent command run.
	listOut, err := runRoot(t, env, "target", "list")
	if exitCode(err) != 0 {
		t.Fatalf("target list after add: %v", err)
	}
	if !strings.Contains(listOut.String(), "aider") {
		t.Errorf("aider missing from target list:\n%s", listOut.String())
	}
	if !strings.Contains(listOut.String(), "custom") {
		t.Errorf("aider should be labeled custom:\n%s", listOut.String())
	}
}

func TestTargetAdd_PreservesExistingContentAndComments(t *testing.T) {
	env := setupMultiTargetEnv(t)
	// Plant a comment in the config; the textual append must keep it.
	raw, err := os.ReadFile(env.configPath)
	if err != nil {
		t.Fatal(err)
	}
	commented := "# my precious comment\n" + string(raw)
	if err := os.WriteFile(env.configPath, []byte(commented), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runRoot(t, env, "target", "add", "zed", "--project-dir", ".zed/skills"); exitCode(err) != 0 {
		t.Fatalf("add failed: %v", err)
	}
	after, err := os.ReadFile(env.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "# my precious comment") {
		t.Errorf("comment lost after target add:\n%s", after)
	}
	if !strings.Contains(string(after), "[targets.zed]") {
		t.Errorf("zed section missing:\n%s", after)
	}
}

func TestTargetAdd_RejectsBuiltinAndDuplicateAndBadInput(t *testing.T) {
	env := setupMultiTargetEnv(t)

	// Builtin collision.
	if _, err := runRoot(t, env, "target", "add", "claude", "--project-dir", "x"); exitCode(err) != 1 {
		t.Errorf("builtin name: exit %d, want 1", exitCode(err))
	}
	// Reserved word.
	if _, err := runRoot(t, env, "target", "add", "all", "--project-dir", "x"); exitCode(err) != 1 {
		t.Errorf("reserved 'all': exit %d, want 1", exitCode(err))
	}
	// Bad name format.
	if _, err := runRoot(t, env, "target", "add", "My_Agent", "--project-dir", "x"); exitCode(err) != 1 {
		t.Errorf("bad name: exit %d, want 1", exitCode(err))
	}
	// No directories at all.
	if _, err := runRoot(t, env, "target", "add", "bare"); exitCode(err) != 1 {
		t.Errorf("no dirs: exit %d, want 1", exitCode(err))
	}
	// Absolute project dir.
	abs := "/abs/skills"
	if _, err := runRoot(t, env, "target", "add", "absy", "--project-dir", abs); exitCode(err) != 1 {
		t.Errorf("absolute project dir: exit %d, want 1", exitCode(err))
	}
	// Duplicate custom section.
	if _, err := runRoot(t, env, "target", "add", "dupe", "--project-dir", ".dupe/skills"); exitCode(err) != 0 {
		t.Fatal("first dupe add should succeed")
	}
	if _, err := runRoot(t, env, "target", "add", "dupe", "--project-dir", ".dupe/skills"); exitCode(err) != 1 {
		t.Errorf("duplicate add: exit %d, want 1", exitCode(err))
	}
}

func TestTargetAdd_CreatesConfigFileWhenMissing(t *testing.T) {
	// Point --config at a file that does not exist yet; add must create
	// the parent directory and the file.
	setHomeForTest(t, t.TempDir())
	cfgPath := filepath.Join(t.TempDir(), "nested", "config.toml")
	env := &multiTargetEnv{configPath: cfgPath}
	out, err := runRoot(t, env, "target", "add", "goose",
		"--personal-dir", "~/.goose/skills")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}
	raw, readErr := os.ReadFile(cfgPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !strings.Contains(string(raw), "[targets.goose]") {
		t.Errorf("created config missing goose section:\n%s", raw)
	}
}

func TestTargetAdd_InstallableAfterAdd(t *testing.T) {
	// End-to-end: add a custom target, then install a local skill to it.
	env := setupMultiTargetEnv(t)
	personal := filepath.Join(t.TempDir(), "myagent-skills")
	if _, err := runRoot(t, env, "target", "add", "myagent",
		"--personal-dir", personal); exitCode(err) != 0 {
		t.Fatal("target add failed")
	}
	src := writeFixtureSkill(t, t.TempDir(), "hello", "Use this skill when greeting.", false)
	if _, err := runRoot(t, env, "--target", "myagent", "install", src); exitCode(err) != 0 {
		t.Fatal("install to custom target failed")
	}
	if _, err := os.Stat(filepath.Join(personal, "hello", "SKILL.md")); err != nil {
		t.Errorf("skill not installed to custom target dir: %v", err)
	}
}
