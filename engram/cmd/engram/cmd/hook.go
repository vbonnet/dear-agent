package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"
)

// Hook represents a verification hook configuration
type Hook struct {
	Name        string   `toml:"name"`
	Event       string   `toml:"event"`
	Priority    int      `toml:"priority"`
	Type        string   `toml:"type"`
	Command     string   `toml:"command"`
	Args        []string `toml:"args"`
	Timeout     int      `toml:"timeout"`
	CommandHash string   `toml:"command_hash"`
}

// HookRegistry represents the hooks.toml configuration
type HookRegistry struct {
	Hooks []Hook `toml:"hooks"`
}

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage verification hooks",
	Long: `Manage verification hooks for session completion checks.

Hooks are configured in ~/.engram/hooks.toml and executed at specific
events (session-completion, phase-completion, pre-commit).`,
}

var hookInstallCmd = &cobra.Command{
	Use:   "install --name=<name> --event=<event> --command=<cmd>",
	Short: "Install a verification hook",
	Long: `Install a verification hook to the registry.

Example:
  engram hook install --name=test-execution --event=session-completion \
    --command=bow-core --args=test --priority=1 --timeout=300`,
	RunE: runHookInstall,
}

var hookUninstallCmd = &cobra.Command{
	Use:   "uninstall <hook-name>",
	Short: "Uninstall a verification hook",
	Long: `Remove a verification hook from the registry.

Example:
  engram hook uninstall test-execution`,
	Args: cobra.ExactArgs(1),
	RunE: runHookUninstall,
}

var hookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered verification hooks",
	Long:  `Display all registered hooks grouped by event type.`,
	RunE:  runHookList,
}

// Flags for hook install
var (
	hookName     string
	hookEvent    string
	hookCommand  string
	hookArgs     []string
	hookPriority int
	hookTimeout  int
	hookType     string
)

func init() {
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookInstallCmd)
	hookCmd.AddCommand(hookUninstallCmd)
	hookCmd.AddCommand(hookListCmd)

	// Install flags
	hookInstallCmd.Flags().StringVar(&hookName, "name", "", "Hook name (required)")
	hookInstallCmd.Flags().StringVar(&hookEvent, "event", "", "Event type: session-completion, phase-completion, pre-commit (required)")
	hookInstallCmd.Flags().StringVar(&hookCommand, "command", "", "Command to execute (required)")
	hookInstallCmd.Flags().StringSliceVar(&hookArgs, "args", []string{}, "Command arguments")
	hookInstallCmd.Flags().IntVar(&hookPriority, "priority", 50, "Priority (1-100, higher runs first)")
	hookInstallCmd.Flags().IntVar(&hookTimeout, "timeout", 60, "Timeout in seconds")
	hookInstallCmd.Flags().StringVar(&hookType, "type", "binary", "Hook type: binary, skill, script")

	hookInstallCmd.MarkFlagRequired("name")
	hookInstallCmd.MarkFlagRequired("event")
	hookInstallCmd.MarkFlagRequired("command")
}

func runHookInstall(cmd *cobra.Command, args []string) error {
	// Validate inputs
	if err := validateHookConfig(); err != nil {
		return fmt.Errorf("invalid hook configuration: %w", err)
	}

	// Calculate command hash
	commandHash, err := calculateCommandHash(hookCommand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Could not calculate command hash: %v\n", err)
		commandHash = "" // Proceed without hash
	}

	// Load existing registry
	registry, registryPath, err := loadHookRegistry()
	if err != nil {
		return fmt.Errorf("failed to load hook registry: %w", err)
	}

	// Check for duplicate name
	for _, h := range registry.Hooks {
		if h.Name == hookName {
			return fmt.Errorf("hook '%s' already exists, uninstall it first", hookName)
		}
	}

	// Add new hook
	newHook := Hook{
		Name:        hookName,
		Event:       hookEvent,
		Priority:    hookPriority,
		Type:        hookType,
		Command:     hookCommand,
		Args:        hookArgs,
		Timeout:     hookTimeout,
		CommandHash: commandHash,
	}
	registry.Hooks = append(registry.Hooks, newHook)

	// Save registry
	if err := saveHookRegistry(registry, registryPath); err != nil {
		return fmt.Errorf("failed to save hook registry: %w", err)
	}

	fmt.Printf("✓ Hook '%s' installed successfully\n", hookName)
	if commandHash != "" {
		fmt.Printf("  Command hash: %s\n", commandHash)
	}
	return nil
}

func runHookUninstall(cmd *cobra.Command, args []string) error {
	hookName := args[0]

	// Load existing registry
	registry, registryPath, err := loadHookRegistry()
	if err != nil {
		return fmt.Errorf("failed to load hook registry: %w", err)
	}

	// Find and remove hook
	found := false
	newHooks := make([]Hook, 0, len(registry.Hooks))
	for _, h := range registry.Hooks {
		if h.Name == hookName {
			found = true
			continue // Skip this hook
		}
		newHooks = append(newHooks, h)
	}

	if !found {
		return fmt.Errorf("hook '%s' not found", hookName)
	}

	registry.Hooks = newHooks

	// Save registry
	if err := saveHookRegistry(registry, registryPath); err != nil {
		return fmt.Errorf("failed to save hook registry: %w", err)
	}

	fmt.Printf("✓ Hook '%s' uninstalled successfully\n", hookName)
	return nil
}

func runHookList(cmd *cobra.Command, args []string) error {
	registry, _, err := loadHookRegistry()
	if err != nil {
		return fmt.Errorf("failed to load hook registry: %w", err)
	}

	if len(registry.Hooks) == 0 {
		fmt.Println("No hooks registered.")
		fmt.Println("Run 'engram hook install' to add hooks.")
		return nil
	}

	// Group by event
	eventGroups := make(map[string][]Hook)
	for _, h := range registry.Hooks {
		eventGroups[h.Event] = append(eventGroups[h.Event], h)
	}

	// Display grouped hooks
	fmt.Printf("Registered Hooks (%d total):\n\n", len(registry.Hooks))
	for event, hooks := range eventGroups {
		fmt.Printf("Event: %s\n", event)
		for _, h := range hooks {
			fmt.Printf("  - %s (priority=%d, timeout=%ds, type=%s)\n",
				h.Name, h.Priority, h.Timeout, h.Type)
			fmt.Printf("    Command: %s %v\n", h.Command, h.Args)
			if h.CommandHash != "" {
				fmt.Printf("    Hash: %s\n", h.CommandHash)
			}
		}
		fmt.Println()
	}

	return nil
}

func validateHookConfig() error {
	// Validate name
	if hookName == "" {
		return fmt.Errorf("hook name is required")
	}

	// Validate event
	validEvents := map[string]bool{
		"session-completion": true,
		"phase-completion":   true,
		"pre-commit":         true,
	}
	if !validEvents[hookEvent] {
		return fmt.Errorf("invalid event '%s', must be one of: session-completion, phase-completion, pre-commit", hookEvent)
	}

	// Validate priority
	if hookPriority < 1 || hookPriority > 100 {
		return fmt.Errorf("priority must be between 1 and 100")
	}

	// Validate timeout
	if hookTimeout < 1 || hookTimeout > 600 {
		return fmt.Errorf("timeout must be between 1 and 600 seconds")
	}

	// Validate type
	validTypes := map[string]bool{
		"binary": true,
		"skill":  true,
		"script": true,
	}
	if !validTypes[hookType] {
		return fmt.Errorf("invalid type '%s', must be one of: binary, skill, script", hookType)
	}

	return nil
}

func calculateCommandHash(command string) (string, error) {
	// For skills, don't calculate hash
	if hookType == "skill" {
		return "", nil
	}

	// Find command binary path
	cmdPath, err := exec.LookPath(command)
	if err != nil {
		return "", fmt.Errorf("command not found: %w", err)
	}

	// Calculate SHA-256 hash
	f, err := os.Open(cmdPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func loadHookRegistry() (*HookRegistry, string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}

	engramDir := filepath.Join(homeDir, ".engram")
	registryPath := filepath.Join(engramDir, "hooks.toml")

	// Create .engram directory if it doesn't exist
	if err := os.MkdirAll(engramDir, 0755); err != nil {
		return nil, "", err
	}

	registry := &HookRegistry{}

	// If file doesn't exist, return empty registry
	if _, err := os.Stat(registryPath); os.IsNotExist(err) {
		return registry, registryPath, nil
	}

	// Load existing registry
	if _, err := toml.DecodeFile(registryPath, registry); err != nil {
		return nil, "", fmt.Errorf("failed to parse hooks.toml: %w", err)
	}

	return registry, registryPath, nil
}

func saveHookRegistry(registry *HookRegistry, path string) error {
	// Create temporary file for atomic write
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}

	// Encode TOML
	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(registry); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return err
	}

	return nil
}
