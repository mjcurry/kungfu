package rules

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

// writeRefSkill creates a SKILL.md with the given body and optional extra
// files under a temp directory, then returns the skill dir.
func writeRefSkill(t *testing.T, body string, extras map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	content := "---\nname: " + filepath.Base(dir) + "\ndescription: Use this skill when testing references.\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(dir, skill.FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	for rel, data := range extras {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestReferencesBroken(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		extras    map[string]string
		wantHits  int
		wantInMsg string
	}{
		{"no refs", "# Hello\n\nNo links here.\n", nil, 0, ""},
		{"link to existing file passes",
			"See [the script](scripts/run.sh).\n",
			map[string]string{"scripts/run.sh": "#!/bin/sh\necho\n"},
			0, ""},
		{"link to missing file flagged",
			"See [the script](scripts/missing.sh).\n",
			nil, 1, "scripts/missing.sh"},
		{"external URL ignored", "See [docs](https://example.com).\n", nil, 0, ""},
		{"anchor ignored", "See [section](#background).\n", nil, 0, ""},
		{"absolute path ignored", "See [config](/etc/foo.toml).\n", nil, 0, ""},
		{"mailto ignored", "Email [me](mailto:me@example.com).\n", nil, 0, ""},
		{"link with fragment to existing file passes",
			"See [section](docs/manual.md#install).\n",
			map[string]string{"docs/manual.md": "# Manual\n"},
			0, ""},
		{"path-like code span resolves",
			"Run `scripts/run.sh` to start.\n",
			map[string]string{"scripts/run.sh": ""},
			0, ""},
		{"path-like code span flagged",
			"Run `scripts/missing.sh` to start.\n",
			nil, 1, "scripts/missing.sh"},
		{"non-path code span ignored", "Pass the `--help` flag.\n", nil, 0, ""},
		{"known extension flagged", "Open `notes.md` for context.\n", nil, 1, ""},
		// Slash-command names look path-like (have a `/`) but are not file
		// references — they should be skipped.
		{"slash command ignored", "Run `/my-skill` to start.\n", nil, 0, ""},
		{"slash command with namespace ignored",
			"Run `/ckm:brand` to start.\n", nil, 0, ""},
		// Template placeholders can never resolve statically; skip them so
		// they do not produce noise on third-party skills.
		{"template placeholder ignored",
			"See `references/{subcommand}.md`.\n", nil, 0, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeRefSkill(t, tc.body, tc.extras)
			diags := ReferencesBroken{}.Check(&skill.Skill{Dir: dir})
			if len(diags) != tc.wantHits {
				t.Fatalf("got %d diags, want %d: %v", len(diags), tc.wantHits, diags)
			}
			if tc.wantInMsg != "" && !strings.Contains(diags[0].Message, tc.wantInMsg) {
				t.Errorf("message %q missing %q", diags[0].Message, tc.wantInMsg)
			}
		})
	}
}

func TestReferencesBrokenLineNumber(t *testing.T) {
	body := "# Heading\n" +
		"\n" +
		"Para one.\n" +
		"\n" +
		"See [the script](scripts/missing.sh) for details.\n"
	dir := writeRefSkill(t, body, nil)
	diags := ReferencesBroken{}.Check(&skill.Skill{Dir: dir})
	if len(diags) != 1 {
		t.Fatalf("got %v", diags)
	}
	// Frontmatter = 4 lines (--- + 2 keys + ---), blank line, then body
	// starts at file line 6. The link is on body line 5, so file line 10.
	if diags[0].Line != 10 {
		t.Errorf("Line = %d, want 10", diags[0].Line)
	}
}
