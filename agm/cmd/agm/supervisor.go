package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/override"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
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
	ID          string    `json:"id"`
	PrimaryFor  string    `json:"primary_for,omitempty"`
	TertiaryFor string    `json:"tertiary_for,omitempty"`
	LastBeatUTC time.Time `json:"last_beat_utc"`
	PID         int       `json:"pid,omitempty"`
}

// vroomHeartbeatFile is the flat JSON shape read by the Overseer SKILL at
// ~/.agm/vroom/heartbeat/<name>.json. The SKILL writes these itself during
// ticks, but when the file goes stale (supervisor crashed, skill gap) the
// authoritative AGM store still shows the supervisor alive — causing false
// STALE alerts. SyncHeartbeatFiles bridges the two stores.
type vroomHeartbeatFile struct {
	TS   float64 `json:"ts"`
	ISO  string  `json:"iso"`
	Role string  `json:"role"`
}

// defaultVroomHeartbeatDir returns the default directory for VROOM flat
// heartbeat files.
func defaultVroomHeartbeatDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".agm", "vroom", "heartbeat"), nil
}

// syncVroomHeartbeatFile writes a single flat VROOM heartbeat file for the
// named supervisor using ts as the authoritative timestamp. Writes atomically
// via temp-file + rename. dir is created if it doesn't exist.
func syncVroomHeartbeatFile(dir, id string, ts time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	rec := vroomHeartbeatFile{
		TS:   float64(ts.UnixMilli()) / 1e3,
		ISO:  ts.UTC().Format(time.RFC3339),
		Role: supervisorRole(id),
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal vroom heartbeat: %w", err)
	}
	dst := filepath.Join(dir, id+".json")
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// SyncHeartbeatFiles mirrors the authoritative AGM supervisor heartbeat
// records to flat VROOM files under dir. Supervisors with a missing or
// zero heartbeat record are skipped (no file written or removed). dir is
// created on demand. Errors for individual supervisors are collected and
// returned as a single joined error so one bad supervisor doesn't block
// the rest.
func SyncHeartbeatFiles(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("home dir: %w", err)
	}
	base := filepath.Join(home, ".agm", "supervisors")
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // no supervisors registered yet
		}
		return err
	}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		rec, err := readHeartbeatRecord(id)
		if err != nil {
			errs = append(errs, fmt.Errorf("read %s: %w", id, err))
			continue
		}
		if rec == nil || rec.LastBeatUTC.IsZero() {
			continue // never heartbeated — skip, don't write a stale file
		}
		if err := syncVroomHeartbeatFile(dir, id, rec.LastBeatUTC); err != nil {
			errs = append(errs, fmt.Errorf("sync %s: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

// supervisorRole returns a human-readable role label for a supervisor ID.
// Used as the "role" field in the VROOM flat heartbeat file so the Overseer
// SKILL can distinguish supervisors by function, not just opaque ID.
func supervisorRole(id string) string {
	switch id {
	case "meta-o":
		return "meta-orchestrator"
	case "orch":
		return "orchestrator"
	case "overseer":
		return "overseer"
	default:
		return id
	}
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
	supervisorID              string
	supervisorPrimaryFor      string
	supervisorTertiaryFor     string
	supervisorSkipOAuthCheck  bool
	supervisorSkipOAuthReason string
	supervisorClaudeBin       string
	supervisorExtraArgs       []string
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
		"skip the CLAUDE_CODE_OAUTH_TOKEN requirement (development only) — requires --reason")
	supervisorRunCmd.Flags().StringVar(&supervisorSkipOAuthReason, "reason", "",
		"justification for --skip-oauth-check, recorded in the override audit log")
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

func (realSupervisorEnv) Getenv(key string) string            { return os.Getenv(key) }
func (realSupervisorEnv) LookPath(bin string) (string, error) { return exec.LookPath(bin) }

// errToSRefusal signals that the supervisor refuses to start due to the
// API-key-present guard. Unwrapped as exit code 2.
var errToSRefusal = errors.New("supervisor refused: ANTHROPIC_API_KEY is set; unset it or use an OAuth-only env")

func runSupervisorRun(cmd *cobra.Command, _ []string) error {
	// Skipping the OAuth-token requirement requires a recorded justification:
	// the gate exists so a supervisor never launches without auth and silently
	// fails downstream.
	if supervisorSkipOAuthCheck {
		if gerr := override.Require(cmd.Context(), override.Guard{
			Tool: "agm supervisor run",
			Flag: "--skip-oauth-check",
			Gate: "CLAUDE_CODE_OAUTH_TOKEN presence requirement",
			Risk: override.RiskP1,
		}, supervisorSkipOAuthReason); gerr != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), gerr)
			os.Exit(2)
		}
	}

	env := realSupervisorEnv{}
	if err := checkSupervisorEnv(env, supervisorSkipOAuthCheck, ""); err != nil {
		// Print to our stderr (so hooks see it) and exit with a stable code.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
		os.Exit(2)
	}
	bin, err := env.LookPath(supervisorClaudeBin)
	if err != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "supervisor: cannot locate claude binary %q: %v\n", supervisorClaudeBin, err)
		os.Exit(2)
	}

	// Announce the role so downstream logs attribute correctly.
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
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
	// Refresh the OAuth token from the live credentials file: a token captured
	// into the orchestrator's env goes stale after Claude Code auto-refreshes
	// the file (ce-dzhz). Strip any stale copy and inject the freshest token so
	// the supervisor's claude never launches with an expired credential.
	claudeCmd.Env = scrubEnvKey(claudeCmd.Env, auth.OAuthEnvVar)
	if token := auth.ResolveOAuthToken(); token != "" {
		claudeCmd.Env = append(claudeCmd.Env, auth.OAuthEnvVar+"="+token)
	}
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
// via the supervisorEnv interface so callers can fake os.Getenv. credsPath
// overrides the OAuth credentials-file location (empty → the real
// ~/.claude/.credentials.json) so tests don't depend on the host's auth.
//
// The OAuth presence guard accepts a token from EITHER the
// CLAUDE_CODE_OAUTH_TOKEN env var OR the live credentials file (ce-dzhz): the
// env var goes stale after Claude Code auto-refreshes the file, so requiring
// the env var alone would wrongly refuse a supervisor that has valid
// file-based auth.
func checkSupervisorEnv(env supervisorEnv, skipOAuthCheck bool, credsPath string) error {
	if env.Getenv("ANTHROPIC_API_KEY") != "" {
		return errToSRefusal
	}
	if !skipOAuthCheck {
		resolver := auth.OAuthResolver{Getenv: env.Getenv, CredentialsPath: credsPath}
		if resolver.Resolve() == "" {
			return errors.New("supervisor refused: no Claude OAuth token available — set CLAUDE_CODE_OAUTH_TOKEN or run `claude setup-token` to populate ~/.claude/.credentials.json; pass --skip-oauth-check for dev")
		}
	}
	return nil
}

// scrubAPIKey returns a copy of env with ANTHROPIC_API_KEY removed. Runs
// as a final safety pass: if the user exported the key between the env
// check and exec (unlikely but possible), the child still won't see it.
func scrubAPIKey(env []string) []string {
	return scrubEnvKey(env, "ANTHROPIC_API_KEY")
}

// scrubEnvKey returns a copy of env with all assignments of key removed.
// Used to drop a stale value before appending a fresh one, since duplicate
// env entries have implementation-defined precedence.
func scrubEnvKey(env []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
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

	now := time.Now().UTC()
	rec := heartbeatRecord{
		ID:          id,
		PrimaryFor:  primary,
		TertiaryFor: tertiary,
		LastBeatUTC: now,
		PID:         os.Getpid(),
	}
	path, err := heartbeatPath(id)
	if err != nil {
		return err
	}
	if err := writeHeartbeatRecord(path, rec); err != nil {
		return err
	}
	// Mirror to the flat VROOM heartbeat file so the Overseer SKILL's
	// file-based staleness check sees fresh data immediately after each tick.
	// Best-effort: a sync failure is printed but does not fail the beat.
	if vroomDir, dirErr := defaultVroomHeartbeatDir(); dirErr == nil {
		if syncErr := syncVroomHeartbeatFile(vroomDir, id, now); syncErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "warn: vroom heartbeat sync: %v\n", syncErr)
		}
	}
	return nil
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

	// Mirror AGM records to the flat VROOM heartbeat files so the Overseer
	// SKILL's file-based staleness check uses the same authoritative data.
	// Best-effort: a sync failure is logged to stderr but does not fail the
	// status command itself (the AGM read already succeeded).
	if vroomDir, dirErr := defaultVroomHeartbeatDir(); dirErr == nil {
		if syncErr := SyncHeartbeatFiles(vroomDir); syncErr != nil {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warn: vroom heartbeat sync: %v\n", syncErr)
		}
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "no supervisors registered")
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
	_, _ = fmt.Fprintf(w, "%-16s %-12s %-10s %s\n", "SUPERVISOR", "AGE", "STATE", "MESH")
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
		_, _ = fmt.Fprintf(w, "%-16s %-12s %-10s %s\n", r.ID, age, state, mesh)
	}
	return nil
}

// Compile-time sanity: supervisorEnv implementations.
var _ supervisorEnv = realSupervisorEnv{}

// Unused-import shims for clarity.
var _ io.Writer = os.Stderr
