package skill

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// scriptExtensions lists suffixes we treat as executables under scripts/.
var scriptExtensions = []string{".sh", ".py", ".bash"}

// FileMode returns the canonical mode for a file at relPath inside a skill
// directory. Files under scripts/ that look like scripts (known suffix or
// no extension) become 0o755; everything else is 0o644.
//
// The helper is shared by extract (remote install) and template Apply
// (kungfu new) so both code paths produce bit-identical permissions for
// the same logical file layout. Callers must pass forward-slash separated
// paths or filepath-style paths consistently — both work, the function
// inspects only the basename and the leading directory component.
func FileMode(relPath string) fs.FileMode {
	rel := filepath.ToSlash(relPath)
	rel = strings.TrimPrefix(rel, "./")
	if !strings.HasPrefix(rel, "scripts/") {
		return 0o644
	}
	name := filepath.Base(rel)
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return 0o755 // bare scripts (e.g. scripts/run) are executable
	}
	for _, e := range scriptExtensions {
		if ext == e {
			return 0o755
		}
	}
	return 0o644
}
