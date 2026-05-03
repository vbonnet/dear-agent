package metacontext

import (
	"testing"
)

// ============================================================================
// Unit Tests: types.go (estimateTokens, validateSize, Clone, AllSignals)
// S7 Plan: Week 4 Testing, Unit Test Category
// ============================================================================

// TestEstimateTokens_EmptyMetacontext tests token estimation for empty metacontext
func TestEstimateTokens_EmptyMetacontext(t *testing.T) {
	mc := &Metacontext{
		Languages:   []Signal{},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	tokens := estimateTokens(mc)
	if tokens != 0 {
		t.Errorf("Empty metacontext should estimate 0 tokens, got %d", tokens)
	}
}

// TestEstimateTokens_SingleSignal tests token estimation for single signal
func TestEstimateTokens_SingleSignal(t *testing.T) {
	mc := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
		},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	tokens := estimateTokens(mc)
	// "Go" (2 chars) + "file" (4 chars) = 6 chars / 4 = 1.5 ≈ 2 tokens
	// Plus JSON overhead (field names, braces, etc.) ≈ 10-15 tokens
	if tokens < 1 || tokens > 30 {
		t.Errorf("Single signal should estimate 1-30 tokens, got %d", tokens)
	}
}

// TestEstimateTokens_MaxSignals tests token estimation at signal limits
func TestEstimateTokens_MaxSignals(t *testing.T) {
	mc := &Metacontext{
		Languages:  make([]Signal, MaxLanguageSignals),  // 10
		Frameworks: make([]Signal, MaxFrameworkSignals), // 15
		Tools:      make([]Signal, MaxToolSignals),      // 20
		Conventions: []Convention{
			{Type: "naming", Description: "snake_case for variables", Confidence: 0.9},
			{Type: "naming", Description: "PascalCase for types", Confidence: 0.9},
		},
		Personas: []Persona{
			{Name: "Senior Engineer", Description: "10 years Go experience", Score: 0.95, Signals: []string{"Go"}},
		},
	}

	// Populate signals
	for i := 0; i < MaxLanguageSignals; i++ {
		mc.Languages[i] = Signal{Name: "Language", Confidence: 0.9, Source: "file"}
	}
	for i := 0; i < MaxFrameworkSignals; i++ {
		mc.Frameworks[i] = Signal{Name: "Framework", Confidence: 0.9, Source: "dependency"}
	}
	for i := 0; i < MaxToolSignals; i++ {
		mc.Tools[i] = Signal{Name: "Tool", Confidence: 0.8, Source: "git"}
	}

	tokens := estimateTokens(mc)
	// Should be well under 5000 tokens at max signal counts
	if tokens > MaxMetacontextTokens {
		t.Errorf("Max signals should fit in token budget, got %d > %d", tokens, MaxMetacontextTokens)
	}
	// But should be substantial (>100 tokens)
	if tokens < 100 {
		t.Errorf("Max signals should estimate >100 tokens, got %d", tokens)
	}
}

// TestValidateSize_UnderBudget tests validation passes for small metacontext
func TestValidateSize_UnderBudget(t *testing.T) {
	mc := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
		},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	err := validateSize(mc)
	if err != nil {
		t.Errorf("Small metacontext should pass validation, got error: %v", err)
	}
}

// TestValidateSize_ExceedsBudget tests validation fails for oversized metacontext
func TestValidateSize_ExceedsBudget(t *testing.T) {
	// Create artificially large metacontext (force token budget overflow)
	mc := &Metacontext{
		Languages:   make([]Signal, 500), // Way over limit
		Frameworks:  make([]Signal, 500), // Way over limit
		Tools:       make([]Signal, 500), // Way over limit
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	// Populate with long names to inflate token count
	for i := 0; i < 500; i++ {
		mc.Languages[i] = Signal{
			Name:       "VeryLongLanguageNameThatExceedsReasonableLengthForTestingPurposes",
			Confidence: 0.9,
			Source:     "file",
			Metadata:   map[string]string{"key": "verylongvalue"},
		}
		mc.Frameworks[i] = Signal{
			Name:       "VeryLongFrameworkNameThatExceedsReasonableLengthForTestingPurposes",
			Confidence: 0.9,
			Source:     "dependency",
		}
		mc.Tools[i] = Signal{
			Name:       "VeryLongToolNameThatExceedsReasonableLengthForTestingPurposes",
			Confidence: 0.8,
			Source:     "git",
		}
	}

	err := validateSize(mc)
	if err == nil {
		t.Error("Oversized metacontext should fail validation")
	}
	// Error wrapping check - actual error wrapped with %w
	// Just check it's not nil (specific error type tested in unit tests)
	// Note: Should use errors.Is() if checking wrapped errors
}

// TestClone_DeepCopy tests Clone() creates independent copy
func TestClone_DeepCopy(t *testing.T) {
	original := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
		},
		Frameworks: []Signal{
			{Name: "Gin", Confidence: 0.9, Source: "dependency"},
		},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
		Metadata:    MinimalMetadata{CacheHit: true, Warnings: []string{"test"}},
	}

	cloned := original.Clone()

	// Verify deep copy (modify original, check cloned not affected)
	original.Languages[0].Name = "MODIFIED"
	original.Frameworks = append(original.Frameworks, Signal{Name: "NEW", Confidence: 0.5, Source: "test"})
	original.Metadata.CacheHit = false

	if cloned.Languages[0].Name == "MODIFIED" {
		t.Error("Clone should be independent, but Languages was modified")
	}
	if len(cloned.Frameworks) != 1 {
		t.Errorf("Clone should have 1 framework, got %d", len(cloned.Frameworks))
	}
	if cloned.Metadata.CacheHit != true {
		t.Error("Clone should preserve original metadata")
	}
}

// TestClone_EmptyMetacontext tests Clone() handles empty metacontext
func TestClone_EmptyMetacontext(t *testing.T) {
	original := &Metacontext{
		Languages:   []Signal{},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	cloned := original.Clone()

	if cloned == nil {
		t.Fatal("Clone should not return nil")
	}
	if len(cloned.Languages) != 0 {
		t.Errorf("Cloned empty metacontext should have 0 languages, got %d", len(cloned.Languages))
	}
}

// TestAllSignals_ReturnsAllSignalTypes tests AllSignals() aggregates correctly
func TestAllSignals_ReturnsAllSignalTypes(t *testing.T) {
	mc := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
			{Name: "TypeScript", Confidence: 0.8, Source: "file"},
		},
		Frameworks: []Signal{
			{Name: "Gin", Confidence: 0.9, Source: "dependency"},
		},
		Tools: []Signal{
			{Name: "Git", Confidence: 1.0, Source: "git"},
		},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	allSignals := mc.AllSignals()

	// Should return 2 languages + 1 framework + 1 tool = 4 signals
	if len(allSignals) != 4 {
		t.Errorf("AllSignals should return 4 signals, got %d", len(allSignals))
	}

	// Verify all signal types present
	hasGo := false
	hasTypeScript := false
	hasGin := false
	hasGit := false

	for _, sig := range allSignals {
		switch sig.Name {
		case "Go":
			hasGo = true
		case "TypeScript":
			hasTypeScript = true
		case "Gin":
			hasGin = true
		case "Git":
			hasGit = true
		}
	}

	if !hasGo || !hasTypeScript || !hasGin || !hasGit {
		t.Error("AllSignals missing expected signals")
	}
}

// TestAllSignals_EmptyMetacontext tests AllSignals() with no signals
func TestAllSignals_EmptyMetacontext(t *testing.T) {
	mc := &Metacontext{
		Languages:   []Signal{},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	allSignals := mc.AllSignals()

	if len(allSignals) != 0 {
		t.Errorf("Empty metacontext should return 0 signals, got %d", len(allSignals))
	}
}

// TestSignalConstants tests signal limit constants are sensible
func TestSignalConstants(t *testing.T) {
	if MaxMetacontextTokens != 5000 {
		t.Errorf("MaxMetacontextTokens should be 5000, got %d", MaxMetacontextTokens)
	}
	if MaxLanguageSignals != 10 {
		t.Errorf("MaxLanguageSignals should be 10, got %d", MaxLanguageSignals)
	}
	if MaxFrameworkSignals != 15 {
		t.Errorf("MaxFrameworkSignals should be 15, got %d", MaxFrameworkSignals)
	}
	if MaxToolSignals != 20 {
		t.Errorf("MaxToolSignals should be 20, got %d", MaxToolSignals)
	}
	if MaxConventions != 10 {
		t.Errorf("MaxConventions should be 10, got %d", MaxConventions)
	}
	if MaxPersonas != 5 {
		t.Errorf("MaxPersonas should be 5, got %d", MaxPersonas)
	}
}
