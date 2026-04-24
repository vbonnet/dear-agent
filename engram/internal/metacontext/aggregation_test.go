package metacontext

import (
	"testing"
	"time"
)

// ============================================================================
// Unit Tests: aggregation.go (importance scoring, signal deduplication)
// S7 Plan: Week 4 Testing, Unit Test Category
// Validates CRITICAL FIX #3 (importance-based prioritization)
// ============================================================================

// TestCalculateImportance_Confidence tests confidence factor (40% weight)
func TestCalculateImportance_Confidence(t *testing.T) {
	sig := Signal{Name: "Go", Confidence: 0.8, Source: "file"}
	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	importance := calculateImportance(sig, context)

	// Only confidence factor (0.8 * 0.4 = 0.32)
	expected := 0.32
	if importance < expected-0.01 || importance > expected+0.01 {
		t.Errorf("Expected importance ~%.2f, got %.2f", expected, importance)
	}
}

// TestCalculateImportance_Recency tests recency factor (20% weight)
func TestCalculateImportance_Recency(t *testing.T) {
	sig := Signal{Name: "Go", Confidence: 0.8, Source: "file"}
	context := AnalyzeContext{
		FileModTimes: map[string]time.Time{
			"file": time.Now().Add(-5 * 24 * time.Hour), // 5 days ago
		},
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	importance := calculateImportance(sig, context)

	// Confidence (0.8 * 0.4) + Recency (5/30 decay * 0.2) ≈ 0.32 + 0.17 = 0.49
	if importance < 0.45 || importance > 0.55 {
		t.Errorf("Expected importance ~0.49, got %.2f", importance)
	}
}

// TestCalculateImportance_UserMentions tests user mention factor (30% weight)
// CRITICAL FIX #3: User mentions boost importance to prevent truncation
func TestCalculateImportance_UserMentions(t *testing.T) {
	sig := Signal{Name: "Vue", Confidence: 0.6, Source: "conversation"}
	context := AnalyzeContext{
		FileModTimes: make(map[string]time.Time),
		ConversationMatches: map[string]int{
			"Vue": 2, // Mentioned 2 times
		},
		PrimarySignals: make(map[string]bool),
	}

	importance := calculateImportance(sig, context)

	// Confidence (0.6 * 0.4) + User mentions (2/5 * 0.3) = 0.24 + 0.12 = 0.36
	expected := 0.36
	if importance < expected-0.05 || importance > expected+0.05 {
		t.Errorf("Expected importance ~%.2f, got %.2f", expected, importance)
	}

	// User mentions should boost rank above confidence-only
	// Vue (0.6 confidence, 2 mentions) vs ObscureLib (0.61 confidence, 0 mentions)
	obscureLib := Signal{Name: "ObscureLib", Confidence: 0.61, Source: "file"}
	obscureContext := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	obscureImportance := calculateImportance(obscureLib, obscureContext)

	// CRITICAL: Vue importance (0.36) should be > ObscureLib importance (0.244)
	if importance <= obscureImportance {
		t.Errorf("User-mentioned signal should rank higher: Vue %.2f vs ObscureLib %.2f",
			importance, obscureImportance)
	}
}

// TestCalculateImportance_PrimarySignal tests primary signal boost (10% weight)
func TestCalculateImportance_PrimarySignal(t *testing.T) {
	sig := Signal{Name: "Go", Confidence: 0.8, Source: "file"}
	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals: map[string]bool{
			"Go": true, // Primary signal
		},
	}

	importance := calculateImportance(sig, context)

	// Confidence (0.8 * 0.4) + Primary (0.1) = 0.32 + 0.1 = 0.42
	expected := 0.42
	if importance < expected-0.01 || importance > expected+0.01 {
		t.Errorf("Expected importance ~%.2f, got %.2f", expected, importance)
	}
}

// TestDeduplicateSignalsWithImportance_Deduplication tests duplicate merging
func TestDeduplicateSignalsWithImportance_Deduplication(t *testing.T) {
	signals := []Signal{
		{Name: "Go", Confidence: 0.8, Source: "file"},
		{Name: "Go", Confidence: 0.9, Source: "dependency"}, // Duplicate (higher confidence)
		{Name: "TypeScript", Confidence: 0.7, Source: "file"},
	}

	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	result := deduplicateSignalsWithImportance(signals, context, 10)

	// Should return 2 unique signals (Go, TypeScript)
	if len(result) != 2 {
		t.Errorf("Expected 2 unique signals, got %d", len(result))
	}

	// Go should have higher confidence (0.9, not 0.8)
	for _, sig := range result {
		if sig.Name == "Go" && sig.Confidence != 0.9 {
			t.Errorf("Duplicate merge should keep highest confidence, got %.2f", sig.Confidence)
		}
	}
}

// TestDeduplicateSignalsWithImportance_Truncation tests max signal limit
func TestDeduplicateSignalsWithImportance_Truncation(t *testing.T) {
	signals := []Signal{
		{Name: "Signal1", Confidence: 0.9, Source: "file"},
		{Name: "Signal2", Confidence: 0.8, Source: "file"},
		{Name: "Signal3", Confidence: 0.7, Source: "file"},
		{Name: "Signal4", Confidence: 0.6, Source: "file"},
		{Name: "Signal5", Confidence: 0.5, Source: "file"},
	}

	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	// Limit to 3 signals
	result := deduplicateSignalsWithImportance(signals, context, 3)

	if len(result) != 3 {
		t.Errorf("Expected 3 signals (truncated), got %d", len(result))
	}

	// Top 3 by importance should be Signal1, Signal2, Signal3
	expected := map[string]bool{"Signal1": true, "Signal2": true, "Signal3": true}
	for _, sig := range result {
		if !expected[sig.Name] {
			t.Errorf("Unexpected signal in top 3: %s", sig.Name)
		}
	}
}

// TestDeduplicateSignalsWithImportance_ImportanceSort tests sorting by importance
// CRITICAL FIX #3: Sort by importance (NOT confidence)
func TestDeduplicateSignalsWithImportance_ImportanceSort(t *testing.T) {
	signals := []Signal{
		{Name: "React", Confidence: 0.95, Source: "file"},       // High confidence, no mentions
		{Name: "Vue", Confidence: 0.60, Source: "conversation"}, // Low confidence, user mentioned
	}

	context := AnalyzeContext{
		FileModTimes: make(map[string]time.Time),
		ConversationMatches: map[string]int{
			"Vue": 3, // User mentioned 3 times
		},
		PrimarySignals: make(map[string]bool),
	}

	result := deduplicateSignalsWithImportance(signals, context, 10)

	// Vue importance: 0.6*0.4 + 3/5*0.3 = 0.24 + 0.18 = 0.42
	// React importance: 0.95*0.4 = 0.38
	// Vue should rank higher (0.42 > 0.38)

	if len(result) < 2 {
		t.Fatalf("Expected at least 2 signals, got %d", len(result))
	}

	// First signal should be Vue (higher importance)
	if result[0].Name != "Vue" {
		t.Errorf("Expected Vue to rank first (user mentioned), got %s", result[0].Name)
	}
}

// TestBuildAnalyzeContext tests context construction
func TestBuildAnalyzeContext(t *testing.T) {
	req := &AnalyzeRequest{WorkingDir: "/tmp/test"}
	signals := []Signal{
		{Name: "Go", Confidence: 0.95, Source: "file"},
		{Name: "Gin", Confidence: 0.9, Source: "dependency"},
		{Name: "Vue", Confidence: 0.7, Source: "conversation"},
		{Name: "Vue", Confidence: 0.6, Source: "conversation"}, // Duplicate mention
	}

	context := buildAnalyzeContext(req, signals)

	// Conversation matches should count Vue twice
	if context.ConversationMatches["Vue"] != 2 {
		t.Errorf("Expected 2 Vue mentions, got %d", context.ConversationMatches["Vue"])
	}

	// Primary signals should identify highest confidence per source
	if !context.PrimarySignals["Go"] {
		t.Error("Go should be primary signal (highest confidence in file)")
	}
}

// TestGroupByType tests signal categorization
func TestGroupByType(t *testing.T) {
	signals := []Signal{
		{Name: "Go", Confidence: 0.95, Source: "file"},
		{Name: "TypeScript", Confidence: 0.8, Source: "file"},
		{Name: "Gin", Confidence: 0.9, Source: "dependency"},
		{Name: "Git", Confidence: 1.0, Source: "git"},
	}

	languages, frameworks, tools := groupByType(signals)

	// Go, TypeScript should be languages
	if len(languages) != 2 {
		t.Errorf("Expected 2 languages, got %d", len(languages))
	}

	// Gin should be framework
	if len(frameworks) != 1 {
		t.Errorf("Expected 1 framework, got %d", len(frameworks))
	}

	// Git should be tool
	if len(tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(tools))
	}
}

// TestIsLanguage tests language identification
func TestIsLanguage(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"Go", true},
		{"TypeScript", true},
		{"Python", true},
		{"Gin", false},
		{"React", false},
		{"Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isLanguage(tt.name)
			if result != tt.expected {
				t.Errorf("isLanguage(%s) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}

// TestIsFramework tests framework identification
func TestIsFramework(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"React", true},
		{"Vue", true},
		{"Django", true},
		{"Go", false},
		{"TypeScript", false},
		{"Unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFramework(tt.name)
			if result != tt.expected {
				t.Errorf("isFramework(%s) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}
