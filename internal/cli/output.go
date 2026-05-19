package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/mjcurry/kungfu/internal/ui"
)

// renderJSON writes v to w as indented JSON followed by a newline. It is the
// shared implementation behind every command's --json output so the JSON
// contract stays consistent as commands are added.
func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("cli: encoding JSON output: %w", err)
	}
	return nil
}

// FormatError renders an error for terminal display using the shared error
// style. It is exported for the program entrypoint.
func FormatError(err error) string {
	if err == nil {
		return ""
	}
	return ui.Error.Render("error:") + " " + err.Error()
}
