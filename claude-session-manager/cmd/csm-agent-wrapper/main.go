package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/debug"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/readiness"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
)

// AgentConfig defines agent-specific behavior
type AgentConfig struct {
	Name          string
	CommandPath   string
	InitFunc      func(sessionName string) error
	UUIDExtractor func() (string, error)
}

// agentConfigs maps agent names to their configurations
var agentConfigs = map[string]AgentConfig{
	"gemini": {
		Name:          "gemini",
		CommandPath:   "/home/linuxbrew/.linuxbrew/bin/gemini",
		InitFunc:      initGemini,
		UUIDExtractor: extractGeminiUUID,
	},
	// Future: "claude", "gpt", etc.
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Parse command-line arguments
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: csm-agent-wrapper --agent=<agent-name> <session-name>")
	}

	// Parse --agent flag
	agentName := ""
	sessionName := ""

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if len(arg) > 8 && arg[:8] == "--agent=" {
			agentName = arg[8:]
		} else {
			sessionName = arg
		}
	}

	// Validate arguments
	if agentName == "" {
		return fmt.Errorf("--agent flag required\nUsage: csm-agent-wrapper --agent=<agent-name> <session-name>")
	}
	if sessionName == "" {
		return fmt.Errorf("session name required\nUsage: csm-agent-wrapper --agent=<agent-name> <session-name>")
	}

	// Lookup agent config
	config, ok := agentConfigs[agentName]
	if !ok {
		return fmt.Errorf("unknown agent %q\nSupported agents: gemini\nRun with -h for help", agentName)
	}

	debug.Log("Initializing agent %q for session %q", agentName, sessionName)

	// Initialize agent (start process, wait for readiness, create ready-file)
	if err := config.InitFunc(sessionName); err != nil {
		return fmt.Errorf("failed to initialize %s: %w", agentName, err)
	}

	debug.Log("Agent initialized successfully, attaching to session")

	// Exec into tmux attach (replaces current process)
	socketPath := tmux.GetSocketPath()
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return fmt.Errorf("tmux not found in PATH: %w", err)
	}

	args := []string{"tmux", "-S", socketPath, "attach-session", "-t", sessionName}
	env := os.Environ()

	debug.Log("Exec into tmux attach: %s %v", tmuxPath, args)
	return syscall.Exec(tmuxPath, args, env)
}

// initGemini initializes a Gemini agent session
func initGemini(sessionName string) error {
	// 1. Start Gemini in tmux pane (non-blocking)
	cmd := "gemini && exit"
	if err := tmux.SendCommand(sessionName, cmd); err != nil {
		return fmt.Errorf("failed to start gemini: %w", err)
	}

	debug.Log("Gemini started in tmux pane, waiting for initialization")

	// 2. Wait for initialization (configurable delay)
	delay := 500 * time.Millisecond
	if envDelay := os.Getenv("CSM_AGENT_INIT_DELAY"); envDelay != "" {
		if d, err := time.ParseDuration(envDelay); err == nil {
			delay = d
			debug.Log("Using custom init delay: %v", delay)
		}
	}
	time.Sleep(delay)

	// 3. Extract UUID (graceful degradation if fails)
	uuid, err := extractGeminiUUID()
	if err != nil {
		debug.Log("Warning: UUID extraction failed: %v", err)
		fmt.Fprintf(os.Stderr, "Warning: Failed to extract Gemini UUID (session still created)\n")
		fmt.Fprintf(os.Stderr, "You can associate manually: csm associate %s\n", sessionName)
		// Continue without UUID (graceful degradation)
		uuid = ""
	} else {
		debug.Log("Extracted Gemini UUID: %s", uuid)
	}

	// 4. Create ready-file
	manifestPath := filepath.Join(os.Getenv("HOME"), ".csm", "sessions", sessionName, "manifest.yaml")
	if err := readiness.CreateReadyFile(sessionName, manifestPath); err != nil {
		return fmt.Errorf("failed to create ready-file: %w", err)
	}

	debug.Log("Ready-file created for session %s", sessionName)
	return nil
}

// extractGeminiUUID extracts the session UUID from Gemini's --list-sessions output
func extractGeminiUUID() (string, error) {
	// Run gemini --list-sessions
	cmd := exec.Command("gemini", "--list-sessions")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run gemini --list-sessions: %w", err)
	}

	// Parse output with regex: find UUIDs in brackets
	// Format: "1. Session title (7 days ago) [uuid-here]"
	re := regexp.MustCompile(`\[([0-9a-f-]{36})\]`)
	matches := re.FindAllStringSubmatch(string(output), -1)

	if len(matches) == 0 {
		return "", fmt.Errorf("no UUIDs found in --list-sessions output")
	}

	// Return last UUID (most recent session)
	lastMatch := matches[len(matches)-1]
	uuid := lastMatch[1]

	return uuid, nil
}
