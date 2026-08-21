package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
)

// maxHistoryLineBytes caps a single history record on read. One conversation
// turn can easily exceed the Scanner default of 64 KiB, and a record this
// large is far likelier to be a corrupt file than a real message.
const maxHistoryLineBytes = 8 * 1024 * 1024

// GeminiCLIAdapter is the concrete compatibility adapter for Gemini CLI.
//
// It runs Gemini CLI in tmux (like Claude) and provides concrete lifecycle
// abstraction for Gemini sessions.
type GeminiCLIAdapter struct {
	sessionStore SessionStore
}

var (
	geminiResumeHasSession       = tmux.HasSession
	geminiResumeNewSession       = tmux.NewSession
	geminiResumeIsProcessRunning = tmux.IsProcessRunning
)

// NewGeminiCLIAdapter creates a new Gemini CLI adapter instance.
//
// If sessionStore is nil, creates a default JSON-backed store at ~/.agm/sessions.json.
func NewGeminiCLIAdapter(sessionStore SessionStore) (*GeminiCLIAdapter, error) {
	if sessionStore == nil {
		store, err := NewJSONSessionStore("")
		if err != nil {
			return nil, fmt.Errorf("failed to create session store: %w", err)
		}
		sessionStore = store
	}

	return &GeminiCLIAdapter{
		sessionStore: sessionStore,
	}, nil
}

// Name returns the agent identifier
func (a *GeminiCLIAdapter) Name() string {
	return "gemini-cli"
}

// Version returns the model name
func (a *GeminiCLIAdapter) Version() string {
	return "gemini-3.5-flash"
}

// CreateSession creates a new Gemini session.
//
// Creates a tmux session with Gemini CLI and stores the SessionID mapping.
func (a *GeminiCLIAdapter) CreateSession(ctx SessionContext) (SessionID, error) {
	// Generate unique SessionID
	sessionID := SessionID(uuid.New().String())

	// Use session name as tmux session name (or generate one)
	tmuxName := ctx.Name
	if tmuxName == "" {
		tmuxName = fmt.Sprintf("gemini-%s", time.Now().Format("20060102-150405"))
	}
	values := append([]string{ctx.WorkingDirectory}, ctx.AuthorizedDirs...)
	if err := validatePastedShellValues(values...); err != nil {
		return "", fmt.Errorf("validate Gemini launch: %w", err)
	}

	// Check if tmux session already exists
	exists, err := tmux.HasSession(tmuxName)
	if err != nil {
		return "", fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		// Create new tmux session
		if err := tmux.NewSession(tmuxName, ctx.WorkingDirectory); err != nil {
			return "", fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	// Build Gemini command with directory authorization
	// Use --include-directories to pre-approve workspace and avoid trust prompt
	geminiCmd := buildGeminiStartCommand(ctx.WorkingDirectory, ctx.AuthorizedDirs)

	// Start Gemini CLI in tmux
	if err := sendPastedShellCommand(tmuxName, geminiCmd, values...); err != nil {
		// Clean up tmux session on error if we created it
		if !exists {
			_ = tmux.SendCommand(tmuxName, "exit\r")
		}
		return "", fmt.Errorf("failed to start Gemini in tmux session: %w", err)
	}

	// Wait for Gemini to be ready (prompt appears)
	if err := tmux.WaitForProcessReady(tmuxName, "gemini", 30*time.Second); err != nil {
		// Non-fatal warning - Gemini may still be initializing
		fmt.Fprintf(os.Stderr, "Warning: Gemini prompt not detected (still initializing)\n")
	}

	// Extract Gemini session UUID from --list-sessions output
	// This UUID is needed for --resume flag to restore session state
	geminiUUID, err := a.extractLatestGeminiUUID(ctx.WorkingDirectory)
	if err != nil {
		// Non-fatal: UUID extraction failure doesn't block session creation
		// Resume will fall back to "latest" if UUID not available
		fmt.Fprintf(os.Stderr, "Warning: failed to extract Gemini UUID: %v\n", err)
		geminiUUID = "" // Empty UUID means resume will use "latest"
	}

	// Store session metadata
	metadata := &SessionMetadata{
		TmuxName:   tmuxName,
		Title:      ctx.Name, // Set initial title from session name
		CreatedAt:  time.Now(),
		WorkingDir: ctx.WorkingDirectory,
		Project:    ctx.Project,
		UUID:       geminiUUID, // Store Gemini's native UUID
	}

	if err := a.sessionStore.Set(sessionID, metadata); err != nil {
		// Clean up tmux session on error
		_ = tmux.SendCommand(tmuxName, "exit\r")
		return "", fmt.Errorf("failed to store session metadata: %w", err)
	}

	return sessionID, nil
}

// ResumeSession resumes an existing Gemini session.
//
// Attaches to the tmux session associated with the SessionID.
func (a *GeminiCLIAdapter) ResumeSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Check if tmux session exists
	exists, err := geminiResumeHasSession(metadata.TmuxName)
	if err != nil {
		return fmt.Errorf("failed to check tmux session: %w", err)
	}

	sendCommands := !exists
	if exists {
		// Check if Gemini is already running
		geminiRunning, err := geminiResumeIsProcessRunning(metadata.TmuxName, "gemini")
		if err != nil {
			// Detection failed - skip commands for safety
			sendCommands = false
		} else if !geminiRunning {
			sendCommands = true
		}
	}

	if sendCommands {
		if err := validatePastedShellValues(metadata.WorkingDir, metadata.UUID); err != nil {
			return fmt.Errorf("validate Gemini resume: %w", err)
		}
		if !exists {
			// Validate every value that can reach the terminal before creating
			// a replacement session. A healthy existing Gemini process needs no
			// paste and therefore does not reject legacy metadata unnecessarily.
			if err := geminiResumeNewSession(metadata.TmuxName, metadata.WorkingDir); err != nil {
				return fmt.Errorf("failed to create tmux session: %w", err)
			}
		}

		// Build resume command with UUID.
		// If UUID is stored, use it. Otherwise fall back to "latest".
		resumeCmd := buildGeminiResumeCommand(metadata.WorkingDir, metadata.UUID)

		if err := sendPastedShellCommand(metadata.TmuxName, resumeCmd, metadata.WorkingDir, metadata.UUID); err != nil {
			return fmt.Errorf("failed to resume Gemini: %w", err)
		}

		// Wait for ready
		if err := tmux.WaitForProcessReady(metadata.TmuxName, "gemini", 30*time.Second); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Gemini prompt not detected\n")
		}
	}

	return nil
}

// TerminateSession terminates a Gemini session.
//
// Sends exit command to Gemini and removes from session store.
func (a *GeminiCLIAdapter) TerminateSession(sessionID SessionID) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Send exit to Gemini (graceful shutdown)
	if err := tmux.SendCommand(metadata.TmuxName, "exit\r"); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to send exit to session: %v\n", err)
	}

	// Remove from session store
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session from store: %w", err)
	}

	return nil
}

// GetSessionStatus returns the status of a Gemini session.
//
// Queries tmux to determine if session is active or terminated.
func (a *GeminiCLIAdapter) GetSessionStatus(sessionID SessionID) (Status, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		// Session not in store = terminated
		return StatusTerminated, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	// Check if tmux session exists
	exists, err := tmux.HasSession(metadata.TmuxName)
	if err != nil {
		return StatusTerminated, fmt.Errorf("failed to check tmux session: %w", err)
	}

	if !exists {
		return StatusTerminated, nil
	}

	// TODO: Differentiate between active and suspended
	return StatusActive, nil
}

// SendMessage sends a message to Gemini.
//
// Uses tmux send-keys to deliver the message to the Gemini CLI.
func (a *GeminiCLIAdapter) SendMessage(sessionID SessionID, message Message) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	// Send message content to tmux pane
	if err := tmux.SendCommand(metadata.TmuxName, message.Content); err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	return nil
}

// GetHistory retrieves conversation history.
//
// Parses the history.jsonl file for the session.
func (a *GeminiCLIAdapter) GetHistory(sessionID SessionID) ([]Message, error) {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return nil, fmt.Errorf("session not found: %w", err)
	}

	// Find history file for this session
	// Note: Gemini CLI stores sessions in ~/.gemini/tmp/<project_hash>/chats/
	// For now, we'll use a simplified approach similar to Claude.
	// getHistoryPath is the single owner of this layout so a read and an
	// import can never disagree about where history lives.
	historyPath, err := a.getHistoryPath(metadata)
	if err != nil {
		return nil, err
	}

	// Check if history file exists
	if _, err := os.Stat(historyPath); os.IsNotExist(err) {
		// No history yet (new session)
		return []Message{}, nil
	}

	// Parse JSONL file
	file, err := os.Open(historyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var messages []Message
	scanner := bufio.NewScanner(file)
	// A default Scanner tops out at 64 KiB per line, which is smaller than a
	// single long conversation turn. Without this an import would write a
	// record that the matching read then fails on, breaking the round trip
	// AGP-63 and AGP-65 promise. The +1 is the trailing newline writeHistory
	// appends after every record: ScanLines can only locate that delimiter by
	// buffering it along with the content before it, so a maxHistoryLineBytes
	// record plus its newline needs a token buffer one byte larger than
	// maxHistoryLineBytes, not equal to it.
	scanner.Buffer(make([]byte, 0, 64*1024), maxHistoryLineBytes+1)
	for scanner.Scan() {
		var msg Message
		// UseNumber, matching parseImportedMessages: the default decoder
		// would turn a Metadata integer back into a float64 on every read,
		// rounding values outside float64's exact range even though
		// writeHistory persisted the original digits.
		dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		dec.UseNumber()
		if err := dec.Decode(&msg); err != nil {
			// Skip malformed lines
			continue
		}
		// Decode stops after one JSON value and ignores whatever follows, so
		// a line holding a valid value plus trailing garbage would yield the
		// prefix as if it were the whole record. The json.Unmarshal this
		// replaced rejected such a line, so without this check the switch to
		// a decoder would start silently returning altered data from an
		// already-corrupted history file. Requiring a second Decode to hit
		// io.EOF is the actual "nothing left" test.
		if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
			// Skip lines with trailing data, as before.
			continue
		}
		messages = append(messages, msg)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read history file: %w", err)
	}

	return messages, nil
}

// ExportConversation exports conversation in specified format.
func (a *GeminiCLIAdapter) ExportConversation(sessionID SessionID, format ConversationFormat) ([]byte, error) {
	messages, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	switch format {
	case FormatJSONL:
		// Export as JSONL (one JSON object per line)
		result := make([]byte, 0)
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal message: %w", err)
			}
			result = append(result, data...)
			result = append(result, '\n')
		}
		return result, nil

	case FormatMarkdown:
		// Export as Markdown
		var result string
		result += fmt.Sprintf("# Gemini Conversation\n\nSession ID: %s\n\n", sessionID)
		for _, msg := range messages {
			role := "User"
			if msg.Role == RoleAssistant {
				role = "Assistant"
			}
			result += fmt.Sprintf("## %s (%s)\n\n%s\n\n", role, msg.Timestamp.Format(time.RFC3339), msg.Content)
		}
		return []byte(result), nil

	case FormatHTML:
		return nil, fmt.Errorf("HTML export not supported for Gemini adapter")

	case FormatNative:
		return nil, fmt.Errorf("native format export not supported for Gemini adapter")

	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// parseImportedMessages decodes serialized conversation data into messages.
//
// Only FormatJSONL is decodable; every other format is rejected rather than
// silently importing an empty conversation.
func parseImportedMessages(data []byte, format ConversationFormat) ([]Message, error) {
	if format != FormatJSONL {
		return nil, fmt.Errorf("unsupported import format: %s", format)
	}

	messages := []Message{}
	for line := range bytes.SplitSeq(data, []byte{'\n'}) {
		// bytes.TrimSpace uses unicode.IsSpace, which accepts characters JSON
		// does not treat as whitespace between tokens (a vertical tab, a
		// non-breaking space, ...). Trimming those would silently normalize a
		// malformed record instead of rejecting it, undoing AGP-67. RFC 8259
		// permits exactly these four bytes as insignificant whitespace, which
		// is also what a blank line or a CRLF line ending actually needs.
		line = bytes.Trim(line, " \t\r\n")
		if len(line) == 0 {
			continue
		}
		// json.Unmarshal replaces invalid UTF-8 inside a JSON string with
		// U+FFFD instead of erroring, which would silently alter the
		// imported content rather than reject the malformed record it came
		// from. AGP-67 requires rejecting it instead.
		if !utf8.Valid(line) {
			return nil, fmt.Errorf("message record is not valid UTF-8")
		}
		// utf8.Valid only sees raw bytes, not \uXXXX escapes: an unpaired
		// UTF-16 surrogate escape like \ud800 is itself valid ASCII, so it
		// passes that check, but encoding/json accepts it during decode and
		// silently substitutes U+FFFD rather than erroring. That is the same
		// silent-corruption failure mode as invalid UTF-8, just reached
		// through an escape instead of a raw byte.
		if hasLoneSurrogateEscape(line) {
			return nil, fmt.Errorf("message record has an unpaired surrogate escape")
		}

		// UseNumber, not the default: decoding into map[string]interface{}
		// turns every JSON number into a float64, which silently rounds an
		// integer outside float64's exact range (9007199254740993 becomes
		// ...92). writeHistory re-marshals the decoded value, so the round
		// trip would return altered metadata.
		var msg Message
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		if err := dec.Decode(&msg); err != nil {
			return nil, fmt.Errorf("failed to parse message: %w", err)
		}
		// Decode reads exactly one JSON value and leaves anything after it
		// alone, so a line holding a valid message immediately followed by
		// trailing data would decode the first value and silently discard the
		// rest instead of rejecting the record. dec.More() is not the right
		// check: it reports whether another element remains in the array or
		// object currently being parsed, so it also returns false when the
		// very next byte is a stray `]` or `}` — trailing garbage, not proof
		// there is nothing left. Requiring a second Decode to hit io.EOF is
		// the actual "nothing left" check: a nil error means another value
		// followed, and any other error means unparseable trailing bytes:
		// both are trailing data.
		if err := dec.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("message record has trailing data after its JSON value")
		}
		// json.Unmarshal accepts `null` and `{}` into a Message and leaves it
		// zero-valued, so decoding alone does not establish that a record IS a
		// message. AGP-67 requires a record with no supported role to be
		// rejected rather than imported as an empty turn with no speaker.
		if msg.Role != RoleUser && msg.Role != RoleAssistant {
			return nil, fmt.Errorf("message has unsupported role %q", msg.Role)
		}
		// GetHistory's scanner caps a single line at maxHistoryLineBytes, and
		// what it reads back is writeHistory's re-marshaled encoding of msg,
		// not this raw input line. Re-marshaling adds fields the wire format
		// omits (ID, Timestamp, Metadata) and can expand escaped characters,
		// so a line just under the limit can still persist a record over it.
		// Checking the encoding that is actually written, rather than the one
		// that was read, is what makes AGP-65's limit and AGP-63's round trip
		// agree; rejecting here keeps the promise from AGP-68 that a failed
		// import leaves no session behind, since none exists yet.
		encoded, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("failed to validate message size: %w", err)
		}
		if len(encoded) > maxHistoryLineBytes {
			return nil, fmt.Errorf("message record is %d bytes once persisted, over the %d-byte history limit", len(encoded), maxHistoryLineBytes)
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// hasLoneSurrogateEscape reports whether a JSON string literal in line
// contains a \uXXXX escape for a UTF-16 surrogate half (U+D800-U+DFFF) that
// is not immediately paired with its matching other half. encoding/json
// accepts such an escape during decode and silently substitutes U+FFFD
// instead of erroring, which would let a malformed record through as altered
// content rather than being rejected.
func hasLoneSurrogateEscape(line []byte) bool {
	inString := false
	for pos := 0; pos < len(line); {
		b := line[pos]
		if !inString {
			if b == '"' {
				inString = true
			}
			pos++
			continue
		}
		switch {
		case b == '"':
			inString = false
			pos++
		case b != '\\':
			pos++
		case pos+1 >= len(line):
			// A truncated escape at the end of the record; json.Decode will
			// report the real syntax error.
			return false
		case line[pos+1] != 'u':
			pos += 2 // an ordinary two-byte escape such as \n or \"
		case pos+6 > len(line):
			return false
		default:
			r, err := strconv.ParseUint(string(line[pos+2:pos+6]), 16, 32)
			if err != nil {
				return false
			}
			pos += 6
			if r < 0xD800 || r > 0xDFFF {
				continue // an ordinary \uXXXX escape, not a surrogate half
			}
			if r <= 0xDBFF && pos+6 <= len(line) && line[pos] == '\\' && line[pos+1] == 'u' {
				if r2, err2 := strconv.ParseUint(string(line[pos+2:pos+6]), 16, 32); err2 == nil && r2 >= 0xDC00 && r2 <= 0xDFFF {
					pos += 6 // consumed the matching low surrogate too
					continue
				}
			}
			return true // a high surrogate with no matching low, or a lone low surrogate
		}
	}
	return false
}

// writeHistory persists messages as the session's history.jsonl.
//
// The file is written through a temporary sibling and renamed, so a failed
// write leaves any prior history intact rather than truncated. The encoding
// matches what GetHistory parses, which is what makes an import readable by a
// later GetHistory or ExportConversation call.
func (a *GeminiCLIAdapter) writeHistory(metadata *SessionMetadata, messages []Message) error {
	historyPath, err := a.getHistoryPath(metadata)
	if err != nil {
		return fmt.Errorf("failed to get history path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
		return fmt.Errorf("failed to create history directory: %w", err)
	}

	var buf bytes.Buffer
	for _, msg := range messages {
		encoded, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("failed to marshal message: %w", err)
		}
		buf.Write(encoded)
		buf.WriteByte('\n')
	}

	tmp, err := os.CreateTemp(filepath.Dir(historyPath), "history-*.jsonl")
	if err != nil {
		return fmt.Errorf("failed to create temporary history file: %w", err)
	}
	tmpPath := tmp.Name()
	installed := false
	// Clean up only when the rename did not happen. Closing and removing on
	// the success path would be two failing syscalls against a file that no
	// longer exists under that name.
	defer func() {
		if installed {
			return
		}
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to write history file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close history file: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("failed to set history file mode: %w", err)
	}
	if err := os.Rename(tmpPath, historyPath); err != nil {
		return fmt.Errorf("failed to install history file: %w", err)
	}
	installed = true

	return nil
}

// importedSessionName builds the tmux session name for an import.
//
// It carries a random suffix, not just a timestamp. CreateSession reuses an
// existing tmux session of the same name and getHistoryPath keys storage by
// that name, so two imports in the same second would otherwise share one
// history file and the later one would silently replace the earlier one's
// conversation.
func importedSessionName() string {
	return fmt.Sprintf("imported-%s-%s", time.Now().Format("20060102-150405"), uuid.New().String()[:8])
}

// ImportConversation imports conversation from serialized data.
//
// The decoded messages are written to the new session's history file, so a
// subsequent GetHistory or ExportConversation returns what was imported.
func (a *GeminiCLIAdapter) ImportConversation(data []byte, format ConversationFormat) (SessionID, error) {
	messages, err := parseImportedMessages(data, format)
	if err != nil {
		return "", err
	}

	// Held so rollback has the tmux name even when the store read below is
	// what fails: CreateSession uses this name verbatim.
	tmuxName := importedSessionName()

	sessionID, err := a.CreateSession(SessionContext{
		Name:             tmuxName,
		WorkingDirectory: os.TempDir(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}

	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		// The session is launched and registered but its ID never reaches the
		// caller, so this leaks exactly like a failed history write. A remote
		// or transient store makes this reachable.
		if rollbackErr := a.rollbackFailedImport(sessionID, tmuxName); rollbackErr != nil {
			return "", fmt.Errorf("failed to read imported session metadata: %w (rollback also failed: %w)", err, rollbackErr)
		}
		return "", fmt.Errorf("failed to read imported session metadata: %w", err)
	}

	if err := a.writeHistory(metadata, messages); err != nil {
		// The caller never receives this session's ID, so it could not
		// terminate the tmux process or clear the store entry itself. Roll
		// back rather than leave an unreachable session behind.
		if rollbackErr := a.rollbackFailedImport(sessionID, metadata.TmuxName); rollbackErr != nil {
			return "", fmt.Errorf("failed to import conversation history: %w (rollback also failed: %w)", err, rollbackErr)
		}
		return "", fmt.Errorf("failed to import conversation history: %w", err)
	}

	return sessionID, nil
}

// rollbackFailedImport cleans up a session created for an import whose
// history could not be persisted.
//
// TerminateSession's graceful "exit" is best-effort by design: it logs a
// send failure and still deletes the store entry, which is the right
// behavior for a caller that holds the session ID and can retry. Here the
// caller never received the ID, so a discarded store entry would make the
// session permanently unreachable even if the tmux process were still
// alive. KillSessionChecked instead confirms the process is gone (or was
// already gone) before the store entry is removed, so a cleanup failure
// stays observable and the record survives for another cleanup attempt
// instead of being silently dropped.
func (a *GeminiCLIAdapter) rollbackFailedImport(sessionID SessionID, tmuxName string) error {
	if err := tmux.KillSessionChecked(tmuxName); err != nil {
		return fmt.Errorf("tmux session %q could not be confirmed terminated: %w", tmuxName, err)
	}
	if err := a.sessionStore.Delete(sessionID); err != nil {
		return fmt.Errorf("failed to remove session record: %w", err)
	}
	return nil
}

// Capabilities returns Gemini's feature capabilities.
func (a *GeminiCLIAdapter) Capabilities() Capabilities {
	return Capabilities{
		SupportsSlashCommands: true,    // Gemini CLI supports /chat, /memory, etc.
		SupportsHooks:         true,    // Gemini CLI supports hooks (SessionStart, SessionEnd, etc.)
		SupportsTools:         true,    // Gemini supports function calling
		SupportsVision:        true,    // Gemini 3.x supports vision
		SupportsMultimodal:    true,    // Gemini 3.x supports audio/video
		SupportsStreaming:     true,    // Gemini CLI supports streaming
		SupportsSystemPrompts: true,    // Gemini supports system instructions
		MaxContextWindow:      1048576, // 1M input tokens (3.5 Flash, GA 2026-05-19)
		ModelName:             "gemini-3.5-flash",
	}
}

// ExecuteCommand executes a generic command.
//
// Translates generic commands to Gemini CLI-specific operations.
func (a *GeminiCLIAdapter) ExecuteCommand(cmd Command) error {
	// Validate session_id parameter
	sessionIDStr, err := getStringParam(cmd.Params, "session_id")
	if err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	metadata, err := a.sessionStore.Get(SessionID(sessionIDStr))
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	switch cmd.Type {
	case CommandRename:
		return a.cmdRename(cmd, sessionIDStr, metadata)
	case CommandSetDir:
		return a.cmdSetDir(cmd, sessionIDStr, metadata)
	case CommandAuthorize:
		// Gemini CLI doesn't have runtime directory authorization
		// (directories are pre-authorized via --include-directories at session creation)
		return nil
	case CommandClearHistory:
		return a.cmdClearHistory(metadata)
	case CommandSetSystemPrompt:
		return a.cmdSetSystemPrompt(cmd, sessionIDStr, metadata)
	case CommandRunHook:
		return a.cmdRunHook(cmd, sessionIDStr, metadata)
	default:
		return fmt.Errorf("unsupported command type: %s", cmd.Type)
	}
}

func (a *GeminiCLIAdapter) cmdRename(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	newName, err := getStringParam(cmd.Params, "name")
	if err != nil {
		return fmt.Errorf("rename command: %w", err)
	}
	if err := ValidateSendKeysText("session name", newName); err != nil {
		return fmt.Errorf("rename command: %w", err)
	}
	if err := tmux.SendCommand(metadata.TmuxName, fmt.Sprintf("/chat save %s\r", newName)); err != nil {
		return fmt.Errorf("failed to send chat save command: %w", err)
	}
	metadata.Title = newName
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update session title: %w", err)
	}
	return nil
}

func (a *GeminiCLIAdapter) cmdSetDir(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	newPath, err := getStringParam(cmd.Params, "path")
	if err != nil {
		return fmt.Errorf("setdir command: %w", err)
	}
	if err := ValidateSendDirPath(newPath); err != nil {
		return fmt.Errorf("setdir command: %w", err)
	}
	if err := sendPastedShellCommand(metadata.TmuxName, buildSetDirCommand(newPath), newPath); err != nil {
		return fmt.Errorf("failed to send cd command: %w", err)
	}
	metadata.WorkingDir = newPath
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update working directory: %w", err)
	}
	return nil
}

func (a *GeminiCLIAdapter) cmdClearHistory(metadata *SessionMetadata) error {
	historyPath, err := a.getHistoryPath(metadata)
	if err != nil {
		return fmt.Errorf("failed to get history path: %w", err)
	}
	if err := os.Remove(historyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove history file: %w", err)
	}
	return nil
}

func (a *GeminiCLIAdapter) cmdSetSystemPrompt(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	prompt, err := getStringParam(cmd.Params, "prompt")
	if err != nil {
		return fmt.Errorf("set_system_prompt command: %w", err)
	}
	metadata.SystemPrompt = prompt
	if err := a.sessionStore.Set(SessionID(sessionIDStr), metadata); err != nil {
		return fmt.Errorf("failed to update system prompt: %w", err)
	}
	return nil
}

func (a *GeminiCLIAdapter) cmdRunHook(cmd Command, sessionIDStr string, metadata *SessionMetadata) error {
	hookName, err := getStringParam(cmd.Params, "hook_name")
	if err != nil {
		return fmt.Errorf("run_hook command: %w", err)
	}
	return a.executeHook(SessionID(sessionIDStr), metadata.TmuxName, hookName)
}

// RunHook executes a session lifecycle hook for the Gemini CLI.
//
// Triggers Gemini CLI hooks (SessionStart, SessionEnd, BeforeAgent, AfterAgent)
// via subprocess execution. Hooks output JSON that gets injected into session context.
//
// Hook failures are logged but don't block the session (graceful degradation).
func (a *GeminiCLIAdapter) RunHook(sessionID SessionID, hookName string) error {
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("session not found: %w", err)
	}

	return a.executeHook(sessionID, metadata.TmuxName, hookName)
}

// executeHook runs a Gemini CLI hook and processes its output.
//
// Hooks are triggered via the Gemini CLI lifecycle. The hook script:
// 1. Receives hook name and session context as environment variables
// 2. Executes custom logic (e.g., log session start, update metadata)
// 3. Outputs JSON with context updates
//
// Hook output is parsed and injected into session context.
// Errors are logged but don't fail the operation (graceful degradation).
func (a *GeminiCLIAdapter) executeHook(sessionID SessionID, tmuxName, hookName string) error {
	// Get session metadata for hook context
	metadata, err := a.sessionStore.Get(sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session metadata: %w", err)
	}

	// Build hook execution environment
	// Gemini CLI hooks are typically configured in ~/.gemini/config.yaml
	// and triggered via lifecycle events (SessionStart, SessionEnd, etc.)
	//
	// For now, we simulate hook execution by:
	// 1. Creating a hook ready-file signal (similar to Claude's approach)
	// 2. Allowing hooks to output JSON for context injection

	// Create hook ready-file directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get home directory for hook: %v\n", err)
		return nil // Non-fatal: hooks are optional
	}

	hookDir := filepath.Join(homeDir, ".agm", "gemini-hooks")
	if err := os.MkdirAll(hookDir, 0o700); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to create hook directory: %v\n", err)
		return nil // Non-fatal
	}

	// Create hook execution marker
	hookFile := filepath.Join(hookDir, fmt.Sprintf("%s-%s.json", string(sessionID), hookName))

	// Prepare hook context data
	hookContext := map[string]interface{}{
		"session_id":   string(sessionID),
		"hook_name":    hookName,
		"session_name": metadata.Title,
		"working_dir":  metadata.WorkingDir,
		"project":      metadata.Project,
		"tmux_session": tmuxName,
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	// Write hook context to file
	contextData, err := json.MarshalIndent(hookContext, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to marshal hook context: %v\n", err)
		return nil // Non-fatal
	}

	if err := os.WriteFile(hookFile, contextData, 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to write hook context: %v\n", err)
		return nil // Non-fatal
	}

	// Log hook execution
	fmt.Fprintf(os.Stderr, "[Gemini Hook] Executed %s hook for session %s\n", hookName, metadata.Title)

	// TODO: In a future implementation, we could:
	// 1. Execute actual hook scripts via subprocess
	// 2. Parse JSON output from hooks
	// 3. Inject parsed data into session metadata
	// 4. Handle hook timeouts and failures
	//
	// For now, we create the hook context file which can be:
	// - Read by external hook scripts
	// - Used for debugging and testing
	// - Extended with actual subprocess execution

	return nil
}

// Helper functions

// getHistoryPath returns the path to the history file for a given session.
func (a *GeminiCLIAdapter) getHistoryPath(metadata *SessionMetadata) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	return filepath.Join(homeDir, ".gemini", "sessions", metadata.TmuxName, "history.jsonl"), nil
}

// extractLatestGeminiUUID extracts the most recent Gemini session UUID for the given project directory.
//
// It runs `gemini --list-sessions` in the project directory and parses the output to find the latest UUID.
// Returns empty string if extraction fails (non-fatal - resume will use "latest" fallback).
func (a *GeminiCLIAdapter) extractLatestGeminiUUID(workingDir string) (string, error) {
	// Run gemini --list-sessions in the working directory
	// Output format (from investigation):
	// 0: Wed, Feb 26, 2025, 01:06:06 PM [23a6e871-bb1f-48ec-bdbe-1f6ae90f9686]
	// 1: Wed, Feb 26, 2025, 01:05:57 PM [8c123456-abcd-1234-5678-9012345678ab]
	//
	// Latest session is index 0 (most recent first)

	// Use exec.Command to run gemini --list-sessions
	cmd := exec.Command("gemini", "--list-sessions")
	cmd.Dir = workingDir

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run gemini --list-sessions: %w", err)
	}

	// Parse output to extract UUID from first line
	// Expected format: "0: <timestamp> [<uuid>]"
	lines := strings.Split(string(output), "\n")
	if len(lines) == 0 {
		return "", fmt.Errorf("no sessions found in output")
	}

	// Find first line with UUID pattern [...]
	uuidPattern := regexp.MustCompile(`\[([a-f0-9-]+)\]`)
	for _, line := range lines {
		matches := uuidPattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			// matches[0] is full match "[uuid]", matches[1] is captured UUID
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("no UUID found in --list-sessions output")
}
