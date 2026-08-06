package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mjcurry/kungfu/internal/fetch"
	"github.com/mjcurry/kungfu/internal/testutil/githubfake"
)

func setupSearchEnv(t *testing.T, items []githubfake.SearchItem) (*githubfake.Server, *multiTargetEnv) {
	t.Helper()
	srv := githubfake.NewServer()
	t.Cleanup(srv.Close)
	srv.SetSearchResults(items)
	t.Setenv(fetch.EnvAPIBase, srv.APIBase())
	return srv, setupMultiTargetEnv(t)
}

func TestSearch_RendersTableAndScopesToTopic(t *testing.T) {
	srv, env := setupSearchEnv(t, []githubfake.SearchItem{
		{FullName: "acme/csv-skill", Description: "Read and write CSV files", Stars: 42,
			URL: "https://github.com/acme/csv-skill"},
		{FullName: "beta/csv-tools", Description: "line1\nline2\ttabbed", Stars: 7,
			URL: "https://github.com/beta/csv-tools", Archived: true},
	})

	out, err := runRoot(t, env, "search", "csv")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v\nout:\n%s", exitCode(err), err, out.String())
	}
	body := out.String()
	if !strings.Contains(body, "acme/csv-skill") || !strings.Contains(body, "42") {
		t.Errorf("first result missing:\n%s", body)
	}
	if !strings.Contains(body, "(archived)") {
		t.Errorf("archived marker missing:\n%s", body)
	}
	// Newlines/tabs in descriptions must be flattened.
	if !strings.Contains(body, "line1 line2 tabbed") {
		t.Errorf("description not sanitized to one line:\n%s", body)
	}
	if !strings.Contains(body, "kungfu install") {
		t.Errorf("install hint missing:\n%s", body)
	}
	// The default query scopes to the agent-skills topic.
	if srv.LastSearchQuery != "csv topic:agent-skills" {
		t.Errorf("query = %q, want 'csv topic:agent-skills'", srv.LastSearchQuery)
	}
}

func TestSearch_AllReposDropsTopic(t *testing.T) {
	srv, env := setupSearchEnv(t, nil)
	if _, err := runRoot(t, env, "search", "--all-repos", "csv"); exitCode(err) != 0 {
		t.Fatal("search --all-repos failed")
	}
	if srv.LastSearchQuery != "csv" {
		t.Errorf("query = %q, want bare 'csv'", srv.LastSearchQuery)
	}
}

func TestSearch_JSONOutput(t *testing.T) {
	_, env := setupSearchEnv(t, []githubfake.SearchItem{
		{FullName: "acme/one", Description: "d", Stars: 3, URL: "https://github.com/acme/one"},
	})
	out, err := runRoot(t, env, "search", "--json", "one")
	if exitCode(err) != 0 {
		t.Fatalf("exit %d: %v", exitCode(err), err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}
	if len(rows) != 1 || rows[0]["name"] != "acme/one" || rows[0]["stars"] != float64(3) {
		t.Errorf("rows = %v", rows)
	}
}

func TestSearch_NoResultsExitsZeroWithTip(t *testing.T) {
	_, env := setupSearchEnv(t, nil)
	out, err := runRoot(t, env, "search", "nothing-matches")
	if exitCode(err) != 0 {
		t.Fatalf("zero hits should still exit 0, got %d", exitCode(err))
	}
	if !strings.Contains(out.String(), "no repositories found") {
		t.Errorf("missing empty-result message:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "--all-repos") {
		t.Errorf("missing --all-repos tip:\n%s", out.String())
	}
}

func TestSearch_BadLimitExits1(t *testing.T) {
	_, env := setupSearchEnv(t, nil)
	if _, err := runRoot(t, env, "search", "--limit", "0", "x"); exitCode(err) != 1 {
		t.Errorf("limit 0: exit %d, want 1", exitCode(err))
	}
	if _, err := runRoot(t, env, "search", "--limit", "101", "x"); exitCode(err) != 1 {
		t.Errorf("limit 101: exit %d, want 1", exitCode(err))
	}
}

func TestSearch_NetworkFailureExits5(t *testing.T) {
	srv, env := setupSearchEnv(t, nil)
	srv.Close() // kill the server so the request fails
	if _, err := runRoot(t, env, "search", "x"); exitCode(err) != 5 {
		t.Errorf("dead server: exit %d, want 5", exitCode(err))
	}
}
