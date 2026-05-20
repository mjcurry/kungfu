package template

import (
	"errors"
	"fmt"
	"path/filepath"
)

// Template describes one built-in scaffold.
type Template struct {
	// Name is the kebab-case identifier passed to `kungfu new --template`.
	Name string
	// Description is a one-line summary shown in interactive selection.
	Description string
}

// Vars is the data passed into each rendered file. Templates use Go's
// text/template syntax against this struct, so {{.Name}} substitutes the
// skill name, {{.Description}} the trigger phrase, and so on.
type Vars struct {
	// Name is the new skill's name (lowercase-kebab-case).
	Name string
	// Description is the trigger condition. The basic / document /
	// data / api-wrapper SKILL.md templates prepend "Use this skill when "
	// to satisfy the description/no-trigger-phrase lint rule, so the value
	// here should read naturally as a clause completing that sentence.
	Description string
	// Year is the current year, useful for license stubs or footers.
	Year int
	// CreatedAt is the current date in RFC3339 form, available for
	// templates that want to embed a created-at field.
	CreatedAt string
}

// ErrTemplateNotFound is returned by ByName when name does not match a
// built-in template.
var ErrTemplateNotFound = errors.New("template: not found")

// builtins is the ordered list of templates surfaced to the CLI. The order
// determines how options appear in interactive selection.
var builtins = []Template{
	{Name: "basic", Description: "Minimal SKILL.md scaffold with placeholders to fill in."},
	{Name: "document", Description: "Producing structured prose documents (reports, memos, summaries)."},
	{Name: "data", Description: "Inspecting and summarising tabular data with a stdlib Python helper."},
	{Name: "api-wrapper", Description: "Calling an HTTP API behind an env-driven bash wrapper."},
}

// Builtins returns the built-in scaffolds in their canonical order.
func Builtins() []Template {
	out := make([]Template, len(builtins))
	copy(out, builtins)
	return out
}

// ByName returns the named built-in template, or ErrTemplateNotFound when
// name is not one of the known scaffolds.
func ByName(name string) (Template, error) {
	for _, t := range builtins {
		if t.Name == name {
			return t, nil
		}
	}
	return Template{}, fmt.Errorf("%w: %q", ErrTemplateNotFound, name)
}

// Apply renders t into destDir, which must not already exist. On success
// the returned absolute path is the rendered scaffold root. On any error
// mid-walk destDir is removed so callers do not have to deal with a
// half-rendered skill.
func (t Template) Apply(destDir string, vars Vars) (string, error) {
	abs, err := filepath.Abs(destDir)
	if err != nil {
		return "", fmt.Errorf("template: resolving destination: %w", err)
	}
	if _, err := stat(abs); err == nil {
		return "", fmt.Errorf("template: destination %s already exists", abs)
	}
	if err := renderTemplate(t.Name, abs, vars); err != nil {
		// Best-effort cleanup: leave nothing partial on disk.
		_ = removeAll(abs)
		return "", err
	}
	return abs, nil
}
