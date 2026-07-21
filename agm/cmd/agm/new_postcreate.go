package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// runHarnessPostCreate runs the harness-specific post-create flow (deterministic
// association + readiness signal for Claude, prompt-readiness wait + prompt
// delivery for CLI harnesses).
func runHarnessPostCreate(ctx context.Context, sessionName string, modeAppliedAtStartup bool) error {
	switch {
	case harnessName == "claude-code" && os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "":
		return runClaudePostCreate(ctx, sessionName, modeAppliedAtStartup)
	case harnessName == "claude-code":
		debug.Phase("Skip Association (Test Environment)")
		debug.Log("Skipping deterministic association: AGM_TEST_RUN_ID=%s AGM_TEST_ENV=%s",
			os.Getenv("AGM_TEST_RUN_ID"), os.Getenv("AGM_TEST_ENV"))
		ui.PrintSuccess("Test session ready (association skipped)")
		return nil
	case harnessName == "gemini-cli":
		return runGeminiPostCreate(ctx, sessionName)
	case harnessName == "opencode-cli":
		return runOpenCodePostCreate(ctx, sessionName)
	case harnessName == "codex-cli":
		return runCodexPostCreate(ctx, sessionName)
	case harnessName == "agy":
		return runAgyPostCreate(ctx, sessionName)
	default:
		debug.Log("Skipping initialization sequence for harness: %s", harnessName)
		return nil
	}
}

// runClaudePostCreate associates the freshly-spawned Claude session and signals
// readiness deterministically from Go, then delivers the initial prompt.
//
// This replaces the old InitSequence (send /rename + /agm:agm-assoc into the
// live Claude pane, then wait up to AGM_READY_TIMEOUT_SECONDS for the in-session
// subprocess to write the ready-file). That chain silently no-ops in spawned /
// sandbox sessions where the agm plugin slash command is not loaded, costing one
// ready-file timeout per session and leaving the session unassociated (ce-o1sg).
// associateSpawnedClaudeSession does the same work from the spawner with no
// dependency on the spawned session running anything.
func runClaudePostCreate(ctx context.Context, sessionName string, modeAppliedAtStartup bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	associateSpawnedClaudeSession(sessionName)
	if err := ctx.Err(); err != nil {
		return err
	}
	if modeFlagValue != "" && !modeAppliedAtStartup {
		applyCreationModeSwitchContext(ctx, sessionName, harnessName, modeFlagValue)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ui.PrintSuccess("Claude is ready and session associated!")
	return deliverInitialPrompt(ctx, sessionName, true, true)
}

// deliverInitialPrompt sends the user-supplied --prompt or --prompt-file to the
// session. The multiLine flag selects SendMultiLinePromptSafeContext (Claude) vs
// SendPromptLiteral (Gemini/OpenCode/Codex). verifyDelivery enables the generic
// retry verifier, which depends on Claude-style prompt echo/processing signals.
func deliverInitialPrompt(ctx context.Context, sessionName string, multiLine, verifyDelivery bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch {
	case prompt != "":
		return deliverInitialPromptText(ctx, sessionName, multiLine, verifyDelivery)
	case promptFile != "":
		return deliverInitialPromptFile(ctx, sessionName, verifyDelivery)
	default:
		return nil
	}
}

func deliverInitialPromptText(ctx context.Context, sessionName string, multiLine, verifyDelivery bool) error {
	debug.Log("Sending prompt from --prompt flag")
	send := func() error {
		if multiLine {
			return tmux.SendMultiLinePromptSafeContext(ctx, sessionName, prompt, false)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return tmux.SendPromptLiteral(sessionName, prompt, false)
	}
	if err := send(); err != nil {
		return reportInitialPromptSendFailure(ctx, err, "Failed to send prompt", "")
	}
	if !verifyDelivery {
		return nil
	}
	return verifyAndRetryPromptDelivery(ctx, sessionName, prompt, send)
}

func deliverInitialPromptFile(ctx context.Context, sessionName string, verifyDelivery bool) error {
	debug.Log("Sending prompt from --prompt-file flag: %s", promptFile)
	promptContent, readable := readPromptForVerification(promptFile)
	send := func() error {
		return tmux.SendPromptFileSafeContext(ctx, sessionName, promptFile, false)
	}
	if err := send(); err != nil {
		return reportInitialPromptSendFailure(ctx, err, "Failed to send prompt from file", promptFile)
	}
	if !verifyDelivery || !readable {
		return nil
	}
	return verifyAndRetryPromptDelivery(ctx, sessionName, string(promptContent), send)
}

func readPromptForVerification(file string) ([]byte, bool) {
	content, err := os.ReadFile(file)
	return content, err == nil
}

func reportInitialPromptSendFailure(ctx context.Context, sendErr error, message, file string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if file == "" {
		logger.Warn(message, "error", sendErr)
	} else {
		logger.Warn(message, "error", sendErr, "file", file)
	}
	fmt.Println("  • You can manually enter the prompt in the session")
	return nil
}

// runGeminiPostCreate waits for the Gemini prompt and delivers --prompt /
// --prompt-file in non-test, non-detached mode.
func runGeminiPostCreate(ctx context.Context, sessionName string) error {
	debug.Phase("Gemini Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping Gemini prompt wait")
		ui.PrintSuccess("Gemini test session ready (init sequence skipped)")
	case !detached:
		debug.Log("Waiting for Gemini prompt readiness before prompt delivery")
		if err := tmux.WaitForPromptSimpleContext(ctx, sessionName, 30*time.Second); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			debug.Log("Gemini prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("Gemini prompt detected, session ready")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := deliverInitialPrompt(ctx, sessionName, false, true); err != nil {
			return err
		}
	default:
		debug.Log("Detached mode: skipping Gemini prompt wait and prompt delivery")
	}
	return nil
}

// runCodexPostCreate waits for the Codex prompt and delivers --prompt /
// --prompt-file. Codex workers are normally created detached, so detached mode
// still delivers the startup prompt after the composer is ready.
//
// It uses the Codex-specific WaitForCodexPrompt (rather than the generic
// WaitForPromptSimple) so readiness keys on Codex's composer signals and any
// first-run trust/onboarding prompt is auto-accepted inside the wait, ensuring
// prompt delivery never races the consent dialog or a not-yet-ready TUI.
func runCodexPostCreate(ctx context.Context, sessionName string) error {
	debug.Phase("Codex Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping Codex prompt wait")
		ui.PrintSuccess("Codex test session ready (init sequence skipped)")
	case detached && prompt == "" && promptFile == "":
		debug.Log("Detached mode with no prompt: skipping Codex prompt wait")
	default:
		debug.Log("Waiting for Codex prompt readiness before prompt delivery")
		if err := tmux.WaitForCodexPromptContext(ctx, sessionName, 30*time.Second); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			debug.Log("Codex prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("Codex prompt detected, session ready")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := deliverInitialPrompt(ctx, sessionName, false, false); err != nil {
			return err
		}
	}
	return nil
}

// runAgyPostCreate waits for the AGY prompt, captures the spawned AGY
// conversation ID, and delivers --prompt / --prompt-file even in detached mode
// once the interactive prompt is ready.
type agyPostCreateRuntime struct {
	wait               func(context.Context, string, time.Duration) error
	associate          func(string)
	deliver            func(context.Context, string, bool, bool) error
	associateWithRetry func(context.Context, string, int, time.Duration) error
}

func realAgyPostCreateRuntime() agyPostCreateRuntime {
	return agyPostCreateRuntime{
		wait:               tmux.WaitForAgyPrompt,
		associate:          associateSpawnedAgySession,
		deliver:            deliverInitialPrompt,
		associateWithRetry: associateSpawnedAgySessionWithRetry,
	}
}

func runAgyPostCreate(ctx context.Context, sessionName string) error {
	return runAgyPostCreateWithRuntime(ctx, sessionName, realAgyPostCreateRuntime())
}

func runAgyPostCreateWithRuntime(ctx context.Context, sessionName string, runtime agyPostCreateRuntime) error {
	debug.Phase("AGY Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping AGY prompt wait")
		ui.PrintSuccess("AGY test session ready (init sequence skipped)")
	default:
		debug.Log("Waiting for AGY prompt readiness before metadata capture and prompt delivery")
		if err := runtime.wait(ctx, sessionName, 30*time.Second); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("wait for AGY prompt readiness: %w", err)
		} else {
			debug.Log("AGY prompt detected, session ready")
		}
		runtime.associate(sessionName)
		if err := runtime.deliver(ctx, sessionName, false, false); err != nil {
			return err
		}
		if prompt != "" || promptFile != "" {
			if err := runtime.wait(ctx, sessionName, 60*time.Second); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("wait for AGY post-prompt readiness: %w", err)
			}
			if err := runtime.associateWithRetry(ctx, sessionName, 20, 500*time.Millisecond); err != nil {
				return err
			}
		}
	}
	return nil
}

// runOpenCodePostCreate waits for the OpenCode prompt and delivers
// --prompt / --prompt-file in non-test, non-detached mode.
func runOpenCodePostCreate(ctx context.Context, sessionName string) error {
	debug.Phase("OpenCode Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping OpenCode prompt wait")
		ui.PrintSuccess("OpenCode test session ready (init sequence skipped)")
	case !detached:
		debug.Log("Waiting for OpenCode prompt readiness before prompt delivery")
		if err := tmux.WaitForPromptSimpleContext(ctx, sessionName, 30*time.Second); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			debug.Log("OpenCode prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("OpenCode prompt detected, session ready")
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := deliverInitialPrompt(ctx, sessionName, false, true); err != nil {
			return err
		}
	default:
		debug.Log("Detached mode: skipping OpenCode prompt wait and prompt delivery")
	}
	return nil
}
