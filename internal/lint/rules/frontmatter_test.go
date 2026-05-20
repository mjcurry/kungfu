package rules

import (
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

func TestFrontmatterNameMissing(t *testing.T) {
	if got := (FrontmatterNameMissing{}).Check(&skill.Skill{Dir: "/x"}); len(got) != 1 {
		t.Fatalf("missing name should be flagged: %v", got)
	}
	if got := (FrontmatterNameMissing{}).Check(&skill.Skill{Dir: "/x", Name: "x"}); len(got) != 0 {
		t.Fatalf("present name should pass: %v", got)
	}
}

func TestFrontmatterNameMismatch(t *testing.T) {
	cases := []struct {
		dir, name string
		want      bool
	}{
		{"/skills/alpha", "alpha", false},
		{"/skills/alpha", "beta", true},
		{"/skills/alpha", "", false}, // covered by name-missing
	}
	for _, tc := range cases {
		got := FrontmatterNameMismatch{}.Check(&skill.Skill{Dir: tc.dir, Name: tc.name})
		if tc.want && len(got) != 1 {
			t.Errorf("dir=%s name=%s expected 1 diag, got %v", tc.dir, tc.name, got)
		}
		if !tc.want && len(got) != 0 {
			t.Errorf("dir=%s name=%s expected 0 diags, got %v", tc.dir, tc.name, got)
		}
	}
}

func TestFrontmatterNameFormat(t *testing.T) {
	cases := []struct {
		name string
		bad  bool
	}{
		{"hello-world", false},
		{"hello", false},
		{"hello_world", true},
		{"helloWorld", true},
		{"-hello", true},
		{"hello-", true},
		{"hello--world", true},
		{strings.Repeat("a", 65), true},
		{strings.Repeat("a", 64), false},
		{"", false}, // covered by name-missing
	}
	for _, tc := range cases {
		got := FrontmatterNameFormat{}.Check(&skill.Skill{Dir: "/x", Name: tc.name})
		if tc.bad && len(got) == 0 {
			t.Errorf("expected diagnostic for %q", tc.name)
		}
		if !tc.bad && len(got) != 0 {
			t.Errorf("unexpected diagnostic for %q: %v", tc.name, got)
		}
	}
}

func TestFrontmatterDescriptionMissing(t *testing.T) {
	if got := (FrontmatterDescriptionMissing{}).Check(&skill.Skill{Dir: "/x"}); len(got) != 1 {
		t.Errorf("missing description not flagged: %v", got)
	}
	if got := (FrontmatterDescriptionMissing{}).Check(&skill.Skill{Dir: "/x", Description: "hi"}); len(got) != 0 {
		t.Errorf("present description should pass: %v", got)
	}
}

func TestFrontmatterDescriptionTooLong(t *testing.T) {
	limit := strings.Repeat("a", maxDescriptionLen)
	over := strings.Repeat("a", maxDescriptionLen+1)
	if got := (FrontmatterDescriptionTooLong{}).Check(&skill.Skill{Dir: "/x", Description: limit}); len(got) != 0 {
		t.Errorf("at limit should pass: %v", got)
	}
	if got := (FrontmatterDescriptionTooLong{}).Check(&skill.Skill{Dir: "/x", Description: over}); len(got) != 1 {
		t.Errorf("over limit should be flagged: %v", got)
	}
}

func TestFrontmatterAllowedToolsType(t *testing.T) {
	cases := []struct {
		name  string
		value any
		bad   bool
	}{
		{"absent", nil, false},
		{"list of strings", []any{"Read", "Bash"}, false},
		{"typed list", []string{"Read"}, false},
		{"single string", "Read", true},
		{"list with int", []any{"Read", 42}, true},
		{"map", map[string]any{"a": 1}, true},
		{"nil entry", []any{"Read", nil}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fm := map[string]any{}
			if tc.value != nil {
				fm["allowed-tools"] = tc.value
			}
			got := FrontmatterAllowedToolsType{}.Check(&skill.Skill{Dir: "/x", Frontmatter: fm})
			if tc.bad && len(got) != 1 {
				t.Errorf("expected diag, got %v", got)
			}
			if !tc.bad && len(got) != 0 {
				t.Errorf("unexpected diag: %v", got)
			}
		})
	}
}
