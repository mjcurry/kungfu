package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// projectRoot climbs from the current working directory until it finds a
// go.mod, so `go test` from any package can build cmd/kungfu.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := dir; d != string(filepath.Separator); d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d
		}
	}
	t.Fatal("could not find project root (no go.mod up the tree)")
	return ""
}

// buildBinary compiles cmd/kungfu into a fresh temp file and returns its
// path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "kungfu")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/kungfu")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// runBinary executes the compiled kungfu binary with extraEnv appended onto
// os.Environ(), and returns stdout, stderr, and the resolved exit code.
func runBinary(t *testing.T, bin string, extraEnv []string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	var sout, serr bytes.Buffer
	cmd.Stdout = &sout
	cmd.Stderr = &serr
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return sout.String(), serr.String(), ee.ExitCode()
		}
		t.Fatalf("exec %s: %v\nstderr=%s", bin, err, serr.String())
	}
	return sout.String(), serr.String(), 0
}

// TestE2E_MultiTargetLifecycle drives the binary through a complete
// multi-target install → list → show → remove → list lifecycle, asserting
// each step's output and exit code.
func TestE2E_MultiTargetLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("e2e test skipped under -short")
	}

	bin := buildBinary(t)
	root := t.TempDir()
	dirs := map[string]string{
		"claude":  filepath.Join(root, "claude"),
		"codex":   filepath.Join(root, "codex"),
		"copilot": filepath.Join(root, "copilot"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := filepath.Join(root, "config.toml")
	// TOML literal strings (single quotes) for paths: avoids backslash
	// escape interpretation on Windows where tempdirs are C:\Users\...
	cfg := "default_targets = [\"claude\"]\ndefault_scope = \"personal\"\n\n"
	for _, name := range []string{"claude", "codex", "copilot"} {
		cfg += "[targets." + name + "]\npersonal_dir = '" + dirs[name] + "'\n\n"
	}
	cfg += "[targets.cursor]\nproject_dir = '.cursor/skills'\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	src := writeFixtureSkill(t, t.TempDir(), "demo", "Use this skill when demoing the lifecycle.", true)

	env := []string{
		"XDG_CONFIG_HOME=" + filepath.Join(root, "xdg"),
		"NO_COLOR=1",
	}
	common := []string{"--config", cfgPath, "--no-color"}

	// 1. Initial list — no skills installed.
	_, stderr, code := runBinary(t, bin, env, append(common, "list")...)
	if code != 0 {
		t.Fatalf("initial list: code=%d", code)
	}
	if !strings.Contains(stderr, "no skills installed") {
		t.Errorf("expected 'no skills installed' on stderr:\n%s", stderr)
	}

	// 2. Install to claude + codex.
	stdout, _, code := runBinary(t, bin, env, append(common, "--target", "claude,codex", "install", src)...)
	if code != 0 {
		t.Fatalf("install: code=%d\n%s", code, stdout)
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := os.Stat(filepath.Join(dirs[name], "demo", "SKILL.md")); err != nil {
			t.Errorf("%s missing SKILL.md after install: %v", name, err)
		}
	}

	// 3. List shows both rows.
	stdout, _, code = runBinary(t, bin, env, append(common, "list")...)
	if code != 0 {
		t.Fatalf("list: code=%d\n%s", code, stdout)
	}
	for _, want := range []string{"demo", "claude", "codex"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list missing %q:\n%s", want, stdout)
		}
	}

	// 4. show is ambiguous → exits 1.
	_, _, code = runBinary(t, bin, env, append(common, "show", "demo")...)
	if code != 1 {
		t.Errorf("ambiguous show: code=%d, want 1", code)
	}

	// 5. show with --target disambiguates.
	stdout, _, code = runBinary(t, bin, env, append(common, "--target", "claude", "show", "demo")...)
	if code != 0 {
		t.Fatalf("disambiguated show: code=%d\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "Body for demo") {
		t.Errorf("show output missing body:\n%s", stdout)
	}

	// 6. show --path prints the install directory.
	stdout, _, code = runBinary(t, bin, env, append(common, "--target", "claude", "show", "--path", "demo")...)
	if code != 0 {
		t.Fatalf("show --path: code=%d", code)
	}
	if got := strings.TrimSpace(stdout); got != filepath.Join(dirs["claude"], "demo") {
		t.Errorf("show --path = %q, want %q", got, filepath.Join(dirs["claude"], "demo"))
	}

	// 7. Remove from all matched targets.
	stdout, _, code = runBinary(t, bin, env, append(common, "remove", "--yes", "demo")...)
	if code != 0 {
		t.Fatalf("remove: code=%d\n%s", code, stdout)
	}
	for _, name := range []string{"claude", "codex"} {
		if _, err := os.Stat(filepath.Join(dirs[name], "demo")); err == nil {
			t.Errorf("%s still has the skill after remove", name)
		}
	}

	// 8. Final list — empty.
	_, stderr, code = runBinary(t, bin, env, append(common, "list")...)
	if code != 0 {
		t.Fatalf("final list: code=%d", code)
	}
	if !strings.Contains(stderr, "no skills installed") {
		t.Errorf("final list stderr missing 'no skills installed':\n%s", stderr)
	}
}
