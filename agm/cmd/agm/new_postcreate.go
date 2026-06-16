package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/huh/spinner"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/readiness"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// runHarnessPostCreate runs the harness-specific post-create flow (init
// sequence + ready-file wait for Claude, prompt-readiness wait + prompt
// delivery for Gemini/OpenCode).
func runHarnessPostCreate(sessionName string, modeAppliedAtStartup bool) error {
	switch {
	case harnessName == "claude-code" && os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "":
		return runClaudePostCreate(sessionName, modeAppliedAtStartup)
	case harnessName == "claude-code":
		debug.Phase("Skip Init Sequence (Test Environment)")
		debug.Log("Skipping InitSequence: AGM_TEST_RUN_ID=%s AGM_TEST_ENV=%s",
			os.Getenv("AGM_TEST_RUN_ID"), os.Getenv("AGM_TEST_ENV"))
		ui.PrintSuccess("Test session ready (init sequence skipped)")
		return nil
	case harnessName == "gemini-cli":
		runGeminiPostCreate(sessionName)
		return nil
	case harnessName == "opencode-cli":
		runOpenCodePostCreate(sessionName)
		return nil
	default:
		debug.Log("Skipping initialization sequence for harness: %s", harnessName)
		return nil
	}
}

// runClaudePostCreate runs the Claude rename/agm-assoc init sequence and
// waits for the ready-file signal. Sends prompts on success.
func runClaudePostCreate(sessionName string, modeAppliedAtStartup bool) error {
	debug.Phase("Sequenced Initialization")
	debug.Log("Running InitSequence for /rename and /agm:agm-assoc")
	seq := tmux.NewInitSequence(sessionName)
	seq.PromptVerified = true
	if err := seq.Run(); err != nil {
		debug.Log("InitSequence failed: %v", err)
		ui.PrintWarning("Failed to run initialization sequence")
		fmt.Printf("💡 You can manually run:\n")
		fmt.Printf("  /rename %s\n", sessionName)
		fmt.Printf("  /agm:agm-assoc %s\n", sessionName)
	} else {
		debug.Log("InitSequence completed successfully")
	}

	debug.Phase("Wait for Ready Signal")
	readyTimeout := readiness.ReadyTimeout()
	debug.Log("Waiting for ready-file signal (timeout: %v)", readyTimeout)
	var readyErr error
	spinErr2 := spinner.New().
		Title("Waiting for Claude to initialize...").
		Accessible(true).
		Action(func() {
			readyErr = readiness.WaitForReady(sessionName, readyTimeout)
		}).
		Run()
	if spinErr2 != nil {
		return fmt.Errorf("spinner error: %w", spinErr2)
	}
	if readyErr != nil {
		reportClaudeReadyFailure(sessionName, readyErr)
		return nil //nolint:nilerr // failure already surfaced via reportClaudeReadyFailure; CLI continues
	}
	debug.Log("Ready-file detected - agm binary completed")
	waitForSkillCompletion(sessionName)
	if modeFlagValue != "" && !modeAppliedAtStartup {
		applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
	}
	ui.PrintSuccess("Claude is ready and session associated!")
	deliverInitialPrompt(sessionName, true)
	return nil
}

// reportClaudeReadyFailure prints a structured error when the ready-file did
// not appear within the timeout. Non-fatal: session remains usable.
func reportClaudeReadyFailure(sessionName string, readyErr error) {
	debug.Log("Ready-file wait failed: %v", readyErr)
	homeDir, _ := os.UserHomeDir()
	stateDir := filepath.Join(homeDir, ".agm")
	readyFilePath := filepath.Join(stateDir, "ready-"+sessionName)
	ui.PrintError(
		readyErr,
		fmt.Sprintf("Ready-file not created at: %s", readyFilePath),
		fmt.Sprintf("  The init sequence sent '/agm:agm-assoc %s' into the session, but no\n"+
			"  ready-file appeared. The most common cause in a spawned/sandbox session\n"+
			"  is that the agm plugin slash command did not load (Claude reports\n"+
			"  '/agm:agm-assoc: Unknown command'), so association never ran.\n\n"+
			"  Deterministic recovery (does NOT depend on the plugin or permission mode):\n"+
			"    agm session associate %s --create   # run from the session's working dir\n\n"+
			"  Diagnose:\n"+
			"    • Attach and check Claude output: tmux attach -t %s\n"+
			"    • Confirm the plugin is enabled:  agm admin doctor slash-commands\n"+
			"    • Check debug logs:               ls -lt ~/.agm/debug/\n\n"+
			"  Note: Session is still usable, but UUID association may have failed.",
			sessionName, sessionName, sessionName),
	)
}

// waitForSkillCompletion uses layered detection (pattern → idle → prompt) to
// wait for the /agm:agm-assoc skill output to settle.
func waitForSkillCompletion(sessionName string) {
	debug.Log("Waiting for skill to complete output using smart detection")
	if err := tmux.WaitForPattern(sessionName, "[AGM_SKILL_COMPLETE]", 5*time.Second); err == nil {
		debug.Log("✓ Skill completion marker detected")
		return
	}
	debug.Log("Pattern detection timeout (non-fatal)")
	if err := tmux.WaitForOutputIdle(sessionName, 1*time.Second, 15*time.Second); err == nil {
		debug.Log("✓ Output idle detected - skill appears complete")
		return
	}
	debug.Log("Idle detection failed")
	if err := tmux.WaitForClaudePrompt(sessionName, 5*time.Second); err != nil {
		debug.Log("Prompt detection failed (non-fatal): %v", err)
		time.Sleep(1 * time.Second)
	}
}

// deliverInitialPrompt sends the user-supplied --prompt or --prompt-file to
// the session. The multiLine flag selects SendMultiLinePromptSafe (Claude) vs
// SendPromptLiteral (Gemini/OpenCode).
func deliverInitialPrompt(sessionName string, multiLine bool) {
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
		verifyAndRetryPromptDelivery(sessionName, prompt, func() error {
			if multiLine {
				return tmux.SendMultiLinePromptSafe(sessionName, prompt, false)
			}
			return tmux.SendPromptLiteral(sessionName, prompt, false)
		})
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
	if readErr == nil {
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
		deliverInitialPrompt(sessionName, false)
	default:
		debug.Log("Detached mode: skipping Gemini prompt wait and prompt delivery")
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
		deliverInitialPrompt(sessionName, false)
	default:
		debug.Log("Detached mode: skipping OpenCode prompt wait and prompt delivery")
	}
}
