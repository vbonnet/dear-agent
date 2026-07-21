package tmux

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

const (
	// HarnessInputReady means the harness can safely receive input.
	HarnessInputReady = "YES"
	// HarnessInputBusy means the harness does not currently own an empty composer.
	HarnessInputBusy = "QUEUE"
	// HarnessInputPermission means a permission decision currently owns input.
	HarnessInputPermission = "PERMISSION"
	// HarnessInputOverlay means a harness overlay currently owns input.
	HarnessInputOverlay = "OVERLAY"
	// HarnessInputOnboarding means a documented first-run prompt currently owns input.
	HarnessInputOnboarding = "ONBOARDING"
	// HarnessInputNotFound means the exact tmux session does not exist.
	HarnessInputNotFound = "NOT_FOUND"
	// HarnessInputWrongHarness means the expected harness process is not alive.
	HarnessInputWrongHarness = "WRONG_HARNESS"
)

// HarnessInputReadiness is the harness-specific, fail-closed verdict shared by
// every surface before it sends input to a tmux pane.
type HarnessInputReadiness struct {
	Ready   bool
	State   string
	Content string
}

// CheckExpectedHarnessInput proves that the exact session exists, an expected
// harness process is alive, and that harness currently owns the input composer.
// A stale prompt rendered by a dead or different process is never sufficient.
func CheckExpectedHarnessInput(ctx context.Context, sessionName, harness string) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	liveness, err := CheckExpectedHarnessLiveness(ctx, sessionName, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !liveness.SessionExists {
		return HarnessInputReadiness{State: HarnessInputNotFound}, nil
	}
	if !liveness.HarnessAlive {
		return HarnessInputReadiness{State: HarnessInputWrongHarness}, nil
	}
	content, err := CapturePaneOutputContext(ctx, sessionName, 30)
	if err != nil {
		return HarnessInputReadiness{}, fmt.Errorf("capture expected %s pane: %w", harness, err)
	}
	ready, state, err := ClassifyHarnessInput(content, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	return HarnessInputReadiness{Ready: ready, State: state, Content: content}, nil
}

// ClassifyHarnessInput is the pure composer classifier. Readiness is scoped to
// the configured harness and to the pane tail that currently owns input.
func ClassifyHarnessInput(content, harness string) (bool, string, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return false, "", err
	}
	if hasPermissionPrompt(content) {
		return false, HarnessInputPermission, nil
	}
	if hasInputOverlay(content, harness) {
		return false, HarnessInputOverlay, nil
	}
	if hasOnboardingPrompt(content, harness) {
		return false, HarnessInputOnboarding, nil
	}

	tail := paneInputTail(content, 12)
	var ready bool
	switch harness {
	case "claude-code":
		ready = hasExactPromptLine(tail, "❯")
	case "codex-cli":
		ready = isCodexInputComposerReady(tail)
	case "agy":
		ready = hasExactPromptLine(tail, ">")
	case "gemini-cli":
		ready = containsGeminiPromptPattern(tail)
	case "opencode-cli":
		ready = containsOpenCodePromptPattern(tail)
	}
	if ready {
		return true, HarnessInputReady, nil
	}
	return false, HarnessInputBusy, nil
}

// CheckExpectedHarnessLiveness scans the exact session's process tree and
// accepts only processes compatible with the configured harness. Node is a
// valid executable host for Node-backed harnesses; the composer classifier
// provides the second, harness-specific proof.
func CheckExpectedHarnessLiveness(ctx context.Context, sessionName, harness string) (PaneLiveness, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return PaneLiveness{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	pids, err := listPanePIDs(scanCtx, sessionName, GetSocketPath())
	if err != nil {
		return PaneLiveness{}, err
	}
	if len(pids) == 0 {
		return PaneLiveness{SessionExists: false}, nil
	}
	procs, err := readProcessTable(scanCtx)
	if err != nil {
		return PaneLiveness{}, err
	}
	return ClassifyPaneLiveness(pids, procs, expectedHarnessMatcher(harness)), nil
}

// WaitForExpectedHarnessReady owns startup readiness for shared operations.
// It permits documented first-run transitions, but otherwise only observes.
func WaitForExpectedHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	if err := validateReadinessHarness(harness); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	wrongHarnessChecks := 0
	advanced := make(map[string]bool)
	for {
		readiness, err := CheckExpectedHarnessInput(waitCtx, sessionName, harness)
		if err != nil {
			if waitCtx.Err() != nil {
				return fmt.Errorf("wait for %s readiness in %q: %w", harness, sessionName, waitCtx.Err())
			}
			return fmt.Errorf("check %s readiness in %q: %w", harness, sessionName, err)
		}
		ready, err := handleHarnessStartupState(waitCtx, sessionName, harness, readiness, &wrongHarnessChecks, advanced)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for %s readiness in %q (last state %s): %w", harness, sessionName, readiness.State, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func handleHarnessStartupState(
	ctx context.Context,
	sessionName, harness string,
	readiness HarnessInputReadiness,
	wrongHarnessChecks *int,
	advanced map[string]bool,
) (bool, error) {
	switch readiness.State {
	case HarnessInputReady:
		return true, nil
	case HarnessInputNotFound:
		return false, fmt.Errorf("tmux session %q disappeared while waiting for %s readiness", sessionName, harness)
	case HarnessInputWrongHarness:
		*wrongHarnessChecks++
		if *wrongHarnessChecks >= 6 {
			return false, fmt.Errorf("expected %s process is not running in tmux session %q", harness, sessionName)
		}
	case HarnessInputOnboarding, HarnessInputOverlay:
		*wrongHarnessChecks = 0
		if !canAdvanceHarnessStartup(readiness.State, harness, readiness.Content) {
			return false, nil
		}
		transition := readiness.State + ":" + onboardingKind(readiness.Content, harness)
		if !advanced[transition] {
			if err := advanceHarnessStartup(ctx, sessionName, harness, readiness.Content); err != nil {
				return false, fmt.Errorf("advance %s startup in %q: %w", harness, sessionName, err)
			}
			advanced[transition] = true
		}
	default:
		*wrongHarnessChecks = 0
	}
	return false, nil
}

func validateReadinessHarness(harness string) error {
	switch harness {
	case "claude-code", "codex-cli", "agy", "gemini-cli", "opencode-cli":
		return nil
	default:
		return fmt.Errorf("unsupported harness readiness check %q", harness)
	}
}

func expectedHarnessMatcher(harness string) func(string) bool {
	return func(comm string) bool {
		base := filepath.Base(strings.TrimSpace(comm))
		switch harness {
		case "claude-code":
			return base == "claude" || base == "node" || isClaudeProcess(base)
		case "codex-cli":
			return base == "codex" || base == "node"
		case "agy":
			return base == "agy"
		case "gemini-cli":
			return base == "gemini" || base == "node"
		case "opencode-cli":
			return base == "opencode" || base == "node"
		default:
			return false
		}
	}
}

func paneInputTail(content string, maxLines int) string {
	lines := strings.Split(strings.TrimRight(stripANSI(content), "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func hasExactPromptLine(content, prompt string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == prompt {
			return true
		}
	}
	return false
}

func isCodexInputComposerReady(content string) bool {
	return IsCodexComposerReady(content)
}

func hasPermissionPrompt(content string) bool {
	return strings.Contains(content, "Do you want to proceed?") ||
		strings.Contains(content, "Allow this command") ||
		strings.Contains(content, "Approve this action") ||
		strings.Contains(content, "Deny") && strings.Contains(content, "Allow")
}

func hasInputOverlay(content, harness string) bool {
	if strings.Contains(content, "Background tasks") && strings.Contains(content, "to close") {
		return true
	}
	return harness == "agy" && ContainsAgySurveyPrompt(content)
}

func hasOnboardingPrompt(content, harness string) bool {
	switch harness {
	case "claude-code":
		return containsTrustPromptPattern(content)
	case "codex-cli":
		return containsCodexTrustPromptPattern(content) || containsCodexModelUpgradePromptPattern(content)
	case "agy":
		return containsAgyTrustPromptPattern(content)
	default:
		return false
	}
}

func onboardingKind(content, harness string) string {
	if harness == "codex-cli" && containsCodexModelUpgradePromptPattern(content) {
		return "model-upgrade"
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return "survey"
	}
	return "trust"
}

func canAdvanceHarnessStartup(state, harness, content string) bool {
	return state == HarnessInputOnboarding ||
		(state == HarnessInputOverlay && harness == "agy" && ContainsAgySurveyPrompt(content))
}

func advanceHarnessStartup(ctx context.Context, sessionName, harness, content string) error {
	if harness == "codex-cli" && containsCodexModelUpgradePromptPattern(content) {
		if err := sendReadinessKey(ctx, sessionName, "Down"); err != nil {
			return err
		}
		return sendReadinessKey(ctx, sessionName, "Enter")
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return sendReadinessKey(ctx, sessionName, "0")
	}
	return sendReadinessKey(ctx, sessionName, "Enter")
}

func sendReadinessKey(ctx context.Context, sessionName, key string) error {
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", GetSocketPath(), "send-keys", "-t", NormalizeTmuxSessionName(sessionName), key)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
