package tmux

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
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

var agyOnboardingScreens = []struct {
	marker     string
	companions []string
}{
	{
		marker:     "Choose your color scheme:",
		companions: []string{"Welcome to Antigravity CLI!"},
	},
	{
		marker:     "Terms of Service & Data Use",
		companions: []string{"Yes, I agree to help improve Antigravity CLI", "Done"},
	},
}

// ErrAgyOnboardingRequired means AGY is waiting for an operator to make
// first-run preference, legal, or data-use choices. AGM deliberately does not
// accept those choices on the operator's behalf.
var ErrAgyOnboardingRequired = errors.New("AGY first-run onboarding requires explicit operator review")

func containsAgyPromptPattern(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == ">" {
			return true
		}
	}
	return false
}

func containsAgyReadyPattern(content string) bool {
	if !ContainsAgySurveyPrompt(content) {
		return containsAgyPromptPattern(content)
	}
	return containsAgyPromptAfterSurvey(content)
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

func containsAgyOnboardingPrompt(content string) bool {
	lastComposer := -1
	offset := 0
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == ">" {
			lastComposer = offset
		}
		offset += len(line) + 1
	}

	for _, screen := range agyOnboardingScreens {
		marker := strings.LastIndex(content, screen.marker)
		if marker < 0 || marker < lastComposer {
			continue
		}
		activeRegion := content[lastComposer+1:]
		matched := true
		for _, companion := range screen.companions {
			if !strings.Contains(activeRegion, companion) {
				matched = false
				break
			}
		}
		if matched {
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

func containsAgyPromptAfterSurvey(content string) bool {
	lastSurveyEnd := -1
	for _, pattern := range agySurveyPromptPatterns {
		if index := strings.LastIndex(content, pattern); index >= 0 && index+len(pattern) > lastSurveyEnd {
			lastSurveyEnd = index + len(pattern)
		}
	}
	return lastSurveyEnd >= 0 && containsAgyPromptPattern(content[lastSurveyEnd:])
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

type agyPromptRuntime struct {
	capture  func(context.Context, string) ([]byte, error)
	sendKeys func(string, string) error
	sleep    func(context.Context, time.Duration)
}

func realAgyPromptRuntime() agyPromptRuntime {
	return agyPromptRuntime{
		capture: func(ctx context.Context, sessionName string) ([]byte, error) {
			return exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-S", "-20").Output()
		},
		sendKeys: SendKeys,
		sleep: func(ctx context.Context, interval time.Duration) {
			if err := sleepWithContext(ctx, interval); err != nil {
				debug.Log("AGY prompt sleep interrupted: %v", err)
			}
		},
	}
}

// WaitForAgyPrompt polls the pane until AGY shows its idle prompt. If the
// first-run trust prompt appears, it is auto-accepted with Enter.
func WaitForAgyPrompt(ctx context.Context, sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting AGY prompt detection for session: %s", sessionName)
	return waitForAgyPromptWithRuntime(ctx, sessionName, timeout, realAgyPromptRuntime())
}

func dismissAgySurveyOnce(runtime agyPromptRuntime, sessionName, content string, alreadyDismissed bool) (dismissed, handled bool) {
	if alreadyDismissed || !ContainsAgySurveyPrompt(content) {
		return alreadyDismissed, false
	}
	if err := runtime.sendKeys(sessionName, "0"); err != nil {
		debug.Log("Failed to dismiss AGY feedback survey: %v", err)
		return false, false
	}
	debug.Log("AGY feedback survey detected; selected Skip")
	return true, true
}

func waitForAgyPromptWithRuntime(baseCtx context.Context, sessionName string, timeout time.Duration, runtime agyPromptRuntime) error {
	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	checkCount := 0
	trustAccepted := false
	surveyDismissed := false

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for AGY prompt (waited %v, performed %d checks): %w", timeout, checkCount, ctx.Err())
		default:
		}
		checkCount++

		output, err := runtime.capture(ctx, sessionName)
		if ctx.Err() != nil {
			return fmt.Errorf("timeout or cancellation waiting for AGY prompt: %w", ctx.Err())
		}
		if err != nil {
			runtime.sleep(ctx, 500*time.Millisecond)
			continue
		}

		content := string(output)
		if containsAgyOnboardingPrompt(content) {
			return fmt.Errorf("%w: run `agy` interactively, review the theme and Terms of Service/Data Use choices, complete onboarding, then retry AGM; AGM will not accept legal or data-use choices automatically", ErrAgyOnboardingRequired)
		}
		var surveyHandled bool
		surveyDismissed, surveyHandled = dismissAgySurveyOnce(runtime, sessionName, content, surveyDismissed)
		if surveyHandled {
			runtime.sleep(ctx, 500*time.Millisecond)
			continue
		}
		if !trustAccepted && containsAgyTrustPromptPattern(content) {
			debug.Log("🛡️  AGY trust prompt detected (check #%d) — auto-answering with Enter", checkCount)
			if err := runtime.sendKeys(sessionName, "Enter"); err != nil {
				debug.Log("⚠️  Failed to answer AGY trust prompt: %v", err)
			} else {
				trustAccepted = true
			}
			runtime.sleep(ctx, time.Second)
			continue
		}

		if containsAgyReadyPattern(content) || (surveyDismissed && containsAgyPromptAfterSurvey(content)) {
			debug.Log("✓ AGY prompt detected (check #%d)", checkCount)
			runtime.sleep(ctx, 500*time.Millisecond)
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("AGY ready stabilization interrupted: %w", err)
			}
			return nil
		}

		if checkCount%10 == 0 {
			debug.Log("⏳ Still waiting for AGY prompt... (check #%d)", checkCount)
		}
		runtime.sleep(ctx, 500*time.Millisecond)
	}
}
