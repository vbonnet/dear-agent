package main

import (
	"context"
	"testing"
)

func TestVerifyExactReplyPlacementRejectsIDOnlyOrIncompleteEvidenceBeforeProviderAccess(t *testing.T) {
	const (
		threadID    = "PRRT_incomplete_placement_evidence"
		predecessor = "PRRC_predecessor"
		reply       = "PRRC_reply"
		bodyFile    = "/tmp/resolve-review-thread.INCOMPLETE"
		openingBody = "P1: prove exact placement."
		replyBody   = "Exact placement is proved."
	)
	predecessorBodySHA256 := exactBodySHA256([]byte(openingBody))
	replyBodySHA256 := exactBodySHA256([]byte(replyBody))
	tests := []struct {
		name     string
		evidence resolutionEvidence
	}{
		{
			name:     "reply id only",
			evidence: resolutionEvidence{LastID: reply},
		},
		{
			name: "both ids only",
			evidence: resolutionEvidence{
				LastID:        reply,
				PredecessorID: predecessor,
			},
		},
		{
			name: "predecessor digest missing",
			evidence: resolutionEvidence{
				LastID:        reply,
				PredecessorID: predecessor,
				BodySHA256:    replyBodySHA256,
			},
		},
		{
			name: "reply digest missing",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: predecessorBodySHA256,
			},
		},
		{
			name: "predecessor id missing",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "predecessor and reply IDs equal",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         reply,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "predecessor digest invalid",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: "not-a-digest",
				BodySHA256:            replyBodySHA256,
			},
		},
		{
			name: "reply digest invalid",
			evidence: resolutionEvidence{
				LastID:                reply,
				PredecessorID:         predecessor,
				PredecessorBodySHA256: predecessorBodySHA256,
				BodySHA256:            "not-a-digest",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t)
			code, stdout, diagnostics := provider.capture(func() int {
				_, code := verifyReplyPlacementAgainst(
					context.Background(), threadID, tc.evidence, bodyFile,
				)
				return code
			})
			if code == 0 {
				t.Fatalf("incomplete exact placement evidence succeeded:\n%s", diagnostics)
			}
			if stdout != "" {
				t.Errorf("incomplete exact placement evidence printed success output: %q", stdout)
			}
			provider.assertExhausted()
			assertContainsAll(t, diagnostics, "exact placement evidence", "resolution was not attempted")
		})
	}
}
