// Command credential-monitor performs a no-network freshness check on the
// shared Claude Code OAuth credentials file and reports whether the access
// token is fresh, expiring soon, expired, or missing. It is the credential
// half of the VROOM Overseer's monitoring loop — the sibling of fd-pressure —
// and exists so the mesh can 401-avoid BEFORE the token actually dies.
//
// Unlike token-refresher, it never touches the network and never mutates the
// credentials file: it only observes. That means the signal is visible even
// when the token is already dead, with no manual /login required (bead
// ce-77ip.5).
//
// Usage:
//
//	credential-monitor                                  # human-readable status
//	credential-monitor --json                           # JSON status object
//	credential-monitor --stale-within 15m               # override the 10m window
//	credential-monitor --credentials /path/to/creds.json
//	credential-monitor --trail ~/.agm/vroom/trail.jsonl # append alert when stale
//
// When --trail is set AND the credential is stale (expired or expiring within
// the window), one "supervisor.credential.stale" record is appended to the
// named decision trail. The trail write is best-effort: a failure is reported
// on stderr but never changes the exit code, so an Overseer tick keeps going
// even when the trail is unwritable.
//
// Exit codes (matching CredentialState):
//
//	0  fresh
//	1  expiring within the stale window
//	2  expired
//	3  no access token on disk (missing)
//	4  usage / flag error
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

// exitUsage is distinct from the 0-3 CredentialState exit codes so a caller can
// tell "bad flags" apart from "credential is missing".
const exitUsage = 4

func main() {
	code, err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "credential-monitor:", err)
		os.Exit(exitUsage)
	}
	os.Exit(code)
}

type config struct {
	jsonOutput  bool
	credPath    string
	trailPath   string
	staleWithin time.Duration
}

func run(args []string, stdout, stderr io.Writer) (int, error) {
	fs := flag.NewFlagSet("credential-monitor", flag.ContinueOnError)
	fs.SetOutput(stderr)
	cfg := config{}
	fs.BoolVar(&cfg.jsonOutput, "json", false, "emit JSON instead of a human-readable line")
	fs.StringVar(&cfg.credPath, "credentials", "",
		"path to credentials.json (default ~/.claude/.credentials.json)")
	fs.StringVar(&cfg.trailPath, "trail", "",
		"if set, append a supervisor.credential.stale record to this JSONL trail when stale")
	fs.DurationVar(&cfg.staleWithin, "stale-within", supervisor.DefaultCredentialStaleWindow,
		"treat a token expiring within this window as stale")
	if err := fs.Parse(args); err != nil {
		return exitUsage, err
	}

	res := supervisor.CheckCredentialFreshness(cfg.credPath, time.Now, cfg.staleWithin)

	// Wire the alert into the VROOM decision trail. Best-effort by design: a
	// tick must record-and-continue, so a trail-write failure goes to stderr
	// but never changes the exit code.
	if cfg.trailPath != "" && res.State.Stale() {
		if err := appendStaleRecord(context.Background(), cfg.trailPath, res); err != nil {
			fmt.Fprintln(stderr, "credential-monitor: trail append failed:", err)
		}
	}

	if cfg.jsonOutput {
		if err := emitJSON(stdout, res); err != nil {
			return exitUsage, err
		}
	} else {
		fmt.Fprintf(stdout, "credential-monitor: state=%s %s\n", res.State, res.Note())
	}

	return res.State.ExitCode(), nil
}

func appendStaleRecord(ctx context.Context, path string, res supervisor.CredentialFreshness) error {
	trail, err := decisiontrail.OpenJSONL(path)
	if err != nil {
		return err
	}
	defer trail.Close()
	return supervisor.EmitCredentialStale(ctx, trail, res)
}

func emitJSON(w io.Writer, res supervisor.CredentialFreshness) error {
	payload := map[string]any{
		"state":             res.State.String(),
		"stale":             res.State.Stale(),
		"exit_code":         res.State.ExitCode(),
		"has_refresh_token": res.HasRefreshToken,
		"stale_window":      res.StaleWindow.String(),
		"note":              res.Note(),
	}
	if !res.ExpiresAt.IsZero() {
		payload["expires_at"] = res.ExpiresAt.UTC().Format(time.RFC3339)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}
