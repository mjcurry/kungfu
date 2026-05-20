package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newInstallCmd builds the `kungfu install` command.
//
// Exit codes:
//
//	0 — all targets installed
//	1 — pre-install lint errors, or every target was skipped as unsupported
//	2 — destinations already exist and --force was not given
//	3 — partial or total I/O failure during copy
func newInstallCmd() *cobra.Command {
	var (
		force  bool
		dryRun bool
		noLint bool
	)
	cmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Install a skill to one or more targets",
		Long: "Install a skill from a local source directory into each configured\n" +
			"target. <source> must contain a SKILL.md.\n\n" +
			"Use --target to pick targets (\"claude,codex\" or \"all\"); empty uses\n" +
			"default_targets from the config. Use --scope to choose personal or\n" +
			"project; empty uses default_scope.\n\n" +
			"Archives and remote URLs are out of scope for this version; clone\n" +
			"locally and point install at the directory.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			src, err := filepath.Abs(args[0])
			if err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("install: resolving %s: %w", args[0], err)}
			}
			info, err := os.Stat(src)
			if err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("install: %w", err)}
			}
			if !info.IsDir() {
				return &ExitError{Code: 3, Err: fmt.Errorf("install: source %s is not a directory", src)}
			}
			if _, err := os.Stat(filepath.Join(src, skill.FileName)); err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("install: source has no %s", skill.FileName)}
			}

			s, err := skill.Load(src)
			if err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("install: %w", err)}
			}

			if !noLint {
				rep, lerr := lint.NewDefault().Lint(src)
				if lerr != nil {
					return &ExitError{Code: 3, Err: fmt.Errorf("install: lint: %w", lerr)}
				}
				if rep.HasErrors() {
					renderLintHuman(cmd.ErrOrStderr(), rep)
					return &ExitError{Code: 1}
				}
				if len(rep.Warnings()) > 0 {
					renderLintHuman(cmd.OutOrStdout(), rep)
				}
			}

			app, ok := AppFromContext(cmd.Context())
			if !ok {
				return &ExitError{Code: 3, Err: errors.New("install: missing application context")}
			}

			targets, err := app.Config.ResolveTargets(app.TargetFlag)
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}
			scope, err := app.Config.ResolveScope(app.ScopeFlag)
			if err != nil {
				return &ExitError{Code: 3, Err: err}
			}
			cwd, _ := os.Getwd()

			out := cmd.OutOrStdout()
			locs, _ := targetpkg.Locations(targets, scope, cwd, func(t targetpkg.Target, reason string) {
				fmt.Fprintf(out, "%s skipped: %s (%s)\n",
					ui.Warning.Render("⚠"), t.Name, reason)
			})
			if len(locs) == 0 {
				return &ExitError{Code: 1, Err: errors.New("install: every target was unsupported for the requested scope")}
			}

			// Collision check: compute every destination, fail fast if any exists
			// without --force.
			type plan struct {
				loc  targetpkg.Location
				dst  string
				busy bool
			}
			plans := make([]plan, 0, len(locs))
			conflicts := make([]string, 0)
			for _, loc := range locs {
				p := plan{loc: loc, dst: filepath.Join(loc.Dir, s.Name)}
				if _, err := os.Stat(p.dst); err == nil {
					p.busy = true
					conflicts = append(conflicts, fmt.Sprintf("  %s (%s): %s", loc.Target.Name, loc.Scope, p.dst))
				}
				plans = append(plans, p)
			}
			if len(conflicts) > 0 && !force {
				msg := "install: destination already exists:\n" + strings.Join(conflicts, "\n") +
					"\npass --force to overwrite"
				return &ExitError{Code: 2, Err: errors.New(msg)}
			}

			if dryRun {
				for _, p := range plans {
					fmt.Fprintf(out, "would install: %s → %s (%s) at %s\n",
						src, p.loc.Target.Name, p.loc.Scope, p.dst)
				}
				return nil
			}

			succeeded := make([]plan, 0, len(plans))
			failures := make([]error, 0)
			for _, p := range plans {
				if err := skill.Install(src, p.dst, force); err != nil {
					failures = append(failures, fmt.Errorf("%s (%s): %w", p.loc.Target.Name, p.loc.Scope, err))
					fmt.Fprintf(cmd.ErrOrStderr(), "%s install failed for %s (%s): %v\n",
						ui.Error.Render("✗"), p.loc.Target.Name, p.loc.Scope, err)
					continue
				}
				succeeded = append(succeeded, p)
				fmt.Fprintf(out, "%s installed: %s → %s (%s) at %s\n",
					ui.Success.Render("✓"), s.Name,
					ui.Bold.Render(p.loc.Target.Name), p.loc.Scope, p.dst)
			}

			summary := fmt.Sprintf("installed to %d target%s",
				len(succeeded), plural(len(succeeded)))
			if len(failures) > 0 {
				summary += fmt.Sprintf(", %d failed", len(failures))
				fmt.Fprintln(out, ui.Warning.Render(summary))
				return &ExitError{Code: 3, Err: combineErrors(failures)}
			}
			fmt.Fprintln(out, ui.Success.Render(summary))
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing installations")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned actions without changing anything")
	cmd.Flags().BoolVar(&noLint, "no-lint", false, "skip the pre-install lint")
	return cmd
}

// renderLint helpers, plural, combineErrors are shared with other commands.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func combineErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	if len(errs) == 1 {
		return errs[0]
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return errors.New(strings.Join(msgs, "; "))
}
