package skill

import (
	"errors"
	"fmt"
	"regexp"
)

// MaxNameLen is the upper bound on a skill name: enough for realistic
// kebab-case identifiers, small enough for every filesystem.
const MaxNameLen = 64

// NameRE matches lowercase kebab-case names with an optional namespace
// prefix separated by a colon (e.g. `ckm:banner-design`). Each segment is
// ASCII letters and digits separated by single hyphens, no leading,
// trailing, or doubled hyphens. The namespace form is the standard Claude
// slash-command naming convention.
//
// Exported so the lint rule and the `new` command share the same source of
// truth — keeping this in sync with `internal/lint/rules` is mandatory.
var NameRE = regexp.MustCompile(`^([a-z0-9]+(-[a-z0-9]+)*:)?[a-z0-9]+(-[a-z0-9]+)*$`)

// ErrEmptyName is returned by ValidateName when name is "".
var ErrEmptyName = errors.New("skill: name is empty")

// ValidateName checks that name is non-empty, ≤MaxNameLen characters, and
// matches NameRE. It returns nil on success and a human-readable error
// describing the first problem otherwise.
func ValidateName(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	if len(name) > MaxNameLen {
		return fmt.Errorf("skill: name is %d characters; must be at most %d", len(name), MaxNameLen)
	}
	if !NameRE.MatchString(name) {
		return fmt.Errorf("skill: name %q must be lowercase-kebab-case (e.g. my-skill or ns:my-skill)", name)
	}
	return nil
}
