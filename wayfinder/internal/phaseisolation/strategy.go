package phaseisolation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// FileBasedExecutionStrategy uses the eng-swarm pattern for phase execution.
type FileBasedExecutionStrategy struct {
	platform Platform
}

// NewFileBasedExecutionStrategy creates a new FileBasedExecutionStrategy.
func NewFileBasedExecutionStrategy() *FileBasedExecutionStrategy {
	return &FileBasedExecutionStrategy{
		platform: DetectPlatform(),
	}
}

// ExecutePhase executes a phase using file-based coordination.
func (s *FileBasedExecutionStrategy) ExecutePhase(
	phase PhaseDefinition,
	context PhaseContext,
	config OrchestratorConfig,
) (*PhaseArtifact, error) {
	phaseDir, err := s.createPhaseDirectory(phase, config.ProjectPath)
	if err != nil {
		return nil, fmt.Errorf("create phase directory: %w", err)
	}
	fmt.Printf("  Phase directory: %s\n", phaseDir)

	if err := s.writePhaseInput(phaseDir, context); err != nil {
		s.writeStatus(phaseDir, StatusFailed)
		return nil, fmt.Errorf("write phase input: %w", err)
	}
	fmt.Println("  Phase input written")

	if err := s.writePhasePrompt(phaseDir, phase, context); err != nil {
		s.writeStatus(phaseDir, StatusFailed)
		return nil, fmt.Errorf("write phase prompt: %w", err)
	}
	fmt.Println("  Phase prompt written")

	s.writeStatus(phaseDir, StatusPending)

	if err := s.launchSubAgent(phaseDir, phase, context, config); err != nil {
		s.writeStatus(phaseDir, StatusFailed)
		return nil, err
	}

	if ok := s.monitorCompletion(phaseDir, 30*time.Minute); !ok {
		s.writeStatus(phaseDir, StatusFailed)
		return nil, fmt.Errorf("phase execution timed out or failed")
	}

	artifact, err := s.collectArtifact(phaseDir, phase, context)
	if err != nil {
		s.writeStatus(phaseDir, StatusFailed)
		return nil, err
	}
	fmt.Println("  Artifact collected")

	return artifact, nil
}

func (s *FileBasedExecutionStrategy) createPhaseDirectory(phase PhaseDefinition, projectPath string) (string, error) {
	phaseDir := filepath.Join(projectPath, "phases", string(phase.ID))
	if err := os.MkdirAll(phaseDir, 0o755); err != nil {
		return "", err
	}
	return phaseDir, nil
}

func (s *FileBasedExecutionStrategy) writePhaseInput(phaseDir string, context PhaseContext) error {
	var sections []string

	sections = append(sections, fmt.Sprintf("# Phase Input: %s", context.PhaseName))
	sections = append(sections, "")
	sections = append(sections, "This is your COMPLETE context for this phase execution.")
	sections = append(sections, "You should NOT have access to any prior conversation history.")
	sections = append(sections, "")

	sections = append(sections, "## Objective", "", context.PhaseObjective, "")

	if len(context.PriorArtifacts) > 0 {
		sections = append(sections, "## Context from Prior Phases", "")
		sections = append(sections, "Summaries of previous phases:", "")

		for _, artifact := range context.PriorArtifacts {
			sections = append(sections, fmt.Sprintf("### %s: %s", artifact.PhaseID, artifact.PhaseName))
			sections = append(sections, "", artifact.Summary, "")
			sections = append(sections, fmt.Sprintf("Full deliverable: `%s`", artifact.DeliverablePath), "")
		}
	}

	sections = append(sections, "## Success Criteria", "")
	for _, criterion := range context.SuccessCriteria {
		sections = append(sections, "- "+criterion)
	}
	sections = append(sections, "")

	sections = append(sections, "## Deliverable Specification", "")
	sections = append(sections, fmt.Sprintf("**Filename**: %s", context.OutputSpec.Filename))
	sections = append(sections, fmt.Sprintf("**Format**: %s", context.OutputSpec.Format), "")
	sections = append(sections, "**Required Sections**:")
	for _, section := range context.OutputSpec.Sections {
		sections = append(sections, "- "+section)
	}
	sections = append(sections, "")

	sections = append(sections, "## Metadata", "")
	sections = append(sections, fmt.Sprintf("- Session ID: %s", context.Metadata.SessionID))
	sections = append(sections, fmt.Sprintf("- Token Budget: %d tokens", context.Metadata.TokenBudget))
	sections = append(sections, fmt.Sprintf("- Estimated Input: ~%d tokens", context.Metadata.EstimatedTokens))
	sections = append(sections, "")

	content := strings.Join(sections, "\n")
	return os.WriteFile(filepath.Join(phaseDir, "PHASE-INPUT.md"), []byte(content), 0o644)
}

func (s *FileBasedExecutionStrategy) writePhasePrompt(phaseDir string, phase PhaseDefinition, context PhaseContext) error {
	var sections []string

	sections = append(sections, fmt.Sprintf("# Phase %s: %s", phase.ID, phase.Name))
	sections = append(sections, "", "You are executing this Wayfinder phase in **isolated mode**.", "")

	sections = append(sections, "## Context Isolation", "")
	sections = append(sections, "IMPORTANT: Your context is isolated for this phase:")
	sections = append(sections, "- You do NOT have access to any prior conversation history")
	sections = append(sections, "- Your ONLY context is in PHASE-INPUT.md")
	sections = append(sections, "- Read PHASE-INPUT.md first to understand your objective")
	sections = append(sections, "- You may read prior deliverables if paths are provided", "")

	sections = append(sections, "## Instructions", "")
	sections = append(sections, "1. **Read PHASE-INPUT.md**: This contains your complete context")
	sections = append(sections, "2. **Read prior deliverables**: If you need more detail than the summaries")
	sections = append(sections, "3. **Execute phase objective**: Focus on this phase's specific goal")
	sections = append(sections, "4. **Create deliverable**: Write the output file as specified")
	sections = append(sections, "5. **Mark complete**: Update STATUS.md to \"completed\" when done", "")

	sections = append(sections, "## Deliverable", "")
	sections = append(sections, fmt.Sprintf("Create file: **%s**", context.OutputSpec.Filename), "")
	sections = append(sections, "Location: Place in project root (parent of phases/ directory)", "")

	statusPath := filepath.Join(phaseDir, "STATUS.md")
	sections = append(sections, "## Completion Signal", "")
	sections = append(sections, "When you have completed the deliverable:")
	sections = append(sections, "```bash")
	sections = append(sections, fmt.Sprintf("echo \"completed\" > %s", statusPath))
	sections = append(sections, "```", "")

	sections = append(sections, "## System Prompt", "")
	sections = append(sections, context.PhaseSystemPrompt, "")

	content := strings.Join(sections, "\n")
	return os.WriteFile(filepath.Join(phaseDir, "PHASE-PROMPT.md"), []byte(content), 0o644)
}

func (s *FileBasedExecutionStrategy) writeStatus(phaseDir string, status PhaseStatus) {
	_ = os.WriteFile(filepath.Join(phaseDir, "STATUS.md"), []byte(string(status)), 0o644)
}

func (s *FileBasedExecutionStrategy) readStatus(phaseDir string) PhaseStatus {
	data, err := os.ReadFile(filepath.Join(phaseDir, "STATUS.md"))
	if err != nil {
		return StatusPending
	}
	return PhaseStatus(strings.TrimSpace(string(data)))
}

func (s *FileBasedExecutionStrategy) launchSubAgent(
	phaseDir string,
	phase PhaseDefinition,
	context PhaseContext,
	config OrchestratorConfig,
) error {
	fmt.Printf("\n  Launching sub-agent for %s\n\n", phase.ID)

	s.writeStatus(phaseDir, StatusInProgress)

	if s.platform == PlatformClaudeCode {
		return s.launchClaudeCode(phaseDir, phase, config)
	}
	return s.launchManual(phaseDir, phase, config)
}

func (s *FileBasedExecutionStrategy) launchClaudeCode(
	phaseDir string,
	phase PhaseDefinition,
	config OrchestratorConfig,
) error {
	promptPath := filepath.Join(phaseDir, "PHASE-PROMPT.md")
	inputPath := filepath.Join(phaseDir, "PHASE-INPUT.md")

	prompt := fmt.Sprintf(
		"Execute Wayfinder phase %s: %s\n\nInstructions are in: %s\nContext is in: %s\n\n"+
			"Read both files, then execute the phase objective.\n\n"+
			"IMPORTANT: Your context is isolated for this phase.\n\n"+
			"When complete, update STATUS.md in %s to \"completed\".\n\n"+
			"Begin by reading %s to understand your objective.",
		phase.ID, phase.Name, promptPath, inputPath, phaseDir, inputPath)

	cmd := exec.Command("claude", "--print", "--max-budget-usd", "5", "--", prompt)
	cmd.Dir = config.ProjectPath
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch claude CLI: %w", err)
	}

	fmt.Println("  Sub-agent launched (monitoring STATUS.md for completion)")
	return nil
}

func (s *FileBasedExecutionStrategy) launchManual(
	phaseDir string,
	phase PhaseDefinition,
	config OrchestratorConfig,
) error {
	fmt.Println("  Manual execution required (non-Claude Code platform)")
	fmt.Println()
	fmt.Println("  INSTRUCTIONS:")
	fmt.Printf("  1. Read: %s\n", filepath.Join(phaseDir, "PHASE-INPUT.md"))
	fmt.Printf("  2. Read: %s\n", filepath.Join(phaseDir, "PHASE-PROMPT.md"))
	fmt.Println("  3. Execute the phase objective")
	fmt.Printf("  4. Create deliverable: %s\n", filepath.Join(config.ProjectPath, phase.Deliverable))
	fmt.Printf("  5. Mark complete: echo \"completed\" > %s\n", filepath.Join(phaseDir, "STATUS.md"))
	fmt.Println()
	fmt.Println("  Waiting for STATUS.md to be marked \"completed\"...")
	return nil
}

func (s *FileBasedExecutionStrategy) monitorCompletion(phaseDir string, timeout time.Duration) bool {
	start := time.Now()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			status := s.readStatus(phaseDir)
			if status == StatusCompleted {
				return true
			}
			if status == StatusFailed || status == StatusBlocked {
				return false
			}

			elapsed := time.Since(start)
			if elapsed > timeout {
				return false
			}
			if int(elapsed.Seconds())%30 == 0 && elapsed.Seconds() > 1 {
				fmt.Printf("  Still waiting... (%ds elapsed)\n", int(elapsed.Seconds()))
			}
		}
	}
}

func (s *FileBasedExecutionStrategy) collectArtifact(
	phaseDir string,
	phase PhaseDefinition,
	context PhaseContext,
) (*PhaseArtifact, error) {
	deliverablePath := filepath.Join(context.Metadata.ProjectPath, phase.Deliverable)

	content, err := os.ReadFile(deliverablePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read deliverable %s: %w", deliverablePath, err)
	}

	compiler := NewContextCompiler()
	summary := compiler.SummarizeArtifact(phase.ID, string(content))

	depIDs := make([]PhaseID, 0, len(context.PriorArtifacts))
	for _, a := range context.PriorArtifacts {
		depIDs = append(depIDs, a.PhaseID)
	}

	return &PhaseArtifact{
		PhaseID:  phase.ID,
		Summary:  summary,
		FullPath: deliverablePath,
		Metadata: ArtifactMetadata{
			Dependencies: depIDs,
			Timestamp:    time.Now().UnixMilli(),
			TokenCount:   (len(content) + 3) / 4,
		},
	}, nil
}
