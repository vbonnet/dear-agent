package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/debug"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/readiness"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/tmux"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/ui"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/workflow"
)

var (
	detached     bool
	agentName    string
	workflowName string
	projectID    string
	prompt       string
	promptFile   string
)

var newCmd = &cobra.Command{
	Use:   "new [session-name]",
	Short: "Create a new Claude session with tmux",
	Long: `Create a new Claude session with tmux integration.

This command will:
1. Create or use an existing tmux session with the specified name
2. Start Claude CLI in the tmux session
3. Create a manifest linking the tmux session to the Claude session

Arguments:
  session-name - Name for the tmux/Claude session (optional)
                 If not provided and outside tmux, you'll be prompted
                 If not provided and inside tmux, uses current tmux session name

Flags:
  --detached - Create session without attaching (useful when inside tmux)

Behavior:
  • Outside tmux + no name → Prompts for name, creates tmux + claude
  • Outside tmux + name provided → Creates tmux session with that name + claude
  • Inside tmux + no name → Uses current tmux name, starts claude
  • Inside tmux + matching name → Uses current tmux, starts claude
  • Inside tmux + different name → Error (name mismatch) unless --detached
  • --detached flag → Creates session, doesn't attach (stays in current context)

Examples:
  csm new                       # Prompt for name or use current tmux session
  csm new my-project            # Create session named "my-project" and attach
  csm new feature-branch        # Create session named "feature-branch" and attach
  csm new other-session --detached  # Create detached session (from within tmux)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Get debug flag
		debugEnabled, _ := cmd.Flags().GetBool("debug")

		inTmux := os.Getenv("TMUX") != ""
		var sessionName string
		var err error

		// Determine session name based on context
		if len(args) > 0 {
			// User provided a session name
			sessionName = args[0]

			// If inside tmux, verify name matches current session
			if inTmux {
				currentTmuxName, err := tmux.GetCurrentSessionName()
				if err != nil {
					ui.PrintError(err,
						"Failed to get current tmux session name",
						"  • Verify you're inside tmux: echo $TMUX\n"+
							"  • Check tmux is running: tmux list-sessions\n"+
							"  • Exit and re-enter tmux if TMUX env var is stale")
					return err
				}

				if sessionName != currentTmuxName && !detached {
					ui.PrintError(
						fmt.Errorf("session name mismatch: %s (provided) != %s (current tmux)", sessionName, currentTmuxName),
						"Cannot create session with different name while inside tmux",
						"  • Use --detached flag to create separate session, or\n  • Exit tmux first, or\n  • Use 'csm new' without arguments to use current tmux session",
					)
					return fmt.Errorf("session name mismatch")
				}
			}
		} else {
			// No name provided
			if inTmux {
				// Use current tmux session name
				sessionName, err = tmux.GetCurrentSessionName()
				if err != nil {
					ui.PrintError(err,
						"Failed to get current tmux session name",
						"  • Verify you're inside tmux: echo $TMUX\n"+
							"  • Check tmux is running: tmux list-sessions\n"+
							"  • Exit and re-enter tmux if TMUX env var is stale")
					return err
				}
				fmt.Printf("Using current tmux session: %s\n", sessionName)
			} else {
				// Prompt for session name
				var inputName string
				err = huh.NewInput().
					Title("Enter session name:").
					Value(&inputName).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("session name cannot be empty")
						}
						return nil
					}).
					Run()
				if err != nil {
					ui.PrintError(err,
						"Failed to read session name from prompt",
						"  • Provide name as argument: csm new <session-name>\n"+
							"  • Check terminal is interactive (TTY)\n"+
							"  • Try running outside tmux/screen if inside")
					return err
				}
				sessionName = inputName

				if sessionName == "" {
					ui.PrintError(
						fmt.Errorf("session name cannot be empty"),
						"Invalid session name",
						"  • Provide a non-empty session name",
					)
					return fmt.Errorf("empty session name")
				}
			}
		}

		// Prompt for agent if not provided via flag
		if agentName == "" {
			var selectedAgent string
			options := []huh.Option[string]{
				huh.NewOption("Claude (Anthropic CLI)", "claude"),
				huh.NewOption("Gemini (Google Vertex AI)", "gemini"),
			}
			err := huh.NewSelect[string]().
				Title("Which agent would you like to use?").
				Options(options...).
				Value(&selectedAgent).
				Run()
			if err != nil {
				ui.PrintError(err,
					"Failed to read agent selection",
					"  • Use --agent flag for non-interactive usage: csm new --agent=claude\n"+
						"  • Check terminal is interactive (TTY)\n"+
						"  • Available agents: claude, gemini")
				return err
			}
			agentName = selectedAgent
		}

		// Initialize debug logging
		if err := debug.Init(debugEnabled, sessionName); err != nil {
			fmt.Printf("Warning: Failed to initialize debug logging: %v\n", err)
		}
		defer debug.Close()

		debug.Phase("Session Creation Started")
		debug.Log("Session name: %s", sessionName)
		debug.Log("In tmux: %v", inTmux)
		debug.Log("Debug enabled: %v", debugEnabled)

		// Validate agent name
		if err := agent.ValidateAgentName(agentName); err != nil {
			ui.PrintError(err,
				"Invalid agent specified",
				"  • Valid agents: claude, gemini, gpt\n"+
					"  • Run 'agm agent list' to see available agents")
			return err
		}

		// Warn if agent unavailable (but allow session creation)
		if err := agent.ValidateAgentAvailability(agentName); err != nil {
			ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
		}

		debug.Log("Agent: %s", agentName)

		// Validate workflow compatibility if workflow specified
		if workflowName != "" {
			if err := workflow.ValidateCompatibility(workflowName, agentName); err != nil {
				ui.PrintError(err,
					"Workflow not compatible with agent",
					fmt.Sprintf("  • Workflow '%s' does not support agent '%s'\n"+
						"  • Run 'csm workflow list' to see available workflows\n"+
						"  • Run 'csm workflow list --agent=%s' to see compatible workflows",
						workflowName, agentName, agentName))
				return err
			}
			debug.Log("Workflow: %s (compatible with %s)", workflowName, agentName)
		}

		// Set GCP_PROJECT_ID environment variable if provided (for gemini agent)
		if projectID != "" {
			os.Setenv("GCP_PROJECT_ID", projectID)
			debug.Log("Set GCP_PROJECT_ID: %s", projectID)
		}

		// Now we have a session name. Handle the scenarios:
		// 1. Inside tmux + not detached: start Claude in current session
		// 2. Outside tmux OR detached: create tmux session, start Claude, attach (or not if detached)

		if inTmux && !detached {
			return startClaudeInCurrentTmux(sessionName)
		}

		return createTmuxSessionAndStartClaude(sessionName)
	},
}

// createTmuxSessionAndStartClaude creates a new tmux session and starts Claude in it
func createTmuxSessionAndStartClaude(sessionName string) error {
	debug.Phase("Get Working Directory")
	// Get current working directory (prefer PWD to preserve symlinks)
	workDir := os.Getenv("PWD")
	if workDir == "" {
		// Fall back to os.Getwd() if PWD not set
		var err error
		workDir, err = os.Getwd()
		if err != nil {
			ui.PrintError(err,
				"Failed to get current directory",
				"  • Check directory still exists: pwd\n"+
					"  • Verify directory permissions: ls -ld .\n"+
					"  • Try from a different directory")
			return err
		}
		debug.Log("Using os.Getwd(): %s", workDir)
	} else {
		debug.Log("Using $PWD: %s", workDir)
	}

	fmt.Printf("Creating new tmux session: %s (in %s)\n", sessionName, workDir)

	// Add working directory to Claude's additionalDirectories to prevent trust prompt
	debug.Phase("Configure Trust")
	debug.Log("Adding %s to Claude's additionalDirectories", workDir)
	trustPreConfigured := false
	if err := addToAdditionalDirectories(workDir); err != nil {
		debug.Log("Warning: failed to add to additionalDirectories: %v", err)
		ui.PrintWarning("Could not pre-authorize directory - trust prompt may appear")
	} else {
		debug.Log("Successfully added to additionalDirectories")
		trustPreConfigured = true
	}

	// Check if tmux session already exists
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		ui.PrintError(err,
			"Failed to check tmux session",
			"  • Verify tmux is installed: tmux -V\n"+
				"  • Check tmux server is running: tmux list-sessions\n"+
				"  • Try starting tmux server: tmux start-server")
		return err
	}

	if exists {
		// If detached mode, skip prompts and reuse existing session
		if detached {
			fmt.Printf("Reusing existing tmux session: %s (detached mode)\n", sessionName)
		} else {
			// Prompt user for action
			var choiceStr string
			options := []huh.Option[string]{
				huh.NewOption("Reuse existing tmux session (start Claude in it)", "0"),
				huh.NewOption("Choose a different name", "1"),
				huh.NewOption("Cancel", "2"),
			}
			err = huh.NewSelect[string]().
				Title(fmt.Sprintf("Tmux session '%s' already exists. What would you like to do?", sessionName)).
				Options(options...).
				Value(&choiceStr).
				Run()
			if err != nil {
				ui.PrintError(err,
					"Failed to read choice from prompt",
					"  • Choose different name: csm new <different-name>\n"+
						"  • Check terminal is interactive (TTY)\n"+
						"  • Cancel with Ctrl+C and retry")
				return err
			}

			// Convert string choice to int for switch statement
			var choice int
			fmt.Sscanf(choiceStr, "%d", &choice)

			switch choice {
			case 0:
				// Reuse existing session
				fmt.Printf("Reusing existing tmux session: %s\n", sessionName)
			case 1:
				// Prompt for new name
				var newName string
				err = huh.NewInput().
					Title("Enter new session name:").
					Value(&newName).
					Validate(func(s string) error {
						if s == "" {
							return fmt.Errorf("session name cannot be empty")
						}
						return nil
					}).
					Run()
				if err != nil {
					ui.PrintError(err,
						"Failed to read session name from prompt",
						"  • Provide name as argument: csm new <session-name>\n"+
							"  • Check terminal is interactive (TTY)\n"+
							"  • Try running outside tmux/screen if inside")
					return err
				}
				if newName == "" {
					ui.PrintError(
						fmt.Errorf("session name cannot be empty"),
						"Invalid session name",
						"",
					)
					return fmt.Errorf("empty session name")
				}
				sessionName = newName
				// Recursively handle the new name (might also conflict)
				return createTmuxSessionAndStartClaude(sessionName)
			case 2:
				// Cancel
				fmt.Println("Cancelled.")
				return nil
			}
		}
	} else {
		// Create new tmux session
		debug.Phase("Create Tmux Session")
		debug.Log("Creating tmux session: %s in %s", sessionName, workDir)
		if err := tmux.NewSession(sessionName, workDir); err != nil {
			ui.PrintError(err,
				"Failed to create tmux session",
				"  • Verify tmux is installed: tmux -V\n"+
					"  • Check tmux server is running: tmux list-sessions\n"+
					"  • Verify directory exists: ls -ld "+workDir+"\n"+
					"  • Try starting tmux server: tmux start-server")
			return err
		}
		debug.Log("Tmux session created successfully")
		ui.PrintSuccess(fmt.Sprintf("Created tmux session: %s", sessionName))
	}

	// Start agent-specific initialization
	var spinErr error
	switch agentName {
	case "claude":
		// Start Claude in the session
		// Use --add-dir to pre-approve workspace and avoid trust prompt blocking the ">" prompt
		debug.Phase("Start Claude")
		claudeCmd := fmt.Sprintf("claude --add-dir '%s' && exit", workDir)
		debug.Log("Sending command: %s", claudeCmd)
		if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
			ui.PrintError(err,
				"Failed to start Claude in tmux session",
				"  • Verify Claude is installed: which claude\n"+
					"  • Test Claude manually: claude --version\n"+
					"  • Check tmux session exists: tmux list-sessions\n"+
					"  • Attach and start manually: tmux attach -t "+sessionName)
			// Try to kill the tmux session if we just created it and Claude failed
			if !exists {
				_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
			}
			return err
		}
		debug.Log("Claude command sent successfully")
		ui.PrintSuccess("Started Claude CLI in tmux session")

		// Give Claude a moment to initialize before we start polling
		debug.Log("Initial sleep (500ms) before polling")
		time.Sleep(500 * time.Millisecond)

		// Wait for Claude prompt to appear (not just process)
		// This ensures commands sent via InitSequence go to Claude, not bash
		// Increased timeout to 30s to account for MCP loading and SessionStart hooks
		debug.Phase("Wait for Claude Prompt")
		debug.Log("Waiting for Claude prompt to appear (timeout: 30s)")
		var waitErr error
		spinErr = spinner.New().
			Title("Waiting for Claude to be ready...").
			Accessible(true).
			Action(func() {
				waitErr = tmux.WaitForClaudePrompt(sessionName, 30*time.Second)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if waitErr != nil {
			debug.Log("Prompt wait timed out or failed: %v", waitErr)
			ui.PrintWarning("Claude prompt not detected (still initializing)")
			fmt.Println("  Proceeding anyway - initialization may be delayed")
			// Continue anyway - InitSequence will handle if commands arrive too early
		} else {
			debug.Log("Claude prompt detected - ready for commands")
			ui.PrintSuccess("Claude is ready!")
		}

		// Monitor for trust prompt using control mode (event-driven, not time-based)
		// Only answer if we actually detect the prompt appearing
		// Skip if we successfully pre-configured trust (saves ~30s due to blocking scanner.Scan)
		if trustPreConfigured {
			debug.Phase("Skip Trust Prompt Monitoring")
			debug.Log("Skipping trust prompt monitoring since directory was pre-configured")
		} else {
			debug.Phase("Monitor for Trust Prompt")
			debug.Log("Starting control mode to monitor for trust prompt")
			if err := monitorAndAnswerTrustPrompt(sessionName, 10*time.Second); err != nil {
				debug.Log("Trust prompt handling: %v", err)
				// Non-fatal - either no prompt appeared (good) or we couldn't answer it (user can manually)
			}
		}

		// Wait for SessionStart hooks to complete before sending commands
		// SessionStart hooks run immediately when Claude starts (no message needed)
		debug.Phase("Wait for SessionStart Hooks")
		debug.Log("Waiting for SessionStart hooks to complete (they run at Claude startup)")
		spinErr = spinner.New().
			Title("Waiting for SessionStart hooks to complete...").
			Accessible(true).
			Action(func() {
				time.Sleep(2 * time.Second) // Give hooks time to start (reduced from 5s)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

	case "gemini":
		// Check for csm-agent-wrapper
		debug.Phase("Start Gemini")
		wrapperPath, err := exec.LookPath("csm-agent-wrapper")
		if err != nil {
			// Graceful fallback to direct gemini (wrapper not found)
			debug.Log("csm-agent-wrapper not found, falling back to direct gemini: %v", err)
			geminiCmd := "gemini && exit"
			debug.Log("Sending command: %s", geminiCmd)
			if err := tmux.SendCommand(sessionName, geminiCmd); err != nil {
				ui.PrintError(err,
					"Failed to start Gemini in tmux session",
					"  • Verify Gemini is installed: which gemini\n"+
						"  • Test Gemini manually: gemini --version\n"+
						"  • Check tmux session exists: tmux list-sessions\n"+
						"  • Attach and start manually: tmux attach -t "+sessionName)
				if !exists {
					_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
				}
				return err
			}
			debug.Log("Gemini command sent successfully (direct mode)")
			ui.PrintSuccess("Started Gemini CLI in tmux session")
		} else {
			// Use wrapper for readiness detection
			debug.Log("Found csm-agent-wrapper at: %s", wrapperPath)
			debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini %s", wrapperPath, sessionName)

			// Execute wrapper directly (it will attach to the session)
			// The wrapper handles:
			// 1. Starting Gemini in the tmux session
			// 2. Waiting for readiness
			// 3. Creating ready-file
			// 4. Attaching to session
			// 5. Capturing output on exit
			cmd := exec.Command(wrapperPath, "--agent=gemini", sessionName)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				ui.PrintError(err,
					"Failed to run csm-agent-wrapper",
					"  • Check wrapper installed: which csm-agent-wrapper\n"+
						"  • Try direct mode by temporarily renaming wrapper\n"+
						"  • Attach and check: tmux attach -t "+sessionName)
				if !exists {
					_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
				}
				return err
			}

			// Wrapper finished (user exited session)
			ui.PrintSuccess("Gemini session ended")
			return nil
		}

		// OLD CODE BELOW - ONLY REACHED IN FALLBACK MODE
		ui.PrintSuccess("Started Gemini CLI in tmux session (direct mode)")

		// HACK: The code below this point is the old fallback flow
		// It should never be reached when using the wrapper
		// Keep it for now in case we need fallback behavior
		if false {
			// Wait for ready-file
			debug.Phase("Wait for Ready Signal")
			var readyErr error
			spinErr := spinner.New().
				Title("Waiting for Gemini to initialize...").
				Accessible(true).
				Action(func() {
					readyErr = readiness.WaitForReady(sessionName, 60*time.Second)
				}).
				Run()
			if spinErr != nil {
				return fmt.Errorf("spinner error: %w", spinErr)
			}
			if readyErr != nil {
				debug.Log("Ready-file wait failed: %v", readyErr)
				ui.PrintWarning("Gemini did not signal ready within timeout")
				fmt.Println("  • Attach to session to check status: tmux attach -t " + sessionName)
				fmt.Println("  • Check wrapper logs for errors")
				fmt.Println("  • Try direct mode: csm new --agent gemini " + sessionName + " (after renaming wrapper)")
				return fmt.Errorf("gemini readiness timeout: %w", readyErr)
			}
			debug.Log("Ready signal received successfully")
			ui.PrintSuccess("Gemini initialized successfully")
		}

	default:
		// Other agents (gpt, etc) - no CLI startup configured yet
		debug.Phase("Skip CLI Startup")
		debug.Log("Skipping CLI startup for agent: %s (no CLI configured)", agentName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s agent", sessionName))
	}

	// Create manifest BEFORE sending /rename (so hook can find it)
	debug.Phase("Create Manifest")
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		ui.PrintWarning("Proceeding without manifest - you can run 'csm sync' later")
	} else {
		// Create v2 manifest with proper SessionID and empty Claude UUID
		// The /csm-assoc command will populate the Claude UUID when it runs
		generatedUUID := uuid.New().String()
		debug.Log("Generated SessionID: %s", generatedUUID)
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     generatedUUID, // Generate proper UUID
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "", // Empty = active/stopped
			Context: manifest.Context{
				Project: workDir,
				Purpose: "", // Can be set later
				Tags:    nil,
				Notes:   "",
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Agent: agentName,
			Claude: manifest.Claude{
				UUID: "", // Will be populated by SessionStart hook
			},
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to write manifest: %v", err))
			ui.PrintWarning("Proceeding without manifest - you can run 'csm sync' later")
		} else {
			debug.Log("Manifest created at: %s", manifestPath)
			ui.PrintSuccess(fmt.Sprintf("Created manifest: %s", manifestPath))
		}
	}

	// Claude-specific: Run initialization sequence
	if agentName == "claude" {
		// NOTE: No need to release global lock - using fine-grained tmux lock instead
		// InitSequence will acquire/release tmux lock only during actual operations

		// Use InitSequence to properly sequence /rename and /csm-assoc commands
		// This uses tmux control mode to wait for each command to complete before sending the next
		debug.Phase("Sequenced Initialization")
		debug.Log("Running InitSequence for /rename and /csm-assoc")
		seq := tmux.NewInitSequence(sessionName)
		if err := seq.Run(); err != nil {
			debug.Log("InitSequence failed: %v", err)
			ui.PrintWarning("Failed to run initialization sequence")
			fmt.Printf("💡 You can manually run:\n")
			fmt.Printf("  /rename %s\n", sessionName)
			fmt.Printf("  /csm-tools:csm-assoc %s\n", sessionName)
		} else {
			debug.Log("InitSequence completed successfully")
		}

		// Wait for ready-file (created by csm associate when UUID is captured)
		debug.Phase("Wait for Ready Signal")
		debug.Log("Waiting for ready-file signal (timeout: 60s)")
		var readyErr error
		var spinErr2 error
		spinErr2 = spinner.New().
			Title("Waiting for Claude to initialize...").
			Accessible(true).
			Action(func() {
				readyErr = readiness.WaitForReady(sessionName, 60*time.Second)
			}).
			Run()
		if spinErr2 != nil {
			return fmt.Errorf("spinner error: %w", spinErr2)
		}
		if readyErr != nil {
			debug.Log("Ready-file wait failed: %v", readyErr)
			ui.PrintWarning("Claude did not signal ready within timeout")
			fmt.Println("  • Attach to session to check status: tmux attach -t " + sessionName)
			fmt.Printf("  • Run 'csm sync' later to populate UUID if needed\n")
		} else {
			debug.Log("Ready-file detected - session is ready")
			ui.PrintSuccess("Claude is ready and session associated!")

			// Wait for /csm-assoc skill to finish outputting its completion messages
			// The ready-file signals when 'csm associate' binary completes, but the skill
			// continues to output messages after that. Give it time to finish.
			debug.Log("Waiting for /csm-assoc skill to complete output")
			// NOTE: Skill completion detection not yet implemented (requires control mode output channel)
			// Using fixed sleep as fallback
			time.Sleep(500 * time.Millisecond)

			// Send prompt if provided via --prompt or --prompt-file flags
			if prompt != "" {
				debug.Log("Sending prompt from --prompt flag")
				if err := tmux.SendMultiLinePromptSafe(sessionName, prompt); err != nil {
					log.Printf("Warning: failed to send prompt: %v", err)
					fmt.Println("  • You can manually enter the prompt in the session")
				}
			} else if promptFile != "" {
				debug.Log("Sending prompt from --prompt-file flag: %s", promptFile)
				if err := tmux.SendPromptFileSafe(sessionName, promptFile); err != nil {
					log.Printf("Warning: failed to send prompt from file: %v", err)
					fmt.Println("  • You can manually enter the prompt in the session")
				}
			}
		}
	} else {
		// API-based agents - no initialization sequence needed
		debug.Log("Skipping initialization sequence for API-based agents")
	}

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(sessionName)

	// Attach to session (or show detached message)
	if !detached {
		fmt.Printf("Attaching to tmux session: %s\n", sessionName)
		// Use wrapper for all agents to capture exit summaries
		if err := attachWithCapture(sessionName); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
			fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", sessionName)
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' created (detached)", sessionName))
		fmt.Printf("\nAttach to session with:\n  csm resume %s\n", sessionName)
		fmt.Printf("Or manually:\n  tmux attach -t %s\n", sessionName)
	}

	return nil
}

// attachWithCapture uses csm-attach-wrapper to attach and capture exit summary
func attachWithCapture(sessionName string) error {
	// Find wrapper binary
	wrapperPath, err := exec.LookPath("csm-attach-wrapper")
	if err != nil {
		// Fallback to direct attach if wrapper not found
		debug.Log("Wrapper not found, falling back to direct attach: %v", err)
		return tmux.AttachSession(sessionName)
	}

	// Build arguments
	args := []string{
		"csm-attach-wrapper",
		sessionName,
	}

	// Get environment
	env := os.Environ()

	// Exec wrapper (replaces current process)
	debug.Log("Executing wrapper: %s %v", wrapperPath, args)
	return syscall.Exec(wrapperPath, args, env)
}

// getSessionsDir returns the sessions directory (respects --sessions-dir flag)
func getSessionsDir() string {
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/sessions
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "sessions")
}

// startClaudeInCurrentTmux starts a fresh Claude session in the current tmux session
func startClaudeInCurrentTmux(sessionName string) error {
	fmt.Printf("Starting new Claude session in current tmux: %s\n", sessionName)

	// Get working directory
	workDir, err := os.Getwd()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current directory",
			"  • Check directory still exists: pwd\n"+
				"  • Verify directory permissions: ls -ld .\n"+
				"  • Try from a different directory")
		return err
	}

	// Create or update manifest
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	// Create manifest directory if it doesn't exist
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
	} else {
		// Create v2 manifest with proper SessionID and empty Claude UUID
		// The /csm-assoc command will populate the Claude UUID when it runs
		generatedUUID := uuid.New().String()
		debug.Log("Generated SessionID: %s", generatedUUID)
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     generatedUUID, // Generate proper UUID
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "", // Empty = active
			Context: manifest.Context{
				Project: workDir,
				Purpose: "",
				Tags:    nil,
				Notes:   "",
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Agent: agentName,
		}

		if err := manifest.Write(manifestPath, m); err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to write manifest: %v", err))
		} else {
			ui.PrintSuccess(fmt.Sprintf("Created/updated manifest: %s", manifestPath))
		}
	}

	// Start agent-specific initialization
	switch agentName {
	case "claude":
		// Start Claude in current pane
		fmt.Println("Starting Claude CLI...")
		// Use --add-dir to pre-approve workspace and avoid trust prompt
		// Prefer PWD to preserve symlinks (workDir already set from os.Getwd() above)
		workDirForClaude := os.Getenv("PWD")
		if workDirForClaude == "" {
			workDirForClaude = workDir
		}
		claudeCmd := fmt.Sprintf("claude --add-dir '%s' && exit", workDirForClaude)
		if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
			ui.PrintError(err,
				"Failed to start Claude in current tmux pane",
				"  • Verify Claude is installed: which claude\n"+
					"  • Test Claude manually: claude --version\n"+
					"  • Check you're in tmux: echo $TMUX\n"+
					"  • Exit tmux and try: csm new "+sessionName)
			return err
		}

		// Wait for Claude process to appear (more reliable than prompt character)
		fmt.Println("Waiting for Claude to initialize...")
		if err := tmux.WaitForProcessReady(sessionName, "claude", 30*time.Second); err != nil {
			ui.PrintWarning("Claude may still be initializing")
			fmt.Printf("💡 Session is ready, but process not detected. This is usually fine.\n")
		}

		// Send /csm-tools:csm-assoc command to associate session with CSM
		// This runs the csm-assoc skill which will auto-rename the session
		assocCmd := "/csm-tools:csm-assoc"
		if err := tmux.SendCommand(sessionName, assocCmd); err != nil {
			ui.PrintWarning("Failed to auto-associate session")
			fmt.Printf("💡 You can manually run: /csm-tools:csm-assoc\n")
		} else {
			ui.PrintSuccess("Sent /csm-tools:csm-assoc to associate session")
		}

	default:
		// API-based agents (gemini, gpt) - no CLI needed
		debug.Log("Skipping CLI startup for API-based agent: %s", agentName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s agent", agentName))
	}

	ui.PrintSuccess(fmt.Sprintf("%s session started in current tmux!", agentName))

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(sessionName)

	return nil
}

// monitorAndAnswerTrustPrompt monitors tmux output via control mode and answers trust prompt if detected
// Returns nil if no prompt appears (success), error if prompt appears but we can't answer it
func monitorAndAnswerTrustPrompt(sessionName string, timeout time.Duration) error {
	// Start control mode
	ctrl, err := tmux.StartControlMode(sessionName)
	if err != nil {
		return fmt.Errorf("failed to start control mode: %w", err)
	}
	defer ctrl.Close()

	// Create output watcher
	watcher := tmux.NewOutputWatcher(ctrl.Stdout)

	deadline := time.Now().Add(timeout)
	trustPromptDetected := false

	for time.Now().Before(deadline) {
		// Read next line with short timeout
		line, err := watcher.GetRawLine(1 * time.Second)
		if err != nil {
			// Timeout reading - no more output
			// If we haven't seen trust prompt, assume it won't appear
			if !trustPromptDetected {
				debug.Log("No trust prompt detected (good - additionalDirectories likely worked)")
				return nil
			}
			continue
		}

		// Parse %output events
		if !strings.HasPrefix(line, "%output") {
			continue
		}

		content := tmux.ExtractOutputContent(line)

		// Check for trust prompt
		if strings.Contains(content, "Do you trust the files in this folder?") {
			trustPromptDetected = true
			debug.Log("Trust prompt detected!")
			fmt.Println("📋 Trust prompt appeared - answering automatically...")
		}

		// If we detected the prompt, look for the selection UI
		if trustPromptDetected && strings.Contains(content, "Yes, proceed") {
			debug.Log("Sending Enter to select 'Yes, proceed'")

			// Close control mode before sending keys (mixing control + send-keys doesn't work well)
			ctrl.Close()

			// Send Enter key via regular tmux
			if err := tmux.SendCommand(sessionName, "C-m"); err != nil {
				return fmt.Errorf("failed to answer trust prompt: %w", err)
			}

			debug.Log("Trust prompt answered successfully")
			ui.PrintSuccess("Trust prompt answered")
			return nil
		}
	}

	if trustPromptDetected {
		return fmt.Errorf("trust prompt detected but couldn't find 'Yes, proceed' option")
	}

	// No trust prompt seen - this is success
	return nil
}

// addToAdditionalDirectories adds a directory to Claude's additionalDirectories in settings.json
// This prevents the trust prompt from appearing for this directory
func addToAdditionalDirectories(dir string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")

	// Read existing settings
	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new settings file
			settings = make(map[string]interface{})
		} else {
			return fmt.Errorf("failed to read settings.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse settings.json: %w", err)
		}
	}

	// Get or create additionalDirectories array
	var additionalDirs []string
	if existing, ok := settings["additionalDirectories"]; ok {
		if dirs, ok := existing.([]interface{}); ok {
			for _, d := range dirs {
				if str, ok := d.(string); ok {
					additionalDirs = append(additionalDirs, str)
				}
			}
		}
	}

	// Check if directory already exists
	for _, d := range additionalDirs {
		if d == dir {
			// Already present, no need to add
			return nil
		}
	}

	// Add the new directory
	additionalDirs = append(additionalDirs, dir)
	settings["additionalDirectories"] = additionalDirs

	// Write back to settings.json
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, output, 0600); err != nil {
		return fmt.Errorf("failed to write settings.json: %w", err)
	}

	return nil
}

func init() {
	// Check for CSM_DEBUG environment variable for default value
	debugDefault := false
	if os.Getenv("CSM_DEBUG") == "true" || os.Getenv("CSM_DEBUG") == "1" {
		debugDefault = true
	}

	rootCmd.AddCommand(newCmd)
	newCmd.Flags().BoolP("debug", "d", debugDefault, "Enable debug logging to ~/.csm/debug/ (env: CSM_DEBUG)")
	newCmd.Flags().BoolVar(&detached, "detached", false, "Create detached session without attaching")
	newCmd.Flags().StringVar(&agentName, "agent", "", "AI agent to use (claude, gemini, gpt)")
	newCmd.Flags().StringVar(&workflowName, "workflow", "", "Execution workflow (deep-research, code-review, architect)")
	newCmd.Flags().StringVar(&projectID, "project-id", "", "GCP project ID (required for gemini agent)")
	newCmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send after session initialization")
	newCmd.Flags().StringVar(&promptFile, "prompt-file", "", "File containing prompt to send")
	newCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	// agent flag is now optional - prompt shown if omitted
}
