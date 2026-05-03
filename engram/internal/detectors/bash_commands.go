package detectors

import (
	"context"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// BashCommandPatternDetector detects bash anti-patterns
type BashCommandPatternDetector struct {
	patterns map[string]*regexp.Regexp
}

// Pre-compiled patterns for bash anti-patterns
var defaultBashPatterns = map[string]*regexp.Regexp{
	"never_use_cd_and":   regexp.MustCompile(`cd .* &&`),
	"never_use_cat":      regexp.MustCompile(`\bcat\s+\S+`),
	"never_use_grep":     regexp.MustCompile(`grep .* `),
	"never_use_for_loop": regexp.MustCompile(`for .* in .*; do`),
}

// NewBashCommandPatternDetector creates a new bash pattern detector
func NewBashCommandPatternDetector() *BashCommandPatternDetector {
	return &BashCommandPatternDetector{
		patterns: defaultBashPatterns,
	}
}

// Name returns detector identifier
func (d *BashCommandPatternDetector) Name() string {
	return "bash_command_pattern"
}

// SupportedInstructionTypes returns instruction types handled
func (d *BashCommandPatternDetector) SupportedInstructionTypes() []string {
	return []string{"tool_usage"}
}

// Detect analyzes bash commands for anti-patterns
func (d *BashCommandPatternDetector) Detect(ctx context.Context, input DetectorInput) ([]telemetry.ViolationEvent, error) {
	var violations []telemetry.ViolationEvent

	// Check each pattern
	for ruleID, pattern := range d.patterns {
		if pattern.MatchString(input.Content) {
			// Find corresponding rule for message
			var ruleMessage string
			var ruleConfidence = telemetry.ConfidenceHigh // Default

			for _, r := range input.Rules {
				if r.ID == ruleID {
					ruleMessage = r.Message
					ruleConfidence = r.Confidence
					break
				}
			}

			// Use default message if rule not provided
			if ruleMessage == "" {
				ruleMessage = getDefaultMessage(ruleID)
			}

			violation := telemetry.ViolationEvent{
				ID:              uuid.New().String(),
				Timestamp:       time.Now().UTC(),
				InstructionType: "tool_usage",
				InstructionRule: ruleID,
				ViolationType:   "bash_command_pattern",
				Confidence:      ruleConfidence,
				Agent:           getMetadata(input.Metadata, "agent", "unknown"),
				Context:         truncate(input.Content, 1000),
				DetectionMethod: telemetry.DetectionExternal,
				ProjectPath:     input.Metadata["project_path"],
				Phase:           input.Metadata["phase"],
			}

			violations = append(violations, violation)
		}
	}

	return violations, nil
}

// getDefaultMessage returns default message for rule ID
func getDefaultMessage(ruleID string) string {
	messages := map[string]string{
		"never_use_cd_and":   "Never use 'cd ... &&' pattern - use -C flag instead",
		"never_use_cat":      "Never use 'cat file' - use Read tool instead",
		"never_use_grep":     "Never use 'grep' command - use Grep tool instead",
		"never_use_for_loop": "Never use bash for loops - use parallel tool calls instead",
	}

	if msg, ok := messages[ruleID]; ok {
		return msg
	}
	return "Bash anti-pattern detected"
}

// getMetadata retrieves metadata value with fallback
func getMetadata(metadata map[string]string, key, fallback string) string {
	if val, ok := metadata[key]; ok && val != "" {
		return val
	}
	return fallback
}

// truncate limits context to max length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
