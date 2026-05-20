package rules

import (
	"bytes"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/mjcurry/kungfu/internal/skill"
)

// pathExtensions promotes a code span to a path-like reference candidate
// even when it does not contain a slash.
var pathExtensions = []string{
	".md", ".py", ".sh", ".bash", ".txt",
	".json", ".yaml", ".yml", ".toml",
	".go", ".js", ".mjs", ".ts", ".tsx",
	".html", ".css", ".sql",
	".tmpl", ".tpl",
}

// ReferencesBroken flags markdown links and path-like inline code spans in
// the body that point at files which do not exist under the skill directory.
type ReferencesBroken struct{}

func (ReferencesBroken) ID() string { return "references/broken" }

// Check re-reads SKILL.md from disk so it can attribute diagnostics to file
// lines (the in-memory skill.Skill carries only the body, not its file
// offset).
func (ReferencesBroken) Check(s *skill.Skill) []Diagnostic {
	file := skillFile(s.Dir)
	content, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	_, bodyStr, bodyLine, splitErr := skill.SplitFrontmatter(content)
	if splitErr != nil || bodyStr == "" {
		return nil
	}
	body := []byte(bodyStr)

	parser := goldmark.New().Parser()
	doc := parser.Parse(text.NewReader(body))

	var diags []Diagnostic
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			target := string(v.Destination)
			if !isRelativeFileRef(target) || refExists(s.Dir, target) {
				return ast.WalkContinue, nil
			}
			diags = append(diags, Diagnostic{
				Path:     file,
				Line:     lineForNode(body, n, bodyLine),
				Severity: SeverityError,
				Rule:     "references/broken",
				Message:  "linked file " + target + " does not exist in the skill",
			})
		case *ast.CodeSpan:
			content := codeSpanText(v, body)
			if !looksLikePath(content) || refExists(s.Dir, content) {
				return ast.WalkContinue, nil
			}
			diags = append(diags, Diagnostic{
				Path:     file,
				Line:     lineForNode(body, n, bodyLine),
				Severity: SeverityError,
				Rule:     "references/broken",
				Message:  "code span references " + content + " which does not exist in the skill",
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// isRelativeFileRef reports whether target is a relative file reference we
// should try to resolve. URLs, anchors, and absolute paths are skipped.
func isRelativeFileRef(target string) bool {
	if target == "" {
		return false
	}
	if strings.HasPrefix(target, "#") {
		return false
	}
	if strings.HasPrefix(target, "/") {
		return false
	}
	if u, err := url.Parse(target); err == nil && u.Scheme != "" {
		return false
	}
	return true
}

// refExists reports whether rel resolves to something inside the skill dir.
// Strips fragment and query before resolving.
func refExists(dir, rel string) bool {
	if i := strings.IndexAny(rel, "#?"); i >= 0 {
		rel = rel[:i]
	}
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return true
	}
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	_, err := os.Stat(abs)
	return err == nil
}

// looksLikePath reports whether s reads as a relative file path: either it
// contains a directory separator or ends in a known file extension.
func looksLikePath(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\n") {
		return false
	}
	if strings.Contains(s, "/") {
		return true
	}
	lower := strings.ToLower(s)
	for _, ext := range pathExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// codeSpanText concatenates the text content of an inline code span.
func codeSpanText(n *ast.CodeSpan, source []byte) string {
	var buf bytes.Buffer
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			buf.Write(t.Segment.Value(source))
		}
	}
	return buf.String()
}

// lineForNode returns the 1-indexed file line corresponding to node. It
// uses the first descendant text segment for inline-node accuracy and
// falls back to the nearest ancestor block's first line.
func lineForNode(body []byte, node ast.Node, bodyLine int) int {
	if seg, ok := firstTextSegment(node); ok {
		return bodyLine + bytes.Count(body[:clamp(seg.Start, len(body))], []byte{'\n'})
	}
	for n := node; n != nil; n = n.Parent() {
		if lines := n.Lines(); lines != nil && lines.Len() > 0 {
			s := lines.At(0)
			return bodyLine + bytes.Count(body[:clamp(s.Start, len(body))], []byte{'\n'})
		}
	}
	return bodyLine
}

func firstTextSegment(node ast.Node) (text.Segment, bool) {
	var found text.Segment
	ok := false
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, isText := n.(*ast.Text); isText && !ok {
			found = t.Segment
			ok = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found, ok
}

func clamp(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}
