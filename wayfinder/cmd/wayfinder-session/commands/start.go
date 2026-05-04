package commands

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/resume"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/tracker"
)

// StartCmd is the cobra command that starts a new Wayfinder session.
var StartCmd = &cobra.Command{
	Use:   "start <project-name>",
	Short: "Start a new Wayfinder session",
	Long: `Start a new Wayfinder session and create WAYFINDER-STATUS.md.

Creates a session tracking file and publishes session.started event.

Flags:
  --force          Skip resume detection and overwrite existing files
  --project-type   Project type: feature, research, infrastructure, refactor, bugfix (default: feature)
  --risk-level     Risk level: XS, S, M, L, XL (default: M)

V2 Schema (9 phases):
  CHARTER  - Intake & Waypoint
  PROBLEM  - Discovery & Context
  RESEARCH - Investigation & Options
  DESIGN   - Architecture & Design Spec
  SPEC     - Solution Requirements (includes Stakeholder Alignment)
  PLAN     - Design (includes Research)
  SETUP    - Planning & Task Breakdown
  BUILD    - BUILD Loop (includes Validation & Deployment)
  RETRO    - Closure & Retrospective

Examples:
  # Start feature project with medium risk
  wayfinder-session start myproject

  # Start research project with low risk
  wayfinder-session start myproject --project-type research --risk-level S

  # Force overwrite
  wayfinder-session start myproject --force`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	// Add --force flag for non-interactive mode (FR6)
	StartCmd.Flags().Bool("force", false, "Skip resume detection and overwrite existing files")
	// Add --project-type flag for V2 schema
	StartCmd.Flags().String("project-type", "feature", "Project type: feature, research, infrastructure, refactor, bugfix")
	// Add --risk-level flag for V2 schema
	StartCmd.Flags().String("risk-level", "M", "Risk level: XS, S, M, L, XL")
	// Add --skip-roadmap flag for V2 schema (for small projects)
	StartCmd.Flags().Bool("skip-roadmap", false, "Skip roadmap.* phases (for small projects <3-4 weeks)")
	// Add --version flag for backward compatibility with old tests
	StartCmd.Flags().String("version", "v2", "Wayfinder version (v2 only, flag kept for backward compatibility)")
}

func runStart(cmd *cobra.Command, args []string) error {
	projectName := args[0]

	// Get project directory
	projectDir := GetProjectDirectory()

	// Check --force flag
	forceFlag, _ := cmd.Flags().GetBool("force")
	if forceFlag {
		fmt.Fprintf(os.Stderr, "⚠️  --force: Skipping resume detection\n")
	} else {
		// Detect resumable directory and handle accordingly
		shouldContinue, err := resume.Detect(projectDir, projectName)
		if err != nil {
			return err
		}
		if !shouldContinue {
			// User chose Abort, or created new session in different location
			return nil
		}
	}

	// Get V2 schema flags
	projectType, _ := cmd.Flags().GetString("project-type")
	riskLevel, _ := cmd.Flags().GetString("risk-level")
	skipRoadmap, _ := cmd.Flags().GetBool("skip-roadmap")

	// Create new V2 status
	st := status.NewStatusV2(projectName, projectType, riskLevel)
	st.SkipRoadmap = skipRoadmap

	// Initialize tracker (use project name as session ID for now)
	sessionID := fmt.Sprintf("session-%d", st.CreatedAt.Unix())
	tr, err := tracker.New(sessionID)
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer tr.Close(context.Background())

	// Publish session.started event
	if err := tr.StartSession(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish session.started event: %v\n", err)
		// Continue anyway - tracking is non-critical
	}

	// Write V2 STATUS file to project directory
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Success message
	fmt.Printf("✅ Wayfinder V2 session started\n")
	fmt.Printf("Project: %s\n", projectName)
	fmt.Printf("Type: %s | Risk: %s\n", projectType, riskLevel)
	fmt.Printf("Current Phase: %s (Intake & Waypoint)\n", st.CurrentWaypoint)
	fmt.Printf("Schema Version: %s\n", st.SchemaVersion)
	fmt.Printf("Created: %s\n\n", status.StatusFilename)
	fmt.Printf("Next steps:\n\n")
	fmt.Printf("Run phases manually:\n")
	fmt.Printf("  wayfinder-session next-phase\n\n")
	fmt.Printf("End session:\n")
	fmt.Printf("  wayfinder-session end --status completed\n")

	return nil
}
