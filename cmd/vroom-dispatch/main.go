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
	"time"
)

//go:embed skills/*.md
var skills embed.FS

type supervisor struct {
	Name         string
	ID           string
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

func main() {
	skillsOnly := flag.Bool("skills-only", false, "install SKILL files to ~/.agm/vroom/skills/ and exit")
	statusFlag := flag.Bool("status", false, "show supervisor mesh status and exit")
	bootOnly := flag.Bool("boot-only", false, "create sessions and send boot prompts, then exit (no tick loop)")
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

	state := loadState(home)
	ensureSessions(home, state)
	saveState(home, state)

	if *bootOnly {
		fmt.Println("==> Sessions created and booted. Exiting (boot-only).")
		printStatus(state)
		return
	}

	runHealthMonitor(home, state)
}

// ensureSessions creates AGM sessions for any supervisor that doesn't have a live one,
// sends the boot prompt, and starts the /loop.
func ensureSessions(home string, state *sessionState) {
	fmt.Println("==> Ensuring supervisor sessions...")

	for _, sup := range supervisors {
		if info, ok := state.Sessions[sup.Name]; ok {
			if isSessionAlive(sup.Name) {
				fmt.Printf("    %s: alive (created %s, loop_sent=%v)\n", sup.Name, info.CreatedAt, info.LoopSent)
				if !info.LoopSent {
					sendLoopCommand(sup)
					info.LoopSent = true
					state.Sessions[sup.Name] = info
				}
				continue
			}
			fmt.Printf("    %s: dead, recreating...\n", sup.Name)
			delete(state.Sessions, sup.Name)
		}
		createAndBootSession(home, sup, state)
	}
}

func createAndBootSession(home string, sup supervisor, state *sessionState) {
	fmt.Printf("    %s: creating... ", sup.Name)

	cmd := exec.Command("agm", "session", "new", sup.Name,
		"--detached", "--workspace=oss", "--harness=claude-code")
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("FAILED: %v\n%s\n", err, string(output))
		return
	}
	fmt.Println("created")

	time.Sleep(10 * time.Second)

	sendBootPrompt(home, sup)

	// Wait for boot prompt to be processed before starting loop.
	fmt.Printf("    %s: waiting 30s for boot prompt processing...\n", sup.Name)
	time.Sleep(30 * time.Second)

	sendLoopCommand(sup)

	state.Sessions[sup.Name] = sessionInfo{
		Name:      sup.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		LoopSent:  true,
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
		"--prompt", bootPrompt)
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    %s: boot send failed: %v\n%s\n", sup.Name, err, string(output))
	} else {
		fmt.Printf("    %s: boot prompt sent\n", sup.Name)
	}
}

// sendLoopCommand sends a /loop slash command to a supervisor session. The session's
// built-in /loop handler takes over tick scheduling — no external tick dispatch needed.
// The tick prompt references the SKILL instructions already loaded in the boot prompt.
func sendLoopCommand(sup supervisor) {
	intervalStr := fmt.Sprintf("%ds", int(sup.TickInterval.Seconds()))
	loopCmd := fmt.Sprintf("/loop %s %s Report a brief summary when done.", intervalStr, sup.TickPrompt)

	fmt.Printf("    %s: sending /loop (%s interval)...\n", sup.Name, intervalStr)
	cmd := exec.Command("agm", "send", "msg", sup.Name,
		"--sender", "vroom-dispatch",
		"--prompt", loopCmd)
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "    %s: /loop send failed: %v\n%s\n", sup.Name, err, string(output))
	} else {
		fmt.Printf("    %s: /loop started\n", sup.Name)
	}
}

// runHealthMonitor watches for dead supervisor sessions and recreates them.
// With /loop handling the tick cadence inside each session, the launcher only
// needs to monitor liveness and restart failures.
func runHealthMonitor(home string, state *sessionState) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("==> Health monitor running (Ctrl-C to stop)...")
	printStatus(state)
	fmt.Println("==> Tick loops running inside sessions via /loop. Monitoring health every 60s...")

	for {
		select {
		case <-ctx.Done():
			saveState(home, state)
			fmt.Println("==> Health monitor stopped.")
			return
		case <-time.After(60 * time.Second):
		}

		for _, sup := range supervisors {
			if !isSessionAlive(sup.Name) {
				fmt.Printf("[%s] session dead at %s, recreating...\n", sup.Name, time.Now().Format("15:04:05"))
				delete(state.Sessions, sup.Name)
				createAndBootSession(home, sup, state)
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
