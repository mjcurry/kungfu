package cli

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newRemoveCmd builds the `kungfu remove` command.
//
// Exit codes:
//
//	0 — removed
//	1 — skill not found in any matched target
//	2 — user declined the confirmation prompt
//	3 — partial or total I/O failure during removal
func newRemoveCmd() *cobra.Command {
	var (
		yes    bool
		dryRun bool
	)
	cmd := &cobra.Command{
		Use:     "remove <name>",
		Aliases: []string{"rm"},
		Short:   "Remove a skill from one or more targets",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			app, ok := AppFromContext(cmd.Context())
			if !ok {
				return &ExitError{Code: 3, Err: errors.New("remove: missing application context")}
			}

			targets, err := resolveListTargets(app) // empty target = all configured
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}
			scopes, err := listScopes(app.ScopeFlag) // empty scope = both
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}

			cwd, _ := os.Getwd()
			var locations []targetpkg.Location
			for _, scope := range scopes {
				locs, err := targetpkg.Locations(targets, scope, cwd, func(targetpkg.Target, string) {})
				if err != nil {
					return &ExitError{Code: 3, Err: err}
				}
				locations = append(locations, locs...)
			}

			matches, err := skill.FindByName(name, locations)
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}
			if len(matches) == 0 {
				return &ExitError{Code: 1, Err: fmt.Errorf("remove: skill %q not found in any configured target", name)}
			}

			out := cmd.OutOrStdout()

			if dryRun {
				for _, m := range matches {
					fmt.Fprintf(out, "would remove: %s (%s, %s) at %s\n",
						m.Skill.Name, m.Target, m.Scope, m.Location)
				}
				return nil
			}

			if !yes {
				fmt.Fprintf(out, "remove %q from %d location%s:\n", name, len(matches), plural(len(matches)))
				for _, m := range matches {
					fmt.Fprintf(out, "  %s (%s) at %s\n", m.Target, m.Scope, m.Location)
				}
				fmt.Fprint(out, "proceed? [y/N] ")
				reader := bufio.NewReader(cmd.InOrStdin())
				answer, _ := reader.ReadString('\n')
				answer = strings.ToLower(strings.TrimSpace(answer))
				if answer != "y" && answer != "yes" {
					return &ExitError{Code: 2, Err: errors.New("remove: aborted")}
				}
			}

			removed := make([]skill.Match, 0, len(matches))
			failures := make([]error, 0)
			for _, m := range matches {
				if err := os.RemoveAll(m.Location); err != nil {
					failures = append(failures, fmt.Errorf("%s (%s): %w", m.Target, m.Scope, err))
					fmt.Fprintf(cmd.ErrOrStderr(), "%s remove failed for %s (%s): %v\n",
						ui.Error.Render("✗"), m.Target, m.Scope, err)
					continue
				}
				removed = append(removed, m)
			}

			locStrs := make([]string, 0, len(removed))
			for _, m := range removed {
				locStrs = append(locStrs, fmt.Sprintf("%s (%s)", m.Target, m.Scope))
			}
			summary := fmt.Sprintf("removed: %s from %s", name, strings.Join(locStrs, ", "))
			if len(failures) > 0 {
				summary += fmt.Sprintf(" (%d failed)", len(failures))
				fmt.Fprintln(out, ui.Warning.Render(summary))
				return &ExitError{Code: 3, Err: combineErrors(failures)}
			}
			fmt.Fprintln(out, ui.Success.Render(summary))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation prompt")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be removed without removing")
	return cmd
}
