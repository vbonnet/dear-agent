// Package bus — Discord adapter for the agm-bus broker.
//
// DiscordAdapter registers pseudo-sessions (discord:<userID>) in the Registry
// so that AGM sessions can send messages to Discord users and Discord users
// can send messages to AGM sessions. Each allowed Discord user gets their own
// delivery endpoint; the adapter acts as an internal broker client rather than
// using the wire protocol over a socket.
package bus

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// discordClient is the narrow Discord interface the adapter uses. Abstracting
// this allows tests to substitute a mock without a live Discord connection.
type discordClient interface {
	// UserChannelCreate opens (or retrieves) a DM channel with the given
	// Discord user ID. Returns the channel ID.
	UserChannelCreate(recipientID string) (*discordgo.Channel, error)

	// ChannelMessageSend posts a plain-text message to channelID.
	ChannelMessageSend(channelID, content string) (*discordgo.Message, error)

	// AddHandler registers a handler for Discord gateway events. The handler
	// signature must match discordgo's expected type (e.g. func(*discordgo.Session, *discordgo.MessageCreate)).
	AddHandler(handler interface{}) func()

	// Open connects to the Discord gateway.
	Open() error

	// Close disconnects from the Discord gateway.
	Close() error
}

// discordgoSession wraps *discordgo.Session to satisfy discordClient.
type discordgoSession struct {
	s *discordgo.Session
}

func (d *discordgoSession) UserChannelCreate(recipientID string) (*discordgo.Channel, error) {
	return d.s.UserChannelCreate(recipientID)
}

func (d *discordgoSession) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	return d.s.ChannelMessageSend(channelID, content)
}

func (d *discordgoSession) AddHandler(handler interface{}) func() {
	return d.s.AddHandler(handler)
}

func (d *discordgoSession) Open() error {
	return d.s.Open()
}

func (d *discordgoSession) Close() error {
	return d.s.Close()
}

// discordDelivery implements Delivery for a single Discord user. Deliver
// forwards a frame to the user via DM; permission requests are formatted
// with instructions to reply "yes <id>" or "no <id>".
type discordDelivery struct {
	userID  string
	adapter *DiscordAdapter
}

// Deliver sends a frame to the Discord user as a DM. Permission requests
// include the request id and yes/no instructions.
func (d *discordDelivery) Deliver(f *Frame) error {
	var text string
	switch f.Type { //nolint:exhaustive // default covers non-special frame types
	case FramePermissionRequest:
		text = fmt.Sprintf(
			"[Permission request from %s]\nTool: %s\n%s\nInput: %s\n\nReply with: yes %s  or  no %s",
			f.From, f.ToolName, f.Description, f.InputPreview, f.ID, f.ID,
		)
	case FrameDeliver:
		text = fmt.Sprintf("[%s] %s", f.From, f.Text)
	default:
		// Deliver any other frame type as JSON-ish summary.
		text = fmt.Sprintf("[%s from %s] %s", f.Type, f.From, f.Text)
	}
	return d.adapter.sendDM(d.userID, text)
}

// Close is a no-op for Discord pseudo-sessions; the connection is managed
// by the discordgo gateway, not per-user.
func (d *discordDelivery) Close() error { return nil }

// DiscordAdapter bridges the agm-bus broker and Discord. It registers one
// pseudo-session per allowed Discord user and handles inbound DMs from the
// Discord gateway.
//
// Usage:
//
//	adapter := &DiscordAdapter{
//	    Token:     "Bot MY_BOT_TOKEN",
//	    Registry:  srv.Registry,
//	    ACL:       srv.ACL,
//	    Logger:    logger,
//	    Allowlist: []string{"123456789"},
//	}
//	if err := adapter.Start(ctx); err != nil { ... }
type DiscordAdapter struct {
	// Token is the Discord bot token (include "Bot " prefix).
	Token string

	// Registry is the broker's active-connection table.
	Registry *Registry

	// ACL enforces send permissions. May be nil (allow-all).
	ACL interface {
		Check(sender, target string) ACLDecision
	}

	// Logger for adapter-level events.
	Logger *slog.Logger

	// Allowlist is the set of Discord user IDs permitted to DM the bot.
	// An empty allowlist denies all inbound DMs.
	Allowlist []string

	// client is the Discord session; set by Start (production uses
	// discordgoSession; tests inject a mock).
	client discordClient

	mu      sync.Mutex
	allowed map[string]bool // snapshot of Allowlist for O(1) lookup
}

// Start connects to the Discord gateway and registers pseudo-sessions for
// each allowed user. It blocks until ctx is cancelled, then unregisters and
// disconnects.
func (a *DiscordAdapter) Start(ctx context.Context) error {
	if a.Logger == nil {
		a.Logger = slog.Default()
	}

	// Build the allowed-user index.
	a.mu.Lock()
	a.allowed = make(map[string]bool, len(a.Allowlist))
	for _, id := range a.Allowlist {
		a.allowed[id] = true
	}
	a.mu.Unlock()

	// Create the discordgo session if not already injected by a test.
	if a.client == nil {
		s, err := discordgo.New(a.Token)
		if err != nil {
			return fmt.Errorf("discord: create session: %w", err)
		}
		// Only receive DMs; no guild member list or presence updates needed.
		s.Identify.Intents = discordgo.IntentsDirectMessages
		a.client = &discordgoSession{s: s}
	}

	// Register each allowed user's pseudo-session so the Registry can route
	// frames destined for discord:<userID>.
	for _, userID := range a.Allowlist {
		sessionID := discordSessionID(userID)
		d := &discordDelivery{userID: userID, adapter: a}
		if err := a.Registry.Register(sessionID, d); err != nil {
			// Already registered from a prior Start call (e.g. restart). Log
			// and continue — the old delivery is still valid.
			a.Logger.Warn("discord: session already registered", "session", sessionID, "err", err)
		}
	}

	// Register the inbound-DM gateway handler.
	removeHandler := a.client.AddHandler(a.handleMessageCreate)
	_ = removeHandler // cleaned up by Close()

	if err := a.client.Open(); err != nil {
		a.unregisterAll()
		return fmt.Errorf("discord: open gateway: %w", err)
	}
	a.Logger.Info("discord adapter started", "users", len(a.Allowlist))

	// Wait for context cancellation.
	<-ctx.Done()
	return a.Stop()
}

// Stop unregisters pseudo-sessions and disconnects from the Discord gateway.
func (a *DiscordAdapter) Stop() error {
	a.unregisterAll()
	if a.client != nil {
		if err := a.client.Close(); err != nil {
			a.Logger.Warn("discord: close gateway error", "err", err)
		}
	}
	a.Logger.Info("discord adapter stopped")
	return nil
}

// unregisterAll removes all pseudo-sessions from the Registry.
func (a *DiscordAdapter) unregisterAll() {
	for _, userID := range a.Allowlist {
		a.Registry.Unregister(discordSessionID(userID))
	}
}

// handleMessageCreate is the discordgo gateway handler for incoming DMs.
// It parses the message, runs the ACL, and enqueues a FrameSend via the
// Registry so the broker routes it like any other session-originated send.
func (a *DiscordAdapter) handleMessageCreate(s interface{}, m *discordgo.MessageCreate) {
	// Ignore bot messages (including the bot itself).
	if m.Author == nil || m.Author.Bot {
		return
	}

	userID := m.Author.ID
	a.mu.Lock()
	isAllowed := a.allowed[userID]
	a.mu.Unlock()
	if !isAllowed {
		a.Logger.Debug("discord: ignoring DM from non-allowlisted user", "user", userID)
		return
	}

	// Only handle DMs (channel type 1 = DM).
	if m.GuildID != "" {
		return
	}

	content := strings.TrimSpace(m.Content)
	if content == "" {
		return
	}

	senderID := discordSessionID(userID)

	// Check for permission verdict replies: "yes <id>" or "no <id>".
	if verdict, reqID, ok := parseVerdictReply(content); ok {
		a.handleVerdictReply(senderID, verdict, reqID)
		return
	}

	// Parse message format: "to:<session-id> <text>" or "<session-id> <text>".
	targetID, text, ok := parseOutboundMessage(content)
	if !ok {
		_ = a.sendDM(userID, "Usage: to:<session-id> <message>  or  <session-id> <message>")
		return
	}

	// ACL check.
	if a.ACL != nil {
		d := a.ACL.Check(senderID, targetID)
		if !d.Allowed {
			_ = a.sendDM(userID, fmt.Sprintf("send denied by ACL: %s", d.Reason))
			a.Logger.Debug("discord: send denied by ACL", "from", senderID, "to", targetID, "reason", d.Reason)
			return
		}
	}

	frame := &Frame{
		Type: FrameDeliver,
		ID:   fmt.Sprintf("discord-%d", time.Now().UnixNano()),
		From: senderID,
		To:   targetID,
		Text: text,
		TS:   time.Now().UTC(),
	}

	delivery, err := a.Registry.Route(targetID)
	if err != nil {
		// Target offline — notify user.
		_ = a.sendDM(userID, fmt.Sprintf("session %q is offline; message not delivered", targetID))
		a.Logger.Debug("discord: target offline", "target", targetID, "user", userID)
		return
	}

	if err := delivery.Deliver(frame); err != nil {
		_ = a.sendDM(userID, fmt.Sprintf("delivery error: %v", err))
		a.Logger.Warn("discord: delivery failed", "target", targetID, "err", err)
	}
}

// handleVerdictReply constructs a FramePermissionVerdict from a Discord DM
// reply and routes it to the target session encoded in the request ID.
// Request IDs are expected to embed the target session via the format
// "<targetSession>/<nonce>" (set by the caller when creating the request).
func (a *DiscordAdapter) handleVerdictReply(senderID, verdict, reqID string) {
	// The reqID carries the target session encoded as "<session>/<nonce>".
	// If the format doesn't match, we can't route without extra state.
	targetSession, _, ok := strings.Cut(reqID, "/")
	if !ok {
		// Fallback: the reqID itself may be the session (simple format).
		targetSession = reqID
	}

	frame := &Frame{
		Type:    FramePermissionVerdict,
		ID:      reqID,
		From:    senderID,
		To:      targetSession,
		Verdict: verdict,
		TS:      time.Now().UTC(),
	}

	delivery, err := a.Registry.Route(targetSession)
	if err != nil {
		userID := strings.TrimPrefix(senderID, "discord:")
		_ = a.sendDM(userID, fmt.Sprintf("session %q is offline; verdict dropped", targetSession))
		return
	}
	if err := delivery.Deliver(frame); err != nil {
		a.Logger.Warn("discord: verdict delivery failed", "target", targetSession, "err", err)
	}
}

// sendDM opens a DM channel with userID and posts text. Errors are logged
// but not fatal — the broker continues regardless of Discord availability.
func (a *DiscordAdapter) sendDM(userID, text string) error {
	ch, err := a.client.UserChannelCreate(userID)
	if err != nil {
		a.Logger.Warn("discord: create DM channel failed", "user", userID, "err", err)
		return fmt.Errorf("discord: create DM channel: %w", err)
	}
	if _, err := a.client.ChannelMessageSend(ch.ID, text); err != nil {
		a.Logger.Warn("discord: send DM failed", "user", userID, "err", err)
		return fmt.Errorf("discord: send DM: %w", err)
	}
	return nil
}

// discordSessionID returns the pseudo-session id for a Discord user.
func discordSessionID(userID string) string {
	return "discord:" + userID
}

// parseOutboundMessage parses a Discord DM into a (targetID, text) pair.
// Accepts "to:<session-id> <text>" or "<session-id> <text>" where the
// session-id is the first whitespace-delimited token. Returns ok=false if
// the message has no target or no message body.
func parseOutboundMessage(content string) (targetID, text string, ok bool) {
	// Strip optional "to:" prefix.
	content = strings.TrimPrefix(content, "to:")
	parts := strings.SplitN(content, " ", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	targetID = strings.TrimSpace(parts[0])
	text = strings.TrimSpace(parts[1])
	if targetID == "" || text == "" {
		return "", "", false
	}
	return targetID, text, true
}

// parseVerdictReply detects "yes <id>" or "no <id>" (case-insensitive).
// Returns (verdict, requestID, true) on match.
func parseVerdictReply(content string) (verdict, reqID string, ok bool) {
	lower := strings.ToLower(content)
	var rest string
	switch {
	case strings.HasPrefix(lower, "yes "):
		verdict = "allow"
		rest = strings.TrimSpace(content[4:])
	case strings.HasPrefix(lower, "no "):
		verdict = "deny"
		rest = strings.TrimSpace(content[3:])
	default:
		return "", "", false
	}
	if rest == "" {
		return "", "", false
	}
	return verdict, rest, true
}
