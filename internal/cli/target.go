package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/config"
	targetpkg "github.com/mjcurry/kungfu/internal/target"
	"github.com/mjcurry/kungfu/internal/ui"
)

// targetNameRE constrains custom target names to bare-key-safe lowercase
// kebab-case: the name is written as a TOML section key ([targets.<name>])
// and parsed out of comma-separated --target values, so colons, dots, and
// uppercase are all off the table.
var targetNameRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// newTargetCmd builds the `kungfu target` command group.
func newTargetCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "target",
		Short: "Inspect and define skill-installation targets",
		Long: "List the configured targets (agents) or define a custom one.\n" +
			"A target is an agent's pair of skills directories: personal scope\n" +
			"under your home, project scope relative to a repo root.",
	}
	cmd.AddCommand(newTargetListCmd())
	cmd.AddCommand(newTargetAddCmd())
	return cmd
}

// newTargetListCmd builds `kungfu target list`.
func newTargetListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured targets and their directories",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, ok := AppFromContext(cmd.Context())
			if !ok {
				return &ExitError{Code: 2, Err: errors.New("target list: no app in context")}
			}
			builtin := map[string]bool{}
			for _, b := range targetpkg.Builtins() {
				builtin[b.Name] = true
			}
			defaults := map[string]bool{}
			for _, name := range app.Config.DefaultTargets {
				defaults[name] = true
			}

			out := cmd.OutOrStdout()
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, ui.Muted.Render("NAME\tPERSONAL\tPROJECT\tSOURCE\tDEFAULT"))
			for _, t := range app.Config.Targets {
				source := "custom"
				if builtin[t.Name] {
					source = "builtin"
				}
				def := ""
				if defaults[t.Name] {
					def = "*"
				}
				personal := t.PersonalDir
				if personal == "" {
					personal = "-"
				}
				project := t.ProjectDir
				if project == "" {
					project = "-"
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
					t.Name, abbreviateHome(personal), project, source, def)
			}
			return tw.Flush()
		},
	}
}

// newTargetAddCmd builds `kungfu target add`.
//
// Exit codes: 0 success, 1 invalid input or name collision, 3 I/O failure.
func newTargetAddCmd() *cobra.Command {
	var (
		personalDir string
		projectDir  string
	)
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Define a custom target in your config file",
		Long: "Append a [targets.<name>] section to the kungfu config file so any\n" +
			"agent that reads SKILL.md directories can be installed to by name.\n" +
			"At least one of --personal-dir / --project-dir is required.\n\n" +
			"The section is appended textually, so comments and formatting in\n" +
			"your existing config are preserved.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTargetAdd(cmd, args[0], personalDir, projectDir)
		},
	}
	cmd.Flags().StringVar(&personalDir, "personal-dir", "",
		"personal-scope skills directory (may start with ~/)")
	cmd.Flags().StringVar(&projectDir, "project-dir", "",
		"project-scope skills directory, relative to a project root")
	return cmd
}

func runTargetAdd(cmd *cobra.Command, name, personalDir, projectDir string) error {
	out := cmd.OutOrStdout()

	name = strings.TrimSpace(name)
	if !targetNameRE.MatchString(name) {
		return &ExitError{Code: 1, Err: fmt.Errorf(
			"target add: name %q must be lowercase-kebab-case (e.g. my-agent)", name)}
	}
	if name == "all" {
		return &ExitError{Code: 1, Err: errors.New(
			`target add: "all" is reserved (it means every target in --target)`)}
	}
	for _, b := range targetpkg.Builtins() {
		if b.Name == name {
			return &ExitError{Code: 1, Err: fmt.Errorf(
				"target add: %q is a builtin target; to change its directories, edit the [targets.%s] section in %s",
				name, name, configFilePath())}
		}
	}
	personalDir = strings.TrimSpace(personalDir)
	projectDir = strings.TrimSpace(projectDir)
	if personalDir == "" && projectDir == "" {
		return &ExitError{Code: 1, Err: errors.New(
			"target add: at least one of --personal-dir / --project-dir is required")}
	}
	if filepath.IsAbs(projectDir) {
		return &ExitError{Code: 1, Err: fmt.Errorf(
			"target add: --project-dir %q must be relative to a project root, not absolute", projectDir)}
	}

	path := configFilePath()
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return &ExitError{Code: 3, Err: fmt.Errorf("target add: reading %s: %w", path, err)}
	}
	if hasTargetSection(string(existing), name) {
		return &ExitError{Code: 1, Err: fmt.Errorf(
			"target add: [targets.%s] already exists in %s; edit it there instead", name, path)}
	}

	section := renderTargetSection(name, personalDir, projectDir)
	updated := string(existing)
	if updated != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	if updated != "" {
		updated += "\n"
	}
	updated += section

	// Validate the result parses before we write anything, so a bad path
	// can never leave a broken config behind.
	if err := validateConfigBytes(updated); err != nil {
		return &ExitError{Code: 1, Err: fmt.Errorf("target add: %w", err)}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("target add: creating config dir: %w", err)}
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("target add: writing %s: %w", path, err)}
	}

	fmt.Fprintf(out, "%s added target %s to %s\n",
		ui.Success.Render("✓"), ui.Bold.Render(name), path)
	fmt.Fprintf(out, "  install to it with: kungfu install <source> --target %s\n", name)
	return nil
}

// configFilePath returns the config file the CLI is operating on: the
// --config flag when given, the XDG default otherwise.
func configFilePath() string {
	if flagConfig != "" {
		return flagConfig
	}
	return config.Path()
}

// hasTargetSection reports whether raw already contains a [targets.<name>]
// section header, tolerating whitespace inside the brackets.
func hasTargetSection(raw, name string) bool {
	re := regexp.MustCompile(`(?m)^\s*\[\s*targets\.` + regexp.QuoteMeta(name) + `\s*\]`)
	return re.MatchString(raw)
}

// renderTargetSection renders the TOML section for a custom target. Paths
// are written as literal (single-quoted) strings so Windows backslashes
// survive; a path containing a single quote falls back to a basic string.
func renderTargetSection(name, personalDir, projectDir string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[targets.%s]\n", name)
	if personalDir != "" {
		fmt.Fprintf(&b, "personal_dir = %s\n", tomlString(personalDir))
	}
	if projectDir != "" {
		fmt.Fprintf(&b, "project_dir = %s\n", tomlString(projectDir))
	}
	return b.String()
}

func tomlString(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	return fmt.Sprintf("%q", s)
}

// validateConfigBytes writes the candidate config to a temp file and runs
// the real loader over it, so `target add` refuses to produce a file that
// kungfu itself cannot parse.
func validateConfigBytes(raw string) error {
	tmp, err := os.CreateTemp("", "kungfu-config-check-*.toml")
	if err != nil {
		return fmt.Errorf("validating: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(raw); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("validating: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("validating: %w", err)
	}
	if _, err := config.LoadFrom(tmp.Name()); err != nil {
		return fmt.Errorf("resulting config would not parse: %w", err)
	}
	return nil
}
