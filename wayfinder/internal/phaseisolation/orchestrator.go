package phaseisolation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// PhaseOrchestrator manages Wayfinder workflow execution with phase isolation.
type PhaseOrchestrator struct {
	config          OrchestratorConfig
	artifacts       map[PhaseID]*PhaseArtifact
	contextCompiler *ContextCompiler
	strategy        PhaseExecutionStrategy
	sectionParser   *SectionParser
	scopeValidator  *ScopeValidator
}

// PhaseExecutionOptions controls phase execution behavior.
type PhaseExecutionOptions struct {
	OverrideValidation bool
}

// ValidationError is returned when scope validation fails.
type ValidationError struct {
	Message          string
	ValidationResult ValidationResult
}

func (e *ValidationError) Error() string {
	return e.Message
}

// NewPhaseOrchestrator creates a new PhaseOrchestrator.
func NewPhaseOrchestrator(config OrchestratorConfig, strategy PhaseExecutionStrategy) *PhaseOrchestrator {
	parser := NewSectionParser()
	return &PhaseOrchestrator{
		config:          config,
		artifacts:       make(map[PhaseID]*PhaseArtifact),
		contextCompiler: NewContextCompiler(),
		strategy:        strategy,
		sectionParser:   parser,
		scopeValidator:  NewScopeValidator(parser),
	}
}

// ExecuteWorkflow executes the full Wayfinder workflow with phase isolation.
// Deprecated: Use ExecuteWorkflowCtx for OTel span propagation.
func (po *PhaseOrchestrator) ExecuteWorkflow() (*WorkflowResult, error) {
	return po.ExecuteWorkflowCtx(context.Background())
}

// ExecuteWorkflowCtx executes the full Wayfinder workflow with phase isolation.
// Creates a root "wayfinder_workflow" span and per-phase "wayfinder_phase" child spans.
func (po *PhaseOrchestrator) ExecuteWorkflowCtx(ctx context.Context) (*WorkflowResult, error) {
	tracer := otel.Tracer("engram/wayfinder")
	ctx, rootSpan := tracer.Start(ctx, "wayfinder_workflow",
		trace.WithAttributes(
			attribute.String("session.id", po.config.SessionID),
			attribute.String("project.path", po.config.ProjectPath),
		))
	defer rootSpan.End()

	startTime := time.Now()

	var phases []PhaseDefinition
	if po.config.StartPhase != "" {
		phases = GetPhasesFrom(po.config.StartPhase)
	} else {
		phases = GetAllPhases()
	}

	rootSpan.SetAttributes(attribute.Int("workflow.phase_count", len(phases)))

	var results []PhaseResult

	fmt.Printf("Starting Wayfinder workflow: %d phases\n", len(phases))
	fmt.Printf("Project: %s\n", po.config.ProjectPath)
	fmt.Printf("Session: %s\n\n", po.config.SessionID)

	for _, phase := range phases {
		fmt.Println(strings.Repeat("\u2501", 40))
		fmt.Printf("Phase %s: %s\n", phase.ID, phase.Name)
		fmt.Println(strings.Repeat("\u2501", 40))

		phaseStart := time.Now()

		artifact, err := po.ExecutePhaseCtx(ctx, phase, nil)
		if err != nil {
			duration := time.Since(phaseStart).Milliseconds()
			results = append(results, PhaseResult{
				PhaseID:  phase.ID,
				Status:   StatusFailed,
				Error:    err.Error(),
				Duration: duration,
			})
			fmt.Printf("Phase %s failed: %s\n\n", phase.ID, err.Error())
			break
		}

		po.artifacts[phase.ID] = artifact
		duration := time.Since(phaseStart).Milliseconds()

		results = append(results, PhaseResult{
			PhaseID:    phase.ID,
			Status:     StatusCompleted,
			Artifact:   artifact,
			TokenCount: artifact.Metadata.TokenCount,
			Duration:   duration,
		})

		fmt.Printf("Phase %s completed in %dms\n", phase.ID, duration)
		fmt.Printf("  Tokens: %d\n\n", artifact.Metadata.TokenCount)
	}

	totalDuration := time.Since(startTime).Milliseconds()
	totalTokens := calculateTotalTokens(results)
	tokenSavings := calculateSavings(results)

	rootSpan.SetAttributes(
		attribute.Int("workflow.total_tokens", totalTokens),
		attribute.Int("workflow.token_savings_pct", tokenSavings),
		attribute.Int64("workflow.duration_ms", totalDuration),
	)

	return &WorkflowResult{
		SessionID:     po.config.SessionID,
		Results:       results,
		TotalTokens:   totalTokens,
		TokenSavings:  tokenSavings,
		TotalDuration: totalDuration,
	}, nil
}

// ExecutePhase executes a single phase.
// Deprecated: Use ExecutePhaseCtx for OTel span propagation.
func (po *PhaseOrchestrator) ExecutePhase(phase PhaseDefinition, opts *PhaseExecutionOptions) (*PhaseArtifact, error) {
	return po.ExecutePhaseCtx(context.Background(), phase, opts)
}

// ExecutePhaseCtx executes a single phase with a "wayfinder_phase" span.
func (po *PhaseOrchestrator) ExecutePhaseCtx(ctx context.Context, phase PhaseDefinition, opts *PhaseExecutionOptions) (*PhaseArtifact, error) {
	tracer := otel.Tracer("engram/wayfinder")
	_, span := tracer.Start(ctx, "wayfinder_phase",
		trace.WithAttributes(
			attribute.String("phase", string(phase.ID)),
			attribute.String("phase.name", phase.Name),
		))
	defer span.End()

	fmt.Printf("Compiling context for %s...\n", phase.ID)

	phaseContext := po.contextCompiler.Compile(phase, po.artifacts, po.config.SessionID, po.config.ProjectPath)

	fmt.Printf("Context compiled: ~%d tokens\n", phaseContext.Metadata.EstimatedTokens)
	fmt.Printf("Budget: %d tokens\n", phaseContext.Metadata.TokenBudget)
	fmt.Printf("Dependencies: %d prior phases\n\n", len(phaseContext.PriorArtifacts))

	span.SetAttributes(
		attribute.Int("phase.estimated_tokens", phaseContext.Metadata.EstimatedTokens),
		attribute.Int("phase.token_budget", phaseContext.Metadata.TokenBudget),
	)

	artifact, err := po.strategy.ExecutePhase(phase, phaseContext, po.config)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if opts == nil || !opts.OverrideValidation {
		if err := po.validatePhaseScope(phase, artifact); err != nil {
			span.RecordError(err)
			return nil, err
		}
	}

	span.SetAttributes(attribute.Int("phase.token_count", artifact.Metadata.TokenCount))

	return artifact, nil
}

func (po *PhaseOrchestrator) validatePhaseScope(phase PhaseDefinition, artifact *PhaseArtifact) error {
	content, err := os.ReadFile(artifact.FullPath)
	if err != nil {
		fmt.Printf("Warning: Could not read artifact for validation: %v\n", err)
		return nil
	}

	result := po.scopeValidator.Validate(phase.ID, string(content))
	fmt.Println(po.scopeValidator.FormatReport(result))

	if !result.Passed {
		return &ValidationError{
			Message:          fmt.Sprintf("Scope validation failed for %s", phase.ID),
			ValidationResult: result,
		}
	}

	if len(result.Warnings) > 0 {
		fmt.Printf("Warning: %d warning(s) detected\n", len(result.Warnings))
	}

	return nil
}

// LoadExistingArtifacts loads existing artifacts for resuming a workflow.
func (po *PhaseOrchestrator) LoadExistingArtifacts(phases []PhaseID) {
	fmt.Printf("Loading %d existing artifacts...\n", len(phases))

	for _, phaseID := range phases {
		phase := PhaseDefinitions[phaseID]
		artifactPath := filepath.Join(po.config.ProjectPath, phase.Deliverable)

		content, err := os.ReadFile(artifactPath)
		if err != nil {
			fmt.Printf("  Warning: Could not load %s: %v\n", phaseID, err)
			continue
		}

		summary := po.contextCompiler.SummarizeArtifact(phaseID, string(content))

		artifact := &PhaseArtifact{
			PhaseID:  phaseID,
			Summary:  summary,
			FullPath: artifactPath,
			Metadata: ArtifactMetadata{
				Timestamp:  time.Now().UnixMilli(),
				TokenCount: (len(content) + 3) / 4,
			},
		}

		po.artifacts[phaseID] = artifact
		fmt.Printf("  Loaded %s: %s\n", phaseID, phase.Name)
	}

	fmt.Println()
}

// GetArtifacts returns current artifacts.
func (po *PhaseOrchestrator) GetArtifacts() map[PhaseID]*PhaseArtifact {
	result := make(map[PhaseID]*PhaseArtifact, len(po.artifacts))
	for k, v := range po.artifacts {
		result[k] = v
	}
	return result
}

// GenerateReport generates a workflow report.
func (po *PhaseOrchestrator) GenerateReport(result *WorkflowResult) string {
	var lines []string

	lines = append(lines, "# Wayfinder Workflow Report")
	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("**Session ID**: %s", result.SessionID))
	lines = append(lines, fmt.Sprintf("**Duration**: %dms (%ds)", result.TotalDuration, result.TotalDuration/1000))
	lines = append(lines, fmt.Sprintf("**Total Tokens**: %d", result.TotalTokens))
	lines = append(lines, fmt.Sprintf("**Token Savings**: %d%%", result.TokenSavings))
	lines = append(lines, "")
	lines = append(lines, "## Phase Results")
	lines = append(lines, "")
	lines = append(lines, "| Phase | Status | Tokens | Duration (ms) |")
	lines = append(lines, "|-------|--------|--------|---------------|")

	for _, pr := range result.Results {
		phase := PhaseDefinitions[pr.PhaseID]
		status := "\u2717" // ✗
		if pr.Status == StatusCompleted {
			status = "\u2713" // ✓
		}
		tokens := "-"
		if pr.TokenCount > 0 {
			tokens = fmt.Sprintf("%d", pr.TokenCount)
		}
		duration := "-"
		if pr.Duration > 0 {
			duration = fmt.Sprintf("%d", pr.Duration)
		}
		lines = append(lines, fmt.Sprintf("| %s: %s | %s | %s | %s |", phase.ID, phase.Name, status, tokens, duration))
	}
	lines = append(lines, "")

	var failures []PhaseResult
	for _, r := range result.Results {
		if r.Status == StatusFailed {
			failures = append(failures, r)
		}
	}

	if len(failures) > 0 {
		lines = append(lines, "## Failures")
		lines = append(lines, "")
		for _, f := range failures {
			phase := PhaseDefinitions[f.PhaseID]
			lines = append(lines, fmt.Sprintf("**%s: %s**", phase.ID, phase.Name))
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("Error: %s", f.Error))
			lines = append(lines, "")
		}
	} else {
		lines = append(lines, "## Summary")
		lines = append(lines, "")
		lines = append(lines, "All phases completed successfully")
		lines = append(lines, fmt.Sprintf("%d%% token reduction achieved", result.TokenSavings))
		lines = append(lines, "Workflow execution complete")
	}

	return strings.Join(lines, "\n")
}

func calculateTotalTokens(results []PhaseResult) int {
	total := 0
	for _, r := range results {
		total += r.TokenCount
	}
	return total
}

func calculateSavings(results []PhaseResult) int {
	baselineTokens := map[PhaseID]int{
		PhaseD1: 1000, PhaseD2: 1500, PhaseD3: 2000, PhaseD4: 2500,
		PhaseS4: 3500, PhaseS5: 5000, PhaseS6: 7000, PhaseS7: 10000,
		PhaseS8: 12000, PhaseS9: 15000, PhaseS10: 18000, PhaseS11: 20000,
	}

	var baselineTotal, actualTotal int
	for _, r := range results {
		if r.Status == StatusCompleted && r.TokenCount > 0 {
			baselineTotal += baselineTokens[r.PhaseID]
			actualTotal += r.TokenCount
		}
	}

	if baselineTotal == 0 {
		return 0
	}

	return (baselineTotal - actualTotal) * 100 / baselineTotal
}
