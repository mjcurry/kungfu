package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/lint"
	"github.com/mjcurry/kungfu/internal/skill"
	tpl "github.com/mjcurry/kungfu/internal/template"
	"github.com/mjcurry/kungfu/internal/ui"
)

// triggerPrefix is the leading clause the SKILL.md templates prepend to the
// user-supplied description. The new command strips it from the user's input
// if they type it themselves so the rendered description does not end up
// reading "Use this skill when Use this skill when …".
const triggerPrefix = "Use this skill when "

// newNewCmd builds the `kungfu new` command.
//
// Exit codes:
//
//	0 — created
//	1 — invalid input (bad name, missing required flag, validation failure)
//	2 — destination collision without --force
//	3 — I/O failure
//	4 — internal self-lint failure (a template-shipped bug)
func newNewCmd() *cobra.Command {
	var (
		templateName string
		description  string
		dir          string
		yes          bool
		force        bool
	)
	cmd := &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a new skill from a built-in template",
		Long: "Scaffold a new skill directory using one of the built-in templates.\n" +
			"The result is guaranteed to pass `kungfu lint` cleanly — the same rule\n" +
			"set every install (local or remote) is checked against — so the skill\n" +
			"is ready to install once you fill in the body.",
		Example: "  # interactive\n" +
			"  kungfu new my-skill\n\n" +
			"  # non-interactive (e.g. CI / scripts)\n" +
			"  kungfu new --yes --template basic --description \\\n" +
			"      \"the user asks to format CSV files\" csv-formatter\n\n" +
			"  # pick a different starting point\n" +
			"  kungfu new --template document report-writer",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := skill.ValidateName(name); err != nil {
				return &ExitError{Code: 1, Err: err}
			}

			parentDir, err := filepath.Abs(firstNonEmptyDir(dir))
			if err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("new: resolving --dir: %w", err)}
			}
			destDir := filepath.Join(parentDir, name)

			prompter, interactive := selectPrompter(cmd)

			if err := handleCollision(cmd.OutOrStdout(), destDir, force, yes, interactive, prompter); err != nil {
				return err
			}

			tplName, err := resolveTemplate(prompter, interactive, templateName, yes)
			if err != nil {
				return err
			}
			desc, err := resolveDescription(prompter, interactive, description, yes)
			if err != nil {
				return err
			}

			template, err := tpl.ByName(tplName)
			if err != nil {
				return &ExitError{Code: 1, Err: err}
			}
			vars := tpl.Vars{
				Name:        name,
				Description: desc,
				Year:        time.Now().Year(),
				CreatedAt:   time.Now().Format(time.RFC3339),
			}
			if _, err := template.Apply(destDir, vars); err != nil {
				return &ExitError{Code: 3, Err: fmt.Errorf("new: %w", err)}
			}

			if err := selfLint(cmd.ErrOrStderr(), destDir); err != nil {
				return err
			}

			printNextSteps(cmd.OutOrStdout(), name, destDir)
			return nil
		},
	}
	cmd.Flags().StringVar(&templateName, "template", "", "scaffold to start from (basic|document|data|api-wrapper); empty prompts")
	cmd.Flags().StringVar(&description, "description", "", "trigger condition for the skill; appended to \"Use this skill when ...\"")
	cmd.Flags().StringVar(&dir, "dir", "", "parent directory (default: current working directory)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip prompts; requires --description")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing destination directory")
	return cmd
}

// firstNonEmptyDir returns dir, defaulting to "." when empty.
func firstNonEmptyDir(dir string) string {
	if dir == "" {
		return "."
	}
	return dir
}

// selectPrompter returns the prompter and whether interactive prompting is
// possible. cobra-injected SetIn is honoured so tests can drive the prompts
// even when stdout/stderr are not real terminals.
func selectPrompter(cmd *cobra.Command) (ui.Prompter, bool) {
	in := cmd.InOrStdin()
	out := cmd.ErrOrStderr()
	prompter := ui.NewPrompterFor(in, out)
	// Tests pipe stdin via SetIn; treat any non-os.Stdin as interactive so
	// the prompter is exercised. For real users, fall back to the terminal
	// check.
	if in != os.Stdin {
		return prompter, true
	}
	return prompter, ui.IsInteractive()
}

func handleCollision(out io.Writer, dest string, force, yes, interactive bool, prompter ui.Prompter) error {
	if _, err := os.Stat(dest); err != nil {
		return nil // dest does not exist
	}
	if !force {
		return &ExitError{Code: 2, Err: fmt.Errorf("new: %s already exists; pass --force to overwrite", dest)}
	}
	if !yes && interactive {
		ok, err := prompter.Confirm(fmt.Sprintf("Overwrite existing %s?", dest), false)
		if err != nil {
			return &ExitError{Code: 1, Err: err}
		}
		if !ok {
			return &ExitError{Code: 2, Err: errors.New("new: aborted")}
		}
	}
	if err := os.RemoveAll(dest); err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("new: removing existing %s: %w", dest, err)}
	}
	fmt.Fprintln(out, ui.Muted.Render("removed: "+dest))
	return nil
}

func resolveTemplate(prompter ui.Prompter, interactive bool, flag string, yes bool) (string, error) {
	if flag != "" {
		if _, err := tpl.ByName(flag); err != nil {
			return "", &ExitError{Code: 1, Err: err}
		}
		return flag, nil
	}
	if yes {
		return "basic", nil
	}
	if !interactive {
		return "", &ExitError{Code: 1, Err: errors.New(
			"new: --template required in non-interactive mode (or pass --yes for the basic template)")}
	}
	options := make([]ui.SelectOption, 0, len(tpl.Builtins()))
	for _, t := range tpl.Builtins() {
		options = append(options, ui.SelectOption{Value: t.Name, Description: t.Description})
	}
	chosen, err := prompter.Select("Template", options, "basic")
	if err != nil {
		return "", &ExitError{Code: 1, Err: err}
	}
	return chosen, nil
}

func resolveDescription(prompter ui.Prompter, interactive bool, flag string, yes bool) (string, error) {
	desc := strings.TrimSpace(flag)
	if desc == "" {
		if yes {
			return "", &ExitError{Code: 1, Err: errors.New(
				"new: --description is required when --yes is set")}
		}
		if !interactive {
			return "", &ExitError{Code: 1, Err: errors.New(
				"new: --description is required in non-interactive mode")}
		}
		got, err := prompter.Input(
			"Description",
			`trigger condition (e.g. "the user asks to format CSV files")`,
			"",
			validateDescriptionInput,
		)
		if err != nil {
			return "", &ExitError{Code: 1, Err: err}
		}
		desc = strings.TrimSpace(got)
	}

	// Strip a leading "Use this skill when " — the template prepends it.
	if cleaned, ok := stripTriggerPrefix(desc); ok {
		desc = cleaned
	}

	if err := validateRenderedDescription(desc); err != nil {
		return "", &ExitError{Code: 1, Err: err}
	}
	return desc, nil
}

func validateDescriptionInput(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return errors.New("description must not be empty")
	}
	return validateRenderedDescription(s)
}

// validateRenderedDescription enforces the upper bound on the assembled
// "Use this skill when <user>" string so the produced skill never trips the
// frontmatter/description-too-long lint rule.
func validateRenderedDescription(userPart string) error {
	if len(triggerPrefix)+len(userPart) > 1024 {
		return fmt.Errorf("description is too long: %d characters; must be at most %d",
			len(triggerPrefix)+len(userPart), 1024)
	}
	return nil
}

// stripTriggerPrefix returns userPart with any leading "Use this skill when "
// removed (case-insensitive). It is a courtesy for users who type the prefix
// the template adds for them.
func stripTriggerPrefix(s string) (string, bool) {
	if len(s) < len(triggerPrefix) {
		return s, false
	}
	if !strings.EqualFold(s[:len(triggerPrefix)], triggerPrefix) {
		return s, false
	}
	return strings.TrimSpace(s[len(triggerPrefix):]), true
}

func selfLint(errOut io.Writer, dir string) error {
	rep, err := lint.NewDefault().Lint(dir)
	if err != nil {
		return &ExitError{Code: 3, Err: fmt.Errorf("new: self-lint: %w", err)}
	}
	if len(rep.Errors()) > 0 {
		fmt.Fprintln(errOut, ui.Error.Render("self-lint produced errors — this is a template bug, please file an issue:"))
		renderLintHuman(errOut, rep)
		return &ExitError{Code: 4}
	}
	if len(rep.Warnings()) > 0 {
		fmt.Fprintln(errOut, ui.Warning.Render("self-lint warnings (review before installing):"))
		renderLintHuman(errOut, rep)
	}
	return nil
}

func printNextSteps(out io.Writer, name, dir string) {
	fmt.Fprintln(out, ui.Success.Render("✓")+" created "+name+" at "+dir)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "next steps:")
	rel, err := filepath.Rel(mustGetwd(), dir)
	if err != nil || strings.HasPrefix(rel, "..") {
		rel = dir
	}
	fmt.Fprintln(out, "  cd "+rel)
	fmt.Fprintln(out, "  edit SKILL.md to fill in your instructions")
	fmt.Fprintln(out, "  kungfu lint .")
	fmt.Fprintln(out, "  kungfu install . --target claude")
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}
