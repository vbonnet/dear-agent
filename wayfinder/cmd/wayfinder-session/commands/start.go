package commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/internal/override"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/git"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/review"
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

Canonical schema (9 phases):
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
  wayfinder session start myproject

  # Start research project with low risk
  wayfinder session start myproject --project-type research --risk-level S

  # Force overwrite
  wayfinder session start myproject --force`,
	Args: cobra.ExactArgs(1),
	RunE: runStart,
}

func init() {
	StartCmd.Flags().Bool("force", false, "Skip resume detection and overwrite existing files — requires --reason")
	StartCmd.Flags().String("reason", "", "justification for --force, recorded in the override audit log")
	StartCmd.Flags().String("project-type", "feature", "Project type: feature, research, infrastructure, refactor, bugfix")
	StartCmd.Flags().String("risk-level", "M", "Risk level: XS, S, M, L, XL")
	StartCmd.Flags().Bool("skip-roadmap", false, "Skip roadmap.* phases (for small projects <3-4 weeks)")
}

func runStart(cmd *cobra.Command, args []string) error {
	projectName := args[0]
	if strings.TrimSpace(projectName) == "" {
		return fmt.Errorf("project name is required")
	}

	// Get project directory
	projectDir := GetProjectDirectory()
	if !git.New(projectDir).IsGitWorktree() {
		return fmt.Errorf("wayfinder session project directory must be inside a Git work tree: %s", projectDir)
	}

	// Check --force flag
	forceFlag, _ := cmd.Flags().GetBool("force")
	if forceFlag {
		startReason, _ := cmd.Flags().GetString("reason")
		if gerr := override.Require(context.Background(), override.Guard{
			Tool: "wayfinder session start",
			Flag: "--force",
			Gate: "overwrite-existing wayfinder session",
			Risk: override.RiskP2,
		}, startReason); gerr != nil {
			return gerr
		}
		fmt.Fprintf(os.Stderr, "⚠️  --force: Skipping resume detection\n")
	} else {
		statusPath := filepath.Join(projectDir, status.StatusFilename)
		if _, err := os.Stat(statusPath); err == nil {
			if _, parseErr := status.ParseV2(statusPath); parseErr != nil {
				return fmt.Errorf("existing Wayfinder status is not canonical: %w", parseErr)
			}
			return fmt.Errorf("canonical Wayfinder session already exists; use status or next-phase instead of start")
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to inspect existing Wayfinder status: %w", err)
		}
	}

	// Get canonical schema flags.
	projectType, _ := cmd.Flags().GetString("project-type")
	riskLevel, _ := cmd.Flags().GetString("risk-level")
	skipRoadmap, _ := cmd.Flags().GetBool("skip-roadmap")
	if err := validateStartMetadata(projectType, riskLevel); err != nil {
		return err
	}

	// Create canonical status.
	st := status.NewStatusV2(projectName, projectType, riskLevel)
	st.SkipRoadmap = skipRoadmap

	// Resolve the harness profile from the risk level and persist the phases it
	// skips. ProfileLite (XS/S risk) skips DESIGN/SPEC/PLAN so small tasks are not
	// forced through the full 9-phase sequence (ce-12pl / MVD T2.2). Persisting the
	// resolved set keeps the review profile as the single source of truth while the
	// low-level navigation and validation layers stay decoupled from profile logic.
	profile := review.GetProfileConfigForRisk(review.ParseRiskLevel(riskLevel))
	st.SkipPhases = profile.SkipPhases

	// Initialize tracker (use project name as session ID for now)
	sessionID := fmt.Sprintf("session-%d", st.CreatedAt.Unix())
	tr, err := tracker.New(sessionID)
	if err != nil {
		return fmt.Errorf("failed to initialize tracker: %w", err)
	}
	defer func() { _ = tr.Close(context.Background()) }()

	// Publish session.started event
	if err := tr.StartSession(projectDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to publish session.started event: %v\n", err)
		// Continue anyway - tracking is non-critical
	}

	// Write canonical status to the project directory.
	if err := status.WriteV2ToDir(st, projectDir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	// Commit STATUS file so the immediately following `start-phase CHARTER` sees
	// a clean worktree and does not refuse with "uncommitted files detected".
	// This is the bootstrap fix for ce-11fi: wayfinder session start left
	// WAYFINDER-STATUS.md untracked, causing start-phase to fail on brand-new
	// sessions even after the PR #488 auto-commit fix (which only covers
	// subsequent phase transitions, not the initial session creation).
	gitIntegrator := git.New(projectDir)
	if err := gitIntegrator.CommitSessionInit(projectName); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to commit session init: %v\n", err)
	}

	// Success message
	fmt.Printf("✅ Wayfinder session started\n")
	fmt.Printf("Project: %s\n", projectName)
	fmt.Printf("Type: %s | Risk: %s | Profile: %s\n", projectType, riskLevel, profile.Profile)
	if len(st.SkipPhases) > 0 {
		fmt.Printf("Skipping phases (lite profile): %s\n", strings.Join(st.SkipPhases, ", "))
	}
	fmt.Printf("Current Phase: %s (Intake & Waypoint)\n", st.CurrentWaypoint)
	fmt.Printf("Schema Version: %s\n", st.SchemaVersion)
	fmt.Printf("Created: %s\n\n", status.StatusFilename)
	fmt.Printf("Next steps:\n\n")
	fmt.Printf("Run phases manually:\n")
	fmt.Printf("  wayfinder session next-phase\n\n")
	fmt.Printf("End session:\n")
	fmt.Printf("  wayfinder session end --status completed\n")

	return nil
}

func validateStartMetadata(projectType, riskLevel string) error {
	if !slices.Contains(status.ValidProjectTypes(), projectType) {
		return fmt.Errorf("invalid project type %q (valid: %s)", projectType, strings.Join(status.ValidProjectTypes(), ", "))
	}
	if !slices.Contains(status.ValidRiskLevels(), riskLevel) {
		return fmt.Errorf("invalid risk level %q (valid: %s)", riskLevel, strings.Join(status.ValidRiskLevels(), ", "))
	}
	return nil
}
