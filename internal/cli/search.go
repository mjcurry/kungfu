package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/ui"
)

// newSearchCmd builds the `kungfu search` command.
//
// Exit codes: 0 success (including zero hits), 1 invalid input, 5 network
// or GitHub API failure.
func newSearchCmd() *cobra.Command {
	var (
		limit    int
		topic    string
		jsonOut  bool
		allRepos bool
	)
	cmd := &cobra.Command{
		Use:   "search <term>",
		Short: "Search GitHub for skill repositories",
		Long: "Search GitHub repositories for agent skills, most-starred first.\n" +
			"By default results are narrowed to repositories carrying the\n" +
			"agent-skills topic; use --all-repos to search every repository\n" +
			"matching the term instead.\n\n" +
			"Set GITHUB_TOKEN to raise GitHub's anonymous rate limit.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], topic, limit, jsonOut, allRepos)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 15, "maximum results to show (1-100)")
	cmd.Flags().StringVar(&topic, "topic", "agent-skills", "GitHub topic to filter by")
	cmd.Flags().BoolVar(&allRepos, "all-repos", false, "search all repositories, not just the topic-tagged ones")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit results as JSON")
	return cmd
}

func runSearch(cmd *cobra.Command, term, topic string, limit int, jsonOut, allRepos bool) error {
	out := cmd.OutOrStdout()

	term = strings.TrimSpace(term)
	if term == "" {
		return &ExitError{Code: 1, Err: errors.New("search: empty search term")}
	}
	if limit < 1 || limit > 100 {
		return &ExitError{Code: 1, Err: fmt.Errorf("search: --limit %d out of range (1-100)", limit)}
	}

	query := term
	if !allRepos && strings.TrimSpace(topic) != "" {
		query += " topic:" + strings.TrimSpace(topic)
	}

	client := fetch.NewClient()
	results, err := client.SearchRepositories(cmd.Context(), query, limit)
	if err != nil {
		return &ExitError{Code: 5, Err: fmt.Errorf("search: %w", err)}
	}

	if jsonOut {
		type row struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Stars       int    `json:"stars"`
			URL         string `json:"url"`
			Archived    bool   `json:"archived"`
		}
		rows := make([]row, 0, len(results))
		for _, r := range results {
			rows = append(rows, row{
				Name:        r.FullName,
				Description: r.Description,
				Stars:       r.Stars,
				URL:         r.URL,
				Archived:    r.Archived,
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}

	if len(results) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), ui.Muted.Render("no repositories found for \""+query+"\""))
		if !allRepos {
			fmt.Fprintln(cmd.ErrOrStderr(),
				ui.Muted.Render("tip: retry with --all-repos to search beyond the "+topic+" topic"))
		}
		return nil
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, ui.Muted.Render("REPOSITORY\tSTARS\tDESCRIPTION"))
	for _, r := range results {
		name := r.FullName
		if r.Archived {
			name += " (archived)"
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\n", name, r.Stars,
			truncate(sanitizeOneLine(r.Description), 72))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, ui.Muted.Render("install with: kungfu install <repository>"))
	return nil
}

// sanitizeOneLine collapses whitespace runs (including newlines) to single
// spaces and strips non-printable runes, so repo descriptions from the API
// cannot inject control sequences or break the table layout.
func sanitizeOneLine(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !lastSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			lastSpace = true
			continue
		}
		lastSpace = false
		if unicode.IsPrint(r) {
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}
