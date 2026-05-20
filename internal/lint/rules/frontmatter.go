package rules

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/mjcurry/kungfu/internal/skill"
)

// maxDescriptionLen is the upper bound on the frontmatter description field.
// Beyond this the description tends to read as a tutorial rather than a
// trigger sentence an agent can match on.
const maxDescriptionLen = 1024

// maxNameLen is the upper bound on the frontmatter name field. Wide enough
// for realistic kebab-case identifiers, small enough for every filesystem.
const maxNameLen = 64

// kebabCaseRE matches lowercase kebab-case names with an optional namespace
// prefix separated by a colon (e.g. `ckm:banner-design`). Each segment is
// ASCII letters and digits separated by single hyphens, no leading/trailing
// or repeated hyphens. The namespace form is widely used by Claude slash
// commands.
var kebabCaseRE = regexp.MustCompile(`^([a-z0-9]+(-[a-z0-9]+)*:)?[a-z0-9]+(-[a-z0-9]+)*$`)

// FrontmatterNameMissing flags a skill whose `name` field is absent or empty.
type FrontmatterNameMissing struct{}

func (FrontmatterNameMissing) ID() string { return "frontmatter/name-missing" }

func (FrontmatterNameMissing) Check(s *skill.Skill) []Diagnostic {
	if s.Name != "" {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityError,
		Rule:     "frontmatter/name-missing",
		Message:  "frontmatter is missing the required 'name' field",
	}}
}

// FrontmatterNameMismatch flags a skill whose `name` does not match its
// directory.
type FrontmatterNameMismatch struct{}

func (FrontmatterNameMismatch) ID() string { return "frontmatter/name-mismatch" }

func (FrontmatterNameMismatch) Check(s *skill.Skill) []Diagnostic {
	if s.Name == "" {
		return nil // covered by FrontmatterNameMissing
	}
	dir := filepath.Base(s.Dir)
	if s.Name == dir {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityError,
		Rule:     "frontmatter/name-mismatch",
		Message:  fmt.Sprintf("name %q does not match directory %q", s.Name, dir),
	}}
}

// FrontmatterNameFormat flags names that are not lowercase-kebab-case or
// exceed maxNameLen.
type FrontmatterNameFormat struct{}

func (FrontmatterNameFormat) ID() string { return "frontmatter/name-format" }

func (FrontmatterNameFormat) Check(s *skill.Skill) []Diagnostic {
	if s.Name == "" {
		return nil
	}
	if len(s.Name) > maxNameLen {
		return []Diagnostic{{
			Path:     skillFile(s.Dir),
			Severity: SeverityError,
			Rule:     "frontmatter/name-format",
			Message: fmt.Sprintf("name is %d characters; must be at most %d",
				len(s.Name), maxNameLen),
		}}
	}
	if !kebabCaseRE.MatchString(s.Name) {
		return []Diagnostic{{
			Path:     skillFile(s.Dir),
			Severity: SeverityError,
			Rule:     "frontmatter/name-format",
			Message:  fmt.Sprintf("name %q must be lowercase-kebab-case (e.g. my-skill)", s.Name),
		}}
	}
	return nil
}

// FrontmatterDescriptionMissing flags a skill whose `description` is absent
// or empty.
type FrontmatterDescriptionMissing struct{}

func (FrontmatterDescriptionMissing) ID() string { return "frontmatter/description-missing" }

func (FrontmatterDescriptionMissing) Check(s *skill.Skill) []Diagnostic {
	if s.Description != "" {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityError,
		Rule:     "frontmatter/description-missing",
		Message:  "frontmatter is missing the required 'description' field",
	}}
}

// FrontmatterDescriptionTooLong flags descriptions over maxDescriptionLen.
type FrontmatterDescriptionTooLong struct{}

func (FrontmatterDescriptionTooLong) ID() string { return "frontmatter/description-too-long" }

func (FrontmatterDescriptionTooLong) Check(s *skill.Skill) []Diagnostic {
	if len(s.Description) <= maxDescriptionLen {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityError,
		Rule:     "frontmatter/description-too-long",
		Message: fmt.Sprintf("description is %d characters; must be at most %d",
			len(s.Description), maxDescriptionLen),
	}}
}

// FrontmatterAllowedToolsType flags an `allowed-tools` field that exists but
// is not a YAML list of strings.
type FrontmatterAllowedToolsType struct{}

func (FrontmatterAllowedToolsType) ID() string { return "frontmatter/allowed-tools-type" }

func (FrontmatterAllowedToolsType) Check(s *skill.Skill) []Diagnostic {
	raw, present := s.Frontmatter["allowed-tools"]
	if !present || raw == nil {
		return nil
	}
	if isStringList(raw) {
		return nil
	}
	return []Diagnostic{{
		Path:     skillFile(s.Dir),
		Severity: SeverityError,
		Rule:     "frontmatter/allowed-tools-type",
		Message:  "allowed-tools must be a YAML list of strings",
	}}
}

func isStringList(v any) bool {
	switch list := v.(type) {
	case []string:
		return true
	case []any:
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return false
			}
		}
		return true
	default:
		return false
	}
}
