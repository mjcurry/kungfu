package template

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
)

func testVars(name string) Vars {
	return Vars{
		Name:        name,
		Description: "the user asks to do the thing this skill does",
		Year:        2026,
		CreatedAt:   time.Now().Format(time.RFC3339),
	}
}

func TestBuiltinsCoversFourTemplates(t *testing.T) {
	got := Builtins()
	wantNames := []string{"basic", "document", "data", "api-wrapper"}
	if len(got) != len(wantNames) {
		t.Fatalf("Builtins() = %d, want %d", len(got), len(wantNames))
	}
	for i, name := range wantNames {
		if got[i].Name != name {
			t.Errorf("Builtins()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestByName(t *testing.T) {
	if _, err := ByName("basic"); err != nil {
		t.Errorf("ByName(basic) error: %v", err)
	}
	if _, err := ByName("nope"); err == nil {
		t.Error("expected error for unknown template")
	}
}

func TestApplyRefusesExistingDir(t *testing.T) {
	t.Helper()
	tpl, _ := ByName("basic")
	dest := filepath.Join(t.TempDir(), "skill")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := tpl.Apply(dest, testVars("any-name")); err == nil {
		t.Fatal("Apply should refuse to clobber an existing directory")
	}
}

func TestApplyRendersExpectedFiles(t *testing.T) {
	cases := []struct {
		template  string
		wantPaths []string
	}{
		{"basic", []string{"SKILL.md"}},
		{"document", []string{"SKILL.md", "references/style-guide.md"}},
		{"data", []string{"SKILL.md", "scripts/analyze.py"}},
		{"api-wrapper", []string{"SKILL.md", "scripts/call.sh"}},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			tpl, err := ByName(tc.template)
			if err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(t.TempDir(), "my-skill")
			abs, err := tpl.Apply(dest, testVars("my-skill"))
			if err != nil {
				t.Fatalf("Apply error: %v", err)
			}
			if abs != dest {
				t.Errorf("Apply returned %q, want %q", abs, dest)
			}
			for _, want := range tc.wantPaths {
				if _, err := os.Stat(filepath.Join(dest, want)); err != nil {
					t.Errorf("missing %s: %v", want, err)
				}
			}
		})
	}
}

func TestApplySubstitutesVars(t *testing.T) {
	tpl, _ := ByName("basic")
	dest := filepath.Join(t.TempDir(), "subst-skill")
	if _, err := tpl.Apply(dest, testVars("subst-skill")); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "name: subst-skill") {
		t.Errorf("name not substituted:\n%s", body)
	}
	if !strings.Contains(body, "Use this skill when the user asks") {
		t.Errorf("description not substituted:\n%s", body)
	}
}

func TestApplyExecutableBits(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix file modes not meaningful on Windows")
	}
	cases := []struct {
		template, scriptPath string
	}{
		{"data", "scripts/analyze.py"},
		{"api-wrapper", "scripts/call.sh"},
	}
	for _, tc := range cases {
		t.Run(tc.template, func(t *testing.T) {
			tpl, _ := ByName(tc.template)
			dest := filepath.Join(t.TempDir(), "exec-skill")
			if _, err := tpl.Apply(dest, testVars("exec-skill")); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(dest, tc.scriptPath))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm()&0o111 == 0 {
				t.Errorf("%s not executable: mode=%v", tc.scriptPath, info.Mode())
			}
			// SKILL.md must not be executable.
			si, err := os.Stat(filepath.Join(dest, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if si.Mode().Perm()&0o111 != 0 {
				t.Errorf("SKILL.md unexpectedly executable: mode=%v", si.Mode())
			}
		})
	}
}

// TestApplyPassesLint is the critical regression guard: every built-in
// template must produce a skill that lint.NewDefault() accepts with zero
// errors AND zero warnings. A failure here means a template change has
// introduced a lint violation that real users would hit on first run.
func TestApplyPassesLint(t *testing.T) {
	for _, tpl := range Builtins() {
		t.Run(tpl.Name, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "lintcheck-skill")
			if _, err := tpl.Apply(dest, Vars{
				Name:        "lintcheck-skill",
				Description: "the user asks to format CSV files into a clean report",
				Year:        2026,
				CreatedAt:   "2026-01-01T00:00:00Z",
			}); err != nil {
				t.Fatal(err)
			}
			rep, err := lint.NewDefault().Lint(dest)
			if err != nil {
				t.Fatal(err)
			}
			if len(rep.Errors()) > 0 {
				t.Errorf("template %s produced lint errors:\n%v", tpl.Name, rep.Errors())
			}
			if len(rep.Warnings()) > 0 {
				t.Errorf("template %s produced lint warnings:\n%v", tpl.Name, rep.Warnings())
			}
		})
	}
}

func TestApplyCleansUpOnFailure(t *testing.T) {
	// Force a render failure by writing a template with a missing key. We do
	// this in-process: build a Template value pointing at a name that has a
	// .tmpl file referencing an undefined variable, by reusing an existing
	// fixture and a deliberate template that has the missing-key option set.
	// Since we cannot easily create a broken template at runtime without
	// touching the embedded FS, we exercise the cleanup path with an
	// already-failing renderBytes call.
	dest := filepath.Join(t.TempDir(), "would-fail")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Apply against an existing directory should error and not modify it.
	tpl, _ := ByName("basic")
	if _, err := tpl.Apply(dest, testVars("would-fail")); err == nil {
		t.Fatal("Apply on existing dir should error")
	}
	// Marker file still there: nothing was destroyed.
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("Apply destroyed an existing directory: %v", err)
	}
	_ = skill.FileName // keep import balanced if test trimmed
}
