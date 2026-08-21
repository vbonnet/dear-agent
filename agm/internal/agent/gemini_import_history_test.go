package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newGeminiHistoryFixture returns an adapter whose history writes and reads
// land under a temporary HOME, plus a stored session to address them with.
func newGeminiHistoryFixture(t *testing.T) (*GeminiCLIAdapter, SessionID, *SessionMetadata) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	store, err := NewJSONSessionStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	sessionID := SessionID("import-session")
	metadata := &SessionMetadata{
		TmuxName:   "gemini-import",
		CreatedAt:  time.Now(),
		WorkingDir: home,
	}
	if err := store.Set(sessionID, metadata); err != nil {
		t.Fatalf("failed to store session metadata: %v", err)
	}

	return &GeminiCLIAdapter{sessionStore: store}, sessionID, metadata
}

// TestParseImportedMessagesJSONL covers the decode half of an import.
func TestParseImportedMessagesJSONL(t *testing.T) {
	data := []byte(`{"role":"user","content":"hello"}` + "\n\n" + `{"role":"assistant","content":"hi"}` + "\n")

	messages, err := parseImportedMessages(data, FormatJSONL)
	if err != nil {
		t.Fatalf("parseImportedMessages returned error: %v", err)
	}

	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2: %#v", len(messages), messages)
	}
	if messages[0].Role != RoleUser || messages[0].Content != "hello" {
		t.Errorf("first message = %#v", messages[0])
	}
	if messages[1].Role != RoleAssistant || messages[1].Content != "hi" {
		t.Errorf("second message = %#v", messages[1])
	}
}

// TestParseImportedMessagesRejects covers the formats that cannot be decoded
// and malformed JSONL. Each must be an error, never a silent empty import.
func TestParseImportedMessagesRejects(t *testing.T) {
	tests := []struct {
		name   string
		data   []byte
		format ConversationFormat
	}{
		{name: "markdown", data: []byte("# conversation"), format: FormatMarkdown},
		{name: "html", data: []byte("<html></html>"), format: FormatHTML},
		{name: "native", data: []byte("{}"), format: FormatNative},
		{name: "unknown", data: []byte("{}"), format: ConversationFormat("bogus")},
		{name: "malformed jsonl", data: []byte("{not json}\n"), format: FormatJSONL},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			messages, err := parseImportedMessages(tc.data, tc.format)
			if err == nil {
				t.Fatalf("expected an error, got %d messages", len(messages))
			}
			if messages != nil {
				t.Errorf("expected nil messages alongside the error, got %#v", messages)
			}
		})
	}
}

// TestWriteHistoryRoundTripsThroughGetHistory is the regression test for the
// discarded-append bug (staticcheck SA4010): parsed messages used to be
// dropped on the floor, so an import produced a session with no history.
func TestWriteHistoryRoundTripsThroughGetHistory(t *testing.T) {
	adapter, sessionID, metadata := newGeminiHistoryFixture(t)

	want := []Message{
		{Role: RoleUser, Content: "first"},
		{Role: RoleAssistant, Content: "second"},
	}

	if err := adapter.writeHistory(metadata, want); err != nil {
		t.Fatalf("writeHistory returned error: %v", err)
	}

	got, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}

	if len(got) != len(want) {
		t.Fatalf("got %d messages, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Role != want[i].Role || got[i].Content != want[i].Content {
			t.Errorf("message %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

// TestWriteHistoryIsJSONLGetHistoryCanParse pins the on-disk encoding, since
// GetHistory silently skips any line it cannot unmarshal.
func TestWriteHistoryIsJSONLGetHistoryCanParse(t *testing.T) {
	adapter, _, metadata := newGeminiHistoryFixture(t)

	if err := adapter.writeHistory(metadata, []Message{{Role: RoleUser, Content: "only"}}); err != nil {
		t.Fatalf("writeHistory returned error: %v", err)
	}

	historyPath, err := adapter.getHistoryPath(metadata)
	if err != nil {
		t.Fatalf("getHistoryPath returned error: %v", err)
	}

	raw, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatalf("failed to read history file: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d lines, want 1: %q", len(lines), string(raw))
	}
	var decoded Message
	if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
		t.Fatalf("history line is not valid JSON: %v", err)
	}
	if decoded.Content != "only" {
		t.Errorf("decoded content = %q, want %q", decoded.Content, "only")
	}
}

// TestWriteHistoryEmptyConversation covers importing a conversation with no
// messages: the file must exist and be empty, not absent.
func TestWriteHistoryEmptyConversation(t *testing.T) {
	adapter, sessionID, metadata := newGeminiHistoryFixture(t)

	if err := adapter.writeHistory(metadata, nil); err != nil {
		t.Fatalf("writeHistory returned error: %v", err)
	}

	historyPath, err := adapter.getHistoryPath(metadata)
	if err != nil {
		t.Fatalf("getHistoryPath returned error: %v", err)
	}
	info, err := os.Stat(historyPath)
	if err != nil {
		t.Fatalf("history file missing after empty import: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("history file size = %d, want 0", info.Size())
	}

	got, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d messages, want 0: %#v", len(got), got)
	}
}

// TestWriteHistoryReplacesPriorHistory covers the rename-based install: a
// second write must not leave a partially-overwritten first write behind.
func TestWriteHistoryReplacesPriorHistory(t *testing.T) {
	adapter, sessionID, metadata := newGeminiHistoryFixture(t)

	long := []Message{
		{Role: RoleUser, Content: strings.Repeat("a", 512)},
		{Role: RoleAssistant, Content: strings.Repeat("b", 512)},
	}
	if err := adapter.writeHistory(metadata, long); err != nil {
		t.Fatalf("first writeHistory returned error: %v", err)
	}
	if err := adapter.writeHistory(metadata, []Message{{Role: RoleUser, Content: "short"}}); err != nil {
		t.Fatalf("second writeHistory returned error: %v", err)
	}

	got, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(got) != 1 || got[0].Content != "short" {
		t.Fatalf("history was not replaced: %#v", got)
	}

	// The temporary sibling must not survive as a stray file.
	historyPath, err := adapter.getHistoryPath(metadata)
	if err != nil {
		t.Fatalf("getHistoryPath returned error: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(historyPath))
	if err != nil {
		t.Fatalf("failed to read history directory: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("history directory holds %d entries, want 1", len(entries))
	}
}
