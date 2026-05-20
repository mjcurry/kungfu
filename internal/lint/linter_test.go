package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

// stubRule is a deterministic rule used to test orchestration without
// depending on the real rule set.
type stubRule struct {
	id    string
	diags []Diagnostic
}

func (r stubRule) ID() string                      { return r.id }
func (r stubRule) Check(*skill.Skill) []Diagnostic { return r.diags }
func (r stubRule) emit(d ...Diagnostic) stubRule {
	r.diags = append(r.diags, d...)
	return r
}

func writeSKILL(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, skill.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLintHappyPathRunsAllRules(t *testing.T) {
	dir := writeSKILL(t, "---\nname: hello\ndescription: x\n---\n\nbody\n")

	r1 := stubRule{id: "a/one"}.emit(Diagnostic{Rule: "a/one", Severity: SeverityWarning, Path: dir + "/SKILL.md", Message: "warn"})
	r2 := stubRule{id: "b/two"}.emit(Diagnostic{Rule: "b/two", Severity: SeverityError, Path: dir + "/SKILL.md", Message: "err"})

	rep, err := New(r1, r2).Lint(dir)
	if err != nil {
		t.Fatalf("Lint() error: %v", err)
	}
	if len(rep.Diagnostics) != 2 {
		t.Fatalf("got %d diagnostics, want 2", len(rep.Diagnostics))
	}
	if !rep.HasErrors() {
		t.Error("HasErrors() = false, want true")
	}
	if len(rep.Errors()) != 1 || len(rep.Warnings()) != 1 {
		t.Errorf("Errors/Warnings counts wrong: %d/%d", len(rep.Errors()), len(rep.Warnings()))
	}
}

func TestNewDefaultHasStandardRules(t *testing.T) {
	l := NewDefault()
	if len(l.Rules()) < 5 {
		t.Errorf("NewDefault() returned %d rules; expected the standard set", len(l.Rules()))
	}
}

func TestLintMissingFrontmatterShortCircuits(t *testing.T) {
	dir := writeSKILL(t, "# Just markdown, no frontmatter\n")
	r := stubRule{id: "should/not-run"}.emit(Diagnostic{Rule: "should/not-run", Severity: SeverityError, Message: "do not show"})

	rep, err := New(r).Lint(dir)
	if err != nil {
		t.Fatalf("Lint() error: %v", err)
	}
	if len(rep.Diagnostics) != 1 {
		t.Fatalf("expected 1 diagnostic, got %v", rep.Diagnostics)
	}
	if rep.Diagnostics[0].Rule != "frontmatter/missing" {
		t.Errorf("got rule %q, want frontmatter/missing", rep.Diagnostics[0].Rule)
	}
}

func TestLintUnterminatedFrontmatterShortCircuits(t *testing.T) {
	dir := writeSKILL(t, "---\nname: x\n\n# missing close fence\n")
	rep, err := New().Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Diagnostics) != 1 || rep.Diagnostics[0].Rule != "frontmatter/malformed" {
		t.Errorf("got %v, want frontmatter/malformed", rep.Diagnostics)
	}
}

func TestLintMalformedYAMLShortCircuits(t *testing.T) {
	dir := writeSKILL(t, "---\nname: [a, b\ndescription: x\n---\n\nbody\n")
	rep, err := New().Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Diagnostics) != 1 || rep.Diagnostics[0].Rule != "frontmatter/malformed" {
		t.Fatalf("got %v, want frontmatter/malformed", rep.Diagnostics)
	}
	if rep.Diagnostics[0].Line == 0 {
		t.Errorf("expected non-zero line number from yaml.v3 error")
	}
}

func TestLintMissingDirIsError(t *testing.T) {
	if _, err := New().Lint(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected error for missing directory")
	}
}

func TestLintRequiresDirectory(t *testing.T) {
	f := filepath.Join(t.TempDir(), "a-file.md")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Lint(f); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Errorf("expected 'not a directory' error, got %v", err)
	}
}

func TestReportSortOrder(t *testing.T) {
	dir := writeSKILL(t, "---\nname: x\ndescription: y\n---\n\nbody\n")
	skillPath := filepath.Join(dir, skill.FileName)
	r := stubRule{id: "x"}.emit(
		Diagnostic{Path: skillPath, Line: 10, Severity: SeverityWarning, Rule: "x/b"},
		Diagnostic{Path: skillPath, Line: 5, Severity: SeverityWarning, Rule: "x/a"},
		Diagnostic{Path: skillPath, Line: 10, Severity: SeverityError, Rule: "x/c"},
	)
	rep, err := New(r).Lint(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{rep.Diagnostics[0].Rule, rep.Diagnostics[1].Rule, rep.Diagnostics[2].Rule}
	if got[0] != "x/a" || got[1] != "x/c" || got[2] != "x/b" {
		t.Errorf("sort order = %v, want [x/a x/c x/b]", got)
	}
}
