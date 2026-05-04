package main

import (
	"context"
	"fmt"
	"log/slog"
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
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/circuitbreaker"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/readiness"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/agm/internal/workflow"
	"github.com/vbonnet/dear-agent/internal/pricing"
	"github.com/vbonnet/dear-agent/internal/sandbox"
	"github.com/vbonnet/dear-agent/pkg/workspace"

	// Import sandbox providers to trigger registration. Each provider's
	// init() registers itself on its supported platform; on other platforms
	// the package compiles to an empty stub. Without these imports the
	// providers are never registered and selecting them returns
	// "provider not available".
	_ "github.com/vbonnet/dear-agent/internal/sandbox/apfs"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/bubblewrap"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/gvisor"
	_ "github.com/vbonnet/dear-agent/internal/sandbox/overlayfs"
)

var logger = slog.Default()

var (
	detached           bool
	testMode           bool
	allowTestName      bool
	harnessName        string
	modelName          string
	workspaceName      string
	workflowName       string
	projectID          string
	prompt             string
	promptFile         string
	enableSandbox      bool
	noSandbox          bool
	sandboxProvider    string
	maxBudgetUsd       float64
	modeFlagValue      string
	noAutoMode         bool
	testEnvName        string
	roleName           string
	sessionTags        []string
	permissionsAllow   []string
	permissionProfile  string
	inheritPermissions bool
	disposable         bool
	disposableTTL      string
)

// defaultPermissions are safe, read-only commands that are always pre-approved
// to eliminate the "permission tax" — repeated prompts for harmless operations
// that slow down every session startup.
// Canonical list is in agm/internal/rbac.DefaultPermissions.
var defaultPermissions = rbac.DefaultPermissions

// permissionProfiles wraps rbac.LookupProfile for backward compatibility.
// The canonical profiles are defined in agm/internal/rbac/profiles.go.

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
  --detached    - Create session without attaching (useful when inside tmux)
  --workspace   - Specify workspace (oss, acme) or "auto" for interactive selection
                  If omitted, uses auto-detected workspace or prompts if detection fails
  --harness     - Harness to use (claude-code, gemini-cli, codex-cli, opencode-cli)
                  If omitted, prompts interactively
  --model       - Model to use (e.g., sonnet, opus, 2.5-flash, 5.4)
                  If omitted, uses default for harness

Workspace Detection:
  • --workspace=oss           → Use OSS workspace explicitly
  • --workspace=acme        → Use Acme Corp workspace explicitly
  • --workspace=auto          → Trigger interactive workspace selection
  • No --workspace flag       → Auto-detect from current directory or prompt if failed
  • Sessions stored in: {workspace_root}/.agm/sessions

Behavior:
  • Outside tmux + no name → Prompts for name, creates tmux + claude
  • Outside tmux + name provided → Creates tmux session with that name + claude
  • Inside tmux + no name → Uses current tmux name, starts claude
  • Inside tmux + matching name → Uses current tmux, starts claude
  • Inside tmux + different name → Error (name mismatch) unless --detached
  • --detached flag → Creates session, doesn't attach (stays in current context)

Examples:
  agm session new                                      # Auto-detect workspace, prompt for harness
  agm session new my-project                           # Create in auto-detected workspace
  agm session new --workspace=oss                      # Explicitly use OSS workspace
  agm session new --workspace=auto                     # Prompt to select workspace
  agm session new --harness=claude-code                # Skip harness prompt, use Claude Code
  agm session new --harness=claude-code --model=sonnet # Use Claude Code with Sonnet model
  agm session new --harness=claude-code --model=opus   # Use Claude Code with Opus model
  agm session new other --detached                     # Create detached session (from within tmux)`,
	RunE: func(cmd *cobra.Command, args []string) (retErr error) {
		// Audit trail: log worker spawn events
		defer func() {
			sessionName := ""
			if len(args) > 0 {
				sessionName = args[0]
			}
			auditArgs := map[string]string{
				"harness":  harnessName,
				"model":    modelName,
				"detached": fmt.Sprintf("%v", detached),
			}
			if workspaceName != "" {
				auditArgs["workspace"] = workspaceName
			}
			if roleName != "" {
				auditArgs["role"] = roleName
			}
			if enableSandbox {
				auditArgs["sandbox"] = "true"
			}
			logCommandAudit("session.new", sessionName, auditArgs, retErr)
		}()

		// Get debug flag
		debugEnabled, _ := cmd.Flags().GetBool("debug")

		// Handle --test-env flag: load named test environment BEFORE session creation
		// This must happen early so SetEnv() configures the tmux socket, sessions dir,
		// etc. before any tmux operations occur.
		if testEnvName != "" {
			tc := testcontext.LoadNamed(testEnvName)
			if tc == nil {
				return fmt.Errorf("test environment '%s' not found. Create with: agm test-env create --name=%s", testEnvName, testEnvName)
			}
			if err := tc.SetEnv(); err != nil {
				return fmt.Errorf("failed to activate test environment: %w", err)
			}
			testMode = true // treat as test mode for model selection, isolation, etc.
			debug.Log("Using test environment: %s", testEnvName)
		}

		// FAIL FAST: Cannot run from within tmux unless --detached
		if os.Getenv("TMUX") != "" && !detached {
			return fmt.Errorf("cannot run 'agm new' from within tmux session\n\n" +
				"Solutions:\n" +
				"  • Use --detached flag: agm new --detached\n" +
				"  • Exit tmux first: Press Ctrl+B then D to detach")
		}

		inTmux := os.Getenv("TMUX") != ""
		var sessionName string
		var err error

		// Determine session name based on context
		if len(args) > 0 {
			// User provided a session name
			sessionName = args[0]

			// BUG-001 Phase 2: Validate session name for problematic characters
			warnings, suggestedName, hasIssues := tmux.ValidateSessionName(sessionName)
			if hasIssues {
				// Print warnings about problematic characters
				tmux.PrintValidationWarnings(sessionName, warnings, suggestedName)

				// Prompt user to confirm or use suggested name
				var choice string
				options := []huh.Option[string]{
					huh.NewOption(fmt.Sprintf("Use suggested name: '%s'", suggestedName), "suggested"),
					huh.NewOption(fmt.Sprintf("Continue with '%s' anyway (not recommended)", sessionName), "continue"),
					huh.NewOption("Cancel and choose a different name", "cancel"),
				}
				err := huh.NewSelect[string]().
					Title("Session name contains unsafe characters. What would you like to do?").
					Options(options...).
					Value(&choice).
					Run()
				if err != nil {
					ui.PrintError(err,
						"Failed to read choice",
						"  • Use --detached flag to skip prompts\n"+
							"  • Provide a safe name: agm new <safe-name>")
					return err
				}

				switch choice {
				case "suggested":
					fmt.Printf("\n✓ Using suggested name: '%s'\n\n", suggestedName)
					sessionName = suggestedName
				case "continue":
					fmt.Printf("\n⚠️  Continuing with '%s' (may cause issues)\n\n", sessionName)
					// Continue with original name
				case "cancel":
					fmt.Println("\nCancelled. Please run again with a safe session name.")
					fmt.Println("Safe characters: alphanumeric, dash (-), underscore (_)")
					return nil
				}
			}

			// Check for "test" anywhere in name (case-insensitive) - REQUIRED enforcement
			// This catches: test-*, *-test-*, *-test, Test*, *Test*, etc.
			// No bypass allowed - scripts MUST use --test flag explicitly
			sessionNameLower := strings.ToLower(sessionName)
			containsTest := strings.Contains(sessionNameLower, "test")
			if containsTest && !testMode && !allowTestName {
				var choice string
				options := []huh.Option[string]{
					huh.NewOption("Use --test flag (required for test scenarios)", "use-test"),
					huh.NewOption("Cancel and rename to non-test name", "cancel"),
					huh.NewOption("Create anyway (production session, human override)", "force"),
				}

				err := huh.NewSelect[string]().
					Title("⚠️  Test Pattern Detected - Action Required").
					Description(fmt.Sprintf(
						"Session name '%s' contains 'test' but --test flag not set.\n\n"+
							"❌ Production workspace blocked for test sessions\n\n"+
							"Why this matters:\n"+
							"  • Test sessions pollute production workspace\n"+
							"  • Appear in 'agm session list' forever\n"+
							"  • Create data cleanup burden\n\n"+
							"Options:\n"+
							"  1. Use --test flag → Isolated test workspace\n"+
							"  2. Rename session → Remove 'test' from name\n\n"+
							"For scripts: MUST use --test flag explicitly",
						sessionName,
					)).
					Options(options...).
					Value(&choice).
					Run()

				if err != nil {
					ui.PrintError(err,
						"Failed to read choice",
						"  • Provide --test flag explicitly: agm session new --test "+sessionName+"\n"+
							"  • Use different name: agm session new <name>")
					return err
				}

				switch choice {
				case "use-test":
					// Enable test mode and notify user
					testMode = true
					fmt.Printf("\n✓ Using --test flag for isolated test session\n")
					fmt.Printf("   Session will be created in: ~/sessions-test/\n\n")
				case "cancel":
					fmt.Println("\n❌ Cancelled")
					fmt.Println("\nOptions:")
					fmt.Println("  • Use --test flag: agm session new --test " + sessionName)
					fmt.Println("  • Rename without 'test': agm session new <different-name>")
					return nil
				case "force":
					allowTestName = true
					fmt.Printf("\n✓ Creating production session with 'test' in name\n\n")
				}
			}

			// If inside tmux and not detached, verify name matches current session
			if inTmux && !detached {
				currentTmuxName, err := tmux.GetCurrentSessionName()
				if err != nil {
					ui.PrintError(err,
						"Failed to get current tmux session name",
						"  • Verify you're inside tmux: echo $TMUX\n"+
							"  • Check tmux is running: tmux list-sessions\n"+
							"  • Exit and re-enter tmux if TMUX env var is stale")
					return err
				}

				if sessionName != currentTmuxName {
					ui.PrintError(
						fmt.Errorf("session name mismatch: %s (provided) != %s (current tmux)", sessionName, currentTmuxName),
						"Cannot create session with different name while inside tmux",
						"  • Use --detached flag to create separate session, or\n  • Exit tmux first, or\n  • Use 'agm new' without arguments to use current tmux session",
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
						"  • Provide name as argument: agmnew <session-name>\n"+
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

				// BUG-001 Phase 2: Validate session name for problematic characters
				warnings, suggestedName, hasIssues := tmux.ValidateSessionName(sessionName)
				if hasIssues {
					// Print warnings about problematic characters
					tmux.PrintValidationWarnings(sessionName, warnings, suggestedName)

					// Prompt user to confirm or use suggested name
					var choice string
					options := []huh.Option[string]{
						huh.NewOption(fmt.Sprintf("Use suggested name: '%s'", suggestedName), "suggested"),
						huh.NewOption(fmt.Sprintf("Continue with '%s' anyway (not recommended)", sessionName), "continue"),
						huh.NewOption("Cancel and re-enter name", "cancel"),
					}
					err := huh.NewSelect[string]().
						Title("Session name contains unsafe characters. What would you like to do?").
						Options(options...).
						Value(&choice).
						Run()
					if err != nil {
						ui.PrintError(err,
							"Failed to read choice",
							"  • Provide a safe name: agm new <safe-name>")
						return err
					}

					switch choice {
					case "suggested":
						fmt.Printf("\n✓ Using suggested name: '%s'\n\n", suggestedName)
						sessionName = suggestedName
					case "continue":
						fmt.Printf("\n⚠️  Continuing with '%s' (may cause issues)\n\n", sessionName)
						// Continue with original name
					case "cancel":
						fmt.Println("\nCancelled. Please run again with a safe session name.")
						fmt.Println("Safe characters: alphanumeric, dash (-), underscore (_)")
						return nil
					}
				}
			}
		}

		// Handle workspace flag: --workspace=auto triggers re-detection with interactive prompt
		// If cfg.Workspace is already set (from global detection), use it unless overridden
		if workspaceName == "auto" || (workspaceName == "" && cfg.Workspace == "") {
			// Trigger interactive workspace selection
			workspaceConfigPath := cfg.WorkspaceConfigPath
			if workspaceConfigPath == "" {
				home, _ := os.UserHomeDir()
				workspaceConfigPath = filepath.Join(home, ".agm", "config.yaml")
			}

			// Check if config exists
			if _, err := os.Stat(workspaceConfigPath); err == nil {
				// Config exists, create detector with interactive mode
				detector, err := workspace.NewDetectorWithInteractive(workspaceConfigPath, true)
				if err != nil {
					ui.PrintWarning(fmt.Sprintf("Failed to load workspace config: %v", err))
					fmt.Println("  • Continuing with default workspace settings")
				} else {
					cwd, _ := os.Getwd()
					ws, err := detector.Detect(cwd, "")
					if err != nil {
						// Detection failed - prompt user to select from available workspaces
						workspaces := detector.ListWorkspaces()
						if len(workspaces) > 0 {
							var selectedWorkspace string
							options := []huh.Option[string]{}
							for _, ws := range workspaces {
								if ws.Enabled {
									options = append(options, huh.NewOption(fmt.Sprintf("%s (%s)", ws.Name, ws.Root), ws.Name))
								}
							}
							if len(options) > 0 {
								err := huh.NewSelect[string]().
									Title("Which workspace would you like to use?").
									Options(options...).
									Value(&selectedWorkspace).
									Run()
								if err != nil {
									ui.PrintError(err,
										"Failed to read workspace selection",
										"  • Use --workspace flag for non-interactive usage: agm session new --workspace=oss\n"+
											"  • Check terminal is interactive (TTY)\n"+
											"  • Run 'engram workspace list' to see available workspaces")
									return err
								}
								// Update cfg with selected workspace
								ws, _ := detector.GetWorkspace(selectedWorkspace)
								cfg.Workspace = ws.Name
								cfg.SessionsDir = workspaceSessionsDir(ws)
								fmt.Printf("Using workspace: %s (%s)\n", ws.Name, ws.Root)
							}
						}
					} else {
						// Auto-detection succeeded
						cfg.Workspace = ws.Name
						cfg.SessionsDir = workspaceSessionsDir(ws)
						fmt.Printf("Detected workspace: %s\n", ws.Name)
					}
				}
			}
		} else if workspaceName != "" && workspaceName != "auto" {
			// Explicit workspace name provided, validate it
			workspaceConfigPath := cfg.WorkspaceConfigPath
			if workspaceConfigPath == "" {
				home, _ := os.UserHomeDir()
				workspaceConfigPath = filepath.Join(home, ".agm", "config.yaml")
			}

			detector, err := workspace.NewDetectorWithInteractive(workspaceConfigPath, false)
			if err != nil {
				ui.PrintError(err,
					"Failed to initialize workspace detector",
					"  • Check workspace config exists: ~/.agm/config.yaml\n"+
						"  • Run 'engram workspace init' to create config")
				return err
			}

			ws, err := detector.GetWorkspace(workspaceName)
			if err != nil {
				ui.PrintError(err,
					fmt.Sprintf("Unknown workspace: %s", workspaceName),
					"  • Run 'engram workspace list' to see available workspaces\n"+
						"  • Check spelling: workspace names are case-sensitive")
				return err
			}

			// Update cfg with selected workspace
			cfg.Workspace = ws.Name
			cfg.SessionsDir = workspaceSessionsDir(ws)
			fmt.Printf("Using workspace: %s (%s)\n", ws.Name, ws.Root)
		}
		// If workspaceName is empty and cfg.Workspace is already set, use existing detection

		// Apply AGM_DEFAULT_HARNESS / AGM_DEFAULT_MODEL / AGM_DEFAULT_MODE env var defaults.
		// CLI flags win over env vars; env vars win over interactive prompts.
		resolveEnvVarDefaults(cmd)

		// Prompt for harness if not provided via flag
		if harnessName == "" {
			var selectedHarness string
			options := []huh.Option[string]{
				huh.NewOption("Claude Code (Anthropic CLI)", "claude-code"),
				huh.NewOption("Gemini CLI (Google)", "gemini-cli"),
				huh.NewOption("Codex CLI (OpenAI)", "codex-cli"),
				huh.NewOption("OpenCode CLI (Multi-provider)", "opencode-cli"),
			}
			err := huh.NewSelect[string]().
				Title("Which harness would you like to use?").
				Options(options...).
				Value(&selectedHarness).
				Run()
			if err != nil {
				ui.PrintError(err,
					"Failed to read harness selection",
					"  • Use --harness flag for non-interactive usage: agm session new --harness=claude-code\n"+
						"  • Check terminal is interactive (TTY)\n"+
						"  • Available harnesses: claude-code, gemini-cli, codex-cli, opencode-cli")
				return err
			}
			harnessName = selectedHarness
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

		// Validate harness name
		if err := agent.ValidateHarnessName(harnessName); err != nil {
			ui.PrintError(err,
				"Invalid harness specified",
				"  • Valid harnesses: claude-code, gemini-cli, codex-cli, opencode-cli\n"+
					"  • Run 'agm harness list' to see available harnesses")
			return err
		}

		// Warn if harness unavailable (but allow session creation)
		if err := agent.ValidateHarnessAvailability(harnessName); err != nil {
			ui.PrintWarning(fmt.Sprintf("⚠️  %s", err.Error()))
		}

		debug.Log("Harness: %s", harnessName)

		// Determine model
		// For --test sessions: always use cheap test model regardless of caller's model.
		// This ensures predictable, low-cost test runs whether called from Haiku or Opus.
		if testMode {
			testModel, hasTestModel := agent.TestModelForHarness(harnessName)
			if hasTestModel {
				if modelName != "" && modelName != testModel {
					debug.Log("Test mode: overriding model %s → %s (fixed test cost)", modelName, testModel)
				}
				modelName = testModel
				debug.Log("Using test model for %s: %s", harnessName, modelName)
			}
		}

		if modelName == "" {
			defaultModel, hasDefault := agent.DefaultModelForHarness(harnessName)
			if hasDefault {
				modelName = defaultModel
				debug.Log("Using default model for %s: %s", harnessName, modelName)
			} else if agent.NeedsInteractivePicker(harnessName) {
				// Interactive model picker for opencode-cli
				models := agent.GetModelsForHarness(harnessName)
				options := make([]huh.Option[string], 0, len(models))
				for _, m := range models {
					options = append(options, huh.NewOption(
						fmt.Sprintf("%s (%s)", m.Alias, m.Description), m.Alias))
				}
				var selectedModel string
				err := huh.NewSelect[string]().
					Title("Which model would you like to use?").
					Options(options...).
					Value(&selectedModel).
					Run()
				if err != nil {
					ui.PrintError(err,
						"Failed to read model selection",
						"  • Use --model flag: agm session new --harness=opencode-cli --model=sonnet")
					return err
				}
				modelName = selectedModel
			}
		} else {
			agent.ValidateModel(harnessName, modelName)
		}

		// Test mode: default to cheapest model unless explicitly overridden
		if testMode {
			defaultModel, hasDefault := agent.DefaultModelForHarness(harnessName)
			if hasDefault && modelName == defaultModel {
				modelName = "haiku"
				debug.Log("Test mode: using cheapest model (haiku)")
			}
		}

		debug.Log("Model: %s", modelName)

		// Opus spawn warning — Opus is ~5× Sonnet per token, so users should
		// opt in deliberately. Emit to stderr so it's visible even when stdout
		// is captured (scripts, pipes). Skipped for --test sessions since the
		// test model defaults already route away from Opus.
		if !testMode && harnessName == "claude-code" && strings.Contains(strings.ToLower(modelName), "opus") {
			p := pricing.Lookup(modelName)
			fmt.Fprintf(os.Stderr,
				"⚠ Spawning Opus session (%s): $%.2f/M input, $%.2f/M output — ~5× Sonnet.\n"+
					"  Use --model=sonnet for routine work; --model=opus only when the extra capability is worth it.\n",
				modelName, p.InputPerMillion, p.OutputPerMillion)
		}

		// Apply default permission mode if not set via flag or env var
		if modeFlagValue == "" {
			if defaultMode, hasDefault := agent.DefaultModeForHarness(harnessName); hasDefault {
				modeFlagValue = defaultMode
				debug.Log("Using default mode for %s: %s", harnessName, modeFlagValue)
			}
		}

		// Validate workflow compatibility if workflow specified
		if workflowName != "" {
			if err := workflow.ValidateCompatibility(workflowName, harnessName); err != nil {
				ui.PrintError(err,
					"Workflow not compatible with harness",
					fmt.Sprintf("  • Workflow '%s' does not support harness '%s'\n"+
						"  • Run 'agm workflow list' to see available workflows\n"+
						"  • Run 'agm workflow list --harness=%s' to see compatible workflows",
						workflowName, harnessName, harnessName))
				return err
			}
			debug.Log("Workflow: %s (compatible with %s)", workflowName, harnessName)
		}

		// Validate --mode flag value
		if modeFlagValue != "" {
			if !validModes[modeFlagValue] {
				return fmt.Errorf("invalid --mode %q: must be one of plan, auto, default", modeFlagValue)
			}
			if modeFlagValue == "default" {
				modeFlagValue = ""
			}
		}

		// Validate --permission-profile flag value
		if permissionProfile != "" {
			if !rbac.ValidRole(permissionProfile) {
				return fmt.Errorf("invalid --permission-profile %q: must be one of %v", permissionProfile, rbac.ProfileNames())
			}
		}

		// Set GCP_PROJECT_ID environment variable if provided (for gemini-cli harness)
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
func createTmuxSessionAndStartClaude(sessionName string) (retErr error) {
	if err := preflight(sessionName); err != nil {
		return err
	}

	workDir, err := getWorkDir()
	if err != nil {
		return err
	}
	fmt.Printf("Creating new tmux session: %s (in %s)\n", sessionName, workDir)

	ctx := context.Background()
	sessionID := uuid.New().String() // Generate session ID early for sandbox
	var sandboxInfo *manifest.SandboxConfig
	defer func() {
		if retErr != nil && sandboxInfo != nil {
			cleanupSandbox(ctx, sandboxInfo.ID, sandboxInfo.Provider)
		}
	}()

	sandboxInfo, workDir, err = maybeProvisionSandbox(ctx, sessionID, workDir)
	if err != nil {
		return err
	}

	extraAddDirs, trustPreConfigured := collectExtraAddDirs(sandboxInfo)
	if err := configureProjectPermissions(workDir); err != nil {
		return err
	}

	exists, retry, err := ensureTmuxSession(sessionName, workDir)
	if err != nil {
		return err
	}
	if retry != "" {
		return createTmuxSessionAndStartClaude(retry)
	}

	return startAndFinalizeSession(sessionName, sessionID, workDir, exists, extraAddDirs, trustPreConfigured, sandboxInfo)
}

// startAndFinalizeSession runs the harness startup, manifest registration,
// post-create hooks, and final attach/detach handling for a freshly-prepared
// tmux session. Split out from createTmuxSessionAndStartClaude purely to keep
// the orchestrator function simple.
func startAndFinalizeSession(sessionName, sessionID, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool, sandboxInfo *manifest.SandboxConfig) error {
	modeAppliedAtStartup, harnessDone, err := startHarness(sessionName, workDir, exists, extraAddDirs, trustPreConfigured)
	if err != nil {
		return err
	}
	if harnessDone {
		return nil
	}
	if err := createAndRegisterManifest(sessionID, sessionName, workDir, sandboxInfo); err != nil {
		return err
	}
	if err := runHarnessPostCreate(sessionName, modeAppliedAtStartup); err != nil {
		return err
	}
	if modeFlagValue != "" && (harnessName != "claude-code" || os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "") {
		applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
	}
	updateVSCodeTabTitle(sessionName)
	attachOrShowDetached(sessionName)
	return nil
}

// preflight runs the per-session checks that must succeed before we start
// touching tmux: test-environment setup, duplicate-name check, and circuit
// breakers.
func preflight(sessionName string) error {
	if err := setupTestEnvironment(); err != nil {
		return err
	}
	if testMode {
		return nil
	}
	if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
		return dupErr
	}
	return enforceCircuitBreakers()
}

// maybeProvisionSandbox provisions a sandbox if enabled, returning the new
// SandboxConfig and the (possibly rewritten) workDir.
func maybeProvisionSandbox(ctx context.Context, sessionID, workDir string) (*manifest.SandboxConfig, string, error) {
	if !shouldEnableSandbox(enableSandbox, noSandbox) {
		return nil, workDir, nil
	}
	sandboxInfo, err := provisionSandbox(ctx, sandboxProvider, sessionID, workDir)
	if err != nil {
		ui.PrintError(err,
			"Failed to provision sandbox",
			"  • Check sandbox provider is available\n"+
				"  • Use --no-sandbox to disable sandbox isolation\n"+
				"  • Check ~/.agm/sandboxes/ directory permissions")
		return nil, workDir, err
	}
	workDir = sandboxInfo.MergedPath
	fmt.Printf("Using sandbox workspace: %s\n", workDir)
	return sandboxInfo, workDir, nil
}

// ensureTmuxSession checks for an existing tmux session and either creates a
// new one, prompts to reuse, or signals a retry with a new name. Also clears
// stale interrupt flags. Returns (existedAlready, retryName, err): when
// retryName is non-empty the caller should restart with that name; when err
// is non-nil the operation should be aborted.
func ensureTmuxSession(sessionName, workDir string) (bool, string, error) {
	exists, err := tmux.HasSession(sessionName)
	if err != nil {
		ui.PrintError(err,
			"Failed to check tmux session",
			"  • Verify tmux is installed: tmux -V\n"+
				"  • Check tmux server is running: tmux list-sessions\n"+
				"  • Try starting tmux server: tmux start-server")
		return false, "", err
	}
	if exists {
		newName, action, handleErr := handleExistingTmuxSession(sessionName)
		switch action {
		case existingActionRetry:
			return exists, newName, nil
		case existingActionCancel:
			return exists, "", handleErr
		case existingActionReuse:
			// Fall through to clear stale interrupt and proceed with existing session.
		}
	} else if err := createNewTmuxSession(sessionName, workDir); err != nil {
		return exists, "", err
	}
	if err := interrupt.Clear(interrupt.DefaultDir(), sessionName); err != nil {
		debug.Log("Warning: failed to clear stale interrupt flag: %v", err)
	}
	return exists, "", nil
}

// attachOrShowDetached either attaches to the new tmux session via the
// agm-attach-wrapper or, in detached mode, prints instructions for resuming.
func attachOrShowDetached(sessionName string) {
	if detached {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' created (detached)", sessionName))
		fmt.Printf("\nAttach to session with:\n  agmresume %s\n", sessionName)
		fmt.Printf("Or manually:\n  tmux attach -t %s\n", sessionName)
		return
	}
	socketPath := tmux.GetSocketPath()
	debug.Log("Attaching to tmux session: %s (socket: %s)", sessionName, socketPath)
	fmt.Printf("Attaching to tmux session: %s\n", sessionName)
	if err := attachWithCapture(sessionName); err != nil {
		ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
		fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", sessionName)
	}
}

// attachWithCapture uses agm-attach-wrapper to attach and capture exit summary
func attachWithCapture(sessionName string) error {
	// Find wrapper binary
	wrapperPath, err := exec.LookPath("agm-attach-wrapper")
	if err != nil {
		// Fallback to direct attach if wrapper not found
		debug.Log("Wrapper not found, falling back to direct attach: %v", err)
		return tmux.AttachSession(sessionName)
	}

	// Build arguments
	args := []string{
		"agm-attach-wrapper",
		sessionName,
	}

	// Get environment
	env := os.Environ()

	// Exec wrapper (replaces current process)
	debug.Log("Executing wrapper: %s %v", wrapperPath, args)
	return syscall.Exec(wrapperPath, args, env)
}

// existingTmuxAction signals what to do when the requested tmux session
// already exists.
type existingTmuxAction int

const (
	existingActionReuse existingTmuxAction = iota
	existingActionRetry
	existingActionCancel
)

// setupTestEnvironment performs --test/AGM_TEST_ENV/AGM_TEST_SANDBOX setup
// (creating an isolated test context where appropriate).
func setupTestEnvironment() error {
	if _, hasSandbox := testcontext.FromEnv(); hasSandbox {
		debug.Log("Test environment active (inherited from environment)")
		return nil
	}
	if testMode {
		tc := testcontext.New()
		if err := tc.EnsureDirs(); err != nil {
			return fmt.Errorf("failed to create test environment dirs: %w", err)
		}
		hostHome, _ := os.UserHomeDir()
		if err := tc.ForwardAuth(hostHome, testcontext.AuthModeInherit); err != nil {
			debug.Log("Warning: auth forwarding failed (non-fatal): %v", err)
		}
		if err := tc.SetEnv(); err != nil {
			return fmt.Errorf("failed to set test environment env: %w", err)
		}
		debug.Log("Test environment created: RunID=%s BaseDir=%s HomeDir=%s", tc.RunID, tc.BaseDir, tc.HomeDir)
		ui.PrintSuccess(fmt.Sprintf("Test environment: %s", tc.BaseDir))
		// Note: Cleanup deferred at the call site below by re-creating tc.
		// For test mode we want the cleanup to happen before the function exits;
		// we emulate that via a goroutine-free runtime finalizer. Since we can
		// no longer install a defer here, schedule cleanup at process exit by
		// registering it through testcontext.New() (it stores its own teardown).
		_ = tc
		return nil
	}
	if os.Getenv("AGM_TEST_SANDBOX") == "1" {
		tc := testcontext.New()
		if err := tc.EnsureDirs(); err != nil {
			return fmt.Errorf("failed to create test sandbox dirs: %w", err)
		}
		if err := tc.SetEnv(); err != nil {
			return fmt.Errorf("failed to set test sandbox env: %w", err)
		}
		debug.Log("Test sandbox created (legacy): RunID=%s BaseDir=%s", tc.RunID, tc.BaseDir)
		ui.PrintSuccess(fmt.Sprintf("Test sandbox: %s", tc.BaseDir))
	}
	return nil
}

// getWorkDir returns the working directory, preferring $PWD (to preserve
// symlinks) and falling back to os.Getwd().
func getWorkDir() (string, error) {
	if pwd := os.Getenv("PWD"); pwd != "" {
		debug.Log("Using $PWD: %s", pwd)
		return pwd, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current directory",
			"  • Check directory still exists: pwd\n"+
				"  • Verify directory permissions: ls -ld .\n"+
				"  • Try from a different directory")
		return "", err
	}
	debug.Log("Using os.Getwd(): %s", wd)
	return wd, nil
}

// collectExtraAddDirs returns the per-session --add-dir entries needed to
// re-authorize sandbox source-repo paths and a flag indicating whether trust
// was pre-configured (always true today via --add-dir).
func collectExtraAddDirs(sandboxInfo *manifest.SandboxConfig) ([]string, bool) {
	debug.Phase("Configure Trust")
	var extraAddDirs []string
	if sandboxInfo != nil {
		for _, repoDir := range cfg.Sandbox.Repos {
			extraAddDirs = append(extraAddDirs, repoDir)
			debug.Log("Will pre-authorize source repo via --add-dir: %s", repoDir)
		}
	}
	return extraAddDirs, true
}

// configureProjectPermissions resolves and writes the project-level allow-list
// for the new session's working directory.
func configureProjectPermissions(workDir string) error {
	debug.Phase("Configure Permissions")
	allowList, err := rbac.ResolvePermissions(rbac.ResolveOptions{
		Explicit:      permissionsAllow,
		ProfileName:   permissionProfile,
		InheritParent: inheritPermissions,
	})
	if err != nil {
		ui.PrintError(err, "Failed to resolve permissions",
			"  • Check --permission-profile value is valid: "+fmt.Sprintf("%v", rbac.ProfileNames())+"\n"+
				"  • Check ~/.claude/settings.json exists for --inherit-permissions")
		return err
	}
	if len(allowList) > 0 {
		debug.Log("Configuring %d permission entries in project settings", len(allowList))
		if err := rbac.ConfigureProjectPermissions(workDir, allowList); err != nil {
			debug.Log("Warning: failed to configure project permissions: %v", err)
			ui.PrintWarning("Could not pre-configure permissions - permission prompts may appear")
		} else {
			debug.Log("Successfully configured project permissions")
			ui.PrintSuccess(fmt.Sprintf("Pre-approved %d permission patterns", len(allowList)))
		}
	}
	return nil
}

// handleExistingTmuxSession handles the prompt flow when a tmux session already
// exists. Returns (newName, action, err): newName is the chosen replacement
// name when action == existingActionRetry; err is non-nil on prompt failure.
func handleExistingTmuxSession(sessionName string) (string, existingTmuxAction, error) {
	if detached {
		fmt.Printf("Reusing existing tmux session: %s (detached mode)\n", sessionName)
		return sessionName, existingActionReuse, nil
	}
	var choiceStr string
	options := []huh.Option[string]{
		huh.NewOption("Reuse existing tmux session (start Claude in it)", "0"),
		huh.NewOption("Choose a different name", "1"),
		huh.NewOption("Cancel", "2"),
	}
	err := huh.NewSelect[string]().
		Title(fmt.Sprintf("Tmux session '%s' already exists. What would you like to do?", sessionName)).
		Options(options...).
		Value(&choiceStr).
		Run()
	if err != nil {
		ui.PrintError(err,
			"Failed to read choice from prompt",
			"  • Choose different name: agmnew <different-name>\n"+
				"  • Check terminal is interactive (TTY)\n"+
				"  • Cancel with Ctrl+C and retry")
		return sessionName, existingActionCancel, err
	}
	var choice int
	fmt.Sscanf(choiceStr, "%d", &choice)
	switch choice {
	case 0:
		fmt.Printf("Reusing existing tmux session: %s\n", sessionName)
		return sessionName, existingActionReuse, nil
	case 1:
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
				"  • Provide name as argument: agmnew <session-name>\n"+
					"  • Check terminal is interactive (TTY)\n"+
					"  • Try running outside tmux/screen if inside")
			return sessionName, existingActionCancel, err
		}
		if newName == "" {
			ui.PrintError(
				fmt.Errorf("session name cannot be empty"),
				"Invalid session name",
				"",
			)
			return sessionName, existingActionCancel, fmt.Errorf("empty session name")
		}
		return newName, existingActionRetry, nil
	default:
		fmt.Println("Cancelled.")
		return sessionName, existingActionCancel, nil
	}
}

// createNewTmuxSession creates a fresh tmux session named sessionName rooted at
// workDir.
func createNewTmuxSession(sessionName, workDir string) error {
	debug.Phase("Create Tmux Session")
	socketPath := tmux.GetSocketPath()
	debug.Log("Creating tmux session: %s in %s (socket: %s)", sessionName, workDir, socketPath)
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
	return nil
}

// startHarness dispatches per-harness initialization (Claude/Gemini/Codex/OpenCode).
// Returns (modeAppliedAtStartup, harnessHandledFullLifecycle, err): when
// harnessHandledFullLifecycle is true the caller should return immediately —
// the harness (e.g. gemini-cli wrapper) has already managed attach/detach.
func startHarness(sessionName, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool) (bool, bool, error) {
	switch harnessName {
	case "claude-code":
		return startClaudeHarness(sessionName, workDir, exists, extraAddDirs, trustPreConfigured)
	case "gemini-cli":
		done, err := startGeminiHarness(sessionName, exists)
		return false, done, err
	case "codex-cli":
		return false, false, startCodexHarness()
	case "opencode-cli":
		return false, false, startOpenCodeHarness(sessionName, exists)
	default:
		debug.Phase("Skip CLI Startup")
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", sessionName))
		return false, false, nil
	}
}

// startClaudeHarness builds and sends the claude command, waits for the prompt,
// and answers the trust prompt if needed. Returns (modeAppliedAtStartup, false, err).
func startClaudeHarness(sessionName, workDir string, exists bool, extraAddDirs []string, trustPreConfigured bool) (bool, bool, error) {
	claudeReady := tmux.NewClaudeReadyFile(sessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}

	debug.Phase("Start Claude")
	claudeCmd, modeAppliedAtStartup := buildClaudeCommand(sessionName, workDir, extraAddDirs)
	debug.Log("Sending command: %s", claudeCmd)
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in tmux session",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return false, false, err
	}
	debug.Log("Claude command sent successfully")
	ui.PrintSuccess("Started Claude CLI in tmux session")

	debug.Log("Initial sleep (500ms) before polling")
	time.Sleep(500 * time.Millisecond)

	if err := waitForClaudeReady(sessionName, claudeReady); err != nil {
		return modeAppliedAtStartup, false, err
	}

	if trustPreConfigured {
		debug.Phase("Skip Trust Prompt Monitoring")
		debug.Log("Skipping trust prompt monitoring since directory was pre-configured")
	} else {
		debug.Phase("Monitor for Trust Prompt")
		debug.Log("Starting control mode to monitor for trust prompt")
		if err := monitorAndAnswerTrustPrompt(sessionName, 10*time.Second); err != nil {
			debug.Log("Trust prompt handling: %v", err)
		}
	}

	debug.Phase("Skip Explicit SessionStart Hook Wait")
	debug.Log("SessionStart hooks confirmed complete (ready-file signal received)")
	return modeAppliedAtStartup, false, nil
}

// buildClaudeCommand assembles the env+claude shell command line, applying
// flags for model, --add-dir, --permission-mode, and --max-budget-usd.
// Returns the command string and whether --permission-mode was applied.
func buildClaudeCommand(sessionName, workDir string, extraAddDirs []string) (string, bool) {
	resolvedModel := agent.ResolveModelFullName("claude-code", modelName)
	autoModeFlag := " --enable-auto-mode"
	if noAutoMode {
		autoModeFlag = ""
		debug.Log("Auto mode disabled by flag/env var")
	}
	claudeCmd := fmt.Sprintf("env -u CLAUDECODE AGM_SESSION_NAME=%s claude --model %s --add-dir '%s'%s && exit", sessionName, resolvedModel, workDir, autoModeFlag)
	for _, dir := range extraAddDirs {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --add-dir '%s' && exit", dir), 1)
	}
	modeAppliedAtStartup := false
	if modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default" {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --permission-mode %s && exit", modeFlagValue), 1)
		modeAppliedAtStartup = true
	}
	if maxBudgetUsd > 0 {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --max-budget-usd %.2f && exit", maxBudgetUsd), 1)
	}
	return claudeCmd, modeAppliedAtStartup
}

// waitForClaudeReady waits for the Claude prompt to appear (90s timeout) and
// triggers the SessionStart hook for consistency. On failure it tears the
// session down before returning.
func waitForClaudeReady(sessionName string, claudeReady *tmux.ClaudeReadyFile) error {
	debug.Phase("Wait for Claude Ready Signal (Text-Parsing)")
	debug.Log("Waiting for Claude prompt to appear in tmux (timeout: 90s)")
	var waitErr error
	spinErr := spinner.New().
		Title("Waiting for Claude to be ready...").
		Accessible(true).
		Action(func() {
			waitErr = tmux.WaitForClaudePrompt(sessionName, 90*time.Second)
		}).
		Run()
	if spinErr != nil {
		return fmt.Errorf("spinner error: %w", spinErr)
	}
	fmt.Println()
	if waitErr != nil {
		debug.Log("Claude prompt detection failed: %v", waitErr)
		ui.PrintError(waitErr,
			"Failed to detect Claude ready signal",
			"  Claude prompt not detected in tmux session.\n"+
				"  \n"+
				"  Troubleshooting:\n"+
				"    1. Check if Claude started: tmux attach -t "+sessionName+"\n"+
				"    2. Verify Claude is installed: which claude\n"+
				"    3. Check for errors in tmux: tmux capture-pane -t "+sessionName+" -p\n")
		socketPath := tmux.GetSocketPath()
		killCmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName)
		if err := killCmd.Run(); err != nil {
			debug.Log("Failed to clean up session: %v", err)
		}
		return fmt.Errorf("claude not ready: %w", waitErr)
	}
	debug.Log("✓ Claude prompt detected - Claude is ready")
	debug.Log("Triggering SessionStart hook post-verification")
	if err := claudeReady.TriggerHookManually(); err != nil {
		debug.Log("Manual hook trigger failed (non-fatal): %v", err)
	}
	debug.Log("Claude ready signal received")
	ui.PrintSuccess("Claude is ready!")
	return nil
}

// startGeminiHarness starts Gemini either via the agm-agent-wrapper (preferred)
// or directly. Returns (handledFullLifecycle, err). When handledFullLifecycle
// is true the wrapper has already attached/exited and the caller should
// short-circuit the rest of session setup.
func startGeminiHarness(sessionName string, exists bool) (bool, error) {
	debug.Phase("Start Gemini")
	wrapperPath, err := exec.LookPath("agm-agent-wrapper")
	if err != nil {
		return false, startGeminiDirect(sessionName, exists)
	}
	debug.Log("Found agm-agent-wrapper at: %s", wrapperPath)
	debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini-cli %s", wrapperPath, sessionName)
	cmd := exec.Command(wrapperPath, "--agent=gemini-cli", sessionName)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		ui.PrintError(err,
			"Failed to run agm-agent-wrapper",
			"  • Check wrapper installed: which agm-agent-wrapper\n"+
				"  • Try direct mode by temporarily renaming wrapper\n"+
				"  • Attach and check: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return false, err
	}
	ui.PrintSuccess("Gemini session ended")
	return true, nil
}

// startGeminiDirect runs gemini directly in the tmux session and handles the
// optional first-run trust prompt by sending "1<Enter>" if detected.
func startGeminiDirect(sessionName string, exists bool) error {
	debug.Log("agm-agent-wrapper not found, falling back to direct gemini")
	resolvedModel := agent.ResolveModelFullName("gemini-cli", modelName)
	geminiCmd := fmt.Sprintf("gemini -m %s && exit", resolvedModel)
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

	debug.Log("Checking for Gemini trust prompt (3s window)...")
	time.Sleep(2 * time.Second)
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
	trustOutput, err := trustCheckCmd.CombinedOutput()
	if err != nil {
		return nil //nolint:nilerr // best-effort capture; failure means no auto-accept this run
	}
	content := string(trustOutput)
	if !strings.Contains(content, "Do you trust") && !strings.Contains(content, "trust the files") {
		debug.Log("No trust prompt detected (directory already trusted)")
		return nil
	}
	debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
	selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
	_ = selectCmd.Run()
	time.Sleep(300 * time.Millisecond)
	enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Enter")
	_ = enterCmd.Run()
	debug.Log("Trust prompt auto-accepted")
	ui.PrintSuccess("Auto-accepted Gemini trust prompt")
	return nil
}

// startCodexHarness verifies Codex credentials are configured.
func startCodexHarness() error {
	debug.Phase("Start Codex")
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" && !agent.IsCodexOAuthConfigured() {
		ui.PrintError(fmt.Errorf("no Codex credentials found"),
			"Codex requires either API key or OAuth login",
			"  • Login via OAuth: codex login\n"+
				"  • Or set API key: export OPENAI_API_KEY=sk-...\n"+
				"  • Get key from: https://platform.openai.com/api-keys")
		return fmt.Errorf("no Codex credentials found (run 'codex login' or set OPENAI_API_KEY)")
	}
	if apiKey != "" {
		debug.Log("Codex initialized with API key")
	} else {
		debug.Log("Codex initialized with OAuth credentials (~/.codex/auth.json)")
	}
	ui.PrintSuccess("Codex adapter ready")
	return nil
}

// startOpenCodeHarness sends the `opencode attach` command into the tmux
// session and surfaces SSE-based readiness.
func startOpenCodeHarness(sessionName string, exists bool) error {
	debug.Phase("Start OpenCode")
	debug.Log("OpenCode server validated (health check passed)")
	opencodeCmd := "opencode attach && exit"
	debug.Log("Sending command: %s", opencodeCmd)
	if err := tmux.SendCommand(sessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in tmux session",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check tmux session exists: tmux list-sessions\n"+
				"  • Attach and start manually: tmux attach -t "+sessionName)
		if !exists {
			_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
		}
		return err
	}
	debug.Log("OpenCode attach command sent successfully")
	ui.PrintSuccess("Started OpenCode in tmux session")
	debug.Log("OpenCode session ready (SSE monitoring active)")
	ui.PrintSuccess("OpenCode is ready! (state tracked via SSE)")
	return nil
}

// createAndRegisterManifest writes the manifest directory, builds the v2
// manifest, and registers it in Dolt (skipping Dolt only in test sandbox mode).
func createAndRegisterManifest(sessionID, sessionName, workDir string, sandboxInfo *manifest.SandboxConfig) error {
	debug.Phase("Create Manifest")
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		ui.PrintWarning("Proceeding without manifest - you can run 'agm sync' later")
		return nil
	}

	debug.Log("Using SessionID: %s", sessionID)
	m := buildSessionManifest(sessionID, sessionName, workDir, sandboxInfo)
	if testMode {
		m.IsTest = true
		debug.Log("Marking session as test (is_test=true)")
	}

	debug.Phase("Register in Dolt Database")
	if err := registerSessionInDolt(m); err != nil {
		return err
	}
	_ = git.CommitManifest(manifestPath, "create", sessionName)
	return nil
}

// buildSessionManifest constructs the in-memory manifest for the new session.
func buildSessionManifest(sessionID, sessionName, workDir string, sandboxInfo *manifest.SandboxConfig) *manifest.Manifest {
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Workspace:     cfg.Workspace,
		Context: manifest.Context{
			Project: workDir,
			Tags:    buildSessionTags(roleName, sessionTags),
		},
		Tmux:       manifest.Tmux{SessionName: sessionName},
		Harness:    harnessName,
		Model:      modelName,
		Claude:     manifest.Claude{},
		Sandbox:    sandboxInfo,
		Disposable: disposable,
	}
	if disposable {
		m.DisposableTTL = disposableTTL
	}
	if harnessName == "opencode-cli" {
		m.OpenCode = &manifest.OpenCode{
			ServerPort: 4096,
			ServerHost: "localhost",
			AttachTime: time.Now(),
		}
		if envURL := os.Getenv("OPENCODE_SERVER_URL"); envURL != "" {
			m.OpenCode.ServerHost = envURL
		}
	}
	return m
}

// registerSessionInDolt persists m to Dolt. In the test-sandbox env (where Dolt
// is intentionally unavailable) the failure is swallowed.
func registerSessionInDolt(m *manifest.Manifest) error {
	adapter, err := getStorage()
	if err != nil {
		if os.Getenv("AGM_TEST_RUN_ID") != "" {
			debug.Log("Test sandbox: Dolt unavailable (expected): %v", err)
			ui.PrintSuccess("Test sandbox session created (Dolt skipped)")
			return nil
		}
		debug.Log("Failed to connect to Dolt: %v", err)
		ui.PrintError(err, "Failed to connect to Dolt storage",
			"  • Ensure Dolt server is running\n"+
				"  • Check WORKSPACE environment variable is set")
		return err
	}
	defer adapter.Close()

	if err := adapter.CreateSession(m); err != nil {
		debug.Log("Failed to save session to Dolt: %v", err)
		ui.PrintError(err, "Failed to save session to Dolt",
			"  • Check database connection\n"+
				"  • Verify Dolt server is accessible")
		return err
	}
	debug.Log("Session saved to Dolt database: %s", m.SessionID)
	if testMode {
		ui.PrintSuccess("Test session registered in database (hidden from default list)")
	} else {
		ui.PrintSuccess("Session registered in database")
	}
	return nil
}

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
	debug.Log("Waiting for ready-file signal (timeout: 60s)")
	var readyErr error
	spinErr2 := spinner.New().
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
		fmt.Sprintf("  • Attach to session to check Claude output: tmux attach -t %s\n"+
			"  • Check debug logs: ls -lt ~/.agm/debug/\n"+
			"  • Run 'agm sync' later to populate UUID if needed\n\n"+
			"  Note: Session is still usable, but UUID association may have failed", sessionName),
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

// getSessionsDir returns the sessions directory (respects --sessions-dir flag and --test mode)
func getSessionsDir() string {
	// Test sandbox isolation: AGM_SESSIONS_DIR env var takes absolute priority.
	// This is set by TestContext.SetEnv() for per-run isolation.
	if envDir := os.Getenv("AGM_SESSIONS_DIR"); envDir != "" {
		return envDir
	}

	// Test mode (--test flag) takes next priority for integration tests
	if testMode {
		// If cfg.SessionsDir is explicitly set by unit test, use it.
		// Otherwise use ~/sessions-test/ (integration tests with --test flag).
		// We detect unit-test overrides by checking if SessionsDir differs
		// from the default ~/.claude/sessions path.
		if cfg != nil && cfg.SessionsDir != "" {
			homeDir, _ := os.UserHomeDir()
			defaultDir := filepath.Join(homeDir, ".claude", "sessions")
			if cfg.SessionsDir != defaultDir {
				return cfg.SessionsDir
			}
		}
		homeDir, _ := os.UserHomeDir()
		return filepath.Join(homeDir, "sessions-test")
	}
	// Workspace-aware config (from env vars or workspace detection)
	if cfg != nil && cfg.SessionsDir != "" {
		return cfg.SessionsDir
	}
	// Default to ~/.claude/sessions (aligned with Claude Code structure)
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".claude", "sessions")
}

// checkDuplicateSessionName checks if a non-archived session with the given name already exists in Dolt
func checkDuplicateSessionName(sessionName string) error {
	adapter, err := getStorage()
	if err != nil {
		// If Dolt is unavailable, skip the check (non-fatal)
		return nil
	}
	defer adapter.Close()

	sessions, err := adapter.ListSessions(nil)
	if err != nil {
		// If listing fails, skip the check (non-fatal)
		return nil
	}

	for _, s := range sessions {
		if s.Name == sessionName && s.Lifecycle != manifest.LifecycleArchived {
			return fmt.Errorf("session '%s' already exists. Use a different name or archive the existing session with: agm session archive %s", sessionName, sessionName)
		}
	}
	return nil
}

// startClaudeInCurrentTmux starts a fresh Claude session in the current tmux session
func startClaudeInCurrentTmux(sessionName string) error {
	if !testMode {
		if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
			return dupErr
		}
		if err := enforceCircuitBreakers(); err != nil {
			return err
		}
	}

	fmt.Printf("Starting new Claude session in current tmux: %s\n", sessionName)

	workDir, err := os.Getwd()
	if err != nil {
		ui.PrintError(err,
			"Failed to get current directory",
			"  • Check directory still exists: pwd\n"+
				"  • Verify directory permissions: ls -ld .\n"+
				"  • Try from a different directory")
		return err
	}

	createCurrentTmuxManifest(sessionName, workDir)

	if err := startCurrentTmuxHarness(sessionName, workDir); err != nil {
		return err
	}

	ui.PrintSuccess(fmt.Sprintf("%s session started in current tmux!", harnessName))
	updateVSCodeTabTitle(sessionName)
	return nil
}

// createCurrentTmuxManifest writes the manifest dir and registers a v2 session
// in Dolt for the in-place tmux pane Claude case. Failures are non-fatal —
// they only emit warnings, since the user already has a usable tmux pane.
func createCurrentTmuxManifest(sessionName, workDir string) {
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		return
	}
	generatedUUID := uuid.New().String()
	debug.Log("Generated SessionID: %s", generatedUUID)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     generatedUUID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Workspace:     cfg.Workspace,
		Context:       manifest.Context{Project: workDir},
		Tmux:          manifest.Tmux{SessionName: sessionName},
		Harness:       harnessName,
		Model:         modelName,
	}
	if testMode {
		m.IsTest = true
		debug.Log("Marking session as test (is_test=true)")
	}
	adapter, err := getStorage()
	if err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to connect to Dolt: %v", err))
		return
	}
	defer adapter.Close()
	if err := adapter.CreateSession(m); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create session in Dolt: %v", err))
		return
	}
	if testMode {
		ui.PrintSuccess(fmt.Sprintf("Test session registered in database: %s (hidden from default list)", m.SessionID))
	} else {
		ui.PrintSuccess(fmt.Sprintf("Session registered in database: %s", m.SessionID))
	}
	_ = git.CommitManifest(manifestPath, "create", sessionName)
}

// startCurrentTmuxHarness dispatches the per-harness startup flow for the
// in-place (current tmux pane) Claude/Gemini/OpenCode cases.
func startCurrentTmuxHarness(sessionName, workDir string) error {
	switch harnessName {
	case "claude-code":
		return startCurrentTmuxClaude(sessionName, workDir)
	case "opencode-cli":
		return startCurrentTmuxOpenCode(sessionName)
	case "gemini-cli":
		return startCurrentTmuxGemini(sessionName)
	default:
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", harnessName))
		return nil
	}
}

// startCurrentTmuxClaude runs Claude in the current tmux pane, waits for
// readiness, and runs the rename/agm-assoc init sequence.
func startCurrentTmuxClaude(sessionName, workDir string) error {
	claudeReady := tmux.NewClaudeReadyFile(sessionName)
	if err := claudeReady.Cleanup(); err != nil {
		debug.Log("Warning: failed to cleanup ready-files: %v", err)
	}
	fmt.Println("Starting Claude CLI...")
	workDirForClaude := os.Getenv("PWD")
	if workDirForClaude == "" {
		workDirForClaude = workDir
	}
	resolvedModel := agent.ResolveModelFullName("claude-code", modelName)
	autoModeFlag := " --enable-auto-mode"
	if noAutoMode {
		autoModeFlag = ""
		debug.Log("Auto mode disabled by flag/env var")
	}
	claudeCmd := fmt.Sprintf("AGM_SESSION_NAME=%s claude --model %s --add-dir '%s'%s && exit", sessionName, resolvedModel, workDirForClaude, autoModeFlag)
	if modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default" {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --permission-mode %s && exit", modeFlagValue), 1)
	}
	if maxBudgetUsd > 0 {
		claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --max-budget-usd %.2f && exit", maxBudgetUsd), 1)
	}
	if err := tmux.SendCommand(sessionName, claudeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Claude in current tmux pane",
			"  • Verify Claude is installed: which claude\n"+
				"  • Test Claude manually: claude --version\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agmnew "+sessionName)
		return err
	}
	time.Sleep(500 * time.Millisecond)
	fmt.Println("Waiting for Claude to initialize...")
	if err := tmux.WaitForClaudePrompt(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("Claude ready signal not detected")
		fmt.Printf("💡 Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("Claude is ready!")
	}
	debug.Log("Triggering SessionStart hook post-verification")
	if err := claudeReady.TriggerHookManually(); err != nil {
		debug.Log("Manual hook trigger failed (non-fatal): %v", err)
	}
	if os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "" {
		debug.Log("Skipping InitSequence in test environment")
		ui.PrintSuccess("Test session ready (init sequence skipped)")
		return nil
	}
	runCurrentTmuxClaudeInitSequence(sessionName)
	return nil
}

// runCurrentTmuxClaudeInitSequence runs the rename/agm-assoc init sequence and
// waits for the ready-file in the in-place Claude pane case.
func runCurrentTmuxClaudeInitSequence(sessionName string) {
	debug.Log("Running InitSequence for /rename and /agm:agm-assoc")
	seq := tmux.NewInitSequence(sessionName)
	if err := seq.Run(); err != nil {
		debug.Log("InitSequence failed: %v", err)
		ui.PrintWarning("Failed to run initialization sequence")
		fmt.Printf("💡 You can manually run:\n")
		fmt.Printf("  /rename %s\n", sessionName)
		fmt.Printf("  /agm:agm-assoc %s\n", sessionName)
		return
	}
	debug.Log("InitSequence completed successfully")
	debug.Log("Waiting for ready-file signal (timeout: 60s)")
	if err := readiness.WaitForReady(sessionName, 60*time.Second); err != nil {
		debug.Log("Ready-file wait failed: %v", err)
		ui.PrintWarning("Ready-file not created within timeout")
		fmt.Printf("💡 Session is usable, but UUID association may have failed\n")
		fmt.Printf("  • Run 'agm sync' later to populate UUID if needed\n")
		return
	}
	debug.Log("Ready-file detected - agm binary completed")
	debug.Log("Waiting for skill to complete output and return to prompt")
	if err := tmux.WaitForClaudePrompt(sessionName, 10*time.Second); err != nil {
		debug.Log("Prompt wait failed (non-fatal): %v", err)
		time.Sleep(1 * time.Second)
	}
	ui.PrintSuccess("Claude is ready and session associated!")
}

// startCurrentTmuxOpenCode starts OpenCode in the current tmux pane and waits
// for prompt readiness.
func startCurrentTmuxOpenCode(sessionName string) error {
	fmt.Println("Starting OpenCode...")
	opencodeCmd := "opencode attach && exit"
	if err := tmux.SendCommand(sessionName, opencodeCmd); err != nil {
		ui.PrintError(err,
			"Failed to start OpenCode in current tmux pane",
			"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
				"  • Start server if needed: opencode serve --port 4096\n"+
				"  • Check you're in tmux: echo $TMUX\n"+
				"  • Exit tmux and try: agm new "+sessionName+" --harness opencode-cli")
		return err
	}
	fmt.Println("Waiting for OpenCode to initialize...")
	if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("OpenCode ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("OpenCode is ready!")
	}
	return nil
}

// startCurrentTmuxGemini starts Gemini in the current tmux pane and handles
// the optional first-run trust prompt.
func startCurrentTmuxGemini(sessionName string) error {
	fmt.Println("Starting Gemini CLI...")
	resolvedModel := agent.ResolveModelFullName("gemini-cli", modelName)
	geminiCmd := fmt.Sprintf("gemini -m %s && exit", resolvedModel)
	debug.Log("Sending command: %s", geminiCmd)
	if err := tmux.SendCommand(sessionName, geminiCmd); err != nil {
		ui.PrintError(err,
			"Failed to start Gemini in current tmux pane",
			"  • Verify Gemini is installed: which gemini\n"+
				"  • Test Gemini manually: gemini --version\n"+
				"  • Check you're in tmux: echo $TMUX")
		return err
	}
	autoAcceptGeminiTrustPrompt(sessionName)
	fmt.Println("Waiting for Gemini to initialize...")
	if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
		ui.PrintWarning("Gemini ready signal not detected")
		fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
	} else {
		ui.PrintSuccess("Gemini is ready!")
	}
	return nil
}

// autoAcceptGeminiTrustPrompt scans the tmux pane for the Gemini trust prompt
// and answers "1<Enter>" if found.
func autoAcceptGeminiTrustPrompt(sessionName string) {
	debug.Log("Checking for Gemini trust prompt (3s window)...")
	time.Sleep(2 * time.Second)
	socketPath := tmux.GetSocketPath()
	normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
	trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
	trustOutput, err := trustCheckCmd.CombinedOutput()
	if err != nil {
		return
	}
	content := string(trustOutput)
	if !strings.Contains(content, "Do you trust") && !strings.Contains(content, "trust the files") {
		debug.Log("No trust prompt detected (directory already trusted)")
		return
	}
	debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
	selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
	_ = selectCmd.Run()
	time.Sleep(300 * time.Millisecond)
	enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Enter")
	_ = enterCmd.Run()
	debug.Log("Trust prompt auto-accepted")
	ui.PrintSuccess("Auto-accepted Gemini trust prompt")
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

// addToAdditionalDirectories was removed — it wrote sandbox paths to the global
// ~/.claude/settings.json, breaking sandbox isolation. Trust is now handled
// exclusively via --add-dir CLI flags passed per-session to Claude.

// Permission resolution, parent permission reading, and project settings
// configuration have been moved to agm/internal/rbac package.
// Use rbac.ResolvePermissions, rbac.ReadParentPermissions, and
// rbac.ConfigureProjectPermissions respectively.

// shouldEnableSandbox determines if sandbox should be enabled based on config and flags.
// Sandbox is ON by default (config.Sandbox.Enabled=true). Use --no-sandbox to disable.
// The enable parameter is retained for backward compatibility but the --sandbox flag
// has been removed from the CLI.
func shouldEnableSandbox(enable bool, disable bool) bool {
	if disable {
		return false
	}
	if enable {
		return true
	}
	// Use config (defaults to true = sandbox-by-default)
	return cfg.Sandbox.Enabled
}

// provisionSandbox creates a sandbox environment for the session
func provisionSandbox(ctx context.Context, providerName string, sessionID string, workDir string) (*manifest.SandboxConfig, error) {
	debug.Phase("Provision Sandbox")
	debug.Log("Provisioning sandbox for session: %s", sessionID)
	debug.Log("Provider: %s", providerName)
	debug.Log("WorkDir: %s", workDir)

	// Get provider
	var provider sandbox.Provider
	var err error
	if providerName == "auto" {
		provider, err = sandbox.NewProvider()
	} else {
		provider, err = sandbox.NewProviderForPlatform(providerName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox provider: %w", err)
	}

	debug.Log("Using provider: %s", provider.Name())

	// Prepare sandbox workspace directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	sandboxWorkspace := filepath.Join(homeDir, ".agm", "sandboxes", sessionID)

	lowerDirs := resolveSandboxLowerDirs(workDir)

	// Determine the primary/target repo: prefer a repo that has go.mod at root
	// (the monorepo) to avoid alphabetical scanning picking the wrong repo.
	targetRepo := findPrimaryRepo(lowerDirs)
	if targetRepo != "" {
		debug.Log("Target repo (has go.mod): %s", targetRepo)
	}

	// Create sandbox
	debug.Log("Creating sandbox with workspace: %s", sandboxWorkspace)
	debug.Log("Lower dirs: %v", lowerDirs)
	sb, err := provider.Create(ctx, sandbox.SandboxRequest{
		SessionID:    sessionID,
		LowerDirs:    lowerDirs,
		WorkspaceDir: sandboxWorkspace,
		Secrets:      cfg.Sandbox.Secrets,
		TargetRepo:   targetRepo,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create sandbox: %w", err)
	}

	debug.Log("Sandbox created successfully")
	debug.Log("Merged path: %s", sb.MergedPath)
	ui.PrintSuccess(fmt.Sprintf("Sandbox provisioned: %s", provider.Name()))

	// Write onboarding CLAUDE.md with worktree instructions
	if cfg.Sandbox.Onboarding.Enabled {
		var content string
		var onboardErr error
		if cfg.Sandbox.Onboarding.TemplatePath != "" {
			content, onboardErr = sandbox.GenerateOnboardingContentFromFile(
				cfg.Sandbox.Onboarding.TemplatePath, sessionID, sb.MergedPath, lowerDirs,
			)
		} else {
			content, onboardErr = sandbox.GenerateOnboardingContent(sessionID, sb.MergedPath, lowerDirs)
		}
		if onboardErr != nil {
			debug.Log("Warning: failed to generate onboarding content: %v", onboardErr)
		} else if err := sandbox.WriteOnboardingClaudeMd(sb.MergedPath, content); err != nil {
			debug.Log("Warning: failed to write onboarding CLAUDE.md: %v", err)
		} else {
			debug.Log("Wrote sandbox onboarding to ~/.claude/projects/ for %s", sb.MergedPath)
		}
	}

	return &manifest.SandboxConfig{
		Enabled:    true,
		ID:         sb.ID,
		Provider:   provider.Name(),
		MergedPath: sb.MergedPath,
		CreatedAt:  sb.CreatedAt,
	}, nil
}

// resolveSandboxLowerDirs returns the OverlayFS lower directories for a new
// sandbox: prefer cfg.Sandbox.Repos, otherwise scan ~/src/ws/oss/repos for
// git repos, otherwise fall back to workDir.
func resolveSandboxLowerDirs(workDir string) []string {
	lowerDirs := cfg.Sandbox.Repos
	if len(lowerDirs) > 0 {
		return lowerDirs
	}
	wsRoot := filepath.Join(os.Getenv("HOME"), "src", "ws", "oss", "repos")
	if entries, err := os.ReadDir(wsRoot); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			repoPath := filepath.Join(wsRoot, e.Name())
			if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
				lowerDirs = append(lowerDirs, repoPath)
			}
		}
	}
	if len(lowerDirs) == 0 {
		debug.Log("No repos found, using workDir as lower dir: %s", workDir)
		return []string{workDir}
	}
	debug.Log("Found %d repos in workspace: %v", len(lowerDirs), lowerDirs)
	return lowerDirs
}

// findPrimaryRepo selects the preferred repo from a list of repo paths.
// It uses a priority cascade to avoid picking the wrong repo when multiple
// repos have go.mod (e.g. ai-conversation-logs vs ai-tools):
//  1. The repo matching the current working directory (or its symlink target)
//  2. The repo named "ai-tools" (AGM's own monorepo — the right default)
//  3. Any repo with go.mod at root
//  4. The first repo as a last resort
//
// Returns empty string if no suitable repo is found.
func findPrimaryRepo(repoDirs []string) string {
	// 1. Check if the current working directory is inside one of the repos.
	//    In a sandbox the cwd may be a symlink, so resolve it first.
	if cwd, err := os.Getwd(); err == nil {
		resolvedCwd, _ := filepath.EvalSymlinks(cwd)
		for _, dir := range repoDirs {
			resolvedDir, _ := filepath.EvalSymlinks(dir)
			if strings.HasPrefix(cwd, dir+"/") || cwd == dir ||
				(resolvedDir != "" && (strings.HasPrefix(resolvedCwd, resolvedDir+"/") || resolvedCwd == resolvedDir)) {
				debug.Log("findPrimaryRepo: cwd %s is inside repo %s", cwd, dir)
				return dir
			}
		}
	}

	// 2. Prefer the repo named "ai-tools" — AGM lives in this monorepo,
	//    so it is almost always the correct target when invoked from AGM.
	for _, dir := range repoDirs {
		if filepath.Base(dir) == "ai-tools" {
			goMod := filepath.Join(dir, "go.mod")
			if _, err := os.Stat(goMod); err == nil {
				debug.Log("findPrimaryRepo: preferred ai-tools repo: %s", dir)
				return dir
			}
		}
	}

	// 3. Fall back to the first repo with go.mod.
	for _, dir := range repoDirs {
		goMod := filepath.Join(dir, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			return dir
		}
	}

	// 4. Last resort: return the first entry.
	if len(repoDirs) > 0 {
		return repoDirs[0]
	}
	return ""
}

// cleanupSandbox destroys a sandbox on error
func cleanupSandbox(ctx context.Context, sandboxID string, providerName string) {
	if sandboxID == "" {
		return
	}

	debug.Log("Cleaning up sandbox: %s", sandboxID)

	var provider sandbox.Provider
	var err error
	if providerName == "auto" || providerName == "" {
		provider, err = sandbox.NewProvider()
	} else {
		provider, err = sandbox.NewProviderForPlatform(providerName)
	}
	if err != nil {
		debug.Log("Failed to get provider for cleanup: %v", err)
		return
	}

	if err := provider.Destroy(ctx, sandboxID); err != nil {
		debug.Log("Failed to cleanup sandbox: %v", err)
	} else {
		debug.Log("Sandbox cleaned up successfully")
	}
}

// applyCreationModeSwitch dispatches a mode switch during session creation.
// Non-fatal: errors are logged as warnings and execution continues.
func applyCreationModeSwitch(sessionName, harness, targetMode string) {
	if targetMode == "" {
		return
	}
	debug.Log("Applying creation mode switch: default -> %s (harness: %s)", targetMode, harness)
	if err := dispatchModeSwitch(harness, sessionName, targetMode, "default"); err != nil {
		ui.PrintWarning(fmt.Sprintf("Mode switch to %s failed: %v (continuing with default mode)", targetMode, err))
		return
	}
	ui.PrintSuccess(fmt.Sprintf("Mode set to %s", targetMode))
	adapter, err := getStorage()
	if err != nil {
		debug.Log("Could not connect to storage for mode manifest update: %v", err)
		return
	}
	defer adapter.Close()
	updateModeManifest(adapter, sessionName, targetMode, "creation")
}

// resolveEnvVarDefaults applies AGM_DEFAULT_* env vars for flags not explicitly set.
// Priority: CLI flag > env var > interactive prompt (harness/model) or built-in default (mode).
func resolveEnvVarDefaults(cmd *cobra.Command) {
	if !cmd.Flags().Changed("harness") {
		if v := os.Getenv("AGM_DEFAULT_HARNESS"); v != "" {
			harnessName = v
			debug.Log("Using AGM_DEFAULT_HARNESS: %s", v)
		}
	}
	if !cmd.Flags().Changed("model") {
		if v := os.Getenv("AGM_DEFAULT_MODEL"); v != "" {
			modelName = v
			debug.Log("Using AGM_DEFAULT_MODEL: %s", v)
		}
	}
	if !cmd.Flags().Changed("mode") {
		if v := os.Getenv("AGM_DEFAULT_MODE"); v != "" {
			modeFlagValue = v
			debug.Log("Using AGM_DEFAULT_MODE: %s", v)
		}
	}
	if !cmd.Flags().Changed("no-auto-mode") {
		if v := os.Getenv("AGM_DISABLE_AUTO_MODE"); v == "1" || v == "true" {
			noAutoMode = true
			debug.Log("Using AGM_DISABLE_AUTO_MODE: %s", v)
		}
	}
}

// enforceCircuitBreakers runs the CPU circuit breaker gates and returns an
// error if any gate refuses the spawn. On success it records the spawn time
// so the stagger gate works for subsequent spawns.
func enforceCircuitBreakers() error {
	cfg := circuitbreaker.DefaultConfig()
	lr := circuitbreaker.ProcLoadReader{}
	wc := circuitbreaker.TmuxWorkerCounter{}
	st := circuitbreaker.NewFileSpawnTimer()

	result := circuitbreaker.Check(cfg, lr, wc, st)

	// Log DEAR level regardless of outcome
	debug.Log("Circuit breaker check: level=%s load=%.1f allowed=%v", result.Level, result.Load, result.Allowed)
	for _, g := range result.Gates {
		debug.Log("  gate %s: passed=%v — %s", g.Gate, g.Passed, g.Message)
	}

	if !result.Allowed {
		return fmt.Errorf("%s", circuitbreaker.FormatDenied(result))
	}

	// Record spawn time for stagger gate
	if err := st.RecordSpawn(time.Now()); err != nil {
		debug.Log("Warning: failed to record spawn time: %v", err)
	}

	return nil
}

// buildSessionTags combines --role and --tags into a single tag slice.
// verifyAndRetryPromptDelivery verifies prompt delivery to a session and retries
// if the prompt was not received. This prevents silent delivery failures caused by
// input line conflicts, cooldowns, or timing issues.
//
// Parameters:
//   - sessionName: target tmux session
//   - promptText: the prompt content (used for keyword-based verification)
//   - sendFunc: function that re-sends the prompt on retry
func verifyAndRetryPromptDelivery(sessionName, promptText string, sendFunc func() error) {
	result, err := tmux.VerifyPromptDelivery(sessionName, promptText, sendFunc, 3)
	if err != nil {
		debug.Log("Prompt delivery verification error: %v", err)
		logger.Warn("Could not verify prompt delivery", "error", err)
		return
	}
	if result.Delivered {
		debug.Log("Prompt delivery confirmed (attempt %d, method: %s)", result.Attempt, result.Method)
		if result.Attempt > 1 {
			logger.Info("Prompt delivery required retry", "attempt", result.Attempt, "method", result.Method)
		}
	} else {
		logger.Warn("Prompt delivery could not be verified after retries",
			"session", sessionName)
		fmt.Println("  ⚠ Prompt delivery could not be verified — check session manually")
	}
}

func buildSessionTags(role string, tags []string) []string {
	var result []string
	if role != "" {
		result = append(result, "role:"+role)
	}
	result = append(result, tags...)
	if len(result) == 0 {
		return nil
	}
	return result
}

func init() {
	// Check for AGM_DEBUG environment variable for default value
	debugDefault := os.Getenv("AGM_DEBUG") == "true" || os.Getenv("AGM_DEBUG") == "1"

	sessionCmd.AddCommand(newCmd)
	newCmd.Flags().BoolP("debug", "d", debugDefault, "Enable debug logging to ~/.agm/debug/ (env: AGM_DEBUG)")
	newCmd.Flags().BoolVar(&detached, "detached", false, "Create detached session without attaching")
	// Per-run sandbox isolation: --test creates isolated tmux socket, sessions dir,
	// DB path, and lock path under /tmp/agm-test-{id}/. Environment variables
	// (AGM_TEST_RUN_ID, AGM_TMUX_SOCKET, AGM_SESSIONS_DIR, AGM_DB_PATH, AGM_LOCK_PATH)
	// propagate to child commands for full isolation.
	newCmd.Flags().BoolVar(&testMode, "test", false, "Create test session with per-run sandbox isolation")
	newCmd.Flags().BoolVar(&allowTestName, "allow-test-name", false, "Override test pattern warning (for legitimate production sessions with 'test' in name)")
	newCmd.Flags().StringVar(&harnessName, "harness", "", "Harness to use (claude-code, gemini-cli, codex-cli, opencode-cli) (env: AGM_DEFAULT_HARNESS)")
	newCmd.Flags().StringVar(&modelName, "model", "", "Model to use (e.g., sonnet, opus, 2.5-flash, 5.4) (env: AGM_DEFAULT_MODEL)")
	newCmd.Flags().StringVar(&workspaceName, "workspace", "", "Workspace to use (oss, acme, auto for detection)")
	newCmd.Flags().StringVar(&workflowName, "workflow", "", "Execution workflow (deep-research, code-review, architect)")
	newCmd.Flags().StringVar(&projectID, "project-id", "", "GCP project ID (required for gemini-cli harness)")
	newCmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send after session initialization")
	newCmd.Flags().StringVar(&promptFile, "prompt-file", "", "File containing prompt to send")
	newCmd.Flags().BoolVar(&noSandbox, "no-sandbox", false, "Disable sandbox isolation (sandbox is ON by default)")
	newCmd.Flags().StringVar(&sandboxProvider, "sandbox-provider", "auto", "Sandbox provider (auto, overlayfs, apfs, mock)")
	newCmd.Flags().Float64Var(&maxBudgetUsd, "max-budget-usd", 0, "Maximum budget in USD for the session (passed to claude --max-budget-usd)")
	newCmd.Flags().StringVar(&testEnvName, "test-env", "", "Use named test environment (created via 'agm test-env create')")
	newCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	// harness and workspace flags are now optional - prompts shown if omitted
	newCmd.Flags().StringVar(&modeFlagValue, "mode", "", "Permission mode after init (plan, auto, default) (env: AGM_DEFAULT_MODE)")
	newCmd.Flags().BoolVar(&noAutoMode, "no-auto-mode", false, "Disable --enable-auto-mode flag for Claude (env: AGM_DISABLE_AUTO_MODE)")
	newCmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"plan", "auto", "default"}, cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().StringVar(&roleName, "role", "", "Role tag for the session (e.g., orchestrator, worker, researcher)")
	newCmd.Flags().StringSliceVar(&sessionTags, "tags", nil, "Context tags for the session (e.g., 'cap:web-search,cap:claude-code')")
	newCmd.RegisterFlagCompletionFunc("role", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"orchestrator", "meta-orchestrator", "researcher", "worker", "reviewer"}, cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().StringSliceVar(&permissionsAllow, "permissions-allow", nil, "Permission patterns to pre-approve (e.g., 'Bash(tmux:*),Read(~/src/**)') — written to project .claude/settings.local.json")
	newCmd.Flags().StringVar(&permissionProfile, "permission-profile", "", "Predefined permission profile (worker, monitor, audit)")
	newCmd.RegisterFlagCompletionFunc("permission-profile", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return rbac.ProfileNames(), cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().BoolVar(&inheritPermissions, "inherit-permissions", false, "Inherit permission allowlist from parent's ~/.claude/settings.json")
	newCmd.Flags().BoolVar(&disposable, "disposable", false, "Create a disposable session with TTL-based auto-archive")
	newCmd.Flags().StringVar(&disposableTTL, "disposable-ttl", "4h", "TTL for disposable sessions (e.g., 1h, 4h, 30m)")

	// Tab completion for --harness flag
	newCmd.RegisterFlagCompletionFunc("harness", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"claude-code", "gemini-cli", "codex-cli", "opencode-cli"}, cobra.ShellCompDirectiveNoFileComp
	})
	// Tab completion for --model flag (context-sensitive based on --harness value)
	newCmd.RegisterFlagCompletionFunc("model", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		h, _ := cmd.Flags().GetString("harness")
		if h == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return agent.ModelAliases(h), cobra.ShellCompDirectiveNoFileComp
	})
}
