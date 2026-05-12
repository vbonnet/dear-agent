package audit

import "testing"

func TestProposalLayer_IsValid(t *testing.T) {
	tests := []struct {
		layer ProposalLayer
		want  bool
	}{
		{ProposalDefine, true},
		{ProposalEnforce, true},
		{ProposalLayer("audit"), false},
		{ProposalLayer("resolve"), false},
		{ProposalLayer(""), false},
		{ProposalLayer("DEFINE"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.layer), func(t *testing.T) {
			if got := tt.layer.IsValid(); got != tt.want {
				t.Errorf("%q.IsValid() = %v, want %v", tt.layer, got, tt.want)
			}
		})
	}
}

func TestProposalState_IsValid(t *testing.T) {
	tests := []struct {
		state ProposalState
		want  bool
	}{
		{ProposalProposed, true},
		{ProposalAccepted, true},
		{ProposalRejected, true},
		{ProposalExpired, true},
		{ProposalState(""), false},
		{ProposalState("withdrawn"), false},
		{ProposalState("PROPOSED"), false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			if got := tt.state.IsValid(); got != tt.want {
				t.Errorf("%q.IsValid() = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestProposal_Validate(t *testing.T) {
	good := Proposal{
		Layer:     ProposalDefine,
		Title:     "Add invariant",
		Rationale: "Findings show repeated define-time drift",
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}

	t.Run("invalid layer", func(t *testing.T) {
		p := good
		p.Layer = ProposalLayer("bogus")
		if err := p.Validate(); err == nil {
			t.Error("expected error for invalid Layer")
		}
	})
	t.Run("empty title", func(t *testing.T) {
		p := good
		p.Title = ""
		if err := p.Validate(); err == nil {
			t.Error("expected error for empty Title")
		}
	})
	t.Run("empty rationale", func(t *testing.T) {
		p := good
		p.Rationale = ""
		if err := p.Validate(); err == nil {
			t.Error("expected error for empty Rationale")
		}
	})
}
