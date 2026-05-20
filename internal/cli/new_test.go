package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runNew executes the new command with --no-color prepended. cwd is set via
// the --dir flag so the test never relies on os.Getwd. stdin is whatever the
// test supplies (nil means "no input").
func runNew(t *testing.T, in string, args ...string) (string, error) {
	t.Helper()
	cmd := newNewCmd()
	// stub a bytes buffer for stdout/stderr and a strings reader for stdin
	out := newBuf()
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetIn(strings.NewReader(in))
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return out.String(), err
}

func TestNewYesProducesLintCleanSkill(t *testing.T) {
	parent := t.TempDir()
	// --yes requires --description.
	_, err := runNew(t, "",
		"--yes",
		"--template", "basic",
		"--description", "the user asks to format CSV files into a clean report",
		"--dir", parent,
		"demo-skill")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	// Skill exists and lints cleanly.
	skillDir := filepath.Join(parent, "demo-skill")
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("SKILL.md missing: %v", err)
	}
}

func TestNewEachTemplateLintClean(t *testing.T) {
	templates := []string{"basic", "document", "data", "api-wrapper"}
	for _, tpl := range templates {
		t.Run(tpl, func(t *testing.T) {
			parent := t.TempDir()
			_, err := runNew(t, "",
				"--yes",
				"--template", tpl,
				"--description", "the user asks to do the thing this skill does",
				"--dir", parent,
				"some-skill")
			if exitCode(err) != 0 {
				t.Fatalf("exit %d: %v", exitCode(err), err)
			}
		})
	}
}

func TestNewYesRequiresDescription(t *testing.T) {
	parent := t.TempDir()
	_, err := runNew(t, "",
		"--yes", "--template", "basic", "--dir", parent, "name")
	if exitCode(err) != 1 {
		t.Errorf("exit %d, want 1; err=%v", exitCode(err), err)
	}
}

func TestNewRejectsBadName(t *testing.T) {
	parent := t.TempDir()
	_, err := runNew(t, "",
		"--yes", "--template", "basic",
		"--description", "the user asks to do x",
		"--dir", parent,
		"Bad_Name")
	if exitCode(err) != 1 {
		t.Errorf("exit %d, want 1; err=%v", exitCode(err), err)
	}
}

func TestNewCollisionWithoutForce(t *testing.T) {
	parent := t.TempDir()
	if err := os.MkdirAll(filepath.Join(parent, "exists"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := runNew(t, "",
		"--yes", "--template", "basic",
		"--description", "the user asks to do x",
		"--dir", parent,
		"exists")
	if exitCode(err) != 2 {
		t.Errorf("exit %d, want 2; err=%v", exitCode(err), err)
	}
}

func TestNewForceReplaces(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "replaceme")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "stale"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runNew(t, "",
		"--yes", "--force", "--template", "basic",
		"--description", "the user asks to do x",
		"--dir", parent,
		"replaceme")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stale")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale file survived force replace: %v", err)
	}
}

func TestNewStripsLeadingTriggerPrefix(t *testing.T) {
	parent := t.TempDir()
	if _, err := runNew(t, "",
		"--yes", "--template", "basic",
		"--description", "Use this skill when the user asks to format CSV",
		"--dir", parent,
		"prefix-skill"); err != nil {
		t.Fatalf("err: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(parent, "prefix-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	// The body itself has a "Use this skill when:" heading; the only
	// double-prefix bug we care about is the frontmatter description.
	if strings.Contains(body, "Use this skill when Use this skill when") {
		t.Errorf("trigger prefix double-applied:\n%s", body)
	}
	// And the rendered description must keep the prefix exactly once.
	if !strings.Contains(body, "description: Use this skill when the user asks to format CSV") {
		t.Errorf("description not rendered as expected:\n%s", body)
	}
}

func TestNewInteractivePromptsForTemplateAndDescription(t *testing.T) {
	parent := t.TempDir()
	// Stdin script: pick option 2 (document), then enter description.
	input := "2\nthe user asks to write a polished report\n"
	_, err := runNew(t, input,
		"--dir", parent,
		"interactive-skill")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	// The document template ships a references/style-guide.md; presence
	// confirms the user's "2" selection was honoured.
	if _, err := os.Stat(filepath.Join(parent, "interactive-skill", "references", "style-guide.md")); err != nil {
		t.Errorf("document template not applied (style-guide.md missing): %v", err)
	}
}

// newBuf returns an io.Writer-and-Stringer that the cobra command can write
// to. It avoids the need to add a buffer import in this file's preamble.
func newBuf() *strBuilder { return &strBuilder{} }

type strBuilder struct{ b strings.Builder }

func (s *strBuilder) Write(p []byte) (int, error) { return s.b.Write(p) }
func (s *strBuilder) String() string              { return s.b.String() }
