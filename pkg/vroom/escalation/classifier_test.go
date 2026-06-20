package escalation

import (
	"context"
	"testing"
)

func TestDefaultClassifier(t *testing.T) {
	c := DefaultClassifier{}
	ctx := context.Background()

	cases := []struct {
		name       string
		kind       Kind
		question   string
		wantDisp   Disposition
		wantHuman  bool
		wantAnswer bool // expect a non-empty auto answer
	}{
		{
			name:       "proceed with assigned task auto-resolves",
			kind:       KindQuestion,
			question:   "Should I proceed with the task you asked me to do?",
			wantDisp:   DispAutoResolve,
			wantAnswer: true,
		},
		{
			name:     "may I continue auto-resolves",
			kind:     KindQuestion,
			question: "Can I go ahead and run the tests now?",
			wantDisp: DispAutoResolve, wantAnswer: true,
		},
		{
			name:     "explicit assigned-task reference auto-resolves",
			kind:     KindQuestion,
			question: "Is the task you assigned me the one I should be working on?",
			wantDisp: DispAutoResolve, wantAnswer: true,
		},
		{
			name:      "product decision must reach human",
			kind:      KindDecision,
			question:  "Should we make this product decision to drop the free tier?",
			wantDisp:  DispRouteToHuman,
			wantHuman: true,
		},
		{
			name:      "proceed dressed over a product decision still reaches human",
			kind:      KindQuestion,
			question:  "Should I proceed with the pricing change to $99/mo?",
			wantDisp:  DispRouteToHuman,
			wantHuman: true,
		},
		{
			name:      "deleting production must reach human",
			kind:      KindBlockedAction,
			question:  "I need to delete the production database to fix the migration",
			wantDisp:  DispRouteToHuman,
			wantHuman: true,
		},
		{
			name:     "ordinary how-to question routes to supervisor",
			kind:     KindQuestion,
			question: "Which logging library does this repo prefer?",
			wantDisp: DispRouteToSupervisor,
		},
		{
			name:     "decision without high-stakes marker routes to supervisor",
			kind:     KindDecision,
			question: "Should I split this 400-line function into two?",
			wantDisp: DispRouteToSupervisor,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			esc := &Escalation{Kind: tc.kind, Question: tc.question}
			v, err := c.Classify(ctx, esc)
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if v.Disposition != tc.wantDisp {
				t.Errorf("disposition = %q, want %q (reason: %s)", v.Disposition, tc.wantDisp, v.Reason)
			}
			if v.MustReachHuman != tc.wantHuman {
				t.Errorf("MustReachHuman = %v, want %v", v.MustReachHuman, tc.wantHuman)
			}
			if tc.wantAnswer && v.Answer == "" {
				t.Errorf("expected a non-empty auto answer, got empty")
			}
			if !tc.wantAnswer && v.Answer != "" {
				t.Errorf("expected no auto answer, got %q", v.Answer)
			}
		})
	}
}

func TestNormalizeAndHashQuestion(t *testing.T) {
	a := NormalizeQuestion("  Which   API version??  ")
	b := NormalizeQuestion("which api version")
	if a != b {
		t.Errorf("normalize mismatch: %q vs %q", a, b)
	}
	if HashQuestion(a) != HashQuestion(b) {
		t.Errorf("hash mismatch for equal normalized questions")
	}
	if HashQuestion(a) == HashQuestion(NormalizeQuestion("a different question")) {
		t.Errorf("hash collision for different questions")
	}
}
