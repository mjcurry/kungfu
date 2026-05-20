package rules

import (
	"strings"

	"github.com/mjcurry/kungfu/internal/skill"
)

// BodyEmpty warns when the markdown body after the frontmatter is empty or
// whitespace-only. A skill with no body teaches the agent nothing once it
// is invoked.
type BodyEmpty struct{}

func (BodyEmpty) ID() string { return "body/empty" }

func (BodyEmpty) Check(s *skill.Skill) []Diagnostic {
	if strings.TrimSpace(s.Body) != "" {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityWarning,
		Rule:     "body/empty",
		Message:  "skill body is empty; add instructions an agent can read once the skill is invoked",
	}}
}
