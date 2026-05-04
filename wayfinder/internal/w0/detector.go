// Package w0 implements W0 project framing detection and charter management,
// ported from the TypeScript implementation in cortex/lib/w0-*.ts.
package w0

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// VaguenessSignal represents a vagueness indicator in a request.
type VaguenessSignal string

// VaguenessSignal values describing why a request looks vague.
const (
	SignalShort              VaguenessSignal = "short"
	SignalNoProblemStatement VaguenessSignal = "no_problem_statement"
	SignalNoConstraints      VaguenessSignal = "no_constraints"
	SignalGenericVerbs       VaguenessSignal = "generic_verbs"
	SignalNoSpecifics        VaguenessSignal = "no_specifics"
)

// DetectionResult holds the result of W0 vagueness detection.
type DetectionResult struct {
	Trigger  bool              // True if W0 should be triggered
	Reason   string            // Reason for decision
	Signals  []VaguenessSignal // Detected vagueness signals
	Score    float64           // Vagueness score (0.0-1.0)
	Metadata *DetectionMetadata
}

// DetectionMetadata holds metadata about the detection.
type DetectionMetadata struct {
	WordCount      int
	HasProblem     bool
	HasConstraints bool
	HasSpecifics   bool
}

// DetectionConfig configures W0 detection.
type DetectionConfig struct {
	Enabled            bool
	MinWordCount       int
	MaxSkipWordCount   int
	VaguenessThreshold float64
}

// DefaultDetectionConfig returns the default W0 detection configuration.
func DefaultDetectionConfig() DetectionConfig {
	return DetectionConfig{
		Enabled:            true,
		MinWordCount:       30,
		MaxSkipWordCount:   50,
		VaguenessThreshold: 0.6,
	}
}

// DetectW0Need determines if a wayfinder request needs W0 project framing.
func DetectW0Need(request, projectPath string, cfg ...DetectionConfig) DetectionResult {
	config := DefaultDetectionConfig()
	if len(cfg) > 0 {
		config = cfg[0]
	}

	if !config.Enabled {
		return DetectionResult{Reason: "disabled"}
	}

	w0Path := filepath.Join(projectPath, "W0-project-charter.md")
	if _, err := os.Stat(w0Path); err == nil {
		return DetectionResult{Reason: "w0_exists"}
	}

	if strings.Contains(request, "/minimal") || strings.Contains(request, "skip W0") {
		return DetectionResult{Reason: "user_skip"}
	}

	wordCount := countWords(request)
	if wordCount > config.MaxSkipWordCount {
		hasProblem := hasProblemStatement(request)
		hasConstraints := hasConstraintIndicators(request)
		hasSpecifics := hasSpecificIndicators(request)

		if hasProblem && hasConstraints && hasSpecifics {
			return DetectionResult{
				Reason: "detailed_request",
				Metadata: &DetectionMetadata{
					WordCount:      wordCount,
					HasProblem:     hasProblem,
					HasConstraints: hasConstraints,
					HasSpecifics:   hasSpecifics,
				},
			}
		}
	}

	signals := detectVaguenessSignals(request, config)
	score := float64(len(signals)) / 5.0

	metadata := &DetectionMetadata{
		WordCount:      wordCount,
		HasProblem:     hasProblemStatement(request),
		HasConstraints: hasConstraintIndicators(request),
		HasSpecifics:   hasSpecificIndicators(request),
	}

	if score >= config.VaguenessThreshold {
		return DetectionResult{
			Trigger:  true,
			Reason:   "vague_request",
			Signals:  signals,
			Score:    score,
			Metadata: metadata,
		}
	}

	return DetectionResult{
		Reason:   "clear_enough",
		Signals:  signals,
		Score:    score,
		Metadata: metadata,
	}
}

func detectVaguenessSignals(request string, config DetectionConfig) []VaguenessSignal {
	var signals []VaguenessSignal

	wordCount := countWords(request)
	if wordCount < config.MinWordCount {
		signals = append(signals, SignalShort)
	}

	if !hasProblemStatement(request) {
		signals = append(signals, SignalNoProblemStatement)
	}

	if !hasConstraintIndicators(request) {
		signals = append(signals, SignalNoConstraints)
	}

	if hasGenericVerbs(request) && wordCount < 20 {
		signals = append(signals, SignalGenericVerbs)
	}

	if !hasSpecificIndicators(request) {
		signals = append(signals, SignalNoSpecifics)
	}

	return signals
}

func countWords(text string) int {
	fields := strings.Fields(strings.TrimSpace(text))
	count := 0
	for _, f := range fields {
		if len(f) > 0 {
			count++
		}
	}
	return count
}

func matchesPattern(text, pattern string) bool {
	re, err := regexp.Compile("(?i)\\b(" + pattern + ")\\b")
	if err != nil {
		return false
	}
	return re.MatchString(text)
}

func hasProblemStatement(request string) bool {
	return matchesPattern(request, "because|issue|problem|pain|broken|bug|error|failing")
}

func hasConstraintIndicators(request string) bool {
	return matchesPattern(request, "must|constraint|requirement|need to|can't|cannot|required")
}

func hasGenericVerbs(request string) bool {
	return matchesPattern(request, "improve|enhance|better|fix|optimize|update")
}

var (
	filePathRegex      = regexp.MustCompile(`(?i)\w{3,}\.\w{2,}|/\w+|\./\w+`)
	technicalTermRegex = regexp.MustCompile(`\b[A-Z]{2,}\b`)
	metricsRegex2      = regexp.MustCompile(`(?i)\d+(%|ms|s|x|MB|KB)`)
)

func hasSpecificIndicators(request string) bool {
	return filePathRegex.MatchString(request) ||
		technicalTermRegex.MatchString(request) ||
		metricsRegex2.MatchString(request)
}
