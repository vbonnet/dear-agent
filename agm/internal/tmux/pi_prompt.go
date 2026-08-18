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

var piStartupFailurePatterns = []string{
	"Failed to load extension",
	"Error: --session-id",
	"Invalid session ID",
}

// PiStartupError means Pi cannot reach AGM's managed ready state.
type PiStartupError struct {
	Detail string
}

func (e *PiStartupError) Error() string {
	return "Pi startup failed: " + e.Detail
}

// PiManagedState returns the most recent state published by AGM's mandatory
// Pi authorization extension. An empty result means no valid managed state is
// visible in the supplied pane content.
func PiManagedState(content string) string {
	index := strings.LastIndex(content, "AGM ")
	if index < 0 {
		return ""
	}
	fields := strings.Fields(content[index:])
	if len(fields) < 2 {
		return ""
	}
	modeAndState := strings.SplitN(fields[1], "/", 2)
	if len(modeAndState) != 2 {
		return ""
	}
	switch modeAndState[0] {
	case "plan", "default", "auto":
	default:
		return ""
	}
	switch modeAndState[1] {
	case "ready", "working":
		return modeAndState[1]
	default:
		return ""
	}
}

func containsPiReadyPattern(content string) bool {
	return containsPiReadyPatternForLaunch(content, "")
}

func containsPiReadyPatternForLaunch(content, launchID string) bool {
	index := strings.LastIndex(content, "AGM ")
	if index < 0 {
		return false
	}
	fields := strings.Fields(content[index:])
	if len(fields) < 2 || PiManagedState(content) != "ready" {
		return false
	}
	return launchID == "" || len(fields) >= 3 && fields[2] == launchID
}

func piStartupFailure(content string) string {
	for line := range strings.SplitSeq(content, "\n") {
		for _, pattern := range piStartupFailurePatterns {
			if strings.Contains(line, pattern) {
				return strings.TrimSpace(line)
			}
		}
	}
	return ""
}

// IsPiIdle reports the state published by AGM's mandatory Pi extension.
func IsPiIdle(sessionName string) (bool, error) {
	output, err := exec.Command("tmux", "-S", GetSocketPath(), "capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p").Output()
	if err != nil {
		return false, fmt.Errorf("capture Pi pane: %w", err)
	}
	return containsPiReadyPattern(string(output)), nil
}

type piPromptRuntime struct {
	capture func(context.Context, string) ([]byte, error)
	alive   func(context.Context, string) (bool, error)
	sleep   func(context.Context, time.Duration) error
}

func realPiPromptRuntime() piPromptRuntime {
	return piPromptRuntime{
		capture: func(ctx context.Context, sessionName string) ([]byte, error) {
			return exec.CommandContext(ctx, "tmux", "-S", GetSocketPath(), "capture-pane", "-t", NormalizeTmuxSessionName(sessionName), "-p", "-S", "-30").Output()
		},
		alive: func(ctx context.Context, sessionName string) (bool, error) {
			return IsPiProcessInPaneTreeContext(ctx, sessionName, GetSocketPath())
		},
		sleep: sleepWithContext,
	}
}

// WaitForPiPrompt waits for Pi and AGM's authorization extension to become ready.
func WaitForPiPrompt(sessionName string, timeout time.Duration) error {
	return WaitForPiPromptContext(context.Background(), sessionName, timeout)
}

// WaitForPiPromptContext is the caller-scoped Pi readiness protocol.
func WaitForPiPromptContext(ctx context.Context, sessionName string, timeout time.Duration) error {
	return waitForPiPromptWithRuntime(ctx, sessionName, "", timeout, realPiPromptRuntime())
}

// WaitForPiLaunchPromptContext requires readiness published by the exact Pi
// process launch identified by launchID. A footer left in pane history by an
// earlier process cannot satisfy this gate.
func WaitForPiLaunchPromptContext(ctx context.Context, sessionName, launchID string, timeout time.Duration) error {
	if strings.TrimSpace(launchID) == "" {
		return fmt.Errorf("pi launch readiness requires a launch id")
	}
	return waitForPiPromptWithRuntime(ctx, sessionName, launchID, timeout, realPiPromptRuntime())
}

//nolint:gocyclo // reason: stateful readiness loop keeps capture, fatal startup, liveness, and cancellation transitions explicit
func waitForPiPromptWithRuntime(parent context.Context, sessionName, launchID string, timeout time.Duration, runtime piPromptRuntime) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	checks := 0
	lastContent := ""
	for {
		if err := ctx.Err(); err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return fmt.Errorf("timeout waiting for managed Pi prompt (waited %v, performed %d checks): %w", timeout, checks, err)
			}
			return err
		}
		checks++
		output, err := runtime.capture(ctx, sessionName)
		if ctx.Err() != nil {
			continue
		}
		if err != nil {
			if sleepErr := runtime.sleep(ctx, 500*time.Millisecond); sleepErr != nil {
				return sleepErr
			}
			continue
		}
		content := string(output)
		lastContent = content
		if detail := piStartupFailure(content); detail != "" {
			return &PiStartupError{Detail: detail}
		}
		if containsPiReadyPatternForLaunch(content, launchID) {
			debug.Log("Pi managed prompt detected after %d checks", checks)
			if err := runtime.sleep(ctx, 250*time.Millisecond); err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			return nil
		}
		if checks >= 3 && runtime.alive != nil {
			alive, aliveErr := runtime.alive(ctx, sessionName)
			if aliveErr != nil {
				return &PiStartupError{Detail: fmt.Sprintf("cannot prove exact Pi process liveness: %v", aliveErr)}
			}
			if !alive {
				return &PiStartupError{Detail: fmt.Sprintf("exact Pi process exited before managed readiness; pane tail: %s", paneTail(lastContent, 6))}
			}
		}
		if err := runtime.sleep(ctx, 500*time.Millisecond); err != nil {
			return err
		}
	}
}
