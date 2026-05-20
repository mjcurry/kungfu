// Package config loads kungfu's TOML configuration and resolves effective
// settings from the layered sources kungfu supports: command-line flags,
// environment variables, the config file, and built-in defaults.
//
// As of PR 2, the config models a set of targets (agents) and a default
// scope. The legacy `skills_dir` / `extra_skills_dirs` fields remain for
// back-compat — they are still loaded and exposed on Config, but the new
// target-aware commands do not consult them.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/mjcurry/kungfu/internal/target"
)

const (
	// DefaultSkillsDir is used by the legacy ResolveSkillsDir flow when no
	// source specifies a skills directory.
	DefaultSkillsDir = "~/.claude/skills"

	// EnvSkillsDir overrides the legacy skills directory.
	EnvSkillsDir = "KUNGFU_SKILLS_DIR"

	appName  = "kungfu"
	fileName = "config.toml"
)

// DefaultTarget is the target name applied when default_targets is absent.
var DefaultTarget = "claude"

// DefaultScope is the scope applied when default_scope is absent.
var DefaultScope = target.ScopePersonal

// Config is the on-disk kungfu configuration after merging defaults.
type Config struct {
	// SkillsDir is the legacy primary directory kungfu manages skills in.
	// Retained for back-compat; new target-aware commands ignore it.
	SkillsDir string

	// ExtraSkillsDirs are legacy additional, read-also skill directories.
	ExtraSkillsDirs []string

	// Targets is the merged target set: built-in entries with any
	// per-target overrides applied, plus any custom targets defined by
	// the user. Personal-scope directories are tilde-expanded.
	Targets []target.Target

	// DefaultTargets is the list of target names install/remove default to
	// when --target is not given. List does not consult this field.
	DefaultTargets []string

	// DefaultScope is the scope install/remove/show default to when
	// --scope is not given.
	DefaultScope target.Scope
}

// targetSection mirrors a [targets.<name>] block. The pointer fields let us
// distinguish "key absent (inherit from builtin)" from "key explicitly set
// to empty string (cursor-style: unsupported)".
type targetSection struct {
	PersonalDir *string `toml:"personal_dir"`
	ProjectDir  *string `toml:"project_dir"`
}

type fileConfig struct {
	SkillsDir       string                   `toml:"skills_dir"`
	ExtraSkillsDirs []string                 `toml:"extra_skills_dirs"`
	DefaultTargets  []string                 `toml:"default_targets"`
	DefaultScope    string                   `toml:"default_scope"`
	Targets         map[string]targetSection `toml:"targets"`
}

// Path returns the config file path kungfu would use:
// $XDG_CONFIG_HOME/kungfu/config.toml, falling back to
// ~/.config/kungfu/config.toml when XDG_CONFIG_HOME is unset.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil {
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, appName, fileName)
}

// Load reads the config file at Path. A missing file is not an error: the
// returned Config holds defaults. A present but malformed file is an error.
func Load() (*Config, error) {
	return LoadFrom(Path())
}

// LoadFrom reads the config file at path. Like Load, a missing file yields
// defaults rather than an error.
func LoadFrom(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	var fc fileConfig
	if err := toml.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}

	if strings.TrimSpace(fc.SkillsDir) != "" {
		cfg.SkillsDir = fc.SkillsDir
	}
	if len(fc.ExtraSkillsDirs) > 0 {
		cfg.ExtraSkillsDirs = fc.ExtraSkillsDirs
	}
	if len(fc.DefaultTargets) > 0 {
		cfg.DefaultTargets = fc.DefaultTargets
	}
	if s := strings.TrimSpace(fc.DefaultScope); s != "" {
		scope := target.Scope(s)
		if !scope.IsValid() {
			return nil, fmt.Errorf("config: %s: invalid default_scope %q (want \"personal\" or \"project\")", path, s)
		}
		cfg.DefaultScope = scope
	}

	cfg.Targets = mergeTargets(target.Builtins(), fc.Targets)
	return cfg, nil
}

// defaults returns a Config populated with built-in defaults, including the
// built-in target set (with tildes expanded).
func defaults() *Config {
	return &Config{
		SkillsDir:      DefaultSkillsDir,
		Targets:        mergeTargets(target.Builtins(), nil),
		DefaultTargets: []string{DefaultTarget},
		DefaultScope:   DefaultScope,
	}
}

// mergeTargets folds per-target overrides from the config file onto the
// built-in target set. Builtins keep their declaration order. Overrides
// against unknown target names are appended as new (custom) targets sorted
// by name. Personal directories are tilde-expanded.
func mergeTargets(builtins []target.Target, overrides map[string]targetSection) []target.Target {
	out := make([]target.Target, 0, len(builtins))
	used := map[string]bool{}

	for _, b := range builtins {
		t := b
		if ov, ok := overrides[t.Name]; ok {
			if ov.PersonalDir != nil {
				t.PersonalDir = *ov.PersonalDir
			}
			if ov.ProjectDir != nil {
				t.ProjectDir = *ov.ProjectDir
			}
			used[t.Name] = true
		}
		t.PersonalDir = ExpandPath(t.PersonalDir)
		out = append(out, t)
	}

	customNames := make([]string, 0)
	for name := range overrides {
		if used[name] {
			continue
		}
		customNames = append(customNames, name)
	}
	sort.Strings(customNames)
	for _, name := range customNames {
		ov := overrides[name]
		t := target.Target{Name: name}
		if ov.PersonalDir != nil {
			t.PersonalDir = ExpandPath(*ov.PersonalDir)
		}
		if ov.ProjectDir != nil {
			t.ProjectDir = *ov.ProjectDir
		}
		out = append(out, t)
	}
	return out
}

// ResolveSkillsDir returns the effective legacy skills directory, expanded,
// applying the precedence: flagValue, then $KUNGFU_SKILLS_DIR, then the
// config file, then DefaultSkillsDir. Pass an empty flagValue when the flag
// was not set.
//
// Retained for PR 1 compatibility; the new multi-target commands use
// ResolveTargets instead.
func (c *Config) ResolveSkillsDir(flagValue string) string {
	v := firstNonEmpty(
		strings.TrimSpace(flagValue),
		strings.TrimSpace(os.Getenv(EnvSkillsDir)),
		strings.TrimSpace(c.SkillsDir),
		DefaultSkillsDir,
	)
	return ExpandPath(v)
}

// ResolveExtraSkillsDirs returns the legacy extra_skills_dirs entries with
// tildes expanded and empty entries dropped.
func (c *Config) ResolveExtraSkillsDirs() []string {
	out := make([]string, 0, len(c.ExtraSkillsDirs))
	for _, d := range c.ExtraSkillsDirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		out = append(out, ExpandPath(d))
	}
	return out
}

// ResolveTargets returns the effective set of targets for the current
// invocation. When flag is non-empty its value (a comma-separated list or
// "all") is parsed; otherwise c.DefaultTargets is used.
func (c *Config) ResolveTargets(flag string) ([]target.Target, error) {
	eff := strings.TrimSpace(flag)
	if eff == "" {
		eff = strings.Join(c.DefaultTargets, ",")
	}
	if eff == "" {
		return nil, fmt.Errorf("config: no targets configured (default_targets is empty)")
	}
	return target.Resolve(eff, c.Targets)
}

// ResolveScope returns the effective scope. When flag is non-empty it must
// be a valid scope string; otherwise c.DefaultScope is returned.
func (c *Config) ResolveScope(flag string) (target.Scope, error) {
	eff := strings.TrimSpace(flag)
	if eff == "" {
		return c.DefaultScope, nil
	}
	s := target.Scope(eff)
	if !s.IsValid() {
		return "", fmt.Errorf("config: invalid scope %q (want \"personal\" or \"project\")", eff)
	}
	return s, nil
}

// ExpandPath expands a leading ~ or ~/ to the user's home directory. Any
// other path is returned unchanged. If the home directory cannot be
// determined the original path is returned.
func ExpandPath(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
