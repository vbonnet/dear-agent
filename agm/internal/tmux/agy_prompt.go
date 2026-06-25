package tmux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

var agyTrustPromptPatterns = []string{
	"Do you trust the contents of this project?",
	"Yes, I trust this folder",
}

func containsAgyPromptPattern(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == ">" {
			return true
		}
	}
	return false
}

func containsAgyTrustPromptPattern(content string) bool {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return false
	}
	for _, pattern := range agyTrustPromptPatterns {
		if strings.Contains(trimmed, pattern) {
			return true
		}
	}
	return false
}

// IsAgyIdle reports whether the AGY prompt is currently visible in the pane.
func IsAgyIdle(sessionName string) (bool, error) {
	output, err := exec.Command("tmux", "-S", GetSocketPath(),
		"capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p").Output()
	if err != nil {
		return false, fmt.Errorf("capture-pane failed: %w", err)
	}
	return containsAgyPromptPattern(string(output)), nil
}

// WaitForAgyPrompt polls the pane until AGY shows its idle prompt. If the
// first-run trust prompt appears, it is auto-accepted with Enter.
func WaitForAgyPrompt(sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting AGY prompt detection for session: %s", sessionName)

	baseCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	checkCount := 0
	trustAccepted := false

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for AGY prompt (waited %v, performed %d checks): %w", timeout, checkCount, ctx.Err())
		default:
		}
		checkCount++

		output, err := exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-20").Output()
		if ctx.Err() != nil {
			return fmt.Errorf("timeout or cancellation waiting for AGY prompt: %w", ctx.Err())
		}
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		content := string(output)
		if !trustAccepted && containsAgyTrustPromptPattern(content) {
			debug.Log("🛡️  AGY trust prompt detected (check #%d) — auto-answering with Enter", checkCount)
			if err := SendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to answer AGY trust prompt: %v", err)
			} else {
				trustAccepted = true
			}
			time.Sleep(1 * time.Second)
			continue
		}

		if containsAgyPromptPattern(content) {
			debug.Log("✓ AGY prompt detected (check #%d)", checkCount)
			time.Sleep(500 * time.Millisecond)
			return nil
		}

		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for AGY prompt... (check #%d)", checkCount)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
