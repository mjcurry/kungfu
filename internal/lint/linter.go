package lint

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/mjcurry/kungfu/internal/lint/rules"
	"github.com/mjcurry/kungfu/internal/skill"
)

// Linter runs a set of Rules against a skill and aggregates the resulting
// diagnostics into a Report.
type Linter struct {
	rules []Rule
}

// New constructs a Linter with the given rules, in the order they will run.
func New(rs ...Rule) *Linter {
	return &Linter{rules: rs}
}

// NewDefault constructs a Linter populated with the standard rule set.
func NewDefault() *Linter {
	return New(rules.DefaultRules()...)
}

// Rules returns the rules this linter will run. Callers must not mutate it.
func (l *Linter) Rules() []Rule {
	return l.rules
}

// Lint runs every configured rule against the skill at path and returns a
// Report.
//
// path must be a directory containing a SKILL.md. Lint returns a non-nil
// error only for I/O failures (path missing, unreadable, not a directory).
// Diagnostic-level problems — including missing or malformed frontmatter —
// are returned as Diagnostics in the Report so callers receive every issue
// in a single pass.
func (l *Linter) Lint(path string) (*Report, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("lint: resolving %s: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("lint: stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("lint: %s is not a directory", abs)
	}

	skillFile := filepath.Join(abs, skill.FileName)
	content, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, fmt.Errorf("lint: reading %s: %w", skillFile, err)
	}

	rep := &Report{SkillPath: abs}

	s, preDiags, ok := loadPermissive(abs, skillFile, content)
	rep.Diagnostics = append(rep.Diagnostics, preDiags...)
	if !ok {
		rep.sort()
		return rep, nil
	}

	for _, rule := range l.rules {
		rep.Diagnostics = append(rep.Diagnostics, rule.Check(s)...)
	}
	rep.sort()
	return rep, nil
}

// loadPermissive parses content into a (possibly partial) skill so rules can
// run even when modeled fields are missing or wrong-typed. Frontmatter-level
// problems that prevent any meaningful rule run — no frontmatter, an
// unterminated block, YAML that does not parse, or YAML that is not a
// mapping — short-circuit and produce a single diagnostic.
func loadPermissive(dir, skillFile string, content []byte) (*skill.Skill, []Diagnostic, bool) {
	front, body, _, splitErr := skill.SplitFrontmatter(content)
	if splitErr != nil {
		rule := "frontmatter/malformed"
		if errors.Is(splitErr, skill.ErrNoFrontmatter) {
			rule = "frontmatter/missing"
		}
		return nil, []Diagnostic{{
			Path:     skillFile,
			Line:     1,
			Severity: SeverityError,
			Rule:     rule,
			Message:  splitErr.Error(),
		}}, false
	}

	var fm map[string]any
	if err := yaml.Unmarshal(front, &fm); err != nil {
		return nil, []Diagnostic{{
			Path:     skillFile,
			Line:     yamlErrorLine(err, 1), // +1 to skip past the opening fence
			Severity: SeverityError,
			Rule:     "frontmatter/malformed",
			Message:  "frontmatter YAML did not parse: " + err.Error(),
		}}, false
	}
	if fm == nil {
		fm = map[string]any{}
	}

	s := &skill.Skill{
		Dir:         dir,
		Body:        body,
		Frontmatter: fm,
	}
	if n, ok := fm["name"].(string); ok {
		s.Name = n
	}
	if d, ok := fm["description"].(string); ok {
		s.Description = d
	}
	switch at := fm["allowed-tools"].(type) {
	case []any:
		if strs, ok := allStrings(at); ok {
			s.AllowedTools = strs
		}
	case []string:
		s.AllowedTools = at
	}
	return s, nil, true
}

func allStrings(in []any) ([]string, bool) {
	out := make([]string, 0, len(in))
	for _, v := range in {
		s, ok := v.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// yamlErrorLine extracts the line number from a yaml.v3 error message and
// adds fileLineOffset (the YAML input begins after the opening fence, so
// the file line is the YAML line plus one).
var yamlLineRE = regexp.MustCompile(`yaml: line (\d+):`)

func yamlErrorLine(err error, fileLineOffset int) int {
	if err == nil {
		return 0
	}
	m := yamlLineRE.FindStringSubmatch(err.Error())
	if len(m) < 2 {
		return 0
	}
	n, perr := strconv.Atoi(m[1])
	if perr != nil {
		return 0
	}
	return n + fileLineOffset
}

// Report is the result of running a Linter against a skill.
type Report struct {
	// SkillPath is the absolute path of the skill directory.
	SkillPath string `json:"path"`
	// Diagnostics is every diagnostic emitted by the run, sorted by file
	// path, line, severity descending, and rule id.
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// Errors returns the SeverityError diagnostics in the report.
func (r *Report) Errors() []Diagnostic { return r.bySeverity(SeverityError) }

// Warnings returns the SeverityWarning diagnostics in the report.
func (r *Report) Warnings() []Diagnostic { return r.bySeverity(SeverityWarning) }

// HasErrors reports whether the report contains at least one SeverityError.
func (r *Report) HasErrors() bool {
	for _, d := range r.Diagnostics {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

func (r *Report) bySeverity(want Severity) []Diagnostic {
	out := make([]Diagnostic, 0)
	for _, d := range r.Diagnostics {
		if d.Severity == want {
			out = append(out, d)
		}
	}
	return out
}

// sort orders diagnostics by file path, then line, then severity descending
// (errors before warnings), then rule id.
func (r *Report) sort() {
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Severity != b.Severity {
			return a.Severity > b.Severity
		}
		return a.Rule < b.Rule
	})
}
