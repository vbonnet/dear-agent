package bus

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// mockMatrixClient is a narrow in-memory matrixClient for tests. It
// holds a queue of sync responses the test pre-populates, records all
// SendRoomMessage calls, and can fail-inject per-method errors.
type mockMatrixClient struct {
	mu            sync.Mutex
	joinedRooms   []string
	syncResponses []*matrixSyncResponse
	syncErr       error
	sendErr       error
	sent          []mockMatrixSend
	// syncSignal lets tests drain sync responses in order and block the
	// adapter's next Sync call until the test calls nextSync.
	syncSignal chan struct{}
}

type mockMatrixSend struct {
	RoomID string
	TxnID  string
	Body   string
}

func (m *mockMatrixClient) Sync(ctx context.Context, _ string, _ time.Duration) (*matrixSyncResponse, error) {
	m.mu.Lock()
	if m.syncErr != nil {
		err := m.syncErr
		m.mu.Unlock()
		return nil, err
	}
	if len(m.syncResponses) == 0 {
		m.mu.Unlock()
		// Block until either a response is queued OR ctx is cancelled.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-m.syncSignal:
		}
		m.mu.Lock()
	}
	if len(m.syncResponses) == 0 {
		m.mu.Unlock()
		return &matrixSyncResponse{NextBatch: "empty"}, nil
	}
	resp := m.syncResponses[0]
	m.syncResponses = m.syncResponses[1:]
	m.mu.Unlock()
	return resp, nil
}

func (m *mockMatrixClient) SendRoomMessage(_ context.Context, roomID, txnID, body string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return "", m.sendErr
	}
	m.sent = append(m.sent, mockMatrixSend{RoomID: roomID, TxnID: txnID, Body: body})
	return "$event:" + txnID, nil
}

func (m *mockMatrixClient) JoinedRooms(_ context.Context) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.joinedRooms...), nil
}

func (m *mockMatrixClient) enqueueSync(resp *matrixSyncResponse) {
	m.mu.Lock()
	m.syncResponses = append(m.syncResponses, resp)
	m.mu.Unlock()
	// Non-blocking nudge in case the adapter is waiting on syncSignal.
	select {
	case m.syncSignal <- struct{}{}:
	default:
	}
}

func (m *mockMatrixClient) sentFrames() []mockMatrixSend {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mockMatrixSend(nil), m.sent...)
}

// newTestMatrixAdapter wires a MatrixAdapter to a fresh broker registry +
// the mock client. Returns the adapter, the mock, and a cleanup func.
func newTestMatrixAdapter(t *testing.T, roomID string, allowlist []string) (*MatrixAdapter, *mockMatrixClient, context.CancelFunc) {
	t.Helper()
	reg := NewRegistry()
	mock := &mockMatrixClient{
		joinedRooms: []string{roomID},
		syncSignal:  make(chan struct{}, 16),
	}
	// Seed an initial empty sync so Start's "initial sync" call
	// returns a cursor and the loop enters its main phase.
	mock.syncResponses = []*matrixSyncResponse{{NextBatch: "cursor-0"}}

	a := &MatrixAdapter{
		HomeserverURL: "http://test",
		AccessToken:   "fake",
		UserID:        "@bot:test",
		RoomID:        roomID,
		Allowlist:     allowlist,
		Registry:      reg,
		SyncTimeout:   50 * time.Millisecond,
		client:        mock,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = a.Start(ctx)
		close(done)
	}()

	// Wait briefly for registration to happen (initial sync + session
	// Register calls).
	time.Sleep(50 * time.Millisecond)

	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Log("matrix adapter did not exit within 2s of cancel")
		}
	})
	return a, mock, cancel
}

func TestMatrixAdapterRegistersAllowedUsers(t *testing.T) {
	a, _, _ := newTestMatrixAdapter(t, "!room:test", []string{"@alice:test", "@bob:test"})
	for _, mxid := range []string{"@alice:test", "@bob:test"} {
		if _, err := a.Registry.Route(matrixSessionID(mxid)); err != nil {
			t.Errorf("expected %s registered: %v", mxid, err)
		}
	}
	if _, err := a.Registry.Route(matrixSessionID("@eve:test")); err == nil {
		t.Error("non-allowlisted user should not be registered")
	}
}

func TestMatrixAdapterRefusesIfBotNotInRoom(t *testing.T) {
	reg := NewRegistry()
	mock := &mockMatrixClient{joinedRooms: []string{"!different:test"}}
	a := &MatrixAdapter{
		HomeserverURL: "http://test",
		AccessToken:   "fake",
		RoomID:        "!room:test",
		Registry:      reg,
		client:        mock,
	}
	err := a.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when bot is not in room")
	}
	if !strings.Contains(err.Error(), "not in room") {
		t.Errorf("error = %v, want mention of 'not in room'", err)
	}
}

func TestMatrixAdapterRoutesOutboundMessage(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", []string{"@alice:test"})

	// Register a target session so the routed FrameSend has somewhere to go.
	target := &recordingDelivery{}
	_ = a.Registry.Register("s1", target)

	// Alice types "to:s1 hello" in the room.
	mock.enqueueSync(&matrixSyncResponse{
		NextBatch: "cursor-1",
		Rooms: matrixSyncRooms{Join: map[string]matrixSyncJoinRoom{
			"!room:test": {Timeline: matrixSyncTimeline{Events: []matrixEvent{
				{
					EventID: "$1",
					Type:    "m.room.message",
					Sender:  "@alice:test",
					Content: matrixMessageContent{MsgType: "m.text", Body: "to:s1 hello"},
				},
			}}},
		}},
	})

	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && len(target.Frames()) == 0 {
		time.Sleep(25 * time.Millisecond)
	}
	frames := target.Frames()
	if len(frames) == 0 {
		t.Fatal("s1 never received a frame")
	}
	if frames[0].Text != "hello" {
		t.Errorf("frame.Text = %q, want hello", frames[0].Text)
	}
	if frames[0].From != matrixSessionID("@alice:test") {
		t.Errorf("frame.From = %q, want matrix:@alice:test", frames[0].From)
	}
	// No error posted back to the room.
	for _, s := range mock.sentFrames() {
		if strings.Contains(s.Body, "denied") || strings.Contains(s.Body, "offline") {
			t.Errorf("unexpected error reply: %q", s.Body)
		}
	}
}

func TestMatrixAdapterIgnoresNonAllowlistedSender(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", []string{"@alice:test"})

	target := &recordingDelivery{}
	_ = a.Registry.Register("s1", target)

	mock.enqueueSync(&matrixSyncResponse{
		NextBatch: "cursor-2",
		Rooms: matrixSyncRooms{Join: map[string]matrixSyncJoinRoom{
			"!room:test": {Timeline: matrixSyncTimeline{Events: []matrixEvent{
				{
					EventID: "$2",
					Type:    "m.room.message",
					Sender:  "@eve:test",
					Content: matrixMessageContent{MsgType: "m.text", Body: "to:s1 evil"},
				},
			}}},
		}},
	})
	time.Sleep(200 * time.Millisecond)
	if len(target.Frames()) > 0 {
		t.Error("non-allowlisted sender should not produce a routed frame")
	}
}

func TestMatrixAdapterSkipsSelfMessages(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", []string{"@bot:test"})
	// Bot is the configured UserID; messages from that sender must be
	// filtered so we don't echo ourselves.
	target := &recordingDelivery{}
	_ = a.Registry.Register("s1", target)

	mock.enqueueSync(&matrixSyncResponse{
		NextBatch: "cursor-3",
		Rooms: matrixSyncRooms{Join: map[string]matrixSyncJoinRoom{
			"!room:test": {Timeline: matrixSyncTimeline{Events: []matrixEvent{
				{
					EventID: "$3",
					Type:    "m.room.message",
					Sender:  "@bot:test",
					Content: matrixMessageContent{MsgType: "m.text", Body: "to:s1 echo"},
				},
			}}},
		}},
	})
	time.Sleep(200 * time.Millisecond)
	if len(target.Frames()) > 0 {
		t.Error("bot's own messages must be filtered to avoid echo loops")
	}
}

func TestMatrixAdapterACLDeniesAreSurfaced(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", []string{"@alice:test"})

	target := &recordingDelivery{}
	_ = a.Registry.Register("s1", target)

	// Deny-all ACL.
	a.ACL = &ACL{DefaultAllow: false}

	mock.enqueueSync(&matrixSyncResponse{
		NextBatch: "cursor-4",
		Rooms: matrixSyncRooms{Join: map[string]matrixSyncJoinRoom{
			"!room:test": {Timeline: matrixSyncTimeline{Events: []matrixEvent{
				{
					EventID: "$4",
					Type:    "m.room.message",
					Sender:  "@alice:test",
					Content: matrixMessageContent{MsgType: "m.text", Body: "to:s1 blocked"},
				},
			}}},
		}},
	})
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		for _, s := range mock.sentFrames() {
			if strings.Contains(s.Body, "denied by ACL") {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Error("expected ACL deny message in room output")
}

func TestMatrixAdapterPermissionVerdictReply(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", []string{"@alice:test"})

	worker := &recordingDelivery{}
	_ = a.Registry.Register("w1", worker)

	// Alice types "yes w1/abc".
	mock.enqueueSync(&matrixSyncResponse{
		NextBatch: "cursor-5",
		Rooms: matrixSyncRooms{Join: map[string]matrixSyncJoinRoom{
			"!room:test": {Timeline: matrixSyncTimeline{Events: []matrixEvent{
				{
					EventID: "$5",
					Type:    "m.room.message",
					Sender:  "@alice:test",
					Content: matrixMessageContent{MsgType: "m.text", Body: "yes w1/abc"},
				},
			}}},
		}},
	})
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && len(worker.Frames()) == 0 {
		time.Sleep(25 * time.Millisecond)
	}
	if len(worker.Frames()) == 0 {
		t.Fatal("w1 did not receive the verdict")
	}
	f := worker.Frames()[0]
	if f.Type != FramePermissionVerdict || f.Verdict != "allow" || f.ID != "w1/abc" {
		t.Errorf("unexpected verdict frame: %+v", f)
	}
}

func TestMatrixDeliveryFormatsMessages(t *testing.T) {
	reg := NewRegistry()
	mock := &mockMatrixClient{}
	a := &MatrixAdapter{
		HomeserverURL: "http://test",
		RoomID:        "!room:test",
		Registry:      reg,
		Logger:        nil, // slog.Default()
		client:        mock,
	}
	d := &matrixDelivery{mxid: "@alice:test", adapter: a}

	// Deliver a regular message.
	if err := d.Deliver(&Frame{Type: FrameDeliver, From: "s1", Text: "hi"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	// Deliver a permission request.
	if err := d.Deliver(&Frame{
		Type:         FramePermissionRequest,
		ID:           "w1/abc",
		From:         "s1",
		ToolName:     "Bash",
		Description:  "list dir",
		InputPreview: `{"cmd":"ls"}`,
	}); err != nil {
		t.Fatalf("Deliver permission: %v", err)
	}

	sent := mock.sentFrames()
	if len(sent) != 2 {
		t.Fatalf("got %d sends, want 2", len(sent))
	}
	if !strings.Contains(sent[0].Body, "@alice:test") || !strings.Contains(sent[0].Body, "[s1]") {
		t.Errorf("regular Deliver body missing expected parts: %q", sent[0].Body)
	}
	if !strings.Contains(sent[1].Body, "Permission request") ||
		!strings.Contains(sent[1].Body, "w1/abc") ||
		!strings.Contains(sent[1].Body, "yes w1/abc") {
		t.Errorf("permission request body missing expected parts: %q", sent[1].Body)
	}
}

func TestMatrixAdapterRequiresConfig(t *testing.T) {
	// Use pointers to avoid copylocks — MatrixAdapter embeds sync.Mutex.
	cases := []struct {
		name string
		a    *MatrixAdapter
	}{
		{"no homeserver", &MatrixAdapter{RoomID: "!r", AccessToken: "t"}},
		{"no token", &MatrixAdapter{HomeserverURL: "http://h", RoomID: "!r"}},
		{"no room", &MatrixAdapter{HomeserverURL: "http://h", AccessToken: "t"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.a.Registry = NewRegistry()
			err := tc.a.Start(context.Background())
			if err == nil {
				t.Error("expected config-missing error")
			}
		})
	}
}

func TestMatrixAdapterFailedSendLogsNotAborts(t *testing.T) {
	a, mock, _ := newTestMatrixAdapter(t, "!room:test", nil)
	mock.mu.Lock()
	mock.sendErr = errors.New("network down")
	mock.mu.Unlock()
	// sendRoom should return the error but not panic.
	if err := a.sendRoom("test"); err == nil {
		t.Error("expected send error to propagate")
	}
}
