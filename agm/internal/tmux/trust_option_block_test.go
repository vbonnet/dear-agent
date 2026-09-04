package tmux

import (
	"strings"
	"testing"
)

// realTrustDialogPrefix is the wrapped prose Claude Code 2.1.234 renders above
// the trust dialog's option rows, captured verbatim from a live tmux pane at
// 120 columns. The question and the permission summary both wrap, which is why
// the option-block recognizer must not depend on the prose body's shape.
const realTrustDialogPrefix = ` /Users/vbonnet/.agm/sandboxes/142f6b6f-80ef-47c7-a0e2-e40fb38c507c/upper/repo0

 Quick safety check: Is this a project you created or one you trust? (Like your own code, a well-known open source
 project, or work from your team). If not, take a moment to review what's in this folder first.

 Claude Code'll be able to read, edit, and execute files here.

 ⚠ This folder pre-approves 59 tool permissions in .claude/settings.json and .claude/settings.local.json:
   WebSearch, WebFetch, Bash(resolve-review-threads *), Bash(BEADS_DIR=/Users/vbonnet/beads/context-engine/.beads bd…,
 Bash(GIT_TERMINAL_PROMPT=0 gtimeout 30 git -C ~/src/engram-r…, Bash(curl -sf http://localhost:16686/api/services),
 Bash(git status), Bash(git status *), and 51 more
 These will apply without asking. Only proceed if you trust this configuration.

 Security guide
`

// realTrustDialogNegativeSelected is the live dialog exactly as it first
// renders: unnumbered options, "No, exit" first and selected by default.
const realTrustDialogNegativeSelected = realTrustDialogPrefix + `
 ❯ No, exit
   Yes, I trust this folder

 Enter to confirm · Esc to cancel`

// realTrustDialogAffirmativeSelected is the same dialog after one Down press.
const realTrustDialogAffirmativeSelected = realTrustDialogPrefix + `
   No, exit
 ❯ Yes, I trust this folder

 Enter to confirm · Esc to cancel`

func TestFindClaudeTrustOptionBlockNegativeSelected(t *testing.T) {
	lines := strings.Split(realTrustDialogNegativeSelected, "\n")
	block, ok := findClaudeTrustOptionBlock(lines)
	if !ok {
		t.Fatal("findClaudeTrustOptionBlock() did not recognize the live trust dialog")
	}
	if block.selectorIndex != block.negativeIndex {
		t.Errorf("selectorIndex = %d, want negativeIndex %d", block.selectorIndex, block.negativeIndex)
	}
	if block.affirmativeIndex <= block.negativeIndex {
		t.Errorf("affirmativeIndex = %d, want below negativeIndex %d", block.affirmativeIndex, block.negativeIndex)
	}
}

func TestFindClaudeTrustOptionBlockAffirmativeSelected(t *testing.T) {
	lines := strings.Split(realTrustDialogAffirmativeSelected, "\n")
	block, ok := findClaudeTrustOptionBlock(lines)
	if !ok {
		t.Fatal("findClaudeTrustOptionBlock() did not recognize the answered trust dialog")
	}
	if block.selectorIndex != block.affirmativeIndex {
		t.Errorf("selectorIndex = %d, want affirmativeIndex %d", block.selectorIndex, block.affirmativeIndex)
	}
}

// The recognizer must reject a dialog whose tail has been taken over by newer
// content: answering there could land Enter on whatever owns input now.
func TestFindClaudeTrustOptionBlockRejectsSupersededTail(t *testing.T) {
	superseded := realTrustDialogNegativeSelected + "\n\n❯ \n\n  ⏵⏵ auto mode on (shift+tab to cycle)"
	if _, ok := findClaudeTrustOptionBlock(strings.Split(superseded, "\n")); ok {
		t.Error("findClaudeTrustOptionBlock() accepted a tail owned by newer content")
	}
}

// Option rows without the trust question above them are not the trust dialog.
func TestFindClaudeTrustOptionBlockRequiresQuestion(t *testing.T) {
	noQuestion := " ❯ No, exit\n   Yes, I trust this folder\n\n Enter to confirm · Esc to cancel"
	if _, ok := findClaudeTrustOptionBlock(strings.Split(noQuestion, "\n")); ok {
		t.Error("findClaudeTrustOptionBlock() accepted option rows without the trust question")
	}
}

// A capture where only one of the two options is present is a partial redraw,
// not an answerable dialog.
func TestFindClaudeTrustOptionBlockRequiresBothOptions(t *testing.T) {
	partial := realTrustDialogPrefix + "\n ❯ No, exit\n\n Enter to confirm · Esc to cancel"
	if _, ok := findClaudeTrustOptionBlock(strings.Split(partial, "\n")); ok {
		t.Error("findClaudeTrustOptionBlock() accepted a partially rendered option block")
	}
}

// Both rows carrying a selector marker is an ambiguous capture (a torn redraw);
// refuse rather than guess which one owns input.
func TestFindClaudeTrustOptionBlockRejectsDoubleSelector(t *testing.T) {
	doubled := realTrustDialogPrefix + "\n ❯ No, exit\n ❯ Yes, I trust this folder\n\n Enter to confirm · Esc to cancel"
	if _, ok := findClaudeTrustOptionBlock(strings.Split(doubled, "\n")); ok {
		t.Error("findClaudeTrustOptionBlock() accepted a capture with two selector markers")
	}
}

func TestClassifyTrustDialogOwnershipRecognizesCurrentLayout(t *testing.T) {
	if got := classifyTrustDialogOwnership(realTrustDialogNegativeSelected); got != claudeTrustNegativeSelected {
		t.Errorf("classifyTrustDialogOwnership(negative) = %v, want claudeTrustNegativeSelected", got)
	}
	if got := classifyTrustDialogOwnership(realTrustDialogAffirmativeSelected); got != claudeTrustAffirmativeSelected {
		t.Errorf("classifyTrustDialogOwnership(affirmative) = %v, want claudeTrustAffirmativeSelected", got)
	}
}

// The live dialog must block readiness: before this fix TrustDialogOwnsInput
// returned false for it, so AGM waited for a composer that the dialog was
// holding back and timed out after 90s.
func TestTrustDialogOwnsInputForCurrentLayout(t *testing.T) {
	if !TrustDialogOwnsInput(realTrustDialogNegativeSelected) {
		t.Error("TrustDialogOwnsInput() = false for the live unnumbered trust dialog")
	}
	if TrustSelectorOwnsInput(realTrustDialogNegativeSelected) {
		t.Error("TrustSelectorOwnsInput() = true while the negative option is selected")
	}
	if !TrustSelectorOwnsInput(realTrustDialogAffirmativeSelected) {
		t.Error("TrustSelectorOwnsInput() = false while the affirmative option is selected")
	}
}

func TestTrustAffirmativeNavigationKey(t *testing.T) {
	key, ok := trustAffirmativeNavigationKey(realTrustDialogNegativeSelected)
	if !ok {
		t.Fatal("trustAffirmativeNavigationKey() reported no move for a negative-selected dialog")
	}
	// The live dialog lists "No, exit" first, so the affirmative row is below
	// the selector: Down, not Up.
	if key != "Down" {
		t.Errorf("trustAffirmativeNavigationKey() = %q, want \"Down\"", key)
	}
	if _, ok := trustAffirmativeNavigationKey(realTrustDialogAffirmativeSelected); ok {
		t.Error("trustAffirmativeNavigationKey() proposed a move with the affirmative already selected")
	}
}

// Legacy numbered layouts put "Yes" above "No", so the same helper has to
// resolve to Up there.
func TestTrustAffirmativeNavigationKeyLegacyOrdering(t *testing.T) {
	legacy := " Do you trust the files in this folder?\n\n   1. Yes, I trust this folder\n ❯ 2. No, exit\n\n Enter to confirm · Esc to cancel"
	key, ok := trustAffirmativeNavigationKey(legacy)
	if !ok {
		t.Fatal("trustAffirmativeNavigationKey() reported no move for the legacy numbered layout")
	}
	if key != "Up" {
		t.Errorf("trustAffirmativeNavigationKey() = %q, want \"Up\"", key)
	}
}
