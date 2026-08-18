// Package bus — multi-bot Discord guild-channel portal (ADR-028).
//
// MultiBotDiscordAdapter runs N Discord bot identities (one per agent) in a
// single guild channel so the user can @mention @Claude / @Codex / etc. and
// see, per message, which agent replied. Attribution is structural: each
// reply is posted by that agent's own bot user (not a text prefix).
//
// It is parallel to, and independent of, DiscordAdapter (discord.go), which
// remains the single-bot DM relay for HITL. This adapter never touches the
// DM path or its discordClient interface; it uses its own guildBotClient.
//
// Routing reuses the broker: each agent gets a pseudo-session
// "discord:agent:<name>" in the Registry. Inbound mentions become a
// FrameDeliver to the agent's real bus session; the agent replies via the
// agm-bus MCP `send` tool targeting "discord:agent:<name>", and this adapter
// posts that reply as the agent's bot. Identity/correlation ride in
// Frame.Extra (agent, discord_channel, discord_msg_id, discord_guild).
package bus

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

// discordMaxMessage is Discord's hard per-message content limit. Agent
// output routinely exceeds it, so Deliver chunks to this size.
const discordMaxMessage = 2000

// guildBotClient is the narrow Discord surface the portal needs. It is a
// sibling of discordClient (kept separate so the DM adapter and its mock
// stay untouched per ADR-028).
type guildBotClient interface {
	// Me returns the authenticated bot's own user (GET /users/@me), used to
	// learn each bot's user id for mention matching.
	Me() (*discordgo.User, error)

	// ChannelMessageSendComplex posts a message (supports reply reference and
	// allowed-mentions scoping).
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error)

	// ChannelMessages lists messages in a channel (paging via beforeID).
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string) ([]*discordgo.Message, error)

	// ChannelMessageDelete deletes a single message.
	ChannelMessageDelete(channelID, messageID string) error

	// ChannelMessagesBulkDelete deletes up to 100 messages <14 days old.
	ChannelMessagesBulkDelete(channelID string, messageIDs []string) error

	// AddHandler registers a discordgo gateway handler. Returns a remover.
	AddHandler(handler any) func()

	// Open connects the gateway websocket (only the listener calls this).
	Open() error

	// Close disconnects the gateway websocket.
	Close() error
}

// guildBotSession adapts *discordgo.Session to guildBotClient.
type guildBotSession struct{ s *discordgo.Session }

func (g *guildBotSession) Me() (*discordgo.User, error) { return g.s.User("@me") }

func (g *guildBotSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	return g.s.ChannelMessageSendComplex(channelID, data)
}

func (g *guildBotSession) ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string) ([]*discordgo.Message, error) {
	return g.s.ChannelMessages(channelID, limit, beforeID, afterID, aroundID)
}

func (g *guildBotSession) ChannelMessageDelete(channelID, messageID string) error {
	return g.s.ChannelMessageDelete(channelID, messageID)
}

func (g *guildBotSession) ChannelMessagesBulkDelete(channelID string, messageIDs []string) error {
	return g.s.ChannelMessagesBulkDelete(channelID, messageIDs)
}

func (g *guildBotSession) AddHandler(handler any) func() { return g.s.AddHandler(handler) }
func (g *guildBotSession) Open() error                   { return g.s.Open() }
func (g *guildBotSession) Close() error                  { return g.s.Close() }

// DiscordAgent is one mentionable agent bot in the portal.
type DiscordAgent struct {
	// Name is the logical agent name (e.g. "claude", "codex"). Used for the
	// pseudo-session id discord:agent:<name> and Frame.Extra["agent"].
	Name string

	// Token is the raw Discord bot token for this agent's application
	// (the "Bot " prefix is added internally).
	Token string

	// BusSession is the agm-bus session id where this agent's MCP client is
	// registered; inbound mentions are delivered there.
	BusSession string

	// userID is this bot's Discord user id, resolved at Start via Me().
	userID string

	// client is this bot's Discord client (REST for all; gateway only for
	// the designated listener).
	client guildBotClient
}

// multiBotDelivery is the Registry Delivery for discord:agent:<name>. When the
// agent's MCP `send` reaches this pseudo-session, Deliver posts the reply to
// the channel as that agent's bot, threaded to the triggering message.
type multiBotDelivery struct {
	agent   *DiscordAgent
	adapter *MultiBotDiscordAdapter
}

func (d *multiBotDelivery) Deliver(f *Frame) error {
	text := strings.TrimSpace(f.Text)
	if text == "" {
		// Non-text frame types (e.g. permission relay) are summarized so the
		// channel still shows something attributable rather than silence.
		text = fmt.Sprintf("[%s] (no text)", f.Type)
	}

	var ref *discordgo.MessageReference
	if mid := f.Extra["discord_msg_id"]; mid != "" {
		ref = &discordgo.MessageReference{MessageID: mid, ChannelID: d.adapter.ChannelID}
	}
	chunks := chunkMessage(text, discordMaxMessage)
	for i, chunk := range chunks {
		send := &discordgo.MessageSend{
			Content: chunk,
			// Only allow user mentions; never let an agent @everyone/@here.
			AllowedMentions: &discordgo.MessageAllowedMentions{
				Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
			},
		}
		if i == 0 {
			send.Reference = ref // thread only the first chunk to the prompt
		}
		if _, err := d.agent.client.ChannelMessageSendComplex(d.adapter.ChannelID, send); err != nil {
			return fmt.Errorf("discord-multibot: post as %q: %w", d.agent.Name, err)
		}
	}
	return nil
}

func (d *multiBotDelivery) Close() error { return nil }

// MultiBotDiscordAdapter bridges the agm-bus broker and a single Discord guild
// channel via N per-agent bot identities. See ADR-028.
type MultiBotDiscordAdapter struct {
	// Agents is the set of mentionable agent bots.
	Agents []*DiscordAgent

	// ChannelID is the one channel the portal operates in. Required.
	ChannelID string

	// GuildID, if set, restricts handling to that guild (defense in depth).
	GuildID string

	// Registry is the broker's routing table.
	Registry *Registry

	// ACL enforces send permissions (sender = "discord:portal"). May be nil.
	ACL interface {
		Check(sender, target string) ACLDecision
	}

	// Logger for adapter-level events.
	Logger *slog.Logger

	// AuthorAllowlist optionally restricts which human Discord user ids may
	// invoke agents. Empty = allow any human (bots are always ignored).
	AuthorAllowlist []string

	// newClient builds a guildBotClient for a token; overridable in tests.
	// Production default sets guild-message + message-content intents.
	newClient func(token string) (guildBotClient, error)

	mu            sync.Mutex
	agentByUserID map[string]*DiscordAgent
	agentByName   map[string]*DiscordAgent
	allowedAuthor map[string]bool
	listener      guildBotClient
	started       bool
}

// portalSessionID is the synthetic sender id for frames the portal injects.
const portalSessionID = "discord:portal"

// defaultGuildBotClient builds a real discordgo-backed client with the gateway
// intents the listener needs (harmless for REST-only agents that never Open).
func defaultGuildBotClient(token string) (guildBotClient, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discord-multibot: create session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentGuildMessages | discordgo.IntentMessageContent
	return &guildBotSession{s: s}, nil
}

// Start resolves each bot's identity, registers pseudo-sessions, opens the
// single listener gateway, and blocks until ctx is cancelled.
func (a *MultiBotDiscordAdapter) Start(ctx context.Context) error {
	if a.Logger == nil {
		a.Logger = slog.Default()
	}
	if a.ChannelID == "" {
		return fmt.Errorf("discord-multibot: ChannelID is required")
	}
	if len(a.Agents) == 0 {
		return fmt.Errorf("discord-multibot: no agents configured")
	}
	if a.newClient == nil {
		a.newClient = defaultGuildBotClient
	}

	a.mu.Lock()
	a.agentByUserID = make(map[string]*DiscordAgent, len(a.Agents))
	a.agentByName = make(map[string]*DiscordAgent, len(a.Agents))
	a.allowedAuthor = make(map[string]bool, len(a.AuthorAllowlist))
	for _, id := range a.AuthorAllowlist {
		if id = strings.TrimSpace(id); id != "" {
			a.allowedAuthor[id] = true
		}
	}
	a.mu.Unlock()

	// Build each agent's client and resolve its bot user id.
	for _, ag := range a.Agents {
		if ag.Name == "" || ag.Token == "" || ag.BusSession == "" {
			return fmt.Errorf("discord-multibot: agent needs name, token, bus_session (got name=%q)", ag.Name)
		}
		c, err := a.newClient(ag.Token)
		if err != nil {
			_ = a.Stop() // close any already-built clients; don't leak them
			return fmt.Errorf("discord-multibot: agent %q: %w", ag.Name, err)
		}
		ag.client = c
		me, err := c.Me()
		if err != nil {
			_ = a.Stop() // close this client and earlier ones; don't leak them
			return fmt.Errorf("discord-multibot: agent %q identify: %w", ag.Name, err)
		}
		ag.userID = me.ID

		a.mu.Lock()
		a.agentByUserID[me.ID] = ag
		a.agentByName[ag.Name] = ag
		a.mu.Unlock()

		sessionID := discordAgentSessionID(ag.Name)
		if err := a.Registry.Register(sessionID, &multiBotDelivery{agent: ag, adapter: a}); err != nil {
			a.Logger.Warn("discord-multibot: pseudo-session already registered",
				"session", sessionID, "err", err)
		}
		a.Logger.Info("discord-multibot: agent ready",
			"name", ag.Name, "user_id", me.ID, "bus_session", ag.BusSession)
	}

	// One gateway connection (first agent) listens for everyone's mentions.
	a.listener = a.Agents[0].client
	a.listener.AddHandler(a.handleMessageCreate)
	if err := a.listener.Open(); err != nil {
		_ = a.Stop() // unregister sessions AND close every client; don't leak them
		return fmt.Errorf("discord-multibot: open listener gateway: %w", err)
	}
	a.mu.Lock()
	a.started = true
	a.mu.Unlock()
	a.Logger.Info("discord-multibot portal started",
		"agents", len(a.Agents), "channel", a.ChannelID, "listener", a.Agents[0].Name)

	<-ctx.Done()
	return a.Stop()
}

// Stop unregisters pseudo-sessions and closes all bot clients.
func (a *MultiBotDiscordAdapter) Stop() error {
	a.unregisterAll()
	for _, ag := range a.Agents {
		if ag.client != nil {
			if err := ag.client.Close(); err != nil {
				a.Logger.Warn("discord-multibot: close client error", "agent", ag.Name, "err", err)
			}
		}
	}
	a.Logger.Info("discord-multibot portal stopped")
	return nil
}

func (a *MultiBotDiscordAdapter) unregisterAll() {
	for _, ag := range a.Agents {
		a.Registry.Unregister(discordAgentSessionID(ag.Name))
	}
}

// handleMessageCreate routes a channel message to every mentioned agent.
// Signature mirrors discord.go's proven handler shape (interface{} first arg).
func (a *MultiBotDiscordAdapter) handleMessageCreate(_ any, m *discordgo.MessageCreate) {
	if m == nil || m.Author == nil || m.Author.Bot {
		return
	}
	if m.ChannelID != a.ChannelID {
		return
	}
	if a.GuildID != "" && m.GuildID != a.GuildID {
		return
	}

	a.mu.Lock()
	authorOK := len(a.allowedAuthor) == 0 || a.allowedAuthor[m.Author.ID]
	a.mu.Unlock()
	if !authorOK {
		a.Logger.Debug("discord-multibot: ignoring non-allowlisted author", "user", m.Author.ID)
		return
	}
	if len(m.Mentions) == 0 {
		return
	}

	// Collect every distinct agent mentioned (one message can address several).
	seen := make(map[string]bool)
	for _, u := range m.Mentions {
		a.mu.Lock()
		ag := a.agentByUserID[u.ID]
		a.mu.Unlock()
		if ag == nil || seen[ag.Name] {
			continue
		}
		seen[ag.Name] = true
		a.dispatch(ag, m)
	}
}

// dispatch delivers one mention to one agent's bus session, or posts an
// in-channel notice (as that agent) if it is offline / denied.
func (a *MultiBotDiscordAdapter) dispatch(ag *DiscordAgent, m *discordgo.MessageCreate) {
	text := stripMentions(m.Content)
	if text == "" {
		text = "(no message body)"
	}

	if a.ACL != nil {
		if d := a.ACL.Check(portalSessionID, ag.BusSession); !d.Allowed {
			a.Logger.Debug("discord-multibot: send denied by ACL",
				"agent", ag.Name, "reason", d.Reason)
			a.noticeAs(ag, m.ID, fmt.Sprintf("⚠ routing to %s denied by ACL: %s", ag.Name, d.Reason))
			return
		}
	}

	frame := &Frame{
		Type: FrameDeliver,
		ID:   fmt.Sprintf("discord-%d", time.Now().UnixNano()),
		From: portalSessionID,
		To:   ag.BusSession,
		Text: text,
		TS:   time.Now().UTC(),
		Extra: map[string]string{
			"agent":           ag.Name,
			"discord_channel": m.ChannelID,
			"discord_msg_id":  m.ID,
			"discord_guild":   m.GuildID,
			"discord_author":  m.Author.ID,
		},
	}

	delivery, err := a.Registry.Route(ag.BusSession)
	if err != nil {
		a.Logger.Debug("discord-multibot: agent offline",
			"agent", ag.Name, "bus_session", ag.BusSession)
		a.noticeAs(ag, m.ID, fmt.Sprintf("⚠ %s is not connected right now; message not delivered.", ag.Name))
		return
	}
	if err := delivery.Deliver(frame); err != nil {
		a.Logger.Warn("discord-multibot: deliver failed", "agent", ag.Name, "err", err)
		a.noticeAs(ag, m.ID, fmt.Sprintf("⚠ failed to deliver to %s: %v", ag.Name, err))
	}
}

// noticeAs posts a short status message in-channel as the given agent's bot,
// threaded to the triggering message. Best-effort; errors are logged only.
func (a *MultiBotDiscordAdapter) noticeAs(ag *DiscordAgent, replyTo, text string) {
	send := &discordgo.MessageSend{
		Content: text,
		AllowedMentions: &discordgo.MessageAllowedMentions{
			Parse: []discordgo.AllowedMentionType{discordgo.AllowedMentionTypeUsers},
		},
	}
	if replyTo != "" {
		send.Reference = &discordgo.MessageReference{MessageID: replyTo, ChannelID: a.ChannelID}
	}
	if _, err := ag.client.ChannelMessageSendComplex(a.ChannelID, send); err != nil {
		a.Logger.Warn("discord-multibot: notice post failed", "agent", ag.Name, "err", err)
	}
}

// discordAgentSessionID is the pseudo-session id for an agent's outbound path.
func discordAgentSessionID(name string) string { return "discord:agent:" + name }

// mentionRe matches Discord user mention tokens <@123> and <@!123>.
var mentionRe = regexp.MustCompile(`<@!?\d+>`)

// stripMentions removes all user-mention tokens and collapses whitespace, so
// the agent receives the human's intent without the "<@id>" noise.
func stripMentions(content string) string {
	return strings.Join(strings.Fields(mentionRe.ReplaceAllString(content, " ")), " ")
}

// chunkMessage splits s into pieces no longer than limit runes, preferring to
// break at the last newline, else the last space, within the window. Never
// returns an empty slice.
//
//nolint:unparam // limit is parameterized for unit testing; prod always passes discordMaxMessage
func chunkMessage(s string, limit int) []string {
	r := []rune(s)
	if len(r) <= limit {
		return []string{s}
	}
	var out []string
	for len(r) > limit {
		cut := limit
		// Prefer a newline break, then a space break, within [limit/2, limit].
		if i := lastIndexRune(r[:limit], '\n'); i >= limit/2 {
			cut = i + 1
		} else if i := lastIndexRune(r[:limit], ' '); i >= limit/2 {
			cut = i + 1
		}
		out = append(out, strings.TrimRight(string(r[:cut]), " \n"))
		r = r[cut:]
	}
	if rest := strings.TrimSpace(string(r)); rest != "" {
		out = append(out, rest)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func lastIndexRune(r []rune, target rune) int {
	for i := range slices.Backward(r) {
		if r[i] == target {
			return i
		}
	}
	return -1
}

// ResetChannelByToken builds a REST client for the given bot token and purges
// channelID. The bot must have MANAGE_MESSAGES in that channel. Thin exported
// entrypoint for the gated `agm-bus discord-reset` subcommand (the
// guildBotClient interface is intentionally unexported).
func ResetChannelByToken(token, channelID string, logger *slog.Logger) (int, error) {
	c, err := defaultGuildBotClient(token)
	if err != nil {
		return 0, err
	}
	return ResetChannel(c, channelID, logger)
}

// ResetChannel deletes messages in channelID newest-first using bulk delete
// for messages <14 days old and per-message delete for older ones. Returns
// the number deleted. Used by the gated `agm-bus discord-reset` subcommand;
// never called from the serve path. Stops at the first hard error.
func ResetChannel(client guildBotClient, channelID string, logger *slog.Logger) (int, error) {
	if logger == nil {
		logger = slog.Default()
	}
	deleted := 0
	for {
		// Page with an empty before each iteration: every message returned here
		// is about to be deleted, so the previous page's IDs drop out and the
		// newest-remaining message advances the window naturally. Using the last
		// message's ID as the cursor would point at a just-deleted message.
		msgs, err := client.ChannelMessages(channelID, 100, "", "", "")
		if err != nil {
			return deleted, fmt.Errorf("discord-reset: list messages: %w", err)
		}
		if len(msgs) == 0 {
			return deleted, nil
		}
		var bulk []string
		cutoff := time.Now().Add(-14 * 24 * time.Hour)
		for _, msg := range msgs {
			if msg.Timestamp.After(cutoff) {
				bulk = append(bulk, msg.ID)
				continue
			}
			if err := client.ChannelMessageDelete(channelID, msg.ID); err != nil {
				return deleted, fmt.Errorf("discord-reset: delete %s: %w", msg.ID, err)
			}
			deleted++
		}
		// ChannelMessagesBulkDelete rejects batches with fewer than 2 IDs, so a
		// lone recent message must go through the single-delete path instead.
		if len(bulk) == 1 {
			if err := client.ChannelMessageDelete(channelID, bulk[0]); err != nil {
				return deleted, fmt.Errorf("discord-reset: delete %s: %w", bulk[0], err)
			}
			deleted++
		} else if len(bulk) > 1 {
			if err := client.ChannelMessagesBulkDelete(channelID, bulk); err != nil {
				return deleted, fmt.Errorf("discord-reset: bulk delete: %w", err)
			}
			deleted += len(bulk)
		}
		logger.Info("discord-reset: progress", "deleted", deleted)
	}
}
