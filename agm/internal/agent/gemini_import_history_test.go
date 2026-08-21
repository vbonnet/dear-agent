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
		// json.Unmarshal accepts both of these into a Message and leaves it
		// zero-valued, so decoding alone cannot tell a message from any other
		// JSON value. Each must be rejected, not imported as a speakerless turn.
		{name: "json null", data: []byte("null\n"), format: FormatJSONL},
		{name: "empty object", data: []byte("{}\n"), format: FormatJSONL},
		{name: "unknown role", data: []byte(`{"role":"system","content":"x"}` + "\n"), format: FormatJSONL},
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

// TestWriteHistoryRoundTripsOversizedRecord covers a record larger than the
// 64 KiB default bufio.Scanner token. Without an enlarged read buffer an
// import would write a record its own reader then chokes on, which breaks the
// round trip AGP-66 promises for exactly the long turns most worth keeping.
func TestWriteHistoryRoundTripsOversizedRecord(t *testing.T) {
	adapter, sessionID, metadata := newGeminiHistoryFixture(t)

	huge := strings.Repeat("x", 200*1024)
	if err := adapter.writeHistory(metadata, []Message{{Role: RoleUser, Content: huge}}); err != nil {
		t.Fatalf("writeHistory returned error: %v", err)
	}

	got, err := adapter.GetHistory(sessionID)
	if err != nil {
		t.Fatalf("GetHistory returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if len(got[0].Content) != len(huge) {
		t.Errorf("content length = %d, want %d", len(got[0].Content), len(huge))
	}
}

// TestParseImportedMessagesTrimsBlankLines covers a payload with trailing and
// interior blank lines, which a JSONL file routinely has.
func TestParseImportedMessagesTrimsBlankLines(t *testing.T) {
	data := []byte("\n" + `{"role":"user","content":"a"}` + "\n\n  \n" + `{"role":"assistant","content":"b"}` + "\n\n")

	messages, err := parseImportedMessages(data, FormatJSONL)
	if err != nil {
		t.Fatalf("parseImportedMessages returned error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages, want 2: %#v", len(messages), messages)
	}
}

// TestWriteHistoryLeavesNoTemporaryFileOnSuccess covers the cleanup flag: the
// temporary sibling must be renamed away, not left behind and not removed
// twice.
func TestWriteHistoryLeavesNoTemporaryFileOnSuccess(t *testing.T) {
	adapter, _, metadata := newGeminiHistoryFixture(t)

	if err := adapter.writeHistory(metadata, []Message{{Role: RoleUser, Content: "x"}}); err != nil {
		t.Fatalf("writeHistory returned error: %v", err)
	}

	historyPath, err := adapter.getHistoryPath(metadata)
	if err != nil {
		t.Fatalf("getHistoryPath returned error: %v", err)
	}
	entries, err := os.ReadDir(filepath.Dir(historyPath))
	if err != nil {
		t.Fatalf("failed to read history directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "history.jsonl" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("history directory holds %v, want only history.jsonl", names)
	}
}

// TestImportedSessionNamesAreDistinct covers the collision the timestamp alone
// allowed: two imports in the same second shared a tmux name, and because
// history is keyed by that name the second import replaced the first one's
// conversation.
func TestImportedSessionNamesAreDistinct(t *testing.T) {
	seen := map[string]bool{}
	for range 100 {
		name := importedSessionName()
		if seen[name] {
			t.Fatalf("importedSessionName produced a duplicate within one run: %q", name)
		}
		seen[name] = true
	}
}

// TestParseImportedMessagesRejectsOversizedRecord covers AGP-65's rejection
// half. The reader caps a record at maxHistoryLineBytes, so accepting a larger
// one at import would persist a record the matching GetHistory then fails on.
func TestParseImportedMessagesRejectsOversizedRecord(t *testing.T) {
	oversized := `{"role":"user","content":"` + strings.Repeat("x", maxHistoryLineBytes) + `"}` + "\n"

	messages, err := parseImportedMessages([]byte(oversized), FormatJSONL)
	if err == nil {
		t.Fatalf("expected a rejection, got %d messages", len(messages))
	}
	if !strings.Contains(err.Error(), "history limit") {
		t.Errorf("error = %v, want it to name the history limit", err)
	}
}

// TestParseImportedMessagesAcceptsLargeRecordUnderTheLimit is the other half of
// AGP-65: a record that is big but within the limit must still import, so the
// rejection above cannot be satisfied by refusing everything large.
func TestParseImportedMessagesAcceptsLargeRecordUnderTheLimit(t *testing.T) {
	large := `{"role":"user","content":"` + strings.Repeat("x", 256*1024) + `"}` + "\n"

	messages, err := parseImportedMessages([]byte(large), FormatJSONL)
	if err != nil {
		t.Fatalf("parseImportedMessages returned error: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("got %d messages, want 1", len(messages))
	}
}
