package rules

import (
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

func TestBodyEmpty(t *testing.T) {
	cases := []struct {
		name string
		body string
		bad  bool
	}{
		{"empty", "", true},
		{"whitespace only", "\n  \n\t\n", true},
		{"content", "# Hello\n\nDo stuff.\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := BodyEmpty{}.Check(&skill.Skill{Dir: "/x", Body: tc.body})
			if tc.bad && len(got) != 1 {
				t.Errorf("expected diag, got %v", got)
			}
			if !tc.bad && len(got) != 0 {
				t.Errorf("unexpected diag: %v", got)
			}
		})
	}
}
