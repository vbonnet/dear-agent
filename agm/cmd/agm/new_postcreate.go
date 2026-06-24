package main

import (
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
func runHarnessPostCreate(sessionName string, modeAppliedAtStartup bool) error {
	switch {
	case harnessName == "claude-code" && os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "":
		return runClaudePostCreate(sessionName, modeAppliedAtStartup)
	case harnessName == "claude-code":
		debug.Phase("Skip Association (Test Environment)")
		debug.Log("Skipping deterministic association: AGM_TEST_RUN_ID=%s AGM_TEST_ENV=%s",
			os.Getenv("AGM_TEST_RUN_ID"), os.Getenv("AGM_TEST_ENV"))
		ui.PrintSuccess("Test session ready (association skipped)")
		return nil
	case harnessName == "gemini-cli":
		runGeminiPostCreate(sessionName)
		return nil
	case harnessName == "opencode-cli":
		runOpenCodePostCreate(sessionName)
		return nil
	case harnessName == "codex-cli":
		runCodexPostCreate(sessionName)
		return nil
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
func runClaudePostCreate(sessionName string, modeAppliedAtStartup bool) error {
	associateSpawnedClaudeSession(sessionName)
	if modeFlagValue != "" && !modeAppliedAtStartup {
		applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
	}
	ui.PrintSuccess("Claude is ready and session associated!")
	deliverInitialPrompt(sessionName, true, true)
	return nil
}

// deliverInitialPrompt sends the user-supplied --prompt or --prompt-file to the
// session. The multiLine flag selects SendMultiLinePromptSafe (Claude) vs
// SendPromptLiteral (Gemini/OpenCode/Codex). verifyDelivery enables the generic
// retry verifier, which depends on Claude-style prompt echo/processing signals.
func deliverInitialPrompt(sessionName string, multiLine, verifyDelivery bool) {
	if prompt != "" {
		debug.Log("Sending prompt from --prompt flag")
		var sendErr error
		if multiLine {
			sendErr = tmux.SendMultiLinePromptSafe(sessionName, prompt, false)
		} else {
			sendErr = tmux.SendPromptLiteral(sessionName, prompt, false)
		}
		if sendErr != nil {
			logger.Warn("Failed to send prompt", "error", sendErr)
			fmt.Println("  • You can manually enter the prompt in the session")
			return
		}
		if verifyDelivery {
			verifyAndRetryPromptDelivery(sessionName, prompt, func() error {
				if multiLine {
					return tmux.SendMultiLinePromptSafe(sessionName, prompt, false)
				}
				return tmux.SendPromptLiteral(sessionName, prompt, false)
			})
		}
		return
	}
	if promptFile == "" {
		return
	}
	debug.Log("Sending prompt from --prompt-file flag: %s", promptFile)
	promptContent, readErr := os.ReadFile(promptFile)
	if err := tmux.SendPromptFileSafe(sessionName, promptFile, false); err != nil {
		logger.Warn("Failed to send prompt from file", "error", err, "file", promptFile)
		fmt.Println("  • You can manually enter the prompt in the session")
		return
	}
	if verifyDelivery && readErr == nil {
		verifyAndRetryPromptDelivery(sessionName, string(promptContent), func() error {
			return tmux.SendPromptFileSafe(sessionName, promptFile, false)
		})
	}
}

// runGeminiPostCreate waits for the Gemini prompt and delivers --prompt /
// --prompt-file in non-test, non-detached mode.
func runGeminiPostCreate(sessionName string) {
	debug.Phase("Gemini Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping Gemini prompt wait")
		ui.PrintSuccess("Gemini test session ready (init sequence skipped)")
	case !detached:
		debug.Log("Waiting for Gemini prompt readiness before prompt delivery")
		if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
			debug.Log("Gemini prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("Gemini prompt detected, session ready")
		}
		deliverInitialPrompt(sessionName, false, true)
	default:
		debug.Log("Detached mode: skipping Gemini prompt wait and prompt delivery")
	}
}

// runCodexPostCreate waits for the Codex prompt and delivers --prompt /
// --prompt-file. Codex workers are normally created detached, so detached mode
// still delivers the startup prompt after the composer is ready.
//
// It uses the Codex-specific WaitForCodexPrompt (rather than the generic
// WaitForPromptSimple) so readiness keys on Codex's composer signals and any
// first-run trust/onboarding prompt is auto-accepted inside the wait, ensuring
// prompt delivery never races the consent dialog or a not-yet-ready TUI.
func runCodexPostCreate(sessionName string) {
	debug.Phase("Codex Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping Codex prompt wait")
		ui.PrintSuccess("Codex test session ready (init sequence skipped)")
	case detached && prompt == "" && promptFile == "":
		debug.Log("Detached mode with no prompt: skipping Codex prompt wait")
	default:
		debug.Log("Waiting for Codex prompt readiness before prompt delivery")
		if err := tmux.WaitForCodexPrompt(sessionName, 30*time.Second); err != nil {
			debug.Log("Codex prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("Codex prompt detected, session ready")
		}
		deliverInitialPrompt(sessionName, false, false)
	}
}

// runOpenCodePostCreate waits for the OpenCode prompt and delivers
// --prompt / --prompt-file in non-test, non-detached mode.
func runOpenCodePostCreate(sessionName string) {
	debug.Phase("OpenCode Post-Create")
	switch {
	case os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "":
		debug.Log("Test environment: skipping OpenCode prompt wait")
		ui.PrintSuccess("OpenCode test session ready (init sequence skipped)")
	case !detached:
		debug.Log("Waiting for OpenCode prompt readiness before prompt delivery")
		if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
			debug.Log("OpenCode prompt readiness wait failed (non-fatal): %v", err)
		} else {
			debug.Log("OpenCode prompt detected, session ready")
		}
		deliverInitialPrompt(sessionName, false, true)
	default:
		debug.Log("Detached mode: skipping OpenCode prompt wait and prompt delivery")
	}
}
