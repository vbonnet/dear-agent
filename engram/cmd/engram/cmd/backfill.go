package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
)

var backfillCmd = &cobra.Command{
	Use:   "backfill",
	Short: "Generate missing documentation for existing projects",
	Long: `Backfill commands generate documentation for projects that lack proper specs,
architecture documentation, or ADRs.

Uses hybrid analysis (codebase scanning + LLM synthesis) to create accurate documentation.

COMMANDS
  backfill-spec         - Generate SPEC.md from codebase analysis
  backfill-architecture - Generate ARCHITECTURE.md from codebase analysis
  backfill-adrs         - Generate ADR.md files from git history and code

EXAMPLES
  # Generate SPEC.md for a project
  $ engram backfill-spec --project-dir ~/my-project

  # Generate ARCHITECTURE.md
  $ engram backfill-architecture --project-dir ~/my-project

  # Generate ADRs
  $ engram backfill-adrs --project-dir ~/my-project

REQUIREMENTS
  - Python 3.9+ installed
  - Documentation skills available in engram skills directory
  - ANTHROPIC_API_KEY environment variable (for LLM synthesis)`,
}

var backfillSpecCmd = &cobra.Command{
	Use:   "backfill-spec",
	Short: "Generate SPEC.md from codebase analysis",
	Long: `Analyzes a project's codebase to generate a comprehensive SPEC.md file.

Uses hybrid analysis:
- README scanning
- Git history analysis
- Dependency analysis
- Code structure analysis
- Test file analysis

Then synthesizes findings into a structured SPEC.md with:
- Vision & Goals
- Critical User Journeys
- Scope (In/Out)
- Success Metrics`,
	RunE: runBackfillSpec,
}

var backfillArchitectureCmd = &cobra.Command{
	Use:   "backfill-architecture",
	Short: "Generate ARCHITECTURE.md from codebase analysis",
	Long: `Analyzes a project's codebase to generate ARCHITECTURE.md.

Extracts:
- Component structure
- Dependencies
- Interfaces
- Data flow
- Deployment model`,
	RunE: runBackfillArchitecture,
}

var backfillADRsCmd = &cobra.Command{
	Use:   "backfill-adrs",
	Short: "Generate ADR files from git history and code",
	Long: `Analyzes git history and codebase to generate Architecture Decision Records.

Identifies:
- Major architectural changes
- Technology choices
- Design patterns
- Trade-offs made`,
	RunE: runBackfillADRs,
}

var (
	projectDir string
)

func init() {
	rootCmd.AddCommand(backfillCmd)
	backfillCmd.AddCommand(backfillSpecCmd)
	backfillCmd.AddCommand(backfillArchitectureCmd)
	backfillCmd.AddCommand(backfillADRsCmd)

	// Add flags
	backfillSpecCmd.Flags().StringVar(&projectDir, "project-dir", ".", "Project directory to analyze")
	backfillArchitectureCmd.Flags().StringVar(&projectDir, "project-dir", ".", "Project directory to analyze")
	backfillADRsCmd.Flags().StringVar(&projectDir, "project-dir", ".", "Project directory to analyze")
}

func runBackfillSpec(cmd *cobra.Command, args []string) error {
	return runBackfillSkillPython("backfill-spec", projectDir)
}

func runBackfillArchitecture(cmd *cobra.Command, args []string) error {
	return runBackfillSkillPython("backfill-architecture", projectDir)
}

func runBackfillADRs(cmd *cobra.Command, args []string) error {
	return runBackfillSkillPython("backfill-adrs", projectDir)
}

// runBackfillSkillPython executes a Python-based backfill skill
func runBackfillSkillPython(skillName string, projectPath string) error {
	// Validate project directory exists
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return cli.InvalidInputError("project-dir", projectPath, "must be a valid path")
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return cli.InvalidInputError("project-dir", projectPath, "directory does not exist")
	}

	// Check for API key
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		cli.PrintWarning("ANTHROPIC_API_KEY environment variable not set")
		return fmt.Errorf("ANTHROPIC_API_KEY required for LLM-based backfill")
	}

	// Find skills directory
	skillsDir, err := getSkillsDir()
	if err != nil {
		return fmt.Errorf("failed to locate skills directory: %w", err)
	}

	skillDir := filepath.Join(skillsDir, skillName)

	// Determine script name based on skill type
	var scriptName string
	switch skillName {
	case "backfill-spec":
		scriptName = "backfill_spec.py"
	case "backfill-architecture":
		scriptName = "backfill_architecture.py"
	case "backfill-adrs":
		scriptName = "backfill_adrs.py"
	default:
		return fmt.Errorf("unknown skill: %s", skillName)
	}

	scriptPath := filepath.Join(skillDir, scriptName)

	// Check if script exists
	if _, err := os.Stat(scriptPath); err != nil {
		//nolint:revive // multi-line CLI-facing help text
		return fmt.Errorf("skill script not found: %s\n\nNOTE: Backfill skills are not yet fully implemented.\nThese Wayfinder projects are in progress:\n  - backfill-spec-skill (D4 phase - ~/src/backfill-spec-skill)\n  - backfill-architecture-skill (stub created - ~/src/backfill-architecture-skill)\n  - backfill-adrs-skill (not started - ~/src/backfill-adrs-skill)\n\nThe CLI integration is ready. Python implementations need to be completed and moved to skills/ directory.", scriptPath)
	}

	// Build arguments
	cmdArgs := []string{scriptPath, "--project-dir", absPath}

	// Execute the Python skill
	fmt.Fprintf(os.Stderr, "Running %s on %s...\n", skillName, absPath)

	pythonCmd := exec.Command("python3", cmdArgs...)
	pythonCmd.Stdout = os.Stdout
	pythonCmd.Stderr = os.Stderr
	pythonCmd.Stdin = os.Stdin
	pythonCmd.Env = os.Environ() // Pass through environment variables

	if err := pythonCmd.Run(); err != nil {
		exitErr := &exec.ExitError{}
		if errors.As(err, &exitErr) {
			// Preserve Python script's exit code
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("skill execution failed: %w", err)
	}

	return nil
}

// getSkillsDir returns the path to the engram skills directory.
// It checks relative to the executable first, then falls back to a well-known location.
func getSkillsDir() (string, error) {
	// Try relative to executable
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "..", "skills")
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir, nil
		}
	}

	// Try relative to working directory
	if info, err := os.Stat("skills"); err == nil && info.IsDir() {
		return filepath.Abs("skills")
	}

	// Fall back to home directory location
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	dir := filepath.Join(home, ".engram", "skills")
	return dir, nil
}
