package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mjcurry/kungfu/internal/target"
)

// Match is a found skill with its (target, scope, directory) provenance.
// Target and Scope are strings so callers can put a Match straight into
// JSON output without further conversion.
type Match struct {
	// Skill is the loaded skill.
	Skill *Skill
	// Target is the canonical target name (claude, codex, …).
	Target string
	// Scope is "personal" or "project".
	Scope string
	// Location is the absolute directory containing this skill.
	Location string
}

// FindByName searches the given locations for skills whose directory name
// is name. Locations are searched in slice order, so callers can control
// precedence (typically primary-scope first, then secondary).
//
// FindByName returns every match it finds — a skill of the same name
// installed under multiple targets shows up multiple times, which is the
// behaviour callers like `kungfu show` need to disambiguate. An empty
// slice is not an error; check len(matches).
func FindByName(name string, locations []target.Location) ([]Match, error) {
	if name == "" {
		return nil, nil
	}
	var matches []Match
	for _, loc := range locations {
		candidate := filepath.Join(loc.Dir, name)
		if _, err := os.Stat(filepath.Join(candidate, FileName)); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("skill: looking up %q in %s: %w", name, loc.Dir, err)
		}
		s, err := Load(candidate)
		if err != nil {
			return nil, err
		}
		matches = append(matches, Match{
			Skill:    s,
			Target:   loc.Target.Name,
			Scope:    string(loc.Scope),
			Location: candidate,
		})
	}
	return matches, nil
}
