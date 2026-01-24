package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

	// Attach to tmux session (blocks until user exits)
	socketPath := tmux.GetSocketPath()
	if err := attachToSession(socketPath, sessionName); err != nil {
		return fmt.Errorf("failed to attach to tmux session %q: %w", sessionName, err)
	}

	// Capture pane content after exit (best-effort)
	if err := captureAndPrint(socketPath, sessionName); err != nil {
		// Capture failure is non-fatal (attach succeeded)
		fmt.Fprintf(os.Stderr, "Warning: failed to capture pane content: %v\n", err)
	}

	return nil
}

// attachToSession attaches to the tmux session with full terminal passthrough
func attachToSession(socketPath, sessionName string) error {
	cmd := exec.Command("tmux", "-S", socketPath, "attach-session", "-t", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// captureAndPrint captures the pane content and prints to stdout
func captureAndPrint(socketPath, sessionName string) error {
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-p", "-S", "-", "-t", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("capture-pane failed: %w", err)
	}

	// Print captured output (only if non-empty)
	if len(output) > 0 {
		fmt.Print(string(output))
	}

	return nil
}

// initGemini initializes a Gemini agent session
func initGemini(sessionName string) error {
	// 1. Start Gemini in tmux pane (non-blocking)
	cmd := "gemini && exit"
	if err := tmux.SendCommand(sessionName, cmd); err != nil {
		return fmt.Errorf("failed to start gemini: %w", err)
	}

	debug.Log("Gemini started in tmux pane, waiting for process to appear")

	// 2. Wait for Gemini process to actually start (not just command sent)
	timeout := 30 * time.Second
	if envTimeout := os.Getenv("CSM_AGENT_INIT_TIMEOUT"); envTimeout != "" {
		if t, err := time.ParseDuration(envTimeout); err == nil {
			timeout = t
			debug.Log("Using custom init timeout: %v", timeout)
		}
	}

	if err := tmux.WaitForProcessReady(sessionName, "gemini", timeout); err != nil {
		debug.Log("Warning: Gemini process not detected: %v", err)
		fmt.Fprintf(os.Stderr, "Warning: Gemini process not detected (may still be starting)\n")
		// Continue anyway - graceful degradation
	} else {
		debug.Log("Gemini process detected and running")
	}

	// 2b. Wait for Gemini prompt to be ready (more robust than process detection alone)
	debug.Log("Waiting for Gemini prompt to appear...")
	promptTimeout := timeout // Use same timeout as process detection
	if err := tmux.WaitForGeminiReady(sessionName, promptTimeout); err != nil {
		debug.Log("Warning: Gemini prompt not detected: %v", err)
		fmt.Fprintf(os.Stderr, "Warning: Gemini prompt not fully detected (may still be initializing)\n")
		// Continue anyway - graceful degradation
	} else {
		debug.Log("Gemini prompt detected - ready for input")
	}

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
