package skill

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		dir     string
		wantErr error  // sentinel to match with errors.Is, or nil
		wantSub string // substring expected in the error message
		check   func(t *testing.T, s *Skill)
	}{
		{
			name: "valid skill",
			dir:  "testdata/valid",
			check: func(t *testing.T, s *Skill) {
				if s.Name != "hello-world" {
					t.Errorf("Name = %q, want %q", s.Name, "hello-world")
				}
				if !strings.HasPrefix(s.Description, "Use this skill when the user greets") {
					t.Errorf("Description = %q", s.Description)
				}
				if want := []string{"Read", "Bash"}; !reflect.DeepEqual(s.AllowedTools, want) {
					t.Errorf("AllowedTools = %v, want %v", s.AllowedTools, want)
				}
				if !strings.Contains(s.Body, "# Hello World") {
					t.Errorf("Body missing heading: %q", s.Body)
				}
				if strings.HasPrefix(s.Body, "\n") {
					t.Errorf("Body should not start with a blank line: %q", s.Body[:10])
				}
			},
		},
		{
			name:    "missing name",
			dir:     "testdata/missing-name",
			wantErr: ErrMissingName,
		},
		{
			name:    "malformed yaml",
			dir:     "testdata/malformed-yaml",
			wantSub: "parsing frontmatter",
		},
		{
			name:    "no frontmatter",
			dir:     "testdata/no-frontmatter",
			wantErr: ErrNoFrontmatter,
		},
		{
			name:    "unterminated frontmatter",
			dir:     "testdata/unterminated",
			wantErr: ErrUnterminatedFrontmatter,
		},
		{
			name: "extra unknown fields",
			dir:  "testdata/extra-fields",
			check: func(t *testing.T, s *Skill) {
				if s.Name != "extra-fields" {
					t.Errorf("Name = %q", s.Name)
				}
				if v, ok := s.Frontmatter["version"]; !ok || v != 2 {
					t.Errorf("Frontmatter[version] = %v (ok=%v), want 2", v, ok)
				}
				if v, ok := s.Frontmatter["experimental"]; !ok || v != true {
					t.Errorf("Frontmatter[experimental] = %v (ok=%v), want true", v, ok)
				}
				meta, ok := s.Frontmatter["metadata"].(map[string]any)
				if !ok {
					t.Fatalf("Frontmatter[metadata] = %T, want map", s.Frontmatter["metadata"])
				}
				if meta["author"] != "tank" {
					t.Errorf("metadata.author = %v, want tank", meta["author"])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := Load(tt.dir)
			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Load error = %v, want errors.Is %v", err, tt.wantErr)
				}
			case tt.wantSub != "":
				if err == nil || !strings.Contains(err.Error(), tt.wantSub) {
					t.Fatalf("Load error = %v, want substring %q", err, tt.wantSub)
				}
			default:
				if err != nil {
					t.Fatalf("Load() unexpected error: %v", err)
				}
				tt.check(t, s)
			}
		})
	}
}

func TestLoadMissingSkillFile(t *testing.T) {
	dir := t.TempDir() // empty: no SKILL.md
	_, err := Load(dir)
	if err == nil {
		t.Fatal("Load() on directory without SKILL.md: want error, got nil")
	}
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("error = %v, want it to mention the read failure", err)
	}
}

func TestLoadFile(t *testing.T) {
	s, err := LoadFile("testdata/valid/SKILL.md")
	if err != nil {
		t.Fatalf("LoadFile() error: %v", err)
	}
	if s.Name != "hello-world" {
		t.Errorf("Name = %q, want hello-world", s.Name)
	}
	if filepath.Base(s.Dir) != "valid" {
		t.Errorf("Dir = %q, want it to end in 'valid'", s.Dir)
	}
}

// copyFixture copies a fixture SKILL.md into a fresh temp dir and returns it.
func copyFixture(t *testing.T, fixtureDir string) string {
	t.Helper()
	src := filepath.Join(fixtureDir, FileName)
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", src, err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), data, 0o644); err != nil {
		t.Fatalf("writing fixture copy: %v", err)
	}
	return dir
}

func TestSaveRoundTripPreservesUnknownFields(t *testing.T) {
	dir := copyFixture(t, "testdata/extra-fields")

	orig, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if err := orig.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	reloaded, err := Load(dir)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}

	if !reflect.DeepEqual(orig.Frontmatter, reloaded.Frontmatter) {
		t.Errorf("frontmatter changed across round trip:\n orig = %#v\n new  = %#v",
			orig.Frontmatter, reloaded.Frontmatter)
	}
	if orig.Body != reloaded.Body {
		t.Errorf("body changed across round trip:\n orig = %q\n new  = %q", orig.Body, reloaded.Body)
	}
	for _, key := range []string{"version", "metadata", "experimental"} {
		if _, ok := reloaded.Frontmatter[key]; !ok {
			t.Errorf("unknown field %q lost across round trip", key)
		}
	}
}

func TestSavePersistsModeledChanges(t *testing.T) {
	dir := copyFixture(t, "testdata/extra-fields")

	s, err := Load(dir)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	s.Name = "renamed-skill"
	s.Description = "Updated description."
	s.AllowedTools = nil // removing the field
	if err := s.Save(); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	reloaded, err := Load(dir)
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if reloaded.Name != "renamed-skill" {
		t.Errorf("Name = %q, want renamed-skill", reloaded.Name)
	}
	if reloaded.Description != "Updated description." {
		t.Errorf("Description = %q", reloaded.Description)
	}
	if reloaded.AllowedTools != nil {
		t.Errorf("AllowedTools = %v, want nil after removal", reloaded.AllowedTools)
	}
	if _, ok := reloaded.Frontmatter["allowed-tools"]; ok {
		t.Errorf("allowed-tools key should have been deleted from frontmatter")
	}
	// Unknown fields must still survive a modeled change.
	if _, ok := reloaded.Frontmatter["metadata"]; !ok {
		t.Errorf("metadata lost after modifying a modeled field")
	}
}

func writeSkillDir(t *testing.T, root, name string) {
	t.Helper()
	d := filepath.Join(root, name)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", d, err)
	}
	content := "---\nname: " + name + "\ndescription: Fixture skill " + name + ".\n---\n\n# " + name + "\n"
	if err := os.WriteFile(filepath.Join(d, FileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", d, err)
	}
}

func TestDiscover(t *testing.T) {
	root := t.TempDir()
	writeSkillDir(t, root, "beta")
	writeSkillDir(t, root, "alpha")
	writeSkillDir(t, root, ".hidden") // skipped: hidden
	// A directory without a SKILL.md is skipped.
	if err := os.MkdirAll(filepath.Join(root, "notaskill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notaskill", "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A loose file is skipped.
	if err := os.WriteFile(filepath.Join(root, "loose.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}
	got := make([]string, len(skills))
	for i, s := range skills {
		got[i] = s.Name
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Discover() = %v, want %v (sorted, hidden/non-skill excluded)", got, want)
	}
}

func TestDiscoverPropagatesParseErrors(t *testing.T) {
	root := t.TempDir()
	bad := filepath.Join(root, "broken")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, FileName), []byte("no frontmatter here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(root); err == nil {
		t.Fatal("Discover() with a malformed skill: want error, got nil")
	}
}
