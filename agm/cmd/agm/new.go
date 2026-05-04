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
	// Test environment isolation: create per-run isolated context when
	// --test is set (ephemeral) or AGM_TEST_ENV is already set (named/CI).
	if _, hasSandbox := testcontext.FromEnv(); hasSandbox {
		debug.Log("Test environment active (inherited from environment)")
	} else if testMode {
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
		defer tc.Cleanup()
	} else if os.Getenv("AGM_TEST_SANDBOX") == "1" {
		// Legacy: support AGM_TEST_SANDBOX=1 for backward compatibility
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

	// BUG-002: Check for duplicate session name before creating
	if !testMode {
		if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
			return dupErr
		}
	}

	// CPU circuit breakers: refuse spawn if system is overloaded.
	// Skipped in test mode to avoid blocking test harnesses.
	if !testMode {
		if err := enforceCircuitBreakers(); err != nil {
			return err
		}
	}

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

	// Provision sandbox if enabled
	var sandboxInfo *manifest.SandboxConfig
	ctx := context.Background()
	sessionID := uuid.New().String() // Generate session ID early for sandbox

	// Cleanup sandbox on error
	defer func() {
		if retErr != nil && sandboxInfo != nil {
			cleanupSandbox(ctx, sandboxInfo.ID, sandboxInfo.Provider)
		}
	}()

	if shouldEnableSandbox(enableSandbox, noSandbox) {
		var err error
		sandboxInfo, err = provisionSandbox(ctx, sandboxProvider, sessionID, workDir)
		if err != nil {
			ui.PrintError(err,
				"Failed to provision sandbox",
				"  • Check sandbox provider is available\n"+
					"  • Use --no-sandbox to disable sandbox isolation\n"+
					"  • Check ~/.agm/sandboxes/ directory permissions")
			return err
		}
		// Update workDir to use sandbox merged path
		workDir = sandboxInfo.MergedPath
		fmt.Printf("Using sandbox workspace: %s\n", workDir)
	}

	// Collect extra --add-dir flags for source repo directories (sandbox lowerDirs).
	// Without this, Claude prompts for permission when following git references or
	// worktree paths back to the original repos (e.g. engram-research/).
	// NOTE: We pass these via --add-dir CLI flags (per-session) instead of writing to
	// the global ~/.claude/settings.json additionalDirectories, which would break
	// sandbox isolation by making all sessions aware of all sandbox paths.
	debug.Phase("Configure Trust")
	var extraAddDirs []string
	if sandboxInfo != nil {
		for _, repoDir := range cfg.Sandbox.Repos {
			extraAddDirs = append(extraAddDirs, repoDir)
			debug.Log("Will pre-authorize source repo via --add-dir: %s", repoDir)
		}
	}
	// Trust is always pre-configured via --add-dir on the Claude command line
	trustPreConfigured := true

	// Configure project-level permissions. Default safe permissions are always
	// applied to eliminate the "permission tax" (repeated prompts for read-only
	// git commands, agm, etc.). Additional permissions come from --permissions-allow,
	// --permission-profile, or --inherit-permissions.
	{
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
					"  • Choose different name: agmnew <different-name>\n"+
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
						"  • Provide name as argument: agmnew <session-name>\n"+
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
	}

	// Clear any stale interrupt flags for this session name (from previous runs)
	if err := interrupt.Clear(interrupt.DefaultDir(), sessionName); err != nil {
		debug.Log("Warning: failed to clear stale interrupt flag: %v", err)
	}

	// Start harness-specific initialization
	var spinErr error
	modeAppliedAtStartup := false
	switch harnessName {
	case "claude-code":
		// Cleanup Claude ready-files from previous sessions (if any)
		claudeReady := tmux.NewClaudeReadyFile(sessionName)
		if err := claudeReady.Cleanup(); err != nil {
			debug.Log("Warning: failed to cleanup ready-files: %v", err)
		}

		// Start Claude in the session
		// Use --add-dir to pre-approve workspace and avoid trust prompt blocking the ">" prompt
		// Pass AGM_SESSION_NAME env var so SessionStart hook can create ready-file signal
		// Unset CLAUDECODE to avoid nested-session guard when agm is run from within Claude Code
		debug.Phase("Start Claude")
		resolvedModel := agent.ResolveModelFullName("claude-code", modelName)
		autoModeFlag := " --enable-auto-mode"
		if noAutoMode {
			autoModeFlag = ""
			debug.Log("Auto mode disabled by flag/env var")
		}
		claudeCmd := fmt.Sprintf("env -u CLAUDECODE AGM_SESSION_NAME=%s claude --model %s --add-dir '%s'%s && exit", sessionName, resolvedModel, workDir, autoModeFlag)
		// Append extra --add-dir flags for sandbox source repos (per-session, not global)
		for _, dir := range extraAddDirs {
			claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --add-dir '%s' && exit", dir), 1)
		}
		if modeFlagValue != "" && (modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default") {
			claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --permission-mode %s && exit", modeFlagValue), 1)
			modeAppliedAtStartup = true
		}
		if maxBudgetUsd > 0 {
			claudeCmd = strings.Replace(claudeCmd, " && exit", fmt.Sprintf(" --max-budget-usd %.2f && exit", maxBudgetUsd), 1)
		}
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

		// Use text-parsing to wait for Claude prompt (reliable for tmux-started sessions)
		// Manual hook triggers create false positives since the hook runs immediately
		// but Claude hasn't actually started in tmux yet
		debug.Phase("Wait for Claude Ready Signal (Text-Parsing)")
		debug.Log("Waiting for Claude prompt to appear in tmux (timeout: 90s)")
		var waitErr error
		spinErr = spinner.New().
			Title("Waiting for Claude to be ready...").
			Accessible(true).
			Action(func() {
				// 90s timeout: long enough for slow SessionStart hooks (engram
				// reindex, token-tracker) and post-trust-prompt re-render.
				waitErr = tmux.WaitForClaudePrompt(sessionName, 90*time.Second)
			}).
			Run()
		if spinErr != nil {
			return fmt.Errorf("spinner error: %w", spinErr)
		}

		// Ensure clean line after spinner
		fmt.Println()

		if waitErr != nil {
			// Text-parsing failed - this is a BLOCKING error
			debug.Log("Claude prompt detection failed: %v", waitErr)
			ui.PrintError(waitErr,
				"Failed to detect Claude ready signal",
				"  Claude prompt not detected in tmux session.\n"+
					"  \n"+
					"  Troubleshooting:\n"+
					"    1. Check if Claude started: tmux attach -t "+sessionName+"\n"+
					"    2. Verify Claude is installed: which claude\n"+
					"    3. Check for errors in tmux: tmux capture-pane -t "+sessionName+" -p\n")

			// Clean up the session since we can't proceed
			socketPath := tmux.GetSocketPath()
			killCmd := exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName)
			if err := killCmd.Run(); err != nil {
				debug.Log("Failed to clean up session: %v", err)
			}

			return fmt.Errorf("claude not ready: %w", waitErr)
		}

		debug.Log("✓ Claude prompt detected - Claude is ready")

		// Now manually trigger the hook to create the ready-file for consistency
		// This allows the hook to run even though Claude started non-interactively
		debug.Log("Triggering SessionStart hook post-verification")
		if err := claudeReady.TriggerHookManually(); err != nil {
			debug.Log("Manual hook trigger failed (non-fatal): %v", err)
		}

		// Claude is ready!
		debug.Log("Claude ready signal received")
		ui.PrintSuccess("Claude is ready!")

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

		// NOTE: We no longer need to wait for SessionStart hooks explicitly.
		// The Claude ready-file signal (above) confirms that SessionStart hooks have completed,
		// since the agm-ready-signal hook creates the file after initialization.
		// This eliminates the previous 2-second fixed delay.
		debug.Phase("Skip Explicit SessionStart Hook Wait")
		debug.Log("SessionStart hooks confirmed complete (ready-file signal received)")

	case "gemini-cli":
		// Check for agm-agent-wrapper
		debug.Phase("Start Gemini")
		wrapperPath, err := exec.LookPath("agm-agent-wrapper")
		if err != nil {
			// Graceful fallback to direct gemini (wrapper not found)
			debug.Log("agm-agent-wrapper not found, falling back to direct gemini: %v", err)
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

			// Auto-trust: Gemini CLI asks "Do you trust the files in this folder?"
			// on first launch in a new sandbox/directory. Auto-select option 1.
			debug.Log("Checking for Gemini trust prompt (3s window)...")
			time.Sleep(2 * time.Second) // Wait for Gemini to start and show trust prompt
			socketPath := tmux.GetSocketPath()
			normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
			trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
			if trustOutput, err := trustCheckCmd.CombinedOutput(); err == nil {
				content := string(trustOutput)
				if strings.Contains(content, "Do you trust") || strings.Contains(content, "trust the files") {
					debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
					// Send "1" to select "Yes" option, then Enter to confirm
					selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
					_ = selectCmd.Run()
					time.Sleep(300 * time.Millisecond)
					enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Enter")
					_ = enterCmd.Run()
					debug.Log("Trust prompt auto-accepted")
					ui.PrintSuccess("Auto-accepted Gemini trust prompt")
				} else {
					debug.Log("No trust prompt detected (directory already trusted)")
				}
			}
		} else {
			// Use wrapper for readiness detection
			debug.Log("Found agm-agent-wrapper at: %s", wrapperPath)
			debug.Log("Executing wrapper directly (not via tmux): %s --agent=gemini-cli %s", wrapperPath, sessionName)

			// Execute wrapper directly (it will attach to the session)
			// The wrapper handles:
			// 1. Starting Gemini in the tmux session
			// 2. Waiting for readiness
			// 3. Creating ready-file
			// 4. Attaching to session
			// 5. Capturing output on exit
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
				fmt.Println("  • Try direct mode: agm session new --harness gemini-cli " + sessionName + " (after renaming wrapper)")
				return fmt.Errorf("gemini readiness timeout: %w", readyErr)
			}
			debug.Log("Ready signal received successfully")
			ui.PrintSuccess("Gemini initialized successfully")
		}

	case "codex-cli":
		// Codex supports both API key and OAuth login
		debug.Phase("Start Codex")

		// Check for API key or OAuth credentials
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
		// Session creation continues via adapter.CreateSession() automatically

	case "opencode-cli":
		// OpenCode uses client-server architecture (server must be running)
		debug.Phase("Start OpenCode")

		// Validate OpenCode server is running (health check already done by ValidateHarnessAvailability)
		debug.Log("OpenCode server validated (health check passed)")

		// Start OpenCode attach in the session
		// Unlike Claude (which needs --add-dir), OpenCode attach is simpler
		opencodeCmd := "opencode attach && exit"
		debug.Log("Sending command: %s", opencodeCmd)
		if err := tmux.SendCommand(sessionName, opencodeCmd); err != nil {
			ui.PrintError(err,
				"Failed to start OpenCode in tmux session",
				"  • Verify OpenCode server is running: curl http://localhost:4096/health\n"+
					"  • Start server if needed: opencode serve --port 4096\n"+
					"  • Check tmux session exists: tmux list-sessions\n"+
					"  • Attach and start manually: tmux attach -t "+sessionName)
			// Try to kill the tmux session if we just created it and OpenCode failed
			if !exists {
				_ = tmux.SendCommand(sessionName, "tmux kill-session -t "+sessionName)
			}
			return err
		}
		debug.Log("OpenCode attach command sent successfully")
		ui.PrintSuccess("Started OpenCode in tmux session")

		// Note: OpenCode state detection happens via SSE monitoring (already configured)
		// No need to wait for ready-files like Claude - SSE adapter handles state tracking
		debug.Log("OpenCode session ready (SSE monitoring active)")
		ui.PrintSuccess("OpenCode is ready! (state tracked via SSE)")

	default:
		// Other harnesses - no CLI startup configured yet
		debug.Phase("Skip CLI Startup")
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", sessionName))
	}

	// Create manifest BEFORE sending /rename (so hook can find it)
	debug.Phase("Create Manifest")
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		ui.PrintWarning("Proceeding without manifest - you can run 'agm sync' later")
	} else {
		// Create v2 manifest with proper SessionID and empty Claude UUID
		// The /csm-assoc command will populate the Claude UUID when it runs
		debug.Log("Using SessionID: %s", sessionID)
		m := &manifest.Manifest{
			SchemaVersion: manifest.SchemaVersion,
			SessionID:     sessionID, // Use sessionID generated earlier (for sandbox consistency)
			Name:          sessionName,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Lifecycle:     "",            // Empty = active/stopped
			Workspace:     cfg.Workspace, // Populate from workspace detection
			Context: manifest.Context{
				Project: workDir,
				Purpose: "", // Can be set later
				Tags:    buildSessionTags(roleName, sessionTags),
				Notes:   "",
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Harness: harnessName,
			Model:   modelName,
			Claude: manifest.Claude{
				UUID: "", // Will be populated by SessionStart hook
			},
			Sandbox:    sandboxInfo, // Add sandbox metadata if enabled
			Disposable: disposable,
			DisposableTTL: func() string {
				if disposable {
					return disposableTTL
				}
				return ""
			}(),
		}

		// Populate harness-specific metadata
		if harnessName == "opencode-cli" {
			m.OpenCode = &manifest.OpenCode{
				ServerPort: 4096,        // Default port
				ServerHost: "localhost", // Default host
				AttachTime: time.Now(),  // Time of session creation
			}
			// Override with environment variable if set
			if envURL := os.Getenv("OPENCODE_SERVER_URL"); envURL != "" {
				// Parse URL to extract host and port (simplified)
				m.OpenCode.ServerHost = envURL
			}
		}

		// Write to Dolt database (primary backend)
		// BUG-006: Register ALL sessions in Dolt (including test sessions)
		// Test sessions are marked with is_test=true and filtered from list by default
		if testMode {
			m.IsTest = true
			debug.Log("Marking session as test (is_test=true)")
		}

		// This ensures new sessions appear in 'agm session list' immediately
		debug.Phase("Register in Dolt Database")
		adapter, err := getStorage()
		if err != nil {
			// In test sandbox mode (AGM_TEST_RUN_ID set), Dolt is intentionally unavailable.
			// The sandbox uses isolated paths and doesn't need Dolt.
			if os.Getenv("AGM_TEST_RUN_ID") != "" {
				debug.Log("Test sandbox: Dolt unavailable (expected): %v", err)
				ui.PrintSuccess("Test sandbox session created (Dolt skipped)")
			} else {
				debug.Log("Failed to connect to Dolt: %v", err)
				ui.PrintError(err, "Failed to connect to Dolt storage",
					"  • Ensure Dolt server is running\n"+
						"  • Check WORKSPACE environment variable is set")
				return err
			}
		} else {
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
		}

		// Auto-commit manifest change if in git repo
		_ = git.CommitManifest(manifestPath, "create", sessionName) // Errors logged internally
	}

	// Claude-specific: Run initialization sequence
	// Skip init sequence (rename + agm-assoc) in test environments because:
	// 1. HOME is overridden, so Claude can't find ~/.claude/skills/agm/
	// 2. Test sessions don't need UUID association
	switch {
	case harnessName == "claude-code" && os.Getenv("AGM_TEST_RUN_ID") == "" && os.Getenv("AGM_TEST_ENV") == "":
		// NOTE: No need to release global lock - using fine-grained tmux lock instead
		// InitSequence will acquire/release tmux lock only during actual operations

		// Use InitSequence to properly sequence /rename and /agm:agm-assoc commands
		// This uses tmux control mode to wait for each command to complete before sending the next
		debug.Phase("Sequenced Initialization")
		debug.Log("Running InitSequence for /rename and /agm:agm-assoc")
		seq := tmux.NewInitSequence(sessionName)
		// Skip redundant WaitForClaudePrompt calls since we already verified at line 834
		// This prevents 30s timeout when prompt scrolls off the 50-line capture buffer
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

		// Wait for ready-file (created by agmassociate when UUID is captured)
		debug.Phase("Wait for Ready Signal")
		debug.Log("Waiting for ready-file signal (timeout: 60s)")
		var readyErr error
		var spinErr2 = spinner.New().
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

			// Get state directory for error message
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
		} else {
			debug.Log("Ready-file detected - agm binary completed")

			// Wait for /agm:agm-assoc skill to finish outputting its completion messages
			// The ready-file signals when 'agm associate' binary completes, but the skill
			// continues to output messages after that.
			//
			// Use smart layered detection (Bug fix: prompt interruption):
			// 1. Try pattern detection first (fast if marker present)
			// 2. Fallback to idle detection (detects when output stops)
			// 3. Final fallback to prompt detection
			debug.Log("Waiting for skill to complete output using smart detection")

			// Try pattern detection first (5 second timeout)
			// Look for explicit completion marker added to associate.go
			if err := tmux.WaitForPattern(sessionName, "[AGM_SKILL_COMPLETE]", 5*time.Second); err == nil {
				debug.Log("✓ Skill completion marker detected")
			} else {
				debug.Log("Pattern detection timeout (non-fatal): %v", err)

				// Fallback to idle detection (1 second idle, 15 second total timeout)
				// This detects when skill has stopped producing output
				if err := tmux.WaitForOutputIdle(sessionName, 1*time.Second, 15*time.Second); err == nil {
					debug.Log("✓ Output idle detected - skill appears complete")
				} else {
					debug.Log("Idle detection failed: %v", err)

					// Final fallback to prompt detection (5 second timeout)
					if err := tmux.WaitForClaudePrompt(sessionName, 5*time.Second); err != nil {
						debug.Log("Prompt detection failed (non-fatal): %v", err)
						// Last resort: brief delay to allow completion
						time.Sleep(1 * time.Second)
					}
				}
			}

			// Apply --mode flag if specified (before prompt, after assoc)
			// Skip if mode was already applied via --permission-mode at startup
			if modeFlagValue != "" && !modeAppliedAtStartup {
				applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
			}

			ui.PrintSuccess("Claude is ready and session associated!")

			// Send prompt if provided via --prompt or --prompt-file flags
			// Bug fix (2026-03-14): Pass shouldInterrupt=false to avoid interrupting session init
			if prompt != "" {
				debug.Log("Sending prompt from --prompt flag")
				if err := tmux.SendMultiLinePromptSafe(sessionName, prompt, false); err != nil {
					logger.Warn("Failed to send prompt", "error", err)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else {
					verifyAndRetryPromptDelivery(sessionName, prompt, func() error {
						return tmux.SendMultiLinePromptSafe(sessionName, prompt, false)
					})
				}
			} else if promptFile != "" {
				debug.Log("Sending prompt from --prompt-file flag: %s", promptFile)
				promptContent, readErr := os.ReadFile(promptFile)
				// Bug fix (2026-03-14): Pass shouldInterrupt=false to avoid interrupting
				if err := tmux.SendPromptFileSafe(sessionName, promptFile, false); err != nil {
					logger.Warn("Failed to send prompt from file", "error", err, "file", promptFile)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else if readErr == nil {
					verifyAndRetryPromptDelivery(sessionName, string(promptContent), func() error {
						return tmux.SendPromptFileSafe(sessionName, promptFile, false)
					})
				}
			}
		}
	case harnessName == "claude-code":
		// Test environment: skip init sequence (rename/assoc) because HOME override
		// prevents Claude from finding ~/.claude/skills/agm/ and test sessions
		// don't need UUID association.
		debug.Phase("Skip Init Sequence (Test Environment)")
		debug.Log("Skipping InitSequence: AGM_TEST_RUN_ID=%s AGM_TEST_ENV=%s",
			os.Getenv("AGM_TEST_RUN_ID"), os.Getenv("AGM_TEST_ENV"))
		ui.PrintSuccess("Test session ready (init sequence skipped)")
	case harnessName == "gemini-cli":
		// Gemini CLI: no rename/assoc init sequence, but wait for prompt readiness
		// in non-test mode so that --prompt flag delivery works reliably.
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
			// Deliver --prompt / --prompt-file if provided
			if prompt != "" {
				debug.Log("Sending prompt to Gemini session from --prompt flag")
				if err := tmux.SendPromptLiteral(sessionName, prompt, false); err != nil {
					logger.Warn("Failed to send prompt to Gemini", "error", err)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else {
					verifyAndRetryPromptDelivery(sessionName, prompt, func() error {
						return tmux.SendPromptLiteral(sessionName, prompt, false)
					})
				}
			} else if promptFile != "" {
				debug.Log("Sending prompt from file to Gemini session: %s", promptFile)
				promptContent, readErr := os.ReadFile(promptFile)
				if err := tmux.SendPromptFileSafe(sessionName, promptFile, false); err != nil {
					logger.Warn("Failed to send prompt file to Gemini", "error", err, "file", promptFile)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else if readErr == nil {
					verifyAndRetryPromptDelivery(sessionName, string(promptContent), func() error {
						return tmux.SendPromptFileSafe(sessionName, promptFile, false)
					})
				}
			}
		default:
			debug.Log("Detached mode: skipping Gemini prompt wait and prompt delivery")
		}
	case harnessName == "opencode-cli":
		// OpenCode CLI: no rename/assoc init sequence, but wait for prompt readiness
		// in non-test mode so that --prompt flag delivery works reliably.
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
			// Deliver --prompt / --prompt-file if provided
			if prompt != "" {
				debug.Log("Sending prompt to OpenCode session from --prompt flag")
				if err := tmux.SendPromptLiteral(sessionName, prompt, false); err != nil {
					logger.Warn("Failed to send prompt to OpenCode", "error", err)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else {
					verifyAndRetryPromptDelivery(sessionName, prompt, func() error {
						return tmux.SendPromptLiteral(sessionName, prompt, false)
					})
				}
			} else if promptFile != "" {
				debug.Log("Sending prompt from file to OpenCode session: %s", promptFile)
				promptContent, readErr := os.ReadFile(promptFile)
				if err := tmux.SendPromptFileSafe(sessionName, promptFile, false); err != nil {
					logger.Warn("Failed to send prompt file to OpenCode", "error", err, "file", promptFile)
					fmt.Println("  • You can manually enter the prompt in the session")
				} else if readErr == nil {
					verifyAndRetryPromptDelivery(sessionName, string(promptContent), func() error {
						return tmux.SendPromptFileSafe(sessionName, promptFile, false)
					})
				}
			}
		default:
			debug.Log("Detached mode: skipping OpenCode prompt wait and prompt delivery")
		}
	default:
		// Other CLI harnesses (codex-cli) - no initialization sequence needed
		debug.Log("Skipping initialization sequence for harness: %s", harnessName)
	}

	// Apply --mode flag for non-claude-code paths (test mode, other harnesses)
	// Skip if already applied at Point A (claude-code normal path)
	if modeFlagValue != "" && (harnessName != "claude-code" || os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "") {
		applyCreationModeSwitch(sessionName, harnessName, modeFlagValue)
	}

	// Update VS Code tab title if running in VS Code
	updateVSCodeTabTitle(sessionName)

	// Attach to session (or show detached message)
	if !detached {
		socketPath := tmux.GetSocketPath()
		debug.Log("Attaching to tmux session: %s (socket: %s)", sessionName, socketPath)
		fmt.Printf("Attaching to tmux session: %s\n", sessionName)
		// Use wrapper for all agents to capture exit summaries
		if err := attachWithCapture(sessionName); err != nil {
			ui.PrintWarning(fmt.Sprintf("Could not attach to session: %v", err))
			fmt.Printf("Session created successfully. Attach manually with: tmux attach -t %s\n", sessionName)
		}
	} else {
		ui.PrintSuccess(fmt.Sprintf("Session '%s' created (detached)", sessionName))
		fmt.Printf("\nAttach to session with:\n  agmresume %s\n", sessionName)
		fmt.Printf("Or manually:\n  tmux attach -t %s\n", sessionName)
	}

	return nil
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
	// BUG-002: Check for duplicate session name before creating
	if !testMode {
		if dupErr := checkDuplicateSessionName(sessionName); dupErr != nil {
			return dupErr
		}
	}

	// CPU circuit breakers: refuse spawn if system is overloaded.
	if !testMode {
		if err := enforceCircuitBreakers(); err != nil {
			return err
		}
	}

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
			Lifecycle:     "",            // Empty = active
			Workspace:     cfg.Workspace, // Populate from workspace detection
			Context: manifest.Context{
				Project: workDir,
				Purpose: "",
				Tags:    nil,
				Notes:   "",
			},
			Tmux: manifest.Tmux{
				SessionName: sessionName,
			},
			Harness: harnessName,
			Model:   modelName,
		}

		// Write to Dolt database
		// BUG-006: Register ALL sessions in Dolt (including test sessions)
		if testMode {
			m.IsTest = true
			debug.Log("Marking session as test (is_test=true)")
		}

		adapter, err := getStorage()
		if err != nil {
			ui.PrintWarning(fmt.Sprintf("Failed to connect to Dolt: %v", err))
		} else {
			defer adapter.Close()
			if err := adapter.CreateSession(m); err != nil {
				ui.PrintWarning(fmt.Sprintf("Failed to create session in Dolt: %v", err))
			} else {
				if testMode {
					ui.PrintSuccess(fmt.Sprintf("Test session registered in database: %s (hidden from default list)", m.SessionID))
				} else {
					ui.PrintSuccess(fmt.Sprintf("Session registered in database: %s", m.SessionID))
				}

				// Auto-commit manifest change if in git repo
				_ = git.CommitManifest(manifestPath, "create", sessionName) // Errors logged internally
			}
		}
	}

	// Start harness-specific initialization
	switch harnessName {
	case "claude-code":
		// Cleanup Claude ready-files from previous sessions (if any)
		claudeReady := tmux.NewClaudeReadyFile(sessionName)
		if err := claudeReady.Cleanup(); err != nil {
			debug.Log("Warning: failed to cleanup ready-files: %v", err)
		}

		// Start Claude in current pane
		fmt.Println("Starting Claude CLI...")
		// Use --add-dir to pre-approve workspace and avoid trust prompt
		// Prefer PWD to preserve symlinks (workDir already set from os.Getwd() above)
		// Pass AGM_SESSION_NAME env var so SessionStart hook can create ready-file signal
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
		if modeFlagValue != "" && (modeFlagValue == "auto" || modeFlagValue == "plan" || modeFlagValue == "default") {
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

		// Give Claude a moment to initialize
		time.Sleep(500 * time.Millisecond)

		// Use text-parsing to wait for Claude prompt (reliable for tmux-started sessions)
		// Manual hook triggers create false positives since the hook runs immediately
		// but Claude hasn't actually started in tmux yet
		fmt.Println("Waiting for Claude to initialize...")
		if err := tmux.WaitForClaudePrompt(sessionName, 30*time.Second); err != nil {
			ui.PrintWarning("Claude ready signal not detected")
			fmt.Printf("💡 Session may still work, but initialization timing is uncertain.\n")
		} else {
			ui.PrintSuccess("Claude is ready!")
		}

		// Now manually trigger the hook to create the ready-file for consistency
		// This allows the hook to run even though Claude started non-interactively
		debug.Log("Triggering SessionStart hook post-verification")
		if err := claudeReady.TriggerHookManually(); err != nil {
			debug.Log("Manual hook trigger failed (non-fatal): %v", err)
		}

		// Skip init sequence (rename/assoc) in test environments
		if os.Getenv("AGM_TEST_RUN_ID") != "" || os.Getenv("AGM_TEST_ENV") != "" {
			debug.Log("Skipping InitSequence in test environment")
			ui.PrintSuccess("Test session ready (init sequence skipped)")
		} else {
			// Use InitSequence to properly sequence /rename and /agm:agm-assoc commands
			// This uses tmux control mode to wait for each command to complete before sending the next
			debug.Log("Running InitSequence for /rename and /agm:agm-assoc")
			seq := tmux.NewInitSequence(sessionName)
			if err := seq.Run(); err != nil {
				debug.Log("InitSequence failed: %v", err)
				ui.PrintWarning("Failed to run initialization sequence")
				fmt.Printf("💡 You can manually run:\n")
				fmt.Printf("  /rename %s\n", sessionName)
				fmt.Printf("  /agm:agm-assoc %s\n", sessionName)
			} else {
				debug.Log("InitSequence completed successfully")

				// Wait for ready-file (created by agm associate when UUID is captured)
				debug.Log("Waiting for ready-file signal (timeout: 60s)")
				if err := readiness.WaitForReady(sessionName, 60*time.Second); err != nil {
					debug.Log("Ready-file wait failed: %v", err)
					ui.PrintWarning("Ready-file not created within timeout")
					fmt.Printf("💡 Session is usable, but UUID association may have failed\n")
					fmt.Printf("  • Run 'agm sync' later to populate UUID if needed\n")
				} else {
					debug.Log("Ready-file detected - agm binary completed")

					// Wait for skill to finish outputting and return to prompt
					// The ready-file signals when 'agm associate' binary completes, but the skill
					// continues to output messages after that. Wait for Claude prompt to ensure
					// skill has finished before returning control to user.
					debug.Log("Waiting for skill to complete output and return to prompt")
					if err := tmux.WaitForClaudePrompt(sessionName, 10*time.Second); err != nil {
						debug.Log("Prompt wait failed (non-fatal): %v", err)
						// Fallback to fixed delay if prompt detection fails
						time.Sleep(1 * time.Second)
					}

					ui.PrintSuccess("Claude is ready and session associated!")
				}
			}
		}

	case "opencode-cli":
		// OpenCode client-server architecture - start attach in current pane
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

		// Wait for OpenCode prompt readiness
		fmt.Println("Waiting for OpenCode to initialize...")
		if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
			ui.PrintWarning("OpenCode ready signal not detected")
			fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
		} else {
			ui.PrintSuccess("OpenCode is ready!")
		}

	case "gemini-cli":
		// Start Gemini in current pane
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

		// Auto-trust: Gemini CLI asks "Do you trust the files in this folder?"
		debug.Log("Checking for Gemini trust prompt (3s window)...")
		time.Sleep(2 * time.Second)
		socketPath := tmux.GetSocketPath()
		normalizedName := tmux.NormalizeTmuxSessionName(sessionName)
		trustCheckCmd := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedName, "-p", "-S", "-20")
		if trustOutput, err := trustCheckCmd.CombinedOutput(); err == nil {
			content := string(trustOutput)
			if strings.Contains(content, "Do you trust") || strings.Contains(content, "trust the files") {
				debug.Log("Gemini trust prompt detected, auto-accepting with '1' + Enter")
				selectCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "1")
				_ = selectCmd.Run()
				time.Sleep(300 * time.Millisecond)
				enterCmd := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedName, "Enter")
				_ = enterCmd.Run()
				debug.Log("Trust prompt auto-accepted")
				ui.PrintSuccess("Auto-accepted Gemini trust prompt")
			} else {
				debug.Log("No trust prompt detected (directory already trusted)")
			}
		}

		// Wait for Gemini prompt readiness
		fmt.Println("Waiting for Gemini to initialize...")
		if err := tmux.WaitForPromptSimple(sessionName, 30*time.Second); err != nil {
			ui.PrintWarning("Gemini ready signal not detected")
			fmt.Printf("Session may still work, but initialization timing is uncertain.\n")
		} else {
			ui.PrintSuccess("Gemini is ready!")
		}

	default:
		// Other harnesses - no CLI startup configured yet
		debug.Log("Skipping CLI startup for harness: %s (no CLI configured)", harnessName)
		ui.PrintSuccess(fmt.Sprintf("Session created for %s harness", harnessName))
	}

	ui.PrintSuccess(fmt.Sprintf("%s session started in current tmux!", harnessName))

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

	// Get lower dirs from config, fallback to scanning workspace repos if none configured
	lowerDirs := cfg.Sandbox.Repos
	if len(lowerDirs) == 0 {
		// Try to find repos in the workspace
		// Check common workspace repo locations
		wsRoot := filepath.Join(os.Getenv("HOME"), "src", "ws", "oss", "repos")
		if entries, err := os.ReadDir(wsRoot); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					repoPath := filepath.Join(wsRoot, e.Name())
					gitDir := filepath.Join(repoPath, ".git")
					if _, err := os.Stat(gitDir); err == nil {
						lowerDirs = append(lowerDirs, repoPath)
					}
				}
			}
		}
		if len(lowerDirs) == 0 {
			lowerDirs = []string{workDir}
			debug.Log("No repos found, using workDir as lower dir: %s", workDir)
		} else {
			debug.Log("Found %d repos in workspace: %v", len(lowerDirs), lowerDirs)
		}
	}

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
