package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/modelrouter"
	"github.com/vbonnet/dear-agent/agm/internal/rbac"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/agm/internal/workflow"
	"github.com/vbonnet/dear-agent/internal/pricing"
	"github.com/vbonnet/dear-agent/pkg/workspace"
	// Import sandbox providers to trigger registration. Each provider's
	// init() registers itself on its supported platform; on other platforms
	// the package compiles to an empty stub. Without these imports the
	// providers are never registered and selecting them returns
	// "provider not available".

	// Sandbox providers register themselves via init(); these blank
	// imports are load-bearing (BL-011, guarded by new_sandbox_wiring_test.go).
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
	modelTierFlag      string // --model-tier: cheap, mid, expensive
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
	persistent         bool

	// brakeOverrideReason requests the audited admission-brake override and
	// states why. Empty means the brake is honoured, which is the default.
	brakeOverrideReason string
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
	Short: "Create a new AGM-managed harness session with tmux",
	Long: `Create a new AGM-managed harness session with tmux integration.

This command will:
1. Create or use an existing tmux session with the specified name
2. Start the selected harness CLI in the tmux session
3. Create a manifest linking the tmux session to the harness session

Arguments:
  session-name - Name for the tmux/harness session (optional)
                 If not provided and outside tmux, you'll be prompted
                 If not provided and inside tmux, uses current tmux session name

Flags:
  --detached    - Create session without attaching (useful when inside tmux)
  --workspace   - Specify workspace (oss, acme) or "auto" for interactive selection
                  If omitted, uses auto-detected workspace or prompts if detection fails
  --harness     - Harness to use (claude-code, codex-cli, agy, opencode-cli, pi-cli)
                  Deprecated compatibility: gemini-cli
                  If omitted, prompts interactively
  --model       - Model to use (e.g., sonnet, 3.5-flash, 3.5-flash-low, 5.5)
                  If omitted, uses default for harness

Workspace Detection:
  • --workspace=oss           → Use OSS workspace explicitly
  • --workspace=acme        → Use Acme Corp workspace explicitly
  • --workspace=auto          → Trigger interactive workspace selection
  • No --workspace flag       → Auto-detect from current directory or prompt if failed
  • Sessions stored in: {workspace_root}/.agm/sessions

Behavior:
  • Outside tmux + no name → Prompts for name, creates tmux + selected harness
  • Outside tmux + name provided → Creates tmux session with that name + selected harness
  • Inside tmux + no name → Uses current tmux name, starts selected harness
  • Inside tmux + matching name → Uses current tmux, except AGY requires --detached
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
			tc, err := testcontext.LoadNamed(testEnvName)
			if err != nil {
				return fmt.Errorf("invalid test environment %q: %w", testEnvName, err)
			}
			if err := tc.SetEnv(); err != nil {
				return fmt.Errorf("failed to activate test environment: %w", err)
			}
			testMode = true // treat as test mode for model selection, isolation, etc.
			debug.Log("Using test environment: %s", testEnvName)
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
				huh.NewOption("Codex CLI (OpenAI)", "codex-cli"),
				huh.NewOption("AGY (Antigravity CLI)", "agy"),
				huh.NewOption("OpenCode CLI (Multi-provider)", "opencode-cli"),
				huh.NewOption("Pi (Extensible coding agent)", "pi-cli"),
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
						"  • Available harnesses: claude-code, codex-cli, agy, opencode-cli, pi-cli")
				return err
			}
			harnessName = selectedHarness
		}
		harnessName = agent.NormalizeHarnessName(harnessName)

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
				"  • Valid active harnesses: claude-code, codex-cli, agy, opencode-cli, pi-cli\n"+
					"  • Deprecated compatibility harness: gemini-cli\n"+
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

		// Apply model-tier routing when --model-tier is set and --model was not
		// explicitly specified. The router picks the harness-appropriate alias for
		// the requested tier; if the harness is not in the tier table the router
		// returns an empty model and the normal default falls through below.
		if !testMode && modelTierFlag != "" && modelName == "" {
			routePrompt := prompt
			if routePrompt == "" && promptFile != "" {
				// best-effort: read first 512 bytes for classification
				if data, err := os.ReadFile(promptFile); err == nil {
					runes := []rune(string(data))
					if len(runes) > 512 {
						runes = runes[:512]
					}
					routePrompt = string(runes)
				}
			}
			d, routeErr := modelrouter.Route(harnessName, modelTierFlag, "", routePrompt)
			if routeErr != nil {
				return fmt.Errorf("--model-tier: %w", routeErr)
			}
			if d.Model != "" {
				modelName = d.Model
				debug.Log("Model router: tier=%s model=%s reason=%q", d.Tier, d.Model, d.Reason)
				fmt.Printf("Model router: %s → %s (%s)\n", d.Tier, d.Model, d.Reason)
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
			if err := agent.ValidateModel(harnessName, modelName); err != nil {
				return fmt.Errorf("invalid --model: %w", err)
			}
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

		// Set GCP_PROJECT_ID environment variable if provided (for deprecated gemini-cli harness)
		if projectID != "" {
			os.Setenv("GCP_PROJECT_ID", projectID)
			debug.Log("Set GCP_PROJECT_ID: %s", projectID)
		}

		// Now we have a session name. Handle the scenarios:
		// 1. Inside tmux + not detached: start Claude in current session
		// 2. Outside tmux OR detached: create tmux session, start Claude, attach (or not if detached)

		return startNewSessionForContext(cmd.Context(), inTmux, detached, sessionName, harnessName, realNewSessionStartRuntime())
	},
}

type newSessionStartRuntime struct {
	currentTmux  func(context.Context, string) error
	separateTmux func(context.Context, string) error
}

func realNewSessionStartRuntime() newSessionStartRuntime {
	return newSessionStartRuntime{
		currentTmux:  startClaudeInCurrentTmux,
		separateTmux: createTmuxSessionAndStartClaude,
	}
}

func startNewSessionForContext(ctx context.Context, inTmux, detached bool, sessionName, harness string, runtime newSessionStartRuntime) error {
	if inTmux && !detached {
		if agent.NormalizeHarnessName(harness) == "agy" {
			return currentTmuxAgyUnsupportedError()
		}
		return runtime.currentTmux(ctx, sessionName)
	}
	return runtime.separateTmux(ctx, sessionName)
}

// applyCreationModeSwitch dispatches a mode switch during session creation.
// Non-fatal: errors are logged as warnings and execution continues.
func applyCreationModeSwitch(sessionName, harness, targetMode string) {
	applyCreationModeSwitchContext(context.Background(), sessionName, harness, targetMode)
}

func applyCreationModeSwitchContext(ctx context.Context, sessionName, harness, targetMode string) {
	if targetMode == "" {
		return
	}
	if ctx.Err() != nil {
		return
	}
	debug.Log("Applying creation mode switch: default -> %s (harness: %s)", targetMode, harness)
	if err := dispatchModeSwitchContext(ctx, harness, sessionName, targetMode, "default"); err != nil {
		ui.PrintWarning(fmt.Sprintf("Mode switch to %s failed: %v (continuing with default mode)", targetMode, err))
		return
	}
	ui.PrintSuccess(fmt.Sprintf("Mode set to %s", targetMode))
	if ctx.Err() != nil {
		return
	}
	adapter, err := getStorage()
	if err != nil {
		debug.Log("Could not connect to storage for mode manifest update: %v", err)
		return
	}
	defer func() { _ = adapter.Close() }()
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

// buildSessionTags combines --role and --tags into a single tag slice.
// verifyAndRetryPromptDelivery verifies prompt delivery to a session and retries
// if the prompt was not received. This prevents silent delivery failures caused by
// input line conflicts, cooldowns, or timing issues.
//
// Parameters:
//   - sessionName: target tmux session
//   - promptText: the prompt content (used for keyword-based verification)
//   - sendFunc: function that re-sends the prompt on retry
func verifyAndRetryPromptDelivery(ctx context.Context, sessionName, promptText string, sendFunc func() error) error {
	result, err := tmux.VerifyPromptDeliveryContext(ctx, sessionName, promptText, sendFunc, 3)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		debug.Log("Prompt delivery verification error: %v", err)
		logger.Warn("Could not verify prompt delivery", "error", err)
		return nil
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
	return nil
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
	newCmd.Flags().StringVar(&harnessName, "harness", "", "Harness to use (claude-code, codex-cli, agy, opencode-cli, pi-cli; deprecated: gemini-cli) (env: AGM_DEFAULT_HARNESS)")
	newCmd.Flags().StringVar(&modelName, "model", "", "Model to use (e.g., sonnet, 3.5-flash, 3.5-flash-low, 5.5) (env: AGM_DEFAULT_MODEL)")
	newCmd.Flags().StringVar(&modelTierFlag, "model-tier", "", "Cost tier for model routing: cheap (70%), mid (20%), expensive (10%)")
	newCmd.Flags().StringVar(&codexHookTrustBypassReason, "dangerously-bypass-hook-trust", "",
		"Request the audited Codex hook-trust override, stating why (requires `agm override approve codex-hook-trust --codex-hook-source <reviewed-repo>`)")
	newCmd.Flags().StringVar(&brakeOverrideReason, "brake-override", "",
		"Cross an engaged admission brake once, stating why (requires `agm override approve admission-brake`)")
	_ = newCmd.RegisterFlagCompletionFunc("model-tier", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"cheap", "mid", "expensive"}, cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.MarkFlagsMutuallyExclusive("model", "model-tier")
	newCmd.Flags().StringVar(&workspaceName, "workspace", "", "Workspace to use (oss, acme, auto for detection)")
	newCmd.Flags().StringVar(&workflowName, "workflow", "", "Execution workflow (deep-research, code-review, architect)")
	newCmd.Flags().StringVar(&projectID, "project-id", "", "GCP project ID (deprecated gemini-cli compatibility)")
	newCmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send after session initialization")
	newCmd.Flags().StringVar(&promptFile, "prompt-file", "", "File containing prompt to send")
	newCmd.Flags().BoolVar(&noSandbox, "no-sandbox", false, "Disable sandbox isolation (sandbox is ON by default)")
	newCmd.Flags().StringVar(&sandboxProvider, "sandbox-provider", "auto", "Sandbox provider (auto, bubblewrap, overlayfs, gvisor, apfs, mock)")
	newCmd.Flags().Float64Var(&maxBudgetUsd, "max-budget-usd", 0, "Maximum budget in USD for the session (passed to claude --max-budget-usd)")
	newCmd.Flags().StringVar(&testEnvName, "test-env", "", "Use named test environment (created via 'agm test-env create')")
	newCmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	// harness and workspace flags are now optional - prompts shown if omitted
	newCmd.Flags().StringVar(&modeFlagValue, "mode", "", "Permission mode after init (plan, auto, default) (env: AGM_DEFAULT_MODE)")
	newCmd.Flags().BoolVar(&noAutoMode, "no-auto-mode", false, "Disable --enable-auto-mode flag for Claude (env: AGM_DISABLE_AUTO_MODE)")
	_ = newCmd.RegisterFlagCompletionFunc("mode", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"plan", "auto", "default"}, cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().StringVar(&roleName, "role", "", "Role tag for the session (e.g., orchestrator, worker, researcher)")
	newCmd.Flags().StringSliceVar(&sessionTags, "tags", nil, "Context tags for the session (e.g., 'cap:web-search,cap:claude-code')")
	_ = newCmd.RegisterFlagCompletionFunc("role", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"orchestrator", "meta-orchestrator", "researcher", "worker", "reviewer"}, cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().StringSliceVar(&permissionsAllow, "permissions-allow", nil, "Permission patterns to pre-approve (e.g., 'Bash(tmux:*),Read(~/src/**)') — persisted in the shared policy and projected to the selected harness")
	newCmd.Flags().StringVar(&permissionProfile, "permission-profile", "", "Predefined permission profile (worker, monitor, audit)")
	_ = newCmd.RegisterFlagCompletionFunc("permission-profile", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return rbac.ProfileNames(), cobra.ShellCompDirectiveNoFileComp
	})
	newCmd.Flags().BoolVar(&inheritPermissions, "inherit-permissions", false, "Inherit the canonical parent permission allowlist from ~/.claude/settings.json")
	newCmd.Flags().BoolVar(&disposable, "disposable", false, "Create a disposable session with TTL-based auto-archive")
	newCmd.Flags().StringVar(&disposableTTL, "disposable-ttl", "4h", "TTL for disposable sessions (e.g., 1h, 4h, 30m)")
	newCmd.Flags().BoolVar(&persistent, "persistent", false, "Omit '&&  exit' from the harness launch command; use for long-lived supervisor sessions that must survive a Claude turn/loop ending (e.g. vroom-meta-orchestrator)")

	// Tab completion for --harness flag
	_ = newCmd.RegisterFlagCompletionFunc("harness", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return agent.ActiveHarnesses(), cobra.ShellCompDirectiveNoFileComp
	})
	// Tab completion for --model flag (context-sensitive based on --harness value)
	_ = newCmd.RegisterFlagCompletionFunc("model", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		h, _ := cmd.Flags().GetString("harness")
		if h == "" {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return agent.ModelAliases(h), cobra.ShellCompDirectiveNoFileComp
	})
}
