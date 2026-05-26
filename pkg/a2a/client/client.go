// Copyright 2026 dear-agent contributors. See LICENSE.

package client

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/a2aproject/a2a-go/a2aclient"
	"github.com/a2aproject/a2a-go/a2aclient/agentcard"
)

// AnswerFunc resolves an input-required pause. It receives the prompt
// the agent asked and must return the answer string (or an error to
// abort the task).
//
// Implementations decide *who* answers: a deterministic policy, another
// supervisor, the human operator, or a parent AskInput call up the
// supervisor chain.
type AnswerFunc func(ctx context.Context, prompt string) (string, error)

// Client is a supervisor-side wrapper over a2aclient.Client. Each Send
// drives a single task to a terminal state, calling answer for every
// input-required pause encountered.
type Client struct {
	inner *a2aclient.Client
}

// NewFromCardURL resolves the well-known Agent Card at baseURL and
// builds a client that talks to the endpoint the card advertises.
func NewFromCardURL(ctx context.Context, baseURL string) (*Client, error) {
	card, err := agentcard.DefaultResolver.Resolve(ctx, baseURL)
	if err != nil {
		return nil, fmt.Errorf("a2a/client: resolve card at %s: %w", baseURL, err)
	}
	inner, err := a2aclient.NewFromCard(ctx, card)
	if err != nil {
		return nil, fmt.Errorf("a2a/client: build from card: %w", err)
	}
	return &Client{inner: inner}, nil
}

// NewFromEndpoint builds a client that talks directly to invokeURL via
// JSON-RPC, skipping Agent Card discovery. Useful when the supervisor
// already has the URL from a registry.
func NewFromEndpoint(ctx context.Context, invokeURL string) (*Client, error) {
	inner, err := a2aclient.NewFromEndpoints(ctx, []a2a.AgentInterface{
		{Transport: a2a.TransportProtocolJSONRPC, URL: invokeURL},
	})
	if err != nil {
		return nil, fmt.Errorf("a2a/client: build from endpoint: %w", err)
	}
	return &Client{inner: inner}, nil
}

// Send submits prompt as a new task and drives it to a terminal state.
//
// Every time the task pauses with state=input-required, answer is
// invoked with the agent's question and its result is sent as a
// follow-up message. answer may be nil only if the caller is confident
// the agent will never pause — otherwise a pause returns an error.
func (c *Client) Send(ctx context.Context, prompt string, answer AnswerFunc) (string, error) {
	msg := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: prompt})
	return c.drive(ctx, &a2a.MessageSendParams{Message: msg}, answer)
}

// drive is the inner SendMessage loop. It runs until the task reaches a
// terminal state, the context is cancelled, or answer returns an error.
func (c *Client) drive(ctx context.Context, params *a2a.MessageSendParams, answer AnswerFunc) (string, error) {
	for {
		result, err := c.inner.SendMessage(ctx, params)
		if err != nil {
			return "", fmt.Errorf("a2a/client: send: %w", err)
		}

		switch r := result.(type) {
		case *a2a.Message:
			// The agent replied with a bare Message (no task) — common
			// for one-shot exchanges.
			return extractText(r), nil

		case *a2a.Task:
			switch r.Status.State {
			case a2a.TaskStateCompleted:
				return finalAgentText(r), nil

			case a2a.TaskStateInputRequired:
				if answer == nil {
					return "", fmt.Errorf("a2a/client: task %s requires input but no AnswerFunc supplied", r.ID)
				}
				resp, ansErr := answer(ctx, statusText(r.Status))
				if ansErr != nil {
					return "", fmt.Errorf("a2a/client: AnswerFunc: %w", ansErr)
				}
				followup := a2a.NewMessage(a2a.MessageRoleUser, a2a.TextPart{Text: resp})
				followup.TaskID = r.ID
				followup.ContextID = r.ContextID
				params = &a2a.MessageSendParams{Message: followup}
				continue

			case a2a.TaskStateFailed, a2a.TaskStateCanceled, a2a.TaskStateRejected:
				return "", fmt.Errorf("a2a/client: task %s ended in state %s: %s",
					r.ID, r.Status.State, statusText(r.Status))

			default:
				return "", fmt.Errorf("a2a/client: task %s in unexpected state %s", r.ID, r.Status.State)
			}

		default:
			return "", errors.New("a2a/client: send returned unknown result type")
		}
	}
}

// Close releases any transport resources held by the underlying client.
// Safe to call on a zero-value Client.
func (c *Client) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	// a2aclient.Client does not currently expose Close on the
	// transport-agnostic surface; reserved for future SDK additions.
	return nil
}

// extractText returns the concatenated text-part content of a message.
func extractText(m *a2a.Message) string {
	if m == nil {
		return ""
	}
	var b strings.Builder
	for _, part := range m.Parts {
		if tp, ok := part.(a2a.TextPart); ok {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(tp.Text)
		}
	}
	return b.String()
}

// statusText returns the text of the message attached to a TaskStatus,
// if any.
func statusText(st a2a.TaskStatus) string {
	return extractText(st.Message)
}

// finalAgentText resolves the text of the agent's final reply on a
// completed task. Preference order: the status message attached to the
// terminal state, then the last agent-role message in history.
func finalAgentText(t *a2a.Task) string {
	if t == nil {
		return ""
	}
	if txt := statusText(t.Status); txt != "" {
		return txt
	}
	for i := len(t.History) - 1; i >= 0; i-- {
		if t.History[i].Role == a2a.MessageRoleAgent {
			return extractText(t.History[i])
		}
	}
	return ""
}
