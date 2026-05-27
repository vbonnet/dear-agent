package synchub

// server_cancel_inspect_test.go — coverage for the three REST handlers
// the 2026-05-27 coverage audit flagged as 0%: handleCancel,
// handleInspect, handleInspectLock. Also covers client.Cancel (the only
// client method that drove these handlers and was also 0%) and the
// Server.Token accessor.
//
// All assertions go through the HTTP boundary so the auth + JSON
// decode + error-mapping path is exercised, not just the in-process Hub.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// postJSON is a small helper that POSTs a JSON body with the server's
// bearer token. Tests that go around the Client use this so we can
// observe raw status codes and error envelopes.
func postJSON(t *testing.T, srv *Server, path string, body any) *http.Response {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+srv.Addr()+path, bytes.NewReader(buf))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

// TestServer_Token verifies the accessor returns the same token written
// to TokenFile — the audit flagged Token as a 0% accessor and the
// Client depends on this contract holding.
func TestServer_Token(t *testing.T) {
	_, srv, client := startTestServer(t)
	if srv.Token() == "" {
		t.Fatal("Server.Token() returned empty string")
	}
	if client.Token != srv.Token() {
		t.Fatalf("client token %q != server token %q (token file should write them paired)", client.Token, srv.Token())
	}
}

// TestServer_HandleCancel_OK exercises the happy path: open a round on
// the server, POST /v1/qa/cancel via the typed client, then verify the
// round was actually closed (a follow-up Answer returns ErrClosed).
func TestServer_HandleCancel_OK(t *testing.T) {
	_, _, client := startTestServer(t)
	ctx := context.Background()

	qid, err := client.AskQuestion(ctx, "stop?")
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}
	if err := client.Cancel(ctx, qid); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// After cancel, an Answer must report the round is closed.
	if _, err := client.Answer(ctx, qid, SurfaceTerminal, []byte("late")); !errors.Is(err, ErrClosed) {
		t.Fatalf("post-cancel Answer err = %v, want ErrClosed", err)
	}
}

// TestServer_HandleCancel_NotFound maps an unknown question ID to a 404
// + ErrNotFound on the client side.
func TestServer_HandleCancel_NotFound(t *testing.T) {
	_, _, client := startTestServer(t)
	err := client.Cancel(context.Background(), QuestionID("q-does-not-exist"))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Cancel on bogus id err = %v, want ErrNotFound", err)
	}
}

// TestServer_HandleCancel_IsIdempotent — Hub.Cancel returns nil on
// already-canceled rounds, and the handler must surface that as a 200,
// not a 409. Otherwise a Discord bot's "cancel" button is rate-limited
// by its own past clicks.
func TestServer_HandleCancel_IsIdempotent(t *testing.T) {
	_, _, client := startTestServer(t)
	ctx := context.Background()
	qid, _ := client.AskQuestion(ctx, "?")
	if err := client.Cancel(ctx, qid); err != nil {
		t.Fatalf("first Cancel: %v", err)
	}
	if err := client.Cancel(ctx, qid); err != nil {
		t.Fatalf("second Cancel must be a no-op, got %v", err)
	}
}

// TestServer_HandleInspect_ReturnsSnapshot verifies the inspect handler
// returns a RoundSnapshot JSON that reflects the round's actual state
// (open after Ask, answered after Answer).
func TestServer_HandleInspect_ReturnsSnapshot(t *testing.T) {
	_, srv, client := startTestServer(t)
	ctx := context.Background()
	qid, err := client.AskQuestion(ctx, "color?")
	if err != nil {
		t.Fatalf("AskQuestion: %v", err)
	}

	resp := postJSON(t, srv, "/v1/qa/inspect", inspectReq{ID: qid})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("inspect status = %d, body=%s", resp.StatusCode, body)
	}
	var snap RoundSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.ID != qid {
		t.Errorf("snapshot ID = %q, want %q", snap.ID, qid)
	}
	if snap.Prompt != "color?" {
		t.Errorf("snapshot Prompt = %q, want %q", snap.Prompt, "color?")
	}
	if snap.State != "open" {
		t.Errorf("snapshot State = %q, want open", snap.State)
	}

	// After Answer, the snapshot must report the winner.
	if _, err := client.Answer(ctx, qid, SurfaceDiscord, []byte("blue")); err != nil {
		t.Fatalf("Answer: %v", err)
	}
	resp2 := postJSON(t, srv, "/v1/qa/inspect", inspectReq{ID: qid})
	defer resp2.Body.Close()
	var snap2 RoundSnapshot
	if err := json.NewDecoder(resp2.Body).Decode(&snap2); err != nil {
		t.Fatalf("decode 2: %v", err)
	}
	if snap2.State != "answered" {
		t.Errorf("post-answer State = %q, want answered", snap2.State)
	}
	if snap2.Winner != SurfaceDiscord {
		t.Errorf("Winner = %q, want %q", snap2.Winner, SurfaceDiscord)
	}
}

// TestServer_HandleInspect_NotFound — bogus IDs return a 404 + the
// not_found error code envelope (so surfaces can switch on `code`
// rather than parsing the message string).
func TestServer_HandleInspect_NotFound(t *testing.T) {
	_, srv, _ := startTestServer(t)
	resp := postJSON(t, srv, "/v1/qa/inspect", inspectReq{ID: QuestionID("q-nope")})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var env struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode err envelope: %v", err)
	}
	if env.Code != "not_found" {
		t.Errorf("error code = %q, want not_found", env.Code)
	}
}

// TestServer_HandleInspectLock_OK acquires a lock then asks the
// server-side inspect for it; the snapshot must report the right
// holder + fence + a non-zero deadline.
func TestServer_HandleInspectLock_OK(t *testing.T) {
	_, srv, client := startTestServer(t)
	ctx := context.Background()
	hdl, err := client.Acquire(ctx, "input", "discord-bot", AcquireOptions{Deadline: 5 * time.Second})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	t.Cleanup(func() { _ = hdl.Release(ctx) })

	resp := postJSON(t, srv, "/v1/lock/inspect", inspectLockReq{Key: "input"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("inspect-lock status = %d, body=%s", resp.StatusCode, body)
	}
	var snap LockSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Key != "input" || snap.Holder != "discord-bot" {
		t.Errorf("snapshot = %+v, want key=input holder=discord-bot", snap)
	}
	if snap.Fence != hdl.Fence() {
		t.Errorf("snapshot Fence = %d, want %d (client + server fence must match)", snap.Fence, hdl.Fence())
	}
	if snap.Deadline.IsZero() {
		t.Error("snapshot Deadline is zero; want acquired+deadline")
	}
}

// TestServer_HandleInspectLock_NotHeld returns a 404 with "not held"
// in the body — the handler uses http.Error here, not the JSON
// envelope, because the lock is unambiguously not held.
func TestServer_HandleInspectLock_NotHeld(t *testing.T) {
	_, srv, _ := startTestServer(t)
	resp := postJSON(t, srv, "/v1/lock/inspect", inspectLockReq{Key: "not-acquired"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "not held") {
		t.Errorf("body = %q, want substring 'not held'", body)
	}
}

// TestServer_HandleInspectLock_ReleaseClearsIt asserts that once the
// client releases, /v1/lock/inspect transitions back to 404. This is
// the round-trip the Discord bot does to detect a stale "X is typing"
// indicator.
func TestServer_HandleInspectLock_ReleaseClearsIt(t *testing.T) {
	_, srv, client := startTestServer(t)
	ctx := context.Background()
	hdl, err := client.Acquire(ctx, "k", "h", AcquireOptions{Deadline: time.Second})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	resp := postJSON(t, srv, "/v1/lock/inspect", inspectLockReq{Key: "k"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pre-release status = %d, want 200", resp.StatusCode)
	}

	if err := hdl.Release(ctx); err != nil {
		t.Fatalf("Release: %v", err)
	}

	resp2 := postJSON(t, srv, "/v1/lock/inspect", inspectLockReq{Key: "k"})
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("post-release status = %d, want 404 (lock should no longer be held)", resp2.StatusCode)
	}
}

// TestServer_HandleCancel_RejectsGET ensures the readJSON guard rejects
// the wrong method with 405. The Discord bot's HTTP client uses POST,
// but a misbehaving curl that issues GET must not silently 401-loop.
func TestServer_HandleCancel_RejectsGET(t *testing.T) {
	_, srv, _ := startTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, "http://"+srv.Addr()+"/v1/qa/cancel", nil)
	req.Header.Set("Authorization", "Bearer "+srv.Token())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
}
