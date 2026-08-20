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
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/harnessexec"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/pkg/llm/auth"
	"github.com/vbonnet/dear-agent/pkg/override"
	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
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
	// TmuxSession is the tmux session the heartbeat was written from,
	// self-reported best-effort at beat time. It lets `agm supervisor status`
	// verify that a harness process is actually running where the heartbeat
	// claims to come from — a fresh heartbeat alone is false-green when an
	// orphaned writer keeps beating after the harness died (ce-axsr/ce-qkf7).
	TmuxSession string `json:"tmux_session,omitempty"`
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
	fileID := id
	role := id
	if member, ok := vroomsupervisor.Lookup(id); ok {
		fileID = member.Alias
		role = string(member.Role)
	}
	rec := vroomHeartbeatFile{
		TS:   float64(ts.UnixMilli()) / 1e3,
		ISO:  ts.UTC().Format(time.RFC3339),
		Role: role,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal vroom heartbeat: %w", err)
	}
	dst := filepath.Join(dir, fileID+".json")
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
CLAUDE_CODE_OAUTH_TOKEN (Max plan) on a pinned model and in auto permission
mode so unattended boot cannot block on a model-switch or plan-exit prompt.

The experimental agm-bus channel can be enabled explicitly with
--load-development-channels. It is off by default because Claude Code's
development-channel confirmation prompt is harness-level and cannot be safely
answered by an unattended supervisor during boot.

With the channel enabled, supervisors can:
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
  run         Launch a supervisor session
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
	supervisorClaudeModel     string
	supervisorLoadDevChannels bool
	supervisorExtraArgs       []string
)

// supervisorDefaultClaudeModel pins raw `agm supervisor run` sessions to the
// same non-credit-gated 200k Sonnet default used by vroom-dispatch. Launching
// without an explicit model lets Claude Code stop at an unattended "Switch
// model?" dialog before the supervisor's recurring tick loop is installed.
const supervisorDefaultClaudeModel = "sonnet-200k"

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
supervisors if no id is provided. Beyond per-supervisor staleness, the
report cross-references the mesh liveness graph (primary_for/tertiary_for)
to flag mesh-recovery quorum loss: a stale supervisor whose every recoverer
is also stale cannot be re-driven from inside the mesh and needs an external
respawn/escalation (bead ce-2qbx).

Exit codes (so this can drive a monitoring check with severity gradation):
  0 — all supervisors fresh.
  3 — some supervisor stale, but every stale one still has a live peer able
      to wake it; the mesh is expected to self-heal.
  4 — quorum lost: a stale supervisor has no live recoverer; external action
      is required. Supersedes 3.

A supervisor is stale when its heartbeat is older than --stale-after
(default 5m). Heartbeat freshness alone is NOT proof of liveness: a fresh
heartbeat is cross-checked against the supervisor's tmux pane process tree,
and a fresh heartbeat with no live harness process reports as ZOMBIE and
counts as stale (an orphaned writer is beating for a dead supervisor —
ce-axsr/ce-qkf7).`,
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
	supervisorRunCmd.Flags().StringVar(&supervisorClaudeModel, "model", supervisorDefaultClaudeModel,
		"Claude model alias for raw supervisor sessions; defaults to a 200k-context model to avoid startup model-switch prompts")
	supervisorRunCmd.Flags().BoolVar(&supervisorLoadDevChannels, "load-development-channels", false,
		"opt in to Claude Code development channels for agm-bus; disabled by default because the confirmation prompt blocks unattended boot")
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

var reserveSupervisorDangerousOverride = override.Reserve

func reserveSupervisorOAuthOverride(reason, session string) (*override.Reservation, error) {
	reservation, err := reserveSupervisorDangerousOverride(override.Request{
		Kind:    override.KindSupervisorOAuthCheck,
		Reason:  reason,
		Actor:   override.Actor(),
		Session: session,
	})
	if err != nil {
		return nil, fmt.Errorf("authorize supervisor --skip-oauth-check: %w", err)
	}
	return reservation, nil
}

// submitSupervisorLaunch hands final live-admission effects to the private
// Claude executor before starting it. A confirmed executor-start failure leaves
// the sealed reservations and spawn obligation unconsumed.
func submitSupervisorLaunch(
	beforeSpawn func(...*override.Reservation) ([]*override.Reservation, error),
	bind func(bool, ...*override.Reservation) error,
	run func() error,
	reservation *override.Reservation,
) error {
	var reservations []*override.Reservation
	if reservation != nil {
		reservations = append(reservations, reservation)
	}
	if beforeSpawn != nil {
		var err error
		reservations, err = beforeSpawn(reservations...)
		if err != nil {
			return fmt.Errorf("supervisor: launch admission: %w", err)
		}
	}
	if bind == nil {
		return errors.New("supervisor: private Claude executor cannot bind launch effects")
	}
	if err := bind(true, reservations...); err != nil {
		return fmt.Errorf("supervisor: bind launch override transaction: %w", err)
	}
	if err := run(); err != nil {
		return fmt.Errorf("supervisor: claude exited: %w", err)
	}
	return nil
}

// supervisorPreflight runs every pre-launch guard in order and returns the
// resolved claude binary path, or the first refusal. It returns errors rather
// than exiting so the ordering and the refusals are testable; runSupervisorRun
// owns the exit code.
//
// Order matters. The env and binary guards come first so a misconfigured
// invocation reports its real problem (no OAuth token, no claude on PATH)
// instead of a resource refusal. The admission precheck comes last; the caller
// separately commits the returned one-shot admission immediately before Run.
//
// credsPath overrides the OAuth credentials-file location (empty means the real
// ~/.claude/.credentials.json), matching checkSupervisorEnv, so tests do not
// depend on the host's auth.
//
// admit is the host admission gate. A supervisor is a persistent claude process
// that consumes the same CPU, RAM, and disk as any worker, so it goes through
// the same gates as `agm session new` — this path was entirely ungated while
// the mesh saturated the host on 2026-07-18 (ce-93lw.18).
func supervisorPreflight(env supervisorEnv, skipOAuthCheck bool, credsPath string, admit func() error) (string, error) {
	if err := checkSupervisorEnv(env, skipOAuthCheck, credsPath); err != nil {
		return "", err
	}
	bin, err := env.LookPath(supervisorClaudeBin)
	if err != nil {
		return "", fmt.Errorf("supervisor: cannot locate claude binary %q: %w", supervisorClaudeBin, err)
	}
	if admit != nil {
		if err := admit(); err != nil {
			return "", fmt.Errorf("supervisor: %w", err)
		}
	}
	return bin, nil
}

func runSupervisorRun(cmd *cobra.Command, _ []string) error {
	var admission *circuitBreakerAdmission
	bin, err := supervisorPreflight(realSupervisorEnv{}, supervisorSkipOAuthCheck, "", func() error {
		var admissionErr error
		admission, admissionErr = enforceCircuitBreakers(supervisorID, "claude-code", supervisorClaudeModel)
		return admissionErr
	})
	if err != nil {
		// Print to our stderr (so hooks see it) and exit with a stable code.
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
		os.Exit(2)
	}
	bin, err = filepath.Abs(bin)
	if err != nil {
		return fmt.Errorf("supervisor: resolve Claude executable: %w", err)
	}

	var oauthOverride *override.Reservation
	if supervisorSkipOAuthCheck {
		oauthOverride, err = reserveSupervisorOAuthOverride(supervisorSkipOAuthReason, supervisorID)
		if err != nil {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), err)
			os.Exit(2)
		}
	}

	// Announce the role so downstream logs attribute correctly.
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
		"agm supervisor: id=%q primary-for=%q tertiary-for=%q binary=%q\n",
		supervisorID, supervisorPrimaryFor, supervisorTertiaryFor, bin)

	prepared, err := harnessexec.PrepareClaudeCommand(buildSupervisorClaudeLaunch(
		bin,
		supervisorClaudeModel,
		supervisorLoadDevChannels,
		supervisorExtraArgs,
		// --skip-oauth-check bypasses only the preflight requirement. Preserve
		// the existing behavior of forwarding a fresh token when one is
		// available.
		false,
	), os.Environ())
	if err != nil {
		return fmt.Errorf("supervisor: prepare private Claude executor: %w", err)
	}
	defer func() { _ = prepared.Cancel() }()

	executable, arguments, err := prepared.DirectInvocation()
	if err != nil {
		return fmt.Errorf("supervisor: prepare private Claude invocation: %w", err)
	}
	claudeCmd := exec.Command(executable, arguments...)
	claudeCmd.Stdin = os.Stdin
	claudeCmd.Stdout = cmd.OutOrStdout()
	claudeCmd.Stderr = cmd.ErrOrStderr()
	// Scrub the API key from the private executor too. The prepared handoff
	// carries a fresh OAuth snapshot without exposing it in argv.
	claudeCmd.Env = scrubAPIKey(os.Environ())
	// Mark the supervisor id + mesh role in child env so the channel adapter
	// and any in-session tooling can read them without re-parsing args.
	claudeCmd.Env = append(claudeCmd.Env,
		"AGM_SUPERVISOR_ID="+supervisorID,
		"AGM_SUPERVISOR_PRIMARY_FOR="+supervisorPrimaryFor,
		"AGM_SUPERVISOR_TERTIARY_FOR="+supervisorTertiaryFor,
	)

	if err := submitSupervisorLaunch(
		admission.beforeSpawn,
		prepared.BindOverrideReservations,
		claudeCmd.Run,
		oauthOverride,
	); err != nil {
		return err
	}
	return nil
}

func buildSupervisorClaudeLaunch(
	binary, model string,
	loadDevelopmentChannels bool,
	extraArgs []string,
	disableOAuth bool,
) harnessexec.ClaudeLaunch {
	if model == "" {
		model = supervisorDefaultClaudeModel
	}
	args := append([]string(nil), extraArgs...)
	if loadDevelopmentChannels {
		args = append(
			[]string{"--dangerously-load-development-channels", "server:agm-bus"},
			args...,
		)
	}
	return harnessexec.ClaudeLaunch{
		Binary:       binary,
		SessionName:  supervisorID,
		Model:        agent.ResolveModelFullName("claude-code", model),
		AutoMode:     true,
		Permission:   "auto",
		ExtraArgs:    args,
		DisableOAuth: disableOAuth,
		Persistent:   true,
	}
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
	id, primary, tertiary, err := canonicalizeSupervisorHeartbeatMesh(id, primary, tertiary)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	rec := heartbeatRecord{
		ID:          id,
		PrimaryFor:  primary,
		TertiaryFor: tertiary,
		LastBeatUTC: now,
		PID:         os.Getpid(),
	}
	// Self-report the tmux session so status can verify a harness process is
	// actually running where this heartbeat claims to come from (ce-axsr).
	// Best-effort: outside tmux the field stays empty.
	if tmuxSession, tsErr := tmux.GetCurrentSessionName(); tsErr == nil {
		rec.TmuxSession = tmuxSession
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

// canonicalizeSupervisorHeartbeatMesh applies the canonical VROOM topology to
// heartbeat input. Canonical IDs, compact aliases, and role names all resolve
// to the same identity and peer graph. Unknown IDs retain AGM's generic
// supervisor behavior and pass through unchanged.
func canonicalizeSupervisorHeartbeatMesh(id, primary, tertiary string) (string, string, string, error) {
	member, ok := vroomsupervisor.Lookup(id)
	if !ok {
		return id, primary, tertiary, nil
	}

	canonicalPeer := func(flag, value, expected string) (string, error) {
		if value == "" {
			return expected, nil
		}
		peer, resolved := vroomsupervisor.Lookup(value)
		if !resolved {
			return "", fmt.Errorf("supervisor heartbeat: canonical %s %q must name a VROOM supervisor, got %q", flag, member.ID, value)
		}
		if peer.ID != expected {
			return "", fmt.Errorf("supervisor heartbeat: canonical %s %q is %q, got %q", flag, member.ID, expected, peer.ID)
		}
		return peer.ID, nil
	}

	primary, err := canonicalPeer("primary-for", primary, member.PrimaryFor)
	if err != nil {
		return "", "", "", err
	}
	tertiary, err = canonicalPeer("tertiary-for", tertiary, member.TertiaryFor)
	if err != nil {
		return "", "", "", err
	}
	return member.ID, primary, tertiary, nil
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
//
// Recoverable/QuorumLost/Recoverers capture mesh-recovery reachability, not
// just this supervisor's own liveness. The mesh's mutual-unblock invariant
// (ADR-002) only self-heals a stale supervisor when a *surviving* peer that
// lists it as primary_for/tertiary_for is still ticking to re-drive it. When
// a stale supervisor's every recoverer is also stale, no in-mesh recovery can
// fire (bead ce-2qbx: both meta-o and orch stale at once) — an external
// watchdog must respawn/escalate. These fields make that condition machine-
// readable instead of leaving it to be inferred from a table of independent
// staleness flags.
type supervisorRow struct {
	ID          string           `json:"id"`
	AgeSecs     float64          `json:"age_secs"`
	Stale       bool             `json:"stale"`
	Missing     bool             `json:"missing"`
	Recoverable bool             `json:"recoverable"`
	QuorumLost  bool             `json:"quorum_lost"`
	Recoverers  []string         `json:"recoverers,omitempty"`
	Record      *heartbeatRecord `json:"record,omitempty"`
	// ProcessAlive reports that a harness process was verified running in
	// the supervisor's tmux session pane tree. Heartbeat freshness alone is
	// false-green (ce-axsr/ce-qkf7): an orphaned writer can keep the
	// heartbeat fresh for hours after the harness died.
	ProcessAlive bool `json:"process_alive,omitempty"`
	// Zombie reports a fresh heartbeat WITHOUT a live harness process — the
	// exact ce-qkf7 failure mode. A zombie is treated as stale/DEAD for exit
	// codes and mesh-recovery reachability.
	Zombie bool `json:"zombie,omitempty"`
	// Reason explains a Zombie verdict or why process liveness could not be
	// verified.
	Reason string `json:"reason,omitempty"`
}

// paneProbe is the process-liveness verdict for one tmux session, decoupled
// from the tmux package so status logic is table-testable with fakes.
type paneProbe struct {
	Exists       bool
	HarnessAlive bool
	ZombieWriter bool
	Evidence     string
}

// probeSupervisorPane resolves harness-process liveness for a supervisor's
// tmux session. Injectable for tests.
var probeSupervisorPane = func(sessionName string) (paneProbe, error) {
	pl, err := tmux.CheckPaneLiveness(sessionName, tmux.GetSocketPath())
	if err != nil {
		return paneProbe{}, err
	}
	return paneProbe{
		Exists:       pl.SessionExists,
		HarnessAlive: pl.HarnessAlive,
		ZombieWriter: pl.ZombieWriter,
		Evidence:     pl.Evidence,
	}, nil
}

// supervisorTmuxSession resolves the tmux session to verify for a row.
// Prefers the session the heartbeat self-reported (explicit=true); falls back
// to the supervisor id itself when it is already a VROOM session name (the
// deployed mesh registers ids like "vroom-meta-orchestrator"), then to the
// well-known VROOM session name for short mesh roles (both explicit=false).
// Returns "" when there is nothing to verify against.
func supervisorTmuxSession(r supervisorRow) (name string, explicit bool) {
	if r.Record != nil && r.Record.TmuxSession != "" {
		return r.Record.TmuxSession, true
	}
	if member, ok := vroomsupervisor.Lookup(r.ID); ok {
		return member.ID, false
	}
	return "", false
}

// applyProcessLiveness cross-checks each FRESH heartbeat against the actual
// process state of the supervisor's tmux session (ce-axsr). A fresh heartbeat
// with no live harness process is a zombie: something external (e.g. an
// orphaned agm child, ce-qkf7) is writing the heartbeat, and the supervisor
// is DEAD. Zombies are marked Stale so exit codes and mesh-recovery
// reachability treat them as dead.
//
// Fail-safe rules:
//   - Rows already stale/missing are left alone — heartbeat age was decisive.
//   - A probe error, or no tmux session to verify against, never flips a row
//     red: the row keeps its fresh verdict with Reason noting "unverified".
//   - A MISSING tmux session only proves death when the heartbeat itself
//     recorded that session (explicit): the writer claims to run in a session
//     that no longer exists. For inferred (role-mapped) names, a missing
//     session is treated as unverifiable — the supervisor may legitimately
//     run outside tmux.
//
// Returns true if any row is stale after application. Pure over rows given a
// fake probe, so it is table-testable.
func applyProcessLiveness(rows []supervisorRow, probe func(sessionName string) (paneProbe, error)) bool {
	anyStale := false
	for i := range rows {
		r := &rows[i]
		if r.Stale || r.Missing {
			anyStale = true
			continue
		}
		sessionName, explicit := supervisorTmuxSession(*r)
		if sessionName == "" {
			r.Reason = "process liveness unverified: no tmux session recorded for this supervisor"
			continue
		}
		p, err := probe(sessionName)
		if err != nil {
			r.Reason = fmt.Sprintf("process liveness unverified: probe of %q failed: %v", sessionName, err)
			continue
		}
		switch {
		case p.Exists && p.HarnessAlive:
			r.ProcessAlive = true
		case p.Exists && !p.HarnessAlive:
			r.Zombie = true
			r.Stale = true
			r.Reason = fmt.Sprintf("ZOMBIE: heartbeat is fresh but no harness process is running in tmux session %q (pane tree: %s)", sessionName, p.Evidence)
			if p.ZombieWriter {
				r.Reason += " — an orphaned agm process in the pane tree is likely writing this heartbeat"
			}
		case !p.Exists && explicit:
			r.Zombie = true
			r.Stale = true
			r.Reason = fmt.Sprintf("ZOMBIE: heartbeat is fresh but its recorded tmux session %q no longer exists — an orphaned writer is beating for a dead supervisor", sessionName)
		default: // !p.Exists && !explicit
			r.Reason = fmt.Sprintf("process liveness unverified: inferred tmux session %q not found (supervisor may run outside tmux)", sessionName)
		}
		if r.Stale {
			anyStale = true
		}
	}
	return anyStale
}

// annotateMeshRecovery cross-references the per-supervisor staleness flags
// against the mesh liveness graph (primary_for/tertiary_for) to fill in each
// row's Recoverable/QuorumLost/Recoverers. It returns true if any supervisor
// is quorum-lost — stale with no live peer able to re-drive it.
//
// A supervisor Y's recoverers are the peers Z where Z.PrimaryFor == Y or
// Z.TertiaryFor == Y (the mesh members responsible for waking Y). Y is
// recoverable when it is not stale, or when at least one recoverer is present
// and itself not stale. Y is quorum-lost when it is stale and no recoverer is
// live (all stale/missing, or none declared) — the exact state where the
// in-mesh AGMRecovery cannot run because the supervisor that would call
// `agm send wake-loop Y` is dead too.
//
// Pure over its input slice: mutates the rows' recovery fields and reads no
// clock or filesystem, so it is trivially testable.
func annotateMeshRecovery(rows []supervisorRow) bool {
	// staleByID lets a recoverer's liveness be looked up in O(1). A supervisor
	// named as a peer but never registered has no row and is treated as stale:
	// an absent recoverer cannot recover anyone.
	staleByID := make(map[string]bool, len(rows))
	for _, r := range rows {
		staleByID[r.ID] = r.Stale
	}
	// recoverersOf maps a peer Y to the ids that list it as primary/tertiary.
	recoverersOf := make(map[string][]string)
	for _, r := range rows {
		if r.Record == nil {
			continue
		}
		if p := r.Record.PrimaryFor; p != "" {
			recoverersOf[p] = append(recoverersOf[p], r.ID)
		}
		if tf := r.Record.TertiaryFor; tf != "" && tf != r.Record.PrimaryFor {
			recoverersOf[tf] = append(recoverersOf[tf], r.ID)
		}
	}

	anyQuorumLost := false
	for i := range rows {
		r := &rows[i]
		recoverers := recoverersOf[r.ID]
		r.Recoverers = recoverers
		if !r.Stale {
			// A live supervisor needs no recovery; it is trivially recoverable.
			r.Recoverable = true
			continue
		}
		liveRecoverer := false
		for _, rid := range recoverers {
			if stale, known := staleByID[rid]; known && !stale {
				liveRecoverer = true
				break
			}
		}
		r.Recoverable = liveRecoverer
		if !liveRecoverer {
			r.QuorumLost = true
			anyQuorumLost = true
		}
	}
	return anyQuorumLost
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
	// Heartbeat freshness alone must not prove liveness (ce-axsr/ce-qkf7):
	// verify a harness process actually runs where each fresh heartbeat
	// claims to come from. Zombies (fresh beat, dead process) become stale.
	if processStale := applyProcessLiveness(rows, probeSupervisorPane); processStale {
		anyStale = true
	}
	// Cross-reference staleness against the mesh liveness graph so a stale
	// supervisor whose recoverers are also dead surfaces as quorum-lost rather
	// than an ordinary stale row (bead ce-2qbx).
	quorumLost := annotateMeshRecovery(rows)

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
	// Graded exit status so an external watchdog can distinguish a mesh that
	// will self-heal from one that cannot:
	//   4 — quorum lost: a stale supervisor has no live recoverer; the mesh
	//       cannot re-drive it from inside, so external respawn/escalation is
	//       required. Strictly worse than 3, so it takes precedence.
	//   3 — some supervisor stale, but every stale one still has a live peer
	//       able to wake it; the mesh is expected to self-heal.
	//   0 — all supervisors fresh.
	if quorumLost {
		os.Exit(4)
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
	_, _ = fmt.Fprintf(w, "%-16s %-12s %-10s %-12s %s\n", "SUPERVISOR", "AGE", "STATE", "RECOVERY", "MESH")
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
			if r.Zombie {
				state = "ZOMBIE"
			}
		}
		// RECOVERY reflects whether the mesh can re-drive this supervisor from
		// inside: a live supervisor is "ok"; a stale one is "recoverable" while
		// a peer can still wake it, or "QUORUM-LOST" when none can.
		recovery := "ok"
		if r.Stale {
			if r.QuorumLost {
				recovery = "QUORUM-LOST"
			} else {
				recovery = "recoverable"
			}
		}
		_, _ = fmt.Fprintf(w, "%-16s %-12s %-10s %-12s %s\n", r.ID, age, state, recovery, mesh)
		if r.Reason != "" {
			_, _ = fmt.Fprintf(w, "%-16s   %s\n", "", r.Reason)
		}
	}
	return nil
}

// Compile-time sanity: supervisorEnv implementations.
var _ supervisorEnv = realSupervisorEnv{}

// Unused-import shims for clarity.
var _ io.Writer = os.Stderr
