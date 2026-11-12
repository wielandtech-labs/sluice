// SPDX-License-Identifier: AGPL-3.0-only

package board

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/wielandtech-labs/sluice/pkg/forge"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var funcs = template.FuncMap{
	"ago": func(t time.Time) string { return ago(time.Since(t)) },
	"failing": func(prs []forge.PullRequest) int {
		n := 0
		for _, pr := range prs {
			if pr.Checks == forge.ChecksFailing {
				n++
			}
		}
		return n
	},
}

var pages = map[string]*template.Template{}

func init() {
	for _, name := range []string{"index", "project", "inbox"} {
		pages[name] = template.Must(template.New("layout.html").Funcs(funcs).
			ParseFS(templateFS, "templates/layout.html", "templates/"+name+".html"))
	}
}

// ago formats a duration as a short relative time.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

type projectRow struct {
	Project
	NeedsHuman int
}

type view struct {
	Title      string
	InboxCount int
	Projects   []projectRow
	Project    Project
	Inbox      []InboxItem
}

// Handler returns the board's HTTP handler. Every route is read-only.
func (b *Board) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.FileServerFS(staticFS))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		v := view{Title: "Projects"}
		for _, p := range b.Projects() {
			v.Projects = append(v.Projects, projectRow{p, len(inboxFor(p, b.humanLabel))})
		}
		b.render(w, "index", v)
	})
	mux.HandleFunc("GET /inbox", func(w http.ResponseWriter, _ *http.Request) {
		b.render(w, "inbox", view{Title: "Needs human"})
	})
	mux.HandleFunc("GET /p/{owner}/{repo}", func(w http.ResponseWriter, r *http.Request) {
		p, ok := b.Project(r.PathValue("owner") + "/" + r.PathValue("repo"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		b.render(w, "project", view{Title: p.Repo, Project: p})
	})
	return securityHeaders(mux)
}

func (b *Board) render(w http.ResponseWriter, page string, v view) {
	v.Inbox = b.Inbox()
	v.InboxCount = len(v.Inbox)
	var buf bytes.Buffer
	if err := pages[page].Execute(&buf, v); err != nil {
		slog.Error("render failed", "page", page, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	buf.WriteTo(w)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; style-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}
