package skill

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// fence is the delimiter line that brackets a YAML frontmatter block.
const fence = "---"

// Sentinel errors returned while parsing a SKILL.md file. Callers can test
// for these with errors.Is; Load wraps them with the offending path.
var (
	// ErrNoFrontmatter is returned when the file does not begin with a
	// YAML frontmatter block.
	ErrNoFrontmatter = errors.New("missing YAML frontmatter: file must start with a '---' line")

	// ErrUnterminatedFrontmatter is returned when the opening fence is
	// never followed by a closing fence.
	ErrUnterminatedFrontmatter = errors.New("unterminated YAML frontmatter: missing closing '---'")

	// ErrFrontmatterNotMapping is returned when the frontmatter parses as
	// valid YAML but is not a key/value mapping.
	ErrFrontmatterNotMapping = errors.New("frontmatter must be a YAML mapping")

	// ErrMissingName is returned when the required `name` field is absent
	// or empty.
	ErrMissingName = errors.New("frontmatter is missing required field 'name'")
)

// frontmatterFields are the frontmatter keys the package models explicitly.
type frontmatterFields struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AllowedTools []string `yaml:"allowed-tools"`
}

// splitFrontmatter separates SKILL.md content into the raw frontmatter bytes
// (without the surrounding fences) and the markdown body. A single blank line
// immediately after the closing fence is consumed so the body is not led by a
// stray newline.
func splitFrontmatter(content []byte) (front []byte, body string, err error) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r") != fence {
		return nil, "", ErrNoFrontmatter
	}

	closeIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r") == fence {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return nil, "", ErrUnterminatedFrontmatter
	}

	front = []byte(strings.Join(lines[1:closeIdx], "\n"))

	rest := lines[closeIdx+1:]
	if len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
		rest = rest[1:]
	}
	return front, strings.Join(rest, "\n"), nil
}

// parseFrontmatter parses the raw frontmatter bytes into a mapping node (used
// for lossless round-tripping), the modeled fields, and a decoded map view.
// An empty frontmatter block yields an empty mapping and no error; the caller
// is responsible for enforcing required fields.
func parseFrontmatter(front []byte) (node *yaml.Node, fields frontmatterFields, decoded map[string]any, err error) {
	mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	decoded = map[string]any{}

	if len(bytes.TrimSpace(front)) == 0 {
		return mapping, fields, decoded, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(front, &doc); err != nil {
		// yaml.v3 error messages already carry the offending line number.
		return nil, fields, nil, fmt.Errorf("parsing frontmatter: %w", err)
	}
	if len(doc.Content) == 0 {
		return mapping, fields, decoded, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, fields, nil, ErrFrontmatterNotMapping
	}
	if err := root.Decode(&fields); err != nil {
		return nil, fields, nil, fmt.Errorf("decoding frontmatter fields: %w", err)
	}
	if err := root.Decode(&decoded); err != nil {
		return nil, fields, nil, fmt.Errorf("decoding frontmatter: %w", err)
	}
	return root, fields, decoded, nil
}

// render reconstructs the full SKILL.md content, syncing the modeled fields
// into the preserved frontmatter node before encoding.
func (s *Skill) render() ([]byte, error) {
	node := s.node
	if node == nil {
		node = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}

	setScalar(node, "name", s.Name)
	setScalar(node, "description", s.Description)
	if len(s.AllowedTools) > 0 {
		setStringSeq(node, "allowed-tools", s.AllowedTools)
	} else {
		deleteKey(node, "allowed-tools")
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, fmt.Errorf("skill: encoding frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("skill: encoding frontmatter: %w", err)
	}

	var out bytes.Buffer
	out.WriteString(fence + "\n")
	out.Write(buf.Bytes()) // encoder output is newline-terminated
	out.WriteString(fence + "\n")
	if s.Body != "" {
		out.WriteString("\n")
		out.WriteString(s.Body)
		if !strings.HasSuffix(s.Body, "\n") {
			out.WriteString("\n")
		}
	}
	return out.Bytes(), nil
}

// mappingValue returns the value node for key in a mapping node, or nil.
func mappingValue(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// setScalar sets key to a string scalar, updating an existing entry in place
// (preserving its position) or appending a new one.
func setScalar(m *yaml.Node, key, value string) {
	if v := mappingValue(m, key); v != nil {
		v.Kind = yaml.ScalarNode
		v.Tag = "!!str"
		v.Value = value
		v.Style = 0
		v.Content = nil
		return
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

// setStringSeq sets key to a sequence of string scalars.
func setStringSeq(m *yaml.Node, key string, values []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, v := range values {
		seq.Content = append(seq.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = seq
			return
		}
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		seq,
	)
}

// deleteKey removes key and its value from a mapping node if present.
func deleteKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}
