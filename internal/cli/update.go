package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
	"github.com/mjcurry/kungfu/internal/update"
)

// newUpdateCmd builds the `kungfu update` command.
//
// Exit codes:
//
//	0 — every requested skill is up to date or was updated successfully
//	1 — invalid invocation (no name + no --all, or named skill has no provenance)
//	2 — user declined the confirmation prompt
//	3 — at least one update failed (network, lint, or copy)
func newUpdateCmd() *cobra.Command {
	var (
		all     bool
		dryRun  bool
		yes     bool
		refFlag string
	)
	cmd := &cobra.Command{
		Use:   "update [<name>]",
		Short: "Re-fetch and re-install previously-installed skills",
		Long: "Update one or more installed skills by re-fetching their stored\n" +
			"provenance (kungfu_source + kungfu_ref) and replacing the local\n" +
			"copy. Only skills installed from a remote source are updatable;\n" +
			"locally-installed or scaffolded skills are skipped.\n\n" +
			"Pass a name to update a single skill, --all to update every\n" +
			"installed skill with provenance, or --ref to move a skill to a\n" +
			"different ref (e.g. bump a pinned tag).",
		Example: "  # bring everything tracking a moving ref (like \"main\") up to date\n" +
			"  kungfu update --all\n\n" +
			"  # update one skill\n" +
			"  kungfu update csv-formatter\n\n" +
			"  # bump a pinned skill to a newer tag\n" +
			"  kungfu update csv-formatter --ref v2.0.0",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var nameFilter string
			if len(args) == 1 {
				nameFilter = args[0]
			}
			if nameFilter == "" && !all {
				return &ExitError{Code: 1, Err: errors.New(
					"update: pass a skill name or --all to update every updatable skill")}
			}
			return runUpdate(cmd, nameFilter, refFlag, dryRun, yes)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "update every updatable skill across the configured targets")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would be updated without changing anything")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	cmd.Flags().StringVar(&refFlag, "ref", "", "ref to fetch instead of each skill's stored kungfu_ref")
	return cmd
}

// runUpdate is the body of `kungfu update`; the cobra wrapper above just
// translates flags into arguments.
func runUpdate(cmd *cobra.Command, nameFilter, refFlag string, dryRun, yes bool) error {
	app, ok := AppFromContext(cmd.Context())
	if !ok {
		return &ExitError{Code: 3, Err: errors.New("update: missing application context")}
	}
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()

	targets, err := resolveListTargets(app)
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}
	scopes, err := listScopes(app.ScopeFlag)
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

	updatables, err := update.DiscoverUpdatable(locations, func(dir string, err error) {
		fmt.Fprintln(errOut, ui.Warning.Render("warning:")+" "+dir+": "+err.Error())
	})
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}

	if nameFilter != "" {
		filtered := updatables[:0]
		for _, u := range updatables {
			if u.Skill.Name == nameFilter {
				filtered = append(filtered, u)
			}
		}
		if len(filtered) == 0 {
			// Distinguish "not found at all" from "found but no provenance".
			if anySkillNamed(locations, nameFilter) {
				return &ExitError{Code: 1, Err: fmt.Errorf(
					"update: %q was not installed from a remote source; nothing to update", nameFilter)}
			}
			return &ExitError{Code: 1, Err: fmt.Errorf("update: skill %q not found", nameFilter)}
		}
		updatables = filtered
	}

	if len(updatables) == 0 {
		fmt.Fprintln(out, ui.Muted.Render("no updatable skills found"))
		return nil
	}

	plan, err := buildUpdatePlan(cmd.Context(), updatables, refFlag, errOut, fetch.NewClient())
	if err != nil {
		return &ExitError{Code: 3, Err: err}
	}
	renderUpdatePlan(out, plan)

	if !plan.hasWork() {
		fmt.Fprintln(out, ui.Success.Render("everything up to date"))
		return nil
	}
	if dryRun {
		return nil
	}
	if !yes {
		fmt.Fprintf(out, "\nupdate %d skill%s? [Y/n] ", plan.changeCount(), plural(plan.changeCount()))
		ok, err := readPromptYes(cmd.InOrStdin(), true)
		if err != nil {
			return &ExitError{Code: 3, Err: err}
		}
		if !ok {
			fmt.Fprintln(out, ui.Muted.Render("aborted"))
			return &ExitError{Code: 2}
		}
	}
	return executeUpdatePlan(cmd, plan)
}

// anySkillNamed reports whether the given name has been installed anywhere
// in locations (with or without provenance). Used to produce a different
// error message when the user names a skill that exists but is local-only.
func anySkillNamed(locations []targetpkg.Location, name string) bool {
	for _, loc := range locations {
		if _, err := os.Stat(filepath.Join(loc.Dir, name, skill.FileName)); err == nil {
			return true
		}
	}
	return false
}

// updateRow is one line in the user-facing plan table.
type updateRow struct {
	upd        update.Updatable
	plannedRef string // ref to fetch (override or stored)
	plannedSHA string // resolved SHA
	upToDate   bool
	err        error // non-nil if ref resolution failed; row still shown
}

type updatePlan struct {
	rows []updateRow
}

func (p *updatePlan) hasWork() bool {
	for _, r := range p.rows {
		if !r.upToDate && r.err == nil {
			return true
		}
	}
	return false
}

func (p *updatePlan) changeCount() int {
	n := 0
	for _, r := range p.rows {
		if !r.upToDate && r.err == nil {
			n++
		}
	}
	return n
}

// buildUpdatePlan resolves each Updatable's current SHA. ResolveRef is
// cached per (source, ref) tuple so the same skill installed across
// multiple targets only hits the network once.
func buildUpdatePlan(ctx context.Context, ups []update.Updatable, refFlag string, errOut io.Writer, client *fetch.Client) (*updatePlan, error) {
	type cacheKey struct{ source, ref string }
	cache := map[cacheKey]string{}

	plan := &updatePlan{}
	for _, u := range ups {
		ref := refFlag
		if ref == "" {
			ref = u.StoredRef
		}
		src := *u.Source
		src.Ref = ref

		key := cacheKey{source: src.Owner + "/" + src.Repo, ref: ref}
		sha, ok := cache[key]
		if !ok {
			resolved, _, err := client.ResolveRef(ctx, &src)
			if err != nil {
				fmt.Fprintln(errOut, ui.Warning.Render("warning:")+" "+u.Source.String()+
					"@"+ref+": "+err.Error())
				plan.rows = append(plan.rows, updateRow{
					upd: u, plannedRef: ref, err: err,
				})
				continue
			}
			cache[key] = resolved
			sha = resolved
		}
		plan.rows = append(plan.rows, updateRow{
			upd:        u,
			plannedRef: ref,
			plannedSHA: sha,
			upToDate:   sha == u.StoredSHA,
		})
	}

	// Stable order: by name, then target, then scope.
	sort.SliceStable(plan.rows, func(i, j int) bool {
		a, b := plan.rows[i], plan.rows[j]
		if a.upd.Skill.Name != b.upd.Skill.Name {
			return a.upd.Skill.Name < b.upd.Skill.Name
		}
		if a.upd.Location.Target.Name != b.upd.Location.Target.Name {
			return a.upd.Location.Target.Name < b.upd.Location.Target.Name
		}
		return string(a.upd.Location.Scope) < string(b.upd.Location.Scope)
	})
	return plan, nil
}

func renderUpdatePlan(w io.Writer, plan *updatePlan) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  NAME\tTARGET\tSCOPE\tCURRENT\tPLAN")
	for _, r := range plan.rows {
		name := r.upd.Skill.Name
		target := r.upd.Location.Target.Name
		scope := string(r.upd.Location.Scope)
		current := r.upd.StoredRef + " " + shortSHA(r.upd.StoredSHA)
		var planText string
		switch {
		case r.err != nil:
			planText = ui.Error.Render("resolve failed: " + r.err.Error())
		case r.upToDate:
			planText = ui.Muted.Render("up to date")
		default:
			planText = r.plannedRef + " " + shortSHA(r.plannedSHA)
		}
		marker := "  "
		if r.err == nil && !r.upToDate {
			marker = ui.Success.Render("↑ ")
		}
		fmt.Fprintf(tw, "%s%s\t%s\t%s\t%s\t%s\n", marker, name, target, scope, current, planText)
	}
	_ = tw.Flush()
}

// executeUpdatePlan re-installs each row that needs work. Fetch + extract
// happen once per (source, sha) pair via skill.Install's atomic copy.
func executeUpdatePlan(cmd *cobra.Command, plan *updatePlan) error {
	out := cmd.OutOrStdout()
	errOut := cmd.ErrOrStderr()
	client := fetch.NewClient()
	ctx := cmd.Context()

	type fetched struct{ tarPath string }
	tarballs := map[string]*fetched{} // key: source + ":" + sha

	var succeeded []updateRow
	var failures []error

	for _, r := range plan.rows {
		if r.upToDate || r.err != nil {
			continue
		}
		key := r.upd.Source.Owner + "/" + r.upd.Source.Repo + ":" + r.plannedSHA
		f, ok := tarballs[key]
		if !ok {
			path, err := client.FetchTarball(ctx, r.upd.Source, r.plannedSHA)
			if err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", r.upd.Skill.Name, err))
				fmt.Fprintf(errOut, "%s update failed for %s (%s, %s): %v\n",
					ui.Error.Render("✗"), r.upd.Skill.Name,
					r.upd.Location.Target.Name, r.upd.Location.Scope, err)
				continue
			}
			f = &fetched{tarPath: path}
			tarballs[key] = f
		}

		scratch, err := os.MkdirTemp("", "kungfu-update-*")
		if err != nil {
			failures = append(failures, err)
			continue
		}
		extractTo := filepath.Join(scratch, "raw")
		if err := fetch.Extract(f.tarPath, extractTo, r.upd.Source.Subpath); err != nil {
			_ = os.RemoveAll(scratch)
			failures = append(failures, fmt.Errorf("%s: %w", r.upd.Skill.Name, err))
			continue
		}
		finalDir := filepath.Join(scratch, r.upd.Skill.Name)
		if err := os.Rename(extractTo, finalDir); err != nil {
			_ = os.RemoveAll(scratch)
			failures = append(failures, fmt.Errorf("%s: %w", r.upd.Skill.Name, err))
			continue
		}
		s, err := skill.Load(finalDir)
		if err != nil {
			_ = os.RemoveAll(scratch)
			failures = append(failures, fmt.Errorf("%s: %w", r.upd.Skill.Name, err))
			continue
		}
		// Stamp updated provenance.
		s.Source = r.upd.Skill.Source // unchanged
		s.Ref = r.plannedRef          // may be the override
		s.SHA = r.plannedSHA
		s.InstalledAt = time.Now().UTC().Format(time.RFC3339)
		if err := s.Save(); err != nil {
			_ = os.RemoveAll(scratch)
			failures = append(failures, fmt.Errorf("%s: stamp provenance: %w", r.upd.Skill.Name, err))
			continue
		}
		// Lint the fetched copy before clobbering the installed one.
		rep, err := lint.NewDefault().Lint(finalDir)
		if err == nil && rep.HasErrors() {
			_ = os.RemoveAll(scratch)
			renderLintHuman(errOut, rep)
			failures = append(failures,
				fmt.Errorf("%s: lint errors in fetched copy", r.upd.Skill.Name))
			continue
		}
		dst := filepath.Join(r.upd.Location.Dir, s.Name)
		if err := skill.Install(finalDir, dst, true); err != nil {
			_ = os.RemoveAll(scratch)
			failures = append(failures, fmt.Errorf("%s (%s, %s): %w",
				r.upd.Skill.Name, r.upd.Location.Target.Name, r.upd.Location.Scope, err))
			fmt.Fprintf(errOut, "%s update failed for %s (%s, %s): %v\n",
				ui.Error.Render("✗"), r.upd.Skill.Name,
				r.upd.Location.Target.Name, r.upd.Location.Scope, err)
			_ = os.RemoveAll(scratch)
			continue
		}
		_ = os.RemoveAll(scratch)
		succeeded = append(succeeded, r)
		fmt.Fprintf(out, "%s updated: %s → %s (%s) at %s (%s → %s)\n",
			ui.Success.Render("✓"), s.Name,
			ui.Bold.Render(r.upd.Location.Target.Name), r.upd.Location.Scope, dst,
			shortSHA(r.upd.StoredSHA), shortSHA(r.plannedSHA))
	}

	summary := fmt.Sprintf("updated %d skill%s", len(succeeded), plural(len(succeeded)))
	if len(failures) > 0 {
		summary += fmt.Sprintf(", %d failed", len(failures))
		fmt.Fprintln(out, ui.Warning.Render(summary))
		return &ExitError{Code: 3, Err: combineErrors(failures)}
	}
	fmt.Fprintln(out, ui.Success.Render(summary))
	return nil
}

// readPromptYes reads one line from r and returns true unless the user
// typed n/no (case-insensitive). The default determines the empty-input
// answer.
func readPromptYes(r io.Reader, def bool) (bool, error) {
	buf := make([]byte, 64)
	n, _ := r.Read(buf)
	line := strings.TrimSpace(string(buf[:n]))
	if line == "" {
		return def, nil
	}
	switch strings.ToLower(line) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return def, nil
}
