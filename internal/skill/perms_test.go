package skill

import "testing"

func TestFileMode(t *testing.T) {
	cases := []struct {
		path string
		want uint32
	}{
		{"SKILL.md", 0o644},
		{"references/style.md", 0o644},
		{"scripts/run.sh", 0o755},
		{"scripts/analyze.py", 0o755},
		{"scripts/run.bash", 0o755},
		{"scripts/run", 0o755}, // no extension under scripts/ is executable
		{"scripts/data.txt", 0o644},
		{"scripts/Run.SH", 0o755},   // case-insensitive
		{"./scripts/run.sh", 0o755}, // tolerates leading "./"
		{"top-level.sh", 0o644},     // .sh outside scripts/ is not executable
		{"assets/icon.png", 0o644},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			if got := FileMode(tc.path); uint32(got) != tc.want {
				t.Errorf("FileMode(%q) = 0o%o, want 0o%o", tc.path, got, tc.want)
			}
		})
	}
}
