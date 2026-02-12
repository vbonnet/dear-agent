package csm

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Prompter handles prompt injection into AGM sessions
type Prompter struct {
	// tmpDir is where temp prompt files are written (defaults to /tmp)
	tmpDir string
}

// NewPrompter creates a new prompter
func NewPrompter() *Prompter {
	return &Prompter{
		tmpDir: "/tmp",
	}
}

// NewPrompterWithTmpDir creates a prompter with custom temp directory
func NewPrompterWithTmpDir(tmpDir string) *Prompter {
	return &Prompter{
		tmpDir: tmpDir,
	}
}

// InjectPrompt injects a bead prompt into a AGM session via tmux send-keys
func (p *Prompter) InjectPrompt(sessionName string, beadID string, prompt string) error {
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if beadID == "" {
		return fmt.Errorf("bead ID cannot be empty")
	}
	if prompt == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	// Create temp file for prompt
	tmpPath := filepath.Join(p.tmpDir, fmt.Sprintf("bead-%s.md", beadID))

	// Write prompt to temp file
	if err := os.WriteFile(tmpPath, []byte(prompt), 0644); err != nil {
		return fmt.Errorf("failed to write prompt to temp file: %w", err)
	}

	// Ensure cleanup even on failure
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	// Build tmux command: cat temp file into session
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, fmt.Sprintf("cat %s", tmpPath), "C-m")

	// Execute command
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to inject prompt via tmux: %w (output: %s)", err, string(output))
	}

	return nil
}

// InjectPromptDirect injects a prompt directly without temp file (for short prompts)
func (p *Prompter) InjectPromptDirect(sessionName string, prompt string) error {
	if sessionName == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if prompt == "" {
		return fmt.Errorf("prompt cannot be empty")
	}

	// Send prompt directly via tmux send-keys
	cmd := exec.Command("tmux", "send-keys", "-t", sessionName, prompt, "C-m")

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to inject prompt via tmux: %w (output: %s)", err, string(output))
	}

	return nil
}
