package telemetry

import (
	"regexp"
	"strings"
)

// SessionContext contains extracted context from a session transcript.
type SessionContext struct {
	Language  string   `json:"language"`  // python, go, typescript, etc.
	Framework string   `json:"framework"` // django, react, etc.
	TaskType  string   `json:"task_type"` // debugging, feature, refactor, etc.
	Keywords  []string `json:"keywords"`  // Extracted keywords for matching
}

// ExtractContext analyzes session transcript to extract contextual information.
//
// Uses keyword-based heuristics (S6 design: ≥90% accuracy target, no ML needed for V1).
//
// Input: session transcript text (user messages + Claude responses)
// Output: SessionContext with language, framework, task_type
func ExtractContext(transcript string) SessionContext {
	ctx := SessionContext{
		Language:  "unknown",
		Framework: "unknown",
		TaskType:  "unknown",
		Keywords:  []string{},
	}

	transcriptLower := strings.ToLower(transcript)

	// Extract language (keyword matching)
	ctx.Language = extractLanguage(transcriptLower)

	// Extract framework (keyword matching)
	ctx.Framework = extractFramework(transcriptLower)

	// Extract task type (keyword matching)
	ctx.TaskType = extractTaskType(transcriptLower)

	// Extract keywords for pattern matching
	ctx.Keywords = extractKeywords(transcriptLower)

	return ctx
}

// extractLanguage detects programming language from transcript.
func extractLanguage(transcript string) string {
	// Language patterns with weights (high-confidence patterns get higher scores)
	// Weights: framework name=3, file extension=2, keywords=1
	languagePatterns := map[string]map[string]int{
		"python": {
			`\.py\b`:     2,
			`\bpython\b`: 2,
			`import `:    1,
			`def `:       1,
			`\bdjango\b`: 3, // Framework = high confidence
			`\bflask\b`:  3,
		},
		"go": {
			`\.go\b`:     2,
			`\bgolang\b`: 2,
			`package `:   1,
			`func `:      1,
			`go mod`:     2,
			`go\.mod`:    2,
			`\bgin\b`:    3, // Framework = high confidence
		},
		"javascript": {
			`\.js\b`:         2,
			`\bjavascript\b`: 2,
			`\bnode\b`:       2,
			`\bnpm\b`:        1,
			`const `:         1,
			`let `:           1,
			`var `:           1,
			`\breact\b`:      3, // Framework = high confidence
			`\bvue\b`:        3,
			`\bangular\b`:    3,
			`useState`:       3, // React-specific
		},
		"typescript": {
			`\.ts\b`:         2,
			`\btypescript\b`: 2,
			`interface `:     1,
			`type `:          1,
			`\.tsx\b`:        2,
		},
		"rust": {
			`\.rs\b`:      2,
			`\brust\b`:    2,
			`\bcargo\b`:   2,
			`fn `:         1,
			`impl `:       1,
			`Cargo\.toml`: 2,
		},
		"java": {
			`\.java\b`:     2,
			`\bjava\b`:     2,
			`public class`: 2, // More specific than just "class"
			`\bmaven\b`:    2,
			`\bgradle\b`:   2,
		},
		"cpp": {
			`\.cpp\b`:  2,
			`\.cc\b`:   2,
			`c\+\+`:    2,
			`#include`: 1,
			`std::`:    1,
		},
	}

	// Score each language by weighted pattern matches
	scores := make(map[string]int)
	for lang, patterns := range languagePatterns {
		for pattern, weight := range patterns {
			if matched, _ := regexp.MatchString(pattern, transcript); matched {
				scores[lang] += weight
			}
		}
	}

	// Return language with highest score (must have score > 0)
	maxScore := 0
	detectedLang := "unknown"
	for lang, score := range scores {
		if score > maxScore {
			maxScore = score
			detectedLang = lang
		}
	}

	return detectedLang
}

// extractFramework detects framework from transcript.
func extractFramework(transcript string) string {
	// Framework patterns
	frameworkPatterns := map[string][]string{
		"django":  {`django`, `manage\.py`, `settings\.py`},
		"flask":   {`flask`, `@app\.route`, `from flask`},
		"react":   {`react`, `jsx`, `useState`, `useEffect`},
		"vue":     {`\bvue\b`, `\.vue\b`, `v-model`, `v-bind`},
		"angular": {`angular`, `@Component`, `ng serve`},
		"express": {`express`, `app\.listen`, `req, res`},
		"gin":     {`gin-gonic`, `gin\.`, `c\.JSON`},
	}

	// Score each framework by pattern matches
	scores := make(map[string]int)
	for framework, patterns := range frameworkPatterns {
		for _, pattern := range patterns {
			if matched, _ := regexp.MatchString(pattern, transcript); matched {
				scores[framework]++
			}
		}
	}

	// Return framework with highest score
	maxScore := 0
	detectedFramework := "unknown"
	for framework, score := range scores {
		if score > maxScore {
			maxScore = score
			detectedFramework = framework
		}
	}

	return detectedFramework
}

// extractTaskType detects task type from transcript.
func extractTaskType(transcript string) string {
	// Task type patterns (keywords indicating intent)
	taskPatterns := map[string][]string{
		"debugging":   {`debug`, `bug`, `error`, `fix`, `issue`, `not working`, `fails`},
		"feature":     {`add`, `implement`, `create`, `new feature`, `functionality`},
		"refactor":    {`refactor`, `restructure`, `clean up`, `improve`, `reorganize`},
		"testing":     {`test`, `spec`, `unit test`, `integration test`, `coverage`},
		"docs":        {`document`, `readme`, `comment`, `explain`, `describe`},
		"performance": {`optimize`, `performance`, `slow`, `faster`, `speed`},
	}

	// Score each task type by pattern matches
	scores := make(map[string]int)
	for taskType, patterns := range taskPatterns {
		for _, pattern := range patterns {
			if matched, _ := regexp.MatchString(pattern, transcript); matched {
				scores[taskType]++
			}
		}
	}

	// Return task type with highest score
	maxScore := 0
	detectedTask := "unknown"
	for taskType, score := range scores {
		if score > maxScore {
			maxScore = score
			detectedTask = taskType
		}
	}

	return detectedTask
}

// extractKeywords extracts relevant keywords for engram pattern matching.
func extractKeywords(transcript string) []string {
	// Extract common technical keywords
	// (simplified for V1, can be enhanced in V2)
	keywords := []string{}

	// Extract file extensions (strong signal for language)
	extPattern := regexp.MustCompile(`\.\w{2,4}\b`)
	extMatches := extPattern.FindAllString(transcript, -1)
	keywords = append(keywords, extMatches...)

	// Extract import statements (strong signal for libraries)
	importPattern := regexp.MustCompile(`import\s+\w+`)
	importMatches := importPattern.FindAllString(transcript, -1)
	keywords = append(keywords, importMatches...)

	return keywords
}
