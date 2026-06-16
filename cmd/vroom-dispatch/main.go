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
	"strings"
	"sync"
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
		Name:         "vroom-meta-o",
		ID:           "vroom-meta-o",
		SkillFile:    "meta-orchestrator.md",
		PrimaryFor:   "vroom-orch",
		TertiaryFor:  "vroom-overseer",
		TickInterval: 180 * time.Second,
		TickPrompt: "Execute your Meta-Orchestrator tick: " +
			"1) check peer heartbeats 2) read open beads via bd --db ~/beads/context-engine/.beads list --state=open --format=json " +
			"3) read current roadmap 4) evaluate new beads for roadmap 5) verify Orch dispatch activity " +
			"6) write heartbeat.",
	},
	{
		Name:         "vroom-orch",
		ID:           "vroom-orch",
		SkillFile:    "orchestrator.md",
		PrimaryFor:   "vroom-overseer",
		TertiaryFor:  "vroom-meta-o",
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
		PrimaryFor:   "vroom-meta-o",
		TertiaryFor:  "vroom-orch",
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
	TickCount int    `json:"tick_count"`
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
		fmt.Println("==> Sessions created. Boot-only mode, exiting.")
		printStatus(state)
		return
	}

	runTickLoop(home, state)
}

// ensureSessions creates AGM sessions for any supervisor that doesn't have a live one.
func ensureSessions(home string, state *sessionState) {
	fmt.Println("==> Ensuring supervisor sessions...")

	for _, sup := range supervisors {
		if info, ok := state.Sessions[sup.Name]; ok {
			if isSessionAlive(sup.Name) {
				fmt.Printf("    %s: alive (created %s, %d ticks)\n", sup.Name, info.CreatedAt, info.TickCount)
				continue
			}
			fmt.Printf("    %s: dead, recreating...\n", sup.Name)
			delete(state.Sessions, sup.Name)
		}
		createSession(home, sup, state)
	}
}

func createSession(home string, sup supervisor, state *sessionState) {
	fmt.Printf("    %s: creating... ", sup.Name)

	cmd := exec.Command("agm", "session", "new", sup.Name,
		"--detached", "--workspace=oss", "--harness=claude-code")
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Printf("FAILED: %v\n%s\n", err, string(output))
		return
	}
	fmt.Println("created")

	// Wait for the session to initialize before sending boot prompt.
	time.Sleep(10 * time.Second)

	sendBootPrompt(home, sup)

	state.Sessions[sup.Name] = sessionInfo{
		Name:      sup.Name,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		TickCount: 0,
	}
}

// sendBootPrompt sends the full SKILL + protocol content as the initial message.
// This large static payload gets cached by Claude's prompt cache, making subsequent
// tick messages cheap (they only pay for the tick instruction + tool call tokens).
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

Your session stays alive across ticks — you will receive periodic tick messages
telling you to execute your role's tick behavior. Between ticks, you wait.

=== SHARED PROTOCOL ===
%s

=== YOUR ROLE ===
%s

=== INITIAL SETUP ===
1. Run: mkdir -p ~/.agm/vroom/heartbeat
2. Write your initial heartbeat: agm supervisor heartbeat --id %s --primary-for %s --tertiary-for %s
3. Confirm you are ready and summarize your role.

You will receive tick messages shortly. Each tick message will say
"Execute tick N" — when you see it, run through your tick steps as defined
in your role instructions above, then report a brief summary.`,
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

// runTickLoop is the main loop. It sends tick messages to each supervisor at their
// configured interval, health-checks sessions, and recreates dead ones.
func runTickLoop(home string, state *sessionState) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("==> Starting tick dispatch loop (Ctrl-C to stop)...")
	printStatus(state)

	// Give sessions time to process boot prompts before first tick.
	fmt.Println("==> Waiting 30s for boot prompts to settle...")
	select {
	case <-ctx.Done():
		return
	case <-time.After(30 * time.Second):
	}

	var (
		wg sync.WaitGroup
		mu sync.Mutex
	)
	for i := range supervisors {
		wg.Add(1)
		go func(sup supervisor) {
			defer wg.Done()
			runSupervisorTicks(ctx, home, sup, state, &mu)
		}(supervisors[i])
	}
	wg.Wait()

	saveState(home, state)
	fmt.Println("==> All tick loops stopped.")
}

func runSupervisorTicks(ctx context.Context, home string, sup supervisor, state *sessionState, mu *sync.Mutex) {
	for {
		mu.Lock()
		info := state.Sessions[sup.Name]
		info.TickCount++
		tickNum := info.TickCount
		state.Sessions[sup.Name] = info
		mu.Unlock()

		if !isSessionAlive(sup.Name) {
			fmt.Printf("[%s] session dead, recreating...\n", sup.Name)
			mu.Lock()
			createSession(home, sup, state)
			mu.Unlock()
			select {
			case <-ctx.Done():
				return
			case <-time.After(30 * time.Second):
			}
			continue
		}

		fmt.Printf("[%s] tick %d at %s\n", sup.Name, tickNum, time.Now().Format("15:04:05"))
		sendTick(sup, tickNum)

		if tickNum%5 == 0 {
			mu.Lock()
			saveState(home, state)
			mu.Unlock()
		}

		select {
		case <-ctx.Done():
			fmt.Printf("[%s] shutting down after tick %d\n", sup.Name, tickNum)
			return
		case <-time.After(sup.TickInterval):
		}
	}
}

// sendTick sends a lightweight tick instruction to an existing session.
// The session already has the full SKILL + protocol cached from the boot prompt,
// so this message only needs to carry the tick number and any dynamic context.
func sendTick(sup supervisor, tickNum int) {
	tickMsg := fmt.Sprintf("Execute tick %d. %s Report a brief summary when done.", tickNum, sup.TickPrompt)

	cmd := exec.Command("agm", "send", "msg", sup.Name,
		"--sender", "vroom-dispatch",
		"--prompt", tickMsg)
	cmd.Env = scrubAPIKey(os.Environ())
	if output, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "[%s] tick %d send failed: %v\n%s\n", sup.Name, tickNum, err, string(output))
	}
}

// isSessionAlive checks if an AGM session exists and is not archived.
func isSessionAlive(name string) bool {
	cmd := exec.Command("agm", "session", "list")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), name)
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
	if err := os.WriteFile(path, data, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch: write state: %v\n", err)
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
			fmt.Printf("    %-20s %s\n", sup.Name, status)
		}
	}

	home, _ := os.UserHomeDir()
	if home != "" {
		state := loadState(home)
		if len(state.Sessions) > 0 {
			fmt.Printf("\nPersistent state (updated %s):\n", state.UpdatedAt)
			for name, info := range state.Sessions {
				fmt.Printf("    %-20s created=%s ticks=%d\n", name, info.CreatedAt, info.TickCount)
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
	fmt.Println("    VROOM Supervisor Mesh — Persistent Sessions")
	fmt.Println()
	for _, sup := range supervisors {
		info := state.Sessions[sup.Name]
		alive := isSessionAlive(sup.Name)
		status := "DEAD"
		if alive {
			status = "alive"
		}
		fmt.Printf("    %-22s %s (ticks=%d, interval=%s)\n", sup.Name+":", status, info.TickCount, sup.TickInterval)
	}
	fmt.Println()
	fmt.Println("Monitor:")
	fmt.Println("    agm supervisor status")
	fmt.Println("    agm session list")
	fmt.Println("    tail -f ~/.agm/vroom/trail.jsonl")
	fmt.Println()
	fmt.Println("Talk to a supervisor:")
	fmt.Println("    agm send msg vroom-meta-o --prompt \"status?\"")
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
