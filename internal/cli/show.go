package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newShowCmd builds the `kungfu show` command.
//
// Exit codes:
//
//	0 — printed
//	1 — not found, or ambiguous and the user didn't disambiguate
//	2 — I/O failure
func newShowCmd() *cobra.Command {
	var (
		raw      bool
		pathOnly bool
	)
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Print a skill's contents",
		Long: "Print a skill found in the configured targets. With --target /\n" +
			"--scope (root persistent flags) you can disambiguate when the same\n" +
			"name is installed under multiple targets.\n\n" +
			"--raw prints SKILL.md verbatim; --path prints only the absolute path\n" +
			"(useful as `cd $(kungfu show foo --path)`).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			app, ok := AppFromContext(cmd.Context())
			if !ok {
				return &ExitError{Code: 2, Err: errors.New("show: missing application context")}
			}

			targets, err := resolveListTargets(app)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			scopes, err := listScopes(app.ScopeFlag)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}

			cwd, _ := os.Getwd()
			var locations []targetpkg.Location
			for _, scope := range scopes {
				locs, err := targetpkg.Locations(targets, scope, cwd, func(targetpkg.Target, string) {})
				if err != nil {
					return &ExitError{Code: 2, Err: err}
				}
				locations = append(locations, locs...)
			}

			matches, err := skill.FindByName(name, locations)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}
			if len(matches) == 0 {
				return &ExitError{Code: 1, Err: fmt.Errorf("show: skill %q not found", name)}
			}
			if len(matches) > 1 && app.TargetFlag == "" {
				var b strings.Builder
				fmt.Fprintf(&b, "show: %q exists in multiple targets; disambiguate with --target:\n", name)
				for _, m := range matches {
					fmt.Fprintf(&b, "  %s (%s) at %s\n", m.Target, m.Scope, m.Location)
				}
				return &ExitError{Code: 1, Err: errors.New(strings.TrimRight(b.String(), "\n"))}
			}

			m := matches[0]
			out := cmd.OutOrStdout()
			switch {
			case pathOnly:
				fmt.Fprintln(out, m.Location)
			case raw:
				data, err := os.ReadFile(filepath.Join(m.Location, skill.FileName))
				if err != nil {
					return &ExitError{Code: 2, Err: fmt.Errorf("show: %w", err)}
				}
				_, _ = out.Write(data)
			default:
				renderShow(out, m)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&raw, "raw", false, "print SKILL.md verbatim")
	cmd.Flags().BoolVar(&pathOnly, "path", false, "print only the absolute path of the skill")
	return cmd
}

func renderShow(w io.Writer, m skill.Match) {
	s := m.Skill
	fmt.Fprintln(w, ui.Bold.Render(s.Name))
	fmt.Fprintln(w, ui.Muted.Render(s.Description))
	if len(s.AllowedTools) > 0 {
		fmt.Fprintln(w, ui.Muted.Render("allowed-tools: "+strings.Join(s.AllowedTools, ", ")))
	}
	fmt.Fprintln(w, ui.Muted.Render("target:        "+m.Target))
	fmt.Fprintln(w, ui.Muted.Render("scope:         "+m.Scope))
	fmt.Fprintln(w, ui.Muted.Render("location:      "+m.Location))
	fmt.Fprintln(w)

	body := strings.TrimRight(s.Body, "\n") + "\n"
	if isTerminalWriter(w) && !ui.NoColor() {
		if rendered, err := glamour.Render(body, "auto"); err == nil {
			fmt.Fprint(w, rendered)
			return
		}
	}
	fmt.Fprint(w, body)
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
