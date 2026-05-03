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
	ModeArchitect   Mode = "architect"   // Planning, design, architecture (Opus)
	ModeImplementer Mode = "implementer" // Coding, execution, testing (Sonnet/Haiku)
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
	Mode       Mode           `json:"mode"`       // Which mode to use
	Model      string         `json:"model"`      // Specific model (opus-4.5, sonnet-4.5, haiku-4)
	Complexity TaskComplexity `json:"complexity"` // Assessed task complexity
	Score      int            `json:"score"`      // Complexity score 0-100
	Reasoning  string         `json:"reasoning"`  // Why this routing was chosen
	Confidence float64        `json:"confidence"` // Confidence in decision (0.0-1.0)
}

// TaskContext contains information about the task for routing.
type TaskContext struct {
	Prompt            string            `json:"prompt"`             // User's prompt
	Project           string            `json:"project"`            // Project context
	PreviousMode      Mode              `json:"previous_mode"`      // Previous mode (for hand-offs)
	ToolCount         int               `json:"tool_count"`         // Number of tools available
	ConversationDepth int               `json:"conversation_depth"` // Number of turns in conversation
	Metadata          map[string]string `json:"metadata"`           // Additional context
}

// HandoffContext contains state transfer between modes.
type HandoffContext struct {
	FromMode  Mode              `json:"from_mode"`  // Source mode
	ToMode    Mode              `json:"to_mode"`    // Target mode
	Summary   string            `json:"summary"`    // What was done in source mode
	NextSteps []string          `json:"next_steps"` // Tasks for target mode
	Artifacts map[string]string `json:"artifacts"`  // Files, designs, etc.
	Metadata  map[string]string `json:"metadata"`   // Additional context
	Timestamp int64             `json:"timestamp"`  // When hand-off occurred
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
	// Score complexity using weighted heuristics
	score, breakdown := g.scoreComplexity(taskCtx)

	// Map score to complexity level
	complexity := scoreToComplexity(score)

	// Determine mode based on score
	mode := g.scoreToDetermineMode(score, taskCtx)

	// Select model based on score thresholds: 0-30 Haiku, 31-65 Sonnet, 66-100 Opus
	model := scoreToModel(score)

	decision := &RoutingDecision{
		Mode:       mode,
		Model:      model,
		Complexity: complexity,
		Score:      score,
		Reasoning:  g.generateScoredReasoning(score, breakdown),
		Confidence: g.scoreToConfidence(score),
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

// complexityBreakdown tracks score contributions from each heuristic dimension.
type complexityBreakdown struct {
	promptLength      int // 0-25: based on prompt character count
	toolCount         int // 0-20: based on number of tools available
	conversationDepth int // 0-20: based on conversation turns
	keywordHints      int // 0-35: based on keyword analysis
}

// scoreComplexity computes a 0-100 complexity score using weighted heuristics.
func (g *DualModeGateway) scoreComplexity(taskCtx *TaskContext) (int, complexityBreakdown) {
	var b complexityBreakdown

	// Dimension 1: Prompt length (0-25 points)
	// Longer prompts tend to indicate more complex tasks
	promptLen := len(taskCtx.Prompt)
	switch {
	case promptLen > 2000:
		b.promptLength = 25
	case promptLen > 1000:
		b.promptLength = 20
	case promptLen > 500:
		b.promptLength = 15
	case promptLen > 200:
		b.promptLength = 10
	case promptLen > 50:
		b.promptLength = 5
	default:
		b.promptLength = 0
	}

	// Dimension 2: Tool count (0-20 points)
	// More tools = more complex environment
	switch {
	case taskCtx.ToolCount > 20:
		b.toolCount = 20
	case taskCtx.ToolCount > 10:
		b.toolCount = 15
	case taskCtx.ToolCount > 5:
		b.toolCount = 10
	case taskCtx.ToolCount > 0:
		b.toolCount = 5
	default:
		b.toolCount = 0
	}

	// Dimension 3: Conversation depth (0-20 points)
	// Deeper conversations indicate ongoing complex work
	switch {
	case taskCtx.ConversationDepth > 20:
		b.conversationDepth = 20
	case taskCtx.ConversationDepth > 10:
		b.conversationDepth = 15
	case taskCtx.ConversationDepth > 5:
		b.conversationDepth = 10
	case taskCtx.ConversationDepth > 0:
		b.conversationDepth = 5
	default:
		b.conversationDepth = 0
	}

	// Dimension 4: Keyword hints (0-35 points)
	// Keywords still matter but are now one dimension among several
	b.keywordHints = g.scoreKeywords(taskCtx.Prompt)

	total := b.promptLength + b.toolCount + b.conversationDepth + b.keywordHints
	if total > 100 {
		total = 100
	}

	return total, b
}

// scoreKeywords evaluates keyword signals and returns 0-35.
func (g *DualModeGateway) scoreKeywords(prompt string) int {
	lower := strings.ToLower(prompt)
	score := 0

	// High complexity keywords (+8 each, max contribution from this group)
	highKeywords := []string{
		"architect", "system design", "architecture", "evaluate",
		"analyze trade-offs", "compare approaches", "investigation",
	}
	for _, kw := range highKeywords {
		if strings.Contains(lower, kw) {
			score += 8
		}
	}

	// Medium complexity keywords (+5 each)
	medKeywords := []string{
		"design", "plan", "roadmap", "strategy", "research",
		"refactor", "migrate", "integrate",
	}
	for _, kw := range medKeywords {
		if strings.Contains(lower, kw) {
			score += 5
		}
	}

	// Low complexity keywords (reduce score)
	lowKeywords := []string{
		"fix typo", "add comment", "rename", "delete",
		"format", "lint", "trivial", "simple change",
	}
	for _, kw := range lowKeywords {
		if strings.Contains(lower, kw) {
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 35 {
		score = 35
	}

	return score
}

// scoreToComplexity maps a 0-100 score to a complexity level.
func scoreToComplexity(score int) TaskComplexity {
	switch {
	case score <= 30:
		return ComplexityLow
	case score <= 65:
		return ComplexityMedium
	default:
		return ComplexityHigh
	}
}

// scoreToModel maps a 0-100 score to a model selection.
// 0-30: Haiku (fast, cheap), 31-65: Sonnet (balanced), 66-100: Opus (most capable)
func scoreToModel(score int) string {
	switch {
	case score <= 30:
		return "claude-haiku-4"
	case score <= 65:
		return "claude-sonnet-4.5"
	default:
		return "claude-opus-4.5"
	}
}

// scoreToDetermineMode maps score to operating mode.
func (g *DualModeGateway) scoreToDetermineMode(score int, taskCtx *TaskContext) Mode {
	// High score → Architect
	if score > 65 {
		return ModeArchitect
	}

	// Check for explicit implementer signals even at medium scores
	lower := strings.ToLower(taskCtx.Prompt)
	implKeywords := []string{"implement", "code", "fix", "test", "debug", "write", "create function", "add feature"}
	for _, kw := range implKeywords {
		if strings.Contains(lower, kw) {
			return ModeImplementer
		}
	}

	// Medium score with no strong signal → default mode
	if score > 30 {
		return g.defaultMode
	}

	// Low score → implementer (simple tasks don't need architect)
	return ModeImplementer
}

// generateScoredReasoning explains the routing decision with score breakdown.
func (g *DualModeGateway) generateScoredReasoning(score int, b complexityBreakdown) string {
	parts := []string{
		fmt.Sprintf("Complexity score: %d/100", score),
		fmt.Sprintf("prompt_length=%d, tool_count=%d, conversation_depth=%d, keywords=%d",
			b.promptLength, b.toolCount, b.conversationDepth, b.keywordHints),
	}

	complexity := scoreToComplexity(score)
	model := scoreToModel(score)
	parts = append(parts, fmt.Sprintf("Mapped to %s complexity → %s", complexity, model))

	return strings.Join(parts, "; ")
}

// scoreToConfidence derives confidence from how clearly the score falls in a band.
func (g *DualModeGateway) scoreToConfidence(score int) float64 {
	// Scores near band boundaries (30, 65) have lower confidence
	// Scores near band centers (15, 48, 83) have higher confidence
	boundaries := []int{0, 30, 65, 100}

	distFromBoundary := 100
	for _, b := range boundaries {
		d := score - b
		if d < 0 {
			d = -d
		}
		if d < distFromBoundary {
			distFromBoundary = d
		}
	}

	// Map distance to confidence: 0 distance = 0.5, 15+ distance = 0.95
	confidence := 0.5 + float64(distFromBoundary)*0.03
	if confidence > 0.95 {
		confidence = 0.95
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
