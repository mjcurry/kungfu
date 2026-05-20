package target

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuiltins(t *testing.T) {
	got := Builtins()
	wantNames := []string{"claude", "codex", "cursor", "copilot"}
	if len(got) != len(wantNames) {
		t.Fatalf("Builtins() = %d targets, want %d", len(got), len(wantNames))
	}
	for i, name := range wantNames {
		if got[i].Name != name {
			t.Errorf("Builtins()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
	// Cursor explicitly has no personal directory.
	for _, tgt := range got {
		if tgt.Name == "cursor" && tgt.PersonalDir != "" {
			t.Errorf("cursor.PersonalDir = %q, want empty", tgt.PersonalDir)
		}
	}
}

func TestByName(t *testing.T) {
	targets := Builtins()
	t.Run("found", func(t *testing.T) {
		got, err := ByName("codex", targets)
		if err != nil {
			t.Fatal(err)
		}
		if got.Name != "codex" {
			t.Errorf("Name = %q, want codex", got.Name)
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, err := ByName("aider", targets)
		if err == nil {
			t.Fatal("expected error for unknown target")
		}
	})
}

func TestResolve(t *testing.T) {
	targets := Builtins()

	cases := []struct {
		name      string
		flag      string
		wantNames []string
		wantErr   string // substring; empty = no error
	}{
		{"single", "claude", []string{"claude"}, ""},
		{"multi", "claude,codex", []string{"claude", "codex"}, ""},
		{"spaces", " claude , codex ", []string{"claude", "codex"}, ""},
		{"all", "all", []string{"claude", "codex", "cursor", "copilot"}, ""},
		{"declaration order independent of input order",
			"copilot,claude", []string{"claude", "copilot"}, ""},
		{"empty", "", nil, "no target specified"},
		{"unknown", "aider", nil, "unknown name"},
		{"mix known and unknown", "claude,aider", nil, "unknown name"},
		{"duplicate dedup", "claude,claude,claude", []string{"claude"}, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.flag, targets)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gotNames := make([]string, len(got))
			for i, x := range got {
				gotNames[i] = x.Name
			}
			if !reflect.DeepEqual(gotNames, tc.wantNames) {
				t.Errorf("got %v, want %v", gotNames, tc.wantNames)
			}
		})
	}
}

func TestDir(t *testing.T) {
	claude := Target{Name: "claude", PersonalDir: "/home/u/.claude/skills", ProjectDir: ".claude/skills"}
	cursor := Target{Name: "cursor", PersonalDir: "", ProjectDir: ".cursor/skills"}

	t.Run("personal happy path", func(t *testing.T) {
		got, err := claude.Dir(ScopePersonal, "")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/home/u/.claude/skills" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("project happy path", func(t *testing.T) {
		got, err := claude.Dir(ScopeProject, "/repo")
		if err != nil {
			t.Fatal(err)
		}
		if got != "/repo/.claude/skills" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("cursor personal is unsupported", func(t *testing.T) {
		_, err := cursor.Dir(ScopePersonal, "")
		if err == nil || !strings.Contains(err.Error(), "personal scope") {
			t.Errorf("err = %v, want 'personal scope' error", err)
		}
	})

	t.Run("project requires root", func(t *testing.T) {
		_, err := claude.Dir(ScopeProject, "")
		if err == nil || !strings.Contains(err.Error(), "project root") {
			t.Errorf("err = %v, want 'project root' error", err)
		}
	})

	t.Run("unknown scope", func(t *testing.T) {
		_, err := claude.Dir(Scope("weird"), "")
		if err == nil || !strings.Contains(err.Error(), "unknown scope") {
			t.Errorf("err = %v, want 'unknown scope' error", err)
		}
	})
}

func TestScopeIsValid(t *testing.T) {
	if !ScopePersonal.IsValid() || !ScopeProject.IsValid() {
		t.Errorf("defined scopes should be valid")
	}
	if Scope("other").IsValid() {
		t.Errorf("undefined scope should be invalid")
	}
}
