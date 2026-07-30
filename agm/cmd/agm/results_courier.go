package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
//     always available, no VROOM mesh required (AC-parity with escalation.go).
//   - an ops.SendMessage relay into the orchestrator supervisor session,
//     asking it to call PushNotification, when that session is alive.
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

// courierFileState is the last-processed watermark for one transcript file.
type courierFileState struct {
	Size int64 `json:"size"`
	Line int   `json:"line"`
}

// courierState is the full on-disk cursor: last-seen watermark per
// transcript file path, so restarts and repeat ticks never re-report the
// same completion twice. There is deliberately no global "have we ever run
// before" flag: seeding is decided per file (see scanClaudeProjects), so a
// file that's still mid-stream on the courier's first tick gets seeded
// (not reported-from-scratch) whenever it's next observed, rather than
// silently skipping straight to "known" with no watermark and dumping its
// entire history as a false completion.
type courierState struct {
	Files map[string]courierFileState `json:"files"`
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

// lastAssistantText walks lines[from:] looking for the LAST line that is an
// assistant turn carrying non-empty text content (as opposed to a bare
// tool_use turn). Returns "" if none of the new lines qualify.
func lastAssistantText(lines []string, from int) string {
	headline := ""
	for i := from; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" {
			continue
		}
		for _, block := range entry.Message.Content {
			if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
				headline = strings.TrimSpace(block.Text)
			}
		}
	}
	return headline
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
// Seeding is per file, not a one-time global event: the first time a given
// file is observed (no prior watermark), its current watermark is recorded
// without reporting anything, so deploying the courier does not dump the
// day's entire backlog as a notification storm, and a file that happened to
// be mid-stream on an early tick still gets seeded (not falsely treated as
// "known from line 0") once it finally goes idle.
func scanClaudeProjects(home string, idleGrace time.Duration, st *courierState) ([]resultsCourierEvent, error) {
	matches, err := filepath.Glob(claudeProjectsGlob(home))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches) // deterministic order for tests and logs

	var events []resultsCourierEvent
	now := time.Now()

	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue // raced with deletion/rotation; skip this tick
		}
		prev, known := st.Files[path]
		if known && info.Size() == prev.Size {
			continue // nothing new, cheap skip (no read)
		}
		if now.Sub(info.ModTime()) < idleGrace {
			continue // still being written; re-check next tick, watermark untouched
		}

		lines, err := readLines(path)
		if err != nil {
			continue
		}

		if !known {
			st.Files[path] = courierFileState{Size: info.Size(), Line: len(lines)}
			continue // first sight of this file: seed silently, don't report
		}

		fromLine := prev.Line
		if fromLine > len(lines) {
			fromLine = 0 // file was truncated/rotated; re-scan from the top
		}

		if headline := lastAssistantText(lines, fromLine); headline != "" {
			projectDir := filepath.Base(filepath.Dir(path))
			events = append(events, resultsCourierEvent{
				SessionFile: path,
				Project:     projectLabel(home, projectDir),
				Headline:    truncateHeadline(headline, 220),
				DetectedAt:  now,
			})
		}

		st.Files[path] = courierFileState{Size: info.Size(), Line: len(lines)}
	}

	return events, nil
}

func readLines(path string) ([]string, error) {
	f, err := os.Open(path) //#nosec G304 -- path comes from our own glob under ~/.claude/projects
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // transcripts can have long lines
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
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
		prompt := "RESULTS COURIER RELAY: call the PushNotification tool once with " +
			"status \"proactive\" and exactly this message, then stop: " + title + " — " + body
		result, err := ops.SendMessage(opCtx, &ops.SendMessageRequest{
			Recipient:  orchestratorName,
			Message:    prompt,
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

// appendCourierReceipt appends one JSON line to the courier's own trail file
// — the audit log a human (or a future debugging session) can point at to
// answer "did the courier actually try to tell me about X".
func appendCourierReceipt(stateDir string, receipt courierDeliveryReceipt) error {
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(stateDir, "trail.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) //#nosec G304 -- fixed path under the courier's own state dir
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
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

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events, err := scanClaudeProjects(home, idleGrace, &st)
			if err != nil {
				fmt.Fprintf(os.Stderr, "results-courier: scan error: %v\n", err)
				continue
			}
			if saveErr := saveCourierState(statePath, st); saveErr != nil {
				fmt.Fprintf(os.Stderr, "results-courier: state save error: %v\n", saveErr)
			}
			if len(events) == 0 {
				continue
			}

			out, _ := json.Marshal(struct {
				Timestamp string                `json:"timestamp"`
				Kind      string                `json:"kind"`
				Events    []resultsCourierEvent `json:"events"`
			}{time.Now().Format(time.RFC3339), "results_courier.detected", events})
			fmt.Println(string(out))

			receipt := deliverCourierResults(ctx, opCtx, orchestratorName, events)
			if recErr := appendCourierReceipt(stateDir, receipt); recErr != nil {
				fmt.Fprintf(os.Stderr, "results-courier: receipt write error: %v\n", recErr)
			}
			receiptOut, _ := json.Marshal(receipt)
			fmt.Println(string(receiptOut))
		}
	}
}
