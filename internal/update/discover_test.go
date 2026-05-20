package update

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/skill"
	"github.com/mjcurry/kungfu/internal/target"
)

// writeSkill is a per-test helper that drops a SKILL.md with the requested
// frontmatter into a fresh directory under parent.
func writeSkill(t *testing.T, parent, name, frontmatter string) string {
	t.Helper()
	dir := filepath.Join(parent, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: Use this skill when testing update.\n" +
		frontmatter + "---\n\n# " + name + "\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(dir, skill.FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func locationFor(dir, targetName, scope string) target.Location {
	return target.Location{
		Target: target.Target{Name: targetName},
		Scope:  target.Scope(scope),
		Dir:    dir,
	}
}

func TestDiscoverUpdatable_PicksUpProvenance(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "remote", "kungfu_source: github.com/acme/remote\nkungfu_ref: v1.0.0\nkungfu_sha: a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2\n")

	got, err := DiscoverUpdatable([]target.Location{locationFor(root, "claude", "personal")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1", len(got))
	}
	u := got[0]
	if u.Source.Owner != "acme" || u.Source.Repo != "remote" {
		t.Errorf("Source = %+v", u.Source)
	}
	if u.Source.Ref != "v1.0.0" || u.StoredRef != "v1.0.0" {
		t.Errorf("Ref = %q, StoredRef = %q", u.Source.Ref, u.StoredRef)
	}
	if u.StoredSHA != "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2" {
		t.Errorf("StoredSHA = %q", u.StoredSHA)
	}
}

func TestDiscoverUpdatable_SkipsSkillsWithoutProvenance(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "local-only", "") // no kungfu_* fields
	got, err := DiscoverUpdatable([]target.Location{locationFor(root, "claude", "personal")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no Updatables, got %+v", got)
	}
}

func TestDiscoverUpdatable_WarnsOnMalformedProvenance(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "broken-prov", "kungfu_source: \"://invalid host\"\nkungfu_ref: main\n")

	var warns []string
	got, err := DiscoverUpdatable(
		[]target.Location{locationFor(root, "claude", "personal")},
		func(dir string, err error) { warns = append(warns, err.Error()) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("malformed-provenance skill should be excluded: %+v", got)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "unparseable provenance") {
		t.Errorf("expected one unparseable-provenance warning, got %v", warns)
	}
}

func TestDiscoverUpdatable_NoOnWarnTolerated(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, root, "broken", "kungfu_source: \"://x\"\nkungfu_ref: main\n")
	got, err := DiscoverUpdatable([]target.Location{locationFor(root, "claude", "personal")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no Updatables when warner is nil and provenance is bad, got %+v", got)
	}
}

func TestDiscoverUpdatable_MissingLocationDirIgnored(t *testing.T) {
	// Pointing at a directory that does not exist should not error — empty
	// or absent target dirs are routine during early-life-cycle states.
	got, err := DiscoverUpdatable([]target.Location{locationFor(filepath.Join(t.TempDir(), "missing"), "claude", "personal")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestDiscoverUpdatable_MultipleLocations(t *testing.T) {
	root := t.TempDir()
	claudeDir := filepath.Join(root, "claude")
	codexDir := filepath.Join(root, "codex")
	writeSkill(t, claudeDir, "csv", "kungfu_source: github.com/acme/csv\nkungfu_ref: main\nkungfu_sha: b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3\n")
	writeSkill(t, codexDir, "csv", "kungfu_source: github.com/acme/csv\nkungfu_ref: main\nkungfu_sha: b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3\n")

	got, err := DiscoverUpdatable([]target.Location{
		locationFor(claudeDir, "claude", "personal"),
		locationFor(codexDir, "codex", "personal"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	if got[0].Location.Target.Name != "claude" || got[1].Location.Target.Name != "codex" {
		t.Errorf("locations not preserved in order: %+v", got)
	}
}
