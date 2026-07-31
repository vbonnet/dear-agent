package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/pkg/notify"
)

// --- Results courier (ce-vyu9 follow-up: dispatch-watch root cause) ---
//
// Completions from Claude Code / Cowork work sessions were only reaching
// Valentin when he asked, because the only thing watching for them
// (a Cowork-sandboxed "dispatch-watch" scheduled task) has no access to the
// real session store and no working send-to-user tool: it is structurally
// blind to the sessions it is meant to report on.
//
// startResultsCourier fixes this by running as a second ticker inside the
// ALREADY host-level, ALREADY always-on `agm watch-stalled` daemon (no new
// launchd job, no sandbox): it polls ~/.claude/projects for sessions whose
// last turn is new since the previous check, and for each newly-idle
// completion fires the same two-channel human-notification pattern
// documented in cmd/vroom-dispatch/escalation.go:
//
//   - a native macOS desktop notification (pkg/notify.DesktopDispatcher) —
//     the typed channel that carries transcript-derived display text, with no
//     model interpretation (AC-parity with escalation.go).
//   - an ops.SendMessage relay into the orchestrator supervisor session,
//     asking it to emit only a fixed content-free mobile nudge when that
//     session is alive. Transcript content never crosses this model boundary.
//
// Every scan + delivery attempt is appended to a JSONL receipt so "did a
// completion actually get surfaced" is verifiable after the fact, not just
// asserted.

// resultsCourierEvent is one newly-observed "a session just finished
// replying" signal.
type resultsCourierEvent struct {
	SessionFile string    `json:"session_file"`
	Project     string    `json:"project"`
	Headline    string    `json:"headline"`
	DetectedAt  time.Time `json:"detected_at"`
}

const resultsCourierRelayMarker = "RESULTS COURIER RELAY:"

// courierFileState is the last-processed watermark for one transcript file.
type courierFileState struct {
	Size         int64  `json:"size"`
	Line         int    `json:"line"`
	Identity     string `json:"identity,omitempty"`
	ModifiedAt   int64  `json:"modified_at_unix_nano,omitempty"`
	ChangedAt    int64  `json:"changed_at_unix_nano,omitempty"`
	BoundaryHash string `json:"boundary_hash,omitempty"`
	ContentHash  string `json:"content_hash,omitempty"`
	HashState    string `json:"hash_state,omitempty"`
	RelayPending bool   `json:"relay_pending,omitempty"`
}

// courierState is the full on-disk cursor: last-seen watermark per
// transcript file path, so restarts and repeat ticks never re-report the
// same completion twice. BaselineComplete distinguishes files that existed
// when the courier was first deployed (seed silently) from sessions created
// later (track from line zero so their first completion is deliverable).
type courierState struct {
	Files            map[string]courierFileState `json:"files"`
	BaselineComplete bool                        `json:"baseline_complete"`
}

// resultsCourierStateDir is a writable, VROOM-runtime-appropriate home for
// the courier's cursor and receipt trail.
func resultsCourierStateDir(home string) string {
	return filepath.Join(home, ".agm", "vroom", "results-courier")
}

func loadCourierState(path string) (courierState, error) {
	data, err := os.ReadFile(path) //#nosec G304 -- fixed path under the courier's own state dir
	if os.IsNotExist(err) {
		return courierState{Files: map[string]courierFileState{}}, nil
	}
	if err != nil {
		return courierState{Files: map[string]courierFileState{}}, err
	}
	var st courierState
	if err := json.Unmarshal(data, &st); err != nil {
		return courierState{Files: map[string]courierFileState{}}, err
	}
	if st.Files == nil {
		st.Files = map[string]courierFileState{}
	}
	return st, nil
}

func saveCourierState(path string, st courierState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// claudeProjectsGlob matches every Claude Code session transcript on this
// host: ~/.claude/projects/<project-slug>/<session-uuid>.jsonl
func claudeProjectsGlob(home string) string {
	return filepath.Join(home, ".claude", "projects", "*", "*.jsonl")
}

// lastAssistantText returns text only when the terminal conversational entry
// in lines is an assistant turn with non-empty text and no tool use.
// A later tool invocation, tool result, or user turn clears an earlier
// candidate: file inactivity alone does not prove that an earlier assistant
// message completed the current turn.
func lastAssistantText(lines []string) string {
	headline, _ := courierAssistantText(lines, false)
	return headline
}

func courierAssistantText(lines []string, relayPending bool) (string, bool) {
	headline := ""
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry struct {
			Type        string `json:"type"`
			IsSidechain bool   `json:"isSidechain"`
			Message     struct {
				Content    json.RawMessage `json:"content"`
				StopReason string          `json:"stop_reason"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.IsSidechain {
			continue
		}
		switch entry.Type {
		case "assistant":
			text, completed := completedAssistantText(
				entry.Message.Content,
				entry.Message.StopReason,
			)
			if relayPending {
				headline = ""
				if completed {
					relayPending = false
				}
				continue
			}
			if !completed {
				headline = ""
				continue
			}
			headline = text
		case "user":
			if courierUserContainsToolResult(entry.Message.Content) {
				headline = ""
				continue
			}
			relayPending = courierUserIsRelayPrompt(entry.Message.Content)
			headline = ""
		case "tool_use", "tool_result":
			headline = ""
		}
	}
	return headline, relayPending
}

func courierUserContainsToolResult(content json.RawMessage) bool {
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(content, &blocks) != nil {
		return false
	}
	for _, block := range blocks {
		if block.Type == "tool_result" {
			return true
		}
	}
	return false
}

func courierUserIsRelayPrompt(content json.RawMessage) bool {
	var text string
	if json.Unmarshal(content, &text) != nil {
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(content, &blocks) != nil ||
			len(blocks) != 1 ||
			blocks[0].Type != "text" {
			return false
		}
		text = blocks[0].Text
	}
	if !strings.HasPrefix(text, resultsCourierRelayPromptPrefix) ||
		!strings.HasSuffix(text, resultsCourierRelayPromptSuffix) {
		return false
	}
	countText := strings.TrimSuffix(
		strings.TrimPrefix(text, resultsCourierRelayPromptPrefix),
		resultsCourierRelayPromptSuffix,
	)
	eventCount, err := strconv.Atoi(countText)
	return err == nil && eventCount > 0 && text == courierRelayPrompt(eventCount)
}

func completedAssistantText(content json.RawMessage, stopReason string) (string, bool) {
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		var text string
		if json.Unmarshal(content, &text) != nil {
			return "", false
		}
		text = strings.TrimSpace(text)
		return text, text != "" && stopReason != "tool_use"
	}

	var texts []string
	usesTool := stopReason == "tool_use"
	for _, block := range blocks {
		switch block.Type {
		case "text":
			if text := strings.TrimSpace(block.Text); text != "" {
				texts = append(texts, text)
			}
		case "tool_use":
			usesTool = true
		}
	}
	return strings.Join(texts, "\n"), !usesTool && len(texts) > 0
}

// truncateHeadline caps s to n runes, appending an ellipsis when it had to
// cut, and collapses whitespace to one line. (Named to avoid colliding with
// status.go's own truncate helper, which has different maxLen semantics.)
func truncateHeadline(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ") // collapse newlines/whitespace to one line
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// projectLabel turns a Claude Code project-slug directory name (a
// slugified absolute cwd made by replacing every "/" with "-", e.g.
// "-Users-vbonnet-src-dear-agent" for ~/src/dear-agent) into a short human
// label. Slugification is lossy — a literal "-" in a real path component
// (like "dear-agent") is indistinguishable from a "/" once slugified — so
// this can't perfectly invert it in general. It handles the common case by
// stripping the known home-directory prefix and a following "src"/
// "worktrees" root segment, which covers how this host's sessions are
// actually laid out; anything else falls back to the raw slug.
func projectLabel(home, dir string) string {
	homeSlug := strings.ReplaceAll(strings.TrimRight(home, "/"), "/", "-")
	if dir == homeSlug {
		return "home"
	}
	rest, ok := strings.CutPrefix(dir, homeSlug+"-")
	if !ok {
		return strings.Trim(dir, "-")
	}
	for _, root := range []string{"src-", "worktrees-"} {
		if after, found := strings.CutPrefix(rest, root); found {
			rest = after
			break
		}
	}
	if rest == "" {
		return "home"
	}
	return rest
}

// scanClaudeProjects looks for transcript files that have grown since the
// last recorded watermark, extracts a headline from any newly-idle
// completion, and advances the watermark for files it processed. A file
// modified within idleGrace of "now" is left untouched (still streaming) so
// it gets a clean, complete look on a later tick — its watermark is not
// touched, so it is neither seeded nor reported this round.
//
// The first scan seeds every file that already exists without reporting
// anything, including actively-written files. Once that baseline completes,
// an unknown path represents a newly-created session and starts at line zero
// so its first terminal completion is not discarded.
func scanClaudeProjects(home string, idleGrace time.Duration, st *courierState) ([]resultsCourierEvent, error) {
	matches, err := filepath.Glob(claudeProjectsGlob(home))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches) // deterministic order for tests and logs

	var events []resultsCourierEvent
	now := time.Now()
	initialBaseline := !st.BaselineComplete
	baselineFailed := false

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue // raced with deletion/rotation; skip this tick
		}
		event, baselineReadFailed := scanClaudeTranscript(
			home,
			path,
			info,
			now,
			idleGrace,
			initialBaseline,
			st,
			os.Stat,
		)
		baselineFailed = baselineFailed || baselineReadFailed
		if event != nil {
			events = append(events, *event)
		}
	}
	st.BaselineComplete = !baselineFailed

	return events, nil
}

func scanClaudeTranscript(
	home, path string,
	info os.FileInfo,
	now time.Time,
	idleGrace time.Duration,
	initialBaseline bool,
	st *courierState,
	restat func(string) (os.FileInfo, error),
) (*resultsCourierEvent, bool) {
	prev, known := st.Files[path]
	identity := courierFileIdentity(info)
	modifiedAt := info.ModTime().UnixNano()
	changedAt := courierFileChangeTime(info)
	replaced := courierTranscriptReplaced(
		known,
		prev,
		identity,
		info.Size(),
		modifiedAt,
		changedAt,
		path,
	)
	if known && info.Size() == prev.Size && !replaced {
		if err := refreshCourierUnchangedState(path, info, identity, prev, st); err != nil {
			return nil, false
		}
		return nil, false // nothing new
	}
	if initialBaseline && !known {
		if err := seedCourierBaseline(path, info, identity, st); err != nil {
			return nil, true
		}
		return nil, false
	}
	if !known {
		// This path appeared after the initial deployment baseline. Mark it
		// known at line zero before the idle check so its first completed
		// turn remains eligible once writing settles.
		prev = courierFileState{Identity: identity}
		st.Files[path] = prev
	}
	if now.Sub(info.ModTime()) < idleGrace {
		return nil, false // still being written; leave watermark untouched
	}

	lines, nextState, stable := readStableCourierDelta(
		path,
		prev,
		info,
		identity,
		modifiedAt,
		changedAt,
		replaced,
		restat,
	)
	if !stable {
		return nil, false
	}
	headline, relayPending := courierAssistantText(lines, prev.RelayPending)
	nextState.RelayPending = relayPending
	st.Files[path] = nextState
	if headline == "" {
		return nil, false
	}
	projectDir := filepath.Base(filepath.Dir(path))
	return &resultsCourierEvent{
		SessionFile: path,
		Project:     projectLabel(home, projectDir),
		Headline:    truncateHeadline(headline, 220),
		DetectedAt:  now,
	}, false
}

func readStableCourierDelta(
	path string,
	previous courierFileState,
	info os.FileInfo,
	identity string,
	modifiedAt int64,
	changedAt int64,
	replaced bool,
	restat func(string) (os.FileInfo, error),
) ([]string, courierFileState, bool) {
	lines, lineBase, err := readCourierDelta(path, previous, info, replaced)
	if err != nil {
		return nil, courierFileState{}, false
	}
	boundaryHash, err := courierBoundaryFingerprint(path, info.Size())
	if err != nil {
		return nil, courierFileState{}, false
	}
	contentHash, hashState, err := courierContentFingerprint(
		path,
		info.Size(),
		previous,
		replaced,
	)
	if err != nil {
		return nil, courierFileState{}, false
	}
	confirmedInfo, err := restat(path)
	if err != nil || !courierFileGenerationMatches(info, confirmedInfo) {
		return nil, courierFileState{}, false
	}
	return lines, courierFileState{
		Size:         info.Size(),
		Line:         lineBase + len(lines),
		Identity:     identity,
		ModifiedAt:   modifiedAt,
		ChangedAt:    changedAt,
		BoundaryHash: boundaryHash,
		ContentHash:  contentHash,
		HashState:    hashState,
	}, true
}

func courierFileGenerationMatches(before, after os.FileInfo) bool {
	return before.Size() == after.Size() &&
		before.ModTime().UnixNano() == after.ModTime().UnixNano() &&
		courierFileChangeTime(before) == courierFileChangeTime(after) &&
		courierFileIdentity(before) == courierFileIdentity(after)
}

func refreshCourierUnchangedState(
	path string,
	info os.FileInfo,
	identity string,
	previous courierFileState,
	st *courierState,
) error {
	updated := previous
	updated.Identity = identity
	updated.ModifiedAt = info.ModTime().UnixNano()
	updated.ChangedAt = courierFileChangeTime(info)
	if updated.BoundaryHash == "" {
		boundaryHash, err := courierBoundaryFingerprint(path, info.Size())
		if err != nil {
			return err
		}
		updated.BoundaryHash = boundaryHash
	}
	if updated.ContentHash == "" || updated.HashState == "" {
		contentHash, hashState, err := courierContentFingerprint(
			path,
			info.Size(),
			courierFileState{},
			true,
		)
		if err != nil {
			return err
		}
		updated.ContentHash = contentHash
		updated.HashState = hashState
	}
	if updated != previous {
		st.Files[path] = updated
	}
	return nil
}

func courierTranscriptReplaced(
	known bool,
	previous courierFileState,
	currentIdentity string,
	currentSize int64,
	currentModifiedAt int64,
	currentChangedAt int64,
	path string,
) bool {
	if !known || currentSize < previous.Size {
		return known
	}
	if previous.Identity != "" &&
		currentIdentity != "" &&
		previous.Identity != currentIdentity {
		return true
	}
	if courierTranscriptPrefixChanged(
		previous,
		currentSize,
		currentModifiedAt,
		currentChangedAt,
		path,
	) {
		return true
	}
	if previous.BoundaryHash == "" {
		return false
	}
	currentBoundaryHash, err := courierBoundaryFingerprint(path, previous.Size)
	return err != nil || currentBoundaryHash != previous.BoundaryHash
}

func courierTranscriptPrefixChanged(
	previous courierFileState,
	currentSize int64,
	currentModifiedAt int64,
	currentChangedAt int64,
	path string,
) bool {
	if currentSize < previous.Size || previous.ContentHash == "" {
		return false
	}
	generationChanged := previous.ModifiedAt != 0 &&
		currentModifiedAt != previous.ModifiedAt
	if previous.ChangedAt == 0 ||
		(currentChangedAt != 0 && currentChangedAt != previous.ChangedAt) {
		generationChanged = true
	}
	if !generationChanged {
		return false
	}
	prefixHash, _, err := courierContentFingerprint(
		path,
		previous.Size,
		courierFileState{},
		true,
	)
	return err != nil || prefixHash != previous.ContentHash
}

func seedCourierBaseline(
	path string,
	info os.FileInfo,
	identity string,
	st *courierState,
) error {
	lines, _, err := readLinesFrom(path, 0)
	if err != nil {
		return err
	}
	boundaryHash, err := courierBoundaryFingerprint(path, info.Size())
	if err != nil {
		return err
	}
	contentHash, hashState, err := courierContentFingerprint(
		path,
		info.Size(),
		courierFileState{},
		true,
	)
	if err != nil {
		return err
	}
	st.Files[path] = courierFileState{
		Size:         info.Size(),
		Line:         len(lines),
		Identity:     identity,
		ModifiedAt:   info.ModTime().UnixNano(),
		ChangedAt:    courierFileChangeTime(info),
		BoundaryHash: boundaryHash,
		ContentHash:  contentHash,
		HashState:    hashState,
	}
	return nil
}

func readCourierDelta(
	path string,
	prev courierFileState,
	info os.FileInfo,
	replaced bool,
) ([]string, int, error) {
	offset := int64(0)
	lineBase := 0
	if !replaced && prev.Size > 0 && prev.Size < info.Size() {
		offset = prev.Size
		lineBase = prev.Line
	}
	lines, actualOffset, err := readLinesFrom(path, offset)
	if err != nil {
		return nil, 0, err
	}
	if actualOffset == 0 {
		lineBase = 0
	}
	return lines, lineBase, nil
}

func courierFileIdentity(info os.FileInfo) string {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	return fmt.Sprintf("%d:%d", stat.Dev, stat.Ino)
}

const courierBoundaryWindow = int64(4096)

func courierBoundaryFingerprint(path string, end int64) (string, error) {
	f, err := os.Open(path) //#nosec G304 -- path comes from our own transcript glob
	if err != nil {
		return "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if end < 0 || end > info.Size() {
		return "", fmt.Errorf("invalid transcript boundary %d for size %d", end, info.Size())
	}
	start := max(int64(0), end-courierBoundaryWindow)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, f, end-start); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// courierContentFingerprint extends a validated persisted SHA-256 state with
// the appended suffix. The continuity check separately rehashes the complete
// prior prefix before treating growth as append-only, and generation changes
// hash from byte zero so metadata-only touches remain distinguishable from
// content rewrites.
func courierContentFingerprint(
	path string,
	end int64,
	previous courierFileState,
	replaced bool,
) (string, string, error) {
	f, err := os.Open(path) //#nosec G304 -- path comes from our own transcript glob
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", "", err
	}
	if end < 0 || end > info.Size() {
		return "", "", fmt.Errorf("invalid transcript content boundary %d for size %d", end, info.Size())
	}

	contentHash, start := restoreCourierContentHash(previous, end, replaced)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", "", err
	}
	if _, err := io.CopyN(contentHash, f, end-start); err != nil {
		return "", "", err
	}
	hashState, err := contentHash.(encoding.BinaryMarshaler).MarshalBinary()
	if err != nil {
		return "", "", err
	}
	return fmt.Sprintf("%x", contentHash.Sum(nil)),
		base64.StdEncoding.EncodeToString(hashState),
		nil
}

func restoreCourierContentHash(
	previous courierFileState,
	end int64,
	replaced bool,
) (hash.Hash, int64) {
	fresh := sha256.New()
	if replaced ||
		previous.Size <= 0 ||
		previous.Size >= end ||
		previous.ContentHash == "" ||
		previous.HashState == "" {
		return fresh, 0
	}
	encodedState, err := base64.StdEncoding.DecodeString(previous.HashState)
	if err != nil {
		return fresh, 0
	}
	candidate := sha256.New()
	if err := candidate.(encoding.BinaryUnmarshaler).UnmarshalBinary(encodedState); err != nil {
		return fresh, 0
	}
	if previous.Size < 0 ||
		len(encodedState) < 8 ||
		binary.BigEndian.Uint64(encodedState[len(encodedState)-8:]) != uint64(previous.Size) {
		return fresh, 0
	}
	if fmt.Sprintf("%x", candidate.Sum(nil)) != previous.ContentHash {
		return fresh, 0
	}
	return candidate, previous.Size
}

// readLinesFrom reads only transcript bytes at or after offset. It returns the
// actual offset used so callers can distinguish an appended suffix from a
// defensive full rescan. A watermark that is no longer on a JSONL record
// boundary indicates truncation, replacement, or an incomplete prior line and
// therefore falls back to offset zero.
func readLinesFrom(path string, offset int64) ([]string, int64, error) {
	f, err := os.Open(path) //#nosec G304 -- path comes from our own glob under ~/.claude/projects
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	if offset < 0 {
		offset = 0
	}
	if offset > 0 {
		if _, err := f.Seek(offset-1, io.SeekStart); err != nil {
			offset = 0
		} else {
			var boundary [1]byte
			if _, err := io.ReadFull(f, boundary[:]); err != nil || boundary[0] != '\n' {
				offset = 0
			}
		}
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, 0, err
	}

	var lines []string
	reader := bufio.NewReader(f)
	for {
		line, readErr := reader.ReadString('\n')
		if len(line) > 0 {
			line = strings.TrimSuffix(line, "\n")
			line = strings.TrimSuffix(line, "\r")
			lines = append(lines, line)
		}
		if errors.Is(readErr, io.EOF) {
			return lines, offset, nil
		}
		if readErr != nil {
			return nil, 0, readErr
		}
	}
}

// formatDigest renders events as a single push-friendly message: a short
// title plus one line per session, most-recent-looking first.
func formatDigest(events []resultsCourierEvent) (title, body string) {
	if len(events) == 1 {
		title = "1 session finished"
	} else {
		title = fmt.Sprintf("%d sessions finished", len(events))
	}
	lines := make([]string, 0, len(events))
	for _, e := range events {
		lines = append(lines, fmt.Sprintf("%s: %s", e.Project, truncateHeadline(e.Headline, 100)))
	}
	body = strings.Join(lines, " | ")
	return title, body
}

// courierDeliveryReceipt records what actually happened when the courier
// tried to reach Valentin, so a run can be verified after the fact instead
// of trusted on faith.
type courierDeliveryReceipt struct {
	Timestamp    string   `json:"timestamp"`
	Events       int      `json:"events"`
	Sessions     []string `json:"sessions"`
	Digest       string   `json:"digest"`
	DesktopSent  bool     `json:"desktop_sent"`
	DesktopError string   `json:"desktop_error,omitempty"`
	RelayTarget  string   `json:"relay_target,omitempty"`
	RelaySent    bool     `json:"relay_sent"`
	RelayError   string   `json:"relay_error,omitempty"`
}

// deliverCourierResults fires both notification channels for a batch of
// events and returns a receipt describing what happened on each channel.
// Neither channel blocks the caller for more than the local command's own
// timeout; a wedged notifier can never stall the watch-stalled tick.
func deliverCourierResults(ctx context.Context, opCtx *ops.OpContext, orchestratorName string, events []resultsCourierEvent) courierDeliveryReceipt {
	title, body := formatDigest(events)
	sessions := make([]string, 0, len(events))
	for _, e := range events {
		sessions = append(sessions, e.Project+": "+e.Headline)
	}

	receipt := courierDeliveryReceipt{
		Timestamp: time.Now().Format(time.RFC3339),
		Events:    len(events),
		Sessions:  sessions,
		Digest:    title + " — " + body,
	}

	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	desktop := notify.NewDesktopDispatcher()
	if err := desktop.Dispatch(dctx, &notify.Notification{Title: title, Body: body}); err != nil {
		receipt.DesktopError = err.Error()
	} else {
		receipt.DesktopSent = true
	}

	if orchestratorName != "" {
		receipt.RelayTarget = orchestratorName
		result, err := ops.SendMessage(opCtx, &ops.SendMessageRequest{
			Recipient:  orchestratorName,
			Message:    courierRelayPrompt(len(events)),
			Autonomous: true,
		})
		switch {
		case err != nil:
			receipt.RelayError = err.Error() // expected fallback when no supervisor is alive
		case result != nil:
			receipt.RelaySent = result.Delivered
		}
	}

	return receipt
}

// courierRelayPrompt intentionally accepts only a count, never an event or
// transcript-derived string. The orchestrator receives a fixed instruction
// and fixed mobile message; the typed desktop dispatcher above owns the
// human-readable transcript content.
func courierRelayPrompt(eventCount int) string {
	return fmt.Sprintf("%s%d%s", resultsCourierRelayPromptPrefix, eventCount, resultsCourierRelayPromptSuffix)
}

const (
	resultsCourierRelayPromptPrefix = resultsCourierRelayMarker +
		" call the PushNotification tool once with status \"proactive\" and exactly this message, " +
		"then stop: Results courier detected "
	resultsCourierRelayPromptSuffix = " completed session(s). Review the typed desktop notification " +
		"or the local results-courier receipt trail. Do not read or relay transcript content."
)

func courierDeliverySucceeded(receipt courierDeliveryReceipt) bool {
	return receipt.DesktopSent || receipt.RelaySent
}

// appendCourierReceipt appends one JSON line to the courier's own trail file
// — the audit log a human (or a future debugging session) can point at to
// answer "did the courier actually try to tell me about X".
func appendCourierReceipt(stateDir string, receipt courierDeliveryReceipt) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, "trail.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //#nosec G304 -- fixed path under the courier's own state dir
	if err != nil {
		return err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("append courier receipt: %w", err),
				fmt.Errorf("close courier receipt: %w", closeErr),
			)
		}
		return fmt.Errorf("append courier receipt: %w", err)
	}
	if err := f.Sync(); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("sync courier receipt: %w", err),
				fmt.Errorf("close courier receipt: %w", closeErr),
			)
		}
		return fmt.Errorf("sync courier receipt: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close courier receipt: %w", err)
	}
	return nil
}

func cloneCourierState(st courierState) courierState {
	clone := courierState{
		Files:            make(map[string]courierFileState, len(st.Files)),
		BaselineComplete: st.BaselineComplete,
	}
	maps.Copy(clone.Files, st.Files)
	return clone
}

type courierDeliverFunc func(
	context.Context,
	*ops.OpContext,
	string,
	[]resultsCourierEvent,
) courierDeliveryReceipt

// processResultsCourierTick computes the next cursor on a copy and publishes
// it only after at least one notification channel accepts the entire batch.
// A total delivery failure therefore leaves both durable and in-memory state
// unchanged, so the same completions are retried on the next tick.
func processResultsCourierTick(
	ctx context.Context,
	opCtx *ops.OpContext,
	home, orchestratorName string,
	idleGrace time.Duration,
	st *courierState,
	deliver courierDeliverFunc,
) error {
	stateDir := resultsCourierStateDir(home)
	statePath := filepath.Join(stateDir, "state.json")
	candidate := cloneCourierState(*st)

	events, err := scanClaudeProjects(home, idleGrace, &candidate)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if len(events) == 0 {
		if err := saveCourierState(statePath, candidate); err != nil {
			return fmt.Errorf("save state: %w", err)
		}
		*st = candidate
		return nil
	}

	out, _ := json.Marshal(struct {
		Timestamp string                `json:"timestamp"`
		Kind      string                `json:"kind"`
		Events    []resultsCourierEvent `json:"events"`
	}{time.Now().Format(time.RFC3339), "results_courier.detected", events})
	fmt.Println(string(out))

	receipt := deliver(ctx, opCtx, orchestratorName, events)
	receiptErr := appendCourierReceipt(stateDir, receipt)
	receiptOut, _ := json.Marshal(receipt)
	fmt.Println(string(receiptOut))

	if !courierDeliverySucceeded(receipt) {
		if receiptErr != nil {
			return fmt.Errorf("all notification channels failed; append receipt: %w", receiptErr)
		}
		return fmt.Errorf("all notification channels failed; cursor retained for retry")
	}
	if err := saveCourierState(statePath, candidate); err != nil {
		return fmt.Errorf("notification delivered but save state: %w", err)
	}
	*st = candidate
	if receiptErr != nil {
		return fmt.Errorf("notification delivered but append receipt: %w", receiptErr)
	}
	return nil
}

// startResultsCourier runs the poll-detect-notify loop on its own ticker
// until ctx is done. It is started as a second goroutine from
// runWatchStalled so no new launchd job is needed: the courier rides the
// already-installed, already-always-on watch-stalled daemon.
func startResultsCourier(ctx context.Context, opCtx *ops.OpContext, home, orchestratorName string, checkInterval, idleGrace time.Duration) {
	stateDir := resultsCourierStateDir(home)
	statePath := filepath.Join(stateDir, "state.json")

	st, err := loadCourierState(statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "results-courier: state load error (continuing with fresh state): %v\n", err)
	}

	// Establish the deployment baseline before waiting for the first ticker.
	// Otherwise a short-lived session created during that wait would look like
	// pre-existing history and have its first completion seeded away.
	if err := processResultsCourierTick(
		ctx,
		opCtx,
		home,
		orchestratorName,
		idleGrace,
		&st,
		deliverCourierResults,
	); err != nil {
		fmt.Fprintf(os.Stderr, "results-courier: %v\n", err)
	}

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := processResultsCourierTick(
				ctx,
				opCtx,
				home,
				orchestratorName,
				idleGrace,
				&st,
				deliverCourierResults,
			); err != nil {
				fmt.Fprintf(os.Stderr, "results-courier: %v\n", err)
			}
		}
	}
}
