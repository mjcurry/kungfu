// Package target describes the conventions each supported AI agent follows
// for storing skills on disk: personal-scope directories (under the user's
// home) and project-scope directories (under a project root).
//
// The package is intentionally pure — it does no I/O and does not expand
// "~" in paths. Callers that need expansion (typically the config layer)
// do it themselves before populating Target values.
package target

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Scope is where a skill lives within a target: personal (under the user's
// home directory) or project (under a project root).
type Scope string

// Defined scopes. Any other string is invalid.
const (
	ScopePersonal Scope = "personal"
	ScopeProject  Scope = "project"
)

// IsValid reports whether s is one of the defined scopes.
func (s Scope) IsValid() bool {
	return s == ScopePersonal || s == ScopeProject
}

// Target describes one agent's skill conventions: its canonical name, the
// personal-scope directory (may be empty when the agent has no personal
// concept, e.g. cursor), and the project-scope directory relative to a
// project root.
type Target struct {
	// Name is the canonical identifier: claude, codex, cursor, copilot, …
	Name string
	// PersonalDir is the expanded absolute path for personal scope. Empty
	// means the target does not support personal scope.
	PersonalDir string
	// ProjectDir is the path relative to a project root for project scope.
	ProjectDir string
}

// Dir returns the resolved directory for the given scope. For project scope
// the directory is computed by joining projectRoot with t.ProjectDir.
//
// Dir returns an error when the target does not support the requested scope
// (e.g. cursor + personal) or when project scope is requested without a
// project root.
func (t Target) Dir(scope Scope, projectRoot string) (string, error) {
	switch scope {
	case ScopePersonal:
		if t.PersonalDir == "" {
			return "", fmt.Errorf("target %q does not support personal scope", t.Name)
		}
		return t.PersonalDir, nil
	case ScopeProject:
		if t.ProjectDir == "" {
			return "", fmt.Errorf("target %q does not support project scope", t.Name)
		}
		if projectRoot == "" {
			return "", fmt.Errorf("target %q project scope requires a project root", t.Name)
		}
		return filepath.Join(projectRoot, t.ProjectDir), nil
	default:
		return "", fmt.Errorf("target: unknown scope %q", scope)
	}
}

// Builtins returns the built-in target definitions, with PersonalDir values
// left tilde-unexpanded. The config layer is responsible for expanding "~"
// before exposing targets to commands.
func Builtins() []Target {
	return []Target{
		{Name: "claude", PersonalDir: "~/.claude/skills", ProjectDir: ".claude/skills"},
		{Name: "codex", PersonalDir: "~/.codex/skills", ProjectDir: ".codex/skills"},
		{Name: "cursor", PersonalDir: "", ProjectDir: ".cursor/skills"},
		{Name: "copilot", PersonalDir: "~/.copilot/skills", ProjectDir: ".github/skills"},
	}
}

// ByName looks up a target by canonical name within the given set. The set
// is searched in slice order so callers can control precedence.
func ByName(name string, targets []Target) (Target, error) {
	for _, t := range targets {
		if t.Name == name {
			return t, nil
		}
	}
	return Target{}, fmt.Errorf("target %q not found", name)
}

// Resolve parses a --target flag value against the given set. Accepted forms:
//
//   - "all" — returns every configured target, in declaration order.
//   - "name" — a single target.
//   - "a,b,c" — a comma-separated list. Whitespace around names is ignored.
//
// The returned slice is in declaration order (i.e. the order targets appears
// in), not in the order the user specified. An empty flag is an error;
// callers that want "empty means everything" should detect that explicitly
// before calling.
func Resolve(flag string, targets []Target) ([]Target, error) {
	flag = strings.TrimSpace(flag)
	if flag == "" {
		return nil, fmt.Errorf("target: no target specified")
	}
	if flag == "all" {
		out := make([]Target, len(targets))
		copy(out, targets)
		return out, nil
	}

	requested := map[string]bool{}
	for _, r := range strings.Split(flag, ",") {
		r = strings.TrimSpace(r)
		if r != "" {
			requested[r] = true
		}
	}
	if len(requested) == 0 {
		return nil, fmt.Errorf("target: no target specified")
	}

	out := make([]Target, 0, len(requested))
	for _, t := range targets {
		if requested[t.Name] {
			out = append(out, t)
			delete(requested, t.Name)
		}
	}
	if len(requested) > 0 {
		unknown := make([]string, 0, len(requested))
		for name := range requested {
			unknown = append(unknown, name)
		}
		sort.Strings(unknown)
		return nil, fmt.Errorf("target: unknown name(s): %s", strings.Join(unknown, ", "))
	}
	return out, nil
}
