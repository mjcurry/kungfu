package template

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	texttemplate "text/template"

	"github.com/mjcurry/kungfu/internal/skill"
)

// templateSuffix is the rendered-file extension applied to files inside a
// template directory.
const templateSuffix = ".tmpl"

// renderTemplate walks the embedded template directory for name, renders any
// .tmpl files against vars, copies non-.tmpl files verbatim, and applies
// skill.FileMode to every output file. destDir must already not exist.
func renderTemplate(name, destDir string, vars Vars) error {
	root := path.Join(templatesRoot, name)
	if _, err := fs.Stat(templateFS, root); err != nil {
		return fmt.Errorf("template: %w", err)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("template: creating destination: %w", err)
	}
	return fs.WalkDir(templateFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, root)
		rel = strings.TrimPrefix(rel, "/")
		if rel == "" {
			return nil
		}
		outRel := strings.TrimSuffix(rel, templateSuffix)
		out := filepath.Join(destDir, filepath.FromSlash(outRel))
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			return err
		}
		data, err := fs.ReadFile(templateFS, p)
		if err != nil {
			return err
		}
		if strings.HasSuffix(p, templateSuffix) {
			data, err = renderBytes(p, data, vars)
			if err != nil {
				return err
			}
		}
		return os.WriteFile(out, data, skill.FileMode(outRel))
	})
}

// renderBytes runs text/template against data with vars. missingkey=error
// makes template typos fail loudly rather than silently inserting "<no value>".
func renderBytes(sourceName string, data []byte, vars Vars) ([]byte, error) {
	tmpl, err := texttemplate.New(sourceName).
		Option("missingkey=error").
		Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("template: parsing %s: %w", sourceName, err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return nil, fmt.Errorf("template: rendering %s: %w", sourceName, err)
	}
	return buf.Bytes(), nil
}

// stat / removeAll wrap the corresponding os calls so template.go does not
// have to import "os" just for two thin shims used by Apply.
func stat(p string) (os.FileInfo, error) { return os.Stat(p) }
func removeAll(p string) error           { return os.RemoveAll(p) }
