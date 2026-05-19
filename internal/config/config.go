// Package config loads kungfu's TOML configuration and resolves effective
// settings from the layered sources kungfu supports: command-line flags,
// environment variables, the config file, and built-in defaults.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// DefaultSkillsDir is used when no source specifies a skills directory.
	DefaultSkillsDir = "~/.claude/skills"

	// EnvSkillsDir overrides the configured skills directory.
	EnvSkillsDir = "KUNGFU_SKILLS_DIR"

	appName  = "kungfu"
	fileName = "config.toml"
)

// Config is the on-disk kungfu configuration.
type Config struct {
	// SkillsDir is the primary directory kungfu manages skills in. A
	// leading ~ is expanded against the user's home directory.
	SkillsDir string `toml:"skills_dir"`

	// ExtraSkillsDirs are additional, read-also skill directories.
	ExtraSkillsDirs []string `toml:"extra_skills_dirs"`
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
	cfg := &Config{SkillsDir: DefaultSkillsDir}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("config: reading %s: %w", path, err)
	}

	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parsing %s: %w", path, err)
	}
	if strings.TrimSpace(cfg.SkillsDir) == "" {
		cfg.SkillsDir = DefaultSkillsDir
	}
	return cfg, nil
}

// ResolveSkillsDir returns the effective skills directory, expanded, applying
// the precedence: flagValue, then $KUNGFU_SKILLS_DIR, then the config file,
// then DefaultSkillsDir. Pass an empty flagValue when the flag was not set.
func (c *Config) ResolveSkillsDir(flagValue string) string {
	v := firstNonEmpty(
		strings.TrimSpace(flagValue),
		strings.TrimSpace(os.Getenv(EnvSkillsDir)),
		strings.TrimSpace(c.SkillsDir),
		DefaultSkillsDir,
	)
	return ExpandPath(v)
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
