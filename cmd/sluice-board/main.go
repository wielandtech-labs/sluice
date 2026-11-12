// SPDX-License-Identifier: AGPL-3.0-only

// Command sluice-board serves the read-only Sluice project board.
//
//	SLUICE_GITHUB_TOKEN=... sluice-board -repos owner/a,owner/b
//
// The token needs read access to the listed repositories and may be left
// unset for public ones.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/wielandtech-labs/sluice/internal/board"
	"github.com/wielandtech-labs/sluice/internal/forge/github"
)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func main() {
	listen := flag.String("listen", "127.0.0.1:8080", "address to listen on")
	repoList := flag.String("repos", os.Getenv("SLUICE_REPOS"), "comma-separated owner/name repositories (or SLUICE_REPOS)")
	refresh := flag.Duration("refresh", 2*time.Minute, "how often to re-read the forge")
	humanLabel := flag.String("human-label", "needs-human", "issue label that puts an issue in the inbox")
	flag.Parse()

	var repos []string
	for _, r := range strings.Split(*repoList, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if !repoPattern.MatchString(r) {
			slog.Error("repository must be owner/name", "repo", r)
			os.Exit(2)
		}
		repos = append(repos, r)
	}
	if len(repos) == 0 {
		slog.Error("no repositories given; use -repos or SLUICE_REPOS")
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	b := board.New(github.New(os.Getenv("SLUICE_GITHUB_TOKEN")), repos, *humanLabel)
	go b.Run(ctx, *refresh)

	srv := &http.Server{
		Addr:              *listen,
		Handler:           b.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()

	slog.Info("sluice-board listening", "addr", *listen, "repos", len(repos))
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}
