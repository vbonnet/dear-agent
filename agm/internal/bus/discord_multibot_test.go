package bus

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// mockGuildClient is a test double for guildBotClient.
type mockGuildClient struct {
	mu        sync.Mutex
	me        *discordgo.User
	meWait    <-chan struct{}
	sent      []sentMsg
	pages     [][]*discordgo.Message // ChannelMessages returns these in order
	pageIdx   int
	singleDel []string
	bulkDel   [][]string
	openErr   error
	sendErr   error
	closed    bool
	handlers  []any
}

type sentMsg struct {
	channelID string
	data      *discordgo.MessageSend
}

func (m *mockGuildClient) Me() (*discordgo.User, error) {
	if m.meWait != nil {
		<-m.meWait
	}
	return m.me, nil
}

func (m *mockGuildClient) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	m.sent = append(m.sent, sentMsg{channelID: channelID, data: data})
	return &discordgo.Message{ID: "posted"}, nil
}

func (m *mockGuildClient) ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string) ([]*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pageIdx >= len(m.pages) {
		return nil, nil
	}
	p := m.pages[m.pageIdx]
	m.pageIdx++
	return p, nil
}

func (m *mockGuildClient) ChannelMessageDelete(channelID, messageID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.singleDel = append(m.singleDel, messageID)
	return nil
}

func (m *mockGuildClient) ChannelMessagesBulkDelete(channelID string, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bulkDel = append(m.bulkDel, ids)
	return nil
}

func (m *mockGuildClient) AddHandler(h any) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, h)
	return func() {}
}

func (m *mockGuildClient) Open() error  { return m.openErr }
func (m *mockGuildClient) Close() error { m.mu.Lock(); m.closed = true; m.mu.Unlock(); return nil }

func (m *mockGuildClient) sentSnapshot() []sentMsg {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]sentMsg, len(m.sent))
	copy(out, m.sent)
	return out
}

// mbCapture records frames routed to a bus session (named to avoid colliding
// with captureDelivery in discord_test.go).
type mbCapture struct {
	mu     sync.Mutex
	frames []*Frame
}

func (c *mbCapture) Deliver(f *Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.frames = append(c.frames, f)
	return nil
}
func (c *mbCapture) Close() error { return nil }
func (c *mbCapture) snapshot() []*Frame {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*Frame, len(c.frames))
	copy(out, c.frames)
	return out
}

const testChannel = "chan-1"

// newMultibotTest builds an adapter with two agents (claude, codex) backed by
// mock clients keyed by token.
func newMultibotTest(t *testing.T) (*MultiBotDiscordAdapter, map[string]*mockGuildClient, *Registry) {
	t.Helper()
	reg := NewRegistry()
	clients := map[string]*mockGuildClient{
		"tok-claude": {me: &discordgo.User{ID: "1001", Username: "Claude"}},
		"tok-codex":  {me: &discordgo.User{ID: "1002", Username: "Codex"}},
	}
	a := &MultiBotDiscordAdapter{
		Agents: []*DiscordAgent{
			{Name: "claude", Token: "tok-claude", BusSession: "claude-portal"},
			{Name: "codex", Token: "tok-codex", BusSession: "codex-portal"},
		},
		ChannelID: testChannel,
		Registry:  reg,
		newClient: func(token string) (guildBotClient, error) { return clients[token], nil },
	}
	return a, clients, reg
}

func startMultibot(t *testing.T, a *MultiBotDiscordAdapter) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	startExited, err := waitForMultibotReady(a, done, 2*time.Second)
	if err != nil {
		if startExited {
			cancel()
		} else if stopErr := stopMultibot(cancel, done, 3*time.Second); stopErr != nil {
			t.Error(stopErr)
		}
		t.Fatal(err)
		return nil
	}
	return func() {
		if err := stopMultibot(cancel, done, 3*time.Second); err != nil {
			t.Error(err)
		}
	}
}

func waitForMultibotReady(a *MultiBotDiscordAdapter, done <-chan error, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	poll := time.NewTicker(5 * time.Millisecond)
	defer deadline.Stop()
	defer poll.Stop()
	for !multibotReady(a) {
		select {
		case err := <-done:
			if err != nil {
				return true, fmt.Errorf("start adapter: %w", err)
			}
			return true, errors.New("adapter stopped before becoming ready")
		case <-deadline.C:
			return false, errors.New("adapter did not become ready within timeout")
		case <-poll.C:
		}
	}
	return false, nil
}

func stopMultibot(cancel context.CancelFunc, done <-chan error, timeout time.Duration) error {
	cancel()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("stop adapter: %w", err)
		}
		return nil
	case <-time.After(timeout):
		return errors.New("adapter did not stop within timeout")
	}
}

func multibotReady(a *MultiBotDiscordAdapter) bool {
	a.mu.Lock()
	started := a.started
	a.mu.Unlock()
	if !started {
		return false
	}
	for _, agent := range a.Agents {
		delivery, err := a.Registry.Route(discordAgentSessionID(agent.Name))
		routed, ok := delivery.(*multiBotDelivery)
		if err != nil || !ok || routed.agent != agent || routed.adapter != a {
			return false
		}
	}
	return true
}

func TestMultibotReadyRequiresEveryAgentAndListener(t *testing.T) {
	a, clients, _ := newMultibotTest(t)
	codexIdentity := make(chan struct{})
	clients["tok-codex"].meWait = codexIdentity
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()
	released := false
	startDoneConsumed := false
	defer func() {
		if !released {
			close(codexIdentity)
		}
		if startDoneConsumed {
			cancel()
			return
		}
		if err := stopMultibot(cancel, done, 3*time.Second); err != nil {
			t.Error(err)
		}
	}()

	deadline := time.Now().Add(2 * time.Second)
	claudeReady := false
	for time.Now().Before(deadline) {
		if _, err := a.Registry.Route(discordAgentSessionID("claude")); err == nil {
			claudeReady = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !claudeReady {
		t.Fatal("claude pseudo-session was not registered before timeout")
	}
	if multibotReady(a) {
		t.Fatal("adapter reported ready while codex identity was blocked")
	}
	startExited, err := waitForMultibotReady(a, done, 20*time.Millisecond)
	if startExited {
		startDoneConsumed = true
		t.Fatalf("adapter exited during partial startup: %v", err)
	}
	if err == nil {
		t.Fatal("readiness wait returned during partial startup")
	}

	close(codexIdentity)
	released = true
	startExited, err = waitForMultibotReady(a, done, 2*time.Second)
	if startExited {
		startDoneConsumed = true
	}
	if startExited || err != nil {
		t.Fatalf("readiness wait failed after complete startup: exited=%t err=%v", startExited, err)
	}
}

func msg(channel, guild, authorID string, bot bool, content string, mentionIDs ...string) *discordgo.MessageCreate {
	var mentions []*discordgo.User
	for _, id := range mentionIDs {
		mentions = append(mentions, &discordgo.User{ID: id})
	}
	return &discordgo.MessageCreate{Message: &discordgo.Message{
		ID:        "msg-1",
		Author:    &discordgo.User{ID: authorID, Bot: bot},
		ChannelID: channel,
		GuildID:   guild,
		Content:   content,
		Mentions:  mentions,
	}}
}

func TestStripMentions(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"<@123> hello there", "hello there"},
		{"<@!456>   spaced   out", "spaced out"},
		{"no mentions here", "no mentions here"},
		{"<@1> <@2> middle <@3>", "middle"},
		{"", ""},
	} {
		if got := stripMentions(tc.in); got != tc.want {
			t.Errorf("stripMentions(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestChunkMessage(t *testing.T) {
	if got := chunkMessage("short", 2000); len(got) != 1 || got[0] != "short" {
		t.Fatalf("short: got %v", got)
	}
	// Long with newline breaks: ensure all chunks <= max and content preserved.
	long := strings.Repeat("abcd\n", 800) // 4000 runes
	chunks := chunkMessage(long, 2000)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len([]rune(c)) > 2000 {
			t.Errorf("chunk %d exceeds max: %d", i, len([]rune(c)))
		}
	}
	// No separators: hard split, still <= max, nothing lost.
	noSep := strings.Repeat("x", 5000)
	cs := chunkMessage(noSep, 2000)
	total := 0
	for _, c := range cs {
		if len([]rune(c)) > 2000 {
			t.Errorf("hard-split chunk exceeds max: %d", len([]rune(c)))
		}
		total += len([]rune(c))
	}
	if total != 5000 {
		t.Errorf("hard-split lost content: total=%d want 5000", total)
	}
}

func TestMultiBotDelivery_PostsAsAgentThreadedChunked(t *testing.T) {
	a, clients, reg := newMultibotTest(t)
	stop := startMultibot(t, a)
	defer stop()

	d, err := reg.Route(discordAgentSessionID("claude"))
	if err != nil {
		t.Fatalf("claude pseudo-session not registered: %v", err)
	}
	body := strings.Repeat("y", 4500) // forces >=3 chunks
	if err := d.Deliver(&Frame{
		Type: FrameDeliver, From: portalSessionID, To: "claude-portal",
		Text: body, Extra: map[string]string{"discord_msg_id": "msg-1"},
	}); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	sent := clients["tok-claude"].sentSnapshot()
	if len(sent) < 3 {
		t.Fatalf("expected chunked posts (>=3), got %d", len(sent))
	}
	// Posted to the right channel, threaded on first chunk only, mentions scoped.
	if sent[0].channelID != testChannel {
		t.Errorf("wrong channel: %s", sent[0].channelID)
	}
	if sent[0].data.Reference == nil || sent[0].data.Reference.MessageID != "msg-1" {
		t.Errorf("first chunk not threaded to msg-1: %+v", sent[0].data.Reference)
	}
	if sent[1].data.Reference != nil {
		t.Errorf("non-first chunk should not carry a reference")
	}
	if sent[0].data.AllowedMentions == nil ||
		len(sent[0].data.AllowedMentions.Parse) != 1 ||
		sent[0].data.AllowedMentions.Parse[0] != discordgo.AllowedMentionTypeUsers {
		t.Errorf("allowed-mentions not scoped to users: %+v", sent[0].data.AllowedMentions)
	}
	// Codex bot must not have posted anything.
	if len(clients["tok-codex"].sentSnapshot()) != 0 {
		t.Errorf("codex bot posted unexpectedly")
	}
}

func TestHandleMessageCreate_RoutesMentionToBusSession(t *testing.T) {
	a, _, reg := newMultibotTest(t)
	stop := startMultibot(t, a)
	defer stop()

	cap := &mbCapture{}
	if err := reg.Register("claude-portal", cap); err != nil {
		t.Fatalf("register bus session: %v", err)
	}
	a.handleMessageCreate(nil, msg(testChannel, "", "human-1", false, "<@1001> do the thing", "1001"))

	frames := cap.snapshot()
	if len(frames) != 1 {
		t.Fatalf("expected 1 routed frame, got %d", len(frames))
	}
	f := frames[0]
	if f.To != "claude-portal" || f.Text != "do the thing" {
		t.Errorf("bad routed frame: To=%q Text=%q", f.To, f.Text)
	}
	if f.Extra["agent"] != "claude" || f.Extra["discord_msg_id"] != "msg-1" ||
		f.Extra["discord_channel"] != testChannel || f.Extra["discord_author"] != "human-1" {
		t.Errorf("missing/incorrect Extra: %+v", f.Extra)
	}
}

func TestHandleMessageCreate_IgnoreRules(t *testing.T) {
	a, _, reg := newMultibotTest(t)
	stop := startMultibot(t, a)
	defer stop()
	cap := &mbCapture{}
	_ = reg.Register("claude-portal", cap)

	// bot author -> ignored
	a.handleMessageCreate(nil, msg(testChannel, "", "x", true, "<@1001> hi", "1001"))
	// wrong channel -> ignored
	a.handleMessageCreate(nil, msg("other", "", "h", false, "<@1001> hi", "1001"))
	// no mention -> ignored
	a.handleMessageCreate(nil, msg(testChannel, "", "h", false, "just chatting"))
	if n := len(cap.snapshot()); n != 0 {
		t.Fatalf("expected 0 routed frames, got %d", n)
	}

	// author allowlist enforced
	a.mu.Lock()
	a.allowedAuthor = map[string]bool{"allowed-user": true}
	a.mu.Unlock()
	a.handleMessageCreate(nil, msg(testChannel, "", "blocked-user", false, "<@1001> hi", "1001"))
	if n := len(cap.snapshot()); n != 0 {
		t.Fatalf("disallowed author should be ignored, got %d frames", n)
	}
}

func TestHandleMessageCreate_OfflineAgentPostsNotice(t *testing.T) {
	a, clients, _ := newMultibotTest(t)
	stop := startMultibot(t, a)
	defer stop()
	// No bus session registered for codex-portal -> offline.
	a.handleMessageCreate(nil, msg(testChannel, "", "h", false, "<@1002> ping", "1002"))
	sent := clients["tok-codex"].sentSnapshot()
	if len(sent) != 1 || !strings.Contains(sent[0].data.Content, "not connected") {
		t.Fatalf("expected offline notice as codex bot, got %+v", sent)
	}
	if sent[0].data.Reference == nil || sent[0].data.Reference.MessageID != "msg-1" {
		t.Errorf("offline notice should thread to the prompt")
	}
}

func TestResetChannel(t *testing.T) {
	now := time.Now()
	// Each page carries >=2 recent messages so bulk delete (which Discord
	// rejects for <2 IDs) is genuinely exercised; the lone-recent case is
	// covered separately by TestResetChannel_SingleRecentMessage.
	m := &mockGuildClient{pages: [][]*discordgo.Message{
		{
			{ID: "a", Timestamp: now.Add(-1 * time.Hour)},       // recent -> bulk
			{ID: "a2", Timestamp: now.Add(-90 * time.Minute)},   // recent -> bulk
			{ID: "b", Timestamp: now.Add(-20 * 24 * time.Hour)}, // old -> single
		},
		{
			{ID: "c", Timestamp: now.Add(-2 * time.Hour)},  // recent -> bulk
			{ID: "c2", Timestamp: now.Add(-3 * time.Hour)}, // recent -> bulk
		},
		nil, // end
	}}
	n, err := ResetChannel(m, "chan-x", nil)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n != 5 {
		t.Errorf("deleted = %d, want 5", n)
	}
	if len(m.singleDel) != 1 || m.singleDel[0] != "b" {
		t.Errorf("single deletes = %v, want [b]", m.singleDel)
	}
	if len(m.bulkDel) != 2 {
		t.Errorf("expected two bulk-delete calls, got %d", len(m.bulkDel))
	}
}

// TestResetChannel_SingleRecentMessage covers the case where a page yields
// exactly one recent message: Discord's bulk-delete rejects batches of <2, so
// the lone message must fall through to the single-delete path.
func TestResetChannel_SingleRecentMessage(t *testing.T) {
	now := time.Now()
	m := &mockGuildClient{pages: [][]*discordgo.Message{
		{
			{ID: "x", Timestamp: now.Add(-1 * time.Hour)}, // lone recent -> single delete, NOT bulk
		},
		nil, // end
	}}
	n, err := ResetChannel(m, "chan-y", nil)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n != 1 {
		t.Errorf("deleted = %d, want 1", n)
	}
	if len(m.singleDel) != 1 || m.singleDel[0] != "x" {
		t.Errorf("single deletes = %v, want [x]", m.singleDel)
	}
	if len(m.bulkDel) != 0 {
		t.Errorf("expected no bulk-delete calls for a lone message, got %d", len(m.bulkDel))
	}
}

func TestLoadDiscordAgentsConfig(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "ok.yaml")
	os.WriteFile(good, []byte(`
channel: "C1"
guild: "G1"
agents:
  - name: claude
    token: t1
    bus_session: claude-portal
  - name: codex
    token: t2
    bus_session: codex-portal
`), 0o600)
	cfg, loose, err := LoadDiscordAgentsConfig(good)
	if err != nil {
		t.Fatalf("valid config errored: %v", err)
	}
	if loose {
		t.Errorf("0600 file flagged as loose perms")
	}
	if cfg.Channel != "C1" || len(cfg.ToAgents()) != 2 {
		t.Errorf("bad parse: %+v", cfg)
	}

	loosePath := filepath.Join(dir, "loose.yaml")
	os.WriteFile(loosePath, []byte("channel: C\nagents:\n  - {name: a, token: t, bus_session: s}\n"), 0o644)
	if _, loose, _ := LoadDiscordAgentsConfig(loosePath); !loose {
		t.Errorf("0644 file not flagged as loose perms")
	}

	for name, body := range map[string]string{
		"missing channel": "agents:\n  - {name: a, token: t, bus_session: s}\n",
		"no agents":       "channel: C\n",
		"dup name":        "channel: C\nagents:\n  - {name: a, token: t, bus_session: s}\n  - {name: a, token: u, bus_session: r}\n",
		"missing token":   "channel: C\nagents:\n  - {name: a, bus_session: s}\n",
	} {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".yaml")
		os.WriteFile(p, []byte(body), 0o600)
		if _, _, err := LoadDiscordAgentsConfig(p); err == nil {
			t.Errorf("%s: expected error, got nil", name)
		}
	}
}
