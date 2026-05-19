// Package ui provides a thin set of shared lipgloss styles used across
// kungfu's output, with a single global switch for disabling color.
//
// Color is disabled automatically when the NO_COLOR environment variable is
// set (per https://no-color.org), and can be toggled explicitly with
// SetNoColor — wired to the --no-color flag by the CLI.
package ui

import (
	"os"

	"github.com/charmbracelet/lipgloss"
)

// Pre-built styles. They are package-level variables so SetNoColor can
// neuter them in place; callers should use them directly rather than caching
// copies before color settings are finalized.
var (
	// Error styles fatal, attention-demanding messages.
	Error lipgloss.Style
	// Warning styles recoverable problems.
	Warning lipgloss.Style
	// Success styles confirmations of completed work.
	Success lipgloss.Style
	// Info styles incidental, neutral information.
	Info lipgloss.Style
	// Muted styles de-emphasized secondary text.
	Muted lipgloss.Style
	// Bold styles text that should stand out without color.
	Bold lipgloss.Style
)

func init() {
	_, noColor := os.LookupEnv("NO_COLOR")
	SetNoColor(noColor)
}

// SetNoColor rebuilds the shared styles. When disabled is true the styles
// carry no ANSI color, leaving plain (but still structurally styled, e.g.
// bold) output suitable for pipes, logs, and NO_COLOR environments.
func SetNoColor(disabled bool) {
	if disabled {
		Error = lipgloss.NewStyle().Bold(true)
		Warning = lipgloss.NewStyle()
		Success = lipgloss.NewStyle()
		Info = lipgloss.NewStyle()
		Muted = lipgloss.NewStyle()
		Bold = lipgloss.NewStyle().Bold(true)
		return
	}
	Error = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")) // bright red
	Warning = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))         // bright yellow
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))         // bright green
	Info = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))            // bright blue
	Muted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))            // bright black / grey
	Bold = lipgloss.NewStyle().Bold(true)
}
