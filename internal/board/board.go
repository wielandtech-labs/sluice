// SPDX-License-Identifier: AGPL-3.0-only

// Package board serves the read-only project board.
package board

import (
	"context"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wielandtech-labs/sluice/pkg/forge"
)

// Project is the last known state of one repository.
type Project struct {
	Repo      string
	PRs       []forge.PullRequest
	Issues    []forge.Issue
	FetchedAt time.Time // zero until the first successful fetch
	Err       string    // last fetch error; PRs and Issues are then stale
}

// InboxItem is something waiting on a human.
type InboxItem struct {
	Repo      string
	Kind      string // "PR" or "Issue"
	Number    int
	Title     string
	URL       string
	Reason    string
	UpdatedAt time.Time
}

// Board holds project state in memory and refreshes it from the forge.
type Board struct {
	forge      forge.Reader
	repos      []string
	humanLabel string

	mu       sync.RWMutex
	projects map[string]Project
}

// New returns a board for repos. Issues carrying humanLabel go to the inbox.
func New(f forge.Reader, repos []string, humanLabel string) *Board {
	b := &Board{forge: f, repos: repos, humanLabel: humanLabel, projects: map[string]Project{}}
	for _, r := range repos {
		b.projects[r] = Project{Repo: r}
	}
	return b
}

// Run refreshes every repo now and then every interval until ctx is done.
func (b *Board) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		b.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// refreshWorkers bounds how many repos are read at once, so a slow repo does
// not hold up the rest and the forge is not hit with one large burst.
const refreshWorkers = 4

// Refresh fetches every repo once. A failed repo keeps its previous data and
// records the error.
func (b *Board) Refresh(ctx context.Context) {
	var wg sync.WaitGroup
	slots := make(chan struct{}, refreshWorkers)
	for _, repo := range b.repos {
		slots <- struct{}{}
		wg.Go(func() {
			defer func() { <-slots }()
			b.refreshRepo(ctx, repo)
		})
	}
	wg.Wait()
}

func (b *Board) refreshRepo(ctx context.Context, repo string) {
	prs, err := b.forge.OpenPullRequests(ctx, repo)
	var issues []forge.Issue
	if err == nil {
		issues, err = b.forge.OpenIssues(ctx, repo)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.projects[repo]
	if err != nil {
		p.Err = err.Error()
		slog.Warn("refresh failed", "repo", repo, "err", err)
	} else {
		p = Project{Repo: repo, PRs: prs, Issues: issues, FetchedAt: time.Now()}
	}
	b.projects[repo] = p
}

// Projects returns all projects in configured order.
func (b *Board) Projects() []Project {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]Project, 0, len(b.repos))
	for _, r := range b.repos {
		out = append(out, b.projects[r])
	}
	return out
}

// Project returns one project by "owner/name".
func (b *Board) Project(repo string) (Project, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.projects[repo]
	return p, ok
}

// Inbox returns everything waiting on a human, most recently updated first.
func (b *Board) Inbox() []InboxItem {
	var items []InboxItem
	for _, p := range b.Projects() {
		items = append(items, inboxFor(p, b.humanLabel)...)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

// inboxFor applies the inbox rules to one project. A non-draft pull request
// needs a human when its checks fail, when it is approved and can be merged,
// or when nobody has reviewed it yet. Draft pull requests, pull requests whose
// checks are still running or unknown, and pull requests with changes
// requested (which wait on the author) are left out. An issue needs a human
// when it carries humanLabel.
func inboxFor(p Project, humanLabel string) []InboxItem {
	var items []InboxItem
	for _, pr := range p.PRs {
		if pr.Draft || pr.Checks == forge.ChecksPending || pr.Checks == forge.ChecksUnknown {
			continue
		}
		var reason string
		switch {
		case pr.Checks == forge.ChecksFailing:
			reason = "checks failing: " + strings.Join(pr.FailingChecks, ", ")
		case pr.Review == forge.ReviewChangesRequested:
			continue
		case pr.Review == forge.ReviewApproved:
			reason = "approved, ready to merge"
		case pr.Checks == forge.ChecksNone:
			reason = "ready for review, no checks ran"
		default:
			reason = "ready for review"
		}
		items = append(items, InboxItem{p.Repo, "PR", pr.Number, pr.Title, pr.URL, reason, pr.UpdatedAt})
	}
	for _, is := range p.Issues {
		if slices.Contains(is.Labels, humanLabel) {
			items = append(items, InboxItem{p.Repo, "Issue", is.Number, is.Title, is.URL, "labelled " + humanLabel, is.UpdatedAt})
		}
	}
	return items
}
