// Package bus — Matrix adapter for the agm-bus broker.
//
// MatrixAdapter bridges a single Matrix room into the broker so human users
// on Matrix (or, via bridges like mautrix-googlechat, users on Google Chat)
// can send/receive messages to AGM sessions. Mirrors the DiscordAdapter
// design: each allowlisted Matrix user maps to a `matrix:<mxid>` pseudo-
// session in the Registry; messages in the shared room that start with a
// target session id are routed as FrameSend; outbound frames destined for a
// matrix:<mxid> session are posted back to the room with a "[from <sender>]"
// prefix.
//
// Avoids a heavy external SDK dependency by talking directly to the Matrix
// client-server API over HTTP. Only three endpoints are needed:
//   - GET  /_matrix/client/v3/sync                    — long-poll for new events
//   - PUT  /_matrix/client/v3/rooms/{roomID}/send/... — post a message
//   - GET  /_matrix/client/v3/joined_rooms            — sanity check on startup
//
// For bridged networks (Google Chat via mautrix-googlechat, Slack via
// mautrix-slack, etc.) nothing changes at this layer — the adapter sees
// those as regular Matrix users in the same room.
package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// matrixClient is the narrow interface the adapter uses. Abstracting this
// lets tests substitute a mock without a live homeserver. A single instance
// is reused across sync+send calls.
type matrixClient interface {
	// Sync long-polls for new events. since is the previous next_batch
	// cursor ("" on first call). timeout is the server-side long-poll
	// duration. Returns the parsed sync response plus the next cursor.
	Sync(ctx context.Context, since string, timeout time.Duration) (*matrixSyncResponse, error)

	// SendRoomMessage posts a plain-text m.room.message to roomID. txnID
	// is the client-generated idempotency token (Matrix requires it for
	// PUT /send). Returns the event id.
	SendRoomMessage(ctx context.Context, roomID, txnID, body string) (string, error)

	// JoinedRooms returns the set of room ids the authenticated user is
	// in. Called on Start as a sanity check.
	JoinedRooms(ctx context.Context) ([]string, error)
}

// matrixSyncResponse is the trimmed shape the adapter needs from /sync.
// Full response is larger; we only decode timeline events per joined room.
type matrixSyncResponse struct {
	NextBatch string                     `json:"next_batch"`
	Rooms     matrixSyncRooms            `json:"rooms"`
}

type matrixSyncRooms struct {
	Join map[string]matrixSyncJoinRoom `json:"join"`
}

type matrixSyncJoinRoom struct {
	Timeline matrixSyncTimeline `json:"timeline"`
}

type matrixSyncTimeline struct {
	Events []matrixEvent `json:"events"`
}

type matrixEvent struct {
	EventID string               `json:"event_id"`
	Type    string               `json:"type"`
	Sender  string               `json:"sender"`
	Content matrixMessageContent `json:"content"`
	Origin  int64                `json:"origin_server_ts"`
}

type matrixMessageContent struct {
	MsgType string `json:"msgtype"`
	Body    string `json:"body"`
}

// httpMatrixClient is the production matrixClient backed by net/http. All
// requests send Authorization: Bearer <AccessToken>. A custom HTTP client
// can be injected for tests or proxying.
type httpMatrixClient struct {
	HomeserverURL string
	AccessToken   string
	HTTP          *http.Client
}

func (c *httpMatrixClient) req(ctx context.Context, method, path string, body any) (*http.Response, error) {
	u := strings.TrimRight(c.HomeserverURL, "/") + path
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("matrix: encode body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, fmt.Errorf("matrix: new request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("matrix: %s %s: %w", method, path, err)
	}
	if resp.StatusCode >= 400 {
		defer func() { _ = resp.Body.Close() }()
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("matrix: %s %s: status %d: %s", method, path, resp.StatusCode, string(buf))
	}
	return resp, nil
}

func (c *httpMatrixClient) Sync(ctx context.Context, since string, timeout time.Duration) (*matrixSyncResponse, error) {
	q := url.Values{}
	q.Set("timeout", fmt.Sprintf("%d", timeout.Milliseconds()))
	if since != "" {
		q.Set("since", since)
	}
	path := "/_matrix/client/v3/sync?" + q.Encode()
	resp, err := c.req(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out matrixSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("matrix: decode sync: %w", err)
	}
	return &out, nil
}

func (c *httpMatrixClient) SendRoomMessage(ctx context.Context, roomID, txnID, body string) (string, error) {
	path := fmt.Sprintf("/_matrix/client/v3/rooms/%s/send/m.room.message/%s",
		url.PathEscape(roomID), url.PathEscape(txnID))
	msg := matrixMessageContent{MsgType: "m.text", Body: body}
	resp, err := c.req(ctx, http.MethodPut, path, msg)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		EventID string `json:"event_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("matrix: decode send: %w", err)
	}
	return out.EventID, nil
}

func (c *httpMatrixClient) JoinedRooms(ctx context.Context) ([]string, error) {
	resp, err := c.req(ctx, http.MethodGet, "/_matrix/client/v3/joined_rooms", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		JoinedRooms []string `json:"joined_rooms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("matrix: decode joined_rooms: %w", err)
	}
	return out.JoinedRooms, nil
}

// matrixDelivery implements Delivery for a single Matrix user. Deliver
// posts a formatted message to the shared room; the user with
// delivery.mxid is the semantic recipient, but Matrix room semantics
// don't support per-user addressing — anyone in the room sees the
// message. Real privacy should come from using a DM room per user; this
// adapter intentionally targets the shared-room model for simplicity.
type matrixDelivery struct {
	mxid    string
	adapter *MatrixAdapter
}

// Deliver sends a frame to the Matrix room. Mirrors DiscordDelivery's
// formatting: permission requests include instructions for yes/no reply.
func (d *matrixDelivery) Deliver(f *Frame) error {
	var text string
	switch f.Type { //nolint:exhaustive // default covers non-special frame types
	case FramePermissionRequest:
		text = fmt.Sprintf(
			"@%s: [Permission request from %s]\nTool: %s\n%s\nInput: %s\n\nReply with: yes %s  or  no %s",
			d.mxid, f.From, f.ToolName, f.Description, f.InputPreview, f.ID, f.ID,
		)
	case FrameDeliver:
		text = fmt.Sprintf("@%s: [%s] %s", d.mxid, f.From, f.Text)
	default:
		text = fmt.Sprintf("@%s: [%s from %s] %s", d.mxid, f.Type, f.From, f.Text)
	}
	return d.adapter.sendRoom(text)
}

// Close is a no-op for Matrix pseudo-sessions; the sync loop owns the
// connection and is stopped by the adapter, not per-user.
func (d *matrixDelivery) Close() error { return nil }

// MatrixAdapter bridges the agm-bus broker and a single Matrix room.
//
// Intentionally simpler than a full Matrix bot: one configured room is
// the meeting point between the broker and any allowlisted Matrix users.
// For private conversations users can create dedicated rooms and run
// separate adapter instances against those room ids.
type MatrixAdapter struct {
	// HomeserverURL is the base URL of the Matrix homeserver (e.g.
	// "https://matrix.org"). No trailing slash required.
	HomeserverURL string

	// AccessToken is the bot user's long-lived access token (obtained via
	// /_matrix/client/v3/login or the homeserver admin UI).
	AccessToken string

	// UserID is the bot's Matrix user id (e.g. "@agmbus:example.org").
	// Messages sent by this id are filtered out so the adapter doesn't
	// echo itself.
	UserID string

	// RoomID is the Matrix room id (NOT alias) the adapter listens to
	// and posts to. Get it from a Matrix client's "copy room id" menu.
	RoomID string

	// Allowlist is the set of Matrix user ids (mxids) permitted to send
	// messages through the adapter. Empty denies all inbound.
	Allowlist []string

	// Registry is the broker's active-connection table.
	Registry *Registry

	// ACL enforces send permissions. May be nil (allow-all).
	ACL interface {
		Check(sender, target string) ACLDecision
	}

	// Logger for adapter-level events.
	Logger *slog.Logger

	// SyncTimeout is how long the sync long-poll may block on the server.
	// Default 30s. Shorter values = more round trips; longer = fewer
	// but slower shutdown on ctx cancellation (we wait for the current
	// sync to return before exiting).
	SyncTimeout time.Duration

	// client is the Matrix client; set by Start (production uses
	// httpMatrixClient; tests inject a mock).
	client matrixClient

	mu       sync.Mutex
	allowed  map[string]bool
	txnCount atomic.Uint64
}

// Start registers allowlisted users' pseudo-sessions and spins up the sync
// loop. Returns when ctx is cancelled or the sync loop exits with an
// unrecoverable error.
//
//nolint:gocyclo // linear startup (config validate + probe + register + sync loop); splitting obscures order
func (a *MatrixAdapter) Start(ctx context.Context) error {
	if a.Logger == nil {
		a.Logger = slog.Default()
	}
	if a.SyncTimeout <= 0 {
		a.SyncTimeout = 30 * time.Second
	}
	if a.HomeserverURL == "" || a.AccessToken == "" || a.RoomID == "" {
		return errors.New("matrix: HomeserverURL, AccessToken, and RoomID are required")
	}

	a.mu.Lock()
	a.allowed = make(map[string]bool, len(a.Allowlist))
	for _, id := range a.Allowlist {
		a.allowed[id] = true
	}
	a.mu.Unlock()

	if a.client == nil {
		a.client = &httpMatrixClient{
			HomeserverURL: a.HomeserverURL,
			AccessToken:   a.AccessToken,
		}
	}

	// Sanity: confirm the bot is in the configured room. A fresh bot
	// often isn't, in which case the caller must invite/join out-of-band.
	rooms, err := a.client.JoinedRooms(ctx)
	if err != nil {
		return fmt.Errorf("matrix: joined_rooms probe: %w", err)
	}
	if !contains(rooms, a.RoomID) {
		return fmt.Errorf("matrix: bot not in room %q; invite %s and retry", a.RoomID, a.UserID)
	}

	// Register each allowed user's pseudo-session.
	for _, mxid := range a.Allowlist {
		sessionID := matrixSessionID(mxid)
		d := &matrixDelivery{mxid: mxid, adapter: a}
		if err := a.Registry.Register(sessionID, d); err != nil {
			a.Logger.Warn("matrix: session already registered", "session", sessionID, "err", err)
		}
	}

	a.Logger.Info("matrix adapter started",
		"homeserver", a.HomeserverURL,
		"room", a.RoomID,
		"users", len(a.Allowlist))

	// Initial sync to establish a cursor; skip its events (pre-startup).
	initial, err := a.client.Sync(ctx, "", 0)
	if err != nil {
		a.unregisterAll()
		return fmt.Errorf("matrix: initial sync: %w", err)
	}
	since := initial.NextBatch

	// Sync loop.
	for {
		select {
		case <-ctx.Done():
			a.unregisterAll()
			return nil
		default:
		}
		syncResp, err := a.client.Sync(ctx, since, a.SyncTimeout)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				a.unregisterAll()
				return nil
			}
			a.Logger.Warn("matrix: sync error; retrying", "err", err)
			select {
			case <-ctx.Done():
				a.unregisterAll()
				return nil
			case <-time.After(2 * time.Second):
			}
			continue
		}
		since = syncResp.NextBatch
		a.processSync(syncResp)
	}
}

// Stop unregisters pseudo-sessions. Sync loop exits via ctx cancellation.
func (a *MatrixAdapter) Stop() error {
	a.unregisterAll()
	a.Logger.Info("matrix adapter stopped")
	return nil
}

// processSync handles the timeline events from a sync response.
func (a *MatrixAdapter) processSync(resp *matrixSyncResponse) {
	room, ok := resp.Rooms.Join[a.RoomID]
	if !ok {
		return
	}
	for _, ev := range room.Timeline.Events {
		// Only m.room.message with msgtype m.text; ignore joins, state, etc.
		if ev.Type != "m.room.message" || ev.Content.MsgType != "m.text" {
			continue
		}
		// Ignore our own messages so we don't loop.
		if ev.Sender == a.UserID {
			continue
		}
		a.handleMessage(ev)
	}
}

// handleMessage is the core routing logic for an inbound Matrix message.
// Mirrors the Discord adapter's handleMessageCreate: allowlist check,
// verdict-reply detection, then ACL + target routing.
func (a *MatrixAdapter) handleMessage(ev matrixEvent) {
	a.mu.Lock()
	isAllowed := a.allowed[ev.Sender]
	a.mu.Unlock()
	if !isAllowed {
		a.Logger.Debug("matrix: ignoring msg from non-allowlisted user", "sender", ev.Sender)
		return
	}

	content := strings.TrimSpace(ev.Content.Body)
	if content == "" {
		return
	}
	senderID := matrixSessionID(ev.Sender)

	if verdict, reqID, ok := parseVerdictReply(content); ok {
		a.handleVerdictReply(senderID, verdict, reqID)
		return
	}

	targetID, text, ok := parseOutboundMessage(content)
	if !ok {
		_ = a.sendRoom(fmt.Sprintf("@%s: usage — to:<session-id> <message>", ev.Sender))
		return
	}

	if a.ACL != nil {
		d := a.ACL.Check(senderID, targetID)
		if !d.Allowed {
			_ = a.sendRoom(fmt.Sprintf("@%s: send denied by ACL: %s", ev.Sender, d.Reason))
			return
		}
	}

	frame := &Frame{
		Type: FrameDeliver,
		ID:   fmt.Sprintf("matrix-%d", time.Now().UnixNano()),
		From: senderID,
		To:   targetID,
		Text: text,
		TS:   time.Now().UTC(),
	}
	delivery, err := a.Registry.Route(targetID)
	if err != nil {
		_ = a.sendRoom(fmt.Sprintf("@%s: session %q is offline", ev.Sender, targetID))
		return
	}
	if err := delivery.Deliver(frame); err != nil {
		_ = a.sendRoom(fmt.Sprintf("@%s: delivery error: %v", ev.Sender, err))
	}
}

// handleVerdictReply parses "yes <reqID>" / "no <reqID>" and routes a
// FramePermissionVerdict to the target session encoded in the reqID (same
// "<session>/<nonce>" format the Discord adapter expects).
func (a *MatrixAdapter) handleVerdictReply(senderID, verdict, reqID string) {
	targetSession, _, ok := strings.Cut(reqID, "/")
	if !ok {
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
		_ = a.sendRoom(fmt.Sprintf("session %q is offline; verdict dropped", targetSession))
		return
	}
	if err := delivery.Deliver(frame); err != nil {
		a.Logger.Warn("matrix: verdict delivery failed", "target", targetSession, "err", err)
	}
}

// sendRoom posts a message to the configured room, generating a fresh
// client-side txnID per call so Matrix's idempotency semantics don't
// collapse distinct messages.
func (a *MatrixAdapter) sendRoom(text string) error {
	txn := fmt.Sprintf("agmbus-%d-%d", time.Now().UnixNano(), a.txnCount.Add(1))
	// Use a short per-call context derived from Background so the send
	// doesn't block on the sync long-poll's ctx, which may itself be
	// close to deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := a.client.SendRoomMessage(ctx, a.RoomID, txn, text)
	if err != nil {
		a.Logger.Warn("matrix: send failed", "err", err)
	}
	return err
}

func (a *MatrixAdapter) unregisterAll() {
	for _, mxid := range a.Allowlist {
		a.Registry.Unregister(matrixSessionID(mxid))
	}
}

// matrixSessionID returns the pseudo-session id for a Matrix user.
func matrixSessionID(mxid string) string {
	return "matrix:" + mxid
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
