package context

import (
	"testing"
	"time"
)

func TestGetSessionThresholds(t *testing.T) {
	tests := []struct {
		name     string
		st       SessionType
		wantPrev float64
		wantComp float64
		wantRot  float64
	}{
		{"orchestrator", SessionOrchestrator, 55, 65, 80},
		{"worker", SessionWorker, 70, 80, 90},
		{"meta-orchestrator", SessionMetaOrchestrator, 50, 60, 75},
		{"unknown defaults to orchestrator", SessionType("unknown"), 55, 65, 80},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := GetSessionThresholds(tt.st)
			if th.Prevention != tt.wantPrev {
				t.Errorf("Prevention = %v, want %v", th.Prevention, tt.wantPrev)
			}
			if th.Compaction != tt.wantComp {
				t.Errorf("Compaction = %v, want %v", th.Compaction, tt.wantComp)
			}
			if th.Rotation != tt.wantRot {
				t.Errorf("Rotation = %v, want %v", th.Rotation, tt.wantRot)
			}
		})
	}
}

func newTestEngine(t *testing.T) *CompactEngine {
	t.Helper()
	reg, err := NewRegistry("")
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	return NewCompactEngine(reg)
}

func TestCompact_RequiresFocus(t *testing.T) {
	engine := newTestEngine(t)
	result := engine.Compact(&CompactConfig{
		Focus:       "",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if result.Success {
		t.Error("expected failure when focus is empty")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestCompact_BelowThresholds(t *testing.T) {
	engine := newTestEngine(t)
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve API design context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     80000,
			PercentageUsed: 40.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Layer != "none" {
		t.Errorf("expected layer none, got %s", result.Layer)
	}
	if result.ReductionPct != 0 {
		t.Errorf("expected 0 reduction, got %v", result.ReductionPct)
	}
}

func TestCompact_PreventionLayer(t *testing.T) {
	engine := newTestEngine(t)
	// Orchestrator prevention threshold is 55%
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve API design context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     120000,
			PercentageUsed: 60.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Layer != "prevention" {
		t.Errorf("expected layer prevention, got %s", result.Layer)
	}
	if result.ReductionPct != 10.0 {
		t.Errorf("expected 10%% reduction, got %v", result.ReductionPct)
	}
}

func TestCompact_CompactionLayer(t *testing.T) {
	engine := newTestEngine(t)
	// Orchestrator compaction threshold is 65%
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve API design context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Layer != "compaction" {
		t.Errorf("expected layer compaction, got %s", result.Layer)
	}
	if result.ReductionPct != 20.0 {
		t.Errorf("expected 20%% reduction for conservative, got %v", result.ReductionPct)
	}
}

func TestCompact_CompactionLayerAggressive(t *testing.T) {
	engine := newTestEngine(t)
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve API design context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyAggressive,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Layer != "compaction" {
		t.Errorf("expected layer compaction, got %s", result.Layer)
	}
	if result.ReductionPct != 35.0 {
		t.Errorf("expected 35%% reduction for aggressive, got %v", result.ReductionPct)
	}
}

func TestCompact_RotationLayer(t *testing.T) {
	engine := newTestEngine(t)
	// Orchestrator rotation threshold is 80%
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve API design context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     170000,
			PercentageUsed: 85.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Layer != "rotation" {
		t.Errorf("expected layer rotation, got %s", result.Layer)
	}
	if result.ReductionPct != 80.0 {
		t.Errorf("expected 80%% reduction for rotation, got %v", result.ReductionPct)
	}
}

func TestCompact_WorkerThresholds(t *testing.T) {
	engine := newTestEngine(t)
	// Worker prevention=70, compaction=80, rotation=90
	// At 75%, should be in prevention layer
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve test context",
		SessionType: SessionWorker,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     150000,
			PercentageUsed: 75.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if result.Layer != "prevention" {
		t.Errorf("expected prevention for worker at 75%%, got %s", result.Layer)
	}
}

func TestCompact_MetaOrchestratorThresholds(t *testing.T) {
	engine := newTestEngine(t)
	// Meta-orchestrator prevention=50, compaction=60, rotation=75
	// At 62%, should be in compaction layer
	result := engine.Compact(&CompactConfig{
		Focus:       "preserve coordination context",
		SessionType: SessionMetaOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     124000,
			PercentageUsed: 62.0,
			ModelID:        "claude-sonnet-4.5",
		},
	})

	if result.Layer != "compaction" {
		t.Errorf("expected compaction for meta-orchestrator at 62%%, got %s", result.Layer)
	}
}

func TestCompact_AntiLoopCooldown(t *testing.T) {
	engine := newTestEngine(t)
	cfg := &CompactConfig{
		Focus:       "preserve context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "claude-sonnet-4.5",
		},
	}

	// First 3 compactions should succeed
	for i := 0; i < MaxCompactions; i++ {
		result := engine.Compact(cfg)
		if !result.Success {
			t.Errorf("compaction %d should succeed, got error: %s", i+1, result.Error)
		}
	}

	// 4th should be blocked
	result := engine.Compact(cfg)
	if result.Success {
		t.Error("4th compaction should be blocked by cooldown")
	}
	if result.Cooldown == nil || !result.Cooldown.Active {
		t.Error("cooldown should be active")
	}
}

func TestCompact_CooldownExpiry(t *testing.T) {
	engine := newTestEngine(t)
	// Inject old timestamps that are beyond cooldown
	past := time.Now().Add(-3 * time.Hour)
	engine.history = []time.Time{past, past, past}

	cfg := &CompactConfig{
		Focus:       "preserve context",
		SessionType: SessionOrchestrator,
		Strategy:    StrategyConservative,
		Usage: &Usage{
			TotalTokens:    200000,
			UsedTokens:     140000,
			PercentageUsed: 70.0,
			ModelID:        "claude-sonnet-4.5",
		},
	}

	// Should succeed because old entries expired
	result := engine.Compact(cfg)
	if !result.Success {
		t.Errorf("should succeed after cooldown expires, got error: %s", result.Error)
	}
}

func TestParseSessionType(t *testing.T) {
	tests := []struct {
		input   string
		want    SessionType
		wantErr bool
	}{
		{"orchestrator", SessionOrchestrator, false},
		{"worker", SessionWorker, false},
		{"meta-orchestrator", SessionMetaOrchestrator, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseSessionType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseStrategy(t *testing.T) {
	tests := []struct {
		input   string
		want    Strategy
		wantErr bool
	}{
		{"conservative", StrategyConservative, false},
		{"aggressive", StrategyAggressive, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseStrategy(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}
