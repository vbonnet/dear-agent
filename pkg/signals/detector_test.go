package signals

import (
	"testing"
)

func TestDetectKeywordSignals(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int // expected number of signals
	}{
		{
			name:     "Tier 1 compliance keyword",
			text:     "We need to implement HIPAA compliance for this feature",
			expected: 1,
		},
		{
			name:     "Tier 2 integration keyword",
			text:     "Setting up OAuth authentication",
			expected: 1,
		},
		{
			name:     "Multiple keywords",
			text:     "GDPR compliance with OAuth and SSO",
			expected: 3,
		},
		{
			name:     "No keywords",
			text:     "Simple bug fix in the UI",
			expected: 0,
		},
		{
			name:     "Case insensitive",
			text:     "gdpr and oauth integration",
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detectKeywordSignals(tt.text)
			if len(signals) != tt.expected {
				t.Errorf("expected %d signals, got %d", tt.expected, len(signals))
			}
		})
	}
}

func TestDetectEffortSignals(t *testing.T) {
	tests := []struct {
		name     string
		hours    *float64
		expected int
	}{
		{
			name:     "No beads task",
			hours:    nil,
			expected: 0,
		},
		{
			name:     "20 hours (comprehensive)",
			hours:    floatPtr(20.0),
			expected: 1,
		},
		{
			name:     "8 hours (thorough)",
			hours:    floatPtr(8.0),
			expected: 1,
		},
		{
			name:     "2 hours (standard)",
			hours:    floatPtr(2.0),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := Context{}
			if tt.hours != nil {
				ctx.BeadsTask = &BeadsTask{
					EstimatedHours: tt.hours,
				}
			}
			signals := detectEffortSignals(ctx)
			if len(signals) != tt.expected {
				t.Errorf("expected %d signals, got %d", tt.expected, len(signals))
			}
		})
	}
}

func TestDetectFileSignals(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		expected int
	}{
		{
			name:     "Auth file",
			files:    []string{"auth.go"},
			expected: 1,
		},
		{
			name:     "OAuth file",
			files:    []string{"oauth-handler.ts"},
			expected: 2, // Matches both oauth pattern and auth pattern
		},
		{
			name:     "Multiple matching files",
			files:    []string{"auth.go", "oauth.py", "security.md"},
			expected: 4, // auth.go(1) + oauth.py(2) + security.md(1)
		},
		{
			name:     "No matching files",
			files:    []string{"utils.go", "helpers.ts"},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detectFileSignals(tt.files)
			if len(signals) != tt.expected {
				t.Errorf("expected %d signals, got %d", tt.expected, len(signals))
			}
		})
	}
}

func TestDetectBeadsSignals(t *testing.T) {
	tests := []struct {
		name     string
		labels   []string
		expected int
	}{
		{
			name:     "Security label",
			labels:   []string{"security"},
			expected: 1,
		},
		{
			name:     "Multiple labels",
			labels:   []string{"security", "compliance"},
			expected: 2,
		},
		{
			name:     "Unknown label",
			labels:   []string{"unknown"},
			expected: 0,
		},
		{
			name:     "No labels",
			labels:   []string{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			beadsTask := &BeadsTask{Labels: tt.labels}
			signals := detectBeadsSignals(beadsTask)
			if len(signals) != tt.expected {
				t.Errorf("expected %d signals, got %d", tt.expected, len(signals))
			}
		})
	}
}

func TestDetectPreviousPhaseSignals(t *testing.T) {
	tests := []struct {
		name     string
		phases   []PreviousPhaseOutput
		expected int
	}{
		{
			name:     "No previous phases",
			phases:   []PreviousPhaseOutput{},
			expected: 0,
		},
		{
			name: "Previous comprehensive phase",
			phases: []PreviousPhaseOutput{
				{Phase: "design", Level: RigorLevelComprehensive},
			},
			expected: 1,
		},
		{
			name: "Previous thorough phase",
			phases: []PreviousPhaseOutput{
				{Phase: "design", Level: RigorLevelThorough},
			},
			expected: 1,
		},
		{
			name: "Previous minimal phase",
			phases: []PreviousPhaseOutput{
				{Phase: "design", Level: RigorLevelMinimal},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signals := detectPreviousPhaseSignals(tt.phases)
			if len(signals) != tt.expected {
				t.Errorf("expected %d signals, got %d", tt.expected, len(signals))
			}
		})
	}
}

func TestFuseSignals(t *testing.T) {
	tests := []struct {
		name          string
		signals       []Signal
		expectedLevel RigorLevelName
		minConfidence float64
		maxConfidence float64
	}{
		{
			name:          "No signals",
			signals:       []Signal{},
			expectedLevel: RigorLevelMinimal,
			minConfidence: 0.0,
			maxConfidence: 0.5,
		},
		{
			name: "High confidence comprehensive",
			signals: []Signal{
				{Type: SignalTypeKeyword, Value: "HIPAA", Confidence: 0.90, Weight: 0.4},
			},
			expectedLevel: RigorLevelComprehensive,
			minConfidence: 0.85,
			maxConfidence: 1.0,
		},
		{
			name: "Medium confidence thorough",
			signals: []Signal{
				{Type: SignalTypeKeyword, Value: "OAuth", Confidence: 0.70, Weight: 0.3},
			},
			expectedLevel: RigorLevelThorough,
			minConfidence: 0.60,
			maxConfidence: 0.85,
		},
		{
			name: "Multiple signals fusion",
			signals: []Signal{
				{Type: SignalTypeKeyword, Value: "GDPR", Confidence: 0.90, Weight: 0.4},
				{Type: SignalTypeEffort, Value: "20 hours", Confidence: 0.80, Weight: 0.3},
			},
			expectedLevel: RigorLevelComprehensive,
			minConfidence: 0.85,
			maxConfidence: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confidence, level := fuseSignals(tt.signals)
			if level != tt.expectedLevel {
				t.Errorf("expected level %s, got %s", tt.expectedLevel, level)
			}
			if confidence < tt.minConfidence || confidence > tt.maxConfidence {
				t.Errorf("expected confidence in range [%.2f, %.2f], got %.2f",
					tt.minConfidence, tt.maxConfidence, confidence)
			}
		})
	}
}

func TestAnalyzeContext(t *testing.T) {
	tests := []struct {
		name           string
		ctx            Context
		expectedLevel  RigorLevelName
		expectedAction UserAction
	}{
		{
			name: "HIPAA compliance - auto escalate",
			ctx: Context{
				UserDescription: "Implement HIPAA compliance for patient data",
			},
			expectedLevel:  RigorLevelComprehensive,
			expectedAction: UserActionAuto,
		},
		{
			name: "OAuth integration - offer escalation",
			ctx: Context{
				UserDescription: "Add OAuth authentication",
			},
			expectedLevel:  RigorLevelThorough,
			expectedAction: UserActionOffer,
		},
		{
			name: "Simple bug fix - stay minimal",
			ctx: Context{
				UserDescription: "Fix typo in UI button",
			},
			expectedLevel:  RigorLevelMinimal,
			expectedAction: UserActionNone,
		},
		{
			name: "Multiple signals",
			ctx: Context{
				UserDescription: "GDPR compliance with OAuth",
				BeadsTask: &BeadsTask{
					Labels:         []string{"security", "compliance"},
					EstimatedHours: floatPtr(25.0),
				},
			},
			expectedLevel:  RigorLevelThorough, // Weighted avg: (0.9*0.4 + 0.7*0.3 + 0.8*0.3 + 0.65*0.1 + 0.75*0.1)/(0.4+0.3+0.3+0.1+0.1) = 0.795
			expectedAction: UserActionOffer,    // Confidence ~0.795 < 0.80 threshold for auto
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := AnalyzeContext(tt.ctx)
			if decision.SuggestedLevel != tt.expectedLevel {
				t.Errorf("expected level %s, got %s", tt.expectedLevel, decision.SuggestedLevel)
			}
			if decision.UserAction != tt.expectedAction {
				t.Errorf("expected action %s, got %s", tt.expectedAction, decision.UserAction)
			}
		})
	}
}

func TestLogEscalation(t *testing.T) {
	decision := EscalationDecision{
		ShouldEscalate: true,
		SuggestedLevel: RigorLevelComprehensive,
		Confidence:     0.90,
		Reasoning:      []string{"HIPAA detected"},
		Signals:        []Signal{{Type: SignalTypeKeyword, Value: "HIPAA"}},
		UserAction:     UserActionAuto,
	}

	log := LogEscalation("design", decision, nil)

	if log.Phase != "design" {
		t.Errorf("expected phase 'design', got '%s'", log.Phase)
	}
	if log.Decision != EscalationLogDecisionAutoEscalate {
		t.Errorf("expected decision 'auto-escalate', got '%s'", log.Decision)
	}
	if log.ToLevel != RigorLevelComprehensive {
		t.Errorf("expected toLevel 'comprehensive', got '%s'", log.ToLevel)
	}
	if log.Confidence != 0.90 {
		t.Errorf("expected confidence 0.90, got %.2f", log.Confidence)
	}
}

func TestLogEscalationWithOverride(t *testing.T) {
	decision := EscalationDecision{
		ShouldEscalate: true,
		SuggestedLevel: RigorLevelComprehensive,
		Confidence:     0.90,
		Reasoning:      []string{"HIPAA detected"},
		Signals:        []Signal{{Type: SignalTypeKeyword, Value: "HIPAA"}},
		UserAction:     UserActionAuto,
	}

	override := RigorLevelMinimal
	log := LogEscalation("design", decision, &override)

	if log.ToLevel != RigorLevelMinimal {
		t.Errorf("expected toLevel 'minimal' (user override), got '%s'", log.ToLevel)
	}
	if log.UserOverride == nil || *log.UserOverride != RigorLevelMinimal {
		t.Errorf("expected userOverride 'minimal', got %v", log.UserOverride)
	}
}

// Helper function to create float pointer
func floatPtr(f float64) *float64 {
	return &f
}
