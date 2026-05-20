package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill creates a skill directory named `name` under parent and writes
// the given frontmatter description and markdown body into its SKILL.md.
func writeSkill(t *testing.T, parent, name, desc, body string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	content := "---\nname: " + name + "\ndescription: " + desc + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing SKILL.md: %v", err)
	}
	return dir
}

func runLint(t *testing.T, args ...string) (*bytes.Buffer, error) {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := newLintCmd()
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	return buf, cmd.Execute()
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return -1
}

func TestLintCmd_CleanSkillExits0(t *testing.T) {
	dir := writeSkill(t, t.TempDir(), "clean-skill",
		"Use this skill when extracting tables from PDF files.",
		"# Clean Skill\n\nDo the thing.\n")

	out, err := runLint(t, dir)
	if got := exitCode(err); got != 0 {
		t.Fatalf("exit code = %d, want 0; output:\n%s\nerr: %v", got, out.String(), err)
	}
	if !strings.Contains(out.String(), "no issues") {
		t.Errorf("expected 'no issues' in output, got: %s", out.String())
	}
}

func TestLintCmd_SkillWithErrorsExits1(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "wrong-dir")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: rightname\ndescription: Use this skill when testing.\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runLint(t, dir)
	if got := exitCode(err); got != 1 {
		t.Fatalf("exit code = %d, want 1; output:\n%s", got, out.String())
	}
	if !strings.Contains(out.String(), "name-mismatch") {
		t.Errorf("expected name-mismatch in output: %s", out.String())
	}
}

func TestLintCmd_StrictPromotesWarnings(t *testing.T) {
	dir := writeSkill(t, t.TempDir(), "warn-skill",
		"Handles all sorts of formatting tasks.",
		"# Warn\n\nbody\n")

	if _, err := runLint(t, dir); exitCode(err) != 0 {
		t.Fatalf("without --strict: exit %d, want 0", exitCode(err))
	}
	if _, err := runLint(t, "--strict", dir); exitCode(err) != 2 {
		t.Fatalf("with --strict: exit %d, want 2", exitCode(err))
	}
}

func TestLintCmd_MissingPathExits3(t *testing.T) {
	bogus := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := runLint(t, bogus); exitCode(err) != 3 {
		t.Fatalf("exit code = %d, want 3", exitCode(err))
	}
}

func TestLintCmd_JSONOutputIsValid(t *testing.T) {
	dir := writeSkill(t, t.TempDir(), "json-skill",
		"Use this skill when validating JSON output.",
		"# JSON Skill\n\nbody\n")

	out, err := runLint(t, "--json", dir)
	if got := exitCode(err); got != 0 {
		t.Fatalf("exit = %d, want 0", got)
	}
	var parsed struct {
		Path        string          `json:"path"`
		Diagnostics json.RawMessage `json:"diagnostics"`
		Summary     struct {
			Errors   int `json:"errors"`
			Warnings int `json:"warnings"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if parsed.Summary.Errors != 0 || parsed.Summary.Warnings != 0 {
		t.Errorf("summary = %+v, want all zero", parsed.Summary)
	}
}

func TestLintCmd_FixNormalizesTrailingWhitespace(t *testing.T) {
	dir := writeSkill(t, t.TempDir(), "fix-skill",
		"Use this skill when testing fix.",
		"# Body  \n\nLine with trailing space   \n")
	skillFile := filepath.Join(dir, "SKILL.md")

	before, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(before), "Body  \n") {
		t.Fatalf("fixture lost trailing whitespace before fix: %q", before)
	}
	if _, err := runLint(t, "--fix", dir); exitCode(err) != 0 {
		t.Fatalf("fix exit = %d; err=%v", exitCode(err), err)
	}
	after, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "Body  \n") || strings.Contains(string(after), "space   \n") {
		t.Errorf("trailing whitespace not removed:\n%s", after)
	}
}
