package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	LoopInterval string
	TickPrompt   string
}

var supervisors = []supervisor{
	{
		Name:         "vroom-meta-o",
		ID:           "vroom-meta-o",
		SkillFile:    "meta-orchestrator.md",
		PrimaryFor:   "vroom-orch",
		TertiaryFor:  "vroom-overseer",
		LoopInterval: "180s",
		TickPrompt: "Execute your Meta-Orchestrator tick: " +
			"1) check peer heartbeats 2) read open beads via bd --db ~/beads/context-engine/.beads list --state=open --format=json " +
			"3) read current roadmap 4) evaluate new beads for roadmap 5) verify Orch dispatch activity " +
			"6) write heartbeat. Follow your SKILL instructions at ~/.agm/vroom/skills/meta-orchestrator.md",
	},
	{
		Name:         "vroom-orch",
		ID:           "vroom-orch",
		SkillFile:    "orchestrator.md",
		PrimaryFor:   "vroom-overseer",
		TertiaryFor:  "vroom-meta-o",
		LoopInterval: "90s",
		TickPrompt: "Execute your Orchestrator tick: " +
			"1) check peer heartbeats 2) read accepted roadmap items " +
			"3) dispatch undispatched work to worker sessions via agm session new " +
			"4) monitor active workers 5) detect stale items " +
			"6) write heartbeat. Follow your SKILL instructions at ~/.agm/vroom/skills/orchestrator.md",
	},
	{
		Name:         "vroom-overseer",
		ID:           "vroom-overseer",
		SkillFile:    "overseer.md",
		PrimaryFor:   "vroom-meta-o",
		TertiaryFor:  "vroom-orch",
		LoopInterval: "60s",
		TickPrompt: "Execute your Overseer tick: " +
			"1) check peer heartbeats 2) probe system resources (disk, memory, FDs, gopls) " +
			"3) audit session health 4) reconcile stale beads " +
			"5) audit worktrees 6) verify Meta-O activity " +
			"7) write heartbeat. Follow your SKILL instructions at ~/.agm/vroom/skills/overseer.md",
	},
}

func main() {
	bootOnly := flag.Bool("boot-only", false, "install skills and create sessions but don't start loops")
	loopOnly := flag.Bool("loop-only", false, "start loops on existing sessions (skip creation)")
	status := flag.Bool("status", false, "show supervisor mesh status and exit")
	flag.Parse()

	if *status {
		showStatus()
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}

	if !*loopOnly {
		installSkills(home)
		createSessions()
		sendBootPrompts(home)
	}

	if !*bootOnly {
		if !*loopOnly {
			fmt.Println("==> Waiting for supervisors to process boot prompts (45s)...")
			time.Sleep(45 * time.Second)
		}
		startLoops()
	}

	fmt.Println("\n==> VROOM supervisor mesh launched!")
	fmt.Println()
	fmt.Printf("    %-22s %s\n", "Meta-Orchestrator:", "vroom-meta-o (tick every 3min)")
	fmt.Printf("    %-22s %s\n", "Orchestrator:", "vroom-orch (tick every 90s)")
	fmt.Printf("    %-22s %s\n", "Overseer:", "vroom-overseer (tick every 60s)")
	fmt.Println()
	fmt.Println("Monitor:")
	fmt.Println("    agm supervisor status")
	fmt.Println("    agm session list")
	fmt.Println("    tail -f ~/.agm/vroom/trail.jsonl")
	fmt.Println()
	fmt.Println("Talk to a supervisor:")
	fmt.Println("    agm send msg vroom-meta-o --prompt \"status?\"")
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
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			fatal("write %s: %v", dest, err)
		}
		fmt.Printf("    %s\n", dest)
	}
}

func createSessions() {
	fmt.Println("==> Creating supervisor sessions...")
	for _, sup := range supervisors {
		fmt.Printf("    %s... ", sup.Name)

		existing := exec.Command("agm", "session", "list")
		out, _ := existing.Output()
		if strings.Contains(string(out), sup.Name) {
			fmt.Println("already exists, skipping")
			continue
		}

		cmd := exec.Command("agm", "session", "new", sup.Name,
			"--detached", "--workspace=oss", "--harness=claude-code")
		cmd.Env = scrubAPIKey(os.Environ())
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("WARN: %v\n%s\n", err, string(output))
		} else {
			fmt.Println("created")
		}

		time.Sleep(3 * time.Second)
	}

	fmt.Println("==> Waiting for sessions to initialize (15s)...")
	time.Sleep(15 * time.Second)
}

func sendBootPrompts(home string) {
	fmt.Println("==> Sending boot prompts...")
	for _, sup := range supervisors {
		skillPath := filepath.Join(home, ".agm", "vroom", "skills", sup.SkillFile)
		protocolPath := filepath.Join(home, ".agm", "vroom", "skills", "protocol.md")

		bootPrompt := fmt.Sprintf(`You are being initialized as a VROOM supervisor.

Read these files carefully — they are your operational instructions:
1. Shared protocol: %s
2. Your role instructions: %s

After reading both files:
1. Run: mkdir -p ~/.agm/vroom/heartbeat
2. Write your initial heartbeat: agm supervisor heartbeat --id %s --primary-for %s --tertiary-for %s
3. Write timestamp: date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ > ~/.agm/vroom/heartbeat/%s.json
4. Confirm you are ready and summarize your role.

The launcher will then start your tick loop via /loop.`,
			protocolPath, skillPath,
			sup.ID, sup.PrimaryFor, sup.TertiaryFor,
			strings.TrimPrefix(sup.Name, "vroom-"))

		fmt.Printf("    booting %s...\n", sup.Name)
		cmd := exec.Command("agm", "send", "msg", sup.Name,
			"--sender", "vroom-dispatch",
			"--prompt", bootPrompt)
		cmd.Env = scrubAPIKey(os.Environ())
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("    ERROR: %v\n%s\n", err, string(output))
		}

		time.Sleep(2 * time.Second)
	}
}

func startLoops() {
	fmt.Println("==> Starting tick loops...")
	for _, sup := range supervisors {
		loopCmd := fmt.Sprintf("/loop %s %s", sup.LoopInterval, sup.TickPrompt)
		fmt.Printf("    %s (every %s)...\n", sup.Name, sup.LoopInterval)
		cmd := exec.Command("agm", "send", "msg", sup.Name,
			"--sender", "vroom-dispatch",
			"--prompt", loopCmd)
		cmd.Env = scrubAPIKey(os.Environ())
		if output, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("    ERROR: %v\n%s\n", err, string(output))
		}
		time.Sleep(3 * time.Second)
	}
}

func showStatus() {
	cmd := exec.Command("agm", "supervisor", "status")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Println("\nSession status:")
		for _, sup := range supervisors {
			cmd := exec.Command("agm", "session", "list")
			out, _ := cmd.Output()
			found := strings.Contains(string(out), sup.Name)
			status := "NOT FOUND"
			if found {
				status = "exists"
			}
			fmt.Printf("    %-20s %s\n", sup.Name, status)
		}
	}

	trail := filepath.Join(os.Getenv("HOME"), ".agm", "vroom", "trail.jsonl")
	if info, err := os.Stat(trail); err == nil {
		fmt.Printf("\nTrail: %s (%d bytes)\n", trail, info.Size())
		cmd := exec.Command("tail", "-3", trail)
		cmd.Stdout = os.Stdout
		_ = cmd.Run()
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
