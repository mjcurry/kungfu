package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// multiTargetEnv represents a per-test sandbox: a config file plus dedicated
// personal-scope directories per target, so each test can drive the CLI in
// isolation from the user's real filesystem.
type multiTargetEnv struct {
	configPath string
	dirs       map[string]string // target name -> personal dir
}

// setupMultiTargetEnv writes a config file pointing each builtin target's
// personal_dir at a temp directory and returns the env handle.
func setupMultiTargetEnv(t *testing.T) *multiTargetEnv {
	t.Helper()
	root := t.TempDir()
	env := &multiTargetEnv{dirs: map[string]string{}}
	for _, name := range []string{"claude", "codex", "cursor", "copilot"} {
		env.dirs[name] = filepath.Join(root, name)
		if err := os.MkdirAll(env.dirs[name], 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Use TOML literal strings (single-quoted) for paths so backslashes
	// in Windows tempdir paths are not interpreted as escape sequences.
	cfg := "default_targets = [\"claude\"]\ndefault_scope = \"personal\"\n\n"
	for _, name := range []string{"claude", "codex", "copilot"} {
		cfg += "[targets." + name + "]\n"
		cfg += "personal_dir = '" + env.dirs[name] + "'\n\n"
	}
	// Leave cursor's personal_dir unset so the cursor-personal skip path
	// remains testable end-to-end.
	cfg += "[targets.cursor]\nproject_dir = '.cursor/skills'\n"

	env.configPath = filepath.Join(root, "config.toml")
	if err := os.WriteFile(env.configPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	// Isolate from the user's real XDG config and any leaked env state.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("KUNGFU_SKILLS_DIR", "")
	return env
}

// writeFixtureSkill creates a minimal valid skill at parent/name. Returns
// its absolute directory. withExec also drops an executable scripts/run.sh.
func writeFixtureSkill(t *testing.T, parent, name, desc string, withExec bool) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n# " + name + "\n\nBody for " + name + ".\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if withExec {
		scripts := filepath.Join(dir, "scripts")
		if err := os.MkdirAll(scripts, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(scripts, "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// runRoot executes the root command with --no-color and --config <env path>
// prepended onto args. Returns the captured combined stdout+stderr.
func runRoot(t *testing.T, env *multiTargetEnv, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	prefix := []string{"--no-color"}
	if env != nil {
		prefix = append(prefix, "--config", env.configPath)
	}
	cmd.SetArgs(append(prefix, args...))
	return buf, cmd.ExecuteContext(context.Background())
}

func runRootWithStdin(t *testing.T, env *multiTargetEnv, stdin string, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := NewRootCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetIn(strings.NewReader(stdin))
	prefix := []string{"--no-color", "--config", env.configPath}
	cmd.SetArgs(append(prefix, args...))
	return buf, cmd.ExecuteContext(context.Background())
}

// ---------------- install ----------------

func TestInstall_SingleTarget(t *testing.T) {
	env := setupMultiTargetEnv(t)
	src := writeFixtureSkill(t, t.TempDir(), "tool", "Use this skill when tooling.", true)

	out, err := runRoot(t, env, "install", src)
	if exitCode(err) != 0 {
		t.Fatalf("exit %d, want 0; out:\n%s", exitCode(err), out.String())
	}
	dst := filepath.Join(env.dirs["claude"], "tool")
	if _, err := os.Stat(filepath.Join(dst, "SKILL.md")); err != nil {
		t.Errorf("SKILL.md missing in claude target: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dst, "scripts", "run.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("scripts/run.sh not executable: mode=%v", info.Mode())
		}
	}
}

func TestInstall_MultiTarget(t *testing.T) {
	env := setupMultiTargetEnv(t)
	src := writeFixtureSkill(t, t.TempDir(), "shared", "Use this skill when sharing.", false)

	out, err := runRoot(t, env, "--target", "claude,codex", "install", src)
	if exitCode(err) != 0 {
		t.Fatalf("exit %d, want 0; out:\n%s", exitCode(err), out.String())
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := os.Stat(filepath.Join(env.dirs[name], "shared", "SKILL.md")); err != nil {
			t.Errorf("%s missing SKILL.md: %v", name, err)
		}
	}
}

func TestInstall_AllSkipsCursorPersonal(t *testing.T) {
	env := setupMultiTargetEnv(t)
	src := writeFixtureSkill(t, t.TempDir(), "broad", "Use this skill when broad.", false)

	out, err := runRoot(t, env, "--target", "all", "install", src)
	if exitCode(err) != 0 {
		t.Fatalf("exit %d, want 0; out:\n%s", exitCode(err), out.String())
	}
	if !strings.Contains(out.String(), "cursor") || !strings.Contains(out.String(), "skipped") {
		t.Errorf("expected cursor skip message:\n%s", out.String())
	}
	for _, name := range []string{"claude", "codex", "copilot"} {
		if _, err := os.Stat(filepath.Join(env.dirs[name], "broad", "SKILL.md")); err != nil {
			t.Errorf("%s missing SKILL.md: %v", name, err)
		}
	}
}

func TestInstall_RefusesConflictWithoutForce(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "conflict", "x", false)
	src := writeFixtureSkill(t, t.TempDir(), "conflict", "Use this skill when colliding.", false)

	_, err := runRoot(t, env, "install", src)
	if exitCode(err) != 2 {
		t.Fatalf("exit %d, want 2", exitCode(err))
	}
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should suggest --force: %v", err)
	}
}

func TestInstall_ForceReplaces(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "replace", "x", false)
	src := writeFixtureSkill(t, t.TempDir(), "replace", "Use this skill when colliding.", false)
	if err := os.WriteFile(filepath.Join(src, "marker"), []byte("from-src"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := runRoot(t, env, "install", "--force", src); exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	data, err := os.ReadFile(filepath.Join(env.dirs["claude"], "replace", "marker"))
	if err != nil || string(data) != "from-src" {
		t.Errorf("marker not installed: data=%q err=%v", data, err)
	}
}

func TestInstall_DryRunMakesNoChanges(t *testing.T) {
	env := setupMultiTargetEnv(t)
	src := writeFixtureSkill(t, t.TempDir(), "dry", "Use this skill when dry.", false)

	if _, err := runRoot(t, env, "install", "--dry-run", src); exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "dry")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("dry-run created the destination: %v", err)
	}
}

func TestInstall_LintFailureBlocks(t *testing.T) {
	env := setupMultiTargetEnv(t)
	// name-mismatch → lint error.
	dir := filepath.Join(t.TempDir(), "wrong-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: rightname\ndescription: Use this skill when broken.\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runRoot(t, env, "install", dir); exitCode(err) != 1 {
		t.Errorf("exit %d, want 1", exitCode(err))
	}
	if _, err := runRoot(t, env, "install", "--no-lint", dir); exitCode(err) != 0 {
		t.Errorf("with --no-lint: exit %d, want 0", exitCode(err))
	}
}

func TestInstall_PartialFailureExits3(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based read-only directory not portable on Windows")
	}
	env := setupMultiTargetEnv(t)
	src := writeFixtureSkill(t, t.TempDir(), "partial", "Use this skill when partial.", false)
	// Make codex's personal dir read-only so the rename inside it fails.
	if err := os.Chmod(env.dirs["codex"], 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(env.dirs["codex"], 0o755) })

	out, err := runRoot(t, env, "--target", "claude,codex", "install", src)
	if exitCode(err) != 3 {
		t.Fatalf("exit %d, want 3; out:\n%s", exitCode(err), out.String())
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "partial", "SKILL.md")); err != nil {
		t.Errorf("claude install should have succeeded: %v", err)
	}
}

// ---------------- list ----------------

func TestList_Empty(t *testing.T) {
	env := setupMultiTargetEnv(t)
	out, err := runRoot(t, env, "list")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if !strings.Contains(out.String(), "no skills installed") {
		t.Errorf("expected 'no skills installed': %s", out.String())
	}
}

func TestList_AcrossTargets(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "alpha", "Use this skill when alpha.", false)
	writeFixtureSkill(t, env.dirs["codex"], "beta", "Use this skill when beta.", false)

	out, err := runRoot(t, env, "list")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	for _, want := range []string{"alpha", "beta", "claude", "codex"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestList_TargetFilter(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "alpha", "Use this skill when alpha.", false)
	writeFixtureSkill(t, env.dirs["codex"], "beta", "Use this skill when beta.", false)

	out, err := runRoot(t, env, "--target", "codex", "list")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if strings.Contains(out.String(), "alpha") {
		t.Errorf("alpha should be hidden when --target codex:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "beta") {
		t.Errorf("beta missing:\n%s", out.String())
	}
}

func TestList_JSON(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "one", "Use this skill when one.", false)

	out, err := runRoot(t, env, "list", "--json")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	var items []map[string]any
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(items) != 1 || items[0]["name"] != "one" || items[0]["target"] != "claude" {
		t.Errorf("got %+v, want one entry named 'one' under claude", items)
	}
}

func TestList_MarksBrokenSkill(t *testing.T) {
	env := setupMultiTargetEnv(t)
	// name-mismatch → lint error.
	dir := filepath.Join(env.dirs["claude"], "wrong-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: rightname\ndescription: Use this skill when broken.\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runRoot(t, env, "list")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if !strings.Contains(out.String(), "[!]") {
		t.Errorf("broken marker '[!]' missing:\n%s", out.String())
	}
}

// ---------------- remove ----------------

func TestRemove_DefaultRemovesAllMatching(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "doomed", "x", false)
	writeFixtureSkill(t, env.dirs["codex"], "doomed", "x", false)

	if _, err := runRoot(t, env, "remove", "--yes", "doomed"); exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := os.Stat(filepath.Join(env.dirs[name], "doomed")); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s still has the skill: %v", name, err)
		}
	}
}

func TestRemove_TargetNarrows(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "stays", "x", false)
	writeFixtureSkill(t, env.dirs["codex"], "stays", "x", false)

	if _, err := runRoot(t, env, "--target", "codex", "remove", "--yes", "stays"); exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "stays", "SKILL.md")); err != nil {
		t.Errorf("claude should be untouched: %v", err)
	}
	if _, err := os.Stat(filepath.Join(env.dirs["codex"], "stays")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("codex should be removed: %v", err)
	}
}

func TestRemove_NotFound(t *testing.T) {
	env := setupMultiTargetEnv(t)
	_, err := runRoot(t, env, "remove", "--yes", "ghost")
	if exitCode(err) != 1 {
		t.Errorf("exit %d, want 1", exitCode(err))
	}
}

func TestRemove_DryRunKeeps(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "kept", "x", false)
	if _, err := runRoot(t, env, "remove", "--dry-run", "kept"); exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "kept", "SKILL.md")); err != nil {
		t.Errorf("dry-run removed: %v", err)
	}
}

func TestRemove_DeclinedPrompt(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "maybe", "x", false)
	_, err := runRootWithStdin(t, env, "n\n", "remove", "maybe")
	if exitCode(err) != 2 {
		t.Errorf("exit %d, want 2", exitCode(err))
	}
	if _, err := os.Stat(filepath.Join(env.dirs["claude"], "maybe", "SKILL.md")); err != nil {
		t.Errorf("declined remove should leave skill in place: %v", err)
	}
}

// ---------------- show ----------------

func TestShow_Path(t *testing.T) {
	env := setupMultiTargetEnv(t)
	want := writeFixtureSkill(t, env.dirs["claude"], "located", "Use this skill when locating.", false)

	out, err := runRoot(t, env, "show", "--path", "located")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestShow_Raw(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "raw", "Use this skill when raw.", false)

	out, err := runRoot(t, env, "show", "--raw", "raw")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if !strings.HasPrefix(out.String(), "---\n") {
		t.Errorf("--raw should print verbatim:\n%s", out.String())
	}
}

func TestShow_NotFound(t *testing.T) {
	env := setupMultiTargetEnv(t)
	_, err := runRoot(t, env, "show", "ghost")
	if exitCode(err) != 1 {
		t.Errorf("exit %d, want 1", exitCode(err))
	}
}

func TestShow_AmbiguousExits1(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "dup", "x", false)
	writeFixtureSkill(t, env.dirs["codex"], "dup", "x", false)

	_, err := runRoot(t, env, "show", "dup")
	if exitCode(err) != 1 {
		t.Fatalf("exit %d, want 1", exitCode(err))
	}
	if err == nil || !strings.Contains(err.Error(), "multiple targets") {
		t.Errorf("expected disambiguation message: %v", err)
	}
}

func TestShow_TargetDisambiguates(t *testing.T) {
	env := setupMultiTargetEnv(t)
	writeFixtureSkill(t, env.dirs["claude"], "dup", "Use this skill when claudish.", false)
	writeFixtureSkill(t, env.dirs["codex"], "dup", "Use this skill when codexy.", false)

	out, err := runRoot(t, env, "--target", "claude", "show", "dup")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d", exitCode(err))
	}
	if !strings.Contains(out.String(), "claudish") {
		t.Errorf("expected claude version selected:\n%s", out.String())
	}
}
