// Package rules defines the lint Rule interface, the Diagnostic value type,
// and the standard rule set shipped with kungfu.
//
// Diagnostic and Severity live here (rather than in the parent lint package)
// so rule implementations do not need to import lint — which lets the lint
// package depend on rules without creating an import cycle. The lint package
// re-exports these types as aliases for ergonomic access.
package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mjcurry/kungfu/internal/skill"
)

// Severity describes how serious a diagnostic is. Errors block install and
// cause `kungfu lint` to exit non-zero; warnings are advisory unless the
// caller opts into --strict.
type Severity int

const (
	// SeverityWarning is advisory and does not, by default, cause a
	// non-zero exit code from `kungfu lint`.
	SeverityWarning Severity = iota
	// SeverityError indicates a problem that blocks install and fails
	// `kungfu lint` regardless of flags.
	SeverityError
)

// String returns the lowercase name of the severity ("warning", "error").
func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	default:
		return fmt.Sprintf("severity(%d)", int(s))
	}
}

// Label returns a short, fixed-width label for human output.
func (s Severity) Label() string {
	switch s {
	case SeverityError:
		return "ERROR"
	case SeverityWarning:
		return "WARN "
	default:
		return strings.ToUpper(s.String())
	}
}

// MarshalJSON renders Severity as a lowercase string so the JSON contract is
// readable and not tied to the underlying integer.
func (s Severity) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}

// Diagnostic is a single finding emitted by a rule.
type Diagnostic struct {
	// Path is the file path the diagnostic refers to.
	Path string `json:"path"`
	// Line is the 1-indexed line within Path. 0 when not tied to a line.
	Line int `json:"line,omitempty"`
	// Severity is the diagnostic's seriousness.
	Severity Severity `json:"severity"`
	// Rule is the stable identifier (e.g. "frontmatter/name-mismatch").
	Rule string `json:"rule"`
	// Message is a human-readable, actionable description.
	Message string `json:"message"`
}

// Rule inspects a skill and returns any problems it finds. Rules are pure
// functions of their input: they must not mutate the skill, contact the
// network, or maintain hidden state across calls. Filesystem access is
// permitted when a rule needs to follow paths that originate from the skill,
// such as reference resolution.
type Rule interface {
	// ID returns the stable identifier of the rule. Part of the public
	// contract; users may grep for IDs in scripts.
	ID() string
	// Check returns the diagnostics this rule wants to emit for s. Empty
	// (or nil) means the rule found no problems.
	Check(s *skill.Skill) []Diagnostic
}

// DefaultRules returns the standard rule set, in the order rules should be
// run. The same Rule value is safe to share across linters.
func DefaultRules() []Rule {
	return []Rule{
		FrontmatterNameMissing{},
		FrontmatterNameMismatch{},
		FrontmatterNameFormat{},
		FrontmatterDescriptionMissing{},
		FrontmatterDescriptionTooLong{},
		FrontmatterAllowedToolsType{},
		DescriptionNoTriggerPhrase{},
		DescriptionVague{},
		BodyEmpty{},
		ReferencesBroken{},
		FilenamesNonASCII{},
	}
}

// skillFile is the canonical SKILL.md path used by every rule that needs to
// attribute a diagnostic to the source file.
func skillFile(dir string) string {
	return filepath.Join(dir, skill.FileName)
}
