package agent

import (
	"testing"
)

func TestExtractFeatures(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		wantWC   int
		wantTC   int
		wantSpec float64Range
		wantEx   bool
		wantCon  bool
		wantCtx  float64Range
	}{
		{
			name:     "empty prompt",
			prompt:   "",
			wantWC:   0,
			wantTC:   0,
			wantSpec: float64Range{0.0, 0.0},
			wantEx:   false,
			wantCon:  false,
			wantCtx:  float64Range{0.0, 1.0},
		},
		{
			name:     "simple prompt with constraints",
			prompt:   "Create a function calculateTotal() that takes 100 numbers",
			wantWC:   8,
			wantTC:   8,
			wantSpec: float64Range{0.10, 0.15}, // calculateTotal + 100
			wantEx:   false,
			wantCon:  true,                   // has "100"
			wantCtx:  float64Range{0.0, 1.0}, // self-contained
		},
		{
			name:     "prompt with code block",
			prompt:   "Implement this:\n```go\nfunc main() {}\n```",
			wantWC:   7,
			wantTC:   7,
			wantSpec: float64Range{0.0, 0.3},
			wantEx:   true, // has ```
			wantCon:  false,
			wantCtx:  float64Range{0.5, 1.0},
		},
		{
			name:     "prompt with file path and constraints",
			prompt:   "Read features.go and extract up to 100 functions",
			wantWC:   8,
			wantTC:   8,
			wantSpec: float64Range{0.15, 0.30}, // features.go + 100
			wantEx:   false,
			wantCon:  true, // has "100" and "up to"
			wantCtx:  float64Range{0.8, 1.0},
		},
		{
			name:     "prompt with references (low context score)",
			prompt:   "Do the same thing as above. Use that approach. This is similar to the previous example.",
			wantWC:   16,
			wantTC:   16,
			wantSpec: float64Range{0.0, 0.1},
			wantEx:   false,
			wantCon:  false,
			wantCtx:  float64Range{0.0, 0.5}, // many references (above, that, this, previous)
		},
		{
			name:     "prompt with structured data",
			prompt:   "Parse this JSON: {\"name\": \"test\", \"count\": 42}",
			wantWC:   7,
			wantTC:   7,
			wantSpec: float64Range{0.10, 0.30}, // JSON + 42
			wantEx:   true,                     // has {
			wantCon:  true,                     // has 42
			wantCtx:  float64Range{0.0, 1.0},   // "this" is a reference word
		},
		{
			name:     "prompt with limit keyword",
			prompt:   "Generate a list of 50 items, maximum length 1000 characters",
			wantWC:   10,
			wantTC:   10,
			wantSpec: float64Range{0.15, 0.25}, // 50, 1000
			wantEx:   false,
			wantCon:  true, // has "maximum", 50, 1000
			wantCtx:  float64Range{0.8, 1.0},
		},
		{
			name:     "complex prompt with all features",
			prompt:   "Create a function ProcessData() in data.go that handles up to 1000 records. Example: ```go\nfunc ProcessData() {}\n```",
			wantWC:   18,
			wantTC:   18,
			wantSpec: float64Range{0.15, 0.30}, // ProcessData, data.go, 1000
			wantEx:   true,                     // has ```
			wantCon:  true,                     // has "up to", 1000
			wantCtx:  float64Range{0.5, 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractFeatures(tt.prompt)

			if got.WordCount != tt.wantWC {
				t.Errorf("WordCount = %d, want %d", got.WordCount, tt.wantWC)
			}

			if got.TokenCount != tt.wantTC {
				t.Errorf("TokenCount = %d, want %d", got.TokenCount, tt.wantTC)
			}

			if !inRange(got.SpecificityScore, tt.wantSpec.min, tt.wantSpec.max) {
				t.Errorf("SpecificityScore = %.2f, want [%.2f, %.2f]", got.SpecificityScore, tt.wantSpec.min, tt.wantSpec.max)
			}

			if got.HasExamples != tt.wantEx {
				t.Errorf("HasExamples = %v, want %v", got.HasExamples, tt.wantEx)
			}

			if got.HasConstraints != tt.wantCon {
				t.Errorf("HasConstraints = %v, want %v", got.HasConstraints, tt.wantCon)
			}

			if !inRange(got.ContextEmbeddingScore, tt.wantCtx.min, tt.wantCtx.max) {
				t.Errorf("ContextEmbeddingScore = %.2f, want [%.2f, %.2f]", got.ContextEmbeddingScore, tt.wantCtx.min, tt.wantCtx.max)
			}

			// Validate score ranges
			if got.SpecificityScore < 0.0 || got.SpecificityScore > 1.0 {
				t.Errorf("SpecificityScore out of range: %.2f", got.SpecificityScore)
			}

			if got.ContextEmbeddingScore < 0.0 || got.ContextEmbeddingScore > 1.0 {
				t.Errorf("ContextEmbeddingScore out of range: %.2f", got.ContextEmbeddingScore)
			}
		})
	}
}

func TestCalculateSpecificity(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		wordCount int
		wantRange float64Range
	}{
		{
			name:      "no concrete terms",
			prompt:    "do something nice",
			wordCount: 3,
			wantRange: float64Range{0.0, 0.0},
		},
		{
			name:      "with file path",
			prompt:    "read features.go file",
			wordCount: 3,
			wantRange: float64Range{0.30, 0.35},
		},
		{
			name:      "with camelCase",
			prompt:    "call ExtractFeatures function",
			wordCount: 3,
			wantRange: float64Range{0.30, 0.35},
		},
		{
			name:      "with numbers",
			prompt:    "process 100 records",
			wordCount: 3,
			wantRange: float64Range{0.30, 0.35},
		},
		{
			name:      "multiple concrete terms",
			prompt:    "read data.go and process 100 records using ParseData function",
			wordCount: 10,
			wantRange: float64Range{0.30, 0.50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateSpecificity(tt.prompt, tt.wordCount)
			if !inRange(got, tt.wantRange.min, tt.wantRange.max) {
				t.Errorf("calculateSpecificity() = %.2f, want [%.2f, %.2f]", got, tt.wantRange.min, tt.wantRange.max)
			}
		})
	}
}

func TestDetectExamples(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"no examples", "do something", false},
		{"code block", "example: ```code```", true},
		{"json object", `parse {"key": "value"}`, true},
		{"json array", `parse [1, 2, 3]`, true},
		{"no structured data", "use parentheses (like this)", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectExamples(tt.prompt); got != tt.want {
				t.Errorf("detectExamples() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectConstraints(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   bool
	}{
		{"no constraints", "do something nice", false},
		{"with number", "process 100 items", true},
		{"with limit keyword", "maximum of 50", true},
		{"with must keyword", "must include all", true},
		{"with min keyword", "minimum value 10", true},
		{"with exactly keyword", "exactly 5 elements", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectConstraints(tt.prompt); got != tt.want {
				t.Errorf("detectConstraints() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateContextScore(t *testing.T) {
	tests := []struct {
		name      string
		prompt    string
		wantRange float64Range
	}{
		{
			name:      "fully embedded (no references)",
			prompt:    "Create a new function that processes data.",
			wantRange: float64Range{0.5, 1.0},
		},
		{
			name:      "some references",
			prompt:    "Do the same as above. Use that approach.",
			wantRange: float64Range{0.0, 0.6},
		},
		{
			name:      "many references",
			prompt:    "This is similar to the previous example. That approach works. Use it here.",
			wantRange: float64Range{0.0, 0.4},
		},
		{
			name:      "empty prompt",
			prompt:    "",
			wantRange: float64Range{1.0, 1.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateContextScore(tt.prompt)
			if !inRange(got, tt.wantRange.min, tt.wantRange.max) {
				t.Errorf("calculateContextScore() = %.2f, want [%.2f, %.2f]", got, tt.wantRange.min, tt.wantRange.max)
			}
		})
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		min   float64
		max   float64
		want  float64
	}{
		{"within range", 0.5, 0.0, 1.0, 0.5},
		{"below min", -0.5, 0.0, 1.0, 0.0},
		{"above max", 1.5, 0.0, 1.0, 1.0},
		{"at min", 0.0, 0.0, 1.0, 0.0},
		{"at max", 1.0, 0.0, 1.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clamp(tt.value, tt.min, tt.max); got != tt.want {
				t.Errorf("clamp() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Helper types and functions

type float64Range struct {
	min, max float64
}

func inRange(value, min, max float64) bool {
	return value >= min && value <= max
}
