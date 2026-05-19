// Package cli implements the kungfu command-line interface, built on cobra.
//
// The root command owns the global flags (--no-color, --skills-dir,
// --config) and, in its persistent pre-run, resolves configuration and
// attaches it to the command context so subcommands can retrieve it with
// AppFromContext.
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
	// SkillsDir is the effective skills directory after applying flag,
	// environment, config, and default precedence, with ~ expanded.
	SkillsDir string
}

type contextKey struct{}

var appContextKey = contextKey{}

// global flag values, bound by NewRootCmd.
var (
	flagNoColor   bool
	flagSkillsDir string
	flagConfig    string
)

// NewRootCmd constructs the root cobra command and its subcommand tree. A
// fresh instance is returned each call so tests can execute in isolation.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "kungfu",
		Short:         "Manage AI agent skills",
		Long:          "kungfu manages AI agent skills: directories containing a SKILL.md\nfile that teach an agent a new capability via progressive disclosure.",
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
				Config:    cfg,
				SkillsDir: cfg.ResolveSkillsDir(flagSkillsDir),
			}
			cmd.SetContext(context.WithValue(cmd.Context(), appContextKey, app))
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.BoolVar(&flagNoColor, "no-color", false, "disable colored output")
	pf.StringVar(&flagSkillsDir, "skills-dir", "", "skills directory (overrides config and $"+config.EnvSkillsDir+")")
	pf.StringVar(&flagConfig, "config", "", "path to config file (default "+config.Path()+")")

	root.AddCommand(newVersionCmd())
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
