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
	err := cli.Execute()
	if err != nil {
		if msg := err.Error(); msg != "" {
			fmt.Fprintln(os.Stderr, cli.FormatError(err))
		}
	}
	os.Exit(cli.ExitCodeForError(err))
}
