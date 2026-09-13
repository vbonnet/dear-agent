// Provider-evidence binding for continuation issuance after a reply post.
package main

import (
	"context"
	"errors"
	"fmt"
)

type replyIssuancePredecessor struct {
	ID                string
	BodySHA256        string
	UpdatedAt         string
	EditCount         int
	EditCountPresent  bool
	LastEditID        string
	LastEditIDPresent bool
	OpeningAuthor     string
	Author            string
	ReplyBodySHA256   string
}

func (p replyIssuancePredecessor) validateForReplyPost(body string) error {
	switch {
	case !validContinuationReceiptID(p.ID):
		return errors.New("predecessor has an invalid provider ID")
	case !validExactBodySHA256(p.BodySHA256):
		return errors.New("predecessor has an invalid exact body digest")
	case !validContinuationTimestamp(p.UpdatedAt):
		return errors.New("predecessor omitted a valid update timestamp")
	case !p.EditCountPresent || !p.LastEditIDPresent ||
		!validContinuationEditRevision(p.EditCount, p.LastEditID):
		return errors.New("predecessor omitted a valid edit revision")
	case !validContinuationAuthor(p.OpeningAuthor):
		return errors.New("predecessor snapshot omitted a safe opening author")
	case !validContinuationAuthor(p.Author):
		return errors.New("predecessor snapshot omitted a safe tail author")
	case p.ReplyBodySHA256 != exactBodySHA256([]byte(body)):
		return errors.New("predecessor snapshot is bound to different reply bytes")
	default:
		return nil
	}
}

func recoverIssuedReplyEvidence(
	ctx context.Context,
	threadID string,
	predecessor replyIssuancePredecessor,
	observed replyMutationEvidence,
) (replyMutationEvidence, error) {
	history, err := fetchAllComments(ctx, threadID)
	if err != nil {
		return replyMutationEvidence{}, fmt.Errorf("re-read complete reply history: %w", err)
	}
	return issuedReplyEvidenceFromHistory(history, predecessor, observed)
}

func issuedReplyEvidenceFromHistory(
	history []tailComment,
	predecessor replyIssuancePredecessor,
	observed replyMutationEvidence,
) (replyMutationEvidence, error) {
	replyID := observed.ID
	if !validContinuationReceiptID(replyID) {
		return replyMutationEvidence{}, errors.New("recovered reply has an invalid provider ID")
	}
	pair, err := locateIssuedReplyHistoryPair(history, predecessor.ID, replyID)
	if err != nil {
		return replyMutationEvidence{}, err
	}
	if err := validateRecoveredReplyObservation(pair, observed); err != nil {
		return replyMutationEvidence{}, err
	}
	if err := validateIssuedReplyPredecessor(pair, predecessor); err != nil {
		return replyMutationEvidence{}, err
	}
	if err := validateIssuedReplyComment(pair, predecessor); err != nil {
		return replyMutationEvidence{}, err
	}
	return replyMutationEvidence{
		ID:                pair.reply.ID,
		Author:            pair.reply.Login,
		BodySHA256:        exactBodySHA256([]byte(pair.reply.Body)),
		UpdatedAt:         pair.reply.UpdatedAt,
		EditCount:         pair.reply.EditCount,
		EditCountPresent:  true,
		LastEditID:        pair.reply.LastEditID,
		LastEditIDPresent: true,
		Recovered:         true,
	}, nil
}

type issuedReplyHistoryPair struct {
	openingAuthor string
	predecessor   tailComment
	reply         tailComment
}

func locateIssuedReplyHistoryPair(
	history []tailComment,
	predecessorID, replyID string,
) (issuedReplyHistoryPair, error) {
	predecessorIndex, replyIndex := -1, -1
	for i, comment := range history {
		switch comment.ID {
		case predecessorID:
			if predecessorIndex >= 0 {
				return issuedReplyHistoryPair{}, errors.New("provider history duplicated the named predecessor")
			}
			predecessorIndex = i
		case replyID:
			if replyIndex >= 0 {
				return issuedReplyHistoryPair{}, errors.New("provider history duplicated the recovered reply")
			}
			replyIndex = i
		}
	}
	if predecessorIndex < 0 || replyIndex < 0 {
		return issuedReplyHistoryPair{}, errors.New("provider history omitted the named predecessor or recovered reply")
	}
	if replyIndex != predecessorIndex+1 || replyIndex != len(history)-1 {
		return issuedReplyHistoryPair{}, errors.New("recovered reply is no longer the direct current successor of its predecessor")
	}
	return issuedReplyHistoryPair{
		openingAuthor: history[0].Login,
		predecessor:   history[predecessorIndex],
		reply:         history[replyIndex],
	}, nil
}

func validateRecoveredReplyObservation(
	pair issuedReplyHistoryPair,
	observed replyMutationEvidence,
) error {
	if !recoveredReplyObservationMatches(pair.reply, observed) ||
		!recoveredPredecessorObservationMatches(pair, observed) {
		return errors.New("recovered reply identity, author, body, or update time changed between recovery reads")
	}
	if !safeRecoveredPredecessorObservation(observed) || !safeRecoveredReplyObservation(observed) {
		return errors.New("first recovered-reply observation omitted safe author or update-time evidence")
	}
	return nil
}

func recoveredReplyObservationMatches(reply tailComment, observed replyMutationEvidence) bool {
	return observed.ID == reply.ID &&
		observed.BodySHA256 == exactBodySHA256([]byte(reply.Body)) &&
		observed.Author == reply.Login &&
		observed.UpdatedAt == reply.UpdatedAt &&
		observed.EditCountPresent && observed.EditCount == reply.EditCount &&
		observed.LastEditIDPresent && observed.LastEditID == reply.LastEditID
}

func recoveredPredecessorObservationMatches(
	pair issuedReplyHistoryPair,
	observed replyMutationEvidence,
) bool {
	return observed.ObservedOpeningAuthor == pair.openingAuthor &&
		observed.ObservedPredecessorTime == pair.predecessor.UpdatedAt &&
		observed.ObservedPredecessorEditCountPresent &&
		observed.ObservedPredecessorEditCount == pair.predecessor.EditCount &&
		observed.ObservedPredecessorLastEditIDPresent &&
		observed.ObservedPredecessorLastEditID == pair.predecessor.LastEditID
}

func safeRecoveredPredecessorObservation(observed replyMutationEvidence) bool {
	return validContinuationAuthor(observed.ObservedOpeningAuthor) &&
		validContinuationTimestamp(observed.ObservedPredecessorTime) &&
		validContinuationEditRevision(
			observed.ObservedPredecessorEditCount,
			observed.ObservedPredecessorLastEditID,
		)
}

func safeRecoveredReplyObservation(observed replyMutationEvidence) bool {
	return validContinuationAuthor(observed.Author) &&
		validContinuationTimestamp(observed.UpdatedAt) &&
		validContinuationEditRevision(observed.EditCount, observed.LastEditID)
}

func validateIssuedReplyPredecessor(
	pair issuedReplyHistoryPair,
	predecessor replyIssuancePredecessor,
) error {
	if exactBodySHA256([]byte(pair.predecessor.Body)) != predecessor.BodySHA256 {
		return errors.New("recovered reply predecessor bytes changed across provider reads")
	}
	if exactBodySHA256([]byte(pair.reply.Body)) != predecessor.ReplyBodySHA256 {
		return errors.New("recovered reply bytes changed across provider reads")
	}
	if !validContinuationTimestamp(pair.predecessor.UpdatedAt) ||
		pair.predecessor.UpdatedAt != predecessor.UpdatedAt {
		return errors.New("recovered reply predecessor update time changed across provider reads")
	}
	if !pair.predecessor.EditCountPresent || !pair.predecessor.LastEditIDPresent ||
		pair.predecessor.EditCount != predecessor.EditCount ||
		pair.predecessor.LastEditID != predecessor.LastEditID {
		return errors.New("recovered reply predecessor edit revision changed across provider reads")
	}
	if !validContinuationAuthor(pair.openingAuthor) ||
		pair.openingAuthor != predecessor.OpeningAuthor {
		return errors.New("recovered reply opening author changed across provider reads")
	}
	return nil
}

func validateIssuedReplyComment(
	pair issuedReplyHistoryPair,
	predecessor replyIssuancePredecessor,
) error {
	if !validContinuationAuthor(pair.reply.Login) || pair.reply.Login == predecessor.OpeningAuthor {
		return errors.New("recovered reply history does not prove a distinct safe reply author")
	}
	if !validContinuationTimestamp(pair.reply.UpdatedAt) {
		return errors.New("recovered reply history omitted a valid reply update time")
	}
	if !pair.reply.EditCountPresent || !pair.reply.LastEditIDPresent ||
		!validContinuationEditRevision(pair.reply.EditCount, pair.reply.LastEditID) {
		return errors.New("recovered reply history omitted a valid reply edit revision")
	}
	return nil
}

func rejectUnissuablePostedReply(
	ctx context.Context,
	threadID, bodyFile, detail string,
) int {
	cur, err := fetchThread(ctx, threadID)
	if err != nil {
		return fail("the reply is posted, but its continuation issuance boundary could not be proved (%s), and current resolved state could not be re-read: %v\n%s",
			detail,
			err,
			providerReadRecoveryGuidance(err, inspectReplyOutcomeGuidance(threadID, bodyFile)),
		)
	}
	reopened := false
	if cur.IsResolved {
		reason := "resolved after continuation issuance evidence became inconsistent: " + detail
		if err := reopenOrFail(ctx, threadID, cur.Path, reason, answerChangedEvidence); err != nil {
			message, _ := replyResolutionEvidenceFailure(
				threadID,
				err,
				revisedAnswerRecoveryGuidance(threadID, bodyFile, true),
			)
			return fail("%s", message)
		}
		reopened = true
	}
	return fail("%sthe reply is posted, but its continuation issuance boundary could not be proved (%s); no receipt was minted and no resolution was attempted.\n%s",
		confirmedReopenPrefix(reopened),
		detail,
		inspectReplyOutcomeGuidance(threadID, bodyFile),
	)
}
