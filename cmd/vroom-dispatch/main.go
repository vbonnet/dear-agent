package main

import (
	"bufio"
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/otelsetup"
	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// tracerName is the OTel tracer name for vroom-dispatch. Spans are no-ops
// unless OTEL_EXPORTER_OTLP_ENDPOINT is set (run `otel-local up`).
const tracerName = "vroom-dispatch"

//go:embed skills/*.md
var skills embed.FS

type supervisor struct {
	// Topology fields are populated only by supervisorForRole. Harness, model,
	// skill, and tick fields below remain dispatch-owned launch policy.
	Name         string
	ID           string
	Role         string // RBAC role/profile (agm --role); grants the session's permission profile
	Harness      string
	Model        string
	SkillFile    string
	PrimaryFor   string
	TertiaryFor  string
	TickInterval time.Duration
	TickPrompt   string
}

func supervisorForRole(role vroomsupervisor.Role, policy supervisor) supervisor {
	member, ok := vroomsupervisor.MemberForRole(role)
	if !ok {
		panic(fmt.Sprintf("vroom-dispatch: no canonical topology member for role %q", role))
	}
	policy.Name = member.ID
	policy.ID = member.ID
	policy.Role = string(member.Role)
	policy.PrimaryFor = member.PrimaryFor
	policy.TertiaryFor = member.TertiaryFor
	return policy
}

var supervisors = []supervisor{
	supervisorForRole(vroomsupervisor.RoleMetaOrchestrator, supervisor{
		Harness:      "claude-code",
		Model:        defaultSupervisorModel,
		SkillFile:    "meta-orchestrator.md",
		TickInterval: 180 * time.Second,
		TickPrompt: "Read ~/.agm/vroom/skills/protocol.md and ~/.agm/vroom/skills/meta-orchestrator.md, " +
			"then execute exactly one resilient Meta-Orchestrator tick.",
	}),
	supervisorForRole(vroomsupervisor.RoleOrchestrator, supervisor{
		Harness:      "codex-cli",
		Model:        "gpt-5.5",
		SkillFile:    "orchestrator.md",
		TickInterval: 90 * time.Second,
		TickPrompt: "Read ~/.agm/vroom/skills/protocol.md and ~/.agm/vroom/skills/orchestrator.md, " +
			"then execute exactly one resilient Orchestrator tick.",
	}),
	supervisorForRole(vroomsupervisor.RoleOverseer, supervisor{
		Harness:      "agy",
		Model:        "2.5-flash",
		SkillFile:    "overseer.md",
		TickInterval: 60 * time.Second,
		TickPrompt: "Read ~/.agm/vroom/skills/protocol.md and ~/.agm/vroom/skills/overseer.md, " +
			"then execute exactly one resilient Overseer tick.",
	}),
}

// sessionState tracks persistent AGM sessions across launcher restarts.
type sessionState struct {
	Sessions  map[string]sessionInfo `json:"sessions"`
	UpdatedAt string                 `json:"updated_at"`
}

type sessionInfo struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at"`
	LoopSent  bool   `json:"loop_sent"`
}

const sessionsFile = ".agm/vroom/sessions.json"

// healthCheckInterval is how often the Dispatch Advisor ticks when running
// as a persistent daemon (default mode, without -boot-only).
const healthCheckInterval = 30 * time.Second

const (
	workerHealthProbeTimeout = time.Minute
	readyBeadsProbeTimeout   = 30 * time.Second
)

// Graduated escalation thresholds for worker monitoring. The Dispatch Advisor
// is the programmatic safety net — it gives LLM supervisors (Orchestrator at
// 15/30/45min) a chance to act first, then catches anything they miss.
const (
	workerNudgeAfter    = 20 * time.Minute // Level 1: send status ping
	workerDiagnoseAfter = 35 * time.Minute // Level 2: send wrap-up/defer command
	workerKillAfter     = 50 * time.Minute // Level 3: force-kill (only if stuck state + no progress)
)

// restartTracker tracks per-supervisor restart state for exponential backoff.
type restartTracker struct {
	mu        sync.Mutex
	restarts  map[string]int           // consecutive restart count
	backoff   map[string]time.Duration // current backoff duration
	lastTry   map[string]time.Time     // last restart attempt
	escalated map[string]bool          // whether escalation has been logged for this failure cycle
}

func newRestartTracker() *restartTracker {
	return &restartTracker{
		restarts:  make(map[string]int),
		backoff:   make(map[string]time.Duration),
		lastTry:   make(map[string]time.Time),
		escalated: make(map[string]bool),
	}
}

const (
	initialBackoff = 30 * time.Second
	maxBackoff     = 5 * time.Minute
	maxRestarts    = 3
)

// shouldRestart returns true if enough time has elapsed since the last
// attempt for this supervisor. Returns false if the backoff window hasn't
// elapsed or if max restarts are exhausted.
func (rt *restartTracker) shouldRestart(name string) (bool, int) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	count := rt.restarts[name]
	if count >= maxRestarts {
		return false, count
	}

	bo := rt.backoff[name]
	if bo == 0 {
		bo = initialBackoff
	}

	last := rt.lastTry[name]
	if !last.IsZero() && time.Since(last) < bo {
		return false, count
	}

	return true, count
}

// recordAttempt records a restart attempt and bumps the backoff.
func (rt *restartTracker) recordAttempt(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.restarts[name]++
	rt.lastTry[name] = time.Now()

	bo := rt.backoff[name]
	if bo == 0 {
		bo = initialBackoff
	} else {
		bo *= 2
		if bo > maxBackoff {
			bo = maxBackoff
		}
	}
	rt.backoff[name] = bo
}

// recordRecovery resets the restart counter and backoff when a supervisor
// is observed alive after a restart.
func (rt *restartTracker) recordRecovery(name string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rt.restarts[name] = 0
	rt.backoff[name] = 0
	rt.lastTry[name] = time.Time{}
	rt.escalated[name] = false
}

// shouldEscalate returns true exactly once per failure cycle when a supervisor
// has reached maxRestarts. Subsequent calls return false until recordRecovery
// resets the flag, preventing escalation spam on every health-check tick.
func (rt *restartTracker) shouldEscalate(name string) bool {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.restarts[name] >= maxRestarts && !rt.escalated[name] {
		rt.escalated[name] = true
		return true
	}
	return false
}

// consecutiveRestarts returns the current restart count for a supervisor.
func (rt *restartTracker) consecutiveRestarts(name string) int {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return rt.restarts[name]
}

// supervisorHealth is the liveness classification.
type supervisorHealth int

const (
	healthAlive      supervisorHealth = iota
	healthStale                       // heartbeat old but session exists
	healthDead                        // no session or session archived
	healthAuthFailed                  // session exists but the pane is stuck in provider auth
)

func (h supervisorHealth) String() string {
	switch h {
	case healthAlive:
		return "alive"
	case healthStale:
		return "stale"
	case healthDead:
		return "dead"
	case healthAuthFailed:
		return "auth_failed"
	default:
		return "unknown"
	}
}

// readHeartbeatTime reads a supervisor's heartbeat file and returns the
// timestamp. Returns zero time if the file doesn't exist or can't be parsed.
func readHeartbeatTime(home, name string) time.Time {
	path := filepath.Join(home, ".agm", "vroom", "heartbeat", name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return time.Time{}
	}

	// Heartbeat files contain just a timestamp string (the agm supervisor
	// heartbeat command also writes JSON, but the skill files write a bare
	// date string via `date -u`). Try both formats.
	text := strings.TrimSpace(string(data))

	// Try RFC3339 first (the structured format).
	if t, err := time.Parse(time.RFC3339, text); err == nil {
		return t
	}
	// Try the `date -u` format used by the skill files.
	if t, err := time.Parse("2006-01-02T15:04:05Z", text); err == nil {
		return t
	}
	// Try parsing as JSON with a "timestamp" or "ts" field.
	var obj map[string]string
	if err := json.Unmarshal(data, &obj); err == nil {
		for _, key := range []string{"timestamp", "ts", "last_heartbeat"} {
			if v, ok := obj[key]; ok {
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					return t
				}
			}
		}
	}

	return time.Time{}
}

// classifySupervisor determines the health of a supervisor based on both
// heartbeat freshness and session liveness.
func classifySupervisor(home string, sup supervisor) supervisorHealth {
	sessionUp := isSessionAlive(sup.Name)
	if !sessionUp {
		return healthDead
	}
	if isSupervisorAuthFailed(sup) {
		return healthAuthFailed
	}

	heartbeat := readHeartbeatTime(home, heartbeatFileName(sup.Name))
	if heartbeat.IsZero() {
		// Session exists but no heartbeat file yet — could be booting.
		// Treat as stale rather than dead to avoid killing a session
		// that's still initializing.
		return healthStale
	}

	threshold := 2 * sup.TickInterval
	if time.Since(heartbeat) > threshold {
		return healthStale
	}

	return healthAlive
}

// captureSupervisorPane returns the most recent supervisor pane text. It is a
// package variable so tests can cover auth classification without shelling out.
var captureSupervisorPane = func(name string) (string, error) {
	args := []string{"capture-pane", "-t", name, "-p", "-S", "-80"}
	if socket := os.Getenv("AGM_TMUX_SOCKET"); socket != "" {
		args = append([]string{"-S", socket}, args...)
	}
	cmd := exec.Command("tmux", args...)
	out, err := cmd.Output()
	return string(out), err
}

func isSupervisorAuthFailed(sup supervisor) bool {
	content, err := captureSupervisorPane(sup.Name)
	if err != nil {
		return false
	}
	return supervisorPaneAuthFailed(content, sup.Harness)
}

func supervisorPaneAuthFailed(content, harness string) bool {
	lines := recentSupervisorPaneLines(content)
	if len(lines) == 0 || supervisorPaneEndsAtPrompt(lines) {
		return false
	}
	lower := strings.Join(lines, "\n")
	switch harness {
	case "claude-code":
		return claudePaneAuthFailed(lines, lower)
	case "codex-cli":
		if codexPaneReady(lines) {
			return false
		}
		return codexPaneAuthFailed(lower)
	case "agy":
		return agyPaneAuthFailed(lower)
	default:
		return false
	}
}

// codexPaneReady mirrors the shared Codex readiness contract: after a turn,
// the idle cursor is followed by the structured model/workdir footer. Stale
// auth text above that current composer must not trigger recovery.
func codexPaneReady(lines []string) bool {
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "gpt-") || !strings.Contains(line, " · ") {
			continue
		}
		for j := i - 1; j >= 0 && j >= i-3; j-- {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			if candidate == "›" || candidate == "»" {
				return true
			}
			break
		}
	}
	return false
}

func claudePaneAuthFailed(lines []string, lower string) bool {
	if claudePaneReady(lines) {
		return false
	}
	return hasExactLine(lines, "please run /login") ||
		(strings.Contains(lower, "claude") &&
			(strings.Contains(lower, "/login") || strings.Contains(lower, "oauth")) &&
			hasAny(lower, "401", "unauthorized", "authentication", "session expired", "token expired", "not authenticated"))
}

// claudePaneReady mirrors the shared Claude composer contract: a current
// composer glyph owns input, and the normal footer may follow it. Historical
// auth text above that composer is not evidence of a current auth block.
func claudePaneReady(lines []string) bool {
	composer := -1
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "❯" || strings.HasPrefix(line, "❯ ") {
			composer = i
		}
	}
	if composer < 0 {
		return false
	}
	for _, line := range lines[composer+1:] {
		lower := strings.ToLower(strings.TrimSpace(line))
		if lower == "" || strings.Trim(lower, "─━┄┈╌╍ ") == "" ||
			strings.Contains(lower, "? for shortcuts") ||
			strings.Contains(lower, "shift+tab to cycle") ||
			strings.Contains(lower, "plan mode on") {
			continue
		}
		return false
	}
	return true
}

func codexPaneAuthFailed(lower string) bool {
	return strings.Contains(lower, "codex login") ||
		strings.Contains(lower, "run `codex login`") ||
		(strings.Contains(lower, "openai") && strings.Contains(lower, "api key") &&
			hasAny(lower, "missing", "not found", "unauthorized", "authentication", "not authenticated"))
}

func agyPaneAuthFailed(lower string) bool {
	return strings.Contains(lower, "gcloud auth application-default login") ||
		strings.Contains(lower, "google_application_credentials") ||
		(strings.Contains(lower, "agy") && strings.Contains(lower, "sign in") &&
			hasAny(lower, "authentication", "not authenticated", "session expired", "token expired"))
}

func recentSupervisorPaneLines(content string) []string {
	scanner := bufio.NewScanner(strings.NewReader(content))
	var lines []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, strings.ToLower(line))
		}
	}
	const recentLineLimit = 12
	if len(lines) > recentLineLimit {
		lines = lines[len(lines)-recentLineLimit:]
	}
	return lines
}

func supervisorPaneEndsAtPrompt(lines []string) bool {
	last := strings.TrimSpace(lines[len(lines)-1])
	return last == ">" || last == "›"
}

func hasAny(content string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func hasExactLine(lines []string, want string) bool {
	return slices.Contains(lines, want)
}

// heartbeatFileName maps a supervisor identity to the compact heartbeat file
// basename that AGM mirrors for the peer-check protocol.
func heartbeatFileName(name string) string {
	if member, ok := vroomsupervisor.Lookup(name); ok {
		return member.Alias
	}
	return name
}

// trailRecord is a single entry in the dispatch trail JSONL log.
type trailRecord struct {
	Timestamp string         `json:"ts"`
	Role      string         `json:"role"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload,omitempty"`
}

// writeTrail appends a single trail record to dispatch-trail.jsonl.
func writeTrail(home, kind string, payload map[string]any) {
	rec := trailRecord{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Role:      "dispatch-advisor",
		Kind:      kind,
		Payload:   payload,
	}
	data, err := json.Marshal(rec)
	if err != nil {
		return
	}

	data = append(data, '\n')
	path := filepath.Join(home, ".agm", "vroom", "dispatch-trail.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = f.Write(data) // O_APPEND makes this atomic on POSIX for writes under PIPE_BUF
	_ = f.Close()
}

// writeSelfHeartbeat writes the Dispatch Advisor's own heartbeat so
// supervisors (and humans) can verify the daemon is alive.
func writeSelfHeartbeat(home string) {
	ts := time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(home, ".agm", "vroom", "heartbeat", "dispatch.json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(ts+"\n"), 0o600); err != nil {
		return
	}
	os.Rename(tmp, path)
}

// defaultSupervisorModel is the model alias supervisors spawn with unless
// overridden by -model. It is the 200k-context Sonnet variant, deliberately
// NOT the 1M-context default and NOT Opus:
//   - Sonnet (not Opus) per the current operator directive: conserve Opus
//     quota until the cost/benefit of Opus supervisors is proven. This
//     supersedes the earlier Opus default (PR #507); Opus remains one flag
//     away via `-model=opus-200k`.
//   - The 200k variant (sonnet-200k → claude-sonnet-4-6), not the bare
//     `sonnet` alias → claude-sonnet-4-6[1m]: the 1M-context models are
//     credit-gated on this Max-plan auth, so every tick of a [1m] model fails
//     with "API Error: Usage credits required for 1M context". 200k context is
//     ample for a tick and dodges the gate (ce-84l2).
//
// Override with -model (e.g. `-model=opus-200k` to switch back to Opus). Avoid
// the bare `opus`/`sonnet` aliases: both resolve to credit-gated [1m] models.
const defaultSupervisorModel = "sonnet-200k"

// supervisorMode is the permission mode supervisors spawn with. Detached
// sessions cannot answer interactive approval prompts, so they must start in
// auto mode rather than the claude-code default of plan: a plan-mode detached
// session can plan its tick but never execute it, and exiting plan mode itself
// raises an approval prompt no detached session can self-answer (ce-84l2).
const supervisorMode = "auto"

// sessionNewArgs builds the `agm session new` argument list for spawning a
// supervisor session. Pinning --model and --mode here (rather than relying on
// agm's defaults) is the fix for ce-84l2: detached supervisors cannot clear
// approval prompts. Each supervisor also carries its canonical harness so
// recovery cannot collapse the mesh onto a single provider family (ce-2n5j).
func sessionNewArgs(sup supervisor, claudeModelOverride string) []string {
	model := sup.Model
	if sup.Harness == "claude-code" && claudeModelOverride != "" {
		model = claudeModelOverride
	}
	args := []string{
		"session", "new", sup.Name,
		"--detached", "--workspace=oss", "--harness=" + sup.Harness,
		"--model=" + model,
	}
	if supportsStartupAutoMode(sup.Harness) {
		args = append(args, "--mode="+supervisorMode)
	}
	// --role applies the matching RBAC permission profile (e.g. the
	// orchestrator profile grants `Bash(agm session new *)` so the orchestrator
	// can spawn worker sessions). Without it a supervisor gets only the default
	// permissions and cannot dispatch. (ce-7cdj follow-on)
	if sup.Role != "" {
		args = append(args, "--role="+sup.Role)
	}
	return args
}

func supportsStartupAutoMode(harness string) bool {
	return harness == "claude-code" || harness == "agy"
}

func main() {
	skillsOnly := flag.Bool("skills-only", false, "install SKILL files to ~/.agm/vroom/skills/ and exit")
	statusFlag := flag.Bool("status", false, "show supervisor mesh status and exit")
	bootOnly := flag.Bool("boot-only", false, "create sessions and send boot prompts, then exit (no tick loop)")
	model := flag.String("model", defaultSupervisorModel, "model alias supervisors spawn with (e.g. sonnet-200k, opus-200k; avoid the bare opus/sonnet aliases — they resolve to credit-gated 1M models)")
	flag.Parse()

	if *statusFlag {
		showStatus()
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}

	if *skillsOnly {
		installSkills(home)
		fmt.Println("==> SKILL files installed. Done.")
		return
	}

	// Tracing is opt-in: a no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set
	// (run `otel-local up` and `eval "$(otel-local env)"` to collect spans).
	ctx := context.Background()
	shutdown := otelsetup.InitTracer(tracerName)
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "vroom-dispatch: otel shutdown: %v\n", err)
		}
	}()

	installSkills(home)

	fmt.Printf("==> Supervisor model: %s\n", *model)

	state := loadState(home)
	ensureSessions(ctx, home, state, *model)
	saveState(home, state)

	if *bootOnly {
		fmt.Println("==> Sessions created and booted. Exiting (boot-only).")
		printStatus(state)
		return
	}

	// Persistent Dispatch Advisor mode (default): monitor supervisor health
	// and auto-restart dead sessions with exponential backoff.
	runHealthMonitor(ctx, home, state, *model)
}

// ensureSessions creates AGM sessions for any supervisor that doesn't have a live one,
// sends the boot prompt, and starts the /loop.
func ensureSessions(ctx context.Context, home string, state *sessionState, model string) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "vroom.ensure_sessions",
		trace.WithAttributes(attribute.Int("supervisor.count", len(supervisors))))
	defer span.End()

	fmt.Println("==> Ensuring supervisor sessions...")

	for _, sup := range supervisors {
		if info, ok := state.Sessions[sup.Name]; ok {
			if isSessionAlive(sup.Name) {
				fmt.Printf("    %s: alive (created %s, loop_sent=%v)\n", sup.Name, info.CreatedAt, info.LoopSent)
				if !info.LoopSent {
					if sendLoopCommand(sup) {
						info.LoopSent = true
						state.Sessions[sup.Name] = info
					}
				}
				continue
			}
			fmt.Printf("    %s: dead, recreating...\n", sup.Name)
			delete(state.Sessions, sup.Name)
		}
		createAndBootSession(ctx, home, sup, state, model)
	}
}

// minSpawnInterval mirrors agm's circuitbreaker.MinSpawnInterval (hardcoded at
// 2 minutes). agm refuses any `agm session new` that arrives within this window
// of the previous successful spawn, printing "circuit breaker: spawn refused —
// spawn too soon". createAndBootSession spawns the 3 supervisors only ~40s apart
// (its 10s + 30s post-spawn sleeps), so spawns 2 and 3 land inside the window
// and are refused — leaving just 1 of 3 supervisors up per run (ce-mu36).
const minSpawnInterval = 2 * time.Minute

// maxSpawnAttempts bounds the retry-on-refusal loop in spawnSessionWithRetry.
const maxSpawnAttempts = 3

// spawnCommandTimeout bounds one `agm session new` subprocess. A hung spawn
// must not stall the supervisor recovery loop indefinitely.
const spawnCommandTimeout = 5 * time.Minute

// spawnTooSoonMarker is the substring agm prints when its spawn circuit breaker
// refuses a too-soon spawn. Matching on the message (rather than an exit code)
// keeps us decoupled from agm's internal error taxonomy.
const spawnTooSoonMarker = "spawn too soon"

// governorPauseMarker is the stable AGM diagnostic for the other transient
// spawn-stagger condition. It is intentionally narrower than "resource
// governor" so unrelated governor safety failures do not become retryable.
const governorPauseMarker = "spawns paused by resource governor"

// governorAdmissionMarker introduces the machine-parseable RFC3339 boundary
// in AGM's governor-pause diagnostic.
const governorAdmissionMarker = "earliest possible admission is "

// spawnRetryNow is the clock used to turn AGM's advertised governor boundary
// into a delay. It is injectable so the retry contract has deterministic tests.
var spawnRetryNow = time.Now

func isRetryableSpawnRefusal(output string) bool {
	hasTransientMarker := strings.Contains(output, spawnTooSoonMarker) ||
		strings.Contains(output, governorPauseMarker)
	if !hasTransientMarker {
		return false
	}

	// AGM's FormatDenied output emits one bullet for every failed gate. A
	// spawn-stagger pause can coexist with a hard safety denial, so checking for
	// any transient marker would incorrectly retry disk, process-cap, or
	// admission-brake failures. If structured gate bullets are present, require
	// every failed gate to be the recognized transient spawn-stagger gate.
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "• [") {
			continue
		}
		closeBracket := strings.IndexByte(line, ']')
		if closeBracket < len("• [") {
			return false
		}
		if line[len("• ["):closeBracket] != "spawn_stagger" ||
			(!strings.Contains(line, spawnTooSoonMarker) && !strings.Contains(line, governorPauseMarker)) {
			return false
		}
	}

	// Retain compatibility with the older single-line AGM diagnostic while
	// enforcing the complete gate set whenever current structured output is
	// available.
	return true
}

func spawnRetryDelay(output string) time.Duration {
	if strings.Contains(output, governorPauseMarker) {
		if _, suffix, ok := strings.Cut(output, governorAdmissionMarker); ok {
			fields := strings.Fields(suffix)
			if len(fields) > 0 {
				if boundary, err := time.Parse(time.RFC3339, fields[0]); err == nil {
					if remaining := boundary.Sub(spawnRetryNow()); remaining > 0 {
						return remaining
					}
				}
			}
		}
	}
	return minSpawnInterval
}

// runSpawn executes `agm session new` for one supervisor and returns the
// combined output. It is a package var so tests can stub the spawn without
// shelling out to agm.
var runSpawn = func(sup supervisor, model string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), spawnCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "agm", sessionNewArgs(sup, model)...)
	// ce-v9in: mark the whole spawned session tree as unattended so every
	// `agm send` it makes (including peer-to-peer mesh sends) auto-stashes its
	// own stale input (C-s) instead of deadlocking on it as if a human were typing.
	cmd.Env = append(scrubAPIKey(os.Environ()), "AGM_AUTONOMOUS=1")
	return cmd.CombinedOutput()
}

var runArchiveSupervisor = func(sup supervisor) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), spawnCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", sessionArchiveArgs(sup)...)
	cmd.Env = append(scrubAPIKey(os.Environ()), "AGM_DISPATCHER_SUPERVISOR_REAP=1")
	return cmd.CombinedOutput()
}

func sessionArchiveArgs(sup supervisor) []string {
	return []string{"session", "archive", "--async", "--outcome", "crashed", sup.Name}
}

// sleepFor is the backoff sleep used by spawnSessionWithRetry. It is a package
// var so tests can avoid waiting out the real admission window.
var sleepFor = time.Sleep

// spawnSessionWithRetry runs `agm session new` for a supervisor, retrying the
// two transient spawn-stagger refusals: a recent successful spawn and a
// governor-owned pause. It waits the fixed spawn interval for a recent spawn
// and the advertised earliest admission boundary for a governor pause, up to
// maxSpawnAttempts. A missing, malformed, or expired governor boundary falls
// back to the fixed interval. Any other failure (or success) returns
// immediately.
func spawnSessionWithRetry(sup supervisor, model string) error {
	var lastErr error
	for attempt := 1; attempt <= maxSpawnAttempts; attempt++ {
		output, err := runSpawn(sup, model)
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
		if !isRetryableSpawnRefusal(string(output)) {
			return lastErr
		}
		if attempt < maxSpawnAttempts {
			delay := spawnRetryDelay(string(output))
			fmt.Printf("\n    %s: spawn refused by circuit breaker (attempt %d/%d); waiting %s for the spawn window to clear... ",
				sup.Name, attempt, maxSpawnAttempts, delay)
			sleepFor(delay)
		}
	}
	return fmt.Errorf("circuit breaker still refusing after %d attempts: %w", maxSpawnAttempts, lastErr)
}

func archiveAuthFailedSupervisor(sup supervisor) error {
	deadline := time.Now().Add(spawnCommandTimeout)
	output, err := runArchiveSupervisor(sup)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if isSessionArchived(context.Background(), sup.Name, remaining) {
			return nil
		}
		if remaining <= 0 {
			break
		}
		sleepFor(minDuration(2*time.Second, remaining))
	}
	return fmt.Errorf("auth-failed supervisor %s remained active after archive", sup.Name)
}

// isSessionArchived waits on the durable lifecycle state, not merely tmux
// disappearance. The detached reaper closes tmux before it commits archive
// cleanup, so liveness alone can race duplicate-name admission.
func isSessionArchived(parent context.Context, name string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", "session", "list", "--all", "--json", "--fields", "name,status")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	var result struct {
		Sessions []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return false
	}
	for _, session := range result.Sessions {
		if session.Name == name {
			return session.Status == "archived"
		}
	}
	return false
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// createAndBootSession spawns one supervisor session and walks it through the
// create → boot → loop lifecycle, each step in its own span so a trace shows
// where spawn time goes (the two fixed sleeps dominate) and which step failed.
func createAndBootSession(ctx context.Context, home string, sup supervisor, state *sessionState, model string) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, "vroom.session.spawn",
		trace.WithAttributes(
			attribute.String("supervisor.name", sup.Name),
			attribute.String("supervisor.model", model),
			attribute.String("supervisor.mode", supervisorMode),
		))
	defer span.End()

	fmt.Printf("    %s: creating... ", sup.Name)

	_, createSpan := otel.Tracer(tracerName).Start(ctx, "vroom.session.create")
	if err := spawnSessionWithRetry(sup, model); err != nil {
		createSpan.RecordError(err)
		createSpan.SetStatus(codes.Error, err.Error())
		createSpan.End()
		span.SetStatus(codes.Error, "session create failed")
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	createSpan.End()
	fmt.Println("created")

	time.Sleep(10 * time.Second)

	func() {
		_, bootSpan := otel.Tracer(tracerName).Start(ctx, "vroom.session.boot")
		defer bootSpan.End()
		sendBootPrompt(home, sup)
		// Wait for boot prompt to be processed before starting loop.
		fmt.Printf("    %s: waiting 30s for boot prompt processing...\n", sup.Name)
		time.Sleep(30 * time.Second)
	}()

	var loopSent bool
	func() {
		_, loopSpan := otel.Tracer(tracerName).Start(ctx, "vroom.session.loop")
		defer loopSpan.End()
		loopSent = sendLoopCommand(sup)
	}()

	state.Sessions[sup.Name] = sessionInfo{
		Name:      sup.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		LoopSent:  loopSent,
	}
}

// sendBootPrompt sends the full SKILL + protocol content as the initial message.
// This large static payload gets cached by Claude's prompt cache, making subsequent
// /loop tick iterations cheap (the boot context is already in the conversation).
func sendBootPrompt(home string, sup supervisor) {
	protocolData, err := os.ReadFile(filepath.Join(home, ".agm", "vroom", "skills", "protocol.md"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "    %s: read protocol: %v\n", sup.Name, err)
		return
	}
	skillData, err := os.ReadFile(filepath.Join(home, ".agm", "vroom", "skills", sup.SkillFile))
	if err != nil {
		fmt.Fprintf(os.Stderr, "    %s: read skill: %v\n", sup.Name, err)
		return
	}

	bootPrompt := fmt.Sprintf(`You are %s, a VROOM supervisor running as a persistent session.

Your session stays alive across ticks. After this setup, a /loop command
will be sent that runs your tick behavior at a fixed interval. Each iteration
of the loop should execute your role's tick steps as defined below.

=== SHARED PROTOCOL ===
%s

=== YOUR ROLE ===
%s

=== INITIAL SETUP ===
1. Run: mkdir -p ~/.agm/vroom/heartbeat
2. Write your initial heartbeat: agm supervisor heartbeat --id %s --primary-for %s --tertiary-for %s
3. Confirm you are ready and summarize your role.

A /loop command will start your tick cycle shortly.`,
		sup.ID,
		string(protocolData),
		string(skillData),
		sup.ID, sup.PrimaryFor, sup.TertiaryFor)

	fmt.Printf("    %s: sending boot prompt (%d bytes)...\n", sup.Name, len(bootPrompt))
	cmd := exec.Command("agm", "send", "msg", sup.Name,
		"--sender", "vroom-dispatch",
		"--autonomous",
		"--prompt", bootPrompt)
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    %s: boot send failed: %v\n%s\n", sup.Name, err, string(output))
	} else {
		fmt.Printf("    %s: boot prompt sent\n", sup.Name)
	}
}

// tickResilienceGuard frames every supervisor tick so a single failing iteration
// can never kill the /loop. Without it the happy-path tick prompt lets any error —
// an Anthropic API/credit-gate error, a tool failure, or a transient fault — abort
// the turn; if that turn is the one that arms (or re-arms) the loop schedule, the
// loop halts and never re-fires, leaving the supervisor silently idle (ce-ihok).
// The guard tells the agent to treat a failed tick as a *skipped* tick: log it and
// finish the turn normally so the next interval still fires. It is prepended to the
// role's tick steps so it frames the whole tick, and is identical across all three
// supervisors — hence it lives here, not in each role's TickPrompt.
const tickResilienceGuard = "RESILIENT LOOP — this tick must never end the loop: " +
	"if any step fails for any reason (Anthropic API or credit-gate error, tool failure, " +
	"or transient fault), do NOT stop, exit, or abort the loop. Finish this turn normally " +
	"so the next interval still fires, and treat a failed tick as a skipped tick, never as " +
	"a reason to abort. Best-effort only: note the failure in ~/.agm/vroom/trail.jsonl " +
	"(kind \"supervisor.tick.error\") if you can, but never let logging itself end the loop."

// tickIntervalArg renders a supervisor's tick interval as the /loop interval token (e.g. "90s").
func tickIntervalArg(sup supervisor) string {
	return fmt.Sprintf("%ds", int(sup.TickInterval.Seconds()))
}

// buildLoopCommand constructs the /loop slash command sent to a supervisor session.
// Shape: /loop <interval> <resilience-guard> <role-tick-steps> Report a brief summary when done.
// Extracted (and the guard injected) so the error-tolerance contract is unit-testable.
func buildLoopCommand(sup supervisor) string {
	return fmt.Sprintf("/loop %s %s %s Report a brief summary when done.",
		tickIntervalArg(sup), tickResilienceGuard, sup.TickPrompt)
}

// sendLoopCommand sends a /loop slash command to a supervisor session. The session's
// built-in /loop handler takes over tick scheduling — no external tick dispatch needed.
// The tick prompt references the SKILL instructions already loaded in the boot prompt.
// Returns true if the send succeeded, false otherwise (callers must not set loop_sent=true on failure).
func sendLoopCommand(sup supervisor) bool {
	intervalStr := tickIntervalArg(sup)
	loopCmd := buildLoopCommand(sup)

	fmt.Printf("    %s: sending /loop (%s interval)...\n", sup.Name, intervalStr)
	cmd := exec.Command("agm", "send", "msg", sup.Name,
		"--sender", "vroom-dispatch",
		"--autonomous",
		"--prompt", loopCmd)
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    %s: /loop send failed: %v\n%s\n", sup.Name, err, string(output))
		return false
	}
	fmt.Printf("    %s: /loop started\n", sup.Name)
	return true
}

// runHealthMonitor is the Dispatch Advisor's persistent daemon loop. It
// checks supervisor health every 30s, classifying each as alive/stale/dead
// based on both session liveness and heartbeat freshness. Dead supervisors
// are restarted with exponential backoff. After 3 consecutive restart
// failures for the same supervisor, it calls escalateToHuman, which records the
// escalation and notifies the operator via desktop notification + MCP push
// (ce-mcw2).
func runHealthMonitor(parent context.Context, home string, state *sessionState, model string) {
	ctx, cancel := signal.NotifyContext(parent, os.Interrupt)
	defer cancel()

	tracker := newRestartTracker()
	wTracker := newWorkerTracker()
	var stallStartTime time.Time
	var flowLivenessEscalated bool

	fmt.Println("==> Dispatch Advisor running (Ctrl-C to stop)...")
	printStatus(state)
	fmt.Printf("==> Monitoring supervisor health every %s...\n", healthCheckInterval)

	writeTrail(home, "dispatch.started", nil)
	writeSelfHeartbeat(home)

	for {
		select {
		case <-ctx.Done():
			writeTrail(home, "dispatch.shutdown", nil)
			writeSelfHeartbeat(home)
			saveState(home, state)
			fmt.Println("==> Dispatch Advisor stopped.")
			return
		case <-time.After(healthCheckInterval):
		}

		writeSelfHeartbeat(home)

		// Check for stuck workers before supervisor health — reclaiming
		// worker slots lets the Orchestrator dispatch on the same tick.
		activeWorkers, err := monitorWorkers(ctx, home, wTracker)
		if err == nil {
			_, flErr := checkFlowLiveness(ctx, home, activeWorkers, &stallStartTime, &flowLivenessEscalated, 15*time.Minute)
			if flErr != nil {
				fmt.Fprintf(os.Stderr, "watchdog: flow liveness check failed: %v\n", flErr)
			}
		} else {
			fmt.Fprintf(os.Stderr, "watchdog: failed to monitor workers: %v\n", err)
		}

		for _, sup := range supervisors {
			health := classifySupervisor(home, sup)

			switch health {
			case healthAlive:
				if tracker.consecutiveRestarts(sup.Name) > 0 {
					fmt.Printf("[%s] recovered at %s\n", sup.Name, time.Now().Format("15:04:05"))
					writeTrail(home, "dispatch.supervisor_recovered", map[string]any{
						"supervisor": sup.Name,
					})
				}
				tracker.recordRecovery(sup.Name)

			case healthStale:
				writeTrail(home, "dispatch.supervisor_stale", map[string]any{
					"supervisor": sup.Name,
					"heartbeat":  heartbeatFileName(sup.Name),
				})

			case healthAuthFailed:
				ok, count := tracker.shouldRestart(sup.Name)
				if !ok {
					if tracker.shouldEscalate(sup.Name) {
						msg := fmt.Sprintf("%s: %d consecutive auth-failed restarts — needs human intervention",
							sup.Name, count)
						escalateToHuman(home, "restart_exhausted", msg, map[string]any{
							"supervisor": sup.Name,
							"restarts":   count,
							"reason":     "auth_failed",
						})
						fmt.Printf("[%s] ESCALATION: %s\n", sup.Name, msg)
					}
					continue
				}

				fmt.Printf("[%s] auth failed at %s (attempt %d/%d), archiving and restarting...\n",
					sup.Name, time.Now().Format("15:04:05"), count+1, maxRestarts)
				writeTrail(home, "dispatch.supervisor_auth_failed", map[string]any{
					"supervisor": sup.Name,
					"harness":    sup.Harness,
					"attempt":    count + 1,
				})

				tracker.recordAttempt(sup.Name)
				if err := archiveAuthFailedSupervisor(sup); err != nil {
					fmt.Fprintf(os.Stderr, "[%s] auth-failed archive failed: %v\n", sup.Name, err)
					writeTrail(home, "dispatch.supervisor_auth_archive_failed", map[string]any{
						"supervisor": sup.Name,
						"error":      err.Error(),
					})
					continue
				}
				delete(state.Sessions, sup.Name)
				recreateCtx, recreateSpan := otel.Tracer(tracerName).Start(ctx, "vroom.session.recreate",
					trace.WithAttributes(attribute.String("supervisor.name", sup.Name), attribute.String("supervisor.restart_reason", "auth_failed")))
				createAndBootSession(recreateCtx, home, sup, state, model)
				recreateSpan.End()
				saveState(home, state)

			case healthDead:
				ok, count := tracker.shouldRestart(sup.Name)
				if !ok {
					if tracker.shouldEscalate(sup.Name) {
						msg := fmt.Sprintf("%s: %d consecutive restart failures — needs human intervention",
							sup.Name, count)
						escalateToHuman(home, "restart_exhausted", msg, map[string]any{
							"supervisor": sup.Name,
							"restarts":   count,
						})
						fmt.Printf("[%s] ESCALATION: %s\n", sup.Name, msg)
					}
					continue
				}

				fmt.Printf("[%s] dead at %s (attempt %d/%d), restarting...\n",
					sup.Name, time.Now().Format("15:04:05"), count+1, maxRestarts)
				writeTrail(home, "dispatch.restarting_supervisor", map[string]any{
					"supervisor": sup.Name,
					"attempt":    count + 1,
				})

				tracker.recordAttempt(sup.Name)
				delete(state.Sessions, sup.Name)
				// Each recreation is its own trace root: a long-lived monitor
				// must not accumulate children under one ever-open span.
				recreateCtx, recreateSpan := otel.Tracer(tracerName).Start(ctx, "vroom.session.recreate",
					trace.WithAttributes(attribute.String("supervisor.name", sup.Name)))
				createAndBootSession(recreateCtx, home, sup, state, model)
				recreateSpan.End()
				saveState(home, state)
			}
		}
	}
}

// workerState tracks per-worker progress across Dispatch Advisor ticks.
type workerState struct {
	lastSeenUpdateAt string // last_update_at from previous tick (RFC3339)
	escalationLevel  int    // 0=healthy, 1=nudged, 2=diagnosed, 3=killed
	staleFor         int    // consecutive ticks with no progress
}

// workerTracker tracks per-worker escalation state across ticks.
type workerTracker struct {
	mu      sync.Mutex
	workers map[string]*workerState
}

func newWorkerTracker() *workerTracker {
	return &workerTracker{workers: make(map[string]*workerState)}
}

// healthEntry mirrors the per-session fields from `agm session health --all --json`.
type healthEntry struct {
	Name                string `json:"name"`
	State               string `json:"state"`
	Status              string `json:"status"`
	TimeSinceLastUpdate string `json:"time_since_last_update"`
	LastUpdateAt        string `json:"last_update_at"`
	Health              string `json:"health"`
	CommitCount         int    `json:"commit_count"`
}

type healthResult struct {
	Sessions []healthEntry `json:"sessions"`
}

// monitorWorkers uses graduated escalation to handle stuck workers. It
// compares each worker's last_update_at across ticks to distinguish "slow
// but working" (progressing) from "stuck" (zero progress + stuck state).
// Force-kill is a last resort, only for workers provably stuck.
// Returns the count of active workers and any error listing them.
func monitorWorkers(parent context.Context, home string, wt *workerTracker) (int, error) {
	ctx, cancel := context.WithTimeout(parent, workerHealthProbeTimeout)
	defer cancel()

	out, err := flowProbeOutput(ctx, "agm", "session", "health", "--all", "--json")
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("agm session health canceled: %w", ctx.Err())
		}
		return 0, fmt.Errorf("run agm session health: %w", err)
	}

	var result healthResult
	if err := json.Unmarshal(out, &result); err != nil {
		return 0, fmt.Errorf("decode agm session health: %w", err)
	}

	wt.mu.Lock()
	defer wt.mu.Unlock()

	activeCount := 0
	seen := make(map[string]bool)
	for _, entry := range result.Sessions {
		if !strings.HasPrefix(entry.Name, "worker-") || entry.Status != "active" {
			continue
		}
		activeCount++
		seen[entry.Name] = true
		ws, exists := wt.workers[entry.Name]
		if !exists {
			ws = &workerState{}
			wt.workers[entry.Name] = ws
		}
		escalateIfStuck(home, entry, ws, exists)
	}

	for name := range wt.workers {
		if !seen[name] {
			delete(wt.workers, name)
		}
	}
	return activeCount, nil
}

// countReadyBeadsFunc retrieves the number of ready beads. Overridable in tests.
var countReadyBeadsFunc = defaultCountReadyBeads

var flowProbeOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func defaultCountReadyBeads(parent context.Context, home string) (int, error) {
	ctx, cancel := context.WithTimeout(parent, readyBeadsProbeTimeout)
	defer cancel()

	dbPath := filepath.Join(home, "beads", "context-engine", ".beads")
	out, err := flowProbeOutput(ctx, "bd", "--db", dbPath, "ready", "--json")
	if err != nil {
		if ctx.Err() != nil {
			return 0, fmt.Errorf("bd ready canceled: %w", ctx.Err())
		}
		return 0, fmt.Errorf("run bd ready: %w", err)
	}
	var ready []any
	if err := json.Unmarshal(out, &ready); err != nil {
		return 0, fmt.Errorf("decode bd ready: %w", err)
	}
	return len(ready), nil
}

// checkFlowLiveness checks if ready_beads > 0 && active_workers == 0 has persisted
// for the threshold (15m), and triggers escalation if so.
// Returns escalated=true if it triggered an escalation in this tick.
func checkFlowLiveness(ctx context.Context, home string, activeWorkers int, stallStartTime *time.Time, escalated *bool, threshold time.Duration) (bool, error) {
	readyCount, err := countReadyBeadsFunc(ctx, home)
	if err != nil {
		return false, fmt.Errorf("failed to count ready beads: %w", err)
	}

	if readyCount > 0 && activeWorkers == 0 {
		if stallStartTime.IsZero() {
			*stallStartTime = time.Now()
		} else if time.Since(*stallStartTime) >= threshold && !*escalated {
			msg := fmt.Sprintf("Flywheel stall detected: %d ready beads but 0 active workers for >%v", readyCount, threshold)
			escalateToHuman(home, "flow_liveness_stall", msg, map[string]any{
				"ready_beads":    readyCount,
				"active_workers": 0,
				"duration":       time.Since(*stallStartTime).String(),
			})
			fmt.Printf("[watchdog] ESCALATION: %s\n", msg)
			*escalated = true
			return true, nil
		}
	} else {
		*stallStartTime = time.Time{}
		*escalated = false
	}
	return false, nil
}

// escalateIfStuck applies graduated escalation (nudge → diagnose → kill) to
// one worker based on how long it has had no manifest progress.
func escalateIfStuck(home string, entry healthEntry, ws *workerState, exists bool) {
	progressing := ws.lastSeenUpdateAt != "" && entry.LastUpdateAt != ws.lastSeenUpdateAt
	ws.lastSeenUpdateAt = entry.LastUpdateAt

	if progressing || !exists {
		ws.escalationLevel = 0
		ws.staleFor = 0
		return
	}

	ws.staleFor++
	staleDuration := parseDuration(entry.TimeSinceLastUpdate)
	isStuckState := entry.State == "PERMISSION_PROMPT" || entry.State == "OFFLINE"

	if staleDuration >= workerNudgeAfter && ws.escalationLevel < 1 {
		ws.escalationLevel = 1
		fmt.Printf("[%s] no progress for %s, sending status ping (Level 1)\n",
			entry.Name, entry.TimeSinceLastUpdate)
		sendWorkerMessage(entry.Name, "normal",
			"status? No manifest activity in >20min. Report progress or blockers.")
		writeTrail(home, "dispatch.worker_nudged", map[string]any{
			"worker":    entry.Name,
			"state":     entry.State,
			"stale_for": entry.TimeSinceLastUpdate,
		})
		return
	}

	if staleDuration >= workerDiagnoseAfter && ws.escalationLevel < 2 {
		ws.escalationLevel = 2
		applyDiagnoseEscalation(home, entry, isStuckState)
		return
	}

	if staleDuration >= workerKillAfter && ws.escalationLevel >= 2 && isStuckState {
		ws.escalationLevel = 3
		applyKillEscalation(home, entry, ws)
	}
}

func applyDiagnoseEscalation(home string, entry healthEntry, isStuckState bool) {
	if isStuckState {
		fmt.Printf("[%s] %s for %s with no progress, sending defer nudge (Level 2)\n",
			entry.Name, entry.State, entry.TimeSinceLastUpdate)
		sendWorkerMessage(entry.Name, "urgent",
			"You appear stuck on a permission prompt. Defer the blocked action (file a handoff note) and continue with other work.")
	} else {
		fmt.Printf("[%s] no progress for %s, sending wrap-up (Level 2)\n",
			entry.Name, entry.TimeSinceLastUpdate)
		sendWorkerMessage(entry.Name, "urgent",
			"No manifest activity for >35min. Commit any WIP now, report status, or wrap up.")
	}
	writeTrail(home, "dispatch.worker_diagnosed", map[string]any{
		"worker":    entry.Name,
		"state":     entry.State,
		"stale_for": entry.TimeSinceLastUpdate,
		"action":    map[bool]string{true: "defer_nudge", false: "wrap_up"}[isStuckState],
	})
}

func applyKillEscalation(home string, entry healthEntry, ws *workerState) {
	fmt.Printf("[%s] %s for %s, no progress, prior nudge failed — force-killing (Level 3)\n",
		entry.Name, entry.State, entry.TimeSinceLastUpdate)
	killCmd := exec.Command("agm", "session", "kill", entry.Name, "--confirmed-stuck")
	if killOut, killErr := killCmd.CombinedOutput(); killErr != nil {
		fmt.Fprintf(os.Stderr, "[%s] kill failed: %v\n%s\n", entry.Name, killErr, string(killOut))
		writeTrail(home, "dispatch.worker_kill_failed", map[string]any{
			"worker": entry.Name,
			"state":  entry.State,
			"error":  killErr.Error(),
		})
	} else {
		fmt.Printf("[%s] killed successfully — slot reclaimed\n", entry.Name)
		writeTrail(home, "dispatch.worker_killed_stuck", map[string]any{
			"worker":      entry.Name,
			"state":       entry.State,
			"stale_for":   entry.TimeSinceLastUpdate,
			"stale_ticks": ws.staleFor,
		})
	}
}

// sendWorkerMessage sends an agm message to a worker session.
func sendWorkerMessage(worker, priority, prompt string) {
	cmd := exec.Command("agm", "send", "msg", worker,
		"--sender", "vroom-dispatch",
		"--priority", priority,
		"--prompt", prompt)
	cmd.Env = scrubAPIKey(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] send failed: %v\n%s\n", worker, err, string(out))
	}
}

// parseDuration converts the dashboard's human-friendly duration strings
// (e.g. "45s", "12m", "2h30m", "3d") into time.Duration.
func parseDuration(s string) time.Duration {
	if s == "" || s == "-" {
		return 0
	}
	// Try stdlib parsing first (handles "12m", "2h30m", "45s").
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	// Handle "3d" format.
	if rest, ok := strings.CutSuffix(s, "d"); ok {
		var days int
		if _, err := fmt.Sscanf(rest, "%d", &days); err == nil {
			return time.Duration(days) * 24 * time.Hour
		}
	}
	return 0
}

// isSessionAlive checks if an AGM session with the exact given name exists.
// It matches the name as a whole whitespace-delimited token rather than a
// substring, so a worker session whose name merely contains a supervisor's
// name (e.g. "vroom-orchestrator-worker-1") cannot mask a dead supervisor session.
func isSessionAlive(name string) bool {
	cmd := exec.Command("agm", "session", "list")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	outStr := strings.TrimSpace(string(out))
	if strings.HasPrefix(outStr, "{") {
		var res struct {
			Sessions []struct {
				Name string `json:"name"`
			} `json:"sessions"`
		}
		if err := json.Unmarshal([]byte(outStr), &res); err == nil {
			for _, s := range res.Sessions {
				if s.Name == name {
					return true
				}
			}
			return false
		}
	}
	for line := range strings.SplitSeq(outStr, "\n") {
		if slices.Contains(strings.Fields(line), name) {
			return true
		}
	}
	return false
}

func loadState(home string) *sessionState {
	path := filepath.Join(home, sessionsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return &sessionState{Sessions: make(map[string]sessionInfo)}
	}
	var s sessionState
	if err := json.Unmarshal(data, &s); err != nil {
		return &sessionState{Sessions: make(map[string]sessionInfo)}
	}
	if s.Sessions == nil {
		s.Sessions = make(map[string]sessionInfo)
	}
	return &s
}

func saveState(home string, state *sessionState) {
	state.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch: save state: %v\n", err)
		return
	}
	path := filepath.Join(home, sessionsFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch: mkdir: %v\n", err)
		return
	}
	// Write to a temp file then rename so a crash mid-write cannot leave a
	// truncated or corrupt sessions.json behind (rename is atomic on POSIX).
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch: write state: %v\n", err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch: rename state: %v\n", err)
		_ = os.Remove(tmp)
	}
}

func installSkills(home string) {
	fmt.Println("==> Installing SKILL files...")

	dirs := []string{
		filepath.Join(home, ".agm", "vroom", "heartbeat"),
		filepath.Join(home, ".agm", "vroom", "skills"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fatal("mkdir %s: %v", d, err)
		}
	}

	entries, err := skills.ReadDir("skills")
	if err != nil {
		fatal("read embedded skills: %v", err)
	}
	for _, e := range entries {
		data, err := skills.ReadFile("skills/" + e.Name())
		if err != nil {
			fatal("read %s: %v", e.Name(), err)
		}
		dest := filepath.Join(home, ".agm", "vroom", "skills", e.Name())
		if err := os.WriteFile(dest, data, 0o600); err != nil {
			fatal("write %s: %v", dest, err)
		}
		fmt.Printf("    %s\n", dest)
	}
}

func showStatus() {
	cmd := exec.Command("agm", "supervisor", "status")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("\nSession status:")
		for _, sup := range supervisors {
			alive := isSessionAlive(sup.Name)
			status := "NOT FOUND"
			if alive {
				status = "alive"
			}
			fmt.Printf("    %-25s %s\n", sup.Name, status)
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		state := loadState(home)
		if len(state.Sessions) > 0 {
			fmt.Printf("\nPersistent state (updated %s):\n", state.UpdatedAt)
			for name, info := range state.Sessions {
				fmt.Printf("    %-25s created=%s loop_sent=%v\n", name, info.CreatedAt, info.LoopSent)
			}
		}
	}

	trail := filepath.Join(os.Getenv("HOME"), ".agm", "vroom", "trail.jsonl")
	if info, err := os.Stat(trail); err == nil {
		fmt.Printf("\nTrail: %s (%d bytes)\n", trail, info.Size())
		printTrailTail(trail, 3)
	}
}

func printStatus(state *sessionState) {
	fmt.Println()
	fmt.Println("    VROOM Supervisor Mesh — Persistent Sessions (/loop)")
	fmt.Println()
	for _, sup := range supervisors {
		info := state.Sessions[sup.Name]
		alive := isSessionAlive(sup.Name)
		status := "DEAD"
		if alive {
			status = "alive"
		}
		fmt.Printf("    %-25s %s (loop_sent=%v, interval=%s)\n", sup.Name+":", status, info.LoopSent, sup.TickInterval)
	}
	fmt.Println()
	fmt.Println("Monitor:")
	fmt.Println("    agm supervisor status")
	fmt.Println("    agm session list")
	fmt.Println("    tail -f ~/.agm/vroom/trail.jsonl")
	fmt.Println()
	fmt.Println("Talk to a supervisor:")
	fmt.Println("    agm send msg vroom-meta-orchestrator --prompt \"status?\"")
}

func printTrailTail(path string, n int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	for _, line := range lines {
		fmt.Println(line)
	}
}

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

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vroom-dispatch: "+format+"\n", args...)
	os.Exit(1)
}
