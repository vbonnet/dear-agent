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

var agySurveyPromptPatterns = []string{
	"How's the CLI experience so far?",
	"[1] Good [2] Fine [3] Bad [0] Skip",
}

func containsAgyPromptPattern(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == ">" {
			return true
		}
	}
	return false
}

func containsAgyReadyPattern(content string) bool {
	return !ContainsAgySurveyPrompt(content) && containsAgyPromptPattern(content)
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

// ContainsAgySurveyPrompt reports whether AGY's input-capturing feedback footer
// is visible. The footer can coexist with a bare prompt, so it must win over
// ordinary readiness detection.
func ContainsAgySurveyPrompt(content string) bool {
	for _, pattern := range agySurveyPromptPatterns {
		if strings.Contains(content, pattern) {
			return true
		}
	}
	return false
}

// DismissAgySurveyIfPresent sends the documented Skip option when the survey
// owns focus. It returns true only when a survey was detected and the key sent.
func DismissAgySurveyIfPresent(sessionName, content string) (bool, error) {
	if !ContainsAgySurveyPrompt(content) {
		return false, nil
	}
	if err := SendKeys(sessionName, "0"); err != nil {
		return false, fmt.Errorf("dismiss AGY feedback survey: %w", err)
	}
	return true, nil
}

// IsAgyIdle reports whether the AGY prompt is currently visible in the pane.
func IsAgyIdle(sessionName string) (bool, error) {
	output, err := exec.Command("tmux", "-S", GetSocketPath(),
		"capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p").Output()
	if err != nil {
		return false, fmt.Errorf("capture-pane failed: %w", err)
	}
	content := string(output)
	return containsAgyReadyPattern(content), nil
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
		if dismissed, dismissErr := DismissAgySurveyIfPresent(sessionName, content); dismissErr != nil {
			debug.Log("Failed to dismiss AGY feedback survey: %v", dismissErr)
		} else if dismissed {
			debug.Log("AGY feedback survey detected; selected Skip")
			time.Sleep(500 * time.Millisecond)
			continue
		}
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

		if containsAgyReadyPattern(content) {
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
