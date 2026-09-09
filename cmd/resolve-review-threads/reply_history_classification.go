// Classification helpers for provider-visible reply history.
package main

// priorReplyState describes what a previous reply-resolve run left behind.
type priorReplyState int

const (
	// noPriorReply means this body is not present in the thread tail.
	noPriorReply priorReplyState = iota
	// reviewerHandbackIsLast means the requested body is absent and the
	// opening-side author has the current tail after earlier discussion. It is
	// safe to answer while unresolved, but never a terminal resolved skip.
	reviewerHandbackIsLast
	// priorReplyIsLast means a previous run already posted it and nothing has
	// been said since, so only the resolve remains.
	priorReplyIsLast
	// priorReplySuperseded means a previous run posted it and someone has
	// commented afterwards, so the prewritten body no longer answers the
	// thread.
	priorReplySuperseded
	// differentAnswerIsLast means a distinct participant already holds the
	// stable tail with different bytes. Ordinary reply-resolve must not stack
	// another answer merely because its mutable source changed.
	differentAnswerIsLast
	// unavailableReplyIntent means author identity is missing, so the command
	// cannot distinguish an opening/reviewer hand-back from an independent
	// answer and must fail closed.
	unavailableReplyIntent
)

// classifyPriorReply reports whether this exact reply already sits in the
// thread and, if so, whether anyone has spoken after it. Reposting a
// superseded reply would both duplicate the comment and, because our copy
// would then be last, make the thread look answered while the follow-up went
// unread.
func classifyPriorReply(tail []tailComment, body string) priorReplyState {
	if len(tail) == 0 || tail[0].Login == "" {
		return unavailableReplyIntent
	}
	openingLogin := tail[0].Login
	idx := -1
	missingLogin := false
	for i, comment := range tail {
		if comment.Login == "" {
			missingLogin = true
			continue
		}
		if i > 0 && comment.Login != openingLogin && sameReplyBody(comment.Body, body) {
			idx = i
		}
	}
	switch {
	case idx == len(tail)-1:
		return priorReplyIsLast
	case idx >= 0:
		return priorReplySuperseded
	case missingLogin:
		return unavailableReplyIntent
	case len(tail) == 1:
		return noPriorReply
	case tail[len(tail)-1].Login == openingLogin:
		return reviewerHandbackIsLast
	default:
		return differentAnswerIsLast
	}
}

// findReplyID returns the ID of the last tail comment whose body matches want,
// or "" if none does. A client-side failure can arrive after GitHub accepted a
// mutation, so ambiguous recovery must look for the reply before retrying.
func findReplyID(tail []tailComment, want string) string {
	id := ""
	for _, comment := range tail {
		if sameReplyBody(comment.Body, want) {
			id = comment.ID
		}
	}
	return id
}

// findReplyIDAfter returns the last byte-exact reply observed after predecessor
// and whether predecessor itself was present. Ambiguous mutation recovery must
// not promote trim-equivalent duplicate suppression into exact issuance proof.
func findReplyIDAfter(history []tailComment, predecessor, want string) (id string, predecessorFound bool) {
	for _, comment := range history {
		if !predecessorFound {
			if comment.ID == predecessor {
				predecessorFound = true
			}
			continue
		}
		if comment.Body == want {
			id = comment.ID
		}
	}
	return id, predecessorFound
}
