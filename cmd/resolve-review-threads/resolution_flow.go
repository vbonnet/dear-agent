// Resolution evidence admission, ambiguous-outcome reconciliation, and
// mutation-response validation.
package main

import (
	"context"
	"errors"
	"fmt"
)

func readResolutionCandidate(
	ctx context.Context,
	threadID string,
	force bool,
	evidence resolutionEvidence,
) (thread, error) {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return thread{}, &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread state could not be read before resolution; no mutation was attempted: %v",
			err,
		), cause: err}
	}
	if err := validateNamedResolutionAnchor(ctx, threadID, cur, evidence); err != nil {
		return thread{}, err
	}
	if err := validateExactResolutionEvidence(ctx, threadID, cur, evidence); err != nil {
		return thread{}, err
	}
	if err := validateResolutionEligibility(threadID, cur, force, evidence); err != nil {
		return thread{}, err
	}
	return cur, nil
}

func validateNamedResolutionAnchor(
	ctx context.Context,
	threadID string,
	cur thread,
	evidence resolutionEvidence,
) error {
	if evidence.LastID == "" || cur.LastID == evidence.LastID {
		return nil
	}
	if cur.LastID == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted the current tail ID needed to verify the named resolution anchor",
			threadID,
		)}
	}
	return staleAnchorError(ctx, threadID, cur)
}

func validateExactResolutionEvidence(
	ctx context.Context,
	threadID string,
	cur thread,
	evidence resolutionEvidence,
) error {
	if !evidence.exactReply() {
		return nil
	}
	if err := validateCurrentExactReplyPair(ctx, threadID, cur, evidence); err != nil {
		return err
	}
	if !evidence.issuedReply() {
		return nil
	}
	if err := validateCurrentIssuedReplyRevision(ctx, threadID, cur, evidence); err != nil {
		return err
	}
	return validateCurrentIssuedReplyAuthors(ctx, threadID, cur, evidence)
}

func validateCurrentExactReplyPair(
	ctx context.Context,
	threadID string,
	cur thread,
	evidence resolutionEvidence,
) error {
	if cur.PrevID == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted the predecessor or body needed to verify exact reply evidence",
			threadID,
		)}
	}
	if cur.PrevID != evidence.PredecessorID {
		return invalidatedReplyAnchorError(
			ctx,
			threadID,
			cur,
			fmt.Sprintf("reply predecessor changed from %s to %s", evidence.PredecessorID, cur.PrevID),
		)
	}
	if !cur.PrevBodyPresent || !cur.LastBodyPresent {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted the predecessor or body needed to verify exact reply evidence",
			threadID,
		)}
	}
	if exactBodySHA256([]byte(cur.PrevBody)) != evidence.PredecessorBodySHA256 {
		return invalidatedReplyAnchorError(
			ctx,
			threadID,
			cur,
			"provider-visible predecessor bytes no longer match the authorized digest",
		)
	}
	if exactBodySHA256([]byte(cur.LastBody)) != evidence.BodySHA256 {
		return invalidatedReplyAnchorError(
			ctx,
			threadID,
			cur,
			"provider-visible reply bytes no longer match the authorized digest",
		)
	}
	return nil
}

func validateCurrentIssuedReplyRevision(
	ctx context.Context,
	threadID string,
	cur thread,
	evidence resolutionEvidence,
) error {
	if cur.PrevUpdatedAt == "" || cur.LastUpdatedAt == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted update timestamps needed to verify issued reply evidence",
			threadID,
		)}
	}
	if cur.PrevUpdatedAt != evidence.PredecessorUpdatedAt || cur.LastUpdatedAt != evidence.ReplyUpdatedAt {
		return invalidatedReplyAnchorError(
			ctx,
			threadID,
			cur,
			"provider comment update timestamps changed after continuation issuance",
		)
	}
	if !cur.PrevEditCountPresent || !cur.LastEditCountPresent ||
		!cur.PrevEditIDPresent || !cur.LastEditIDPresent {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted edit revisions needed to verify issued reply evidence",
			threadID,
		)}
	}
	if cur.PrevEditCount != evidence.PredecessorEditCount ||
		cur.LastEditCount != evidence.ReplyEditCount ||
		cur.PrevEditID != evidence.PredecessorLastEditID ||
		cur.LastEditID != evidence.ReplyLastEditID {
		return invalidatedReplyAnchorError(
			ctx,
			threadID,
			cur,
			"provider comment edit revisions changed after continuation issuance",
		)
	}
	return nil
}

func validateCurrentIssuedReplyAuthors(
	ctx context.Context,
	threadID string,
	cur thread,
	evidence resolutionEvidence,
) error {
	if cur.Author == "unknown" || cur.LastAuthor == "unknown" ||
		cur.Author != evidence.OpeningAuthor || cur.LastAuthor != evidence.ReplyAuthor || !cur.Answered {
		return invalidatedIssuedAuthorEvidence(ctx, threadID, cur)
	}
	return nil
}

func invalidatedIssuedAuthorEvidence(ctx context.Context, threadID string, cur thread) error {
	if cur.IsResolved {
		if err := reopenOrFail(
			ctx,
			threadID,
			cur.Path,
			"resolved after issued reply author evidence became missing, equal, or inconsistent",
			retryUnchangedEvidence,
		); err != nil {
			return err
		}
	}
	return &unavailableAnswerEvidenceError{msg: fmt.Sprintf(
		"%s %s: issued opening/reply author evidence is missing, equal, or inconsistent",
		threadID,
		cur.Path,
	)}
}

func validateResolutionEligibility(
	threadID string,
	cur thread,
	force bool,
	evidence resolutionEvidence,
) error {
	if evidence.LastID != "" && !force && !cur.Answered {
		return &unavailableAnswerEvidenceError{msg: fmt.Sprintf(
			"%s %s: the named reply is still last, but missing or equal author identity cannot prove an independent answer%s",
			threadID, cur.Path, outdatedNote(cur),
		)}
	}
	if cur.IsResolved {
		return nil
	}
	if !force && cur.LastID == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"thread %s response omitted the current tail ID needed as resolution evidence",
			threadID,
		)}
	}
	if !cur.Answered && !force {
		return &unansweredError{msg: fmt.Sprintf(
			"%s %s: unanswered (last word: @%s)%s",
			threadID, cur.Path, cur.LastAuthor, outdatedNote(cur),
		)}
	}
	return nil
}

// reconcileResolveMutationError turns a client-side mutation failure into a
// verified provider state. A transport error can arrive after GitHub applied
// the mutation, so the error alone is never proof of a no-op.
func reconcileResolveMutationError(
	ctx context.Context,
	threadID string,
	evidence resolutionEvidence,
	mutationErr error,
) (string, mutationThreadState, error) {
	after, readErr := fetchThread(ctx, threadID)
	if readErr != nil {
		return "", mutationThreadState{}, &unverifiedProviderStateError{msg: fmt.Sprintf(
			"resolve mutation reported %v, and the thread could not be re-read to determine whether it applied: %v",
			mutationErr,
			readErr,
		), cause: errors.Join(mutationErr, readErr)}
	}
	if !after.IsResolved {
		if evidence.LastID != "" && after.LastID == "" {
			return "", mutationThreadState{}, &unverifiedProviderStateError{msg: fmt.Sprintf(
				"resolve mutation reported %v, and the fresh unresolved response for %s omitted the tail ID needed to choose unchanged retry or revision",
				mutationErr,
				threadID,
			), cause: mutationErr}
		}
		if evidence.LastID != "" && after.LastID != evidence.LastID {
			return "", mutationThreadState{}, &supersededEvidenceError{msg: fmt.Sprintf(
				"%s %s: the resolve outcome is unresolved, but last comment changed from %s to %s while the mutation was in flight",
				threadID,
				after.Path,
				evidence.LastID,
				after.LastID,
			), cause: mutationErr}
		}
		if err := validateUnresolvedExactEvidence(threadID, after, evidence, mutationErr); err != nil {
			return "", mutationThreadState{}, err
		}
		return "", mutationThreadState{}, mutationErr
	}
	message := fmt.Sprintf(
		"skipped %s (a fresh read independently found it resolved after the resolve mutation reported an error; this caller is not attributed)",
		threadID,
	)
	if isAccessDenied(mutationErr) {
		message = fmt.Sprintf(
			"skipped %s (a fresh read found it resolved after GitHub denied this caller's mutation)",
			threadID,
		)
	}
	return message, mutationStateFromThread(after), nil
}

func validateResolveMutationAuthorBoundary(
	ctx context.Context,
	threadID string,
	before thread,
	after mutationThreadState,
	mutationErr error,
) error {
	if before.Author != "unknown" &&
		before.LastAuthor != "unknown" &&
		before.Author == after.Author &&
		before.LastAuthor == after.LastAuthor &&
		after.Author != "" &&
		after.LastAuthor != "" &&
		after.Author != "unknown" &&
		after.LastAuthor != "unknown" &&
		after.Author != after.LastAuthor &&
		after.Answered {
		return nil
	}
	reason := "resolved after the answer-author boundary became missing, equal, or inconsistent in the mutation response"
	if err := reopenOrFail(ctx, threadID, before.Path, reason, retryUnchangedEvidence); err != nil {
		if mutationErr != nil {
			return errors.Join(err, mutationErr)
		}
		return err
	}
	detail := "the resolve response was reopened because its author evidence no longer proves the same independent answer"
	if mutationErr != nil {
		detail = "the ambiguous resolve outcome was reopened because fresh author evidence no longer proves the same independent answer"
	}
	return &unavailableAnswerEvidenceError{
		msg: fmt.Sprintf(
			"%s %s: %s",
			threadID,
			before.Path,
			detail,
		),
		cause: mutationErr,
	}
}

func validateUnresolvedExactEvidence(
	threadID string,
	after thread,
	evidence resolutionEvidence,
	mutationErr error,
) error {
	if !evidence.exactReply() {
		return nil
	}
	if err := validateUnresolvedExactReplyPair(threadID, after, evidence, mutationErr); err != nil {
		return err
	}
	if !evidence.issuedReply() {
		return nil
	}
	if err := validateUnresolvedIssuedReplyRevision(threadID, after, evidence, mutationErr); err != nil {
		return err
	}
	return validateUnresolvedIssuedReplyAuthors(threadID, after, evidence, mutationErr)
}

func validateUnresolvedExactReplyPair(
	threadID string,
	after thread,
	evidence resolutionEvidence,
	mutationErr error,
) error {
	if after.PrevID == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"resolve mutation reported %v, and the fresh unresolved response for %s omitted exact reply evidence",
			mutationErr,
			threadID,
		), cause: mutationErr}
	}
	if after.PrevID != evidence.PredecessorID {
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: the resolve outcome is unresolved, but the exact predecessor or reply body changed while the mutation was in flight",
			threadID,
			after.Path,
		), cause: mutationErr}
	}
	if !after.PrevBodyPresent || !after.LastBodyPresent {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"resolve mutation reported %v, and the fresh unresolved response for %s omitted exact reply evidence",
			mutationErr,
			threadID,
		), cause: mutationErr}
	}
	if exactBodySHA256([]byte(after.PrevBody)) != evidence.PredecessorBodySHA256 ||
		exactBodySHA256([]byte(after.LastBody)) != evidence.BodySHA256 {
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: the resolve outcome is unresolved, but the exact predecessor or reply body changed while the mutation was in flight",
			threadID,
			after.Path,
		), cause: mutationErr}
	}
	return nil
}

func validateUnresolvedIssuedReplyRevision(
	threadID string,
	after thread,
	evidence resolutionEvidence,
	mutationErr error,
) error {
	if after.PrevUpdatedAt == "" || after.LastUpdatedAt == "" {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"resolve mutation reported %v, and the fresh unresolved response for %s omitted issued update-time evidence",
			mutationErr,
			threadID,
		), cause: mutationErr}
	}
	if after.PrevUpdatedAt != evidence.PredecessorUpdatedAt ||
		after.LastUpdatedAt != evidence.ReplyUpdatedAt {
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: the resolve outcome is unresolved, but comment update timestamps changed after continuation issuance",
			threadID,
			after.Path,
		), cause: mutationErr}
	}
	if !after.PrevEditCountPresent || !after.LastEditCountPresent ||
		!after.PrevEditIDPresent || !after.LastEditIDPresent {
		return &unverifiedProviderStateError{msg: fmt.Sprintf(
			"resolve mutation reported %v, and the fresh unresolved response for %s omitted issued edit-revision evidence",
			mutationErr,
			threadID,
		), cause: mutationErr}
	}
	if after.PrevEditCount != evidence.PredecessorEditCount ||
		after.LastEditCount != evidence.ReplyEditCount ||
		after.PrevEditID != evidence.PredecessorLastEditID ||
		after.LastEditID != evidence.ReplyLastEditID {
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: the resolve outcome is unresolved, but comment edit revisions changed after continuation issuance",
			threadID,
			after.Path,
		), cause: mutationErr}
	}
	return nil
}

func validateUnresolvedIssuedReplyAuthors(
	threadID string,
	after thread,
	evidence resolutionEvidence,
	mutationErr error,
) error {
	if after.Author == "unknown" || after.LastAuthor == "unknown" ||
		after.Author != evidence.OpeningAuthor || after.LastAuthor != evidence.ReplyAuthor || !after.Answered {
		return &unavailableAnswerEvidenceError{msg: fmt.Sprintf(
			"%s %s: the resolve outcome is unresolved, but issued author evidence became missing, equal, or inconsistent",
			threadID,
			after.Path,
		), cause: mutationErr}
	}
	return nil
}

func mutationStateFromThread(after thread) mutationThreadState {
	return mutationThreadState{
		IsResolved:           true,
		Path:                 after.Path,
		Author:               after.Author,
		LastAuthor:           after.LastAuthor,
		Answered:             after.Answered,
		LastID:               after.LastID,
		PrevID:               after.PrevID,
		PrevBody:             after.PrevBody,
		PrevBodyPresent:      after.PrevBodyPresent,
		LastBody:             after.LastBody,
		LastBodyPresent:      after.LastBodyPresent,
		LastUpdatedAt:        after.LastUpdatedAt,
		PrevUpdatedAt:        after.PrevUpdatedAt,
		LastEditCount:        after.LastEditCount,
		PrevEditCount:        after.PrevEditCount,
		LastEditCountPresent: after.LastEditCountPresent,
		PrevEditCountPresent: after.PrevEditCountPresent,
		LastEditID:           after.LastEditID,
		PrevEditID:           after.PrevEditID,
		LastEditIDPresent:    after.LastEditIDPresent,
		PrevEditIDPresent:    after.PrevEditIDPresent,
	}
}

// validateResolveMutationEvidence checks the provider state returned in the
// same response as the resolve. A stale or incomplete successful resolution
// is reopened before the caller receives a failure.
func validateResolveMutationEvidence(
	ctx context.Context,
	threadID, path string,
	evidence resolutionEvidence,
	state mutationThreadState,
	mutationCause error,
) error {
	if state.LastID == "" {
		reason := "resolved without a response anchor confirming which comment it resolved against"
		if err := reopenOrFail(ctx, threadID, path, reason, retryUnchangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted its last-comment anchor",
			threadID,
			path,
		), cause: mutationCause}
	}
	if evidence.LastID != "" && state.LastID != evidence.LastID {
		reason := fmt.Sprintf(
			"resolved after the evidence anchor changed from %s to %s",
			evidence.LastID,
			state.LastID,
		)
		if err := reopenOrFail(ctx, threadID, path, reason, answerChangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &supersededEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — last comment changed from %s to %s while resolution was in flight",
			threadID,
			path,
			evidence.LastID,
			state.LastID,
		), cause: mutationCause}
	}
	if !evidence.exactReply() {
		return nil
	}
	return validateExactResolveMutationEvidence(
		ctx,
		threadID,
		path,
		evidence,
		state,
		mutationCause,
	)
}

func validateExactResolveMutationEvidence(
	ctx context.Context,
	threadID, path string,
	evidence resolutionEvidence,
	state mutationThreadState,
	mutationCause error,
) error {
	if err := validateExactResolveMutationPair(ctx, threadID, path, evidence, state, mutationCause); err != nil {
		return err
	}
	if !evidence.issuedReply() {
		return nil
	}
	if err := validateIssuedResolveMutationTimestamps(ctx, threadID, path, evidence, state, mutationCause); err != nil {
		return err
	}
	return validateIssuedResolveMutationEdits(ctx, threadID, path, evidence, state, mutationCause)
}

func validateExactResolveMutationPair(
	ctx context.Context,
	threadID, path string,
	evidence resolutionEvidence,
	state mutationThreadState,
	mutationCause error,
) error {
	if state.PrevID == "" {
		reason := "resolved without response evidence for the exact reply predecessor and bodies"
		if err := reopenOrFail(ctx, threadID, path, reason, retryUnchangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted exact reply evidence",
			threadID,
			path,
		), cause: mutationCause}
	}
	if state.PrevID != evidence.PredecessorID {
		reason := "resolved after the exact reply predecessor changed"
		if err := reopenOrFail(ctx, threadID, path, reason, answerChangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — the exact predecessor changed while resolution was in flight",
			threadID,
			path,
		), cause: mutationCause}
	}
	if !state.PrevBodyPresent || !state.LastBodyPresent {
		reason := "resolved without response evidence for the exact reply predecessor and bodies"
		if err := reopenOrFail(ctx, threadID, path, reason, retryUnchangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted exact reply evidence",
			threadID,
			path,
		), cause: mutationCause}
	}
	if exactBodySHA256([]byte(state.PrevBody)) != evidence.PredecessorBodySHA256 ||
		exactBodySHA256([]byte(state.LastBody)) != evidence.BodySHA256 {
		reason := "resolved after the exact predecessor or reply body changed"
		if err := reopenOrFail(ctx, threadID, path, reason, answerChangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — the exact predecessor or provider-visible reply body changed while resolution was in flight",
			threadID,
			path,
		), cause: mutationCause}
	}
	return nil
}

func validateIssuedResolveMutationTimestamps(
	ctx context.Context,
	threadID, path string,
	evidence resolutionEvidence,
	state mutationThreadState,
	mutationCause error,
) error {
	if state.PrevUpdatedAt == "" || state.LastUpdatedAt == "" {
		reason := "resolved without issued comment update-time evidence"
		if err := reopenOrFail(ctx, threadID, path, reason, retryUnchangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted issued update-time evidence",
			threadID,
			path,
		), cause: mutationCause}
	}
	if state.PrevUpdatedAt != evidence.PredecessorUpdatedAt || state.LastUpdatedAt != evidence.ReplyUpdatedAt {
		reason := "resolved after an issued comment update timestamp changed"
		if err := reopenOrFail(ctx, threadID, path, reason, answerChangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — a comment update timestamp changed after continuation issuance",
			threadID,
			path,
		), cause: mutationCause}
	}
	return nil
}

func validateIssuedResolveMutationEdits(
	ctx context.Context,
	threadID, path string,
	evidence resolutionEvidence,
	state mutationThreadState,
	mutationCause error,
) error {
	if !state.PrevEditCountPresent || !state.LastEditCountPresent ||
		!state.PrevEditIDPresent || !state.LastEditIDPresent {
		reason := "resolved without issued comment edit-revision evidence"
		if err := reopenOrFail(ctx, threadID, path, reason, retryUnchangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &unverifiableResolutionError{msg: fmt.Sprintf(
			"%s %s: reopened — the resolve response omitted issued edit-revision evidence",
			threadID,
			path,
		), cause: mutationCause}
	}
	if state.PrevEditCount != evidence.PredecessorEditCount ||
		state.LastEditCount != evidence.ReplyEditCount ||
		state.PrevEditID != evidence.PredecessorLastEditID ||
		state.LastEditID != evidence.ReplyLastEditID {
		reason := "resolved after an issued comment edit revision changed"
		if err := reopenOrFail(ctx, threadID, path, reason, answerChangedEvidence); err != nil {
			return errors.Join(err, mutationCause)
		}
		return &invalidatedReplyEvidenceError{msg: fmt.Sprintf(
			"%s %s: reopened — a comment edit revision changed after continuation issuance",
			threadID,
			path,
		), cause: mutationCause}
	}
	return nil
}
