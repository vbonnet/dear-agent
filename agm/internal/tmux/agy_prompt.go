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

const (
	agyPromptPollInterval           = 500 * time.Millisecond
	agyOnboardingDetectionDisabled  = 0
	agyOnboardingDetectionImmediate = 1
)

const (
	// A cold resume can replay transcript text before rendering the final
	// composer. The bounded confirmation window lets that replay settle while
	// still failing well before the 60-second resume readiness timeout when a
	// real interactive overlay owns the pane.
	agyResumeOnboardingConfirmationWindow = 10 * time.Second
	agyResumeOnboardingConfirmationChecks = 1 + int(agyResumeOnboardingConfirmationWindow/agyPromptPollInterval)
)

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

	activeRegion := strings.Join(strings.Fields(content[lastComposer+1:]), " ")
	for _, screen := range agyOnboardingScreens {
		if !strings.Contains(activeRegion, screen.marker) {
			continue
		}
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
			return exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", sessionName, "-p", "-J", "-S", "-20").Output()
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

// WaitForAgyPromptAfterInput polls for AGY readiness after operator-controlled
// text has been delivered. It does not classify onboarding-shaped transcript
// text as an active first-run screen; onboarding is gated by the pre-input wait.
func WaitForAgyPromptAfterInput(ctx context.Context, sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting post-input AGY prompt detection for session: %s", sessionName)
	return waitForAgyPromptAfterInputWithRuntime(ctx, sessionName, timeout, realAgyPromptRuntime())
}

// WaitForAgyPromptOnResume polls for readiness while a saved AGY conversation
// is replayed. It confirms an onboarding-shaped screen across multiple
// captures so quoted transcript text can settle into a composer, but still
// returns ErrAgyOnboardingRequired when a real interactive overlay persists.
func WaitForAgyPromptOnResume(ctx context.Context, sessionName string, timeout time.Duration) error {
	debug.Log("\n🔍 Starting resumed AGY prompt detection for session: %s", sessionName)
	return waitForAgyPromptOnResumeWithRuntime(ctx, sessionName, timeout, realAgyPromptRuntime())
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
	return waitForAgyPromptWithRuntimeMode(baseCtx, sessionName, timeout, runtime, agyOnboardingDetectionImmediate)
}

func waitForAgyPromptAfterInputWithRuntime(baseCtx context.Context, sessionName string, timeout time.Duration, runtime agyPromptRuntime) error {
	return waitForAgyPromptWithRuntimeMode(baseCtx, sessionName, timeout, runtime, agyOnboardingDetectionDisabled)
}

func waitForAgyPromptOnResumeWithRuntime(baseCtx context.Context, sessionName string, timeout time.Duration, runtime agyPromptRuntime) error {
	return waitForAgyPromptWithRuntimeMode(baseCtx, sessionName, timeout, runtime, agyResumeOnboardingConfirmationChecks)
}

func observeAgyOnboarding(content string, prior, required int) (observations int, pending bool, err error) {
	if required == 0 || !containsAgyOnboardingPrompt(content) {
		return 0, false, nil
	}
	observations = prior + 1
	if observations < required {
		return observations, true, nil
	}
	return observations, false, fmt.Errorf("%w: run `agy` interactively, review the theme and Terms of Service/Data Use choices, complete onboarding, then retry AGM; AGM will not accept legal or data-use choices automatically", ErrAgyOnboardingRequired)
}

type agyPromptWaitState struct {
	onboardingConfirmationChecks int
	onboardingObservations       int
	trustAccepted                bool
	surveyDismissed              bool
}

func (state *agyPromptWaitState) handleBlockingPrompt(ctx context.Context, runtime agyPromptRuntime, sessionName, content string, checkCount int) (bool, error) {
	var onboardingPending bool
	var err error
	state.onboardingObservations, onboardingPending, err = observeAgyOnboarding(content, state.onboardingObservations, state.onboardingConfirmationChecks)
	if err != nil {
		return false, err
	}
	if onboardingPending {
		runtime.sleep(ctx, agyPromptPollInterval)
		return true, nil
	}

	var surveyHandled bool
	state.surveyDismissed, surveyHandled = dismissAgySurveyOnce(runtime, sessionName, content, state.surveyDismissed)
	if surveyHandled {
		runtime.sleep(ctx, agyPromptPollInterval)
		return true, nil
	}
	if state.trustAccepted || !containsAgyTrustPromptPattern(content) {
		return false, nil
	}

	debug.Log("🛡️  AGY trust prompt detected (check #%d) — auto-answering with Enter", checkCount)
	if err := runtime.sendKeys(sessionName, "Enter"); err != nil {
		debug.Log("⚠️  Failed to answer AGY trust prompt: %v", err)
	} else {
		state.trustAccepted = true
	}
	runtime.sleep(ctx, time.Second)
	return true, nil
}

func waitForAgyPromptWithRuntimeMode(baseCtx context.Context, sessionName string, timeout time.Duration, runtime agyPromptRuntime, onboardingConfirmationChecks int) error {
	ctx, cancel := context.WithTimeout(baseCtx, timeout)
	defer cancel()

	checkCount := 0
	state := agyPromptWaitState{onboardingConfirmationChecks: onboardingConfirmationChecks}

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
			runtime.sleep(ctx, agyPromptPollInterval)
			continue
		}

		content := string(output)
		blockingPromptHandled, err := state.handleBlockingPrompt(ctx, runtime, sessionName, content, checkCount)
		if err != nil {
			return err
		}
		if blockingPromptHandled {
			continue
		}

		if containsAgyReadyPattern(content) || (state.surveyDismissed && containsAgyPromptAfterSurvey(content)) {
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
		runtime.sleep(ctx, agyPromptPollInterval)
	}
}
