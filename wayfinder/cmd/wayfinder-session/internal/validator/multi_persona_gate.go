package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// GateTier represents the enforcement level of a gate
type GateTier int

// GateTier values controlling enforcement strictness.
const (
	GateTierNone     GateTier = 0 // No gate (skip review)
	GateTierAdvisory GateTier = 2 // Advisory gate (recommended but non-blocking)
	GateTierBlocking GateTier = 1 // Blocking gate (mandatory approval)
)

// Vote represents a persona's vote on a phase deliverable
type Vote struct {
	Persona         string    `json:"persona"`
	Phase           string    `json:"phase"`
	Timestamp       time.Time `json:"timestamp"`
	Verdict         string    `json:"verdict"`         // GO | NO-GO | ABSTAIN
	Confidence      string    `json:"confidence"`      // HIGH | MEDIUM | LOW
	Severity        string    `json:"severity"`        // CRITICAL | HIGH | MEDIUM | LOW
	Rationale       string    `json:"rationale"`       // Why this verdict
	Blockers        []string  `json:"blockers"`        // Critical issues to resolve
	Recommendations []string  `json:"recommendations"` // Suggestions
}

// GateResult represents the result of a gate check
type GateResult struct {
	Status   string   `json:"status"` // PASSED | BLOCKED | CONDITIONAL | CAUTION
	Message  string   `json:"message"`
	Votes    []Vote   `json:"votes"`
	Blockers []string `json:"blockers"` // Aggregated blockers from NO-GO votes
}

// GateConfig defines gate configuration for phase transitions
type GateConfig struct {
	PhaseTransition         string // e.g., "S6_to_S7"
	Tier                    GateTier
	RequiredPersonas        []string // Must review
	OptionalPersonas        []string // May review based on detection
	LateralThinkingRequired bool
}

// MultiPersonaGate implements Multi-Persona Review Gates
type MultiPersonaGate struct {
	projectDir string
	status     status.StatusInterface
}

// NewMultiPersonaGate creates a new Multi-Persona gate validator
func NewMultiPersonaGate(projectDir string, st status.StatusInterface) *MultiPersonaGate {
	return &MultiPersonaGate{
		projectDir: projectDir,
		status:     st,
	}
}

// ValidateGate checks if a phase transition passes the Multi-Persona Review Gate
// Returns ValidationError if gate is BLOCKED or CONDITIONAL
func ValidateGate(projectDir, phaseName string, st status.StatusInterface) error {
	// Skip multi-persona gate in test environments without API keys or Vertex AI config
	hasAnthropicKey := os.Getenv("ANTHROPIC_API_KEY") != ""
	hasVertexConfig := os.Getenv("ANTHROPIC_VERTEX_PROJECT_ID") != "" &&
		os.Getenv("CLOUD_ML_REGION") != "" &&
		os.Getenv("CLAUDE_CODE_USE_VERTEX") == "1"

	if !hasAnthropicKey && !hasVertexConfig {
		fmt.Fprintf(os.Stderr, "⚠️  Skipping multi-persona gate (no ANTHROPIC_API_KEY or Vertex AI configuration)\n")
		return nil
	}

	gate := NewMultiPersonaGate(projectDir, st)

	// Get gate configuration for this phase
	config := gate.getGateConfig(phaseName)

	// If no gate (Tier 0), skip validation
	if config.Tier == GateTierNone {
		return nil
	}

	// Check if Lateral Thinking required
	if config.LateralThinkingRequired {
		if err := gate.validateLateralThinking(phaseName); err != nil {
			return err
		}
	}

	// Execute Multi-Persona Review
	result, err := gate.executeReview(phaseName, config)
	if err != nil {
		return err
	}

	// Enforce gate decision rules
	return gate.enforceGate(result, config)
}

// getGateConfig returns the gate configuration for a given phase
func (g *MultiPersonaGate) getGateConfig(phaseName string) *GateConfig {
	// Phase-gate mapping (from OSS-EBR-34 spec)
	gateConfigs := map[string]*GateConfig{
		// Tier 1 (Blocking) Gates
		"PLAN": {
			PhaseTransition:         "PLAN_to_SETUP",
			Tier:                    GateTierBlocking,
			RequiredPersonas:        []string{"tech-lead", "security-engineer", "qa-engineer"},
			OptionalPersonas:        []string{"ml-engineer", "fintech-compliance", "data-privacy"},
			LateralThinkingRequired: true,
		},

		// Tier 2 (Advisory) Gates
		"SPEC": {
			PhaseTransition:         "SPEC_to_PLAN",
			Tier:                    GateTierAdvisory,
			RequiredPersonas:        []string{"product-manager", "tech-lead"},
			LateralThinkingRequired: false,
		},
		"SETUP": {
			PhaseTransition:         "SETUP_to_BUILD",
			Tier:                    GateTierAdvisory,
			RequiredPersonas:        []string{"tech-lead", "qa-engineer"},
			LateralThinkingRequired: false,
		},
		"BUILD": {
			PhaseTransition:         "BUILD_to_RETRO",
			Tier:                    GateTierAdvisory,
			RequiredPersonas:        []string{"qa-engineer", "tech-lead"},
			LateralThinkingRequired: false,
		},
	}

	config, exists := gateConfigs[phaseName]
	if !exists {
		// No gate configured for this phase (Tier 0)
		return &GateConfig{Tier: GateTierNone}
	}

	return config
}

// validateLateralThinking checks if deliverable contains ≥3 distinct approaches
func (g *MultiPersonaGate) validateLateralThinking(phaseName string) error {
	// Read phase deliverable
	deliverablePath := filepath.Join(g.projectDir, phaseName+"-"+strings.ToLower(strings.Replace(phaseName, "S", "step", 1))+".md")

	// PLAN → PLAN-design.md
	if phaseName == "PLAN" {
		deliverablePath = filepath.Join(g.projectDir, "PLAN-design.md")
	}

	data, err := os.ReadFile(deliverablePath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to read deliverable: %v", err),
			"Ensure deliverable file exists before completing phase",
		)
	}

	content := string(data)

	// Use enhanced Lateral Thinking validator (checks quality + distinctness)
	return validateLateralThinkingEnhanced(content, phaseName, deliverablePath)
}

// executeReview invokes personas to review the deliverable
func (g *MultiPersonaGate) executeReview(phaseName string, config *GateConfig) (*GateResult, error) {
	// Build deliverable path
	deliverablePath := filepath.Join(g.projectDir, phaseName+"-"+strings.ToLower(strings.Replace(phaseName, "S", "step", 1))+".md")

	// Map phase to deliverable filename
	deliverableMap := map[string]string{
		"SPEC":  "SPEC-solution-requirements.md",
		"PLAN":  "PLAN-design.md",
		"SETUP": "SETUP-plan.md",
		"BUILD": "BUILD-implementation.md",
	}

	if filename, exists := deliverableMap[phaseName]; exists {
		deliverablePath = filepath.Join(g.projectDir, filename)
	}

	// Collect votes from personas
	var votes []Vote
	var blockers []string

	for _, persona := range config.RequiredPersonas {
		vote, err := invokePersonaReview(persona, deliverablePath, phaseName)
		if err != nil {
			// If persona review fails, treat as abstain
			fmt.Fprintf(os.Stderr, "⚠️  Persona %s review failed: %v\n", persona, err)
			continue
		}

		votes = append(votes, *vote)

		// Collect blockers from NO-GO votes
		if vote.Verdict == "NO-GO" && len(vote.Blockers) > 0 {
			blockers = append(blockers, vote.Blockers...)
		}
	}

	// Aggregate votes based on gate tier
	return g.aggregateVotes(votes, blockers, config), nil
}

// enforceGate applies decision rules based on gate tier and votes
func (g *MultiPersonaGate) enforceGate(result *GateResult, config *GateConfig) error {
	switch config.Tier {
	case GateTierBlocking:
		// Tier 1: ALL personas must vote GO
		if result.Status == "BLOCKED" {
			return NewValidationError(
				"gate enforcement",
				"Gate BLOCKED: "+result.Message,
				strings.Join(result.Blockers, "\n")+"\n\nResolve all blockers and re-run complete-phase",
			)
		}
		if result.Status == "CONDITIONAL" {
			return NewValidationError(
				"gate enforcement",
				"Gate CONDITIONAL: "+result.Message,
				"Resolve all concerns before proceeding",
			)
		}

	case GateTierAdvisory:
		// Tier 2: Majority GO recommended (non-blocking warning)
		if result.Status == "CAUTION" {
			fmt.Fprintf(os.Stderr, "⚠️  Gate CAUTION: %s\n", result.Message)
			fmt.Fprintf(os.Stderr, "Recommended: resolve concerns before proceeding.\n")
			// Advisory gates are non-blocking by default (warning only)
		}

	case GateTierNone:
		// No enforcement
		return nil
	}

	return nil
}

// aggregateVotes applies decision rules to collected votes
func (g *MultiPersonaGate) aggregateVotes(votes []Vote, blockers []string, config *GateConfig) *GateResult {
	if len(votes) == 0 {
		// No votes collected (all personas abstained or failed)
		return &GateResult{
			Status:   "CONDITIONAL",
			Message:  "No persona votes collected (review failures)",
			Votes:    votes,
			Blockers: []string{"Unable to collect persona reviews"},
		}
	}

	// Count verdicts
	goCount := 0
	noGoCount := 0
	abstainCount := 0

	for _, vote := range votes {
		switch vote.Verdict {
		case "GO":
			goCount++
		case "NO-GO":
			noGoCount++
		case "ABSTAIN":
			abstainCount++
		}
	}

	// Apply tier-based decision rules
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch config.Tier {
	case GateTierBlocking:
		// Tier 1: ALL personas must vote GO
		if noGoCount > 0 {
			return &GateResult{
				Status:   "BLOCKED",
				Message:  fmt.Sprintf("%d persona(s) voted NO-GO (Tier 1 gate requires all GO)", noGoCount),
				Votes:    votes,
				Blockers: blockers,
			}
		}

		// If no GO votes at all (all abstained or failed), return CONDITIONAL
		if goCount == 0 && len(votes) > 0 {
			return &GateResult{
				Status:   "CONDITIONAL",
				Message:  "No GO votes collected (all abstained or failed)",
				Votes:    votes,
				Blockers: []string{"Not all required personas approved"},
			}
		}

		if goCount < len(config.RequiredPersonas)-abstainCount {
			return &GateResult{
				Status:   "CONDITIONAL",
				Message:  fmt.Sprintf("Insufficient GO votes: %d/%d (excluding %d abstentions)", goCount, len(config.RequiredPersonas), abstainCount),
				Votes:    votes,
				Blockers: []string{"Not all required personas approved"},
			}
		}

		return &GateResult{
			Status:   "PASSED",
			Message:  fmt.Sprintf("All %d persona(s) voted GO", goCount),
			Votes:    votes,
			Blockers: []string{},
		}

	case GateTierAdvisory:
		// Tier 2: Majority GO recommended (tie counts as CAUTION)
		if noGoCount >= goCount && noGoCount > 0 {
			return &GateResult{
				Status:   "CAUTION",
				Message:  fmt.Sprintf("Majority concerns: %d NO-GO vs %d GO", noGoCount, goCount),
				Votes:    votes,
				Blockers: blockers,
			}
		}

		return &GateResult{
			Status:   "PASSED",
			Message:  fmt.Sprintf("Majority approved: %d GO vs %d NO-GO", goCount, noGoCount),
			Votes:    votes,
			Blockers: []string{},
		}
	}

	// Tier 0 (should never reach here, but default to PASSED)
	return &GateResult{
		Status:   "PASSED",
		Message:  "No gate enforcement (Tier 0)",
		Votes:    votes,
		Blockers: []string{},
	}
}

// invokePersonaReview calls multi-persona-review CLI to get persona vote
func invokePersonaReview(persona, deliverablePath, phaseName string) (*Vote, error) {
	// Build command arguments
	args := []string{
		deliverablePath,
		"-p", persona,
		"-f", "json",
		"--no-dedupe",
		"--no-colors",
	}

	// Auto-detect AI provider based on environment variables
	// Priority: VertexAI (VERTEX_PROJECT_ID) > Anthropic (ANTHROPIC_API_KEY)
	vertexProject := os.Getenv("VERTEX_PROJECT_ID")
	vertexLocation := os.Getenv("VERTEX_LOCATION")
	vertexModel := os.Getenv("VERTEX_MODEL")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")

	switch {
	case vertexProject != "":
		// Determine if using Claude via VertexAI or Gemini
		isClaude := vertexModel != "" && strings.Contains(vertexModel, "claude")

		if isClaude {
			// Use Claude via VertexAI Anthropic publisher
			args = append(args, "--provider", "vertexai-claude")
			args = append(args, "--vertex-project", vertexProject)
			if vertexLocation != "" {
				args = append(args, "--vertex-location", vertexLocation)
			} else {
				// Default to us-east5 for Claude (where it's available)
				args = append(args, "--vertex-location", "us-east5")
			}
			args = append(args, "--model", vertexModel)
		} else {
			// Use Gemini via VertexAI
			args = append(args, "--provider", "vertexai")
			args = append(args, "--vertex-project", vertexProject)
			if vertexLocation != "" {
				args = append(args, "--vertex-location", vertexLocation)
			} else {
				// Default to us-central1 for Gemini
				args = append(args, "--vertex-location", "us-central1")
			}
			if vertexModel != "" {
				args = append(args, "--model", vertexModel)
			}
		}
	case anthropicKey != "":
		// Use Anthropic (default provider, no extra flags needed)
		args = append(args, "--provider", "anthropic")
	default:
		// No provider configured - will likely fail, but let multi-persona-review handle the error
		return nil, fmt.Errorf("no AI provider configured: set VERTEX_PROJECT_ID (for VertexAI) or ANTHROPIC_API_KEY (for Anthropic)")
	}

	// Call multi-persona-review CLI
	cmd := exec.Command("multi-persona-review", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("multi-persona-review failed: %w (output: %s)", err, string(output))
	}

	// Parse JSON output
	var review struct {
		Personas []struct {
			Name     string `json:"name"`
			Findings []struct {
				Severity    string   `json:"severity"`
				Message     string   `json:"message"`
				Suggestions []string `json:"suggestions"`
			} `json:"findings"`
			Summary string `json:"summary"`
		} `json:"personas"`
	}

	if err := json.Unmarshal(output, &review); err != nil {
		return nil, fmt.Errorf("failed to parse review JSON: %w", err)
	}

	if len(review.Personas) == 0 {
		return nil, fmt.Errorf("no persona results in review output")
	}

	personaResult := review.Personas[0]

	// Convert findings to vote (verdict/confidence/severity are set per-branch below)
	var verdict, confidence, severity string
	var blockers []string
	var recommendations []string

	// Analyze findings to determine verdict
	criticalCount := 0
	highCount := 0

	for _, finding := range personaResult.Findings {
		switch strings.ToUpper(finding.Severity) {
		case "CRITICAL":
			criticalCount++
			blockers = append(blockers, finding.Message)
		case "HIGH":
			highCount++
			blockers = append(blockers, finding.Message)
		case "MEDIUM":
			recommendations = append(recommendations, finding.Message)
		case "LOW":
			recommendations = append(recommendations, finding.Message)
		}

		// Add suggestions
		recommendations = append(recommendations, finding.Suggestions...)
	}

	// Determine verdict based on findings
	switch {
	case criticalCount > 0 || highCount > 0:
		verdict = "NO-GO"
		if criticalCount > 0 {
			severity = "CRITICAL"
			confidence = "HIGH"
		} else {
			severity = "HIGH"
			confidence = "MEDIUM"
		}
	case len(personaResult.Findings) == 0:
		verdict = "GO"
		severity = "LOW"
		confidence = "HIGH"
	default:
		verdict = "GO"
		severity = "MEDIUM"
		confidence = "MEDIUM"
	}

	return &Vote{
		Persona:         persona,
		Phase:           phaseName,
		Timestamp:       time.Now(),
		Verdict:         verdict,
		Confidence:      confidence,
		Severity:        severity,
		Rationale:       personaResult.Summary,
		Blockers:        blockers,
		Recommendations: recommendations,
	}, nil
}
