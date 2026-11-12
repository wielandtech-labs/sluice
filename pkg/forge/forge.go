// SPDX-License-Identifier: Apache-2.0

// Package forge defines the read-only view of a code forge that Sluice needs.
// Implementations live under internal/forge.
package forge

import (
	"context"
	"time"
)

// CheckState summarises all check runs and commit statuses on a commit.
type CheckState string

const (
	ChecksPassing CheckState = "passing"
	ChecksFailing CheckState = "failing"
	ChecksPending CheckState = "pending"
	ChecksNone    CheckState = "none"
	// ChecksUnknown means the forge could not be asked.
	ChecksUnknown CheckState = "unknown"
)

// ReviewState summarises the reviews of a pull request's head commit by
// people with write access, taking each reviewer's latest approval or change
// request. Reviews of earlier commits do not count.
type ReviewState string

const (
	ReviewNone             ReviewState = "none"
	ReviewApproved         ReviewState = "approved"
	ReviewChangesRequested ReviewState = "changes requested"
	// ReviewUnknown means the forge could not be asked.
	ReviewUnknown ReviewState = "unknown"
)

// PullRequest is an open pull request and the check state of its head commit.
type PullRequest struct {
	Number        int
	Title         string
	URL           string
	Author        string
	HeadSHA       string
	Draft         bool
	UpdatedAt     time.Time
	Checks        CheckState
	FailingChecks []string
	Review        ReviewState
}

// Issue is an open issue. Pull requests are never returned as issues.
type Issue struct {
	Number    int
	Title     string
	URL       string
	Author    string
	Labels    []string
	UpdatedAt time.Time
}

// Reader reads project state from a forge. repo is "owner/name".
type Reader interface {
	OpenPullRequests(ctx context.Context, repo string) ([]PullRequest, error)
	OpenIssues(ctx context.Context, repo string) ([]Issue, error)
}
