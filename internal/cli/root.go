// Package cli implements the kungfu command-line interface, built on cobra.
//
// The root command owns the global flags (--no-color, --skills-dir,
// --config, --target, --scope) and, in its persistent pre-run, resolves
// configuration and attaches it to the command context so subcommands can
// retrieve it with AppFromContext. Target / scope resolution is done by the
// individual subcommands because the right "empty" behavior varies (list
// uses every configured target; install uses default_targets).
package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/config"
	"github.com/mjcurry/kungfu/internal/ui"
)

// App is the resolved runtime configuration shared with subcommands via the
// command context.
type App struct {
	// Config is the loaded (or default) configuration.
	Config *config.Config

	// SkillsDir is the legacy effective skills directory (PR 1 era).
	SkillsDir string

	// TargetFlag is the raw value of --target. Empty when not set; each
	// command interprets empty in its own way.
	TargetFlag string

	// ScopeFlag is the raw value of --scope. Empty when not set.
	ScopeFlag string
}

type contextKey struct{}

var appContextKey = contextKey{}

// global flag values, bound by NewRootCmd.
var (
	flagNoColor   bool
	flagSkillsDir string
	flagConfig    string
	flagTarget    string
	flagScope     string
)

// NewRootCmd constructs the root cobra command and its subcommand tree. A
// fresh instance is returned each call so tests can execute in isolation.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kungfu",
		Short:         "Manage AI agent skills across every supported agent",
		Long:          "kungfu is the package manager for your agent skills.\nOne CLI, every agent: install one skill to claude, codex, cursor, copilot — together.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if flagNoColor || envSet("NO_COLOR") {
				ui.SetNoColor(true)
			}

			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			app := &App{
				Config:     cfg,
				SkillsDir:  cfg.ResolveSkillsDir(flagSkillsDir),
				TargetFlag: flagTarget,
				ScopeFlag:  flagScope,
			}
			cmd.SetContext(context.WithValue(cmd.Context(), appContextKey, app))
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&flagNoColor, "no-color", false, "disable colored output")
	pf.StringVar(&flagSkillsDir, "skills-dir", "", "legacy skills directory (PR 1; overrides config and $"+config.EnvSkillsDir+")")
	pf.StringVar(&flagConfig, "config", "", "path to config file (default "+config.Path()+")")
	pf.StringVar(&flagTarget, "target", "", "comma-separated target names, or \"all\"; empty uses each command's default")
	pf.StringVar(&flagScope, "scope", "", "\"personal\" or \"project\"; empty uses default_scope")

	root.AddCommand(newVersionCmd())
	root.AddCommand(newLintCmd())
	return root
}

// Execute builds and runs the root command. It is the single entrypoint used
// by main and returns any error for the caller to render and exit on.
func Execute() error {
	return NewRootCmd().ExecuteContext(context.Background())
}

// AppFromContext retrieves the resolved App attached by the root command's
// persistent pre-run. The boolean is false if no App is present (for
// example, when a command is run without the root pre-run, as in tests).
func AppFromContext(ctx context.Context) (*App, bool) {
	app, ok := ctx.Value(appContextKey).(*App)
	return app, ok
}

func loadConfig() (*config.Config, error) {
	if flagConfig != "" {
		cfg, err := config.LoadFrom(flagConfig)
		if err != nil {
			return nil, err
		}
		return cfg, nil
	}
	return config.Load()
}

func envSet(key string) bool {
	v, ok := os.LookupEnv(key)
	return ok && v != ""
}
