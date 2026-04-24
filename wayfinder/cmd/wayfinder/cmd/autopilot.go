package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/wayfinder/internal/project"
	"github.com/vbonnet/dear-agent/wayfinder/internal/status"
	"github.com/vbonnet/dear-agent/wayfinder/internal/tracker"
)

var (
	autopilotIsolated bool
)

var autopilotCmd = &cobra.Command{
	Use:   "autopilot [project-path]",
	Short: "Execute all Wayfinder phases automatically",
	Long: `Execute all remaining Wayfinder phases automatically in autopilot mode.

This command will:
- Loop through all phases (D1→D2→D3→D4→S4→S5→S6→S7→S8→S9→S10→S11)
- Execute each phase using Claude CLI
- Track progress and completions

Modes:
  Traditional (default)  - Accumulated context across phases
  Isolated (--isolated)  - Phase isolation for token efficiency

Examples:
  wayfinder autopilot                      # Autopilot project in current directory
  wayfinder autopilot ~/src/my-project
  wayfinder autopilot --isolated           # Use phase isolation mode`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAutopilot,
}

func init() {
	autopilotCmd.Flags().BoolVar(&autopilotIsolated, "isolated", false, "Use phase isolation for token efficiency")
	rootCmd.AddCommand(autopilotCmd)
}

func runAutopilot(cmd *cobra.Command, args []string) error {
	// Determine project directory
	var projectPath string
	if len(args) > 0 {
		projectPath = args[0]
	}

	projectDir, err := project.DetectDir(projectPath)
	if err != nil {
		return err
	}

	// Check Claude CLI is available
	if err := checkClaudeCLI(); err != nil {
		return err
	}

	projectName := filepath.Base(projectDir)

	fmt.Printf("🚀 Starting autopilot mode for project: %s\n", projectName)
	fmt.Printf("Project directory: %s\n", projectDir)

	if autopilotIsolated {
		fmt.Printf("Mode: ISOLATED (phase isolation enabled)\n")
		fmt.Printf("Will execute phases with isolated contexts for token efficiency.\n\n")
		return executeIsolatedWorkflow(projectDir)
	}

	fmt.Printf("Mode: TRADITIONAL (accumulated context)\n")
	fmt.Printf("Will execute all remaining phases automatically.\n\n")
	return executeTraditionalWorkflow(projectDir)
}

func executeTraditionalWorkflow(projectDir string) error {
	phaseCount := 0

	for {
		// Read current status
		st, err := status.ReadFrom(projectDir)
		if err != nil {
			return fmt.Errorf("failed to read status: %w", err)
		}

		// Get next phase
		nextPhase, err := st.NextPhase()
		if err != nil {
			if strings.Contains(err.Error(), "already at final phase") {
				fmt.Printf("\n✅ Autopilot complete! All phases finished.\n")
				fmt.Printf("Total phases executed: %d\n", phaseCount)
				return nil
			}
			return fmt.Errorf("error getting next phase: %w", err)
		}

		phaseCount++

		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Phase %d: %s\n", phaseCount, nextPhase)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

		// Initialize tracker
		tr, err := tracker.New(st.SessionID)
		if err != nil {
			return fmt.Errorf("failed to initialize tracker: %w", err)
		}

		// Start the phase
		if err := tr.StartPhase(nextPhase); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to publish phase.started event: %v\n", err)
		}

		// Update status
		st.CurrentPhase = nextPhase
		st.UpdatePhase(nextPhase, status.PhaseStatusInProgress, "")
		if err := st.WriteTo(projectDir); err != nil {
			tr.Close(context.Background())
			return fmt.Errorf("failed to update status: %w", err)
		}

		fmt.Printf("Phase %s started\n", nextPhase)

		// Execute the phase using Claude CLI
		if err := executePhaseWithRetry(nextPhase, projectDir); err != nil {
			tr.Close(context.Background())
			return fmt.Errorf("failed to execute phase %s: %w", nextPhase, err)
		}

		// Complete the phase
		if err := tr.CompletePhase(nextPhase, "success", nil); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to publish phase.completed event: %v\n", err)
		}

		// Update status
		st.UpdatePhase(nextPhase, status.PhaseStatusCompleted, "success")
		if err := st.WriteTo(projectDir); err != nil {
			tr.Close(context.Background())
			return fmt.Errorf("failed to update status: %w", err)
		}

		tr.Close(context.Background())

		fmt.Printf("Phase %s completed\n\n", nextPhase)

		// If we just completed S11, we're done
		if nextPhase == "S11" {
			fmt.Printf("\n✅ Autopilot complete! All phases finished.\n")
			fmt.Printf("Total phases executed: %d\n", phaseCount)
			return nil
		}
	}
}

func executeIsolatedWorkflow(projectDir string) error {
	// Read status to get session ID
	st, err := status.ReadFrom(projectDir)
	if err != nil {
		return fmt.Errorf("failed to read status: %w", err)
	}

	// Determine starting phase
	startPhase := determineStartPhase(projectDir)

	fmt.Printf("Session ID: %s\n", st.SessionID)
	fmt.Printf("Starting from phase: %s\n\n", startPhase)

	// Execute workflow using TypeScript phase orchestrator CLI
	cmd := exec.Command(
		"wayfinder-phase-isolation",
		"execute-workflow",
		"--project-path", projectDir,
		"--session-id", st.SessionID,
		"--start-phase", startPhase,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Dir = projectDir

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}

	fmt.Printf("\n✅ Isolated workflow complete!\n")
	return nil
}

// checkClaudeCLI verifies Claude CLI is available
func checkClaudeCLI() error {
	claudePath, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH\n\n" +
			"Please install Claude CLI from: https://claude.com/claude-code\n" +
			"Autopilot requires Claude CLI to execute phases.")
	}

	cmd := exec.Command(claudePath, "--help")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude CLI found at %s but not working: %w", claudePath, err)
	}

	return nil
}

// executePhaseWithRetry executes phase with retry logic
func executePhaseWithRetry(phase string, projectDir string) error {
	timeout := 30 * time.Minute
	maxRetries := 3

	for attempt := 1; attempt <= maxRetries; attempt++ {
		fmt.Printf("\n▶ Executing phase %s (attempt %d/%d)...\n\n", phase, attempt, maxRetries)

		err := executePhaseWithTimeout(phase, projectDir, timeout)
		if err == nil {
			// Verify deliverable
			if verifyErr := verifyDeliverable(phase, projectDir); verifyErr != nil {
				err = verifyErr
			} else {
				fmt.Printf("\n✅ Phase %s completed successfully\n", phase)
				return nil
			}
		}

		fmt.Fprintf(os.Stderr, "\n⚠️  Phase %s failed: %v\n", phase, err)

		if attempt < maxRetries {
			fmt.Println("\nOptions:")
			fmt.Println("  1. Retry automatically")
			fmt.Println("  2. Complete phase manually and continue")
			fmt.Println("  3. Abort autopilot (Ctrl+C)")
			fmt.Print("\nChoice (1/2): ")

			var choice string
			fmt.Scanln(&choice)

			if choice == "2" {
				fmt.Printf("\nComplete phase %s manually, then press Enter...\n", phase)
				var dummy string
				fmt.Scanln(&dummy)

				if verifyErr := verifyDeliverable(phase, projectDir); verifyErr != nil {
					fmt.Fprintf(os.Stderr, "\n⚠️  Deliverable still not found: %v\n", verifyErr)
					fmt.Println("Please create the deliverable and try again.")
					continue
				}
				fmt.Printf("✅ Phase %s marked as manually completed\n", phase)
				return nil
			}
			fmt.Println("\nRetrying...")
		}
	}

	return fmt.Errorf("phase %s failed after %d attempts", phase, maxRetries)
}

// executePhaseWithTimeout executes phase using Claude CLI with timeout
func executePhaseWithTimeout(phase string, projectDir string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	engramContent, err := loadPhaseEngram(phase)
	if err != nil {
		return err
	}

	prompt := buildPhasePrompt(phase, engramContent, projectDir)

	cmd := exec.CommandContext(ctx, "claude", "--print", "--max-budget-usd", "5", "--", prompt)
	cmd.Dir = projectDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = nil

	err = cmd.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("phase %s timed out after %v", phase, timeout)
		}
		return fmt.Errorf("claude execution failed: %w", err)
	}

	return nil
}

// loadPhaseEngram reads phase engram from disk
func loadPhaseEngram(phase string) (string, error) {
	engramPath := getEngramPath(phase)

	fileInfo, err := os.Stat(engramPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("engram not found: %s", engramPath)
	}
	if err != nil {
		return "", fmt.Errorf("cannot stat engram: %w", err)
	}

	const maxEngramSize = 50 * 1024 // 50KB
	if fileInfo.Size() > maxEngramSize {
		return "", fmt.Errorf("engram too large: %d bytes (max %d bytes)", fileInfo.Size(), maxEngramSize)
	}

	content, err := os.ReadFile(engramPath)
	if err != nil {
		return "", fmt.Errorf("failed to read engram: %w", err)
	}

	return string(content), nil
}

// getEngramPath maps phase to engram file path
func getEngramPath(phase string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/home/user"
	}

	phaseToEngram := map[string]string{
		"D1":  "d1-problem-validation.ai.md",
		"D2":  "d2-existing-solutions.ai.md",
		"D3":  "d3-approach-decision.ai.md",
		"D4":  "d4-solution-requirements.ai.md",
		"S4":  "s4-stakeholder-alignment.ai.md",
		"S5":  "s5-research.ai.md",
		"S6":  "s6-design.ai.md",
		"S7":  "s7-plan.ai.md",
		"S8":  "s8-implementation.ai.md",
		"S9":  "s9-validation.ai.md",
		"S10": "s10-deploy.ai.md",
		"S11": "s11-retrospective.ai.md",
	}

	engramFilename := phaseToEngram[phase]
	engramBase := filepath.Join(homeDir, "src/engram/core/cortex/engrams/workflows")

	return filepath.Join(engramBase, engramFilename)
}

// buildPhasePrompt constructs Claude prompt for phase execution
func buildPhasePrompt(phase string, engramContent string, projectDir string) string {
	deliverable := getDeliverableFilename(phase)
	previousDeliverables := getPreviousDeliverables(phase, projectDir)

	contextNote := ""
	if len(previousDeliverables) > 0 {
		contextNote = fmt.Sprintf("\nPREVIOUS PHASES (read these for context):\n%s\n",
			strings.Join(previousDeliverables, "\n"))
	}

	return fmt.Sprintf(`You are executing Wayfinder phase %s in autopilot mode.

PROJECT DIRECTORY: %s
%s
PHASE METHODOLOGY:
%s

INSTRUCTIONS:
1. Read previous phase deliverables in the project directory for context
2. Follow the phase methodology above
3. Create the deliverable file: %s
4. Work autonomously - make reasonable decisions without asking questions
5. Be thorough but concise - this is one phase of a larger workflow

Begin executing phase %s now.`,
		phase,
		projectDir,
		contextNote,
		engramContent,
		deliverable,
		phase)
}

// getPreviousDeliverables lists deliverable files from previous phases
func getPreviousDeliverables(currentPhase string, projectDir string) []string {
	phaseOrder := []string{"D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	var previous []string
	for _, p := range phaseOrder {
		if p == currentPhase {
			break
		}

		deliverable := getDeliverableFilename(p)
		deliverablePath := filepath.Join(projectDir, deliverable)
		if _, err := os.Stat(deliverablePath); err == nil {
			previous = append(previous, "- "+deliverable)
		}
	}

	return previous
}

// verifyDeliverable verifies phase deliverable was created
func verifyDeliverable(phase string, projectDir string) error {
	deliverable := getDeliverableFilename(phase)
	deliverablePath := filepath.Join(projectDir, deliverable)

	fileInfo, err := os.Stat(deliverablePath)
	if os.IsNotExist(err) {
		return fmt.Errorf("deliverable not created: %s", deliverable)
	}
	if err != nil {
		return fmt.Errorf("cannot stat deliverable: %w", err)
	}

	if fileInfo.Size() == 0 {
		return fmt.Errorf("deliverable is empty: %s", deliverable)
	}

	content, err := os.ReadFile(deliverablePath)
	if err != nil {
		return fmt.Errorf("cannot read deliverable: %w", err)
	}

	if !strings.Contains(string(content), "#") {
		return fmt.Errorf("deliverable appears invalid (no markdown headings): %s", deliverable)
	}

	return nil
}

// getDeliverableFilename maps phase to deliverable filename
func getDeliverableFilename(phase string) string {
	phaseToDeliverable := map[string]string{
		"D1":  "D1-problem-validation.md",
		"D2":  "D2-existing-solutions.md",
		"D3":  "D3-approach-decision.md",
		"D4":  "D4-solution-requirements.md",
		"S4":  "S4-stakeholder-alignment.md",
		"S5":  "S5-research.md",
		"S6":  "S6-design.md",
		"S7":  "S7-plan.md",
		"S8":  "S8-implementation.md",
		"S9":  "S9-validation.md",
		"S10": "S10-deploy.md",
		"S11": "S11-retrospective.md",
	}
	return phaseToDeliverable[phase]
}

// determineStartPhase checks which deliverables exist and returns next phase
func determineStartPhase(projectDir string) string {
	phases := []string{"D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10", "S11"}

	for _, phase := range phases {
		deliverable := getDeliverableFilename(phase)
		deliverablePath := filepath.Join(projectDir, deliverable)

		if _, err := os.Stat(deliverablePath); os.IsNotExist(err) {
			return phase
		}
	}

	return "S11"
}
