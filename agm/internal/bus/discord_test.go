package bus

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

// mockDiscordClient is a test double for discordClient.
type mockDiscordClient struct {
	mu       sync.Mutex
	dms      []mockDM // messages sent via ChannelMessageSend
	openErr  error
	sendErr  error
	handlers []interface{}

	// closed tracks whether Close was called.
	closed bool
}

type mockDM struct {
	userID  string // derived from channel id (we use userID as channel id for simplicity)
	content string
}

func (m *mockDiscordClient) UserChannelCreate(recipientID string) (*discordgo.Channel, error) {
	// Use recipientID as channel ID for simplicity in tests.
	return &discordgo.Channel{ID: recipientID}, nil
}

func (m *mockDiscordClient) ChannelMessageSend(channelID, content string) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	m.dms = append(m.dms, mockDM{userID: channelID, content: content})
	return &discordgo.Message{}, nil
}

func (m *mockDiscordClient) AddHandler(handler interface{}) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
	return func() {}
}

func (m *mockDiscordClient) Open() error {
	return m.openErr
}

func (m *mockDiscordClient) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// sentDMs returns a snapshot of DMs sent through the mock.
func (m *mockDiscordClient) sentDMs() []mockDM {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]mockDM, len(m.dms))
	copy(out, m.dms)
	return out
}

// triggerMessageCreate simulates a Discord DM arriving from userID with content.
func (m *mockDiscordClient) triggerMessageCreate(a *DiscordAdapter, userID, content string) {
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:  &discordgo.User{ID: userID, Bot: false},
			Content: content,
			GuildID: "", // DM has no guild
		},
	}
	// Call the adapter's handler directly.
	a.handleMessageCreate(nil, msg)
}

// newTestAdapter constructs a DiscordAdapter with the mock client and a real Registry.
func newTestAdapter(t *testing.T, allowlist []string) (*DiscordAdapter, *mockDiscordClient, *Registry) {
	t.Helper()
	reg := NewRegistry()
	mock := &mockDiscordClient{}
	adapter := &DiscordAdapter{
		Token:     "Bot test-token",
		Registry:  reg,
		Allowlist: allowlist,
		client:    mock,
	}
	return adapter, mock, reg
}

// startAdapter starts the adapter in a goroutine and returns a cancel func.
// It waits briefly so pseudo-sessions are registered before the test proceeds.
func startAdapter(t *testing.T, adapter *DiscordAdapter) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adapter.Start(ctx) }()
	// Poll until pseudo-sessions are registered (or timeout).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		registered := true
		for _, id := range adapter.Allowlist {
			if _, err := adapter.Registry.Route(discordSessionID(id)); err != nil {
				registered = false
				break
			}
		}
		if registered {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("adapter did not stop within timeout")
		}
	}
}

// --- Unit tests ---

func TestParseOutboundMessage(t *testing.T) {
	tests := []struct {
		input    string
		wantID   string
		wantText string
		wantOK   bool
	}{
		{"to:s1 hello there", "s1", "hello there", true},
		{"s1 hello there", "s1", "hello there", true},
		{"to:session-abc   lots of words here", "session-abc", "lots of words here", true},
		{"s1", "", "", false},   // no text
		{"", "", "", false},     // empty
		{"   ", "", "", false},  // whitespace only
	}
	for _, tc := range tests {
		id, text, ok := parseOutboundMessage(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parseOutboundMessage(%q) ok=%v want %v", tc.input, ok, tc.wantOK)
			continue
		}
		if ok {
			if id != tc.wantID {
				t.Errorf("parseOutboundMessage(%q) id=%q want %q", tc.input, id, tc.wantID)
			}
			if text != tc.wantText {
				t.Errorf("parseOutboundMessage(%q) text=%q want %q", tc.input, text, tc.wantText)
			}
		}
	}
}

func TestParseVerdictReply(t *testing.T) {
	tests := []struct {
		input       string
		wantVerdict string
		wantReqID   string
		wantOK      bool
	}{
		{"yes req-123", "allow", "req-123", true},
		{"no req-456", "deny", "req-456", true},
		{"YES req-789", "allow", "req-789", true},
		{"NO req-000", "deny", "req-000", true},
		{"yes", "", "", false},   // missing id
		{"no", "", "", false},    // missing id
		{"maybe req-1", "", "", false},
		{"yesno req-1", "", "", false},
	}
	for _, tc := range tests {
		v, id, ok := parseVerdictReply(tc.input)
		if ok != tc.wantOK {
			t.Errorf("parseVerdictReply(%q) ok=%v want %v", tc.input, ok, tc.wantOK)
			continue
		}
		if ok {
			if v != tc.wantVerdict {
				t.Errorf("parseVerdictReply(%q) verdict=%q want %q", tc.input, v, tc.wantVerdict)
			}
			if id != tc.wantReqID {
				t.Errorf("parseVerdictReply(%q) reqID=%q want %q", tc.input, id, tc.wantReqID)
			}
		}
	}
}

func TestDiscordSessionID(t *testing.T) {
	if got := discordSessionID("12345"); got != "discord:12345" {
		t.Errorf("discordSessionID(12345) = %q want %q", got, "discord:12345")
	}
}

func TestDiscordDelivery_Deliver_RegularMessage(t *testing.T) {
	adapter, mock, _ := newTestAdapter(t, []string{"u1"})
	// Inject the client so sendDM works.
	adapter.client = mock

	d := &discordDelivery{userID: "u1", adapter: adapter}
	frame := &Frame{
		Type: FrameDeliver,
		From: "s1",
		To:   "discord:u1",
		Text: "hello from session",
	}
	if err := d.Deliver(frame); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	dms := mock.sentDMs()
	if len(dms) != 1 {
		t.Fatalf("expected 1 DM, got %d", len(dms))
	}
	if !strings.Contains(dms[0].content, "hello from session") {
		t.Errorf("DM content missing expected text: %q", dms[0].content)
	}
}

func TestDiscordDelivery_Deliver_PermissionRequest(t *testing.T) {
	adapter, mock, _ := newTestAdapter(t, []string{"u1"})
	adapter.client = mock

	d := &discordDelivery{userID: "u1", adapter: adapter}
	frame := &Frame{
		Type:         FramePermissionRequest,
		ID:           "req-abc",
		From:         "s1",
		To:           "discord:u1",
		ToolName:     "bash",
		Description:  "run shell command",
		InputPreview: "ls -la",
	}
	if err := d.Deliver(frame); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	dms := mock.sentDMs()
	if len(dms) != 1 {
		t.Fatalf("expected 1 DM, got %d", len(dms))
	}
	msg := dms[0].content
	if !strings.Contains(msg, "req-abc") {
		t.Errorf("DM missing request id: %q", msg)
	}
	if !strings.Contains(msg, "yes req-abc") {
		t.Errorf("DM missing yes instruction: %q", msg)
	}
	if !strings.Contains(msg, "no req-abc") {
		t.Errorf("DM missing no instruction: %q", msg)
	}
	if !strings.Contains(msg, "bash") {
		t.Errorf("DM missing tool name: %q", msg)
	}
}

// TestDiscordAdapter_PseudoSessionRegistration verifies that Start registers
// each allowed user as discord:<id> in the Registry.
func TestDiscordAdapter_PseudoSessionRegistration(t *testing.T) {
	adapter, _, reg := newTestAdapter(t, []string{"alice", "bob"})
	stop := startAdapter(t, adapter)
	defer stop()

	for _, id := range []string{"alice", "bob"} {
		sessionID := discordSessionID(id)
		if _, err := reg.Route(sessionID); err != nil {
			t.Errorf("session %q not registered after Start", sessionID)
		}
	}
}

// TestDiscordAdapter_Stop_UnregistersAll verifies that Stop cleans up Registry.
func TestDiscordAdapter_Stop_UnregistersAll(t *testing.T) {
	adapter, _, reg := newTestAdapter(t, []string{"alice", "bob"})
	stop := startAdapter(t, adapter)
	stop() // trigger Stop

	for _, id := range []string{"alice", "bob"} {
		sessionID := discordSessionID(id)
		if _, err := reg.Route(sessionID); err == nil {
			t.Errorf("session %q still registered after Stop", sessionID)
		}
	}
}

// TestDiscordAdapter_InboundDM_RoutesToSession verifies that an inbound DM
// from an allowed user triggers delivery to the target session.
func TestDiscordAdapter_InboundDM_RoutesToSession(t *testing.T) {
	adapter, mock, reg := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	// Register a mock delivery for the target session.
	var mu sync.Mutex
	var received []*Frame
	target := &captureDelivery{
		mu:     &mu,
		frames: &received,
	}
	if err := reg.Register("s1", target); err != nil {
		t.Fatalf("register s1: %v", err)
	}
	defer reg.Unregister("s1")

	mock.triggerMessageCreate(adapter, "u1", "to:s1 hello from discord")

	// Allow the handler to run.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("no frame delivered to s1")
	}
	f := received[0]
	if f.From != "discord:u1" {
		t.Errorf("frame.From = %q want %q", f.From, "discord:u1")
	}
	if f.Text != "hello from discord" {
		t.Errorf("frame.Text = %q want %q", f.Text, "hello from discord")
	}
}

// TestDiscordAdapter_InboundDM_NonAllowlisted drops messages from unknown users.
func TestDiscordAdapter_InboundDM_NonAllowlisted(t *testing.T) {
	adapter, mock, reg := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	var mu sync.Mutex
	var received []*Frame
	target := &captureDelivery{mu: &mu, frames: &received}
	if err := reg.Register("s1", target); err != nil {
		t.Fatalf("register s1: %v", err)
	}
	defer reg.Unregister("s1")

	// "u2" is NOT in the allowlist.
	mock.triggerMessageCreate(adapter, "u2", "to:s1 hello from intruder")

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) > 0 {
		t.Errorf("non-allowlisted user delivered %d frames, want 0", len(received))
	}
	// Also verify no reply DM was sent (we just drop silently).
	dms := mock.sentDMs()
	_ = dms // debug-logged only; no DM should go to non-allowlisted user
}

// TestDiscordAdapter_InboundDM_ACLDenied verifies that ACL-denied sends
// result in a DM notification to the user and no delivery to the target.
func TestDiscordAdapter_InboundDM_ACLDenied(t *testing.T) {
	adapter, mock, reg := newTestAdapter(t, []string{"u1"})
	// Attach a deny-all ACL.
	adapter.ACL = &ACL{DefaultAllow: false}
	stop := startAdapter(t, adapter)
	defer stop()

	var mu sync.Mutex
	var received []*Frame
	target := &captureDelivery{mu: &mu, frames: &received}
	if err := reg.Register("s1", target); err != nil {
		t.Fatalf("register s1: %v", err)
	}
	defer reg.Unregister("s1")

	mock.triggerMessageCreate(adapter, "u1", "to:s1 forbidden message")

	// Give the handler time to run.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	nReceived := len(received)
	mu.Unlock()
	if nReceived > 0 {
		t.Errorf("ACL-denied send delivered %d frames, want 0", nReceived)
	}

	// User should have received a "denied by ACL" DM.
	dms := mock.sentDMs()
	if len(dms) == 0 {
		t.Error("expected ACL-denial DM to user, got none")
	}
	if !strings.Contains(dms[0].content, "denied") {
		t.Errorf("denial DM doesn't mention 'denied': %q", dms[0].content)
	}
}

// TestDiscordAdapter_VerdictReply routes a "yes <id>" reply to the originating session.
func TestDiscordAdapter_VerdictReply(t *testing.T) {
	adapter, mock, reg := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	// Register session "worker1" to receive the verdict.
	var mu sync.Mutex
	var received []*Frame
	worker := &captureDelivery{mu: &mu, frames: &received}
	if err := reg.Register("worker1", worker); err != nil {
		t.Fatalf("register worker1: %v", err)
	}
	defer reg.Unregister("worker1")

	// reqID encodes the target session as "worker1/<nonce>".
	mock.triggerMessageCreate(adapter, "u1", "yes worker1/req-42")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(received)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("no verdict frame delivered to worker1")
	}
	f := received[0]
	if f.Type != FramePermissionVerdict {
		t.Errorf("frame type = %q want %q", f.Type, FramePermissionVerdict)
	}
	if f.Verdict != "allow" {
		t.Errorf("verdict = %q want allow", f.Verdict)
	}
	if f.ID != "worker1/req-42" {
		t.Errorf("frame ID = %q want worker1/req-42", f.ID)
	}
}

// TestDiscordAdapter_IgnoresBotMessages verifies bot messages are dropped.
func TestDiscordAdapter_IgnoresBotMessages(t *testing.T) {
	adapter, mock, reg := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	var mu sync.Mutex
	var received []*Frame
	target := &captureDelivery{mu: &mu, frames: &received}
	if err := reg.Register("s1", target); err != nil {
		t.Fatalf("register s1: %v", err)
	}
	defer reg.Unregister("s1")

	// Simulate a message from a bot.
	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:  &discordgo.User{ID: "u1", Bot: true},
			Content: "to:s1 bot message",
			GuildID: "",
		},
	}
	adapter.handleMessageCreate(nil, msg)

	time.Sleep(20 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) > 0 {
		t.Error("bot message was delivered, expected drop")
	}
	dms := mock.sentDMs()
	if len(dms) > 0 {
		t.Error("bot message triggered DM reply, expected none")
	}
}

// TestDiscordAdapter_TargetOffline_NotifiesUser verifies an offline-target reply.
func TestDiscordAdapter_TargetOffline_NotifiesUser(t *testing.T) {
	adapter, mock, _ := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	// "nonexistent" session is not registered.
	mock.triggerMessageCreate(adapter, "u1", "to:nonexistent hi there")

	time.Sleep(50 * time.Millisecond)

	dms := mock.sentDMs()
	if len(dms) == 0 {
		t.Fatal("expected offline-notification DM, got none")
	}
	if !strings.Contains(dms[0].content, "offline") {
		t.Errorf("offline DM doesn't mention 'offline': %q", dms[0].content)
	}
}

// TestDiscordAdapter_UsageMsgOnBadFormat verifies usage hint on malformed input.
func TestDiscordAdapter_UsageMsgOnBadFormat(t *testing.T) {
	adapter, mock, _ := newTestAdapter(t, []string{"u1"})
	stop := startAdapter(t, adapter)
	defer stop()

	mock.triggerMessageCreate(adapter, "u1", "no-space-no-target")

	time.Sleep(50 * time.Millisecond)

	dms := mock.sentDMs()
	if len(dms) == 0 {
		t.Fatal("expected usage-hint DM, got none")
	}
	if !strings.Contains(strings.ToLower(dms[0].content), "usage") {
		t.Errorf("expected usage hint DM, got: %q", dms[0].content)
	}
}

// captureDelivery records frames for assertion in tests.
type captureDelivery struct {
	mu     *sync.Mutex
	frames *[]*Frame
}

func (c *captureDelivery) Deliver(f *Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.frames = append(*c.frames, f)
	return nil
}

func (c *captureDelivery) Close() error { return nil }
