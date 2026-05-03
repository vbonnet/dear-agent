// Command workflow-inspector serves a read-only HTML view of the
// runs.db that Phase 0 introduced. Renders a list of runs, drill-down
// per run, and per-run audit timeline. No authoring, no controls —
// the substrate's existing CLIs (workflow-approve, workflow-cancel)
// own state changes; this is the "what's happening" pane.
//
// Phase 5.5 — see ROADMAP. The intent is "the CLI is good enough but
// a UI helps a non-engineer triage; ship it cheaply on top of the
// existing query helpers".
//
// Usage:
//
//	workflow-inspector --db ./runs.db [--addr :8080]
//
// Exit codes: 0 on clean shutdown (SIGINT/SIGTERM), 1 on startup error.
//
// Security notes: the server binds to the addr passed by the operator;
// the default :8080 is loopback-only. There is no authentication —
// run behind a reverse proxy if exposing beyond localhost. All
// responses are derived from runs.db only; no other state is read.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("workflow-inspector", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		dbPath = fs.String("db", "runs.db", "path to runs.db")
		addr   = fs.String("addr", "127.0.0.1:8080", "listen address (default loopback only)")
	)
	fs.Usage = func() {
		fmt.Fprintln(stderr, "Usage: workflow-inspector [--db ./runs.db] [--addr :8080]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	db, err := sql.Open("sqlite", *dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	defer db.Close()

	srv := NewServer(db)
	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpSrv.Shutdown(shutdownCtx)
	}()

	fmt.Fprintf(stdout, "workflow-inspector: serving %s on http://%s\n", *dbPath, *addr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(stderr, err)
		return 1
	}
	wg.Wait()
	return 0
}

// Server is the HTTP handler. Construct with NewServer; pass to
// http.ListenAndServe or any test transport.
type Server struct {
	db *sql.DB
}

// NewServer constructs a handler over an open *sql.DB. The DB is
// expected to be a runs.db with the Phase 0 schema; queries that
// fail because the schema is absent surface as 500s with the SQL
// error in the response body.
func NewServer(db *sql.DB) *Server {
	return &Server{db: db}
}

// ServeHTTP routes:
//
//	GET /            → run list
//	GET /run/<id>    → run detail (nodes + audit timeline)
//	GET /healthz     → 200 if the DB is reachable
func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	switch {
	case req.URL.Path == "/" || req.URL.Path == "/runs":
		s.handleList(w, req)
	case req.URL.Path == "/healthz":
		s.handleHealth(w, req)
	case strings.HasPrefix(req.URL.Path, "/run/"):
		runID := strings.TrimPrefix(req.URL.Path, "/run/")
		if runID == "" {
			http.NotFound(w, req)
			return
		}
		s.handleRun(w, req, runID)
	default:
		http.NotFound(w, req)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, req *http.Request) {
	if err := s.db.PingContext(req.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

func (s *Server) handleList(w http.ResponseWriter, req *http.Request) {
	stateFilter := workflow.RunState(req.URL.Query().Get("state"))
	runs, err := workflow.List(req.Context(), s.db, workflow.ListOptions{State: stateFilter, Limit: 100})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := listTmpl.Execute(w, listView{Runs: runs, StateFilter: string(stateFilter)}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, req *http.Request, runID string) {
	st, err := workflow.Status(req.Context(), s.db, runID)
	if err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			http.NotFound(w, req)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := workflow.Logs(req.Context(), s.db, runID, workflow.LogsOptions{Limit: 200})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := runTmpl.Execute(w, runView{Run: st, Events: events}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// --- view models ---

type listView struct {
	Runs        []workflow.RunSummary
	StateFilter string
}

type runView struct {
	Run    *workflow.RunStatus
	Events []workflow.AuditEvent
}

// --- templates ---

var baseStyles = `
<style>
  body { font: 14px/1.5 -apple-system, BlinkMacSystemFont, sans-serif; max-width: 980px; margin: 1.5rem auto; padding: 0 1rem; color: #222; }
  h1 { font-size: 18px; margin-bottom: 0.5rem; }
  table { border-collapse: collapse; width: 100%; margin: 0.5rem 0 1.5rem; }
  th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #eee; vertical-align: top; }
  th { font-weight: 600; background: #fafafa; }
  td.code, td.mono, .mono { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 12px; }
  a { color: #0366d6; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .state { display: inline-block; padding: 1px 8px; border-radius: 4px; font-size: 12px; }
  .state-running { background: #fff5b1; }
  .state-succeeded { background: #d4edda; }
  .state-failed { background: #f8d7da; }
  .state-cancelled, .state-skipped { background: #e2e3e5; color: #666; }
  .state-awaiting_hitl { background: #d1ecf1; }
  .state-pending { background: #f0f0f0; color: #666; }
  nav { font-size: 12px; margin-bottom: 1rem; color: #888; }
  nav a { margin-right: 0.75rem; }
  pre { background: #f6f8fa; padding: 8px; overflow-x: auto; font-size: 12px; }
  small { color: #666; }
</style>
`

var listTmpl = template.Must(template.New("list").Funcs(template.FuncMap{
	"fmtTime": fmtTime,
}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>workflow-inspector — runs</title>` + baseStyles + `</head>
<body>
<nav>
  <a href="/">all</a>
  <a href="/?state=running">running</a>
  <a href="/?state=succeeded">succeeded</a>
  <a href="/?state=failed">failed</a>
  <a href="/?state=awaiting_hitl">awaiting_hitl</a>
</nav>
<h1>Runs{{if .StateFilter}} — {{.StateFilter}}{{end}}</h1>
{{if not .Runs}}
  <p><small>No runs match.</small></p>
{{else}}
<table>
  <thead><tr><th>Run</th><th>Workflow</th><th>State</th><th>Started</th><th>Finished</th><th>Trigger</th></tr></thead>
  <tbody>
  {{range .Runs}}
    <tr>
      <td class="mono"><a href="/run/{{.RunID}}">{{.RunID}}</a></td>
      <td>{{.Workflow}}</td>
      <td><span class="state state-{{.State}}">{{.State}}</span></td>
      <td>{{fmtTime .StartedAt}}</td>
      <td>{{if .FinishedAt}}{{fmtTime .FinishedAt}}{{else}}—{{end}}</td>
      <td>{{.Trigger}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{end}}
</body></html>
`))

var runTmpl = template.Must(template.New("run").Funcs(template.FuncMap{
	"fmtTime": fmtTime,
}).Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>workflow-inspector — run</title>` + baseStyles + `</head>
<body>
<nav><a href="/">← all runs</a></nav>
<h1>{{.Run.Workflow}} <small class="mono">{{.Run.RunID}}</small></h1>
<table>
  <tr><th>State</th><td><span class="state state-{{.Run.State}}">{{.Run.State}}</span></td></tr>
  <tr><th>Started</th><td>{{fmtTime .Run.StartedAt}}</td></tr>
  <tr><th>Finished</th><td>{{if .Run.FinishedAt}}{{fmtTime .Run.FinishedAt}}{{else}}—{{end}}</td></tr>
  <tr><th>Tokens</th><td>{{.Run.TotalTokens}}</td></tr>
  <tr><th>Dollars</th><td>${{printf "%.4f" .Run.TotalDollars}}</td></tr>
  {{if .Run.Error}}<tr><th>Error</th><td><pre>{{.Run.Error}}</pre></td></tr>{{end}}
</table>

<h1>Nodes</h1>
<table>
  <thead><tr><th>Node</th><th>State</th><th>Attempts</th><th>Tokens</th><th>$</th><th>Started</th><th>Finished</th></tr></thead>
  <tbody>
  {{range .Run.Nodes}}
    <tr>
      <td class="mono">{{.NodeID}}</td>
      <td><span class="state state-{{.State}}">{{.State}}</span></td>
      <td>{{.Attempts}}</td>
      <td>{{.TokensUsed}}</td>
      <td>${{printf "%.4f" .DollarsSpent}}</td>
      <td>{{if .StartedAt}}{{fmtTime .StartedAt}}{{else}}—{{end}}</td>
      <td>{{if .FinishedAt}}{{fmtTime .FinishedAt}}{{else}}—{{end}}</td>
    </tr>
  {{end}}
  </tbody>
</table>

<h1>Audit timeline</h1>
{{if not .Events}}
  <p><small>No events recorded.</small></p>
{{else}}
<table>
  <thead><tr><th>When</th><th>Node</th><th>Transition</th><th>Reason</th><th>Actor</th></tr></thead>
  <tbody>
  {{range .Events}}
    <tr>
      <td class="mono">{{fmtTime .OccurredAt}}</td>
      <td class="mono">{{.NodeID}}</td>
      <td class="mono">{{.FromState}} → {{.ToState}}</td>
      <td>{{.Reason}}</td>
      <td>{{.Actor}}</td>
    </tr>
  {{end}}
  </tbody>
</table>
{{end}}
</body></html>
`))

// fmtTime renders a time.Time (or *time.Time) for the templates.
// Accepts both so the templates can pass the field directly.
func fmtTime(v any) string {
	switch t := v.(type) {
	case time.Time:
		if t.IsZero() {
			return "—"
		}
		return t.UTC().Format("2006-01-02 15:04:05Z")
	case *time.Time:
		if t == nil || t.IsZero() {
			return "—"
		}
		return t.UTC().Format("2006-01-02 15:04:05Z")
	default:
		return fmt.Sprintf("%v", v)
	}
}
