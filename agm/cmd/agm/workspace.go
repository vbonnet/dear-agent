package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/tabwriter"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/fileutil"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/pkg/workspace"
	"gopkg.in/yaml.v3"
)

// workspaceInfo holds display information for a workspace
type workspaceInfo struct {
	name         string
	path         string
	sessionCount int
	isCurrent    bool
}

var workspaceCmd = &cobra.Command{
	Use:   "workspace",
	Short: "Manage AGM workspaces",
	Long: `Workspace commands for managing multi-workspace configurations.

Examples:
  agm workspace new my-proj  # Create new workspace
  agm workspace list         # List all configured workspaces
  agm workspace show oss     # Show workspace details
  agm workspace del acme   # Remove workspace from config`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var workspaceNewCmd = &cobra.Command{
	Use:   "new <name>",
	Short: "Create a new workspace",
	Long: `Create a new AGM workspace with interactive prompts.

The workspace name must be alphanumeric with hyphens and underscores only.
You will be prompted to specify the workspace root path.

The command will:
  1. Validate the workspace name doesn't already exist
  2. Prompt for workspace root path (with validation)
  3. Update ~/.agm/config.yaml atomically
  4. Create .agm/sessions/ directory in workspace root

Examples:
  agm workspace new my-project
  agm workspace new acme-work
  agm workspace new personal-dev`,
	Args: cobra.ExactArgs(1),
	RunE: runWorkspaceNew,
}

var workspaceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured workspaces",
	Long: `Display a table of all configured workspaces showing:
- Name: Workspace identifier
- Path: Root directory path
- Sessions: Number of sessions in workspace

The current workspace (if detectable) is marked with an indicator.

Examples:
  agm workspace list    # Show all workspaces`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Determine workspace config path
		workspaceConfigPath := cfg.WorkspaceConfigPath
		if workspaceConfigPath == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			workspaceConfigPath = filepath.Join(home, ".agm", "config.yaml")
		}

		// Check if config exists
		if _, err := os.Stat(workspaceConfigPath); os.IsNotExist(err) {
			ui.PrintWarning(fmt.Sprintf("No workspace config found at %s", workspaceConfigPath))
			fmt.Printf("\nWorkspaces are not configured. Using default sessions directory.\n")
			fmt.Printf("\nTo configure workspaces, create %s with:\n", workspaceConfigPath)
			fmt.Printf("  version: 1\n")
			fmt.Printf("  default_workspace: default\n")
			fmt.Printf("  workspaces:\n")
			fmt.Printf("    - name: default\n")
			fmt.Printf("      root: ~/src\n")
			fmt.Printf("      enabled: true\n")
			return nil
		}

		// Load workspace config
		config, err := workspace.LoadConfig(workspaceConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load workspace config: %w", err)
		}

		// Check if any workspaces configured
		if len(config.Workspaces) == 0 {
			ui.PrintWarning("No workspaces configured")
			return nil
		}

		// Build workspace list with session counts
		var workspaceList []workspaceInfo

		for _, ws := range config.Workspaces {
			// Skip disabled workspaces
			if !ws.Enabled {
				continue
			}

			// Check if this is the current workspace
			isCurrent := cfg.Workspace == ws.Name

			// Count sessions in workspace
			sessionCount := 0

			// Only query Dolt for the current workspace
			// Non-current workspaces show 0 (Phase 3 limitation - will be fixed in Phase 4)
			if isCurrent {
				adapter, err := getStorage()
				if err == nil {
					defer adapter.Close()
					manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
					if err == nil {
						sessionCount = len(manifests)
					}
				}
			}
			// Note: sessionCount remains 0 for non-current workspaces

			workspaceList = append(workspaceList, workspaceInfo{
				name:         ws.Name,
				path:         ws.Root,
				sessionCount: sessionCount,
				isCurrent:    isCurrent,
			})
		}

		if len(workspaceList) == 0 {
			ui.PrintWarning("No enabled workspaces found")
			return nil
		}

		// Format as table
		output := formatWorkspaceTable(workspaceList)
		fmt.Print(output)

		return nil
	},
}

func formatWorkspaceTable(workspaces []workspaceInfo) string {
	uiCfg := ui.GetGlobalConfig()
	palette := ui.GetPalette(uiCfg.UI.Theme)

	// Header style
	headerStyle := lipgloss.NewStyle().
		Foreground(palette.Header).
		Bold(true)

	// Current workspace style (green/active)
	currentStyle := lipgloss.NewStyle().
		Foreground(palette.Active).
		Bold(true)

	// Normal workspace style
	normalStyle := lipgloss.NewStyle().
		Foreground(palette.Info)

	// Create table
	var buf bytes.Buffer
	w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

	// Header
	fmt.Fprintf(w, "%s\t%s\t%s\n",
		"NAME",
		"PATH",
		"SESSIONS")

	// Rows
	for _, ws := range workspaces {
		// Compact home directory in path
		displayPath := compactHomePath(ws.path)

		// Format row
		indicator := "  "
		if ws.isCurrent {
			indicator = "● " // Current workspace indicator
		}

		fmt.Fprintf(w, "%s%s\t%s\t%d\n",
			indicator,
			ws.name,
			displayPath,
			ws.sessionCount)
	}

	w.Flush()

	// Apply styling to output
	var result bytes.Buffer
	lines := bytes.Split(buf.Bytes(), []byte("\n"))

	// Style header (first line)
	if len(lines) > 0 {
		result.WriteString(headerStyle.Render(string(lines[0])))
		result.WriteString("\n")
	}

	// Style data rows
	for i, ws := range workspaces {
		if i+1 >= len(lines) {
			break
		}
		line := string(lines[i+1])
		if line == "" {
			continue
		}

		// Use different style for current workspace
		if ws.isCurrent {
			result.WriteString(currentStyle.Render(line))
		} else {
			result.WriteString(normalStyle.Render(line))
		}
		result.WriteString("\n")
	}

	// Add helpful footer
	result.WriteString("\n")
	footerStyle := lipgloss.NewStyle().Foreground(palette.Info)
	if cfg.Workspace != "" {
		result.WriteString(footerStyle.Render(fmt.Sprintf("● Current workspace: %s", cfg.Workspace)))
	} else {
		result.WriteString(footerStyle.Render("Using default sessions directory (no workspace detected)"))
	}
	result.WriteString("\n")

	return result.String()
}

// compactHomePath replaces home directory with ~ for display
func compactHomePath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == homeDir {
		return "~"
	}
	if len(path) > len(homeDir) && path[:len(homeDir)] == homeDir {
		return "~" + path[len(homeDir):]
	}
	return path
}

var workspaceShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show detailed workspace information",
	Long: `Display detailed information about a workspace including its configuration,
path, session count, and list of sessions.

Examples:
  agm workspace show oss         # Show OSS workspace details
  agm workspace show acme      # Show Acme Corp workspace details`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaceName := args[0]

		// Load workspace config
		workspaceConfigPath, err := getWorkspaceConfigPath()
		if err != nil {
			return err
		}

		wsConfig, err := workspace.LoadConfig(workspaceConfigPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("workspace config not found at %s\n  Run 'agm workspace init' to create configuration", workspaceConfigPath)
			}
			return fmt.Errorf("failed to load workspace config: %w", err)
		}

		// Find workspace
		var ws *workspace.Workspace
		for i := range wsConfig.Workspaces {
			if wsConfig.Workspaces[i].Name == workspaceName {
				ws = &wsConfig.Workspaces[i]
				break
			}
		}

		if ws == nil {
			return fmt.Errorf("workspace '%s' not found\n  Available workspaces: %s",
				workspaceName, listWorkspaceNames(wsConfig))
		}

		// Display workspace information
		fmt.Printf("\n")
		fmt.Printf("Workspace: %s\n", ui.Blue(ws.Name))
		fmt.Printf("Root:      %s\n", ws.Root)
		fmt.Printf("Status:    %s\n", formatWorkspaceStatus(ws, wsConfig.DefaultWorkspace == ws.Name))

		// Calculate sessions directory
		sessionsDir := filepath.Join(ws.Root, ".agm", "sessions")
		fmt.Printf("Sessions:  %s\n", sessionsDir)

		// Count and list sessions
		sessionCount, sessions, err := countWorkspaceSessions(workspaceName, sessionsDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("Count:     0 (no sessions directory)\n")
			} else {
				fmt.Printf("Count:     error reading sessions: %v\n", err)
			}
		} else {
			fmt.Printf("Count:     %d\n", sessionCount)
			if sessionCount > 0 {
				fmt.Printf("\nSessions:\n")
				for _, s := range sessions {
					status := "stopped"
					// Check lifecycle status
					if s.Lifecycle == manifest.LifecycleArchived {
						status = "archived"
					}
					fmt.Printf("  • %s (%s)\n", ui.Blue(s.Name), status)
				}
			}
		}

		fmt.Printf("\n")
		return nil
	},
}

var workspaceDelCmd = &cobra.Command{
	Use:   "del <name>",
	Short: "Remove workspace from configuration",
	Long: `Remove a workspace from AGM configuration. This does NOT delete any session
files or directories - it only removes the workspace from the config file.

Sessions in the workspace directory will remain intact and can still be accessed
by explicitly specifying the sessions directory with --sessions-dir flag.

Examples:
  agm workspace del old-project  # Remove old-project workspace from config`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaceName := args[0]

		// Load workspace config
		workspaceConfigPath, err := getWorkspaceConfigPath()
		if err != nil {
			return err
		}

		wsConfig, err := workspace.LoadConfig(workspaceConfigPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("workspace config not found at %s\n  Run 'agm workspace init' to create configuration", workspaceConfigPath)
			}
			return fmt.Errorf("failed to load workspace config: %w", err)
		}

		// Find workspace
		workspaceIndex := -1
		var ws *workspace.Workspace
		for i := range wsConfig.Workspaces {
			if wsConfig.Workspaces[i].Name == workspaceName {
				workspaceIndex = i
				ws = &wsConfig.Workspaces[i]
				break
			}
		}

		if ws == nil {
			return fmt.Errorf("workspace '%s' not found\n  Available workspaces: %s",
				workspaceName, listWorkspaceNames(wsConfig))
		}

		// Count sessions for informational message
		sessionsDir := filepath.Join(ws.Root, ".agm", "sessions")
		sessionCount, _, _ := countWorkspaceSessions(workspaceName, sessionsDir)

		// Show confirmation prompt
		confirmed, err := confirmWorkspaceDeletion(workspaceName, ws.Root, sessionCount)
		if err != nil {
			return err
		}

		if !confirmed {
			fmt.Println("Cancelled.")
			return nil
		}

		// Remove workspace from config
		wsConfig.Workspaces = append(wsConfig.Workspaces[:workspaceIndex], wsConfig.Workspaces[workspaceIndex+1:]...)

		// If this was the default workspace, clear the default
		if wsConfig.DefaultWorkspace == workspaceName {
			wsConfig.DefaultWorkspace = ""
			ui.PrintWarning(fmt.Sprintf("Removed default workspace '%s'. Set a new default with 'agm workspace set-default'", workspaceName))
		}

		// Save updated config
		if err := workspace.SaveConfig(workspaceConfigPath, wsConfig); err != nil {
			return fmt.Errorf("failed to save workspace config: %w", err)
		}

		fmt.Printf("\n")
		ui.PrintSuccess(fmt.Sprintf("Removed workspace '%s' from configuration", workspaceName))
		fmt.Printf("\n")
		if sessionCount > 0 {
			fmt.Printf("Note: %d session(s) remain in %s\n", sessionCount, sessionsDir)
			fmt.Printf("      Sessions are not deleted, only removed from workspace config.\n")
			fmt.Printf("      To access them, use: agm --sessions-dir %s\n", sessionsDir)
		}
		fmt.Printf("\n")

		return nil
	},
}

func runWorkspaceNew(cmd *cobra.Command, args []string) error {
	workspaceName := args[0]

	// Validate workspace name (alphanumeric, hyphens, underscores only)
	validName := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validName.MatchString(workspaceName) {
		return fmt.Errorf("invalid workspace name '%s': must be alphanumeric with hyphens and underscores only", workspaceName)
	}

	// Check length constraints
	if len(workspaceName) < 2 {
		return fmt.Errorf("workspace name must be at least 2 characters")
	}
	if len(workspaceName) > 64 {
		return fmt.Errorf("workspace name must be 64 characters or less")
	}

	// Load existing workspace config (or create new one)
	workspaceConfigPath, err := getWorkspaceConfigPath()
	if err != nil {
		return err
	}

	var wsConfig *workspace.Config
	if _, err := os.Stat(workspaceConfigPath); os.IsNotExist(err) {
		// Create default config
		wsConfig = &workspace.Config{
			Version:    1,
			Workspaces: []workspace.Workspace{},
		}
	} else {
		wsConfig, err = workspace.LoadConfig(workspaceConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load workspace config: %w", err)
		}
	}

	// Check if workspace already exists
	for _, ws := range wsConfig.Workspaces {
		if ws.Name == workspaceName {
			return fmt.Errorf("workspace '%s' already exists", workspaceName)
		}
	}

	// Prompt for workspace root path
	var rootPath string
	err = huh.NewInput().
		Title("Workspace root path").
		Description("Enter the root directory for this workspace (will be created if doesn't exist)").
		Placeholder("~/src/my-project").
		Value(&rootPath).
		Validate(func(s string) error {
			if s == "" {
				return fmt.Errorf("root path cannot be empty")
			}
			// Expand home directory
			expanded := workspace.ExpandHome(s)
			// Must be absolute path
			if !filepath.IsAbs(expanded) {
				return fmt.Errorf("path must be absolute: %s", expanded)
			}
			// Check if path exists
			if _, err := os.Stat(expanded); os.IsNotExist(err) {
				// Path doesn't exist - check if parent is writable
				parent := filepath.Dir(expanded)
				if _, err := os.Stat(parent); os.IsNotExist(err) {
					return fmt.Errorf("parent directory does not exist: %s", parent)
				}
				// Try to create a test file in parent to check writability
				testFile := filepath.Join(parent, ".agm-test-"+filepath.Base(expanded))
				if f, err := os.Create(testFile); err == nil {
					f.Close()
					os.Remove(testFile)
				} else {
					return fmt.Errorf("parent directory is not writable: %s", parent)
				}
			}
			return nil
		}).
		Run()

	if err != nil {
		return err
	}

	// Expand home directory in root path
	expandedRoot := workspace.ExpandHome(rootPath)

	// Create workspace root if it doesn't exist
	if err := os.MkdirAll(expandedRoot, 0755); err != nil {
		return fmt.Errorf("failed to create workspace root: %w", err)
	}

	// Create .agm/sessions directory in workspace root
	sessionsDir := filepath.Join(expandedRoot, ".agm", "sessions")
	if err := os.MkdirAll(sessionsDir, 0755); err != nil {
		return fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Create new workspace
	newWorkspace := workspace.Workspace{
		Name:      workspaceName,
		Root:      expandedRoot,
		OutputDir: filepath.Join(expandedRoot, ".agm"),
		Enabled:   true,
	}

	// Add to config
	wsConfig.Workspaces = append(wsConfig.Workspaces, newWorkspace)

	// If this is the first workspace, make it default
	if len(wsConfig.Workspaces) == 1 {
		wsConfig.DefaultWorkspace = workspaceName
	}

	// Save config (atomically with backup)
	if err := saveWorkspaceConfigAtomic(workspaceConfigPath, wsConfig); err != nil {
		return fmt.Errorf("failed to save workspace config: %w", err)
	}

	fmt.Printf("\n")
	ui.PrintSuccess(fmt.Sprintf("Created workspace '%s'", workspaceName))
	fmt.Printf("  Root:     %s\n", expandedRoot)
	fmt.Printf("  Sessions: %s\n", sessionsDir)
	if wsConfig.DefaultWorkspace == workspaceName {
		fmt.Printf("  Status:   %s (default)\n", ui.Green("enabled"))
	} else {
		fmt.Printf("  Status:   %s\n", ui.Green("enabled"))
	}
	fmt.Printf("\nWorkspace is now available for use.\n")
	fmt.Printf("\n")

	return nil
}

// saveWorkspaceConfigAtomic saves workspace config atomically with backup
func saveWorkspaceConfigAtomic(path string, config *workspace.Config) error {
	// Validate config before saving
	if err := workspace.ValidateConfig(config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Create backup of existing config if it exists
	if _, err := os.Stat(path); err == nil {
		backupPath := path + ".backup"
		if err := os.Rename(path, backupPath); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to create backup: %v\n", err)
		} else {
			// Remove backup on success
			defer os.Remove(backupPath)
		}
	}

	// Write atomically using fileutil.AtomicWrite
	if err := fileutil.AtomicWrite(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func init() {
	rootCmd.AddCommand(workspaceCmd)
	workspaceCmd.AddCommand(workspaceNewCmd)
	workspaceCmd.AddCommand(workspaceListCmd)
	workspaceCmd.AddCommand(workspaceShowCmd)
	workspaceCmd.AddCommand(workspaceDelCmd)
}

// getWorkspaceConfigPath returns the path to the workspace config file
func getWorkspaceConfigPath() (string, error) {
	if cfg != nil && cfg.WorkspaceConfigPath != "" {
		return cfg.WorkspaceConfigPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(home, ".agm", "config.yaml"), nil
}

// listWorkspaceNames returns a comma-separated list of workspace names
func listWorkspaceNames(wsConfig *workspace.Config) string {
	names := make([]string, 0, len(wsConfig.Workspaces))
	for _, ws := range wsConfig.Workspaces {
		names = append(names, ws.Name)
	}
	if len(names) == 0 {
		return "(none)"
	}
	return strings.Join(names, ", ")
}

// formatWorkspaceStatus returns a formatted status string for the workspace
func formatWorkspaceStatus(ws *workspace.Workspace, isDefault bool) string {
	status := ""
	if ws.Enabled {
		status = ui.Green("enabled")
	} else {
		status = ui.Yellow("disabled")
	}

	if isDefault {
		status += " (default)"
	}

	return status
}

// countWorkspaceSessions counts sessions in a workspace using Dolt
// Note: Only works for the current workspace (Phase 3 limitation)
func countWorkspaceSessions(workspaceName string, _ string) (int, []*manifest.Manifest, error) {
	// Only query Dolt for the current workspace
	if cfg.Workspace == workspaceName {
		adapter, err := getStorage()
		if err != nil {
			return 0, nil, fmt.Errorf("failed to connect to Dolt: %w", err)
		}
		defer adapter.Close()

		manifests, err := adapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			return 0, nil, fmt.Errorf("failed to list sessions from Dolt: %w", err)
		}
		return len(manifests), manifests, nil
	}

	// For non-current workspaces, cannot query their Dolt database
	// This is a temporary Phase 3 limitation
	return 0, nil, fmt.Errorf("cannot query non-current workspace (current: %s)", cfg.Workspace)
}

// confirmWorkspaceDeletion prompts user for confirmation before deleting workspace
func confirmWorkspaceDeletion(name, root string, sessionCount int) (bool, error) {
	var confirmed bool

	desc := fmt.Sprintf("Root: %s\n", root)
	if sessionCount > 0 {
		desc += fmt.Sprintf("Sessions: %d (will NOT be deleted)\n", sessionCount)
	} else {
		desc += "Sessions: 0\n"
	}
	desc += "\nThis only removes the workspace from config.\nSession files will remain intact."

	err := huh.NewConfirm().
		Title(fmt.Sprintf("Remove workspace '%s'?", name)).
		Description(desc).
		Affirmative("Yes, remove").
		Negative("Cancel").
		Value(&confirmed).
		Run()

	if err != nil {
		return false, err
	}

	return confirmed, nil
}
