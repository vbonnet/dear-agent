package tmux

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

const maxPromptFileSize = 10 * 1024 // 10KB

// QueuedInputType classifies what kind of input is queued in a session
type QueuedInputType int

// QueuedInputType values classifying input observed in a session pane.
const (
	QueuedInputNone  QueuedInputType = iota // No queued input detected
	QueuedInputAGM                          // Queued input is a stuck AGM message ([From: header)
	QueuedInputHuman                        // Queued input is freeform human text
)

// hasActiveSpinner checks if the pane content contains a Claude Code spinner
// character, indicating AI is actively generating output. When the spinner is
// visible, any pane content changes are AI output — not human input.
//
// Bug fix (2026-04-10): Prevents false "human input in progress" detection
// during active AI generation. The spinner characters cycle while Claude is
// thinking/generating, and content changes between captures were being
// misclassified as human typing.
func hasActiveSpinner(paneContent string) bool {
	return strings.ContainsAny(paneContent, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")
}

// InputLineHasContent checks if the input line (the line containing the Claude
// prompt character ❯) has any user-typed content after the prompt marker.
// This detects when a human is actively typing and delivery should be aborted.
//
// Bug fix (2026-03-31): Prevents agent messages from interrupting human typing.
// Previously only [Pasted text] indicators were checked; actual typed text on the
// input line was not detected.
func InputLineHasContent(paneContent string) bool {
	return inputLineAfterPrompt(paneContent) != ""
}

// HasGhostTextInPrompt reports whether the content after the ❯ prompt is
// Claude Code ghost/placeholder text (dim attribute \x1b[2m) rather than real
// human input. Ghost text is rendered dim and cannot be cleared with C-u/C-k
// (Claude Code re-renders it); it must not be treated as a human typing event.
//
// This is a live-capture call; prefer HasGhostTextInANSI when an ANSI capture
// is already available to avoid a second tmux round-trip.
func HasGhostTextInPrompt(sessionName string) bool {
	socketPath := GetSocketPath()
	cmd := exec.Command("tmux", "-S", socketPath, "capture-pane",
		"-t", NormalizeTmuxSessionName(sessionName), "-p", "-e", "-S", "-5")
	out, err := cmd.Output()
	if err != nil {
		return false // Can't capture — fail safe (don't suppress the guard)
	}
	return HasGhostTextInANSI(string(out))
}

// HasGhostTextInANSI is the pure-logic version of HasGhostTextInPrompt that
// operates on an already-captured ANSI pane buffer. Use this instead of
// HasGhostTextInPrompt when an ANSI capture is already available, to avoid
// the two-capture race where pane state changes between captures.
func HasGhostTextInANSI(ansiContent string) bool {
	for line := range strings.SplitSeq(ansiContent, "\n") {
		plainLine := stripANSI(line)
		if !strings.Contains(plainLine, "❯") {
			continue
		}
		idx := strings.Index(line, "❯")
		if idx < 0 {
			continue
		}
		return IsDimOrGreySGR(line[idx:])
	}
	return false
}

// sgrParamRe matches an ANSI SGR escape sequence and captures its (possibly
// empty, possibly semicolon-separated) numeric parameter list.
var sgrParamRe = regexp.MustCompile(`\x1b\[([0-9;]*)m`)

// IsDimOrGreySGR reports whether s contains any ANSI SGR escape sequence that
// renders text dim or grey. Claude Code styles ghost/placeholder text with the
// dim attribute (\x1b[2m), but other overseers — notably the
// vroom-meta-orchestrator — use 256-color grey (\x1b[38;5;241m) for the same
// hint text after the ❯ prompt. This generalizes the original dim-only check
// (ce-v9in / PR #512) to all dim/grey SGR variants (ce-5miu):
//
//   - 2            dim attribute (alone or combined, e.g. \x1b[2;38;5;241m)
//   - 90           bright-black (grey) foreground
//   - 38;5;N       256-color foreground where N is a grey shade (8 or 232–255)
//   - 38;2;r;g;b   truecolor foreground that is a dim grey (channels ≈ equal)
func IsDimOrGreySGR(s string) bool {
	for _, m := range sgrParamRe.FindAllStringSubmatch(s, -1) {
		if hasDimOrGreySGRParams(strings.Split(m[1], ";")) {
			return true
		}
	}
	return false
}

// hasDimOrGreySGRParams checks a split SGR parameter list for dim/grey attributes.
func hasDimOrGreySGRParams(params []string) bool {
	for i := 0; i < len(params); i++ {
		switch params[i] {
		case "2", "90": // dim attribute or bright-black (grey)
			return true
		case "38", "48": // extended fg/bg color — consume sub-params so the "5"/"2"
			// selector and color operands are not re-interpreted as standalone codes.
			grey, skip := consumeExtendedColor(params, i)
			if grey {
				return true
			}
			i += skip
		}
	}
	return false
}

// consumeExtendedColor parses an extended-color (38/48) parameter sequence at
// index i and returns (isFgGrey, paramsToSkip). Only 38 (foreground) sequences
// that resolve to a grey shade return true; 48 (background) sequences are
// consumed and skipped without signaling grey.
func consumeExtendedColor(params []string, i int) (bool, int) {
	if i+2 < len(params) && params[i+1] == "5" {
		n, err := strconv.Atoi(params[i+2])
		isFgGrey := params[i] == "38" && err == nil && isGreyIndex(n)
		return isFgGrey, 2
	}
	if i+4 < len(params) && params[i+1] == "2" {
		r, e1 := strconv.Atoi(params[i+2])
		g, e2 := strconv.Atoi(params[i+3])
		b, e3 := strconv.Atoi(params[i+4])
		allOK := e1 == nil && e2 == nil && e3 == nil
		isFgGrey := params[i] == "38" && allOK && isGreyRGB(r, g, b)
		return isFgGrey, 4
	}
	return false, 0
}

// isGreyIndex reports whether a 256-color palette index renders as grey: the
// dim palette grey (8) or any entry on the greyscale ramp (232–255).
func isGreyIndex(n int) bool {
	return n == 8 || (n >= 232 && n <= 255)
}

// isGreyRGB reports whether an (r,g,b) truecolor is a dim grey: the three
// channels are near-equal (so it is on the grey axis) and not bright (so plain
// white/near-white normal text is not misclassified as a ghost hint).
func isGreyRGB(r, g, b int) bool {
	maxc, minc := r, r
	for _, c := range []int{g, b} {
		if c > maxc {
			maxc = c
		}
		if c < minc {
			minc = c
		}
	}
	return maxc-minc <= 16 && (r+g+b)/3 < 180
}

// hasQueuedInput checks if the session has queued pasted text or user input
func hasQueuedInput(paneContent string) bool {
	// Claude and older harnesses use "[Pasted text"; current Codex collapses
	// large unsubmitted input into a "[Pasted Content N chars]" chip.
	if strings.Contains(paneContent, "[Pasted text") ||
		strings.Contains(paneContent, "[Pasted Content") {
		return true
	}

	// Look for "Press up to edit queued messages" pattern
	if strings.Contains(paneContent, "Press up to edit queued messages") {
		return true
	}

	return false
}

var (
	agmMessageHeaderPattern = regexp.MustCompile(`^\[From: ([A-Za-z0-9_-]{1,64}) \| ID: ([^ |\]]+) \| Sent: ([^|\]]+?)(?: \| Reply-To: ([^ |\]]+))?\]$`)
	agmMessageIDPattern     = regexp.MustCompile(`^(\d{13})-([A-Za-z0-9_-]{1,8})-(\d{3,})$`)
)

// ClassifyQueuedInput inspects pane content to determine whether queued input
// is a stuck AGM message or human-typed text. Returns the classification and
// a user-facing error message.
func ClassifyQueuedInput(paneContent string) (QueuedInputType, string) {
	if !hasQueuedInput(paneContent) {
		return QueuedInputNone, ""
	}

	// Bind identity to the most recent queued marker. A historical AGM header
	// elsewhere in the capture must not turn a newer human paste into AGM-owned
	// input. The generated header is always the first non-empty line after its
	// marker; any other intervening content makes the association unprovable.
	lines := strings.Split(paneContent, "\n")
	marker := -1
	for i, line := range lines {
		if hasQueuedInputMarker(line) {
			marker = i
		}
	}
	for _, line := range lines[marker+1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if sender, ok := parseCompleteAGMHeader(trimmed); ok {
			return QueuedInputAGM, fmt.Sprintf("session has queued AGM message (stuck paste-buffer from %s). Use agm send clear-input SESSION or retry with --force", sender)
		}
		break
	}

	return QueuedInputHuman, "session has human input in progress - not sending. Retry later"
}

func hasQueuedInputMarker(line string) bool {
	return strings.Contains(line, "[Pasted text") ||
		strings.Contains(line, "[Pasted Content") ||
		strings.Contains(line, "Press up to edit queued messages")
}

func parseCompleteAGMHeader(line string) (string, bool) {
	matches := agmMessageHeaderPattern.FindStringSubmatch(line)
	if matches == nil || !validGeneratedMessageID(matches[2], matches[1]) {
		return "", false
	}
	if _, err := time.Parse(time.RFC3339, strings.TrimSpace(matches[3])); err != nil {
		return "", false
	}
	// Reply-To is caller-supplied and follows the public message-ID contract,
	// which intentionally accepts legacy IDs that are not generator-shaped.
	if matches[4] != "" && !messages.ValidateMessageID(matches[4]) {
		return "", false
	}
	return matches[1], true
}

func validGeneratedMessageID(messageID, sender string) bool {
	matches := agmMessageIDPattern.FindStringSubmatch(messageID)
	if matches == nil {
		return false
	}
	if _, err := strconv.ParseInt(matches[1], 10, 64); err != nil {
		return false
	}
	if sender == "" {
		return true
	}
	senderShort := sender
	if len(senderShort) > 8 {
		senderShort = senderShort[:8]
	}
	return matches[2] == senderShort
}

// extractSender pulls the sender name from an AGM message header line.
// Format: [From: sender | ID: ... | Sent: ...]
func extractSender(headerLine string) string {
	after, found := strings.CutPrefix(headerLine, "[From: ")
	if !found {
		return "unknown"
	}
	if idx := strings.Index(after, " |"); idx > 0 {
		return after[:idx]
	}
	return "unknown"
}

// SendPromptLiteral sends prompt text to a tmux session using atomic paste-buffer,
// then sends Enter separately.
//
// Bug fix (2026-04-07): Switched from send-keys -l to load-buffer + paste-buffer.
// send-keys -l delivers text character-by-character through the terminal emulator.
// For large messages, the delay before Enter was insufficient — the terminal was
// still rendering when C-m arrived, causing Claude Code to treat the input as
// queued/pasted text ("Press up to edit queued messages") instead of submitting it.
// load-buffer + paste-buffer is atomic and eliminates this race condition entirely.
//
// Bug fix (2026-03-14): Added shouldInterrupt parameter to make ESC sending conditional.
// ESC interrupts Claude's thinking state, which should only happen when explicitly requested.
// When shouldInterrupt=false, prompts are queued instead of interrupting.
func SendPromptLiteral(target, prompt string, shouldInterrupt bool) error {
	return sendPromptLiteral(target, prompt, shouldInterrupt, "")
}

// SendPromptLiteralForHarness sends prompt text using the terminal paste
// semantics required by harness. AGY's composer enables bracketed paste and
// treats carriage returns as submissions, so tmux must preserve embedded line
// feeds (-r) and wrap the paste when requested by the application (-p).
func SendPromptLiteralForHarness(target, prompt string, shouldInterrupt bool, harness string) error {
	return sendPromptLiteral(target, prompt, shouldInterrupt, harness)
}

//nolint:gocyclo // reason: linear protocol — capture pane, optional ESC, load-buffer, paste-buffer, C-m, retry — extracting each step into a helper would obscure the linear flow.
func sendPromptLiteral(target, prompt string, shouldInterrupt bool, harness string) error {
	ctx := context.Background()
	socketPath := GetSocketPath()

	// Normalize session name to match tmux's conversion (dots/colons → dashes)
	normalizedTarget := NormalizeTmuxSessionName(target)

	// Acquire concurrency semaphore to prevent resource exhaustion
	// Bug fix (2026-04-02): SendPromptLiteral previously had no concurrency control,
	// allowing unbounded parallel send-keys operations that could exhaust fds.
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return fmt.Errorf("tmux concurrency limit reached: %w", err)
	}
	defer releaseTmuxSemaphore()

	// Lock tmux server for the entire send sequence (Escape → paste-buffer → C-m → retries).
	// Bug fix (2026-04-02): Without this lock, concurrent SendPromptLiteral calls on different
	// sessions could interleave their multi-step tmux command sequences at the server level,
	// causing stray bytes to leak across sessions and trigger copy-mode on unrelated sessions.
	// The lock serializes all tmux write operations, matching the pattern used by SendCommand.
	return withTmuxLock(func() error {
		// Step 0: Check if there's already text in the input box.
		// Uses ANSI capture (-e flag) so ghost-text detection can check for
		// the dim attribute (\x1b[2m) in the same buffer, eliminating the
		// two-capture race where pane state changes between captures.
		cmdCapture := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-e")
		output, err := cmdCapture.Output()
		if err != nil {
			return fmt.Errorf("failed to capture pane: %w", err)
		}

		ansiContent := string(output)
		// Non-blocking human_typing (the guard over-captures; see stash.go): stash
		// any stale/human input with the harness stash key, then deliver anyway.
		// Blocking on leftover text was the #1 cause of mesh deadlocks; the stash
		// preserves genuine human input (Claude Code auto-unstashes after the next
		// submit) and records over-capture via false-positive telemetry.
		stashStaleInputLocked(socketPath, normalizedTarget, ansiContent)
		// Queued paste artifacts ([Pasted text] / "Press up to edit queued
		// messages") are a SEPARATE guard from human_typing and still abort, so a
		// send does not pile onto an already-queued message. shouldInterrupt and
		// autonomous/force override this exactly as before.
		if !AutonomousMode() {
			if err := checkPaneForQueuedInput(ansiContent, shouldInterrupt); err != nil {
				return err
			}
		}

		// Step 1: Conditionally send ESC to interrupt thinking state (Bug fix: only if shouldInterrupt=true)
		// When shouldInterrupt=false, prompts are queued instead of interrupting active operations.
		// This fixes Bug 2 where ESC was unconditionally sent, interrupting operations.
		if shouldInterrupt {
			// Send ESC to interrupt any thinking state
			// This prevents prompts from being queued as "pasted text"
			cmdEsc := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "Escape")
			if err := cmdEsc.Run(); err != nil {
				return fmt.Errorf("failed to send Escape: %w", err)
			}

			// Wait for session to process ESC
			time.Sleep(500 * time.Millisecond)
		}

		// Step 2: Load prompt text into tmux paste buffer, then paste atomically.
		// Bug fix (2026-04-07): Replaced send-keys -l with load-buffer + paste-buffer.
		// send-keys -l sends text character-by-character, creating a race condition where
		// Enter (C-m) can arrive before the terminal finishes rendering. paste-buffer is
		// atomic — the entire text appears in the input at once, eliminating the race.
		// This matches the reliable pattern used by SendCommand.

		// Ensure buffer is cleaned up on any error path
		bufferLoaded := false
		defer func() {
			if bufferLoaded {
				deleteBuffer()
			}
		}()

		timeout := getAdaptiveTimeout()
		cmdLoad, cancel1 := CommandWithTimeout(ctx, timeout, "tmux", "-S", socketPath, "load-buffer", "-b", "agm-cmd", "-")
		defer cancel1()

		stdin, err := cmdLoad.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe for load-buffer: %w", err)
		}

		if err := cmdLoad.Start(); err != nil {
			return fmt.Errorf("failed to start load-buffer: %w", err)
		}

		if _, err := stdin.Write([]byte(prompt)); err != nil {
			stdin.Close()
			cmdLoad.Wait()
			return fmt.Errorf("failed to write to load-buffer stdin: %w", err)
		}
		stdin.Close()

		if err := cmdLoad.Wait(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux load-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to load prompt into tmux buffer: %w", err)
		}
		bufferLoaded = true

		// Paste buffer to session atomically. AGY needs raw bracketed paste: tmux's
		// default LF-to-CR conversion submits the attribution header as a complete
		// request and leaves the message body behind in the composer.
		cmdPaste, cancel2 := CommandWithTimeout(ctx, timeout, "tmux", pasteBufferArgs(socketPath, normalizedTarget, harness)...)
		defer cancel2()
		if err := cmdPaste.Run(); err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return &TimeoutError{
					Problem:  fmt.Sprintf("tmux paste-buffer timed out after %v (server may be hung)", timeout),
					Recovery: "  pkill -9 tmux    # Kill hung tmux server\n  agm session list         # Verify recovery",
					Duration: timeout,
				}
			}
			return fmt.Errorf("failed to paste buffer to tmux session: %w", err)
		}
		bufferLoaded = false // paste-buffer -d already deleted it

		// Step 3: Send Enter reliably using hex 0x0d instead of C-m.
		// sendEnterReliable waits 100ms, sends -H 0d, then auto-retries
		// once if the Enter didn't register (replaces the old 50ms + C-m +
		// retryEnterAfterPaste sequence).
		if err := sendEnterReliable(socketPath, normalizedTarget); err != nil {
			return err
		}

		verifyAndResubmitQueuedPrompt(socketPath, normalizedTarget)

		if os.Getenv("AGM_DEBUG") == "1" {
			hash := sha256.Sum256([]byte(prompt))
			slog.Debug("Sent prompt", "hash", fmt.Sprintf("%x", hash[:8]), "length", len(prompt), "source", "--prompt")
		}

		return nil
	})
}

func pasteBufferArgs(socketPath, target, harness string) []string {
	return pasteBufferArgsForDelivery(socketPath, target, harness, false)
}

func pasteBufferArgsForDelivery(socketPath, target, harness string, rawBracketedPaste bool) []string {
	return pasteBufferArgsForNamedDelivery(socketPath, "agm-cmd", target, harness, rawBracketedPaste)
}

func pasteBufferArgsForNamedDelivery(socketPath, bufferName, target, harness string, rawBracketedPaste bool) []string {
	return []string{"-S", socketPath, "paste-buffer", "-b", bufferName, "-t", target, pasteBufferDeleteFlag(harness, rawBracketedPaste)}
}

func pasteBufferDeleteFlag(harness string, rawBracketedPaste bool) string {
	if harness == "agy" || rawBracketedPaste {
		return "-dpr"
	}
	return "-d"
}

// checkPaneForQueuedInput examines an ANSI pane capture and returns an error if
// the input box already holds queued paste text ([Pasted text] / "Press up to
// edit queued messages"), so a send does not pile onto an already-queued
// message. shouldInterrupt=true short-circuits all checks.
//
// This deliberately no longer aborts on plain "input line has content" — that
// was the human_typing block, which over-captured and is now handled
// non-blockingly by stashStaleInputLocked (see stash.go). Queued-paste
// detection is a separate guard and is retained.
//
// The caller provides ANSI content (captured with -e flag) so detection
// operates on a single snapshot, eliminating the two-capture race (ce-v9in retro).
func checkPaneForQueuedInput(ansiContent string, shouldInterrupt bool) error {
	if shouldInterrupt {
		return nil
	}
	plainContent := stripANSI(ansiContent)
	if hasActiveSpinner(plainContent) {
		return nil
	}
	if hasQueuedInput(plainContent) {
		_, msg := ClassifyQueuedInput(plainContent)
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// verifyAndResubmitQueuedPrompt is Step 4 of SendPromptLiteral: detect when a
// session was busy at submit time so the prompt remained queued, and re-send
// Enter once the prompt returns. Up to 5 retries.
func verifyAndResubmitQueuedPrompt(socketPath, normalizedTarget string) {
	for retry := 0; retry < 5; retry++ {
		time.Sleep(500 * time.Millisecond)
		cmdCheck := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-e", "-J", "-S", "-5")
		checkOutput, err := cmdCheck.Output()
		if err != nil {
			return
		}
		checkContent := string(checkOutput)
		if !hasQueuedInput(stripANSI(checkContent)) {
			return
		}
		if !containsAnyHarnessPromptPattern(checkContent) {
			continue
		}
		if os.Getenv("AGM_DEBUG") == "1" {
			slog.Debug("Detected queued [Pasted text] at prompt — re-sending Enter", "retry", retry+1)
		}
		cmdResubmit := exec.Command("tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, "-H", "0d")
		_ = cmdResubmit.Run()
		time.Sleep(300 * time.Millisecond)
		cmdVerify := exec.Command("tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-e", "-J", "-S", "-5")
		verifyOutput, err := cmdVerify.Output()
		if err == nil && !hasQueuedInput(stripANSI(string(verifyOutput))) {
			return
		}
	}
}

// SendPromptFromFile sends the contents of filePath to the tmux target as a
// prompt, optionally interrupting any running command first.
func SendPromptFromFile(target, filePath string, shouldInterrupt bool) error {
	// Validate file exists and get size
	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("prompt file not found: %s", filePath)
	}

	// Enforce 10KB size limit
	if stat.Size() > maxPromptFileSize {
		return fmt.Errorf("prompt file too large: %d bytes (max 10KB)", stat.Size())
	}

	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read prompt file: %w", err)
	}

	if os.Getenv("AGM_DEBUG") == "1" {
		hash := sha256.Sum256(content)
		slog.Debug("Sent prompt", "hash", fmt.Sprintf("%x", hash[:8]), "length", len(content), "source", "--prompt-file "+filePath)
	}

	// Send using literal mode with conditional interrupt
	return SendPromptLiteral(target, string(content), shouldInterrupt)
}
