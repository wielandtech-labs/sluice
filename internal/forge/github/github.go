// SPDX-License-Identifier: AGPL-3.0-only

// Package github implements forge.Reader against the GitHub REST API.
package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/wielandtech-labs/sluice/pkg/forge"
)

// Lists are read pageSize items at a time, up to maxPages pages.
const (
	pageSize = 100
	maxPages = 10
)

// Client reads from GitHub. Token may be empty for public repositories.
type Client struct {
	BaseURL string // e.g. https://api.github.com, no trailing slash
	Token   string
	HTTP    *http.Client
}

// New returns a client for github.com.
func New(token string) *Client {
	return &Client{
		BaseURL: "https://api.github.com",
		Token:   token,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "sluice-board")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: %s: %s", path, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// list reads every page of a JSON array endpoint. path must already contain a
// query string.
func list[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var all []T
	for page := 1; page <= maxPages; page++ {
		var items []T
		if err := c.get(ctx, fmt.Sprintf("%s&per_page=%d&page=%d", path, pageSize, page), &items); err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) < pageSize {
			break
		}
	}
	return all, nil
}

type apiUser struct {
	Login string `json:"login"`
}

type apiPull struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	HTMLURL   string    `json:"html_url"`
	User      apiUser   `json:"user"`
	Draft     bool      `json:"draft"`
	UpdatedAt time.Time `json:"updated_at"`
	Head      struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

type apiCheckRuns struct {
	CheckRuns []apiCheckRun `json:"check_runs"`
}

type apiCheckRun struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
}

type apiIssue struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	HTMLURL     string    `json:"html_url"`
	User        apiUser   `json:"user"`
	UpdatedAt   time.Time `json:"updated_at"`
	PullRequest *struct{} `json:"pull_request"`
	Labels      []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// OpenPullRequests lists open pull requests with the check state of each head.
func (c *Client) OpenPullRequests(ctx context.Context, repo string) ([]forge.PullRequest, error) {
	pulls, err := list[apiPull](ctx, c, fmt.Sprintf("/repos/%s/pulls?state=open", repo))
	if err != nil {
		return nil, err
	}
	out := make([]forge.PullRequest, 0, len(pulls))
	for _, p := range pulls {
		var runs apiCheckRuns
		if err := c.get(ctx, fmt.Sprintf("/repos/%s/commits/%s/check-runs?per_page=%d", repo, p.Head.SHA, pageSize), &runs); err != nil {
			return nil, err
		}
		state, failing := summarise(runs.CheckRuns)
		out = append(out, forge.PullRequest{
			Number:        p.Number,
			Title:         p.Title,
			URL:           p.HTMLURL,
			Author:        p.User.Login,
			HeadSHA:       p.Head.SHA,
			Draft:         p.Draft,
			UpdatedAt:     p.UpdatedAt,
			Checks:        state,
			FailingChecks: failing,
		})
	}
	return out, nil
}

// OpenIssues lists open issues, leaving out pull requests.
func (c *Client) OpenIssues(ctx context.Context, repo string) ([]forge.Issue, error) {
	issues, err := list[apiIssue](ctx, c, fmt.Sprintf("/repos/%s/issues?state=open", repo))
	if err != nil {
		return nil, err
	}
	out := make([]forge.Issue, 0, len(issues))
	for _, i := range issues {
		if i.PullRequest != nil {
			continue
		}
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, forge.Issue{
			Number:    i.Number,
			Title:     i.Title,
			URL:       i.HTMLURL,
			Author:    i.User.Login,
			Labels:    labels,
			UpdatedAt: i.UpdatedAt,
		})
	}
	return out, nil
}

// summarise reduces check runs to one state. Any failed run makes the commit
// failing; otherwise any unfinished run makes it pending.
func summarise(runs []apiCheckRun) (forge.CheckState, []string) {
	if len(runs) == 0 {
		return forge.ChecksNone, nil
	}
	var failing []string
	pending := false
	for _, r := range runs {
		if r.Status != "completed" {
			pending = true
			continue
		}
		switch r.Conclusion {
		case "success", "neutral", "skipped":
		default:
			failing = append(failing, r.Name)
		}
	}
	sort.Strings(failing)
	switch {
	case len(failing) > 0:
		return forge.ChecksFailing, failing
	case pending:
		return forge.ChecksPending, nil
	}
	return forge.ChecksPassing, nil
}
