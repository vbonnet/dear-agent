package w0

import (
	"math"
	"regexp"
	"strings"
)

// TechnicalLevel indicates the technical sophistication of a response.
type TechnicalLevel string

const (
	TechLevelHigh TechnicalLevel = "high"
	TechLevelLow  TechnicalLevel = "low"
)

// ResponseValidationResult holds the result of response validation.
type ResponseValidationResult struct {
	Valid            bool
	Feedback         string
	NeedsElaboration bool
	Uncertainty      bool
	WordCount        int
	TechnicalTerms   []string
	TechnicalLevel   TechnicalLevel
	ClarityScore     float64
}

// KnownTechnicalTerms is the list of known technical terms for detection.
var KnownTechnicalTerms = []string{
	// Auth & Security
	"OAuth", "SSO", "SAML", "JWT", "authentication", "authorization", "middleware", "2FA", "MFA",
	// APIs
	"API", "REST", "GraphQL", "gRPC", "WebSocket", "HTTP", "HTTPS",
	// Infrastructure
	"microservices", "Kubernetes", "Docker", "container", "CI/CD", "DevOps", "deployment", "infrastructure",
	// Data
	"database", "SQL", "NoSQL", "PostgreSQL", "MySQL", "MongoDB", "cache", "Redis", "Memcached",
	// Architecture
	"frontend", "backend", "full-stack", "serverless", "monolith", "service-oriented", "event-driven",
	// Frameworks
	"React", "Vue", "Angular", "Node.js", "Express", "Django", "Flask", "Spring",
	// Cloud
	"AWS", "Azure", "GCP", "Google Cloud", "Heroku", "Vercel", "Netlify",
	// Concepts
	"refactoring", "migration", "integration", "scalability", "performance", "latency", "throughput", "bottleneck",
	// Business
	"SaaS", "PaaS", "IaaS",
}

var uncertaintyPatterns = []string{
	"don't know", "not sure", "no idea", "unclear", "uncertain",
	"maybe", "i guess", "not certain", "hard to say",
}

var acceptableShortResponses = []string{"none", "not sure", "n/a", "na", "no"}

// ValidateUserResponse validates a user's free-text response.
func ValidateUserResponse(response string, questionType QuestionType) ResponseValidationResult {
	if response == "" || strings.TrimSpace(response) == "" {
		return ResponseValidationResult{
			Feedback:       "I didn't catch that. Could you provide an answer?",
			TechnicalLevel: TechLevelLow,
		}
	}

	trimmed := strings.TrimSpace(response)
	wc := countWords(trimmed)
	techTerms := DetectTechnicalTerms(trimmed)
	techLevel := TechLevelLow
	if len(techTerms) > 2 {
		techLevel = TechLevelHigh
	}

	// Too brief check
	if wc < 3 {
		isAcceptable := false
		lower := strings.ToLower(trimmed)
		for _, a := range acceptableShortResponses {
			if lower == a {
				isAcceptable = true
				break
			}
		}
		if !isAcceptable {
			return ResponseValidationResult{
				Valid:            true,
				NeedsElaboration: true,
				Feedback:         "Could you elaborate a bit more? A few more details would help.",
				WordCount:        wc,
				TechnicalTerms:   techTerms,
				TechnicalLevel:   techLevel,
			}
		}
	}

	// Uncertainty check
	if HasUncertaintyIndicators(trimmed) {
		return ResponseValidationResult{
			Valid:          true,
			Uncertainty:    true,
			Feedback:       "That's okay! We can explore this together in D1. For now, what's your best guess or instinct?",
			WordCount:      wc,
			TechnicalTerms: techTerms,
			TechnicalLevel: techLevel,
		}
	}

	clarityScore := CalculateClarityScore(trimmed, questionType)

	return ResponseValidationResult{
		Valid:          true,
		WordCount:      wc,
		TechnicalTerms: techTerms,
		TechnicalLevel: techLevel,
		ClarityScore:   clarityScore,
	}
}

// DetectTechnicalTerms detects known technical terms in text.
func DetectTechnicalTerms(text string) []string {
	var found []string
	for _, term := range KnownTechnicalTerms {
		re, err := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(term) + `\b`)
		if err != nil {
			continue
		}
		if re.MatchString(text) {
			found = append(found, term)
		}
	}
	return found
}

// HasUncertaintyIndicators checks if response contains uncertainty indicators.
func HasUncertaintyIndicators(text string) bool {
	lower := strings.ToLower(text)
	for _, pattern := range uncertaintyPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// CalculateClarityScore calculates a clarity score (0.0-1.0).
func CalculateClarityScore(response string, questionType QuestionType) float64 {
	score := 0.0
	wc := countWords(response)
	techTerms := DetectTechnicalTerms(response)

	// Factor 1: Word count
	optimalMin := 8
	optimalMax := 40
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch questionType {
	case QuestionImpact:
		optimalMin = 20
		optimalMax = 80
	case QuestionProblem:
		optimalMin = 10
		optimalMax = 50
	}

	if wc >= optimalMin && wc <= optimalMax {
		score += 0.3
	} else if wc < optimalMin {
		score += float64(wc) / float64(optimalMin) * 0.3
	} else {
		score += 0.2
	}

	// Factor 2: Specificity
	if (questionType == QuestionProblem || questionType == QuestionImpact) &&
		matchesPattern(response, "because|issue|problem|pain|broken|bug|error|failing|slow|missing|cannot|can't|lack") {
		score += 0.25
	}

	if questionType == QuestionConstraints &&
		matchesPattern(response, "must|constraint|requirement|need to|can't|cannot|required|should") {
		score += 0.25
	}

	hasNumbers := regexp.MustCompile(`(?i)\d+(%|ms|s|x|MB|KB|users?|customers?|days?|hours?)`).MatchString(response)
	hasFilePaths := regexp.MustCompile(`(?i)\w{3,}\.\w{2,}|/\w+|\./`).MatchString(response)
	hasAcronyms := regexp.MustCompile(`\b[A-Z]{2,}\b`).MatchString(response)

	if hasNumbers || hasFilePaths || hasAcronyms || len(techTerms) > 0 {
		score += 0.2
	}

	// Factor 3: Structure
	sentences := strings.FieldsFunc(response, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	nonEmpty := 0
	for _, s := range sentences {
		if strings.TrimSpace(s) != "" {
			nonEmpty++
		}
	}
	if nonEmpty >= 2 {
		score += 0.15
	}

	if questionType == QuestionApproach &&
		regexp.MustCompile(`(?i)\b(add|create|build|implement|integrate|refactor|optimize|fix|migrate)\b`).MatchString(response) {
		score += 0.15
	}

	// Penalty: Too vague
	vaguePhrases := []string{"improve", "enhance", "better", "update", "make it", "work on"}
	vagueCount := 0
	for _, phrase := range vaguePhrases {
		re, _ := regexp.Compile(`(?i)\b` + regexp.QuoteMeta(phrase) + `\b`)
		if re != nil && re.MatchString(response) {
			vagueCount++
		}
	}
	if vagueCount > 2 {
		score -= 0.1
	}

	return math.Max(0, math.Min(1, score))
}
