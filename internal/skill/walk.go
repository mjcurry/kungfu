package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Discover finds every valid skill directory one level deep under dir. A
// subdirectory is treated as a skill when it contains a SKILL.md file.
// Hidden directories (those whose name begins with ".") are skipped.
//
// The returned skills are sorted by name. If a candidate directory contains a
// SKILL.md that fails to parse, Discover returns an error identifying it
// rather than silently skipping it.
func Discover(dir string) ([]*Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("skill: reading skills directory %s: %w", dir, err)
	}

	var skills []*Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(sub, FileName)); err != nil {
			continue // not a skill directory
		}
		s, err := Load(sub)
		if err != nil {
			return nil, fmt.Errorf("skill: discovering skills in %s: %w", dir, err)
		}
		skills = append(skills, s)
	}

	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}
