// SPDX-License-Identifier: AGPL-3.0-only

package board

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wielandtech-labs/sluice/pkg/forge"
)

type fakeForge struct {
	prs    []forge.PullRequest
	issues []forge.Issue
	err    error
}

func (f *fakeForge) OpenPullRequests(context.Context, string) ([]forge.PullRequest, error) {
	return f.prs, f.err
}

func (f *fakeForge) OpenIssues(context.Context, string) ([]forge.Issue, error) {
	return f.issues, f.err
}

func sample() *fakeForge {
	now := time.Now()
	return &fakeForge{
		prs: []forge.PullRequest{
			{Number: 1, Title: "broken", URL: "https://x/1", Checks: forge.ChecksFailing, FailingChecks: []string{"build", "test"}, UpdatedAt: now.Add(-3 * time.Hour)},
			{Number: 2, Title: "ready", URL: "https://x/2", Checks: forge.ChecksPassing, UpdatedAt: now.Add(-1 * time.Hour)},
			{Number: 3, Title: "running", URL: "https://x/3", Checks: forge.ChecksPending, UpdatedAt: now},
			{Number: 4, Title: "wip", URL: "https://x/4", Draft: true, Checks: forge.ChecksFailing, UpdatedAt: now},
			{Number: 7, Title: "unreadable", URL: "https://x/7", Checks: forge.ChecksUnknown, UpdatedAt: now},
			{Number: 8, Title: "approved", URL: "https://x/8", Checks: forge.ChecksPassing, Review: forge.ReviewApproved, UpdatedAt: now.Add(-4 * time.Hour)},
			{Number: 9, Title: "sent back", URL: "https://x/9", Checks: forge.ChecksPassing, Review: forge.ReviewChangesRequested, UpdatedAt: now},
			{Number: 10, Title: "no ci", URL: "https://x/10", Checks: forge.ChecksNone, UpdatedAt: now.Add(-5 * time.Hour)},
			{Number: 11, Title: "sent back and red", URL: "https://x/11", Checks: forge.ChecksFailing, FailingChecks: []string{"lint"}, Review: forge.ReviewChangesRequested, UpdatedAt: now.Add(-6 * time.Hour)},
		},
		issues: []forge.Issue{
			{Number: 5, Title: "<script>alert(1)</script>", URL: "https://x/5", Labels: []string{"needs-human"}, UpdatedAt: now.Add(-2 * time.Hour)},
			{Number: 6, Title: "for an agent", URL: "javascript:alert(1)", Labels: []string{"agent-ready"}, UpdatedAt: now},
		},
	}
}

func TestInboxRules(t *testing.T) {
	b := New(sample(), []string{"o/r"}, "needs-human")
	b.Refresh(context.Background())

	got := b.Inbox()
	want := []struct {
		number int
		reason string
	}{
		{2, "ready for review"},
		{5, "labelled needs-human"},
		{1, "checks failing: build, test"},
		{8, "approved, ready to merge"},
		{10, "ready for review, no checks ran"},
		{11, "checks failing: lint"},
	}
	if len(got) != len(want) {
		t.Fatalf("inbox has %d items, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Number != w.number || got[i].Reason != w.reason {
			t.Errorf("item %d = #%d %q, want #%d %q", i, got[i].Number, got[i].Reason, w.number, w.reason)
		}
	}
}

func TestRefreshErrorKeepsOldData(t *testing.T) {
	f := sample()
	b := New(f, []string{"o/r"}, "needs-human")
	b.Refresh(context.Background())

	f.err = errors.New("boom")
	b.Refresh(context.Background())

	p, _ := b.Project("o/r")
	if p.Err != "boom" || len(p.PRs) != 9 || p.FetchedAt.IsZero() {
		t.Errorf("project after failed refresh = err %q, %d PRs, fetched %v", p.Err, len(p.PRs), p.FetchedAt)
	}
}

func TestPages(t *testing.T) {
	b := New(sample(), []string{"o/r"}, "needs-human")
	b.Refresh(context.Background())
	h := b.Handler()

	get := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}

	for path, want := range map[string]string{
		"/":                 `href="/p/o/r"`,
		"/inbox":            "checks failing: build, test",
		"/p/o/r":            "for an agent",
		"/healthz":          "ok",
		"/static/style.css": "color-scheme",
	} {
		rec := get(http.MethodGet, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("GET %s = %d, body missing %q", path, rec.Code, want)
		}
	}

	page := get(http.MethodGet, "/p/o/r")
	if body := page.Body.String(); strings.Contains(body, "<script>alert") || strings.Contains(body, `href="javascript:`) {
		t.Error("forge text was not escaped")
	}
	if page.Header().Get("Content-Security-Policy") == "" {
		t.Error("missing Content-Security-Policy")
	}

	if rec := get(http.MethodGet, "/p/o/unknown"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown project = %d", rec.Code)
	}
	if rec := get(http.MethodPost, "/inbox"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /inbox = %d, want 405", rec.Code)
	}
}

func TestAgo(t *testing.T) {
	for d, want := range map[time.Duration]string{
		10 * time.Second: "just now",
		5 * time.Minute:  "5m ago",
		3 * time.Hour:    "3h ago",
		72 * time.Hour:   "3d ago",
	} {
		if got := ago(d); got != want {
			t.Errorf("ago(%v) = %q, want %q", d, got, want)
		}
	}
}
