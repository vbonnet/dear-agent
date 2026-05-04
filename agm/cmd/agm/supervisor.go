package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// supervisorStateDir returns the per-supervisor state directory under
// $HOME/.agm/supervisors/. Creates it if missing. Heartbeat files and
// future mesh state live here.
func supervisorStateDir(id string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, ".agm", "supervisors", id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return dir, nil
}

// heartbeatPath returns the absolute path to the supervisor's heartbeat
// file. The file exists iff a heartbeat has been written; its modtime is
// the last beat and its JSON contents carry mesh role info.
func heartbeatPath(id string) (string, error) {
	dir, err := supervisorStateDir(id)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "heartbeat.json"), nil
}

// heartbeatRecord is the JSON shape written by `agm supervisor heartbeat`
// and consumed by `agm supervisor status` and the sentinel loop_monitor.
type heartbeatRecord struct {
	ID           string    `json:"id"`
	PrimaryFor   string    `json:"primary_for,omitempty"`
	TertiaryFor  string    `json:"tertiary_for,omitempty"`
	LastBeatUTC  time.Time `json:"last_beat_utc"`
	PID          int       `json:"pid,omitempty"`
}

// supervisorCmd exposes the agm supervisor subcommand group. Supervisor
// sessions are persistent Claude Code CLI processes that participate in
// the three-way supervisor mesh: they own the dear-agent-costs-style
// cost-reduction properties by using Max-plan OAuth (CLAUDE_CODE_OAUTH_TOKEN)
// instead of a metered API key.
//
// The ToS-safety invariant is that the supervisor process runs the official
// `claude` CLI with OAuth, never the Agent SDK with OAuth. The `run`
// subcommand refuses to start if ANTHROPIC_API_KEY is set in the env,
// which is a belt-and-suspenders guard: a stale API key left in the env
// would cause `claude` to prefer it over the OAuth token and silently bill
// against the metered account.
var supervisorCmd = &cobra.Command{
	Use:   "supervisor",
	Short: "Manage agm supervisor sessions (Max-plan OAuth + agm-bus channel)",
	Long: `Manage agm supervisor sessions for the three-way supervisor mesh.

Supervisors launch a persistent Claude Code CLI session authenticated with
CLAUDE_CODE_OAUTH_TOKEN (Max plan) and load the agm-bus channel so they
can:
  - receive A2A messages from worker sessions
  - relay worker permission prompts (claude/channel/permission) to a peer
    or a human via the Discord adapter
  - emit /loop heartbeats for the liveness mesh

The run subcommand refuses to start if ANTHROPIC_API_KEY is set — a stale
API key left in the env would cause ` + "`claude`" + ` to prefer metered billing
over the Max-plan OAuth token, silently defeating the cost-reduction goal.
That's a documented ToS violation for Agent SDK + OAuth; the CLI path is
explicitly allowed, so we keep this guard in place even though it's a
belt-and-suspenders check.

Subcommands:
  run         Launch a supervisor session (execs ` + "`claude`" + ` with --channels)
  status      Report liveness by heartbeat freshness
  heartbeat   Write a heartbeat timestamp for this supervisor (call from /loop)`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var supervisorRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Launch a supervisor session",
	Long: `Launch a supervisor session.

The chosen supervisor id identifies this process in the mesh. --primary-for
and --tertiary-for wire the liveness graph: this supervisor acts as the
primary responder for one peer and the tertiary backup for another.

Exit conditions:
  - ANTHROPIC_API_KEY is set in env              → refuses, exit 2
  - CLAUDE_CODE_OAUTH_TOKEN is NOT set           → refuses, exit 2
  - ` + "`claude`" + ` binary not found on $PATH            → refuses, exit 2

Examples:
  agm supervisor run --id s1 --primary-for s2 --tertiary-for s3
  agm supervisor run --id s1 --skip-oauth-check   # dev only`,
	RunE: runSupervisorRun,
}

var (
	supervisorID             string
	supervisorPrimaryFor     string
	supervisorTertiaryFor    string
	supervisorSkipOAuthCheck bool
	supervisorClaudeBin      string
	supervisorExtraArgs      []string
)

var supervisorHeartbeatCmd = &cobra.Command{
	Use:   "heartbeat",
	Short: "Write a heartbeat timestamp for this supervisor",
	Long: `Write a heartbeat record to ~/.agm/supervisors/<id>/heartbeat.json.

Intended to be called from a /loop slash command inside the supervisor
session (e.g. /loop 5m agm supervisor heartbeat --id s1). The sentinel
loop_monitor reads the file's mtime to detect stale supervisors and
escalate via the peer mesh.`,
	RunE: runSupervisorHeartbeat,
}

var supervisorStatusCmd = &cobra.Command{
	Use:   "status [id]",
	Short: "Report supervisor liveness by heartbeat age",
	Long: `Print the current heartbeat age for one supervisor, or all known
supervisors if no id is provided. Exits non-zero if any supervisor's
heartbeat is older than --stale-after (default 5m) so this can drive a
monitoring check.`,
	RunE: runSupervisorStatus,
}

var (
	supervisorStatusStaleAfter time.Duration
	supervisorStatusJSON       bool
)

func init() {
	supervisorCmd.AddCommand(supervisorRunCmd)
	supervisorCmd.AddCommand(supervisorHeartbeatCmd)
	supervisorCmd.AddCommand(supervisorStatusCmd)
	rootCmd.AddCommand(supervisorCmd)

	supervisorHeartbeatCmd.Flags().StringVar(&supervisorID, "id", "",
		"supervisor id (reads AGM_SUPERVISOR_ID if unset)")
	supervisorHeartbeatCmd.Flags().StringVar(&supervisorPrimaryFor, "primary-for", "",
		"peer this supervisor is primary responder for (reads AGM_SUPERVISOR_PRIMARY_FOR if unset)")
	supervisorHeartbeatCmd.Flags().StringVar(&supervisorTertiaryFor, "tertiary-for", "",
		"peer this supervisor is tertiary backup for (reads AGM_SUPERVISOR_TERTIARY_FOR if unset)")

	supervisorStatusCmd.Flags().DurationVar(&supervisorStatusStaleAfter, "stale-after", 5*time.Minute,
		"heartbeat age beyond which a supervisor is reported stale")
	supervisorStatusCmd.Flags().BoolVar(&supervisorStatusJSON, "json", false, "emit JSON instead of a table")

	supervisorRunCmd.Flags().StringVar(&supervisorID, "id", "", "supervisor id in the mesh (required)")
	supervisorRunCmd.Flags().StringVar(&supervisorPrimaryFor, "primary-for", "",
		"peer this supervisor is primary responder for")
	supervisorRunCmd.Flags().StringVar(&supervisorTertiaryFor, "tertiary-for", "",
		"peer this supervisor is tertiary backup for")
	supervisorRunCmd.Flags().BoolVar(&supervisorSkipOAuthCheck, "skip-oauth-check", false,
		"skip the CLAUDE_CODE_OAUTH_TOKEN requirement (development only)")
	supervisorRunCmd.Flags().StringVar(&supervisorClaudeBin, "claude-bin", "claude",
		"path to the claude binary (must be on $PATH by default)")
	supervisorRunCmd.Flags().StringSliceVar(&supervisorExtraArgs, "claude-arg", nil,
		"extra arg to pass to claude (repeatable)")
	_ = supervisorRunCmd.MarkFlagRequired("id")
}

// supervisorEnv captures the env checks so tests can exercise them without
// having to spawn a real process. Real runs get os.Getenv; tests inject.
type supervisorEnv interface {
	Getenv(string) string
	LookPath(string) (string, error)
}

type realSupervisorEnv struct{}

func (realSupervisorEnv) Getenv(key string) string              { return os.Getenv(key) }
func (realSupervisorEnv) LookPath(bin string) (string, error)   { return exec.LookPath(bin) }

// errToSRefusal signals that the supervisor refuses to start due to the
// API-key-present guard. Unwrapped as exit code 2.
var errToSRefusal = errors.New("supervisor refused: ANTHROPIC_API_KEY is set; unset it or use an OAuth-only env")

func runSupervisorRun(cmd *cobra.Command, _ []string) error {
	env := realSupervisorEnv{}
	if err := checkSupervisorEnv(env, supervisorSkipOAuthCheck); err != nil {
		// Print to our stderr (so hooks see it) and exit with a stable code.
		fmt.Fprintln(cmd.ErrOrStderr(), err)
		os.Exit(2)
	}
	bin, err := env.LookPath(supervisorClaudeBin)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "supervisor: cannot locate claude binary %q: %v\n", supervisorClaudeBin, err)
		os.Exit(2)
	}

	// Announce the role so downstream logs attribute correctly.
	fmt.Fprintf(cmd.ErrOrStderr(),
		"agm supervisor: id=%q primary-for=%q tertiary-for=%q binary=%q\n",
		supervisorID, supervisorPrimaryFor, supervisorTertiaryFor, bin)

	// Build the claude invocation. The `--channels` flag is the integration
	// point for the agm-bus channel MCP (lands in a subsequent commit as
	// part of agm/agm-plugin/channels/agm-bus/). Until that's on the
	// Anthropic-approved marketplace we pass -dangerously-load-development-
	// channels too. Worker sessions never get this flag by default.
	claudeArgs := append([]string{
		"--dangerously-load-development-channels",
		"server:agm-bus",
	}, supervisorExtraArgs...)

	claudeCmd := exec.Command(bin, claudeArgs...)
	claudeCmd.Stdin = os.Stdin
	claudeCmd.Stdout = cmd.OutOrStdout()
	claudeCmd.Stderr = cmd.ErrOrStderr()
	// Scrub the env one more time before exec — defense in depth.
	claudeCmd.Env = scrubAPIKey(os.Environ())
	// Mark the supervisor id + mesh role in child env so the channel adapter
	// and any in-session tooling can read them without re-parsing args.
	claudeCmd.Env = append(claudeCmd.Env,
		"AGM_SUPERVISOR_ID="+supervisorID,
		"AGM_SUPERVISOR_PRIMARY_FOR="+supervisorPrimaryFor,
		"AGM_SUPERVISOR_TERTIARY_FOR="+supervisorTertiaryFor,
	)

	if err := claudeCmd.Run(); err != nil {
		return fmt.Errorf("supervisor: claude exited: %w", err)
	}
	return nil
}

// checkSupervisorEnv runs the two pre-launch guards. Exported for testing
// via the supervisorEnv interface so callers can fake os.Getenv.
func checkSupervisorEnv(env supervisorEnv, skipOAuthCheck bool) error {
	if env.Getenv("ANTHROPIC_API_KEY") != "" {
		return errToSRefusal
	}
	if !skipOAuthCheck && env.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" {
		return errors.New("supervisor refused: CLAUDE_CODE_OAUTH_TOKEN not set; run `claude setup-token` or pass --skip-oauth-check for dev")
	}
	return nil
}

// scrubAPIKey returns a copy of env with ANTHROPIC_API_KEY removed. Runs
// as a final safety pass: if the user exported the key between the env
// check and exec (unlikely but possible), the child still won't see it.
func scrubAPIKey(env []string) []string {
	const prefix = "ANTHROPIC_API_KEY="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if len(e) >= len(prefix) && e[:len(prefix)] == prefix {
			continue
		}
		out = append(out, e)
	}
	return out
}

func runSupervisorHeartbeat(cmd *cobra.Command, _ []string) error {
	id := supervisorID
	if id == "" {
		id = os.Getenv("AGM_SUPERVISOR_ID")
	}
	if id == "" {
		return errors.New("supervisor heartbeat: --id not set and AGM_SUPERVISOR_ID empty")
	}
	primary := supervisorPrimaryFor
	if primary == "" {
		primary = os.Getenv("AGM_SUPERVISOR_PRIMARY_FOR")
	}
	tertiary := supervisorTertiaryFor
	if tertiary == "" {
		tertiary = os.Getenv("AGM_SUPERVISOR_TERTIARY_FOR")
	}

	rec := heartbeatRecord{
		ID:          id,
		PrimaryFor:  primary,
		TertiaryFor: tertiary,
		LastBeatUTC: time.Now().UTC(),
		PID:         os.Getpid(),
	}
	path, err := heartbeatPath(id)
	if err != nil {
		return err
	}
	return writeHeartbeatRecord(path, rec)
}

// writeHeartbeatRecord marshals rec and writes it atomically via a temp
// file + rename so the sentinel never reads a half-written file.
func writeHeartbeatRecord(path string, rec heartbeatRecord) error {
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal heartbeat: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// readHeartbeatRecord loads a supervisor's latest heartbeat. Returns
// (nil, nil) if the file doesn't exist — never-heartbeated is not an
// error, it's just missing signal.
func readHeartbeatRecord(id string) (*heartbeatRecord, error) {
	path, err := heartbeatPath(id)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var rec heartbeatRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return &rec, nil
}

// supervisorRow is the per-supervisor row produced by status reporting.
type supervisorRow struct {
	ID      string           `json:"id"`
	AgeSecs float64          `json:"age_secs"`
	Stale   bool             `json:"stale"`
	Missing bool             `json:"missing"`
	Record  *heartbeatRecord `json:"record,omitempty"`
}

func runSupervisorStatus(cmd *cobra.Command, args []string) error {
	ids, err := resolveSupervisorIDs(cmd, args)
	if err != nil {
		return err
	}
	if ids == nil {
		return nil
	}
	rows, anyStale, err := buildSupervisorStatusRows(ids)
	if err != nil {
		return err
	}
	if err := emitSupervisorStatus(cmd, rows); err != nil {
		return err
	}
	if anyStale {
		os.Exit(3)
	}
	return nil
}

// resolveSupervisorIDs returns the supervisor IDs to inspect: either the
// user-supplied args, or all directories under ~/.agm/supervisors. Returns
// (nil, nil) when no supervisors are registered (and prints a friendly note).
func resolveSupervisorIDs(cmd *cobra.Command, args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home: %w", err)
	}
	base := filepath.Join(home, ".agm", "supervisors")
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(cmd.OutOrStdout(), "no supervisors registered")
			return nil, nil
		}
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// buildSupervisorStatusRows reads each supervisor's heartbeat and returns
// (rows, anyStale, err).
func buildSupervisorStatusRows(ids []string) ([]supervisorRow, bool, error) {
	now := time.Now().UTC()
	var rows []supervisorRow
	anyStale := false
	for _, id := range ids {
		rec, err := readHeartbeatRecord(id)
		if err != nil {
			return nil, false, fmt.Errorf("read %s: %w", id, err)
		}
		r := supervisorRow{ID: id, Record: rec}
		if rec == nil {
			r.Missing = true
			r.Stale = true
			anyStale = true
		} else {
			r.AgeSecs = now.Sub(rec.LastBeatUTC).Seconds()
			if now.Sub(rec.LastBeatUTC) > supervisorStatusStaleAfter {
				r.Stale = true
				anyStale = true
			}
		}
		rows = append(rows, r)
	}
	return rows, anyStale, nil
}

// emitSupervisorStatus writes rows in the requested format (JSON or columnar).
func emitSupervisorStatus(cmd *cobra.Command, rows []supervisorRow) error {
	if supervisorStatusJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "%-16s %-12s %-10s %s\n", "SUPERVISOR", "AGE", "STATE", "MESH")
	for _, r := range rows {
		age := "—"
		mesh := ""
		if !r.Missing {
			age = fmt.Sprintf("%.1fs", r.AgeSecs)
			if r.Record != nil {
				mesh = fmt.Sprintf("primary-for=%s tertiary-for=%s", r.Record.PrimaryFor, r.Record.TertiaryFor)
			}
		}
		state := "ok"
		if r.Stale {
			state = "STALE"
			if r.Missing {
				state = "NEVER"
			}
		}
		fmt.Fprintf(w, "%-16s %-12s %-10s %s\n", r.ID, age, state, mesh)
	}
	return nil
}

// Compile-time sanity: supervisorEnv implementations.
var _ supervisorEnv = realSupervisorEnv{}

// Unused-import shims for clarity.
var _ io.Writer = os.Stderr
