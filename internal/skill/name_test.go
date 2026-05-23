package skill

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	cases := []struct {
		name  string
		input string
		ok    bool
		want  string // substring expected in error message when !ok
	}{
		{"happy lowercase kebab", "my-skill", true, ""},
		{"happy single segment", "hello", true, ""},
		{"empty", "", false, "empty"},
		{"underscore", "my_skill", false, "kebab-case"},
		{"upper", "My-Skill", false, "kebab-case"},
		{"leading hyphen", "-skill", false, "kebab-case"},
		{"trailing hyphen", "skill-", false, "kebab-case"},
		{"double hyphen", "my--skill", false, "kebab-case"},
		{"too long", strings.Repeat("a", MaxNameLen+1), false, "characters"},
		{"at the limit", strings.Repeat("a", MaxNameLen), true, ""},
		// Namespace-prefix form used by Claude slash commands.
		{"namespaced", "ckm:banner-design", true, ""},
		{"namespaced short", "a:b", true, ""},
		{"empty namespace", ":banner", false, "kebab-case"},
		{"empty name part", "ckm:", false, "kebab-case"},
		{"double colon", "ckm::banner", false, "kebab-case"},
		{"upper after colon", "ckm:Banner", false, "kebab-case"},
		{"upper before colon", "CKM:banner", false, "kebab-case"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateName(tc.input)
			switch {
			case tc.ok && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case !tc.ok && err == nil:
				t.Fatal("expected error, got nil")
			case !tc.ok && !strings.Contains(err.Error(), tc.want):
				t.Errorf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestValidateNameSentinels(t *testing.T) {
	if !errors.Is(ValidateName(""), ErrEmptyName) {
		t.Errorf("empty name should match ErrEmptyName via errors.Is")
	}
}
