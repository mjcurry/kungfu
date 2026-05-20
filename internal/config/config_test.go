package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/target"
)

// setHomeForTest sets the env var that os.UserHomeDir consults on the
// current platform. On Linux/macOS that's HOME; on Windows it's USERPROFILE.
// Tests that only set HOME pass on Unix but silently no-op on Windows.
func setHomeForTest(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}
}

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
	setHomeForTest(t, home)
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
		setHomeForTest(t, home)
		want := filepath.Join(home, ".claude", "skills")
		if got := empty.ResolveSkillsDir(""); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}

func TestExpandPath(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)

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
	setHomeForTest(t, home)
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

// ---------------- target-aware schema ----------------

func TestLoadMissingTargetsSectionUsesBuiltins(t *testing.T) {
	home := t.TempDir()
	setHomeForTest(t, home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	wantNames := []string{"claude", "codex", "cursor", "copilot"}
	if len(cfg.Targets) != len(wantNames) {
		t.Fatalf("got %d targets, want %d", len(cfg.Targets), len(wantNames))
	}
	for i, name := range wantNames {
		if cfg.Targets[i].Name != name {
			t.Errorf("targets[%d].Name = %q, want %q", i, cfg.Targets[i].Name, name)
		}
	}
	// Tildes must be expanded against HOME.
	claude, err := target.ByName("claude", cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".claude", "skills")
	if claude.PersonalDir != want {
		t.Errorf("claude.PersonalDir = %q, want %q", claude.PersonalDir, want)
	}
	// Cursor still has no personal dir.
	cursor, _ := target.ByName("cursor", cfg.Targets)
	if cursor.PersonalDir != "" {
		t.Errorf("cursor.PersonalDir = %q, want empty", cursor.PersonalDir)
	}
}

func TestLoadPartialTargetOverride(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()
	setHomeForTest(t, home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	writeConfig(t, xdg, `
[targets.claude]
personal_dir = "/srv/claude"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	claude, err := target.ByName("claude", cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	if claude.PersonalDir != "/srv/claude" {
		t.Errorf("PersonalDir = %q, want /srv/claude", claude.PersonalDir)
	}
	// project_dir was NOT overridden → falls back to the builtin value.
	if claude.ProjectDir != ".claude/skills" {
		t.Errorf("ProjectDir = %q, want builtin '.claude/skills'", claude.ProjectDir)
	}
	// Other targets stay at builtin values.
	codex, _ := target.ByName("codex", cfg.Targets)
	if codex.PersonalDir != filepath.Join(home, ".codex", "skills") {
		t.Errorf("codex.PersonalDir = %q, want builtin", codex.PersonalDir)
	}
}

func TestLoadCustomTargetAdded(t *testing.T) {
	xdg := t.TempDir()
	home := t.TempDir()
	setHomeForTest(t, home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	writeConfig(t, xdg, `
[targets.aider]
personal_dir = "~/.aider/skills"
project_dir  = ".aider/skills"
`)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 5 {
		t.Fatalf("got %d targets, want 4 builtins + 1 custom", len(cfg.Targets))
	}
	aider, err := target.ByName("aider", cfg.Targets)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".aider", "skills")
	if aider.PersonalDir != want {
		t.Errorf("aider.PersonalDir = %q, want %q", aider.PersonalDir, want)
	}
	if aider.ProjectDir != ".aider/skills" {
		t.Errorf("aider.ProjectDir = %q", aider.ProjectDir)
	}
}

func TestLoadInvalidScopeErrors(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	writeConfig(t, xdg, `default_scope = "kitchen-sink"`+"\n")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "default_scope") {
		t.Fatalf("err = %v, want default_scope validation error", err)
	}
}

func TestResolveTargets(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("flag overrides defaults", func(t *testing.T) {
		got, err := cfg.ResolveTargets("codex,cursor")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Name != "codex" || got[1].Name != "cursor" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("empty flag uses default_targets", func(t *testing.T) {
		got, err := cfg.ResolveTargets("")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].Name != "claude" {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("all expands to every configured target", func(t *testing.T) {
		got, err := cfg.ResolveTargets("all")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 4 {
			t.Errorf("len = %d, want 4", len(got))
		}
	})

	t.Run("unknown name errors", func(t *testing.T) {
		if _, err := cfg.ResolveTargets("aider"); err == nil {
			t.Errorf("want error for unknown target name")
		}
	})
}

func TestResolveScope(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(t.TempDir(), "missing.toml"))
	if err != nil {
		t.Fatal(err)
	}

	t.Run("empty flag uses default_scope", func(t *testing.T) {
		got, err := cfg.ResolveScope("")
		if err != nil {
			t.Fatal(err)
		}
		if got != target.ScopePersonal {
			t.Errorf("got %q, want personal", got)
		}
	})

	t.Run("flag wins", func(t *testing.T) {
		got, err := cfg.ResolveScope("project")
		if err != nil {
			t.Fatal(err)
		}
		if got != target.ScopeProject {
			t.Errorf("got %q, want project", got)
		}
	})

	t.Run("invalid flag errors", func(t *testing.T) {
		if _, err := cfg.ResolveScope("kitchen"); err == nil {
			t.Errorf("want error for invalid scope")
		}
	})
}
