// Command token-refresher keeps the Claude Code OAuth credentials file fresh so
// the VROOM supervisor mesh (three Claude Code sessions in separate tmux panes,
// sharing one ~/.claude/.credentials.json) survives access-token expiry without
// a human re-running /login.
//
// It is the single-owner, file-locked refresher mandated by ce-rnpt / ce-f3e3:
// the whole read-check-exchange-write cycle runs under a cross-process lock, so
// two panes never each spend the single-use refresh token and poison the family.
//
// Modes:
//
//	token-refresher              # ensure fresh, print the access token to stdout
//	token-refresher -check       # report status only, no network, no mutation
//	token-refresher -force       # refresh even if the current token is fresh
//
// The default (print) mode is designed for use as a Claude Code `apiKeyHelper`:
// it emits ONLY the access token on stdout (all logs go to stderr), so the CLI
// can drive the refresh cadence (every CLAUDE_CODE_API_KEY_HELPER_TTL_MS, or on
// HTTP 401).
//
// Exit codes:
//
//	0  success
//	1  generic / usage error
//	2  token family dead (invalid_grant) — re-authentication required
//	3  refresh succeeded on the server but could not be persisted (critical)
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

const (
	exitOK              = 0
	exitError           = 1
	exitTokenFamilyDead = 2
	exitNotPersisted    = 3

	httpTimeout = 30 * time.Second
	// forceSkew, applied in -force mode, makes any on-disk token read as stale
	// so a refresh is always attempted.
	forceSkew = 100 * 365 * 24 * time.Hour
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("token-refresher", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		check       = fs.Bool("check", false, "report credential status only; no network call or mutation")
		force       = fs.Bool("force", false, "refresh even if the current token is still fresh")
		quiet       = fs.Bool("quiet", false, "suppress structured stderr logs (stdout still gets the token)")
		credPath    = fs.String("credentials", "", "path to credentials.json (default ~/.claude/.credentials.json)")
		endpoint    = fs.String("endpoint", "", "OAuth token endpoint override (default built-in / $CLAUDE_OAUTH_TOKEN_ENDPOINT)")
		clientID    = fs.String("client-id", "", "OAuth client ID override (default built-in / $CLAUDE_OAUTH_CLIENT_ID)")
		lockTimeout = fs.Duration("lock-timeout", 0, "max wait for the cross-process credentials lock (default 10s)")
		auditPath   = fs.String("audit-log", defaultAuditPath(), "JSONL audit log path (empty to disable)")
	)
	if err := fs.Parse(args); err != nil {
		return exitError
	}

	var logger *slog.Logger
	if !*quiet {
		logger = slog.New(slog.NewJSONHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}

	shutdown := otelsetup.InitTracer("token-refresher")
	defer func() {
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := shutdown(sctx); err != nil && logger != nil {
			logger.Warn("otel tracer shutdown failed", "error", err.Error())
		}
	}()
	ctx, span := otel.Tracer("token-refresher").Start(context.Background(), "token-refresher.run")
	defer span.End()

	r := auth.OAuthResolver{
		CredentialsPath: *credPath,
		TokenEndpoint:   *endpoint,
		ClientID:        *clientID,
		LockTimeout:     *lockTimeout,
		Logger:          logger,
		HTTPClient:      &http.Client{Timeout: httpTimeout},
	}

	if *check {
		st := r.Status()
		span.SetAttributes(
			attribute.String("mode", "check"),
			attribute.Bool("fresh", st.Fresh),
		)
		printStatus(stderr, st)
		writeAudit(*auditPath, auditRecord{Mode: "check", Outcome: "ok", Fresh: st.Fresh, ExpiresAt: msOrZero(st.ExpiresAt)})
		return exitOK
	}

	if *force {
		r.ExpirySkew = forceSkew
		span.SetAttributes(attribute.Bool("forced", true))
	}

	token, refreshed, err := r.EnsureFresh(ctx)
	mode := "ensure"
	if *force {
		mode = "force"
	}
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return handleRefreshError(err, mode, *auditPath, stderr)
	}

	span.SetAttributes(
		attribute.String("mode", mode),
		attribute.Bool("refreshed", refreshed),
		attribute.String("outcome", "ok"),
	)
	writeAudit(*auditPath, auditRecord{Mode: mode, Outcome: "ok", Refreshed: refreshed})

	// apiKeyHelper contract: stdout carries ONLY the token.
	fmt.Fprintln(stdout, token)
	return exitOK
}

// handleRefreshError maps a refresh error to a clear stderr message, an audit
// record, and an exit code. The typed errors get distinct codes so a wrapper
// (or operator) can escalate the unrecoverable ones.
func handleRefreshError(err error, mode, auditPath string, stderr io.Writer) int {
	switch {
	case errors.Is(err, auth.ErrTokenFamilyDead):
		fmt.Fprintf(stderr, "token-refresher: refresh token rejected (invalid_grant). The token family is dead.\n"+
			"  Re-authenticate on the host with `claude /login` or `claude setup-token`, then restart the mesh.\n")
		writeAudit(auditPath, auditRecord{Mode: mode, Outcome: "token_family_dead", Error: err.Error()})
		return exitTokenFamilyDead
	case errors.Is(err, auth.ErrRefreshNotPersisted):
		fmt.Fprintf(stderr, "token-refresher: CRITICAL — refresh succeeded but new credentials could not be written.\n"+
			"  The rotated refresh token is not on disk; the next refresh may fail. Investigate disk/permissions now.\n  cause: %v\n", err)
		writeAudit(auditPath, auditRecord{Mode: mode, Outcome: "not_persisted", Error: err.Error()})
		return exitNotPersisted
	default:
		fmt.Fprintf(stderr, "token-refresher: %v\n", err)
		writeAudit(auditPath, auditRecord{Mode: mode, Outcome: "error", Error: err.Error()})
		return exitError
	}
}

func printStatus(w io.Writer, st auth.TokenStatus) {
	var state string
	switch {
	case !st.HasToken:
		state = "no access token on disk"
	case st.Fresh:
		state = "FRESH"
	default:
		state = "STALE (needs refresh)"
	}
	fmt.Fprintf(w, "token-refresher: status=%s has_refresh_token=%t", state, st.HasRefreshToken)
	if !st.ExpiresAt.IsZero() {
		fmt.Fprintf(w, " expires_at=%s (%s)", st.ExpiresAt.UTC().Format(time.RFC3339), humanizeUntil(st.ExpiresAt))
	}
	fmt.Fprintln(w)
}

func humanizeUntil(t time.Time) string {
	d := time.Until(t).Round(time.Second)
	if d < 0 {
		return "expired " + (-d).String() + " ago"
	}
	return "in " + d.String()
}

// auditRecord is one line of the JSONL audit log. It deliberately holds no
// token values — only metadata an operator needs to debug a refresh.
type auditRecord struct {
	Timestamp string `json:"timestamp"`
	Mode      string `json:"mode"`
	Outcome   string `json:"outcome"`
	Refreshed bool   `json:"refreshed,omitempty"`
	Fresh     bool   `json:"fresh,omitempty"`
	ExpiresAt int64  `json:"expires_at_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

func writeAudit(path string, rec auditRecord) {
	if path == "" {
		return
	}
	rec.Timestamp = time.Now().UTC().Format(time.RFC3339)
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	if _, werr := f.Write(append(data, '\n')); werr != nil {
		_ = f.Close()
		return
	}
	// Check the close error: for an appended writable file a failed close can
	// mean a lost record (CodeQL: writable handle closed without handling).
	if cerr := f.Close(); cerr != nil {
		return
	}
}

func defaultAuditPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "dear-agent", "token-refresher-audit.jsonl")
}

func msOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
