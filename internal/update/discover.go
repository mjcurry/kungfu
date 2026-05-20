// Package update is the orchestration layer for `kungfu update`. It walks
// configured target locations, parses the provenance frontmatter PR 4
// stamps onto installed skills, and exposes the Updatable type the CLI
// uses to compute the fetch + reinstall plan.
//
// The package lives outside internal/skill because Updatable holds a
// *fetch.Source, and fetch already depends on skill (for FileMode and
// SplitFrontmatter). Putting the helper here breaks the cycle without
// muddying either package.
package update

import (
	"fmt"
	"os"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/skill"
	"github.com/mjcurry/kungfu/internal/target"
)

// Updatable is a single installed skill we know how to re-fetch: it has
// kungfu_source provenance (and that source parses cleanly).
type Updatable struct {
	// Skill is the loaded skill, with provenance fields populated.
	Skill *skill.Skill
	// Location is where this copy of the skill lives (target + scope +
	// directory). One logical skill may have multiple Updatables when the
	// same install spans multiple targets.
	Location target.Location
	// Source is the parsed remote source the skill was fetched from.
	// Source.Ref is populated from kungfu_ref so callers can resolve it
	// directly via fetch.Client.ResolveRef.
	Source *fetch.Source
	// StoredSHA is the kungfu_sha frontmatter value — the commit the
	// installed copy was fetched from.
	StoredSHA string
	// StoredRef is the kungfu_ref frontmatter value (the user-supplied
	// ref, e.g. "v1.0.0" or "main"; empty means "default branch").
	StoredRef string
}

// DiscoverUpdatable walks each location and returns every installed skill
// that has parseable kungfu_source provenance. Skills with no provenance
// (locally-installed, scaffolded, or installed by other tools) are silently
// skipped — they are not updatable through kungfu. Skills with malformed
// provenance are reported via onWarn (if non-nil) and excluded from the
// result so a single broken skill cannot poison an `update --all`.
//
// onWarn may be nil; in that case malformed-provenance skills are skipped
// silently. The error return is reserved for I/O problems against the
// locations themselves (e.g. a Discover failure on a directory that exists
// but cannot be read).
func DiscoverUpdatable(locations []target.Location, onWarn func(skillDir string, err error)) ([]Updatable, error) {
	var out []Updatable
	for _, loc := range locations {
		if _, err := os.Stat(loc.Dir); err != nil {
			continue
		}
		skills, err := skill.Discover(loc.Dir)
		if err != nil {
			return nil, fmt.Errorf("update: discovering skills in %s: %w", loc.Dir, err)
		}
		for _, s := range skills {
			if s.Source == "" {
				continue
			}
			src, err := fetch.Parse(s.Source)
			if err != nil {
				if onWarn != nil {
					onWarn(s.Dir, fmt.Errorf("unparseable provenance source %q: %w", s.Source, err))
				}
				continue
			}
			// Use the stored ref so ResolveRef behaves consistently with
			// the original install (e.g. "main" re-resolves to whatever
			// main points at now, "v1.0.0" stays put).
			src.Ref = s.Ref
			out = append(out, Updatable{
				Skill:     s,
				Location:  loc,
				Source:    src,
				StoredSHA: s.SHA,
				StoredRef: s.Ref,
			})
		}
	}
	return out, nil
}
