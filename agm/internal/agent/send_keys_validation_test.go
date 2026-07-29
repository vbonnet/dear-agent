package agent

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestValidateSendKeysText covers SPEC R41 and R42.
func TestValidateSendKeysText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Rejected: anything that breaks out of a single TUI input line.
		{"carriage return submits the line", "ok\rrm -rf ~", true},
		{"line feed submits the line", "ok\nrm -rf ~", true},
		{"NUL", "ok\x00evil", true},
		{"escape starts a terminal sequence", "ok\x1b[2J", true},
		{"tab triggers completion rather than inserting", "ok\tevil", true},
		{"DEL", "ok\x7f", true},
		{"invalid UTF-8", string([]byte{'o', 'k', 0xff}), true},
		{"bare carriage return", "\r", true},
		{"empty", "", true},

		// Accepted: a session title is human-facing prose. Shell metacharacters
		// are ordinary characters here — the parser is the agent's input
		// widget, not a shell — so rejecting them would be the wrong tool and
		// would mangle legitimate titles.
		{"plain", "my session", false},
		{"apostrophe", "valentin's session", false},
		{"shell metacharacters are harmless in a TUI", "fix $HOME && `ls`; echo", false},
		{"non-ASCII", "réunion — 会議", false},
		{"emoji", "ship it 🚢", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSendKeysText("session name", tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("ValidateSendKeysText(%q) = nil, want error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("ValidateSendKeysText(%q) = %v, want nil", tt.input, err)
			}
		})
	}
}

// newRenameTestStore returns a store holding one session, plus its ID.
func newRenameTestStore(t *testing.T, tmuxName string) (SessionStore, SessionID) {
	t.Helper()
	store, err := NewJSONSessionStore(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}
	sessionID := SessionID("test-session-rename-injection")
	if err := store.Set(sessionID, &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      "Original Title",
		WorkingDir: t.TempDir(),
		Project:    "test-project",
	}); err != nil {
		t.Fatalf("failed to set session metadata: %v", err)
	}
	return store, sessionID
}

// TestRenameRejectsControlCharacters covers SPEC R41 and R43 across all four
// adapters that send a rename to a harness TUI.
//
// The assertion is on the error *message*, not merely on non-nil. These
// adapters talk to a tmux server that does not exist under test, so a rename
// with no validation at all would also return an error — just a tmux failure
// arriving after the injected text had already been written to the pane.
// Requiring "control character" proves the value was rejected before any send.
func TestRenameRejectsControlCharacters(t *testing.T) {
	const hostileName = "innocent\x1b[201~\x15/quit"

	adapters := []struct {
		name string
		exec func(store SessionStore) func(Command) error
	}{
		{
			name: "claude",
			exec: func(store SessionStore) func(Command) error {
				return (&ClaudeAdapter{sessionStore: store}).ExecuteCommand
			},
		},
		{
			name: "gemini",
			exec: func(store SessionStore) func(Command) error {
				return (&GeminiCLIAdapter{sessionStore: store}).ExecuteCommand
			},
		},
		{
			name: "agy",
			exec: func(store SessionStore) func(Command) error {
				return (&AgyAdapter{sessionStore: store}).ExecuteCommand
			},
		},
		{
			name: "pi",
			exec: func(store SessionStore) func(Command) error {
				return (&PiAdapter{sessionStore: store}).ExecuteCommand
			},
		},
	}

	for _, a := range adapters {
		t.Run(a.name, func(t *testing.T) {
			store, sessionID := newRenameTestStore(t, a.name+"-test-rename")

			err := a.exec(store)(Command{
				Type: CommandRename,
				Params: map[string]any{
					"session_id": string(sessionID),
					"name":       hostileName,
				},
			})

			if err == nil {
				t.Fatalf("rename with %q was accepted; want rejection", hostileName)
			}
			if !strings.Contains(err.Error(), "control character") {
				t.Fatalf("rename failed for the wrong reason: %v\n"+
					"want a validation error naming the control character, which proves the "+
					"name was rejected before any text reached the pane", err)
			}

			// The title must not have been recorded either.
			got, getErr := store.Get(sessionID)
			if getErr != nil {
				t.Fatalf("failed to re-read session metadata: %v", getErr)
			}
			if got.Title != "Original Title" {
				t.Errorf("session title was updated to %q despite rejection", got.Title)
			}
		})
	}
}
