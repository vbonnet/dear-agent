package modelrouter

import "testing"

func TestParseTier(t *testing.T) {
	tests := []struct {
		in   string
		want Tier
		ok   bool
	}{
		{"cheap", TierCheap, true},
		{"mid", TierMid, true},
		{"expensive", TierExpensive, true},
		{"free", "", false},
		{"", "", false},
		{"CHEAP", "", false}, // case-sensitive
	}
	for _, tc := range tests {
		got, ok := ParseTier(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseTier(%q) = (%v, %v), want (%v, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestClassifyPrompt(t *testing.T) {
	tests := []struct {
		prompt string
		want   TaskType
	}{
		// cheap
		{"health check endpoint is alive", TaskHealthCheck},
		{"run gofmt on all Go source files", TaskFormatting},
		{"extract data from the CSV file", TaskExtraction},
		{"fix typo in the CLAUDE.md file", TaskSimpleCode},
		// expensive
		{"write adr for the new distributed auth system", TaskArchitecture},
		{"meta-o loop step for governance gate", TaskSupervisor},
		// mid
		{"code review the new session manager PR", TaskPRReview},
		{"implement the user auth feature", TaskCodeGen},
		{"write tests for pkg/llm/router", TaskCodeGen},
		{"fix bug in routing logic", TaskCodeGen},
		{"summarize the weekly meeting notes", TaskReportAnalysis},
		// unknown
		{"unknown random prompt with no keywords", TaskUnknown},
	}
	for _, tc := range tests {
		got := ClassifyPrompt(tc.prompt)
		if got != tc.want {
			t.Errorf("ClassifyPrompt(%q) = %v, want %v", tc.prompt, got, tc.want)
		}
	}
}

func TestRoute_ExplicitTier(t *testing.T) {
	d, err := Route("claude-code", "cheap", "", "some prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Tier != TierCheap {
		t.Errorf("tier = %v, want cheap", d.Tier)
	}
	if d.Model != "haiku" {
		t.Errorf("model = %v, want haiku", d.Model)
	}
	if !d.ExplicitTier {
		t.Errorf("ExplicitTier should be true for explicit flag")
	}
}

func TestRoute_InvalidTier(t *testing.T) {
	_, err := Route("claude-code", "ultra", "", "some prompt")
	if err == nil {
		t.Error("expected error for unknown tier, got nil")
	}
}

func TestRoute_PromptHeuristic(t *testing.T) {
	// Architecture decision prompt → expensive → opus
	d, err := Route("claude-code", "", "", "write adr for the distributed auth migration plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Tier != TierExpensive {
		t.Errorf("tier = %v, want expensive", d.Tier)
	}
	if d.Model != "opus" {
		t.Errorf("model = %v, want opus", d.Model)
	}
}

func TestRoute_BeadOverride(t *testing.T) {
	// Bead says expensive, prompt is a health check → bead wins
	d, err := Route("claude-code", "", "expensive", "health check the mesh")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Tier != TierExpensive {
		t.Errorf("tier = %v, want expensive", d.Tier)
	}
}

func TestRoute_ExplicitBeatsBeadOverride(t *testing.T) {
	// Explicit CLI flag wins over bead override
	d, err := Route("claude-code", "cheap", "expensive", "some prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Tier != TierCheap {
		t.Errorf("explicit flag should win: got tier=%v, want cheap", d.Tier)
	}
	if !d.ExplicitTier {
		t.Error("ExplicitTier should be true")
	}
}

func TestRoute_UnknownHarness(t *testing.T) {
	d, err := Route("unknown-harness", "mid", "", "some prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No tier mapping for unknown harness — model should be empty
	if d.Model != "" {
		t.Errorf("expected empty model for unknown harness, got %q", d.Model)
	}
}

func TestModelForTier_AllHarnesses(t *testing.T) {
	// Verify every harness+tier combo has a non-empty model
	for harness, tiers := range TierModels {
		for tier, model := range tiers {
			if model == "" {
				t.Errorf("TierModels[%q][%q] is empty", harness, tier)
			}
		}
	}
}
