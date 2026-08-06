package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// RepoResult is one repository from a GitHub search: the subset of fields
// the search command renders.
type RepoResult struct {
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
	URL         string `json:"html_url"`
	Archived    bool   `json:"archived"`
}

// maxSearchResponse caps the search API response body. Search returns at
// most 100 items of metadata; 4MB is generous.
const maxSearchResponse int64 = 4 * 1024 * 1024

// SearchRepositories runs a GitHub repository search and returns up to
// limit results ordered by stars (descending). The query string uses
// GitHub's search syntax verbatim (e.g. "csv topic:agent-skills").
func (c *Client) SearchRepositories(ctx context.Context, query string, limit int) ([]RepoResult, error) {
	if limit <= 0 {
		limit = 15
	}
	if limit > 100 {
		limit = 100
	}
	u := fmt.Sprintf("%s/search/repositories?q=%s&sort=stars&order=desc&per_page=%d",
		c.BaseURL, url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("fetch: search: %w", err)
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: search request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusForbidden, http.StatusTooManyRequests:
		return nil, fmt.Errorf(
			"fetch: search: GitHub rate limit hit (%s); set GITHUB_TOKEN to raise the limit",
			resp.Status)
	default:
		return nil, fmt.Errorf("fetch: search: GitHub returned %s", resp.Status)
	}

	var payload struct {
		Items []RepoResult `json:"items"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxSearchResponse)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("fetch: search: decoding response: %w", err)
	}
	return payload.Items, nil
}
