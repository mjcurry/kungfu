package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProvenanceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	content := "---\n" +
		"name: " + filepath.Base(dir) + "\n" +
		"description: Use this skill when testing provenance.\n" +
		"kungfu_source: github.com/owner/repo\n" +
		"kungfu_ref: v1.0.0\n" +
		"kungfu_sha: 0123456789abcdef0123456789abcdef01234567\n" +
		"kungfu_installed_at: 2026-05-19T00:00:00Z\n" +
		"---\n\n# Body\n"
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Source != "github.com/owner/repo" {
		t.Errorf("Source = %q", s.Source)
	}
	if s.Ref != "v1.0.0" {
		t.Errorf("Ref = %q", s.Ref)
	}
	if s.SHA != "0123456789abcdef0123456789abcdef01234567" {
		t.Errorf("SHA = %q", s.SHA)
	}
	if s.InstalledAt != "2026-05-19T00:00:00Z" {
		t.Errorf("InstalledAt = %q", s.InstalledAt)
	}

	// Round-trip through Save: provenance must survive.
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.SHA != s.SHA || reloaded.Ref != s.Ref {
		t.Errorf("provenance lost on round trip")
	}
	// Clearing provenance and saving drops the keys.
	reloaded.Source = ""
	reloaded.Ref = ""
	reloaded.SHA = ""
	reloaded.InstalledAt = ""
	if err := reloaded.Save(); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, FileName))
	for _, key := range []string{
		FrontmatterSource, FrontmatterRef, FrontmatterSHA, FrontmatterInstalledAt,
	} {
		if strings.Contains(string(data), key+":") {
			t.Errorf("expected %q to be removed after clearing", key)
		}
	}
}
