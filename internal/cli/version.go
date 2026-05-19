package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/ui"
)

// Build metadata, overridden at link time via -ldflags
// "-X github.com/mjcurry/kungfu/internal/cli.version=..." (see the
// Makefile). The defaults apply to `go run` and `go install` builds.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// VersionInfo is the structured form of `kungfu version`, also used as the
// --json payload.
type VersionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// currentVersion captures build and runtime metadata.
func currentVersion() VersionInfo {
	return VersionInfo{
		Version:   version,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}

func newVersionCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := currentVersion()
			if asJSON {
				return renderJSON(cmd.OutOrStdout(), info)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, ui.Bold.Render("kungfu "+info.Version))
			fmt.Fprintln(out, ui.Muted.Render("  commit: "+info.Commit))
			fmt.Fprintln(out, ui.Muted.Render("  built:  "+info.Date))
			fmt.Fprintln(out, ui.Muted.Render(fmt.Sprintf("  go:     %s (%s/%s)", info.GoVersion, info.OS, info.Arch)))
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "output as JSON")
	return cmd
}
