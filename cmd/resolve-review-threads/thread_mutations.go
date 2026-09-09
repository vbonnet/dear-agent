// GraphQL mutations and their response validation for review threads.
package main

import (
	"context"
	"encoding/json"
	"fmt"
)

// resolveMutation asks for both ends of the author boundary and the exact tail
// pair in the same response as the mutation, not via a separate read afterward.
// A comment landing between the pre-mutation evidence check and this mutation
// actually applying is a real window a subsequent read cannot fully close (it
// has its own latency); having GitHub report the post-mutation boundary lets the
// caller detect that race directly off the mutation it just made.
const resolveMutation = `mutation($threadId:ID!) {
  resolveReviewThread(input:{threadId:$threadId}) {
    thread {
      id
      isResolved
      opening: comments(first:1) { nodes { author { login } } }
      recent: comments(last:2) { nodes { id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } } }
    }
  }
}`

const unresolveMutation = `mutation($threadId:ID!) {
  unresolveReviewThread(input:{threadId:$threadId}) {
    thread { id isResolved }
  }
}`

// replyMutation posts a reply onto an existing review thread. Note the input
// field here is pullRequestReviewThreadId, NOT the threadId that
// resolveReviewThread takes — the two mutations disagree, which is why the
// reply and the resolve are wrapped together rather than left to callers.
const replyMutation = `mutation($threadId:ID!, $body:String!) {
  addPullRequestReviewThreadReply(input:{pullRequestReviewThreadId:$threadId, body:$body}) {
    comment { id author { login } body updatedAt userContentEdits(last:1) { totalCount nodes { id } } }
  }
}`

// mutationThreadState is the provider state returned in the same response as
// a thread mutation. Resolve returns the last two comment identities and the
// exact last body so receipt-bound evidence can be checked at mutation time.
type mutationThreadState struct {
	IsResolved           bool
	Path                 string
	Author               string
	LastAuthor           string
	Answered             bool
	LastID               string
	PrevID               string
	PrevBody             string
	PrevBodyPresent      bool
	LastBody             string
	LastBodyPresent      bool
	LastUpdatedAt        string
	PrevUpdatedAt        string
	LastEditCount        int
	PrevEditCount        int
	LastEditCountPresent bool
	PrevEditCountPresent bool
	LastEditID           string
	PrevEditID           string
	LastEditIDPresent    bool
	PrevEditIDPresent    bool
}

type threadMutationResponse struct {
	Data map[string]threadMutationPayload `json:"data"`
}

type threadMutationPayload struct {
	Thread threadMutationNode `json:"thread"`
}

type threadMutationNode struct {
	ID         string `json:"id"`
	IsResolved *bool  `json:"isResolved"`
	Opening    struct {
		Nodes []threadMutationComment `json:"nodes"`
	} `json:"opening"`
	Recent struct {
		Nodes []threadMutationComment `json:"nodes"`
	} `json:"recent"`
}

type threadMutationComment struct {
	ID               string                    `json:"id"`
	Body             *string                   `json:"body"`
	UpdatedAt        string                    `json:"updatedAt"`
	UserContentEdits *userContentEditsEvidence `json:"userContentEdits"`
	Author           struct {
		Login string `json:"login"`
	} `json:"author"`
}

// mutateThread resolves ("resolve") or re-opens ("unresolve") one thread and
// returns the requested postcondition from the mutation response rather than
// assuming a nil client error proves it.
func mutateThread(
	ctx context.Context,
	action, threadID string,
) (msg string, state mutationThreadState, err error) {
	query, field, wantResolved, err := threadMutationOperation(action)
	if err != nil {
		return "", mutationThreadState{}, err
	}
	raw, err := ghGraphQL(ctx, query, map[string]any{"threadId": threadID})
	if err != nil {
		return "", mutationThreadState{}, err
	}
	th, err := decodeThreadMutationResponse(raw, field)
	if err != nil {
		return "", mutationThreadState{}, err
	}
	state, err = validateThreadMutationResponse(field, threadID, th, wantResolved)
	if err != nil {
		return "", state, err
	}
	state = mutationStateFromResponse(state, th)
	return fmt.Sprintf("%sd %s (isResolved=%t)", action, th.ID, state.IsResolved), state, nil
}

func threadMutationOperation(action string) (query, field string, wantResolved bool, err error) {
	switch action {
	case "resolve":
		return resolveMutation, "resolveReviewThread", true, nil
	case "unresolve":
		return unresolveMutation, "unresolveReviewThread", false, nil
	default:
		return "", "", false, fmt.Errorf("unsupported thread mutation action %q", action)
	}
}

func decodeThreadMutationResponse(raw []byte, field string) (threadMutationNode, error) {
	var resp threadMutationResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return threadMutationNode{}, fmt.Errorf("parse %s response: %w", field, err)
	}
	return resp.Data[field].Thread, nil
}

func validateThreadMutationResponse(
	field, threadID string,
	th threadMutationNode,
	wantResolved bool,
) (mutationThreadState, error) {
	var state mutationThreadState
	if th.ID == "" {
		return state, fmt.Errorf(
			"%s response omitted the requested thread identity %s",
			field,
			threadID,
		)
	}
	if th.ID != threadID {
		return state, fmt.Errorf(
			"%s response returned mismatched thread ID %s for requested %s",
			field,
			th.ID,
			threadID,
		)
	}
	if th.IsResolved == nil {
		return state, fmt.Errorf(
			"%s response for %s omitted the requested resolved-state postcondition",
			field,
			threadID,
		)
	}
	state.IsResolved = *th.IsResolved
	if state.IsResolved != wantResolved {
		return state, fmt.Errorf(
			"%s response for %s reported isResolved=%t, want %t",
			field,
			threadID,
			state.IsResolved,
			wantResolved,
		)
	}
	return state, nil
}

func mutationStateFromResponse(state mutationThreadState, th threadMutationNode) mutationThreadState {
	if len(th.Opening.Nodes) > 0 {
		state.Author = th.Opening.Nodes[0].Author.Login
	}
	if comments := th.Recent.Nodes; len(comments) > 0 {
		last := comments[len(comments)-1]
		state.LastID = last.ID
		state.LastAuthor = last.Author.Login
		state.LastUpdatedAt = last.UpdatedAt
		if editCount, lastEditID, present := observedEditRevision(last.UserContentEdits); present {
			state.LastEditCount = editCount
			state.LastEditCountPresent = true
			state.LastEditID = lastEditID
			state.LastEditIDPresent = true
		}
		if last.Body != nil {
			state.LastBody = *last.Body
			state.LastBodyPresent = true
		}
		if len(comments) > 1 {
			previous := comments[len(comments)-2]
			state.PrevID = previous.ID
			state.PrevUpdatedAt = previous.UpdatedAt
			if editCount, lastEditID, present := observedEditRevision(previous.UserContentEdits); present {
				state.PrevEditCount = editCount
				state.PrevEditCountPresent = true
				state.PrevEditID = lastEditID
				state.PrevEditIDPresent = true
			}
			if previous.Body != nil {
				state.PrevBody = *previous.Body
				state.PrevBodyPresent = true
			}
		}
	}
	state.Answered = state.Author != "" && state.LastAuthor != "" && state.Author != state.LastAuthor
	return state
}
