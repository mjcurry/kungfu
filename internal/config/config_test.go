package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, xdg, body string) {
	t.Helper()
	dir := filepath.Join(xdg, appName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, fileName), []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestPathUsesXDG(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "kungfu", "config.toml")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestPathFallsBackToHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", home)
	want := filepath.Join(home, ".config", "kungfu", "config.toml")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

func TestLoadMissingFileReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty: no config file
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.SkillsDir != DefaultSkillsDir {
		t.Errorf("SkillsDir = %q, want default %q", cfg.SkillsDir, DefaultSkillsDir)
	}
	if len(cfg.ExtraSkillsDirs) != 0 {
		t.Errorf("ExtraSkillsDirs = %v, want empty", cfg.ExtraSkillsDirs)
	}
}

func TestLoadReadsConfigFile(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeConfig(t, xdg, "skills_dir = \"/srv/skills\"\nextra_skills_dirs = [\"/a\", \"/b\"]\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.SkillsDir != "/srv/skills" {
		t.Errorf("SkillsDir = %q, want /srv/skills", cfg.SkillsDir)
	}
	if len(cfg.ExtraSkillsDirs) != 2 {
		t.Errorf("ExtraSkillsDirs = %v, want 2 entries", cfg.ExtraSkillsDirs)
	}
}

func TestLoadMalformedTOMLErrors(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeConfig(t, xdg, "skills_dir = = oops\n")

	if _, err := Load(); err == nil {
		t.Fatal("Load() with malformed TOML: want error, got nil")
	}
}

func TestResolveSkillsDirPrecedence(t *testing.T) {
	cfg := &Config{SkillsDir: "/from/config"}

	t.Run("flag wins", func(t *testing.T) {
		t.Setenv(EnvSkillsDir, "/from/env")
		if got := cfg.ResolveSkillsDir("/from/flag"); got != "/from/flag" {
			t.Errorf("got %q, want /from/flag", got)
		}
	})

	t.Run("env beats config", func(t *testing.T) {
		t.Setenv(EnvSkillsDir, "/from/env")
		if got := cfg.ResolveSkillsDir(""); got != "/from/env" {
			t.Errorf("got %q, want /from/env", got)
		}
	})

	t.Run("config beats default", func(t *testing.T) {
		t.Setenv(EnvSkillsDir, "")
		if got := cfg.ResolveSkillsDir(""); got != "/from/config" {
			t.Errorf("got %q, want /from/config", got)
		}
	})

	t.Run("default when all empty", func(t *testing.T) {
		t.Setenv(EnvSkillsDir, "")
		empty := &Config{}
		home := t.TempDir()
		t.Setenv("HOME", home)
		want := filepath.Join(home, ".claude", "skills")
		if got := empty.ResolveSkillsDir(""); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	tests := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/.claude/skills", filepath.Join(home, ".claude", "skills")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~notme/x", "~notme/x"}, // only ~ and ~/ expand
	}
	for _, tt := range tests {
		if got := ExpandPath(tt.in); got != tt.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveSkillsDirExpandsTilde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvSkillsDir, "")
	cfg := &Config{SkillsDir: "~/skills"}
	want := filepath.Join(home, "skills")
	if got := cfg.ResolveSkillsDir(""); got != want {
		t.Errorf("ResolveSkillsDir() = %q, want %q", got, want)
	}
	if strings.Contains(cfg.ResolveSkillsDir(""), "~") {
		t.Errorf("tilde was not expanded")
	}
}
