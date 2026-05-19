// Command kungfu is a CLI for managing AI agent skills: directories
// containing a SKILL.md file that teach an agent a new capability via the
// progressive-disclosure pattern.
package main

import (
	"fmt"
	"os"

	"github.com/mjcurry/kungfu/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, cli.FormatError(err))
		os.Exit(1)
	}
}
