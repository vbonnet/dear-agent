package ops

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/contracts"
)

func TestClassifyTrust(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	tests := []struct {
		score int
		want  TrustTier
	}{
		{100, TrustTierPreferred},
		{80, TrustTierPreferred},
		{60, TrustTierPreferred},
		{59, TrustTierNormal},
		{50, TrustTierNormal},
		{30, TrustTierNormal},
		{29, TrustTierProbation},
		{20, TrustTierProbation},
		{19, TrustTierBlocked},
		{0, TrustTierBlocked},
	}

	for _, tt := range tests {
		got := ClassifyTrust(tt.score)
		if got != tt.want {
			t.Errorf("ClassifyTrust(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestTrustPenalty(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	tests := []struct {
		score      int
		restricted bool
	}{
		{100, false},
		{60, false},
		{30, false},
		{29, true},  // probation
		{20, true},  // probation
		{19, true},  // blocked
		{0, true},   // blocked
	}

	for _, tt := range tests {
		restricted, reason := TrustPenalty(tt.score)
		if restricted != tt.restricted {
			t.Errorf("TrustPenalty(%d) restricted = %v, want %v (reason: %s)",
				tt.score, restricted, tt.restricted, reason)
		}
		if restricted && reason == "" {
			t.Errorf("TrustPenalty(%d) restricted but no reason given", tt.score)
		}
		if !restricted && reason != "" {
			t.Errorf("TrustPenalty(%d) not restricted but reason = %q", tt.score, reason)
		}
	}
}

func TestTrustAwareDispatch_RankedByScore(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	// Create agents with different trust histories:
	// agent-high: 4 successes => 50 + 20 = 70 (preferred)
	// agent-mid:  0 events => 50 (normal)
	// agent-low:  3 false_completions => 50 - 45 = 5 (blocked)
	for i := 0; i < 4; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "agent-high", EventType: "success",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "agent-low", EventType: "false_completion",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustAwareDispatch([]string{"agent-low", "agent-mid", "agent-high"})
	if err != nil {
		t.Fatalf("TrustAwareDispatch: %v", err)
	}

	// Ranked should have agent-high (70) then agent-mid (50)
	if len(result.Ranked) != 2 {
		t.Fatalf("Ranked length = %d, want 2", len(result.Ranked))
	}
	if result.Ranked[0].SessionName != "agent-high" {
		t.Errorf("Ranked[0] = %q, want agent-high", result.Ranked[0].SessionName)
	}
	if result.Ranked[0].TrustScore != 70 {
		t.Errorf("Ranked[0].TrustScore = %d, want 70", result.Ranked[0].TrustScore)
	}
	if result.Ranked[0].TrustTier != TrustTierPreferred {
		t.Errorf("Ranked[0].TrustTier = %q, want preferred", result.Ranked[0].TrustTier)
	}
	if result.Ranked[1].SessionName != "agent-mid" {
		t.Errorf("Ranked[1] = %q, want agent-mid", result.Ranked[1].SessionName)
	}
	if result.Ranked[1].TrustScore != 50 {
		t.Errorf("Ranked[1].TrustScore = %d, want 50", result.Ranked[1].TrustScore)
	}

	// Blocked should have agent-low (5)
	if len(result.Blocked) != 1 {
		t.Fatalf("Blocked length = %d, want 1", len(result.Blocked))
	}
	if result.Blocked[0].SessionName != "agent-low" {
		t.Errorf("Blocked[0] = %q, want agent-low", result.Blocked[0].SessionName)
	}
	if result.Blocked[0].TrustTier != TrustTierBlocked {
		t.Errorf("Blocked[0].TrustTier = %q, want blocked", result.Blocked[0].TrustTier)
	}
}

func TestTrustAwareDispatch_EmptyList(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	result, err := TrustAwareDispatch(nil)
	if err != nil {
		t.Fatalf("TrustAwareDispatch: %v", err)
	}
	if len(result.Ranked) != 0 {
		t.Errorf("Ranked length = %d, want 0", len(result.Ranked))
	}
	if len(result.Blocked) != 0 {
		t.Errorf("Blocked length = %d, want 0", len(result.Blocked))
	}
}

func TestTrustAwareDispatch_AllNew(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	// All new agents get base score (50) — all should be in Ranked
	result, err := TrustAwareDispatch([]string{"new-a", "new-b", "new-c"})
	if err != nil {
		t.Fatalf("TrustAwareDispatch: %v", err)
	}
	if len(result.Ranked) != 3 {
		t.Errorf("Ranked length = %d, want 3", len(result.Ranked))
	}
	if len(result.Blocked) != 0 {
		t.Errorf("Blocked length = %d, want 0", len(result.Blocked))
	}
	for _, c := range result.Ranked {
		if c.TrustScore != 50 {
			t.Errorf("%s TrustScore = %d, want 50", c.SessionName, c.TrustScore)
		}
		if c.TrustTier != TrustTierNormal {
			t.Errorf("%s TrustTier = %q, want normal", c.SessionName, c.TrustTier)
		}
	}
}

func TestTrustAwareDispatch_ProbationTier(t *testing.T) {
	cleanup := setupTrustDir(t)
	defer cleanup()
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	// Create agent with score in probation range (20-29)
	// 2 false_completions: 50 - 30 = 20 (probation)
	for i := 0; i < 2; i++ {
		if _, err := TrustRecord(nil, &TrustRecordRequest{
			SessionName: "prob-agent", EventType: "false_completion",
		}); err != nil {
			t.Fatalf("TrustRecord: %v", err)
		}
	}

	result, err := TrustAwareDispatch([]string{"prob-agent"})
	if err != nil {
		t.Fatalf("TrustAwareDispatch: %v", err)
	}

	// Score 20 is >= minDispatch (30)? No, 20 < 30, so should be blocked
	// Wait: 20 < 30 means blocked. Let me recalculate: probation is >= 20 && < 30
	// but minDispatch is 30, so probation agents are below minDispatch and go to Blocked.
	// That's by design — probation agents don't get unsupervised work.
	if len(result.Blocked) != 1 {
		t.Fatalf("Blocked length = %d, want 1", len(result.Blocked))
	}
	if result.Blocked[0].TrustTier != TrustTierProbation {
		t.Errorf("TrustTier = %q, want probation", result.Blocked[0].TrustTier)
	}
}

func TestTrustThresholds(t *testing.T) {
	contracts.ResetForTesting()
	defer contracts.ResetForTesting()

	minD, pref, prob := TrustThresholds()
	if minD != 30 {
		t.Errorf("MinDispatchScore = %d, want 30", minD)
	}
	if pref != 60 {
		t.Errorf("PreferredScore = %d, want 60", pref)
	}
	if prob != 20 {
		t.Errorf("ProbationScore = %d, want 20", prob)
	}
}
