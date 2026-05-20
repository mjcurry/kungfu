package target

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocations_Personal(t *testing.T) {
	targets := []Target{
		{Name: "claude", PersonalDir: "/u/.claude/skills", ProjectDir: ".claude/skills"},
		{Name: "cursor", PersonalDir: "", ProjectDir: ".cursor/skills"},
	}

	var skips []string
	got, err := Locations(targets, ScopePersonal, "", func(t Target, reason string) {
		skips = append(skips, t.Name+": "+reason)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Target.Name != "claude" {
		t.Errorf("got %+v, want only claude", got)
	}
	if len(skips) != 1 || !strings.Contains(skips[0], "cursor") {
		t.Errorf("expected cursor in skips, got %v", skips)
	}
}

func TestLocations_Project(t *testing.T) {
	targets := []Target{
		{Name: "claude", PersonalDir: "/u/.claude/skills", ProjectDir: ".claude/skills"},
		{Name: "cursor", PersonalDir: "", ProjectDir: ".cursor/skills"},
	}
	got, err := Locations(targets, ScopeProject, "/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	// filepath.Join uses backslashes on Windows; build expectations the
	// same way the implementation does.
	wantClaude := filepath.Join("/repo", ".claude/skills")
	wantCursor := filepath.Join("/repo", ".cursor/skills")
	if got[0].Dir != wantClaude || got[1].Dir != wantCursor {
		t.Errorf("dirs = %q, %q; want %q, %q",
			got[0].Dir, got[1].Dir, wantClaude, wantCursor)
	}
}

func TestLocations_NilOnSkipErrors(t *testing.T) {
	targets := []Target{{Name: "cursor", PersonalDir: "", ProjectDir: ".cursor/skills"}}
	_, err := Locations(targets, ScopePersonal, "", nil)
	if err == nil {
		t.Fatal("expected error when no onSkip callback")
	}
}

func TestLocations_InvalidScope(t *testing.T) {
	_, err := Locations(nil, Scope("weird"), "", nil)
	if err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestErrNoSupportedLocations(t *testing.T) {
	if !errors.Is(ErrNoSupportedLocations, ErrNoSupportedLocations) {
		t.Fatal("sanity")
	}
}
