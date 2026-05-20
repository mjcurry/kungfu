package rules

import (
	"io/fs"
	"path/filepath"
	"unicode"

	"github.com/mjcurry/kungfu/internal/skill"
)

// FilenamesNonASCII warns about files and directories inside the skill
// whose names contain non-ASCII characters or whitespace. Such names work
// locally but tend to cause friction across operating systems, archive
// formats, and shell pipelines.
type FilenamesNonASCII struct{}

func (FilenamesNonASCII) ID() string { return "filenames/non-ascii" }

func (FilenamesNonASCII) Check(s *skill.Skill) []Diagnostic {
	var diags []Diagnostic
	_ = filepath.WalkDir(s.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || path == s.Dir {
			return nil
		}
		name := d.Name()
		if !hasUnsafeChar(name) {
			return nil
		}
		diags = append(diags, Diagnostic{
			Path:     path,
			Severity: SeverityWarning,
			Rule:     "filenames/non-ascii",
			Message:  "filename contains non-ASCII or whitespace characters",
		})
		return nil
	})
	return diags
}

// hasUnsafeChar reports whether name contains any byte outside printable
// ASCII or any whitespace rune.
func hasUnsafeChar(name string) bool {
	for _, r := range name {
		if r > unicode.MaxASCII {
			return true
		}
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}
