// Package discord implements a HITL backend that surfaces approval
// requests in a Discord channel and accepts replies as decisions.
//
// Design:
//   - Request renders the approval as a Discord message and stores the
//     resulting message id alongside the approval id so a reply can be
//     correlated.
//   - Wait subscribes to a delivery channel populated by the Discord
//     gateway handler when a reply arrives, then writes the decision
//     into the approvals table via workflow.RecordHITLDecision.
//
// The backend is wire-compatible with workflow.HITLBackend so a runner
// can swap the SQLite default for the Discord one without code changes:
//
//	r.HITLBackend = discord.NewBackend(client, channelID, db)
//
// The Discord client interface is intentionally narrow (Send + Listen)
// so production code wraps a *discordgo.Session and tests substitute an
// in-memory fake.
package discord

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// MessageSender abstracts "send a plain-text message" so the backend works
// against either a *discordgo.Session or a test fake. Returning the
// message id is required so the backend can correlate replies (Discord's
// reply payloads include the parent message id).
type MessageSender interface {
	SendMessage(ctx context.Context, channelID, content string) (messageID string, err error)
}

// ReplyEvent is what the gateway dispatches into the backend's reply
// channel. The Discord gateway gives more than this; the backend only
// needs the parent message + author + body.
type ReplyEvent struct {
	ParentMessageID string
	AuthorID        string
	AuthorName      string
	Content         string
	Role            string
	OccurredAt      time.Time
}

// Backend is the HITL backend over Discord. It is safe for concurrent use:
// Request and Wait are guarded by the same mutex so the message-id ↔
// approval-id correlation map stays consistent.
type Backend struct {
	Sender    MessageSender
	ChannelID string
	DB        *sql.DB

	// AllowedRoles, when non-empty, restricts who can reply. A reply
	// from an author whose role is not in the set is ignored. Empty
	// means "any reply counts".
	AllowedRoles []string

	mu sync.Mutex
	// pending maps approvalID → its delivery channel. Wait blocks on
	// this channel until a reply arrives.
	pending map[string]chan workflow.HITLResolution
	// byMessage maps the parent message id to the approval id so an
	// inbound reply can be correlated.
	byMessage map[string]string
}

// NewBackend constructs a Backend with the empty correlation maps
// initialised. ChannelID defaults to the message-sender's discretion if
// empty (the sender can read it from environment, etc.).
func NewBackend(sender MessageSender, channelID string, db *sql.DB) *Backend {
	return &Backend{
		Sender:    sender,
		ChannelID: channelID,
		DB:        db,
		pending:   map[string]chan workflow.HITLResolution{},
		byMessage: map[string]string{},
	}
}

// Request renders the approval and posts it to the channel.
func (b *Backend) Request(ctx context.Context, req workflow.HITLRequest) error {
	if b == nil || b.Sender == nil {
		return errors.New("discord HITL backend: sender not configured")
	}
	msg := renderRequest(req)
	id, err := b.Sender.SendMessage(ctx, b.ChannelID, msg)
	if err != nil {
		return fmt.Errorf("discord HITL: send: %w", err)
	}

	b.mu.Lock()
	if _, ok := b.pending[req.ApprovalID]; !ok {
		b.pending[req.ApprovalID] = make(chan workflow.HITLResolution, 1)
	}
	if id != "" {
		b.byMessage[id] = req.ApprovalID
	}
	b.mu.Unlock()
	return nil
}

// Wait blocks until OnReply has been called with a matching message id (or
// ctx fires). The caller still needs to commit the decision via the
// workflow package — Wait returns the resolution, OnReply persists it.
func (b *Backend) Wait(ctx context.Context, approvalID string) (workflow.HITLResolution, error) {
	b.mu.Lock()
	ch, ok := b.pending[approvalID]
	if !ok {
		ch = make(chan workflow.HITLResolution, 1)
		b.pending[approvalID] = ch
	}
	b.mu.Unlock()

	select {
	case res := <-ch:
		return res, nil
	case <-ctx.Done():
		return workflow.HITLResolution{}, ctx.Err()
	}
}

// OnReply is the entry point a Discord gateway handler calls when a reply
// arrives. It looks up the approval id, parses the decision, persists it
// to the approvals table, and unblocks Wait.
//
// Recognised reply prefixes (case-insensitive): "approve", "ok", "lgtm",
// "yes" → approve; "reject", "no", "deny" → reject. Anything else is
// ignored (the human can re-reply). This loose grammar is deliberately
// chat-shaped — formal "/approve" slash commands ride on top of the
// same primitives.
func (b *Backend) OnReply(ctx context.Context, ev ReplyEvent) error {
	if ev.ParentMessageID == "" {
		return nil
	}
	b.mu.Lock()
	approvalID, ok := b.byMessage[ev.ParentMessageID]
	ch := b.pending[approvalID]
	b.mu.Unlock()
	if !ok {
		return nil // reply is not for one of our approvals
	}
	dec, recognised := parseDecision(ev.Content)
	if !recognised {
		return nil
	}
	if !b.roleAllowed(ev.Role) {
		return fmt.Errorf("discord HITL: role %q not in allow-list", ev.Role)
	}

	resolvedAt := ev.OccurredAt
	if resolvedAt.IsZero() {
		resolvedAt = time.Now()
	}
	if b.DB != nil {
		if err := workflow.RecordHITLDecision(ctx, b.DB, approvalID, dec, ev.AuthorName, ev.Role, ev.Content, resolvedAt); err != nil {
			return fmt.Errorf("discord HITL: record: %w", err)
		}
	}

	resolution := workflow.HITLResolution{
		ApprovalID: approvalID,
		Decision:   dec,
		Approver:   ev.AuthorName,
		Role:       ev.Role,
		Reason:     ev.Content,
		ResolvedAt: resolvedAt,
	}
	select {
	case ch <- resolution:
	default:
		// Wait already returned (timeout, cancellation). Drop the value.
	}
	return nil
}

// roleAllowed returns true when AllowedRoles is empty or contains role.
func (b *Backend) roleAllowed(role string) bool {
	if len(b.AllowedRoles) == 0 {
		return true
	}
	for _, r := range b.AllowedRoles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

// renderRequest formats the human-facing message. Kept compact: most
// Discord channels strip out anything beyond a few lines.
func renderRequest(r workflow.HITLRequest) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "**HITL approval needed**: `%s/%s`\n", r.WorkflowName, r.NodeID)
	if r.ApproverRole != "" {
		fmt.Fprintf(&sb, "Required role: `%s`\n", r.ApproverRole)
	}
	if r.Reason != "" {
		fmt.Fprintf(&sb, "Reason: %s\n", r.Reason)
	}
	if r.Confidence > 0 {
		fmt.Fprintf(&sb, "Reported confidence: %.2f\n", r.Confidence)
	}
	if r.NodeOutput != "" {
		preview := r.NodeOutput
		if len(preview) > 500 {
			preview = preview[:500] + "…"
		}
		fmt.Fprintf(&sb, "```\n%s\n```\n", preview)
	}
	sb.WriteString("Reply `approve` or `reject` (`lgtm`/`deny` accepted).")
	return sb.String()
}

// parseDecision returns the HITLDecision encoded by content's first word.
// (HITLDecision, true) on a recognised prefix; (_, false) otherwise so
// the caller can ignore non-decision replies.
func parseDecision(content string) (workflow.HITLDecision, bool) {
	first := strings.ToLower(strings.TrimSpace(content))
	if i := strings.IndexFunc(first, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t'
	}); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSuffix(first, "!")
	switch first {
	case "approve", "ok", "lgtm", "yes", "y":
		return workflow.HITLDecisionApprove, true
	case "reject", "deny", "no", "n":
		return workflow.HITLDecisionReject, true
	}
	return "", false
}
