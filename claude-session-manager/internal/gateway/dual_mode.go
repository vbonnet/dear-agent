package gateway

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Mode represents the agent operating mode.
type Mode string

const (
	ModeArchitect    Mode = "architect"   // Planning, design, architecture (Opus)
	ModeImplementer  Mode = "implementer" // Coding, execution, testing (Sonnet/Haiku)
)

// TaskComplexity represents the assessed complexity of a task.
type TaskComplexity string

const (
	ComplexityLow    TaskComplexity = "low"    // Simple, well-defined tasks
	ComplexityMedium TaskComplexity = "medium" // Moderate complexity
	ComplexityHigh   TaskComplexity = "high"   // Complex, requires planning
)

// RoutingDecision represents the gateway's routing choice.
type RoutingDecision struct {
	Mode       Mode           `json:"mode"`        // Which mode to use
	Model      string         `json:"model"`       // Specific model (opus-4.5, sonnet-4.5, haiku-4)
	Complexity TaskComplexity `json:"complexity"`  // Assessed task complexity
	Reasoning  string         `json:"reasoning"`   // Why this routing was chosen
	Confidence float64        `json:"confidence"`  // Confidence in decision (0.0-1.0)
}

// TaskContext contains information about the task for routing.
type TaskContext struct {
	Prompt        string            `json:"prompt"`         // User's prompt
	Project       string            `json:"project"`        // Project context
	PreviousMode  Mode              `json:"previous_mode"`  // Previous mode (for hand-offs)
	Metadata      map[string]string `json:"metadata"`       // Additional context
}

// HandoffContext contains state transfer between modes.
type HandoffContext struct {
	FromMode    Mode              `json:"from_mode"`     // Source mode
	ToMode      Mode              `json:"to_mode"`       // Target mode
	Summary     string            `json:"summary"`       // What was done in source mode
	NextSteps   []string          `json:"next_steps"`    // Tasks for target mode
	Artifacts   map[string]string `json:"artifacts"`     // Files, designs, etc.
	Metadata    map[string]string `json:"metadata"`      // Additional context
	Timestamp   int64             `json:"timestamp"`     // When hand-off occurred
}

// DualModeGateway routes tasks between Architect and Implementer modes.
type DualModeGateway struct {
	defaultMode Mode
}

// NewDualModeGateway creates a new gateway instance.
func NewDualModeGateway(defaultMode Mode) *DualModeGateway {
	if defaultMode == "" {
		defaultMode = ModeImplementer // Default to cost-effective mode
	}

	return &DualModeGateway{
		defaultMode: defaultMode,
	}
}

// RouteTask determines which mode should handle a task.
func (g *DualModeGateway) RouteTask(ctx context.Context, taskCtx *TaskContext) (*RoutingDecision, error) {
	// Assess task complexity
	complexity := g.assessComplexity(taskCtx)

	// Determine mode based on complexity and keywords
	mode := g.determineMode(taskCtx, complexity)

	// Select specific model for the mode
	model := g.selectModel(mode, complexity)

	// Generate routing decision
	decision := &RoutingDecision{
		Mode:       mode,
		Model:      model,
		Complexity: complexity,
		Reasoning:  g.generateReasoning(taskCtx, complexity, mode),
		Confidence: g.calculateConfidence(taskCtx, complexity, mode),
	}

	return decision, nil
}

// CreateHandoff generates a hand-off context for mode transitions.
func (g *DualModeGateway) CreateHandoff(ctx context.Context, fromMode Mode, toMode Mode, summary string, artifacts map[string]string) (*HandoffContext, error) {
	handoff := &HandoffContext{
		FromMode:  fromMode,
		ToMode:    toMode,
		Summary:   summary,
		Artifacts: artifacts,
		Metadata:  make(map[string]string),
		Timestamp: time.Now().Unix(),
	}

	// Generate next steps based on transition
	handoff.NextSteps = g.generateNextSteps(fromMode, toMode, summary)

	return handoff, nil
}

// assessComplexity analyzes the task to determine its complexity level.
func (g *DualModeGateway) assessComplexity(taskCtx *TaskContext) TaskComplexity {
	prompt := strings.ToLower(taskCtx.Prompt)

	// High complexity indicators
	highComplexityKeywords := []string{
		"architect", "design", "plan", "roadmap", "strategy",
		"system design", "architecture", "evaluate", "analyze trade-offs",
		"compare approaches", "research", "investigation",
	}

	for _, keyword := range highComplexityKeywords {
		if strings.Contains(prompt, keyword) {
			return ComplexityHigh
		}
	}

	// Low complexity indicators
	lowComplexityKeywords := []string{
		"fix typo", "add comment", "rename", "delete",
		"format", "lint", "trivial", "simple change",
	}

	for _, keyword := range lowComplexityKeywords {
		if strings.Contains(prompt, keyword) {
			return ComplexityLow
		}
	}

	// Medium complexity (default)
	return ComplexityMedium
}

// determineMode selects the appropriate mode based on task context.
func (g *DualModeGateway) determineMode(taskCtx *TaskContext, complexity TaskComplexity) Mode {
	prompt := strings.ToLower(taskCtx.Prompt)

	// Architect mode triggers
	architectKeywords := []string{
		"design", "architect", "plan", "evaluate", "compare",
		"should i", "which approach", "how should", "strategy",
		"roadmap", "research", "investigate",
	}

	for _, keyword := range architectKeywords {
		if strings.Contains(prompt, keyword) {
			return ModeArchitect
		}
	}

	// Implementer mode triggers
	implementerKeywords := []string{
		"implement", "code", "fix", "test", "debug",
		"write", "create function", "add feature",
	}

	for _, keyword := range implementerKeywords {
		if strings.Contains(prompt, keyword) {
			return ModeImplementer
		}
	}

	// High complexity → Architect by default
	if complexity == ComplexityHigh {
		return ModeArchitect
	}

	// Default to implementer (cost-effective)
	return g.defaultMode
}

// selectModel chooses the specific model for a mode.
func (g *DualModeGateway) selectModel(mode Mode, complexity TaskComplexity) string {
	switch mode {
	case ModeArchitect:
		return "claude-opus-4.5" // Use most capable model for planning

	case ModeImplementer:
		// Use Sonnet for medium/high complexity, Haiku for low
		if complexity == ComplexityLow {
			return "claude-haiku-4" // Fast, cost-effective for simple tasks
		}
		return "claude-sonnet-4.5" // Balance of capability and cost

	default:
		return "claude-sonnet-4.5" // Safe default
	}
}

// generateReasoning explains why this routing decision was made.
func (g *DualModeGateway) generateReasoning(taskCtx *TaskContext, complexity TaskComplexity, mode Mode) string {
	reasons := []string{}

	// Complexity reasoning
	switch complexity {
	case ComplexityHigh:
		reasons = append(reasons, "Task complexity assessed as high")
	case ComplexityMedium:
		reasons = append(reasons, "Task complexity assessed as medium")
	case ComplexityLow:
		reasons = append(reasons, "Task complexity assessed as low")
	}

	// Mode reasoning
	switch mode {
	case ModeArchitect:
		reasons = append(reasons, "Requires planning/design capabilities")
	case ModeImplementer:
		reasons = append(reasons, "Implementation-focused task")
	}

	// Keyword analysis
	prompt := strings.ToLower(taskCtx.Prompt)
	if strings.Contains(prompt, "design") || strings.Contains(prompt, "architect") {
		reasons = append(reasons, "Architecture/design keywords detected")
	}
	if strings.Contains(prompt, "implement") || strings.Contains(prompt, "code") {
		reasons = append(reasons, "Implementation keywords detected")
	}

	return strings.Join(reasons, "; ")
}

// calculateConfidence estimates confidence in the routing decision.
func (g *DualModeGateway) calculateConfidence(taskCtx *TaskContext, complexity TaskComplexity, mode Mode) float64 {
	confidence := 0.5 // Base confidence

	prompt := strings.ToLower(taskCtx.Prompt)

	// High confidence indicators
	if mode == ModeArchitect && (strings.Contains(prompt, "design") || strings.Contains(prompt, "plan")) {
		confidence += 0.3
	}
	if mode == ModeImplementer && (strings.Contains(prompt, "implement") || strings.Contains(prompt, "fix")) {
		confidence += 0.3
	}

	// Complexity-based confidence
	if complexity == ComplexityHigh && mode == ModeArchitect {
		confidence += 0.2
	}
	if complexity == ComplexityLow && mode == ModeImplementer {
		confidence += 0.2
	}

	// Cap at 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// generateNextSteps creates action items for the target mode.
func (g *DualModeGateway) generateNextSteps(fromMode Mode, toMode Mode, summary string) []string {
	if fromMode == ModeArchitect && toMode == ModeImplementer {
		return []string{
			"Review the architecture plan from Architect mode",
			"Implement the designed components",
			"Write tests for the implementation",
			"Validate against requirements",
		}
	}

	if fromMode == ModeImplementer && toMode == ModeArchitect {
		return []string{
			"Review the implementation from Implementer mode",
			"Assess architectural consistency",
			"Identify refactoring opportunities",
			"Plan next iteration",
		}
	}

	return []string{
		fmt.Sprintf("Continue work from %s mode", fromMode),
	}
}
