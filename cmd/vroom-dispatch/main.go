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
)

//go:embed skills/*.md
var skills embed.FS

type supervisor struct {
	Name         string
	ID           string
	Role         string // RBAC role/profile (agm --role); grants the session's permission profile
	SkillFile    string
	PrimaryFor   string
	TertiaryFor  string
	TickInterval time.Duration
	TickPrompt   string
}

var supervisors = []supervisor{
	{
		Name:         "vroom-meta-orchestrator",
		ID:           "vroom-meta-orchestrator",
		Role:         "meta-orchestrator",
		SkillFile:    "meta-orchestrator.md",
		PrimaryFor:   "vroom-orchestrator",
		TertiaryFor:  "vroom-overseer",
		TickInterval: 180 * time.Second,
		TickPrompt: "Execute your Meta-Orchestrator tick: " +
			"1) check peer heartbeats 2) read open beads via bd --db ~/beads/context-engine/.beads list --state=open --format=json " +
			"3) read current roadmap 4) evaluate new beads for roadmap 5) verify Orch dispatch activity " +
			"6) write heartbeat.",
	},
	{
		Name:         "vroom-orchestrator",
		ID:           "vroom-orchestrator",
		Role:         "orchestrator",
		SkillFile:    "orchestrator.md",
		PrimaryFor:   "vroom-overseer",
		TertiaryFor:  "vroom-meta-orchestrator",
		TickInterval: 90 * time.Second,
		TickPrompt: "Execute your Orchestrator tick: " +
			"1) check peer heartbeats 2) read accepted roadmap items " +
			"3) dispatch undispatched work to worker sessions via agm session new " +
			"4) monitor active workers 5) detect stale items " +
			"6) write heartbeat.",
	},
	{
		Name:         "vroom-overseer",
		ID:           "vroom-overseer",
		Role:         "overseer",
		SkillFile:    "overseer.md",
		PrimaryFor:   "vroom-meta-orchestrator",
		TertiaryFor:  "vroom-orchestrator",
		TickInterval: 60 * time.Second,
		TickPrompt: "Execute your Overseer tick: " +
			"1) check peer heartbeats 2) probe system resources (disk, memory, FDs, gopls) " +
			"3) audit session health 4) reconcile stale beads " +
			"5) audit worktrees 6) verify Meta-O activity " +
			"7) write heartbeat.",
	},
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
	healthAlive supervisorHealth = iota
	healthStale                  // heartbeat old but session exists
	healthDead                   // no session or session archived
)

func (h supervisorHealth) String() string {
	switch h {
	case healthAlive:
		return "alive"
	case healthStale:
		return "stale"
	case healthDead:
		return "dead"
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

// heartbeatFileName maps a supervisor name to its heartbeat file basename
// (without extension). The skill files write to meta-o.json, orch.json,
// overseer.json.
func heartbeatFileName(name string) string {
	switch name {
	case "vroom-meta-orchestrator":
		return "meta-o"
	case "vroom-orchestrator":
		return "orch"
	case "vroom-overseer":
		return "overseer"
	default:
		return name
	}
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
// agm's defaults) is the fix for ce-84l2: the defaults are sonnet at 1M context
// (credit-gated) in plan mode (non-executable when detached). The model is
// supplied by the caller (default defaultSupervisorModel, overridable via
// -model); mode is always auto.
func sessionNewArgs(name, model, role string) []string {
	args := []string{
		"session", "new", name,
		"--detached", "--workspace=oss", "--harness=claude-code",
		"--model=" + model,
		"--mode=" + supervisorMode,
	}
	// --role applies the matching RBAC permission profile (e.g. the
	// orchestrator profile grants `Bash(agm session new *)` so the orchestrator
	// can spawn worker sessions). Without it a supervisor gets only the default
	// permissions and cannot dispatch. (ce-7cdj follow-on)
	if role != "" {
		args = append(args, "--role="+role)
	}
	return args
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

	installSkills(home)

	fmt.Printf("==> Supervisor model: %s\n", *model)

	state := loadState(home)
	ensureSessions(home, state, *model)
	saveState(home, state)

	if *bootOnly {
		fmt.Println("==> Sessions created and booted. Exiting (boot-only).")
		printStatus(state)
		return
	}

	// Persistent Dispatch Advisor mode (default): monitor supervisor health
	// and auto-restart dead sessions with exponential backoff.
	runHealthMonitor(home, state, *model)
}

// ensureSessions creates AGM sessions for any supervisor that doesn't have a live one,
// sends the boot prompt, and starts the /loop.
func ensureSessions(home string, state *sessionState, model string) {
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
		createAndBootSession(home, sup, state, model)
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

// spawnTooSoonMarker is the substring agm prints when its spawn circuit breaker
// refuses a too-soon spawn. Matching on the message (rather than an exit code)
// keeps us decoupled from agm's internal error taxonomy.
const spawnTooSoonMarker = "spawn too soon"

// runSpawn executes `agm session new` for one supervisor and returns the
// combined output. It is a package var so tests can stub the spawn without
// shelling out to agm.
var runSpawn = func(sup supervisor, model string) ([]byte, error) {
	cmd := exec.Command("agm", sessionNewArgs(sup.Name, model, sup.Role)...)
	// ce-v9in: mark the whole spawned session tree as unattended so every
	// `agm send` it makes (including peer-to-peer mesh sends) auto-stashes its
	// own stale input (C-s) instead of deadlocking on it as if a human were typing.
	cmd.Env = append(scrubAPIKey(os.Environ()), "AGM_AUTONOMOUS=1")
	return cmd.CombinedOutput()
}

// sleepFor is the backoff sleep used by spawnSessionWithRetry. It is a package
// var so tests can avoid waiting out the real 2-minute window.
var sleepFor = time.Sleep

// spawnSessionWithRetry runs `agm session new` for a supervisor, retrying when
// agm's circuit breaker refuses the spawn as "spawn too soon" (ce-mu36). On a
// refusal it waits out the full minSpawnInterval window before retrying, up to
// maxSpawnAttempts times. Waiting the full window (rather than only the
// remainder) keeps the fix simple and safe: we don't track the prior spawn's
// timestamp, and an over-wait only costs time, never correctness. Any other
// failure (or success) returns immediately — only the breaker refusal retries.
func spawnSessionWithRetry(sup supervisor, model string) error {
	var lastErr error
	for attempt := 1; attempt <= maxSpawnAttempts; attempt++ {
		output, err := runSpawn(sup, model)
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
		if !strings.Contains(string(output), spawnTooSoonMarker) {
			return lastErr
		}
		if attempt < maxSpawnAttempts {
			fmt.Printf("\n    %s: spawn refused by circuit breaker (attempt %d/%d); waiting %s for the spawn window to clear... ",
				sup.Name, attempt, maxSpawnAttempts, minSpawnInterval)
			sleepFor(minSpawnInterval)
		}
	}
	return fmt.Errorf("circuit breaker still refusing after %d attempts: %w", maxSpawnAttempts, lastErr)
}

func createAndBootSession(home string, sup supervisor, state *sessionState, model string) {
	fmt.Printf("    %s: creating... ", sup.Name)

	if err := spawnSessionWithRetry(sup, model); err != nil {
		fmt.Printf("FAILED: %v\n", err)
		return
	}
	fmt.Println("created")

	time.Sleep(10 * time.Second)

	sendBootPrompt(home, sup)

	// Wait for boot prompt to be processed before starting loop.
	fmt.Printf("    %s: waiting 30s for boot prompt processing...\n", sup.Name)
	time.Sleep(30 * time.Second)

	loopSent := sendLoopCommand(sup)

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
// failures for the same supervisor, it logs an escalation (future: desktop
// notification via ce-mcw2).
func runHealthMonitor(home string, state *sessionState, model string) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	tracker := newRestartTracker()

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

			case healthDead:
				ok, count := tracker.shouldRestart(sup.Name)
				if !ok {
					if tracker.shouldEscalate(sup.Name) {
						writeTrail(home, "dispatch.escalation.restart_exhausted", map[string]any{
							"supervisor": sup.Name,
							"restarts":   count,
						})
						fmt.Printf("[%s] ESCALATION: %d consecutive restart failures — needs human intervention\n",
							sup.Name, count)
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
				createAndBootSession(home, sup, state, model)
				saveState(home, state)
			}
		}
	}
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
	for line := range strings.SplitSeq(string(out), "\n") {
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
