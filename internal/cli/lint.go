package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newLintCmd builds the `kungfu lint` command.
//
// Exit codes:
//
//	0 — clean (or warnings without --strict)
//	1 — errors present
//	2 — warnings present and --strict
//	3 — I/O or load failure
func newLintCmd() *cobra.Command {
	var (
		strict bool
		asJSON bool
		fix    bool
	)
	cmd := &cobra.Command{
		Use:   "lint <path>",
		Short: "Validate a skill against the kungfu rule set",
		Long: "Validate a skill directory against the kungfu rule set.\n\n" +
			"<path> must be a directory containing a SKILL.md file. Each rule emits\n" +
			"diagnostics with a stable rule id (e.g. frontmatter/name-mismatch) so\n" +
			"you can grep for and act on them in a script.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]

			if fix {
				if err := runFix(cmd.OutOrStdout(), path); err != nil {
					return &ExitError{Code: 3, Err: err}
				}
			}

			linter := lint.NewDefault()
			rep, err := linter.Lint(path)
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}

			if asJSON {
				if err := renderLintJSON(cmd.OutOrStdout(), rep); err != nil {
					return &ExitError{Code: 3, Err: err}
				}
			} else {
				renderLintHuman(cmd.OutOrStdout(), rep)
			}

			switch {
			case rep.HasErrors():
				return &ExitError{Code: 1}
			case strict && len(rep.Warnings()) > 0:
				return &ExitError{Code: 2}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit non-zero on warnings as well as errors")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output diagnostics as JSON")
	cmd.Flags().BoolVar(&fix, "fix", false, "trim trailing whitespace and re-serialize frontmatter cleanly")
	return cmd
}

// lintJSONOutput is the schema emitted by `kungfu lint --json`.
type lintJSONOutput struct {
	Path        string            `json:"path"`
	Diagnostics []lint.Diagnostic `json:"diagnostics"`
	Summary     lintSummary       `json:"summary"`
}

type lintSummary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
}

func renderLintJSON(w io.Writer, rep *lint.Report) error {
	out := lintJSONOutput{
		Path:        rep.SkillPath,
		Diagnostics: rep.Diagnostics,
		Summary: lintSummary{
			Errors:   len(rep.Errors()),
			Warnings: len(rep.Warnings()),
		},
	}
	return renderJSON(w, out)
}

func renderLintHuman(w io.Writer, rep *lint.Report) {
	cwd, _ := os.Getwd()
	display := func(p string) string {
		if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
		return p
	}

	if len(rep.Diagnostics) == 0 {
		fmt.Fprintln(w, ui.Success.Render("✓")+" "+display(rep.SkillPath)+": no issues")
		return
	}

	byPath := map[string][]lint.Diagnostic{}
	for _, d := range rep.Diagnostics {
		byPath[d.Path] = append(byPath[d.Path], d)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		fmt.Fprintln(w, ui.Bold.Render(display(p)))
		for _, d := range byPath[p] {
			label := ui.Error.Render(d.Severity.Label())
			if d.Severity == lint.SeverityWarning {
				label = ui.Warning.Render(d.Severity.Label())
			}
			location := ""
			if d.Line > 0 {
				location = ui.Muted.Render(fmt.Sprintf(" (line %d)", d.Line))
			}
			fmt.Fprintf(w, "  %s  %s  %s%s\n",
				label, ui.Muted.Render(d.Rule), d.Message, location)
		}
		fmt.Fprintln(w)
	}

	errs := len(rep.Errors())
	warns := len(rep.Warnings())
	summary := fmt.Sprintf("%d %s, %d %s",
		errs, pluralize(errs, "error"),
		warns, pluralize(warns, "warning"))
	switch {
	case errs > 0:
		fmt.Fprintln(w, ui.Error.Render(summary))
	case warns > 0:
		fmt.Fprintln(w, ui.Warning.Render(summary))
	default:
		fmt.Fprintln(w, ui.Success.Render(summary))
	}
}

func pluralize(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}

// runFix normalizes the SKILL.md at path: trims trailing whitespace from
// each body line and re-serializes the frontmatter through the canonical
// encoder. Structural fixes are out of scope.
func runFix(out io.Writer, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("fix: resolving %s: %w", path, err)
	}
	skillFile := filepath.Join(abs, skill.FileName)
	before, err := os.ReadFile(skillFile)
	if err != nil {
		return fmt.Errorf("fix: reading %s: %w", skillFile, err)
	}

	s, err := skill.Load(abs)
	if err != nil {
		return fmt.Errorf("fix: %w", err)
	}

	if s.Body != "" {
		lines := strings.Split(s.Body, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t\r")
		}
		s.Body = strings.Join(lines, "\n")
	}

	if err := s.Save(); err != nil {
		return fmt.Errorf("fix: saving %s: %w", skillFile, err)
	}

	after, err := os.ReadFile(skillFile)
	if err != nil {
		return fmt.Errorf("fix: reading %s after save: %w", skillFile, err)
	}
	if bytes.Equal(before, after) {
		fmt.Fprintln(out, ui.Muted.Render("fix: no changes needed"))
	} else {
		fmt.Fprintln(out, ui.Success.Render("fix: normalized ")+skillFile)
	}
	return nil
}
