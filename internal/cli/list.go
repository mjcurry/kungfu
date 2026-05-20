package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// listItem is one row in `kungfu list` output, also the JSON shape.
type listItem struct {
	Name         string   `json:"name"`
	Target       string   `json:"target"`
	Scope        string   `json:"scope"`
	Path         string   `json:"path"`
	Description  string   `json:"description"`
	AllowedTools []string `json:"allowed_tools"`
	HasErrors    bool     `json:"has_errors"`
}

func newListCmd() *cobra.Command {
	var (
		long   bool
		asJSON bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List installed skills across configured targets",
		Long: "List skills found in the configured targets.\n\n" +
			"By default kungfu scans every configured target at both personal and\n" +
			"project scope. --target narrows to specific targets; --scope (root\n" +
			"persistent) narrows to one of \"personal\", \"project\", or \"both\".",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, ok := AppFromContext(cmd.Context())
			if !ok {
				return &ExitError{Code: 2, Err: errors.New("list: missing application context")}
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
			items, err := gatherListItems(targets, scopes, cwd)
			if err != nil {
				return &ExitError{Code: 2, Err: err}
			}

			out := cmd.OutOrStdout()
			errOut := cmd.ErrOrStderr()

			if asJSON {
				if items == nil {
					items = []listItem{}
				}
				if err := renderJSON(out, items); err != nil {
					return &ExitError{Code: 2, Err: err}
				}
				return nil
			}

			if len(items) == 0 {
				fmt.Fprintln(errOut, "no skills installed")
				return nil
			}
			renderListHuman(out, items, long)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&long, "long", "l", false, "include description column")
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}

// resolveListTargets honours the list-specific rule "empty --target means
// every configured target", not the install/remove rule of using
// default_targets.
func resolveListTargets(app *App) ([]targetpkg.Target, error) {
	if app.TargetFlag == "" {
		out := make([]targetpkg.Target, len(app.Config.Targets))
		copy(out, app.Config.Targets)
		return out, nil
	}
	return targetpkg.Resolve(app.TargetFlag, app.Config.Targets)
}

// listScopes interprets the --scope flag for the list command, where the
// empty value means "both" (a meaning specific to list).
func listScopes(flag string) ([]targetpkg.Scope, error) {
	eff := strings.TrimSpace(flag)
	if eff == "" || eff == "both" {
		return []targetpkg.Scope{targetpkg.ScopePersonal, targetpkg.ScopeProject}, nil
	}
	s := targetpkg.Scope(eff)
	if !s.IsValid() {
		return nil, fmt.Errorf("list: invalid scope %q (want \"personal\", \"project\", or \"both\")", eff)
	}
	return []targetpkg.Scope{s}, nil
}

// gatherListItems discovers skills across every (target, scope, dir) tuple
// and runs a lint pass on each to compute HasErrors.
func gatherListItems(targets []targetpkg.Target, scopes []targetpkg.Scope, projectRoot string) ([]listItem, error) {
	linter := lint.NewDefault()
	var items []listItem
	for _, scope := range scopes {
		locs, err := targetpkg.Locations(targets, scope, projectRoot, func(targetpkg.Target, string) {
			// Unsupported (target, scope) combos are silently skipped in list.
		})
		if err != nil {
			return nil, err
		}
		for _, loc := range locs {
			if _, err := os.Stat(loc.Dir); err != nil {
				continue
			}
			skills, err := skill.Discover(loc.Dir)
			if err != nil {
				return nil, err
			}
			for _, s := range skills {
				hasErrors := false
				if rep, lintErr := linter.Lint(s.Dir); lintErr == nil {
					hasErrors = rep.HasErrors()
				}
				items = append(items, listItem{
					Name:         s.Name,
					Target:       loc.Target.Name,
					Scope:        string(loc.Scope),
					Path:         s.Dir,
					Description:  s.Description,
					AllowedTools: s.AllowedTools,
					HasErrors:    hasErrors,
				})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		if items[i].Target != items[j].Target {
			return items[i].Target < items[j].Target
		}
		return items[i].Scope < items[j].Scope
	})
	return items, nil
}

func renderListHuman(w io.Writer, items []listItem, long bool) {
	width := writerWidth(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	if long {
		fmt.Fprintln(tw, "   NAME\tTARGET\tSCOPE\tLOCATION\tDESCRIPTION")
	} else {
		fmt.Fprintln(tw, "   NAME\tTARGET\tSCOPE\tLOCATION")
	}
	for _, it := range items {
		marker := "   "
		if it.HasErrors {
			if ui.NoColor() {
				marker = "[!]"
			} else {
				marker = ui.Warning.Render("⚠") + "  "
			}
		}
		loc := abbreviateHome(filepath.Dir(it.Path))
		targetCell := ui.Bold.Render(it.Target)
		if long {
			fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\n",
				marker, it.Name, targetCell, it.Scope, loc,
				truncate(it.Description, descBudget(width)))
		} else {
			fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\n",
				marker, it.Name, targetCell, it.Scope, loc)
		}
	}
	_ = tw.Flush()
}

func descBudget(termWidth int) int {
	const overhead = 60 // marker + name + target + scope + location columns
	budget := termWidth - overhead
	if budget < 20 {
		return 20
	}
	return budget
}

func truncate(s string, max int) string {
	if max <= 1 || len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func writerWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 80
	}
	if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}

func abbreviateHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(home, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	if rel == "." {
		return "~"
	}
	return filepath.Join("~", rel)
}
