package rules

import (
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
)

func TestDescriptionNoTriggerPhrase(t *testing.T) {
	cases := []struct {
		name string
		desc string
		bad  bool
	}{
		{"use this skill when", "Use this skill when extracting PDFs.", false},
		{"lowercase variant", "use this skill when the user asks", false},
		{"use when", "Use when the user mentions JSON.", false},
		{"apply when", "Apply when extracting tables.", false},
		{"invoke when", "Invoke when the user wants charts.", false},
		{"use this to", "Use this to scaffold a project.", false},
		{"missing phrase", "Handles PDF stuff.", true},
		{"empty description skipped", "", false},
		{"phrase beyond window ignored",
			"This skill, painstakingly crafted with great care for many varied user scenarios, is designed for use this skill when needed",
			true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DescriptionNoTriggerPhrase{}.Check(&skill.Skill{Dir: "/x", Description: tc.desc})
			if tc.bad && len(got) != 1 {
				t.Errorf("expected diag, got %v", got)
			}
			if !tc.bad && len(got) != 0 {
				t.Errorf("unexpected diag: %v", got)
			}
		})
	}
}

func TestDescriptionVague(t *testing.T) {
	cases := []struct {
		name string
		desc string
		bad  bool
	}{
		{"vague no domain terms", "Use this skill for various things.", true},
		{"vague with domain terms",
			"Use this skill for various PDF, JSON, and Markdown formatting tasks involving the Python toolchain.", false},
		{"no vague tokens", "Use this skill when extracting tables from PDFs.", false},
		{"multi-word vague", "Apply when the user needs many things done quickly.", true},
		{"substring is not whole word", "Use this skill to regenerate the index file.", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DescriptionVague{}.Check(&skill.Skill{Dir: "/x", Description: tc.desc})
			if tc.bad && len(got) != 1 {
				t.Errorf("expected diag, got %v", got)
			}
			if !tc.bad && len(got) != 0 {
				t.Errorf("unexpected diag: %v", got)
			}
		})
	}
}
