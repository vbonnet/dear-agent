package tmux

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

// WaitForPaneClose waits for a tmux pane to close
// Polls list-panes until it fails (indicating pane closed)
func WaitForPaneClose(sessionName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	socketPath := GetSocketPath()
	checkCount := 0

	log.Printf("⏳ Monitoring pane closure for session: %s", sessionName)

	for time.Now().Before(deadline) {
		checkCount++

		// Check if pane still exists
		cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", sessionName, "-F", "#{pane_id}")
		output, err := cmd.CombinedOutput()

		if err != nil {
			// Exit code != 0 means pane doesn't exist anymore
			log.Printf("✓ Pane closed after %d checks (%.1fs)", checkCount, time.Since(deadline.Add(-timeout)).Seconds())
			return nil
		}

		// Log first few checks and periodically thereafter for debugging
		if checkCount <= 3 || checkCount%10 == 0 {
			log.Printf("📊 Pane still active (check %d): %s", checkCount, strings.TrimSpace(string(output)))
		}

		// Pane still exists, wait a bit
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for pane to close after %d checks (waited %v)", checkCount, timeout)
}

// SendKeysToPane sends keys to a specific pane
// Sends the keys followed by C-m (Enter) as separate commands
func SendKeysToPane(sessionName string, keys string) error {
	socketPath := GetSocketPath()

	log.Printf("⌨️  Sending keys to %s: %q", sessionName, keys)

	// Send the text without Enter first
	cmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "-l", keys)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send keys: %w (output: %s)", err, string(output))
	}

	log.Printf("✓ Keys sent: %q", keys)

	// Now send Enter as a separate command
	cmd = exec.Command("tmux", "-S", socketPath, "send-keys", "-t", sessionName, "C-m")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to send Enter key: %w (output: %s)", err, string(output))
	}

	log.Printf("✓ Enter key sent")
	return nil
}

// IsPaneActive checks if a pane is currently active
func IsPaneActive(sessionName string) (bool, error) {
	socketPath := GetSocketPath()

	cmd := exec.Command("tmux", "-S", socketPath, "list-panes", "-t", sessionName)
	err := cmd.Run()

	if err != nil {
		// Non-zero exit = pane doesn't exist
		return false, nil
	}

	return true, nil
}
