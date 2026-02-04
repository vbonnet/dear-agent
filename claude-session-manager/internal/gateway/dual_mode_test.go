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

func TestRouteTask_ArchitectMode(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)
	ctx := context.Background()

	tests := []struct {
		name   string
		prompt string
	}{
		{"design keyword", "Design a new authentication system"},
		{"architect keyword", "Architect the database schema"},
		{"plan keyword", "Plan the implementation roadmap"},
		{"evaluate keyword", "Evaluate different caching approaches"},
		{"compare keyword", "Compare REST vs GraphQL for our API"},
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

			if decision.Mode != ModeArchitect {
				t.Errorf("Mode = %v, want %v (prompt: %q)", decision.Mode, ModeArchitect, tt.prompt)
			}

			if decision.Model != "claude-opus-4.5" {
				t.Errorf("Model = %v, want claude-opus-4.5", decision.Model)
			}
		})
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
		{"test keyword", "Test the API endpoint"},
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
				t.Errorf("Mode = %v, want %v (prompt: %q)", decision.Mode, ModeImplementer, tt.prompt)
			}

			// Should use Sonnet for medium complexity
			if decision.Model != "claude-sonnet-4.5" {
				t.Errorf("Model = %v, want claude-sonnet-4.5", decision.Model)
			}
		})
	}
}

func TestAssessComplexity_High(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	highComplexityPrompts := []string{
		"Design the system architecture for a microservices platform",
		"Evaluate trade-offs between different database solutions",
		"Research best practices for distributed systems",
		"Architect a scalable caching strategy",
	}

	for _, prompt := range highComplexityPrompts {
		taskCtx := &TaskContext{Prompt: prompt}
		complexity := gateway.assessComplexity(taskCtx)

		if complexity != ComplexityHigh {
			t.Errorf("Complexity = %v, want %v (prompt: %q)", complexity, ComplexityHigh, prompt)
		}
	}
}

func TestAssessComplexity_Low(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	lowComplexityPrompts := []string{
		"Fix typo in README",
		"Add comment to function",
		"Rename variable foo to bar",
		"Delete unused import",
		"Format code with prettier",
	}

	for _, prompt := range lowComplexityPrompts {
		taskCtx := &TaskContext{Prompt: prompt}
		complexity := gateway.assessComplexity(taskCtx)

		if complexity != ComplexityLow {
			t.Errorf("Complexity = %v, want %v (prompt: %q)", complexity, ComplexityLow, prompt)
		}
	}
}

func TestAssessComplexity_Medium(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	mediumComplexityPrompts := []string{
		"Add a new API endpoint for user registration",
		"Refactor the authentication middleware",
		"Update the database schema to include timestamps",
	}

	for _, prompt := range mediumComplexityPrompts {
		taskCtx := &TaskContext{Prompt: prompt}
		complexity := gateway.assessComplexity(taskCtx)

		if complexity != ComplexityMedium {
			t.Errorf("Complexity = %v, want %v (prompt: %q)", complexity, ComplexityMedium, prompt)
		}
	}
}

func TestSelectModel_LowComplexity(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Low complexity → Haiku
	model := gateway.selectModel(ModeImplementer, ComplexityLow)
	if model != "claude-haiku-4" {
		t.Errorf("Model = %v, want claude-haiku-4 for low complexity", model)
	}
}

func TestSelectModel_MediumComplexity(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Medium complexity → Sonnet
	model := gateway.selectModel(ModeImplementer, ComplexityMedium)
	if model != "claude-sonnet-4.5" {
		t.Errorf("Model = %v, want claude-sonnet-4.5 for medium complexity", model)
	}
}

func TestSelectModel_ArchitectMode(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	// Architect mode → Opus (regardless of complexity)
	model := gateway.selectModel(ModeArchitect, ComplexityLow)
	if model != "claude-opus-4.5" {
		t.Errorf("Model = %v, want claude-opus-4.5 for architect mode", model)
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

	steps := gateway.generateNextSteps(ModeArchitect, ModeImplementer, "Design complete")

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

	steps := gateway.generateNextSteps(ModeImplementer, ModeArchitect, "Implementation complete")

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

func TestCalculateConfidence(t *testing.T) {
	gateway := NewDualModeGateway(ModeImplementer)

	tests := []struct {
		name          string
		prompt        string
		complexity    TaskComplexity
		mode          Mode
		minConfidence float64
	}{
		{
			name:          "architect with design keyword",
			prompt:        "Design the authentication system",
			complexity:    ComplexityHigh,
			mode:          ModeArchitect,
			minConfidence: 0.8,
		},
		{
			name:          "implementer with implement keyword",
			prompt:        "Implement the login function",
			complexity:    ComplexityMedium,
			mode:          ModeImplementer,
			minConfidence: 0.8,
		},
		{
			name:          "low confidence",
			prompt:        "Do something",
			complexity:    ComplexityMedium,
			mode:          ModeImplementer,
			minConfidence: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			taskCtx := &TaskContext{Prompt: tt.prompt}
			confidence := gateway.calculateConfidence(taskCtx, tt.complexity, tt.mode)

			if confidence < tt.minConfidence {
				t.Errorf("Confidence = %.2f, want >= %.2f", confidence, tt.minConfidence)
			}

			if confidence > 1.0 {
				t.Errorf("Confidence = %.2f, should not exceed 1.0", confidence)
			}
		})
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

	// Reasoning should mention complexity
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
