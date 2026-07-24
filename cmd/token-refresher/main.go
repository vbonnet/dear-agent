// Command token-refresher keeps the Claude Code OAuth credentials file fresh so
// the VROOM supervisor mesh (three Claude Code sessions in separate tmux panes,
// sharing one ~/.claude/.credentials.json) survives access-token expiry without
// a human re-running /login.
//
// It is the single-owner, file-locked refresher mandated by ce-rnpt / ce-f3e3:
// the whole read-check-exchange-write cycle runs under a cross-process lock, so
// two panes never each spend the single-use refresh token and poison the family.
//
// That lock only binds callers of THIS binary. Since apiKeyHelper was removed on
// 2026-07-10 (Claude Code >=2.1.205 lets a configured helper shadow healthy
// OAuth -- anthropics/claude-code#11587), the Claude Code runtimes on the host
// refresh on their own and take no lock, so "single-owner" no longer describes
// reality. Every audit line therefore carries a fingerprint of the refresh token
// it was about to present (fingerprint.go), which is what lets a post-mortem
// name the client that actually spent the token. See ce-77ip.
//
// Modes:
//
//	token-refresher              # ensure fresh, print the access token to stdout
//	token-refresher -check       # report status only, no network, no mutation
//	token-refresher -force       # refresh even if the current token is fresh
//	token-refresher -cadence     # unattended launchd mode (see cadence.go)
//
// The default (print) mode emits ONLY the access token on stdout (all logs go
// to stderr) so it composes with any caller that wants to capture the token.
//
// Do NOT wire this binary in as a Claude Code `apiKeyHelper`. That was its
// original purpose and it is retired: since claude-code 2.1.205 a configured
// helper is treated as an external API key that shadows a healthy claude.ai
// OAuth login and refuses to fall back to it, so the CLI fails with "Invalid
// API key" even when the credentials file is fresh (anthropics/claude-code
// #11587, #9694, #23568). It was removed from the host on 2026-07-10 after
// causing a multi-day mesh outage. The launchd idle backstop is the only
// sanctioned wiring -- see cmd/token-refresher/README.md ("Retired wiring").
//
// Exit codes:
//
//	0  success
//	1  generic / usage error
//	2  token family dead (invalid_grant) — re-authentication required
//	3  refresh succeeded on the server but could not be persisted (critical)
//	4  refresh token quarantined — an earlier refresh may have spent it
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
	exitQuarantined     = 4

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
		cadence     = fs.Bool("cadence", false, "unattended launchd mode: alert on token-family death and always exit 0 so launchd keeps the schedule")
		lockTimeout = fs.Duration("lock-timeout", 0, "max wait for the cross-process credentials lock (default 10s)")
		auditPath   = fs.String("audit-log", defaultAuditPath(), "JSONL audit log path (empty to disable)")
		quarPath    = fs.String("quarantine", defaultQuarantinePath(), "refresh-token quarantine marker path (empty to disable quarantine)")
		clearQuar   = fs.Bool("clear-quarantine", false, "clear the refresh-token quarantine and exit (operator override)")
	)
	if err := fs.Parse(args); err != nil {
		return exitError
	}
	quarantineExplicit := false
	fs.Visit(func(f *flag.Flag) {
		quarantineExplicit = quarantineExplicit || f.Name == "quarantine"
	})
	if !quarantineExplicit {
		*quarPath = defaultQuarantinePathForCredentials(*credPath)
	}
	if *check && *clearQuar {
		fmt.Fprintln(stderr, "token-refresher: -check and -clear-quarantine are mutually exclusive")
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
		QuarantinePath:  *quarPath,
		Logger:          logger,
		HTTPClient:      &http.Client{Timeout: httpTimeout},
	}

	if *clearQuar {
		if err := r.ClearQuarantine(); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not clear quarantine: %v\n", err)
			return exitError
		}
		if err := clearCadenceSentinel(defaultStateDir(), cadenceSentinelName(*quarPath)); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not re-arm cadence alert: %v\n", err)
			return exitError
		}
		if err := clearCadenceStop(*credPath); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not re-arm cadence refresh: %v\n", err)
			return exitError
		}
		fmt.Fprintln(stderr, "token-refresher: refresh-token quarantine cleared; automatic refresh re-armed.")
		writeAudit(*auditPath, auditRecord{Mode: "clear-quarantine", Outcome: "ok"})
		return exitOK
	}

	// Fingerprint the on-disk refresh token BEFORE doing anything, so the audit
	// line records the token this tick was about to present. See fingerprint.go.
	fp, credMod := credentialsFingerprint(*credPath)

	// finish adapts the exit code for the unattended cadence caller. See
	// cadence.go: it alerts on a dead family and keeps launchd's schedule alive.
	finish := func(code int) int {
		if *cadence {
			if code == exitNotPersisted {
				if err := writeCadenceStop(*credPath); err != nil {
					fmt.Fprintf(stderr, "token-refresher: CRITICAL — could not persist cadence stop: %v\n", err)
					return exitNotPersisted
				}
				fmt.Fprintln(stderr, "token-refresher: cadence refresh STOPPED until -clear-quarantine re-arms it.")
				return exitOK
			}
			return cadenceExit(code, defaultStateDir(), cadenceSentinelName(*quarPath), stderr)
		}
		return code
	}

	if *check {
		st := r.Status()
		span.SetAttributes(
			attribute.String("mode", "check"),
			attribute.Bool("fresh", st.Fresh),
		)
		printStatus(stderr, st)
		// A marker naming a token that is no longer on disk holds nothing back,
		// so it must not be reported as if it did — that would send the operator
		// to -clear-quarantine for no reason. See CTR-13.
		if qfp, qat, qreason, active := r.QuarantineStatus(); active {
			fmt.Fprintf(stderr, "token-refresher: QUARANTINED refresh token %s since %s (%s)\n"+
				"  Automatic refresh is held back. Clear with: token-refresher -clear-quarantine\n", qfp, qat, qreason)
		} else if qfp != "" {
			fmt.Fprintf(stderr, "token-refresher: stale quarantine marker for %s (token has since rotated); "+
				"it is inert and the next refresh will remove it.\n", qfp)
		}
		writeAudit(*auditPath, auditRecord{
			Mode: "check", Outcome: "ok", Fresh: st.Fresh, ExpiresAt: msOrZero(st.ExpiresAt),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitOK
	}
	if *cadence {
		stopped, stopErr := cadenceStopped(*credPath)
		if stopErr != nil {
			fmt.Fprintf(stderr, "token-refresher: could not inspect cadence stop: %v\n", stopErr)
			return exitNotPersisted
		}
		if stopped {
			fmt.Fprintln(stderr, "token-refresher: cadence refresh is STOPPED after an unpersisted quarantine; run -clear-quarantine after remediation.")
			return exitOK
		}
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
		return finish(handleRefreshError(err, mode, *auditPath, stderr, fp, credMod, *quarPath))
	}

	span.SetAttributes(
		attribute.String("mode", mode),
		attribute.Bool("refreshed", refreshed),
		attribute.String("outcome", "ok"),
	)
	writeAudit(*auditPath, auditRecord{
		Mode: mode, Outcome: "ok", Refreshed: refreshed,
		RefreshTokenFP: fp, CredentialsModTime: credMod,
	})

	// stdout-only token contract: stdout carries ONLY the token.
	fmt.Fprintln(stdout, token)
	return finish(exitOK)
}

// handleRefreshError maps a refresh error to a clear stderr message, an audit
// record, and an exit code. The typed errors get distinct codes so a wrapper
// (or operator) can escalate the unrecoverable ones.
func handleRefreshError(err error, mode, auditPath string, stderr io.Writer, fp, credMod, quarPath string) int {
	switch {
	case errors.Is(err, auth.ErrTokenFamilyDead):
		fmt.Fprintf(stderr, "token-refresher: refresh token rejected (invalid_grant). The token family is dead.\n"+
			"  Re-authenticate on the host with `claude /login` or `claude setup-token`, then restart the mesh.\n"+
			"  Spent refresh token fingerprint: %s (credentials mtime %s).\n"+
			"  Compare against the preceding audit lines: if the fingerprint changed since our last\n"+
			"  successful refresh, another OAuth client on this host rotated the token. If it did not,\n"+
			"  the token was spent by a client that never wrote back to this credentials file.\n", fpOrUnknown(fp), credModOrUnknown(credMod))
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "token_family_dead", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitTokenFamilyDead
	case errors.Is(err, auth.ErrQuarantineNotPersisted):
		fmt.Fprintf(stderr, "token-refresher: CRITICAL — the refresh token may be spent and the quarantine marker could NOT be written.\n"+
			"  Nothing will stop the next tick from presenting it again, which would revoke the whole token family.\n"+
			"  Fix the state directory (%s) now, or re-authenticate with `claude /login` to retire the token.\n  cause: %v\n",
			quarPath, err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "quarantine_not_persisted", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitNotPersisted

	case errors.Is(err, auth.ErrQuarantineUnreadable):
		fmt.Fprintf(stderr, "token-refresher: refresh SKIPPED — a quarantine marker exists but could not be read.\n"+
			"  It may name the token on disk, so presenting it could revoke the token family. Failing closed.\n"+
			"  Inspect %s, then either repair it or run: token-refresher -clear-quarantine\n  cause: %v\n",
			quarPath, err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "quarantine_unreadable", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitQuarantined

	case errors.Is(err, auth.ErrRefreshOutcomeUnknown):
		if quarPath == "" {
			fmt.Fprintf(stderr, "token-refresher: refresh outcome UNKNOWN — the request may have reached the server but the response did not.\n"+
				"  Quarantine is DISABLED, so the next tick may re-present this possibly spent token.\n"+
				"  Re-run with a quarantine path or re-authenticate with `claude /login` before retrying.\n  cause: %v\n", err)
			writeAudit(auditPath, auditRecord{
				Mode: mode, Outcome: "refresh_outcome_unknown_unquarantined", Error: err.Error(),
				RefreshTokenFP: fp, CredentialsModTime: credMod,
			})
			return exitNotPersisted
		}
		fmt.Fprintf(stderr, "token-refresher: refresh outcome UNKNOWN — the request reached the server but the response did not.\n"+
			"  The refresh token may already be spent. It has been quarantined (fingerprint %s) so the next tick will NOT\n"+
			"  present it again; replaying a spent token is what revokes the whole token family.\n"+
			"  If the access token expires before another client refreshes, re-authenticate with `claude /login`.\n"+
			"  To override and retry anyway: token-refresher -clear-quarantine\n  cause: %v\n", fpOrUnknown(fp), err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "refresh_outcome_unknown", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitQuarantined

	case errors.Is(err, auth.ErrRefreshQuarantined):
		fmt.Fprintf(stderr, "token-refresher: refresh SKIPPED — the on-disk refresh token is quarantined.\n"+
			"  An earlier refresh may have spent it, so presenting it again risks killing the token family.\n"+
			"  This clears itself as soon as any client rotates the token successfully.\n"+
			"  To override: token-refresher -clear-quarantine\n  detail: %v\n", err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "refresh_quarantined", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitQuarantined

	case errors.Is(err, auth.ErrRefreshNotPersisted):
		fmt.Fprintf(stderr, "token-refresher: CRITICAL — refresh succeeded but new credentials could not be written.\n"+
			"  The rotated refresh token is not on disk; the next refresh may fail. Investigate disk/permissions now.\n  cause: %v\n", err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "not_persisted", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitNotPersisted
	default:
		fmt.Fprintf(stderr, "token-refresher: %v\n", err)
		writeAudit(auditPath, auditRecord{
			Mode: mode, Outcome: "error", Error: err.Error(),
			RefreshTokenFP: fp, CredentialsModTime: credMod,
		})
		return exitError
	}
}

// fpOrUnknown and credModOrUnknown keep the operator-facing death message
// readable when the credentials file could not be read at all.
func fpOrUnknown(fp string) string {
	if fp == "" {
		return "unknown"
	}
	return fp
}

func credModOrUnknown(m string) string {
	if m == "" {
		return "unknown"
	}
	return m
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

	// RefreshTokenFP is a short SHA-256 prefix of the refresh token that was on
	// disk when this tick STARTED, and CredentialsModTime is that file's mtime.
	// Comparing them across consecutive ticks reveals whether a third-party
	// OAuth client rotated the token behind our back -- the question the logs
	// could not answer during the ce-77ip family-death investigation. Neither
	// field is reversible to the token itself.
	RefreshTokenFP     string `json:"refresh_token_fp,omitempty"`
	CredentialsModTime string `json:"credentials_mtime,omitempty"`
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

// defaultQuarantinePath puts the quarantine marker alongside the audit log and
// the death sentinel, so all refresher state lives in one directory.
func defaultQuarantinePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "state", "dear-agent", "refresh-token-quarantine.json")
}

// defaultQuarantinePathForCredentials keeps separate credentials files from
// clearing one another's quarantine marker. An explicitly supplied
// -quarantine path remains an operator-controlled shared marker when needed.
func defaultQuarantinePathForCredentials(credentialsPath string) string {
	credentialsPath = canonicalCredentialsPath(credentialsPath)
	if credentialsPath == canonicalCredentialsPath(defaultCredentialsPath()) {
		return defaultQuarantinePath()
	}
	return credentialsPath + ".refresh-quarantine.json"
}

func defaultCredentialsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", ".credentials.json")
}

func canonicalCredentialsPath(path string) string {
	if path == "" {
		return defaultCredentialsPath()
	}
	if absolute, err := filepath.Abs(path); err == nil {
		path = absolute
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return filepath.Clean(resolved)
	}
	return path
}

func msOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}
