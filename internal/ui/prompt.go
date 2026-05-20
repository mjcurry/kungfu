package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// Prompter collects user input for interactive commands. The default
// implementation reads from stdin and writes prompts to stderr so that
// stdout — which a caller may be capturing — stays clean. Commands that
// run non-interactively should detect this with IsInteractive and fall
// back to flag-only behaviour rather than calling Prompter.
type Prompter interface {
	// Select asks the user to choose one of options. defaultValue is
	// preselected; if the user accepts (empty input), it is returned.
	Select(label string, options []SelectOption, defaultValue string) (string, error)
	// Input asks for a free-form line. hint is rendered in muted style on
	// the same line; defaultValue is used when the user submits empty.
	// validate, if non-nil, is invoked on the resolved value; non-nil
	// error re-prompts up to a small bounded number of times.
	Input(label, hint, defaultValue string, validate func(string) error) (string, error)
	// Confirm asks a yes/no question. defaultValue determines which
	// answer is taken on empty input.
	Confirm(label string, defaultValue bool) (bool, error)
}

// SelectOption is one entry in a Select prompt.
type SelectOption struct {
	// Value is what's returned when the option is chosen.
	Value string
	// Description is the human-readable explanation shown next to the
	// option's value in the list.
	Description string
}

// NewPrompter returns a Prompter that reads from os.Stdin and writes
// prompts to os.Stderr.
func NewPrompter() Prompter {
	return newStdPrompter(os.Stdin, os.Stderr)
}

// NewPrompterFor returns a Prompter bound to specific readers/writers.
// Useful in tests.
func NewPrompterFor(in io.Reader, out io.Writer) Prompter {
	return newStdPrompter(in, out)
}

func newStdPrompter(in io.Reader, out io.Writer) *stdPrompter {
	return &stdPrompter{reader: bufio.NewReader(in), out: out}
}

// IsInteractive reports whether stdin is connected to a terminal. Commands
// that need interactive prompts use this to decide whether to call Prompter
// at all; in CI / pipes they require the equivalent flags to be set.
func IsInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ErrPromptAborted is returned when the user cancels a prompt (EOF on
// stdin) or the validator rejects too many times.
var ErrPromptAborted = errors.New("ui: prompt aborted")

const maxValidationAttempts = 5

type stdPrompter struct {
	// reader is a *single* bufio.Reader shared across calls so a line
	// consumed by Select does not strand the next prompt's input inside a
	// throwaway buffer.
	reader *bufio.Reader
	out    io.Writer
}

func (p *stdPrompter) Select(label string, options []SelectOption, defaultValue string) (string, error) {
	if len(options) == 0 {
		return "", errors.New("ui: Select called with no options")
	}
	defaultIdx := 0
	for i, o := range options {
		if o.Value == defaultValue {
			defaultIdx = i
		}
	}
	for attempt := 0; attempt < maxValidationAttempts; attempt++ {
		fmt.Fprintln(p.out, Bold.Render(label))
		for i, o := range options {
			marker := "  "
			if i == defaultIdx {
				marker = Bold.Render("▸ ")
			}
			fmt.Fprintf(p.out, "%s%d. %s %s\n", marker, i+1,
				Bold.Render(o.Value), Muted.Render(o.Description))
		}
		fmt.Fprintf(p.out, "[1-%d, default %d]: ", len(options), defaultIdx+1)

		line, err := p.reader.ReadString('\n')
		if err != nil && line == "" {
			return "", ErrPromptAborted
		}
		line = strings.TrimSpace(line)
		if line == "" {
			return options[defaultIdx].Value, nil
		}
		n, err := strconv.Atoi(line)
		if err == nil && n >= 1 && n <= len(options) {
			return options[n-1].Value, nil
		}
		// Also accept the value string itself for convenience.
		for _, o := range options {
			if strings.EqualFold(o.Value, line) {
				return o.Value, nil
			}
		}
		fmt.Fprintln(p.out, Warning.Render("please choose a number from the list"))
	}
	return "", ErrPromptAborted
}

func (p *stdPrompter) Input(label, hint, defaultValue string, validate func(string) error) (string, error) {
	for attempt := 0; attempt < maxValidationAttempts; attempt++ {
		fmt.Fprint(p.out, Bold.Render(label))
		if hint != "" {
			fmt.Fprint(p.out, "  "+Muted.Render(hint))
		}
		if defaultValue != "" {
			fmt.Fprint(p.out, Muted.Render("  ["+defaultValue+"]"))
		}
		fmt.Fprint(p.out, "\n> ")

		line, err := p.reader.ReadString('\n')
		if err != nil && line == "" {
			return "", ErrPromptAborted
		}
		v := strings.TrimSpace(line)
		if v == "" {
			v = defaultValue
		}
		if validate != nil {
			if verr := validate(v); verr != nil {
				fmt.Fprintln(p.out, Warning.Render(verr.Error()))
				continue
			}
		}
		return v, nil
	}
	return "", ErrPromptAborted
}

func (p *stdPrompter) Confirm(label string, defaultValue bool) (bool, error) {
	suffix := "[y/N]"
	if defaultValue {
		suffix = "[Y/n]"
	}
	fmt.Fprintf(p.out, "%s %s ", Bold.Render(label), Muted.Render(suffix))

	line, err := p.reader.ReadString('\n')
	if err != nil && line == "" {
		return false, ErrPromptAborted
	}
	v := strings.ToLower(strings.TrimSpace(line))
	if v == "" {
		return defaultValue, nil
	}
	switch v {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return defaultValue, nil
	}
}
