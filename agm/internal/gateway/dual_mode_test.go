package gateway

import (
	"context"
	"testing"
)

func TestNewDualModeGateway(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	if gateway.defaultMode != ModeImplementer {
		t.Errorf("defaultMode = %v, want %v", gateway.defaultMode, ModeImplementer)
	}
}

func TestNewDualModeGateway_DefaultMode(t *testing.T) {
	gateway := NewDualModeGateway("")

	// Should default to Implementer (cost-effective)
	if gateway.defaultMode != ModeImplementer {
		t.Errorf("defaultMode = %v, want %v", gateway.defaultMode, ModeImplementer)
	}
}

func TestScoreComplexity_HighKeywords(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Long prompt with architecture keywords + tools + depth → high score
	taskCtx := &TaskContext{
		Prompt:            "Architect the database schema for a distributed system design with trade-offs evaluation across multiple services. We need to compare approaches and investigate the best architecture patterns for horizontal scaling across data centers.",
		ToolCount:         25,
		ConversationDepth: 25,
	}

	score, _ := gateway.scoreComplexity(taskCtx)

	if score <= 65 {
		t.Errorf("Score = %d, want > 65 for high-complexity task", score)
	}
}

func TestScoreComplexity_LowKeywords(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	taskCtx := &TaskContext{
		Prompt:            "Fix typo in README",
		ToolCount:         0,
		ConversationDepth: 0,
	}

	score, _ := gateway.scoreComplexity(taskCtx)

	if score > 30 {
		t.Errorf("Score = %d, want <= 30 for low-complexity task", score)
	}
}

func TestScoreComplexity_MediumPrompt(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Medium-length prompt, some tools, moderate depth, no strong keywords
	taskCtx := &TaskContext{
		Prompt:            "Add a new API endpoint for user registration with input validation and proper error handling for the REST service",
		ToolCount:         8,
		ConversationDepth: 3,
	}

	score, _ := gateway.scoreComplexity(taskCtx)

	if score <= 10 || score > 65 {
		t.Errorf("Score = %d, want between 10-65 for medium-complexity task", score)
	}
}

func TestScoreToModel(t *testing.T) {
	tests := []struct {
		score int
		model string
	}{
		{0, "claude-haiku-4"},
		{15, "claude-haiku-4"},
		{30, "claude-haiku-4"},
		{31, "claude-sonnet-4.5"},
		{50, "claude-sonnet-4.5"},
		{65, "claude-sonnet-4.5"},
		{66, "claude-opus-4.5"},
		{85, "claude-opus-4.5"},
		{100, "claude-opus-4.5"},
	}

	for _, tt := range tests {
		model := scoreToModel(tt.score)
		if model != tt.model {
			t.Errorf("scoreToModel(%d) = %q, want %q", tt.score, model, tt.model)
		}
	}
}

func TestScoreToComplexity(t *testing.T) {
	tests := []struct {
		score      int
		complexity TaskComplexity
	}{
		{0, ComplexityLow},
		{30, ComplexityLow},
		{31, ComplexityMedium},
		{65, ComplexityMedium},
		{66, ComplexityHigh},
		{100, ComplexityHigh},
	}

	for _, tt := range tests {
		c := scoreToComplexity(tt.score)
		if c != tt.complexity {
			t.Errorf("scoreToComplexity(%d) = %q, want %q", tt.score, c, tt.complexity)
		}
	}
}

func TestRouteTask_ArchitectMode(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	// High-scoring prompt: long, architecture keywords, many tools, deep conversation
	taskCtx := &TaskContext{
		Prompt:            "Design and architect a new distributed caching system with evaluation of trade-offs between Redis and Memcached for our microservices architecture platform. We need to compare approaches and investigate the best system design patterns for horizontal scaling across multiple data centers with careful analysis.",
		ToolCount:         25,
		ConversationDepth: 25,
	}

	decision, err := gateway.RouteTask(ctx, taskCtx)
	if err != nil {
		t.Fatalf("RouteTask failed: %v", err)
	}

	if decision.Mode != ModeArchitect {
		t.Errorf("Mode = %v, want %v (score=%d)", decision.Mode, ModeArchitect, decision.Score)
	}

	if decision.Model != "claude-opus-4.5" {
		t.Errorf("Model = %v, want claude-opus-4.5", decision.Model)
	}

	if decision.Score <= 65 {
		t.Errorf("Score = %d, want > 65 for architect-level task", decision.Score)
	}
}

func TestRouteTask_ImplementerMode(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	tests := []struct {
		name   string
		prompt string
	}{
		{"implement keyword", "Implement the login function"},
		{"fix keyword", "Fix the authentication bug"},
		{"code keyword", "Code the user model"},
		{"debug keyword", "Debug the performance issue"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskCtx := &TaskContext{
				Prompt: tt.prompt,
			}

			decision, err := gateway.RouteTask(ctx, taskCtx)
			if err != nil {
				t.Fatalf("RouteTask failed: %v", err)
			}

			if decision.Mode != ModeImplementer {
				t.Errorf("Mode = %v, want %v (prompt: %q, score=%d)", decision.Mode, ModeImplementer, tt.prompt, decision.Score)
			}
		})
	}
}

func TestRouteTask_ScoreInDecision(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	taskCtx := &TaskContext{
		Prompt: "Fix typo in README",
	}

	decision, err := gateway.RouteTask(ctx, taskCtx)
	if err != nil {
		t.Fatalf("RouteTask failed: %v", err)
	}

	// Score should be present and reasonable
	if decision.Score < 0 || decision.Score > 100 {
		t.Errorf("Score = %d, want 0-100", decision.Score)
	}
}

func TestScoreKeywords(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// High-complexity keywords should score high
	highScore := gateway.scoreKeywords("architect system design with trade-offs")
	if highScore < 15 {
		t.Errorf("High keywords score = %d, want >= 15", highScore)
	}

	// Low-complexity keywords should score low
	lowScore := gateway.scoreKeywords("fix typo rename delete")
	if lowScore > 5 {
		t.Errorf("Low keywords score = %d, want <= 5", lowScore)
	}

	// No keywords
	noScore := gateway.scoreKeywords("hello world")
	if noScore != 0 {
		t.Errorf("No keywords score = %d, want 0", noScore)
	}
}

func TestScoreToConfidence(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Near center of band → higher confidence
	centerConf := gateway.scoreToConfidence(15)
	// Near boundary → lower confidence
	boundaryConf := gateway.scoreToConfidence(30)

	if centerConf <= boundaryConf {
		t.Errorf("Center confidence (%.2f) should be > boundary confidence (%.2f)", centerConf, boundaryConf)
	}

	// Should never exceed 0.95
	maxConf := gateway.scoreToConfidence(50)
	if maxConf > 0.95 {
		t.Errorf("Confidence = %.2f, should not exceed 0.95", maxConf)
	}
}

func TestCreateHandoff(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	summary := "Designed authentication system with OAuth2 flow"
	artifacts := map[string]string{
		"design_doc": "/path/to/design.md",
		"diagram":    "/path/to/architecture.png",
	}

	handoff, err := gateway.CreateHandoff(ctx, ModeArchitect, ModeImplementer, summary, artifacts)
	if err != nil {
		t.Fatalf("CreateHandoff failed: %v", err)
	}

	if handoff.FromMode != ModeArchitect {
		t.Errorf("FromMode = %v, want %v", handoff.FromMode, ModeArchitect)
	}

	if handoff.ToMode != ModeImplementer {
		t.Errorf("ToMode = %v, want %v", handoff.ToMode, ModeImplementer)
	}

	if handoff.Summary != summary {
		t.Errorf("Summary = %q, want %q", handoff.Summary, summary)
	}

	if len(handoff.Artifacts) != 2 {
		t.Errorf("Expected 2 artifacts, got %d", len(handoff.Artifacts))
	}

	if len(handoff.NextSteps) == 0 {
		t.Error("NextSteps should not be empty")
	}

	if handoff.Timestamp == 0 {
		t.Error("Timestamp should be set")
	}
}

func TestGenerateNextSteps_ArchitectToImplementer(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	steps := gateway.generateNextSteps(ModeArchitect, ModeImplementer)

	if len(steps) == 0 {
		t.Error("Expected next steps, got empty list")
	}

	// Should mention implementation
	found := false
	for _, step := range steps {
		if containsIgnoreCase(step, "implement") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Next steps should mention implementation")
	}
}

func TestGenerateNextSteps_ImplementerToArchitect(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	steps := gateway.generateNextSteps(ModeImplementer, ModeArchitect)

	if len(steps) == 0 {
		t.Error("Expected next steps, got empty list")
	}

	// Should mention review or assessment
	found := false
	for _, step := range steps {
		if containsIgnoreCase(step, "review") || containsIgnoreCase(step, "assess") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Next steps should mention review or assessment")
	}
}

func TestRoutingDecision_Reasoning(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	taskCtx := &TaskContext{
		Prompt: "Design a new caching layer",
	}

	decision, err := gateway.RouteTask(ctx, taskCtx)
	if err != nil {
		t.Fatalf("RouteTask failed: %v", err)
	}

	if decision.Reasoning == "" {
		t.Error("Reasoning should not be empty")
	}

	// Reasoning should mention complexity score
	if !containsIgnoreCase(decision.Reasoning, "complexity") {
		t.Errorf("Reasoning should mention complexity: %q", decision.Reasoning)
	}
}

// Helper function
func containsIgnoreCase(s, substr string) bool {
	s = toLower(s)
	substr = toLower(substr)
	return contains(s, substr)
}

func toLower(s string) string {
	result := ""
	for _, c := range s {
		if c >= 'A' && c <= 'Z' {
			result += string(c + 32)
		} else {
			result += string(c)
		}
	}
	return result
}

func contains(s, substr string) bool {
	if len(substr) > len(s) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
