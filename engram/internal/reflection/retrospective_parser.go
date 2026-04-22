package reflection

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// RetrospectiveParser extracts failure learnings from S11 retrospective files
// (Task 1.2.1: Parse "What Went Wrong" section)
type RetrospectiveParser struct {
	// Section headers to look for
	improvementHeaderPattern *regexp.Regexp
	challengesHeaderPattern  *regexp.Regexp

	// Bullet point pattern
	bulletPattern *regexp.Regexp
}

// NewRetrospectiveParser creates a new retrospective parser
func NewRetrospectiveParser() *RetrospectiveParser {
	return &RetrospectiveParser{
		// Matches "What Could Improve", "What Went Wrong", "Challenges", etc.
		improvementHeaderPattern: regexp.MustCompile(`(?i)^#{2,3}\s+(What (Could|Went) (Improve|Wrong)|Challenges?|Problems?|Issues?)`),

		// Matches "Technical Challenges:", "Process Breakdowns:", etc.
		challengesHeaderPattern: regexp.MustCompile(`(?i)^\*\*Technical (Challenges?|Problems?|Issues?):`),

		// Matches bullet points: "- ", "* ", "1. ", etc.
		bulletPattern: regexp.MustCompile(`^[-*•]\s+(.+)$|^(\d+)\.\s+(.+)$`),
	}
}

// ParseFile parses a retrospective file and extracts failure learnings
func (p *RetrospectiveParser) ParseFile(filepath string) ([]*FailureLearning, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to open retrospective file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	return p.parseContent(scanner)
}

// parseContent processes the file content line by line
func (p *RetrospectiveParser) parseContent(scanner *bufio.Scanner) ([]*FailureLearning, error) {
	var learnings []*FailureLearning

	inImprovementSection := false
	inTechnicalChallenges := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Update section state
		inImprovementSection, inTechnicalChallenges = p.updateSectionState(
			trimmed, inImprovementSection, inTechnicalChallenges,
		)

		// Extract learnings from technical challenges section
		if inTechnicalChallenges && trimmed != "" {
			if learning := p.extractLearningFromBullet(trimmed); learning != nil {
				learnings = append(learnings, learning)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading retrospective: %w", err)
	}

	return learnings, nil
}

// updateSectionState updates section tracking based on current line
func (p *RetrospectiveParser) updateSectionState(line string, inImprovement, inChallenges bool) (bool, bool) {
	// Entering improvement section
	if p.improvementHeaderPattern.MatchString(line) {
		return true, false
	}

	// Entering technical challenges subsection
	if inImprovement && p.challengesHeaderPattern.MatchString(line) {
		return inImprovement, true
	}

	// Exit on new major header
	if p.isNonImprovementMajorHeader(line) {
		return false, false
	}

	// Exit technical challenges on new subsection
	if inChallenges && p.isNonChallengesSubsection(line) {
		return inImprovement, false
	}

	return inImprovement, inChallenges
}

// isNonImprovementMajorHeader checks if line is a major header that's not an improvement section
func (p *RetrospectiveParser) isNonImprovementMajorHeader(line string) bool {
	return strings.HasPrefix(line, "##") && !p.improvementHeaderPattern.MatchString(line)
}

// isNonChallengesSubsection checks if line is a subsection that's not technical challenges
func (p *RetrospectiveParser) isNonChallengesSubsection(line string) bool {
	return strings.HasPrefix(line, "**") && !p.challengesHeaderPattern.MatchString(line)
}

// extractLearningFromBullet extracts a FailureLearning from a bullet point line
func (p *RetrospectiveParser) extractLearningFromBullet(line string) *FailureLearning {
	matches := p.bulletPattern.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	// Get the captured text (either from - or numbered list)
	text := matches[1]
	if text == "" {
		text = matches[3] // Numbered list
	}

	// Skip template placeholders
	if isTemplatePlaceholder(text) {
		return nil
	}

	return &FailureLearning{
		Description: strings.TrimSpace(text),
		Source:      "retrospective",
	}
}

// isTemplatePlaceholder checks if text is a template placeholder like "[TODO]"
func isTemplatePlaceholder(text string) bool {
	return strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]")
}

// ConvertToReflection converts a failure learning to a reflection
// (Task 1.2.1: Generate failure reflection markdown)
func (p *RetrospectiveParser) ConvertToReflection(learning *FailureLearning, sessionID string) *Reflection {
	// Create reflection with basic fields
	reflection := &Reflection{
		SessionID: sessionID,
		Timestamp: time.Now(),
		Trigger: Trigger{
			Type:        TriggerRepeatedFailureToSuccess,
			Description: learning.Description,
		},
		Learning: learning.Description,
		Tags:     []string{"retrospective", "failure"},
	}

	// Note: Outcome, ErrorCategory, LessonLearned will be set by FailureDetector
	// when saved using SaveWithAutoDetect()

	return reflection
}

// ExtractAndConvert extracts failures from retrospective and converts to reflections
// (Task 1.2.1: Main entry point for retrospective processing)
func (p *RetrospectiveParser) ExtractAndConvert(filepath string, sessionID string) ([]*Reflection, error) {
	// Parse the retrospective file
	learnings, err := p.ParseFile(filepath)
	if err != nil {
		return nil, err
	}

	// Convert to reflections
	var reflections []*Reflection
	for _, learning := range learnings {
		reflection := p.ConvertToReflection(learning, sessionID)
		reflections = append(reflections, reflection)
	}

	return reflections, nil
}

// FailureLearning represents a failure extracted from a retrospective
type FailureLearning struct {
	Description string // The failure description
	Source      string // Where it came from ("retrospective")
}
