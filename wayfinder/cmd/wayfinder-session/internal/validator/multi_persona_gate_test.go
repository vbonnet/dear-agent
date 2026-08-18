package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestLateralThinkingMinimumCount tests that ≥3 approaches are required
func TestLateralThinkingMinimumCount(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
		errorMsg  string
	}{
		{
			name: "0 approaches - should fail",
			content: `# PLAN Design

No approaches listed.
`,
			wantError: true,
			errorMsg:  "found 0 approach(es), need ≥3",
		},
		{
			name: "2 approaches - should fail",
			content: `# PLAN Design

## Approach A: Microservices

**Pros:**
- Scalable

**Cons:**
- Complex

## Approach B: Monolith

**Pros:**
- Simple

**Cons:**
- Limited scaling
`,
			wantError: true,
			errorMsg:  "found 2 approach(es), need ≥3",
		},
		{
			name: "3 approaches - should pass",
			content: `# PLAN Design

## Approach A: Microservices

**Pros:**
- Scalable

**Cons:**
- Complex

## Approach B: Monolith

**Pros:**
- Simple

**Cons:**
- Limited scaling

## Approach C: Serverless

**Pros:**
- No infrastructure

**Cons:**
- Cold starts
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLateralThinkingEnhanced(tt.content, "PLAN", "/tmp/PLAN-design.md")

			if tt.wantError && err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}

// TestLateralThinkingTradeoffValidation tests that each approach has pros/cons
func TestLateralThinkingTradeoffValidation(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
		errorMsg  string
	}{
		{
			name: "Approach missing pros/cons - should fail",
			content: `# PLAN Design

## Approach A: Microservices

This is a microservices architecture.

## Approach B: Monolith

**Pros:**
- Simple

**Cons:**
- Limited

## Approach C: Serverless

**Pros:**
- Scalable

**Cons:**
- Expensive
`,
			wantError: true,
			errorMsg:  "Approach 1 (Microservices) missing tradeoff analysis",
		},
		{
			name: "All approaches have pros/cons - should pass",
			content: `# PLAN Design

## Approach A: Microservices

**Pros:**
- Scalable
- Independent deployment

**Cons:**
- Complex
- Network overhead

## Approach B: Monolith

**Pros:**
- Simple
- Easy debugging

**Cons:**
- Limited scaling

## Approach C: Serverless

**Pros:**
- No infrastructure
- Auto-scaling

**Cons:**
- Cold starts
`,
			wantError: false,
		},
		{
			name: "Approach has tradeoffs section instead of pros/cons - should pass",
			content: `# PLAN Design

## Approach A: Microservices

**Tradeoffs:**
- Scalability vs complexity
- Autonomy vs coordination

## Approach B: Monolith

**Pros:**
- Simple

**Cons:**
- Limited

## Approach C: Serverless

**Advantages:**
- Scalable

**Disadvantages:**
- Expensive
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLateralThinkingEnhanced(tt.content, "PLAN", "/tmp/PLAN-design.md")

			if tt.wantError && err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}

// TestLateralThinkingDistinctness tests that approaches are not too similar
func TestLateralThinkingDistinctness(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantError bool
		errorMsg  string
	}{
		{
			name: "Too similar approaches - should fail",
			content: `# PLAN Design

## Approach A: Microservices with REST API

Split monolith into services. Use REST APIs for communication.
PostgreSQL databases.

**Pros:**
- Scalable

**Cons:**
- Complex

## Approach B: Microservices with GraphQL API

Split monolith into services. Use GraphQL APIs for communication.
PostgreSQL databases.

**Pros:**
- Scalable

**Cons:**
- Complex

## Approach C: Monolith

Single deployment.

**Pros:**
- Simple

**Cons:**
- Limited
`,
			wantError: true,
			errorMsg:  "too similar",
		},
		{
			name: "Distinct approaches - should pass",
			content: `# PLAN Design

## Approach A: Microservices Architecture

Split monolith into 5 independent services. REST APIs for communication.
Each service has own PostgreSQL database.

**Pros:**
- Independent scaling
- Team autonomy

**Cons:**
- Distributed complexity
- Network latency

## Approach B: Modular Monolith

Single deployment unit with clear module boundaries. Domain-driven design.
Shared PostgreSQL database with schema namespaces.

**Pros:**
- Simpler deployment
- Easier debugging

**Cons:**
- Limited scaling granularity
- Shared database bottleneck

## Approach C: Event-Driven Serverless

AWS Lambda functions triggered by events. DynamoDB for data storage.
No servers to manage.

**Pros:**
- Zero infrastructure management
- Automatic scaling

**Cons:**
- Cold start latency
- Vendor lock-in
`,
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLateralThinkingEnhanced(tt.content, "PLAN", "/tmp/PLAN-design.md")

			if tt.wantError && err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}

// TestSimilarityCalculation tests Jaccard similarity calculation
func TestSimilarityCalculation(t *testing.T) {
	tests := []struct {
		name      string
		content1  string
		content2  string
		wantRange [2]float64 // min, max expected similarity
	}{
		{
			name:      "Identical content",
			content1:  "Microservices with REST API and PostgreSQL database",
			content2:  "Microservices with REST API and PostgreSQL database",
			wantRange: [2]float64{0.95, 1.0}, // Should be very high
		},
		{
			name:      "Very similar content",
			content1:  "Microservices with REST API and PostgreSQL database",
			content2:  "Microservices with GraphQL API and PostgreSQL database",
			wantRange: [2]float64{0.7, 0.9}, // High but not identical
		},
		{
			name:      "Somewhat similar content",
			content1:  "Microservices architecture with REST API",
			content2:  "Modular monolith with shared database",
			wantRange: [2]float64{0.0, 0.4}, // Low similarity
		},
		{
			name:      "Completely different content",
			content1:  "Microservices architecture with independent services",
			content2:  "Event-driven serverless AWS Lambda functions",
			wantRange: [2]float64{0.0, 0.2}, // Very low similarity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			similarity := calculateSimilarity(tt.content1, tt.content2)

			if similarity < tt.wantRange[0] || similarity > tt.wantRange[1] {
				t.Errorf("Similarity %.2f not in expected range [%.2f, %.2f]", similarity, tt.wantRange[0], tt.wantRange[1])
			}
		})
	}
}

// TestGateConfiguration tests gate config retrieval
func TestGateConfiguration(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{
		ProjectName:     "test",
		CurrentWaypoint: "PLAN",
	}

	gate := NewMultiPersonaGate(tmpDir, st)

	tests := []struct {
		name                    string
		phase                   string
		expectedTier            GateTier
		expectedPersonaCount    int
		expectedLateralThinking bool
	}{
		{
			name:                    "PLAN gate (Tier 1 Blocking)",
			phase:                   "PLAN",
			expectedTier:            GateTierBlocking,
			expectedPersonaCount:    3, // tech-lead, security-engineer, qa-engineer
			expectedLateralThinking: true,
		},
		{
			name:                    "BUILD gate (Tier 2 Advisory)",
			phase:                   "BUILD",
			expectedTier:            GateTierAdvisory,
			expectedPersonaCount:    2, // qa-engineer, tech-lead
			expectedLateralThinking: false,
		},
		{
			name:                    "SETUP gate (Tier 2 Advisory)",
			phase:                   "SETUP",
			expectedTier:            GateTierAdvisory,
			expectedPersonaCount:    2, // tech-lead, qa-engineer
			expectedLateralThinking: false,
		},
		{
			name:                    "PROBLEM gate (Tier 0 None)",
			phase:                   "PROBLEM",
			expectedTier:            GateTierNone,
			expectedPersonaCount:    0,
			expectedLateralThinking: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := gate.getGateConfig(tt.phase)

			if config.Tier != tt.expectedTier {
				t.Errorf("Expected tier %d, got %d", tt.expectedTier, config.Tier)
			}

			if len(config.RequiredPersonas) != tt.expectedPersonaCount {
				t.Errorf("Expected %d personas, got %d", tt.expectedPersonaCount, len(config.RequiredPersonas))
			}

			if config.LateralThinkingRequired != tt.expectedLateralThinking {
				t.Errorf("Expected LateralThinkingRequired=%v, got %v", tt.expectedLateralThinking, config.LateralThinkingRequired)
			}
		})
	}
}

// TestVoteAggregation tests tier-based voting decision rules
func TestVoteAggregation(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{
		ProjectName:     "test",
		CurrentWaypoint: "PLAN",
	}

	gate := NewMultiPersonaGate(tmpDir, st)

	tests := []struct {
		name           string
		tier           GateTier
		votes          []Vote
		blockers       []string
		requiredCount  int
		expectedStatus string
	}{
		{
			name: "Tier 1 - All GO (PASSED)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "GO", Persona: "security-engineer"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{},
			requiredCount:  3,
			expectedStatus: "PASSED",
		},
		{
			name: "Tier 1 - One NO-GO (BLOCKED)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "NO-GO", Persona: "security-engineer"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{"Missing threat model"},
			requiredCount:  3,
			expectedStatus: "BLOCKED",
		},
		{
			name: "Tier 1 - All ABSTAIN (CONDITIONAL)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "ABSTAIN", Persona: "tech-lead"},
				{Verdict: "ABSTAIN", Persona: "security-engineer"},
				{Verdict: "ABSTAIN", Persona: "qa-engineer"},
			},
			blockers:       []string{},
			requiredCount:  3,
			expectedStatus: "CONDITIONAL",
		},
		{
			// Regression: fail-open bug. A required persona abstaining
			// must NOT lower the GO threshold. With the pre-fix logic
			// `goCount < required-abstainCount` => `2 < 3-1` => false =>
			// the gate incorrectly PASSED on only 2 of 3 approvals.
			name: "Tier 1 - One ABSTAIN must not lower GO threshold (CONDITIONAL)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "ABSTAIN", Persona: "security-engineer"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{},
			requiredCount:  3,
			expectedStatus: "CONDITIONAL",
		},
		{
			// A failed/errored required review records no vote (see
			// executeReview). The missing vote must not be treated as
			// implicit approval: 2 GO of 3 required => not unanimous.
			name: "Tier 1 - Missing vote from failed review (CONDITIONAL)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{},
			requiredCount:  3,
			expectedStatus: "CONDITIONAL",
		},
		{
			// Smaller degenerate case: 2 required, 1 GO + 1 ABSTAIN.
			// Pre-fix: `1 < 2-1` => `1 < 1` => false => wrongly PASSED.
			name: "Tier 1 - Two required, one ABSTAIN (CONDITIONAL)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "ABSTAIN", Persona: "security-engineer"},
			},
			blockers:       []string{},
			requiredCount:  2,
			expectedStatus: "CONDITIONAL",
		},
		{
			// ABSTAIN alongside a NO-GO still hard-blocks (NO-GO wins).
			name: "Tier 1 - NO-GO with an ABSTAIN (BLOCKED)",
			tier: GateTierBlocking,
			votes: []Vote{
				{Verdict: "NO-GO", Persona: "tech-lead"},
				{Verdict: "ABSTAIN", Persona: "security-engineer"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{"Threat model missing"},
			requiredCount:  3,
			expectedStatus: "BLOCKED",
		},
		{
			name: "Tier 2 - Majority GO (PASSED)",
			tier: GateTierAdvisory,
			votes: []Vote{
				{Verdict: "GO", Persona: "tech-lead"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{},
			requiredCount:  2,
			expectedStatus: "PASSED",
		},
		{
			name: "Tier 2 - Majority NO-GO (CAUTION)",
			tier: GateTierAdvisory,
			votes: []Vote{
				{Verdict: "NO-GO", Persona: "tech-lead"},
				{Verdict: "GO", Persona: "qa-engineer"},
			},
			blockers:       []string{"Unrealistic timeline"},
			requiredCount:  2,
			expectedStatus: "CAUTION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &GateConfig{
				Tier:             tt.tier,
				RequiredPersonas: make([]string, tt.requiredCount),
			}

			result := gate.aggregateVotes(tt.votes, tt.blockers, config)

			if result.Status != tt.expectedStatus {
				t.Errorf("Expected status %s, got %s", tt.expectedStatus, result.Status)
			}

			if tt.expectedStatus == "BLOCKED" && len(result.Blockers) != len(tt.blockers) {
				t.Errorf("Expected %d blockers, got %d", len(tt.blockers), len(result.Blockers))
			}
		})
	}
}

// TestDeriveVote_FailClosed is the regression test for the PR #18 gemini
// security P0: an empty/degenerate persona review must NOT default to
// GO/HIGH (fail-open). It must ABSTAIN so the aggregator cannot count it
// as an approval.
func TestDeriveVote_FailClosed(t *testing.T) {
	tests := []struct {
		name           string
		review         personaReview
		wantVerdict    string
		wantConfidence string
		wantSeverity   string
	}{
		{
			// THE bug: zero findings + no rationale used to be GO/HIGH.
			name:           "empty findings, empty summary -> ABSTAIN (degenerate review)",
			review:         personaReview{Name: "security-engineer"},
			wantVerdict:    "ABSTAIN",
			wantConfidence: "LOW",
			wantSeverity:   "HIGH",
		},
		{
			// No over-rotation: a genuine clean review still passes.
			name:           "empty findings, real summary -> GO",
			review:         personaReview{Name: "security-engineer", Summary: "Reviewed thoroughly; no issues found."},
			wantVerdict:    "GO",
			wantConfidence: "HIGH",
			wantSeverity:   "LOW",
		},
		{
			name:           "whitespace-only summary -> still ABSTAIN",
			review:         personaReview{Name: "x", Summary: "   \n\t "},
			wantVerdict:    "ABSTAIN",
			wantConfidence: "LOW",
			wantSeverity:   "HIGH",
		},
		{
			name: "critical finding -> NO-GO",
			review: personaReview{
				Name:     "security-engineer",
				Findings: []personaFinding{{Severity: "critical", Message: "secret in repo"}},
				Summary:  "found a leak",
			},
			wantVerdict:    "NO-GO",
			wantConfidence: "HIGH",
			wantSeverity:   "CRITICAL",
		},
		{
			name: "high finding -> NO-GO",
			review: personaReview{
				Name:     "qa",
				Findings: []personaFinding{{Severity: "HIGH", Message: "no tests"}},
			},
			wantVerdict:    "NO-GO",
			wantConfidence: "MEDIUM",
			wantSeverity:   "HIGH",
		},
		{
			name: "only medium/low findings -> GO",
			review: personaReview{
				Name:     "tech-lead",
				Findings: []personaFinding{{Severity: "medium", Message: "nit"}, {Severity: "low", Message: "style"}},
				Summary:  "minor only",
			},
			wantVerdict:    "GO",
			wantConfidence: "MEDIUM",
			wantSeverity:   "MEDIUM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveVote(tt.review, tt.review.Name, "SPEC")
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %q, want %q", got.Verdict, tt.wantVerdict)
			}
			if got.Confidence != tt.wantConfidence {
				t.Errorf("Confidence = %q, want %q", got.Confidence, tt.wantConfidence)
			}
			if got.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", got.Severity, tt.wantSeverity)
			}
		})
	}
}

// TestDeriveVote_DegenerateReviewCannotPassBlockingGate ties the unit fix
// to the security outcome: a degenerate review (empty findings + empty
// summary) must not let a Tier-1 blocking gate reach PASSED.
func TestDeriveVote_DegenerateReviewCannotPassBlockingGate(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test", CurrentWaypoint: "PLAN"}
	gate := NewMultiPersonaGate(tmpDir, st)

	// Three personas all return degenerate (empty) reviews.
	var votes []Vote
	for _, p := range []string{"tech-lead", "security-engineer", "qa-engineer"} {
		votes = append(votes, *deriveVote(personaReview{Name: p}, p, "SPEC"))
	}

	config := &GateConfig{Tier: GateTierBlocking, RequiredPersonas: make([]string, 3)}
	result := gate.aggregateVotes(votes, nil, config)

	if result.Status == "PASSED" {
		t.Fatalf("fail-open: degenerate reviews PASSED a Tier-1 blocking gate (votes=%+v)", votes)
	}
}

// TestGateEnforcement tests gate enforcement logic
func TestGateEnforcement(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{
		ProjectName:     "test",
		CurrentWaypoint: "PLAN",
	}

	gate := NewMultiPersonaGate(tmpDir, st)

	tests := []struct {
		name      string
		tier      GateTier
		status    string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "Tier 1 PASSED - no error",
			tier:      GateTierBlocking,
			status:    "PASSED",
			wantError: false,
		},
		{
			name:      "Tier 1 BLOCKED - error",
			tier:      GateTierBlocking,
			status:    "BLOCKED",
			wantError: true,
			errorMsg:  "Gate BLOCKED",
		},
		{
			name:      "Tier 1 CONDITIONAL without override - error",
			tier:      GateTierBlocking,
			status:    "CONDITIONAL",
			wantError: true,
			errorMsg:  "Gate CONDITIONAL",
		},
		{
			name:      "Tier 2 CAUTION - no error (warning only)",
			tier:      GateTierAdvisory,
			status:    "CAUTION",
			wantError: false,
		},
		{
			name:      "Tier 0 any status - no error",
			tier:      GateTierNone,
			status:    "BLOCKED",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &GateResult{
				Status:   tt.status,
				Message:  "Test message",
				Votes:    []Vote{},
				Blockers: []string{},
			}

			config := &GateConfig{
				Tier: tt.tier,
			}

			err := gate.enforceGate(result, config)

			if tt.wantError && err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
			}

			if !tt.wantError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}

			if tt.wantError && err != nil && !strings.Contains(err.Error(), tt.errorMsg) {
				t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
			}
		})
	}
}

// TestValidateGateIntegration tests full gate validation flow
func TestValidateGateIntegration(t *testing.T) {
	// Skip test if multi-persona-review CLI not available
	// (Integration testing requires the CLI to be installed)
	t.Skip("Skipping integration test (requires multi-persona-review CLI)")

	tmpDir := t.TempDir()

	// Create PLAN-design.md deliverable
	deliverablePath := filepath.Join(tmpDir, "PLAN-design.md")
	content := `# PLAN Design

## Approach A: Microservices

**Pros:**
- Scalable

**Cons:**
- Complex

## Approach B: Monolith

**Pros:**
- Simple

**Cons:**
- Limited

## Approach C: Serverless

**Pros:**
- No infrastructure

**Cons:**
- Cold starts
`

	if err := os.WriteFile(deliverablePath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test deliverable: %v", err)
	}

	st := &status.StatusV2{
		ProjectName:     "test",
		CurrentWaypoint: "PLAN",
	}

	// Test gate validation (should pass Lateral Thinking, persona review via CLI)
	err := ValidateGate(tmpDir, "PLAN", st)

	// Should pass: Lateral Thinking validates 3 approaches, personas vote via CLI
	if err != nil {
		t.Errorf("Expected no error from gate validation, got: %v", err)
	}
}
