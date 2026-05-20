// Package skill models AI agent skills. A skill is a directory containing a
// SKILL.md file with YAML frontmatter and a markdown body, optionally
// accompanied by scripts/, references/, and assets/ subdirectories.
//
// The package can Load a skill from disk, Save it back while preserving any
// frontmatter fields it does not explicitly model, and Discover all skills in
// a directory tree.
package skill

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the canonical name of the file that defines a skill.
const FileName = "SKILL.md"

// Skill is a single skill loaded from a SKILL.md file.
//
// The explicitly modeled fields (Name, Description, AllowedTools) are kept in
// sync with the underlying frontmatter on Save. Any other frontmatter fields
// are retained verbatim so a Load followed by a Save does not lose data.
type Skill struct {
	// Dir is the directory that contains the SKILL.md file.
	Dir string

	// Name is the skill's unique identifier, from the frontmatter `name`.
	Name string

	// Description tells an agent when the skill should be used, from the
	// frontmatter `description`.
	Description string

	// AllowedTools optionally restricts the tools the skill may invoke,
	// from the frontmatter `allowed-tools`. It is nil when unset.
	AllowedTools []string

	// Body is the markdown content that follows the frontmatter block.
	Body string

	// Frontmatter is a decoded view of every frontmatter field, including
	// those not modeled above. It is provided for inspection; mutating it
	// has no effect on Save. Use the typed fields to change modeled values.
	Frontmatter map[string]any

	// node is the parsed frontmatter mapping. It preserves key order and
	// unmodeled fields across a Load/Save round trip.
	node *yaml.Node

	// raw is the original, unmodified file content as read from disk.
	raw []byte
}

// Load reads and parses the SKILL.md file in the given directory.
func Load(dir string) (*Skill, error) {
	return LoadFile(filepath.Join(dir, FileName))
}

// LoadFile reads and parses a SKILL.md file given its path directly.
func LoadFile(path string) (*Skill, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("skill: reading %s: %w", path, err)
	}

	front, body, _, err := SplitFrontmatter(content)
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", path, err)
	}

	node, fields, decoded, err := parseFrontmatter(front)
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", path, err)
	}
	if fields.Name == "" {
		return nil, fmt.Errorf("skill: %s: %w", path, ErrMissingName)
	}

	return &Skill{
		Dir:          filepath.Dir(path),
		Name:         fields.Name,
		Description:  fields.Description,
		AllowedTools: fields.AllowedTools,
		Body:         body,
		Frontmatter:  decoded,
		node:         node,
		raw:          content,
	}, nil
}

// Save writes the skill back to its SKILL.md file. The modeled fields are
// synced into the frontmatter while every other frontmatter field is
// preserved. The directory must already exist.
func (s *Skill) Save() error {
	out, err := s.render()
	if err != nil {
		return err
	}
	path := filepath.Join(s.Dir, FileName)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("skill: writing %s: %w", path, err)
	}
	s.raw = out
	return nil
}

// Raw returns the file content as it was last read from or written to disk.
// The returned slice must not be modified.
func (s *Skill) Raw() []byte {
	return s.raw
}
