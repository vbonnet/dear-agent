package restbackend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/backend"
)

// TestInterfaceCompliance verifies RestBackend implements backend.Backend.
func TestInterfaceCompliance(t *testing.T) {
	var _ backend.Backend = (*RestBackend)(nil)
}

func TestNewRestBackend(t *testing.T) {
	b := New("")
	if b.claudePath != "claude" {
		t.Errorf("expected default claude path, got %q", b.claudePath)
	}

	b2 := New("/usr/local/bin/claude")
	if b2.claudePath != "/usr/local/bin/claude" {
		t.Errorf("expected custom path, got %q", b2.claudePath)
	}
}

func TestHasSession_Empty(t *testing.T) {
	b := New("")
	has, err := b.HasSession("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("expected false for nonexistent session")
	}
}

func TestListSessions_Empty(t *testing.T) {
	b := New("")
	sessions, err := b.ListSessions()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestListSessionsWithInfo_Empty(t *testing.T) {
	b := New("")
	infos, err := b.ListSessionsWithInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(infos))
	}
}

func TestListClients_NotFound(t *testing.T) {
	b := New("")
	_, err := b.ListClients("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestAttachSession_Unsupported(t *testing.T) {
	b := New("")
	err := b.AttachSession("any")
	if err == nil {
		t.Error("expected error for attach on process backend")
	}
}

func TestSendKeys_NotFound(t *testing.T) {
	b := New("")
	err := b.SendKeys("nonexistent", "hello")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestReadOutput_NotFound(t *testing.T) {
	b := New("")
	_, err := b.ReadOutput("nonexistent", 10)
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestGetProcessState_NotFound(t *testing.T) {
	b := New("")
	_, err := b.GetProcessState("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestTerminateSession_NotFound(t *testing.T) {
	b := New("")
	err := b.TerminateSession("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestCreateSession_InvalidBinary(t *testing.T) {
	b := New("/nonexistent/binary/path")
	err := b.CreateSession("test-session", "/tmp")
	if err == nil {
		t.Error("expected error when claude binary doesn't exist")
	}
}

// --- Ring Buffer Tests ---

func TestRingBuffer_Basic(t *testing.T) {
	rb := newRingBuffer(3)

	rb.Write("a")
	rb.Write("b")
	rb.Write("c")

	all := rb.ReadAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	if all[0] != "a" || all[1] != "b" || all[2] != "c" {
		t.Errorf("unexpected items: %v", all)
	}
}

func TestRingBuffer_Overflow(t *testing.T) {
	rb := newRingBuffer(3)

	rb.Write("a")
	rb.Write("b")
	rb.Write("c")
	rb.Write("d") // overwrites "a"

	all := rb.ReadAll()
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}
	if all[0] != "b" || all[1] != "c" || all[2] != "d" {
		t.Errorf("expected [b c d], got %v", all)
	}
}

func TestRingBuffer_ReadLast(t *testing.T) {
	rb := newRingBuffer(5)
	for i := 0; i < 5; i++ {
		rb.Write(string(rune('a' + i)))
	}

	last2 := rb.ReadLast(2)
	if len(last2) != 2 {
		t.Fatalf("expected 2 items, got %d", len(last2))
	}
	if last2[0] != "d" || last2[1] != "e" {
		t.Errorf("expected [d e], got %v", last2)
	}

	// Ask for more than available
	last10 := rb.ReadLast(10)
	if len(last10) != 5 {
		t.Fatalf("expected 5 items, got %d", len(last10))
	}
}

// --- HTTP API Tests ---

func TestAPI_ListSessions_Empty(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodGet, "/sessions", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var sessions []sessionResponse
	json.NewDecoder(w.Body).Decode(&sessions)
	if len(sessions) != 0 {
		t.Errorf("expected empty list, got %d sessions", len(sessions))
	}
}

func TestAPI_CreateSession_MissingName(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPI_CreateSession_InvalidBinary(t *testing.T) {
	b := New("/nonexistent/binary")
	srv := NewServer(b)

	body := `{"name":"test-session","workdir":"/tmp"}`
	req := httptest.NewRequest(http.MethodPost, "/sessions", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_GetSession_NotFound(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodGet, "/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPI_SendMessage_NotFound(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	body := `{"message":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/sessions/nonexistent/message", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPI_SendMessage_EmptyMessage(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	body := `{"message":""}`
	req := httptest.NewRequest(http.MethodPost, "/sessions/test/message", strings.NewReader(body))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAPI_GetOutput_NotFound(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodGet, "/sessions/nonexistent/output", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPI_DeleteSession_NotFound(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodDelete, "/sessions/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAPI_InvalidSubpath(t *testing.T) {
	b := New("")
	srv := NewServer(b)

	req := httptest.NewRequest(http.MethodGet, "/sessions/test/unknown", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

// TestRegistration verifies the process backend is registered.
func TestRegistration(t *testing.T) {
	if !backend.IsRegistered("process") {
		t.Error("expected process backend to be registered")
	}

	b, err := backend.GetBackendByName("process")
	if err != nil {
		t.Fatalf("failed to get process backend: %v", err)
	}

	var _ = b
}
