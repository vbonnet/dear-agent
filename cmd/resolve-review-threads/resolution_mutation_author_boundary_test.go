package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestResolveMutationRequestsAndParsesAtomicAuthorBoundary(t *testing.T) {
	for _, selection := range []string{
		"opening: comments(first:1)",
		"recent: comments(last:2)",
		"author { login }",
	} {
		if !strings.Contains(resolveMutation, selection) {
			t.Errorf("resolve mutation omitted %q:\n%s", selection, resolveMutation)
		}
	}

	const threadID = "PRRT_mutation_author_parser"
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: prove the author boundary."}
	answer := providerComment{id: "PRRC_answer", login: "author", body: "The boundary is proved."}
	provider := installSequencedProvider(t,
		providerStep{stdout: resolveResponseWithExactComments(threadID, true, opening, answer)},
	)

	_, state, err := mutateThread(context.Background(), "resolve", threadID)
	if err != nil {
		t.Fatalf("parse resolve mutation state: %v", err)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "resolve")
	if state.Author != opening.login || state.LastAuthor != answer.login || !state.Answered {
		t.Fatalf("mutation author boundary = opening %q, answer %q, answered %t; want %q, %q, true",
			state.Author, state.LastAuthor, state.Answered, opening.login, answer.login)
	}
	if state.PrevID != opening.id || state.LastID != answer.id {
		t.Fatalf("mutation tail pair = (%q, %q), want (%q, %q)",
			state.PrevID, state.LastID, opening.id, answer.id)
	}
}

func TestBareNonForceResolveReopensChangedAuthorBoundary(t *testing.T) {
	const threadID = "PRRT_bare_mutation_author_boundary"
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: preserve both identities."}
	answer := providerComment{id: "PRRC_answer", login: "author", body: "Both identities are required."}

	tests := []struct {
		name       string
		lastAuthor string
	}{
		{name: "missing answer author"},
		{name: "answer author equals opener", lastAuthor: opening.login},
		{name: "answer author changed", lastAuthor: "different-author"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mutationAnswer := answer
			mutationAnswer.login = tc.lastAuthor
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, opening, answer)},
				providerStep{stdout: resolveMutationResponseWithOpeningAuthor(
					threadID,
					opening.login,
					opening,
					mutationAnswer,
				)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			msg, mutated, err := resolveWithEvidence(
				context.Background(),
				threadID,
				false,
				resolutionEvidence{},
			)
			if err == nil {
				t.Fatal("bare non-force resolve accepted a changed author boundary")
			}
			if mutated || msg != "" {
				t.Fatalf("changed author boundary returned (msg=%q, mutated=%t), want no success attribution", msg, mutated)
			}
			var unavailable *unavailableAnswerEvidenceError
			if !errors.As(err, &unavailable) {
				t.Fatalf("changed author boundary error = %T %v, want unavailableAnswerEvidenceError", err, err)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
			assertContainsAll(t, strings.ToLower(err.Error()), "reopened", "author evidence", "independent answer")
			assertContainsNone(t, err.Error(), opening.body, answer.body)
		})
	}
}

func TestExactNonForceResolveReopensChangedOpeningAuthor(t *testing.T) {
	const threadID = "PRRT_exact_mutation_opening_author"
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: bind the opening identity."}
	answer := providerComment{id: "PRRC_answer", login: "author", body: "The exact reply remains byte-identical."}
	evidence := resolutionEvidence{
		LastID:                answer.id,
		PredecessorID:         opening.id,
		PredecessorBodySHA256: exactBodySHA256([]byte(opening.body)),
		BodySHA256:            exactBodySHA256([]byte(answer.body)),
	}

	for _, tc := range []struct {
		name          string
		openingAuthor string
	}{
		{name: "opening author missing"},
		{name: "opening author changed", openingAuthor: "different-reviewer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := installSequencedProvider(t,
				providerStep{stdout: threadResponse(threadID, false, opening, answer)},
				providerStep{stdout: resolveMutationResponseWithOpeningAuthor(
					threadID,
					tc.openingAuthor,
					opening,
					answer,
				)},
				providerStep{stdout: unresolveResponse(threadID)},
			)

			msg, mutated, err := resolveWithEvidence(context.Background(), threadID, false, evidence)
			if err == nil {
				t.Fatal("exact non-force resolve accepted a changed opening author")
			}
			if mutated || msg != "" {
				t.Fatalf("changed opening author returned (msg=%q, mutated=%t), want no success attribution", msg, mutated)
			}
			var unavailable *unavailableAnswerEvidenceError
			if !errors.As(err, &unavailable) {
				t.Fatalf("changed opening author error = %T %v, want unavailableAnswerEvidenceError", err, err)
			}
			provider.assertExhausted()
			assertQueryKinds(t, provider, "thread", "resolve", "unresolve")
			assertContainsAll(t, strings.ToLower(err.Error()), "reopened", "author evidence", "independent answer")
			assertContainsNone(t, err.Error(), opening.body, answer.body)
		})
	}
}

func TestForcedResolveDoesNotRequireIndependentAuthorBoundary(t *testing.T) {
	const threadID = "PRRT_force_unanswered_author_boundary"
	opening := providerComment{id: "PRRC_opening", login: "reviewer", body: "P2: force remains an explicit override."}
	provider := installSequencedProvider(t,
		providerStep{stdout: threadResponse(threadID, false, opening)},
		providerStep{stdout: resolveMutationResponseWithOpeningAuthor(threadID, opening.login, opening)},
	)

	msg, mutated, err := resolveWithEvidence(context.Background(), threadID, true, resolutionEvidence{})
	if err != nil {
		t.Fatalf("forced resolve rejected an unanswered author boundary: %v", err)
	}
	if !mutated {
		t.Fatalf("forced resolve returned (msg=%q, mutated=false), want attributed mutation", msg)
	}
	provider.assertExhausted()
	assertQueryKinds(t, provider, "thread", "resolve")
}

func resolveMutationResponseWithOpeningAuthor(
	threadID, openingAuthor string,
	comments ...providerComment,
) string {
	start := 0
	if len(comments) > 2 {
		start = len(comments) - 2
	}
	recent := make([]string, 0, len(comments)-start)
	for _, comment := range comments[start:] {
		recent = append(recent, fmt.Sprintf(
			`{"id":%q,"author":{"login":%q},"body":%q,"updatedAt":%q%s}`,
			comment.id,
			comment.login,
			comment.body,
			comment.providerUpdatedAt(),
			comment.providerEditEvidence(),
		))
	}
	return fmt.Sprintf(
		`{"data":{"resolveReviewThread":{"thread":{"id":%q,"isResolved":true,"opening":{"nodes":[{"author":{"login":%q}}]},"recent":{"nodes":[%s]}}}}}`,
		threadID,
		openingAuthor,
		strings.Join(recent, ","),
	)
}
