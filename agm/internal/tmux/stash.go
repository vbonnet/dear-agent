package tmux

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// --- Non-blocking human_typing stash ---
//
// The human_typing guard used to BLOCK a send whenever it saw non-ghost text
// after the ❯ prompt. It over-captures badly: stale scrollback, old prompts,
// ghost-text remnants, and AGM's own un-submitted paste all read as "a human is
// typing", and blocking was the #1 cause of VROOM mesh stalls and reaper/
// dispatch failures (the ce-v9in family).
//
// The guard is now advisory only. Instead of blocking, we STASH the composer's
// current input with the harness's stash key and deliver anyway. Claude Code's
// C-s stashes the draft and auto-unstashes it once the next message is
// submitted, so genuine human input is preserved and reinstated after AGM's
// message goes through — never destroyed (unlike a C-u clear) and never a block.
//
// Because the guard over-captures, we also MEASURE it: after sending the stash
// we re-read the input line. If it cleared, real input was set aside (true
// positive). If it is unchanged, the stash did nothing and the "typing" was a
// false positive. Both outcomes flow to telemetry so the heuristic can be
// improved later. See internal/telemetry human_typing.* counters.

// stashHarness records the harness of the session currently being sent to, so
// the stash step can pick the correct stash key. It is a process-global set by
// the CLI layer per send, mirroring autonomousMode / forceDelivery: agm is a
// single-shot CLI (one invocation == one send). The empty default is treated as
// claude-code, preserving historical behavior for legacy callers and the
// agm-daemon (which drives Claude Code sessions).
//
// Known limitation (shared with autonomousMode / forceDelivery): a parallel
// multi-recipient send to sessions on different harnesses races on this global.
// Threading the harness through every Send* signature is a follow-up.
var stashHarness atomic.Value // string

// SetStashHarness records the recipient harness for the pending send so the
// non-blocking human_typing stash uses that harness's stash key.
func SetStashHarness(h string) { stashHarness.Store(h) }

// StashHarness returns the recipient harness recorded for the pending send
// (empty string == unset, treated as claude-code).
func StashHarness() string {
	if v, ok := stashHarness.Load().(string); ok {
		return v
	}
	return ""
}

// normalizeStashHarness canonicalizes a harness name for the stash-key registry
// and telemetry tags. Empty/claude aliases resolve to claude-code.
func normalizeStashHarness(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case "", "claude", "claude-code":
		return "claude-code"
	case "codex", "codex-cli":
		return "codex-cli"
	case "agy", "antigravity":
		return "agy"
	case "opencode", "opencode-cli":
		return "opencode-cli"
	case "pi", "pi-cli":
		return "pi-cli"
	default:
		return strings.ToLower(strings.TrimSpace(h))
	}
}

// StashKeyForHarness returns the tmux key that stashes the input composer for a
// harness, and whether that mapping is verified. Only Claude Code's C-s is
// confirmed: it stashes the draft and auto-unstashes it after the next submit,
// so genuine human input survives. Other harnesses default to C-s as a
// best-effort until their real draft/stash key is confirmed (follow-up). C-s is
// byte 0x13 (XOFF) which only freezes a cooked-mode terminal; every harness
// composer runs a raw-mode TUI, so it is delivered as an ordinary keypress. The
// unverified flag is recorded in telemetry so the mapping can be validated from
// production data.
//
// The key return is C-s for every harness today; the registry is deliberately
// shaped to return a per-harness key so that, once codex-cli/agy/opencode-cli/pi-cli
// draft keys are confirmed, non-Claude harnesses return their own key here.
//
//nolint:unparam // key intentionally uniform today; registry shape is future-proofing (see doc above).
func StashKeyForHarness(h string) (key string, verified bool) {
	switch normalizeStashHarness(h) {
	case "claude-code":
		return "C-s", true
	default:
		return "C-s", false
	}
}

// stashTmuxTimeout bounds each raw tmux subprocess issued by the stash step.
// The stash runs while holding the tmux server lock, so an unbounded exec
// against a hung tmux server would stall every other send behind the lock —
// exactly the mesh-wide stall the non-blocking stash exists to prevent.
const stashTmuxTimeout = 5 * time.Second

// StashOutcome reports what happened when the non-blocking human_typing stash
// ran before a send. It feeds telemetry and test assertions.
type StashOutcome struct {
	Attempted     bool   // input held non-ghost content, so a stash was warranted
	Sent          bool   // the stash key was actually delivered to the pane
	Cleared       bool   // the input line was empty after the stash (real input set aside)
	FalsePositive bool   // stash sent but the input line was unchanged → over-capture
	Harness       string // normalized recipient harness
	Verified      bool   // whether the harness's stash key is a confirmed mapping
}

// shouldClearStaleInput reports whether the input box holds stashable stale
// text: real (non-ghost) content after the ❯ prompt. Ghost text (Claude Code's
// dim placeholder, \x1b[2m) is excluded — there is nothing real to stash and it
// never blocks submission. Pure decision helper (no tmux I/O) for testability.
func shouldClearStaleInput(ansiContent string) bool {
	return InputLineHasContent(stripANSI(ansiContent)) && !HasGhostTextInANSI(ansiContent)
}

// inputLineAfterPrompt returns the trimmed text after the last ❯ prompt in a
// plain-text pane capture, or "" if the input line is empty or absent.
func inputLineAfterPrompt(paneContent string) string {
	lines := strings.Split(paneContent, "\n")
	for _, line := range slices.Backward(lines) {
		if _, after, found := strings.Cut(line, "❯"); found {
			return strings.TrimSpace(after)
		}
	}
	return ""
}

// stashStaleInputLocked implements the non-blocking human_typing behavior: when
// the input composer holds real (non-ghost) text, stash it with the harness
// stash key and let delivery proceed, rather than blocking as the guard used
// to. It is a no-op (Attempted=false) when the composer is empty or holds only
// ghost text.
//
// It also measures over-capture: the input line is compared before and after
// the stash. Cleared → real input was set aside; unchanged → the stash did
// nothing, so the detection was a false positive. The outcome is emitted to
// telemetry and returned.
//
// MUST be called while already holding the tmux server lock (it issues raw
// send-keys/capture-pane via exec rather than re-entering withTmuxLock, which is
// not reentrant). socketPath and normalizedTarget must be pre-resolved.
func stashStaleInputLocked(socketPath, normalizedTarget, ansiContent string) StashOutcome {
	outcome := StashOutcome{Harness: normalizeStashHarness(StashHarness())}
	// Never stash while the harness is actively generating: text after ❯ during a
	// spinner is AI output mid-render, not stashable input, and sending a stash
	// key then would be wrong (2026-04-10 spinner fix). Skip as a no-op.
	if hasActiveSpinner(stripANSI(ansiContent)) {
		return outcome
	}
	if !shouldClearStaleInput(ansiContent) {
		return outcome // empty or ghost text — nothing real to stash
	}
	outcome.Attempted = true
	key, verified := StashKeyForHarness(StashHarness())
	outcome.Verified = verified

	before := inputLineAfterPrompt(stripANSI(ansiContent))

	// Bound both raw tmux execs (see stashTmuxTimeout). No caller threads a
	// ctx into the send path (SendPromptLiteral starts from context.Background()),
	// so the deadline is derived locally.
	ctx, cancel := context.WithTimeout(context.Background(), stashTmuxTimeout)
	defer cancel()

	if err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "send-keys", "-t", normalizedTarget, key).Run(); err != nil {
		return outcome // send failed or timed out; nothing to record beyond the attempt
	}
	outcome.Sent = true
	// Give the harness a moment to re-render the now-(maybe-)empty input line.
	time.Sleep(50 * time.Millisecond)

	// Re-read the input line to classify the outcome. If the capture fails or
	// the deadline hit, the outcome is UNKNOWN: leave Cleared and FalsePositive
	// both false rather than defaulting after to before, which would misreport
	// a capture failure as a human_typing false positive in telemetry.
	if out, err := exec.CommandContext(ctx, "tmux", "-S", socketPath, "capture-pane", "-t", normalizedTarget, "-p", "-e").Output(); err == nil && ctx.Err() == nil {
		switch inputLineAfterPrompt(stripANSI(string(out))) {
		case "":
			outcome.Cleared = true // real input was stashed
		case before:
			outcome.FalsePositive = true // stash did nothing → human_typing over-captured
		}
	}

	telemetry.RecordHumanTypingStash(context.Background(), outcome.Harness,
		outcome.Verified, outcome.Sent, outcome.Cleared, outcome.FalsePositive)
	return outcome
}
