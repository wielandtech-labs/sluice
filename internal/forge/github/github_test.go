// SPDX-License-Identifier: AGPL-3.0-only

package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/wielandtech-labs/sluice/pkg/forge"
)

func testClient(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("Authorization = %q", got)
		}
		body, ok := routes[r.URL.Path]
		if !ok {
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "tok", HTTP: srv.Client()}
}

func TestOpenPullRequests(t *testing.T) {
	c := testClient(t, map[string]string{
		"/repos/o/r/pulls": `[
			{"number":1,"title":"one","html_url":"https://x/1","user":{"login":"a"},"draft":false,"updated_at":"2026-11-01T10:00:00Z","head":{"sha":"aaa"}},
			{"number":2,"title":"two","html_url":"https://x/2","user":{"login":"b"},"draft":true,"updated_at":"2026-11-02T10:00:00Z","head":{"sha":"bbb"}}]`,
		"/repos/o/r/commits/aaa/check-runs": `{"check_runs":[
			{"name":"test","status":"completed","conclusion":"failure"},
			{"name":"build","status":"completed","conclusion":"success"},
			{"name":"lint","status":"in_progress","conclusion":null}]}`,
		"/repos/o/r/commits/aaa/status":     `{"statuses":[]}`,
		"/repos/o/r/commits/bbb/check-runs": `{"check_runs":[]}`,
		"/repos/o/r/commits/bbb/status":     `{"statuses":[]}`,
		"/repos/o/r/pulls/1/reviews":        `[{"user":{"login":"x"},"state":"APPROVED","commit_id":"aaa","author_association":"OWNER"}]`,
	})
	prs, err := c.OpenPullRequests(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d pull requests", len(prs))
	}
	if prs[0].Checks != forge.ChecksFailing || !reflect.DeepEqual(prs[0].FailingChecks, []string{"test"}) {
		t.Errorf("pr 1 checks = %s %v", prs[0].Checks, prs[0].FailingChecks)
	}
	if prs[0].Author != "a" || prs[0].HeadSHA != "aaa" || prs[0].UpdatedAt.IsZero() {
		t.Errorf("pr 1 fields = %+v", prs[0])
	}
	if prs[0].Review != forge.ReviewApproved {
		t.Errorf("pr 1 review = %q", prs[0].Review)
	}
	// The reviews call for pr 2 returns 404.
	if prs[1].Review != forge.ReviewUnknown {
		t.Errorf("pr 2 review = %q", prs[1].Review)
	}
	if !prs[1].Draft || prs[1].Checks != forge.ChecksNone {
		t.Errorf("pr 2 = %+v", prs[1])
	}
}

func TestOpenIssuesSkipsPullRequests(t *testing.T) {
	c := testClient(t, map[string]string{
		"/repos/o/r/issues": `[
			{"number":3,"title":"bug","html_url":"https://x/3","user":{"login":"a"},"updated_at":"2026-11-01T10:00:00Z","labels":[{"name":"needs-human"}]},
			{"number":4,"title":"a pr","html_url":"https://x/4","user":{"login":"a"},"updated_at":"2026-11-01T10:00:00Z","labels":[],"pull_request":{}}]`,
	})
	issues, err := c.OpenIssues(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].Number != 3 || !reflect.DeepEqual(issues[0].Labels, []string{"needs-human"}) {
		t.Errorf("issues = %+v", issues)
	}
}

func TestChecksIncludeCommitStatuses(t *testing.T) {
	routes := map[string]string{
		"/repos/o/r/commits/aaa/check-runs": `{"check_runs":[{"name":"build","status":"completed","conclusion":"success"}]}`,
		"/repos/o/r/commits/aaa/status":     `{"statuses":[{"context":"ci/external","state":"failure"}]}`,
		"/repos/o/r/commits/bbb/check-runs": `{"check_runs":[]}`,
		"/repos/o/r/commits/bbb/status":     `{"statuses":[{"context":"ci/external","state":"pending"}]}`,
		"/repos/o/r/commits/ccc/check-runs": `{"check_runs":[]}`,
	}
	c := testClient(t, routes)
	if state, failing := c.checks(context.Background(), "o/r", "aaa"); state != forge.ChecksFailing || !reflect.DeepEqual(failing, []string{"ci/external"}) {
		t.Errorf("aaa = %s %v", state, failing)
	}
	if state, _ := c.checks(context.Background(), "o/r", "bbb"); state != forge.ChecksPending {
		t.Errorf("bbb = %s", state)
	}
	// The status call for ccc returns 404: the state is unknown, not an error.
	if state, _ := c.checks(context.Background(), "o/r", "ccc"); state != forge.ChecksUnknown {
		t.Errorf("ccc = %s", state)
	}
}

func TestListFollowsPages(t *testing.T) {
	var mu sync.Mutex
	var pages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		mu.Lock()
		pages = append(pages, page)
		mu.Unlock()
		n := 0
		switch page {
		case "1":
			n = pageSize
		case "2":
			n = 3
		}
		w.Write([]byte("[" + strings.TrimSuffix(strings.Repeat(`{"number":1},`, n), ",") + "]"))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	issues, err := c.OpenIssues(context.Background(), "o/r")
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(issues) != pageSize+3 || !reflect.DeepEqual(pages, []string{"1", "2"}) {
		t.Errorf("got %d issues over pages %v", len(issues), pages)
	}
}

func TestErrorIncludesStatus(t *testing.T) {
	c := testClient(t, nil)
	_, err := c.OpenIssues(context.Background(), "o/missing")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %v", err)
	}
}

func TestSummariseReviews(t *testing.T) {
	r := func(user, state string) apiReview {
		return apiReview{User: apiUser{Login: user}, State: state, CommitID: "head", AuthorAssociation: "COLLABORATOR"}
	}
	old := r("a", "APPROVED")
	old.CommitID = "earlier"
	stranger := r("s", "CHANGES_REQUESTED")
	stranger.AuthorAssociation = "NONE"
	ghost := r("", "APPROVED")
	cases := []struct {
		name    string
		reviews []apiReview
		want    forge.ReviewState
	}{
		{"none", nil, forge.ReviewNone},
		{"comments only", []apiReview{r("a", "COMMENTED")}, forge.ReviewNone},
		{"approved", []apiReview{r("a", "APPROVED")}, forge.ReviewApproved},
		{"comment keeps approval", []apiReview{r("a", "APPROVED"), r("a", "COMMENTED")}, forge.ReviewApproved},
		{"later approval replaces request", []apiReview{r("a", "CHANGES_REQUESTED"), r("a", "APPROVED")}, forge.ReviewApproved},
		{"request outweighs approval", []apiReview{r("a", "APPROVED"), r("b", "CHANGES_REQUESTED")}, forge.ReviewChangesRequested},
		{"dismissed", []apiReview{r("a", "CHANGES_REQUESTED"), r("a", "DISMISSED")}, forge.ReviewNone},
		{"review of an earlier commit", []apiReview{old}, forge.ReviewNone},
		{"stranger cannot block", []apiReview{r("a", "APPROVED"), stranger}, forge.ReviewApproved},
		{"deleted user", []apiReview{ghost}, forge.ReviewNone},
	}
	for _, tc := range cases {
		if got := summariseReviews(tc.reviews, "head"); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestSummarise(t *testing.T) {
	done := func(conclusion string) apiCheckRun {
		return apiCheckRun{Name: conclusion, Status: "completed", Conclusion: conclusion}
	}
	cases := []struct {
		name string
		runs []apiCheckRun
		want forge.CheckState
	}{
		{"none", nil, forge.ChecksNone},
		{"all good", []apiCheckRun{done("success"), done("skipped"), done("neutral")}, forge.ChecksPassing},
		{"pending", []apiCheckRun{done("success"), {Name: "x", Status: "queued"}}, forge.ChecksPending},
		{"failing beats pending", []apiCheckRun{done("timed_out"), {Name: "x", Status: "queued"}}, forge.ChecksFailing},
		{"cancelled fails", []apiCheckRun{done("cancelled")}, forge.ChecksFailing},
	}
	for _, tc := range cases {
		if got, _ := summarise(tc.runs); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
