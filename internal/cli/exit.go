package cli

import "errors"

// ExitError signals that the CLI should terminate with a specific exit code.
// The optional inner Err is rendered to stderr via FormatError before exit;
// pass nil to exit silently (typical when the command has already printed
// command-specific output such as diagnostics).
type ExitError struct {
	Code int
	Err  error
}

// Error reports the inner error message, or empty when there is none.
func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

// Unwrap exposes the inner error for errors.Is / errors.As.
func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// ExitCodeForError returns the exit code main should use for err: 0 for nil,
// the ExitError code when one is present in the chain, otherwise 1.
func ExitCodeForError(err error) int {
	if err == nil {
		return 0
	}
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}
