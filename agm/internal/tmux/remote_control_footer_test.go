package tmux

import (
	"strings"
	"testing"
)

// osc8 builds the hyperlink escape sequence Claude Code uses for the remote
// control affordance: OSC 8 with an id, the visible label, then the closing
// empty-target OSC 8.
func osc8(id, uri, label string) string {
	return "\x1b]8;id=" + id + ";" + uri + "\x1b\\" + label + "\x1b]8;;\x1b\\"
}

// remoteControlRow reproduces the row captured verbatim from a live AGM-managed
// pane: a right-aligned "/rc" link to the session on claude.ai, rendered below
// the status footer.
var remoteControlRow = strings.Repeat(" ", 75) +
	osc8("18yq9i7", "https://claude.ai/code/session_01GPbVR6KfUoV5JNH8sPngiQ?from=cli", "/rc")

// claudeIdleComposerWithRemoteControl is the tail of a live, idle, authenticated
// Claude pane started by AGM.
var claudeIdleComposerWithRemoteControl = strings.Join([]string{
	"",
	strings.Repeat("─", 80),
	"❯ ",
	strings.Repeat("─", 80),
	"  vbonnet@mac:/Users/vbonnet/.agm/sandboxes/796516c8-57ec-415b-9a7c-a4e0f1d1f…",
	"  ⏸ plan mode on (shift+tab to cycle) · ← for agents",
	remoteControlRow,
	"",
}, "\n")

// The bring-up regression: an authenticated Claude renders the remote control
// link below its status footer. That row is static chrome, but it was
// unrecognised, so the composer never counted as owning the tail and AGM waited
// out its whole budget on a session that was already live. It never reproduced
// from an unauthenticated launch, where the link is absent.
func TestClaudeComposerFooterChromeAcceptsRemoteControlLink(t *testing.T) {
	if !isClaudeComposerFooterChrome(remoteControlRow) {
		t.Error("isClaudeComposerFooterChrome() rejected the remote control link row")
	}
}

func TestTailOwnedComposerWithRemoteControlLink(t *testing.T) {
	if !hasTailOwnedClaudeComposer(claudeIdleComposerWithRemoteControl) {
		t.Error("hasTailOwnedClaudeComposer() = false for a live idle authenticated composer")
	}
}

// Only a link to Claude's own session surface is chrome. A hyperlink to
// anywhere else below the composer is content, and content means a turn is
// running.
func TestClaudeComposerFooterChromeRejectsForeignHyperlink(t *testing.T) {
	row := "  " + osc8("x", "https://example.com/build/42", "build #42")
	if isClaudeComposerFooterChrome(row) {
		t.Error("isClaudeComposerFooterChrome() accepted a hyperlink to an unrelated host")
	}
}

// A link that shares the line with other text is part of rendered output, not
// the footer's standalone affordance.
func TestClaudeComposerFooterChromeRejectsLinkBesideText(t *testing.T) {
	row := "  Opened " + osc8("x", "https://claude.ai/code/session_abc", "/rc") + " while running tests"
	if isClaudeComposerFooterChrome(row) {
		t.Error("isClaudeComposerFooterChrome() accepted a link embedded in output text")
	}
}

// The label alone must not qualify: without the hyperlink escape, "/rc" is just
// text and could be model output.
func TestClaudeComposerFooterChromeRejectsBareLabel(t *testing.T) {
	if isClaudeComposerFooterChrome(strings.Repeat(" ", 75) + "/rc") {
		t.Error("isClaudeComposerFooterChrome() accepted a bare /rc label with no hyperlink")
	}
}

// The existing fail-closed guarantees still hold with a remote control link on
// screen: a running turn is not idle chrome (ce-wn4qe).
func TestRemoteControlLinkDoesNotMaskRunningTurn(t *testing.T) {
	running := claudeIdleComposerWithRemoteControl + "\n  ✻ Thinking… (esc to interrupt)"
	if hasTailOwnedClaudeComposer(running) {
		t.Error("hasTailOwnedClaudeComposer() = true while a turn is running")
	}
}
