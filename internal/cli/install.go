package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newInstallCmd builds the `kungfu install` command.
//
// Source forms accepted: a local path, or a GitHub source — user/repo,
// user/repo@ref, user/repo/subpath[@ref], github.com/..., https://github.com/...
//
// Exit codes:
//
//	0 — all targets installed
//	1 — pre-install lint errors, or every target was skipped as unsupported
//	2 — destinations already exist and --force was not given
//	3 — partial or total I/O failure during copy
//	5 — network or tarball failure (remote sources only)
//	6 — unrecognised source string (neither a local path nor a GitHub source)
//	7 — extracted source has no SKILL.md
func newInstallCmd() *cobra.Command {
	var (
		force   bool
		dryRun  bool
		noLint  bool
		yes     bool
		refFlag string
		noCache bool
	)
	cmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Install a skill from a local path or GitHub",
		Long: "Install a skill into each configured target.\n\n" +
			"<source> may be a local directory containing a SKILL.md, or a\n" +
			"GitHub reference like user/repo, user/repo@v1.0.0, or\n" +
			"user/repo/subpath@ref. Use --ref to set the ref via flag instead\n" +
			"of the @-suffix.\n\n" +
			"Archives and non-GitHub hosts are out of scope for this version.\n" +
			"Locally-cloned copies of any git host work via the local path form.",
		Example: "  # local\n" +
			"  kungfu install ./my-skill\n\n" +
			"  # GitHub\n" +
			"  kungfu install user/repo\n" +
			"  kungfu install user/repo@v1.0.0\n" +
			"  kungfu install user/repo/path/to/skill@main\n\n" +
			"  # multi-target\n" +
			"  kungfu install user/repo --target claude,codex",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := args[0]

			// Local source: if the input resolves to an existing path on
			// disk, install from it directly. This matches the PR 3 flow
			// exactly.
			if _, err := os.Stat(arg); err == nil {
				abs, err := filepath.Abs(arg)
				if err != nil {
					return &ExitError{Code: 3, Err: fmt.Errorf("install: resolving %s: %w", arg, err)}
				}
				return runLocalInstall(cmd, abs, force, dryRun, noLint)
			}

			// Remote source.
			src, err := fetch.Parse(arg)
			if errors.Is(err, fetch.ErrNotRemote) {
				return &ExitError{Code: 6, Err: fmt.Errorf(
					"install: unrecognised source %q; expected a local path or a GitHub source like user/repo[@ref][/subpath]", arg)}
			}
			if err != nil {
				return &ExitError{Code: 6, Err: fmt.Errorf("install: %w", err)}
			}
			if refFlag != "" {
				src.Ref = refFlag
			}
			return runRemoteInstall(cmd, src, force, dryRun, noLint, yes, noCache)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing installations")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print planned actions without changing anything")
	cmd.Flags().BoolVar(&noLint, "no-lint", false, "skip the pre-install lint")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the pre-install confirmation prompt (remote installs)")
	cmd.Flags().StringVar(&refFlag, "ref", "", "GitHub ref override (tag, branch, or full SHA)")
	cmd.Flags().BoolVar(&noCache, "no-cache", false, "skip the tarball cache and refetch")
	return cmd
}

// runLocalInstall is the PR 3 install flow: source is a local directory
// already on disk. No fetch, no provenance.
func runLocalInstall(cmd *cobra.Command, src string, force, dryRun, noLint bool) error {
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
	if err := runLintBeforeInstall(cmd, src, noLint); err != nil {
		return err
	}
	return runInstallPlan(cmd, s, src, force, dryRun, false /* confirm */, "" /* sourceLabel */)
}

// runRemoteInstall handles a GitHub source: resolve → fetch → extract →
// stamp provenance → lint → plan → confirm → install.
func runRemoteInstall(cmd *cobra.Command, src *fetch.Source, force, dryRun, noLint, yes, noCache bool) error {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, ui.Muted.Render("fetching "+src.String()+" ..."))

	client := fetch.NewClient()
	if noCache {
		client.Cache = nil
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = cmd.Context()
	}

	sha, _, err := client.ResolveRef(ctx, src)
	if err != nil {
		return &ExitError{Code: 5, Err: fmt.Errorf("install: %w", err)}
	}
	refDisplay := src.Ref
	if refDisplay == "" {
		refDisplay = "default branch"
	}
	fmt.Fprintln(out, ui.Muted.Render(fmt.Sprintf("resolved %s → %s", refDisplay, shortSHA(sha))))

	tarPath, err := client.FetchTarball(ctx, src, sha)
	if err != nil {
		return &ExitError{Code: 5, Err: fmt.Errorf("install: %w", err)}
	}
	if info, err := os.Stat(tarPath); err == nil {
		fmt.Fprintln(out, ui.Muted.Render("downloaded tarball ("+humanBytes(info.Size())+", cached)"))
	}

	scratchParent, err := os.MkdirTemp("", "kungfu-extract-*")
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("install: scratch dir: %w", err)}
	}
	defer os.RemoveAll(scratchParent)

	// Extract into a placeholder dir first; once we know the skill name we
	// rename it so lint's frontmatter/name-mismatch rule sees a matching
	// basename. Without this the scratch tempdir's name would never match
	// the frontmatter's name.
	placeholder := filepath.Join(scratchParent, "raw")
	if err := fetch.Extract(tarPath, placeholder, src.Subpath); err != nil {
		return &ExitError{Code: 7, Err: fmt.Errorf("install: %w", err)}
	}

	// Some repos are skill *collections* — one repo with multiple
	// SKILL.md files nested in subdirectories rather than at the root.
	// discoverSkillRoot handles both shapes: a single root SKILL.md, a
	// single nested SKILL.md (auto-pick), or multiple (error with a list
	// telling the user how to pick).
	skillRoot, discoveredSubpath, err := discoverSkillRoot(placeholder, src.Subpath)
	if err != nil {
		return &ExitError{Code: 7, Err: fmt.Errorf("install: %w", err)}
	}
	if discoveredSubpath != "" {
		fmt.Fprintln(out, ui.Muted.Render("discovered skill: "+discoveredSubpath))
	}

	// Compose the effective subpath for provenance: any user-supplied
	// subpath plus whatever discoverSkillRoot resolved beneath it.
	effectiveSubpath := joinSubpath(src.Subpath, discoveredSubpath)

	s, err := skill.Load(skillRoot)
	if err != nil {
		return &ExitError{Code: 7, Err: fmt.Errorf("install: %w", err)}
	}
	scratch := filepath.Join(scratchParent, s.Name)
	if err := os.Rename(skillRoot, scratch); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("install: renaming scratch dir: %w", err)}
	}
	s, err = skill.Load(scratch)
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("install: reload after rename: %w", err)}
	}
	fmt.Fprintln(out, ui.Muted.Render("extracted skill: "+s.Name))

	// Stamp provenance and persist. effectiveSubpath includes any auto-
	// discovered nested path so a later `kungfu update` re-fetches the
	// same nested skill rather than the repo root.
	provSource := fmt.Sprintf("%s/%s/%s", src.Host, src.Owner, src.Repo)
	if effectiveSubpath != "" {
		provSource += "/" + effectiveSubpath
	}
	s.Source = provSource
	s.Ref = src.Ref
	s.SHA = sha
	s.InstalledAt = time.Now().UTC().Format(time.RFC3339)
	if err := s.Save(); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("install: stamping provenance: %w", err)}
	}

	if err := runLintBeforeInstall(cmd, scratch, noLint); err != nil {
		return err
	}

	sourceLabel := fmt.Sprintf("%s@%s from %s", s.Name, shortSHA(sha), provSource)
	confirm := !yes
	return runInstallPlan(cmd, s, scratch, force, dryRun, confirm, sourceLabel)
}

// runLintBeforeInstall runs the standard rule set against srcDir unless
// noLint is set. Errors block the install (exit 1); warnings print but
// proceed.
func runLintBeforeInstall(cmd *cobra.Command, srcDir string, noLint bool) error {
	if noLint {
		return nil
	}
	rep, err := lint.NewDefault().Lint(srcDir)
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("install: lint: %w", err)}
	}
	if rep.HasErrors() {
		renderLintHuman(cmd.ErrOrStderr(), rep)
		return &ExitError{Code: 1}
	}
	if len(rep.Warnings()) > 0 {
		renderLintHuman(cmd.OutOrStdout(), rep)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), ui.Success.Render("lint: 0 errors, 0 warnings"))
	}
	return nil
}

// installPlan is one (target, scope, destination) tuple computed before the
// copy phase. The runInstallPlan helper collects them so it can show
// conflicts atomically, then either dry-run, prompt, or install.
type installPlan struct {
	loc  targetpkg.Location
	dst  string
	busy bool
}

// runInstallPlan handles everything from target/scope resolution through
// the final copy. It is shared by both the local and remote install flows;
// the difference is whether sourceLabel is non-empty and confirm is set.
func runInstallPlan(cmd *cobra.Command, s *skill.Skill, srcDir string, force, dryRun, confirm bool, sourceLabel string) error {
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
		fmt.Fprintf(out, "%s skipped: %s (%s)\n", ui.Warning.Render("⚠"), t.Name, reason)
	})
	if len(locs) == 0 {
		return &ExitError{Code: 1, Err: errors.New("install: every target was unsupported for the requested scope")}
	}

	plans := make([]installPlan, 0, len(locs))
	conflicts := make([]string, 0)
	for _, loc := range locs {
		p := installPlan{loc: loc, dst: filepath.Join(loc.Dir, s.Name)}
		if _, err := os.Stat(p.dst); err == nil {
			p.busy = true
			conflicts = append(conflicts,
				fmt.Sprintf("  %s (%s): %s", loc.Target.Name, loc.Scope, p.dst))
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
				s.Name, p.loc.Target.Name, p.loc.Scope, p.dst)
		}
		return nil
	}

	if confirm {
		targetList := make([]string, 0, len(plans))
		for _, p := range plans {
			targetList = append(targetList, fmt.Sprintf("%s (%s)", p.loc.Target.Name, p.loc.Scope))
		}
		prompt := "install"
		if sourceLabel != "" {
			prompt += " " + sourceLabel
		} else {
			prompt += " " + s.Name
		}
		prompt += " to " + strings.Join(targetList, ", ") + "?"
		ok, err := promptConfirm(cmd, prompt, true)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		if !ok {
			fmt.Fprintln(out, ui.Muted.Render("aborted"))
			return nil
		}
	}

	succeeded := make([]installPlan, 0, len(plans))
	failures := make([]error, 0)
	for _, p := range plans {
		if err := skill.Install(srcDir, p.dst, force); err != nil {
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

	summary := fmt.Sprintf("installed to %d target%s", len(succeeded), plural(len(succeeded)))
	if len(failures) > 0 {
		summary += fmt.Sprintf(", %d failed", len(failures))
		fmt.Fprintln(out, ui.Warning.Render(summary))
		return &ExitError{Code: 3, Err: combineErrors(failures)}
	}
	fmt.Fprintln(out, ui.Success.Render(summary))
	return nil
}

// promptConfirm asks the user a yes/no question using the cobra-injected
// stdin so tests can drive the prompt. The default answer is yes (Y/n).
func promptConfirm(cmd *cobra.Command, question string, defaultYes bool) (bool, error) {
	suffix := "[y/N]"
	if defaultYes {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\n%s %s ", question, suffix)
	reader := bufio.NewReader(cmd.InOrStdin())
	line, _ := reader.ReadString('\n')
	v := strings.ToLower(strings.TrimSpace(line))
	if v == "" {
		return defaultYes, nil
	}
	switch v {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultYes, nil
	}
}

// discoverSkillRoot returns the directory under extractDir that holds the
// skill's SKILL.md plus the in-extract sub-path it lives at (empty when it
// is at the root).
//
// Resolution order:
//
//  1. If extractDir itself has SKILL.md, use it (the common single-skill
//     repo case, no behavioural change vs. PR 4).
//  2. Otherwise walk for SKILL.md. If exactly one is found, install that
//     nested skill — repos like anthropics/skills publish many skills in
//     one repo; auto-picking when there is no ambiguity makes
//     `kungfu install user/repo` work for them.
//  3. If more than one is found, error with a list. The user picks one
//     with the subpath syntax (`user/repo/<subpath>`); without it we have
//     no way to know which skill they meant.
//
// requestedSubpath is the user-supplied subpath, used only to phrase the
// "no SKILL.md" error message when the extraction was already scoped.
func discoverSkillRoot(extractDir, requestedSubpath string) (string, string, error) {
	if _, err := os.Stat(filepath.Join(extractDir, skill.FileName)); err == nil {
		return extractDir, "", nil
	}
	var matches []string
	walkErr := filepath.WalkDir(extractDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == skill.FileName {
			matches = append(matches, filepath.Dir(p))
		}
		return nil
	})
	if walkErr != nil {
		return "", "", fmt.Errorf("scanning for SKILL.md: %w", walkErr)
	}
	sort.Strings(matches)

	switch len(matches) {
	case 0:
		if requestedSubpath != "" {
			return "", "", fmt.Errorf("subpath %q contains no SKILL.md", requestedSubpath)
		}
		return "", "", errors.New("extracted source has no SKILL.md")
	case 1:
		rel, err := filepath.Rel(extractDir, matches[0])
		if err != nil {
			return "", "", fmt.Errorf("computing subpath: %w", err)
		}
		return matches[0], filepath.ToSlash(rel), nil
	default:
		rels := make([]string, 0, len(matches))
		for _, m := range matches {
			if rel, err := filepath.Rel(extractDir, m); err == nil {
				rels = append(rels, filepath.ToSlash(rel))
			}
		}
		return "", "", fmt.Errorf(
			"this repo contains %d skills; specify one with the subpath syntax (user/repo/<subpath>):\n  - %s",
			len(matches), strings.Join(rels, "\n  - "))
	}
}

// joinSubpath stitches a user-supplied subpath together with one resolved
// by auto-discovery, dropping the empty pieces and normalising separators.
// The result is always forward-slash-separated so it can go straight into
// the kungfu_source frontmatter value.
func joinSubpath(user, discovered string) string {
	parts := []string{}
	for _, p := range []string{user, discovered} {
		p = strings.Trim(filepath.ToSlash(p), "/")
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "/")
}

// shortSHA returns the first 7 characters of sha — the canonical "short"
// commit identifier.
func shortSHA(sha string) string {
	if len(sha) >= 7 {
		return sha[:7]
	}
	return sha
}

// humanBytes renders byte counts in K / M units suitable for short status
// lines.
func humanBytes(n int64) string {
	const k = 1024
	switch {
	case n < k:
		return fmt.Sprintf("%d B", n)
	case n < k*k:
		return fmt.Sprintf("%.1f KB", float64(n)/k)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(k*k))
	}
}

// plural returns "" for n == 1 and "s" otherwise — for "1 target" / "2 targets".
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
