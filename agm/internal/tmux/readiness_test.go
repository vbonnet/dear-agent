package tmux

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/vbonnet/dear-agent/agm/internal/lock"
)

func TestHandleHarnessStartupStateWaitsForSlowInitialProcess(t *testing.T) {
	t.Parallel()

	observedHarness := false
	advanced := make(map[string]bool)
	for attempt := range 12 {
		ready, err := handleHarnessStartupState(context.Background(), "slow", "codex-cli", HarnessInputReadiness{State: HarnessInputWrongHarness}, &observedHarness, advanced)
		if err != nil || ready {
			t.Fatalf("pre-start attempt %d = ready:%t err:%v, want continued wait", attempt, ready, err)
		}
	}
	ready, err := handleHarnessStartupState(context.Background(), "slow", "codex-cli", HarnessInputReadiness{State: HarnessInputBusy}, &observedHarness, advanced)
	if err != nil || ready || !observedHarness {
		t.Fatalf("observed busy harness = ready:%t observed:%t err:%v", ready, observedHarness, err)
	}
	_, err = handleHarnessStartupState(context.Background(), "slow", "codex-cli", HarnessInputReadiness{State: HarnessInputWrongHarness}, &observedHarness, advanced)
	if err == nil || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("post-start wrong harness error = %v, want stopped-process failure", err)
	}
}

func TestHandleHarnessStartupStateFailsHookReviewWithoutAdvancing(t *testing.T) {
	t.Parallel()

	observedHarness := false
	advanced := make(map[string]bool)
	ready, err := handleHarnessStartupState(context.Background(), "hooks", "codex-cli", HarnessInputReadiness{
		State:   HarnessInputReviewRequired,
		Content: "Hooks need review",
	}, &observedHarness, advanced)
	if ready {
		t.Fatal("hook review startup reported ready")
	}
	if !errors.Is(err, ErrCodexHookReviewRequired) {
		t.Fatalf("hook review startup error = %v, want ErrCodexHookReviewRequired", err)
	}
	if !observedHarness {
		t.Fatal("hook review did not record the observed Codex harness")
	}
	if len(advanced) != 0 {
		t.Fatalf("hook review advanced transitions = %v, want none", advanced)
	}
}

func TestHandleClaudeStartupReprovesTrustBeforeAdvancing(t *testing.T) {
	t.Parallel()

	readiness := HarnessInputReadiness{
		State:   HarnessInputOnboarding,
		Content: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit",
	}
	t.Run("selector moved to No before atomic probe", func(t *testing.T) {
		t.Parallel()
		observed := false
		advanced := make(map[string]bool)
		calls := 0
		ready, err := handleHarnessStartupStateWithProbe(t.Context(), "session", "claude-code", readiness, &observed, advanced,
			func(_ context.Context, session string, autoAnswer bool) (ClaudeInputProbe, error) {
				calls++
				if session != "session" || !autoAnswer {
					t.Fatalf("probe arguments = %q, %t", session, autoAnswer)
				}
				return ClaudeInputProbe{DialogOwnsInput: true}, nil
			})
		if err != nil || ready {
			t.Fatalf("result = ready:%t err:%v", ready, err)
		}
		if calls != 1 || !observed || len(advanced) != 0 {
			t.Fatalf("calls=%d observed=%t advanced=%v", calls, observed, advanced)
		}
	})

	t.Run("atomic probe alone records a completed trust transition", func(t *testing.T) {
		t.Parallel()
		observed := false
		advanced := make(map[string]bool)
		_, err := handleHarnessStartupStateWithProbe(t.Context(), "session", "claude-code", readiness, &observed, advanced,
			func(context.Context, string, bool) (ClaudeInputProbe, error) {
				return ClaudeInputProbe{DialogOwnsInput: true, TrustAnswered: true}, nil
			})
		if err != nil {
			t.Fatal(err)
		}
		if !advanced[HarnessInputOnboarding+":trust"] {
			t.Fatalf("advanced transitions = %v", advanced)
		}
	})
}

func TestClassifyHarnessInputRequiresCurrentHarnessComposer(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		content string
		ready   bool
		state   string
	}{
		{
			name:    "structural Codex composer",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.5 /model to change │\n›",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "current Codex welcome ghost composer",
			harness: "codex-cli",
			content: "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n\x1b[2m│ model: \x1b[0mgpt-5.5 high\x1b[2m \x1b[0m/model to change │\nTo get started, describe a task or try /review\n\n\x1b[1m›\x1b[0m \x1b[2mRun /review on my current changes\x1b[0m\n\ngpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "current Codex welcome human draft",
			harness: "codex-cli",
			content: "\x1b[2m│ >_ \x1b[0;1mOpenAI Codex\x1b[0;2m (v0.145.0) │\x1b[0m\n\x1b[2m│ model: \x1b[0mgpt-5.5 high\x1b[2m \x1b[0m/model to change │\n\x1b[1m›\x1b[0m Run /review on my current changes\n\ngpt-5.5 high · ~/src/project",
			state:   HarnessInputBusy,
		},
		{
			name:    "current Codex post-turn composer",
			harness: "codex-cli",
			content: "»\n\ngpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "current Codex post-turn human draft",
			harness: "codex-cli",
			content: "» preserve this human draft\n\ngpt-5.5 high · ~/.agm/sandboxes/example/merged/repo0",
			state:   HarnessInputBusy,
		},
		{
			name:    "Codex update selector owns input",
			harness: "codex-cli",
			content: "✨ Update available! 0.145.0 -> 0.146.0\n› 1. Update now (runs `brew upgrade --cask codex`)\n  2. Skip\n  3. Skip until next version\nPress enter to continue",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Codex model footer without input owner",
			harness: "codex-cli",
			content: "Working (12s)\ngpt-5.5 xhigh · /repo",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale Codex header without input owner",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.5 /model to change │\nWorking (12s)",
			state:   HarnessInputBusy,
		},
		{
			name:    "Codex trust owns input",
			harness: "codex-cli",
			content: "Do you trust the contents of this directory?\n› 1. Yes, continue",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Codex hook review requires operator",
			harness: "codex-cli",
			content: "Hooks need review\n4 hooks are new or changed.\nHooks can run outside the sandbox after you trust them.\n› 1. Review hooks\n  2. Trust all and continue\n  3. Continue without trusting (hooks won't run)\nPress enter to confirm or esc to go back" + strings.Repeat("\n", 18),
			state:   HarnessInputReviewRequired,
		},
		{
			name:    "Codex active hooks dashboard overrides retained composer",
			harness: "codex-cli",
			content: "Hooks\nLifecycle hooks from config and enabled plugins.\n⚠ 11 hooks need review before they can run.\nEvent Installed Active Review Description\nPress t to trust all; enter to review hooks; esc to close\n│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 high /model to change │\n›\ngpt-5.6 high · ~/src/project\nHooks\nLifecycle hooks from config and enabled plugins.\n⚠ 11 hooks need review before they can run.\nEvent Installed Active Review Description\nPress t to trust all; enter to review hooks; esc to close",
			state:   HarnessInputReviewRequired,
		},
		{
			name:    "Codex closed hooks dashboard yields to newer composer",
			harness: "codex-cli",
			content: "Hooks\nLifecycle hooks from config and enabled plugins.\n⚠ 11 hooks need review before they can run.\nEvent Installed Active Review Description\nPress t to trust all; enter to review hooks; esc to close\n│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 high /model to change │\n›\ngpt-5.6 high · ~/src/project",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Codex closed hooks dashboard yields to occupied composer",
			harness: "codex-cli",
			content: "Hooks\nLifecycle hooks from config and enabled plugins.\n⚠ 11 hooks need review before they can run.\nEvent Installed Active Review Description\nPress t to trust all; enter to review hooks; esc to close\n› preserve this human draft\ngpt-5.6 high · ~/src/project",
			state:   HarnessInputBusy,
		},
		{
			name:    "Codex closed hooks dashboard yields to working turn",
			harness: "codex-cli",
			content: "Hooks\nLifecycle hooks from config and enabled plugins.\n⚠ 11 hooks need review before they can run.\nEvent Installed Active Review Description\nPress t to trust all; enter to review hooks; esc to close\n› inspect the release\n• Working (3s • esc to interrupt)\ngpt-5.6 high · ~/src/project",
			state:   HarnessInputProcessing,
		},
		{
			name:    "resolved Codex onboarding before live composer",
			harness: "codex-cli",
			content: "Do you trust the contents of this directory?\n› 1. Yes, continue\napproved\n│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.5 /model to change │\n›",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Claude trust owns input before generic permission choices",
			harness: "claude-code",
			content: "Do you trust the files in this folder?\n❯ 1. Yes, proceed\n  2. No, exit",
			state:   HarnessInputOnboarding,
		},
		{
			// ce-wn4qe: current Claude Code (2.x) trust wording. The classifier
			// must report Onboarding (not a false ready and not a hang) so the
			// shared readiness path auto-answers it with Enter.
			name:    "Claude current-wording trust owns input",
			harness: "claude-code",
			content: " Quick safety check: Is this a project you created or one you trust?\n\n ⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n Read, Bash(git status)\n These will apply without asking. Only proceed if you trust this configuration.\n\n ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			state:   HarnessInputOnboarding,
		},
		{
			// A paired composer draft is not trust authority without the question;
			// generic confirmation chrome still keeps it fail-closed as permission.
			name:    "Claude unanchored paired trust-looking rows are not onboarding",
			harness: "claude-code",
			content: " ⚠ This folder pre-approves 20 tool permissions in .claude/settings.local.json:\n These will apply without asking. Only proceed if you trust this configuration.\n\n ❯ 1. Yes, I trust this folder\n   2. No, exit\n\n Enter to confirm · Esc to cancel",
			state:   HarnessInputPermission,
		},
		{
			name:    "Claude selected negative still owns onboarding input",
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\n\nEnter to confirm",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Claude ANSI selected negative still owns onboarding input",
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n\x1b[1m❯\x1b[0m 2. \x1b[38;5;105mNo, exit\x1b[0m\n\nEnter to confirm",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Claude partially rendered trust question and bare cursor own onboarding input",
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n❯ ",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "Claude long known partial trust body cannot push question outside classifier",
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				"This folder pre-approves project permissions:\n" + strings.Repeat("Read\n", 14) + "❯ ",
			state: HarnessInputOnboarding,
		},
		{
			name:    "Claude historical question and arbitrary response expose current composer",
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n" +
				strings.Repeat("model response complete\n", 14) + "❯ ",
			ready: true,
			state: HarnessInputReady,
		},
		{
			// ce-wn4qe: current Claude Code idle composer with the multi-line
			// status footer (cwd@host, mode, auth, effort) below the ❯ box.
			// The prior chrome whitelist rejected the cwd/login/effort lines and
			// timed out every spawn; the composer must classify as ready.
			name:    "Claude current idle composer with status footer",
			harness: "claude-code",
			content: "────────────────────────────────────────\n❯ \n────────────────────────────────────────\n  vbonnet@mac:/private/tmp/wd\n  ⏸ plan mode on (shift+tab to cycle) · ← for agents\n                    Not logged in · Run /login\n                    ● high · /effort",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			// A ❯ with an active-work signal below it is a running turn, not a
			// ready composer.
			name:    "Claude composer running a turn is not ready",
			harness: "claude-code",
			content: "❯ \n  ✻ Working… (12s · esc to interrupt)\n  vbonnet@mac:/private/tmp/wd",
			state:   HarnessInputProcessing,
		},
		{
			// Active output beneath an empty ❯ that merely contains a slash token
			// like "/model" must not be mistaken for idle footer chrome.
			name:    "Claude output mentioning a slash token is not ready",
			harness: "claude-code",
			content: "❯ \nRunning /model migration\nApplying changes…",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude permission wins over prompt glyph",
			harness: "claude-code",
			content: "Do you want to proceed?\n❯ 1. Yes\n  2. No",
			state:   HarnessInputPermission,
		},
		{
			name:    "selector-only permission choices own tail",
			harness: "claude-code",
			content: "command preview\n❯ 1. Allow\n  2. Deny\nEsc to cancel",
			state:   HarnessInputPermission,
		},
		{
			name:    "resolved Claude permission inside live tail",
			harness: "claude-code",
			content: "Do you want to proceed?\n❯ 1. Allow\n  2. Deny\napproved\n❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "ordinary Allow and Deny output is not permission",
			harness: "claude-code",
			content: "The policy maps Deny before Allow.\noperation complete\n❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "permission question followed by work does not own input",
			harness: "claude-code",
			content: "Do you want to proceed?\nrequest already approved\n✻ Working…",
			state:   HarnessInputBusy,
		},
		{
			name:    "resolved Claude permission above live composer",
			harness: "claude-code",
			content: "Do you want to proceed?\n❯ 1. Allow\n  2. Deny\napproved\n" + strings.Repeat("historical output\n", 12) + "❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "resolved Claude trust before live composer",
			harness: "claude-code",
			content: "Do you trust the files in this folder?\n❯ 1. Yes\napproved\n❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "wrong harness composer",
			harness: "codex-cli",
			content: "❯",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude tail-owned composer",
			harness: "claude-code",
			content: "response\n❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Claude dim ghost text is an empty composer",
			harness: "claude-code",
			content: "response\n❯ \x1b[2mstart the loop\x1b[0m\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Claude grey ghost text is an empty composer",
			harness: "claude-code",
			content: "response\n❯ \x1b[38;5;241mstart the loop\x1b[0m\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Claude human draft is not an empty composer",
			harness: "claude-code",
			content: "response\n❯ start the loop\n────────────────\n? for shortcuts",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude human queued paste remains generic busy",
			harness: "claude-code",
			content: "response\n❯ [Pasted text #1 +2 lines]\nplease fix the bug in auth.go\n────────────────\n? for shortcuts",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude queued AGM paste is positively identified",
			harness: "claude-code",
			content: "response\n❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\nrecover\n────────────────\n? for shortcuts",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "long Claude queued AGM paste is classified beyond the display tail",
			harness: "claude-code",
			content: "response\n❯ [Pasted text #1 +14 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\n" + strings.Repeat("payload line\n", 13) + "────────────────\n? for shortcuts",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "historical AGM paste cannot occupy current empty Claude composer",
			harness: "claude-code",
			content: "❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\nrecover\nshort response\n❯\n────────────────\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "historical AGM header cannot own current human Claude paste",
			harness: "claude-code",
			content: "❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\nrecover\nshort response\n❯ [Pasted text #2 +1 lines]\nplease preserve my draft\n────────────────\n? for shortcuts",
			state:   HarnessInputBusy,
		},
		{
			name:    "partial AGM header cannot authorize Claude recovery",
			harness: "claude-code",
			content: "response\n❯ [Pasted text #1 +1 lines]\n[From: notes | ID:\n────────────────\n? for shortcuts",
			state:   HarnessInputBusy,
		},
		{
			name:    "opaque Codex pasted-content chip remains protected",
			harness: "codex-cli",
			content: "response\n› [Pasted Content 2172 chars]\n  gpt-5.6 xhigh · /repo",
			state:   HarnessInputBusy,
		},
		{
			name:    "post-turn Codex queue without empty cursor remains protected",
			harness: "codex-cli",
			content: "response\n› [Pasted Content 2172 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover\ngpt-5.6 xhigh · /repo",
			state:   HarnessInputBusy,
		},
		{
			name:    "active Codex turn with queued AGM paste remains protected",
			harness: "codex-cli",
			content: "response\n• Working on tests\n› [Pasted Content 2172 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover\ngpt-5.6 xhigh · /repo",
			state:   HarnessInputBusy,
		},
		{
			name:    "initial Codex composer can own observable queued AGM paste",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\n╰──────────────────────────╯\n› [Pasted Content 90 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "initial Codex queue preserves payload-ending newline in extent",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\n╰──────────────────────────╯\n› [Pasted Content 91 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover\n\n",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "active output after initial Codex queue suppresses recovery",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\n╰──────────────────────────╯\n› [Pasted Content 90 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover\nordinary active-work output",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale initial Codex header cannot own newer queued input",
			harness: "codex-cli",
			content: "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\nold response\n› [Pasted Content 2172 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover",
			state:   HarnessInputBusy,
		},
		{
			name:    "historical Codex paste cannot occupy current empty composer",
			harness: "codex-cli",
			content: "› [Pasted Content 2172 chars]\n[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\nrecover\nshort response\n›\ngpt-5.6 xhigh · /repo",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "active work suppresses stale queued AGM recovery",
			harness: "claude-code",
			content: "❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\nrecover\n✻ Working…\nRunning tests",
			state:   HarnessInputBusy,
		},
		{
			name:    "active output after stale Claude footer suppresses recovery",
			harness: "claude-code",
			content: "❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\nrecover\n────────────────\n? for shortcuts\nordinary active-work output",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale Claude composer before working view",
			harness: "claude-code",
			content: "response\n❯\n✻ Working…\nRunning tests",
			state:   HarnessInputBusy,
		},
		{
			name:    "Gemini structural composer owns tail",
			harness: "gemini-cli",
			content: "response\n╭────────────────╮\n│ >   Type your message or @path/to/file │\n╰────────────────╯\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Gemini generic border is not a composer",
			harness: "gemini-cli",
			content: "tool output\n╭────────────────╮\nWorking on request",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale Gemini composer before working view",
			harness: "gemini-cli",
			content: ">   Type your message or @path/to/file\nWorking on request",
			state:   HarnessInputBusy,
		},
		{
			name:    "Gemini queued AGM paste is positively identified",
			harness: "gemini-cli",
			content: "│ > [Pasted text #1 +2 lines] │\n[From: orchestrator | ID: 1774872000000-orchestr-003 | Sent: 2026-07-21T12:00:00Z]\nrecover\n╰────────────────╯\n? for shortcuts",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "active output after stale Gemini footer suppresses recovery",
			harness: "gemini-cli",
			content: "│ > [Pasted text #1 +2 lines] │\n[From: orchestrator | ID: 1774872000000-orchestr-003 | Sent: 2026-07-21T12:00:00Z]\nrecover\n? for shortcuts\nordinary active-work output",
			state:   HarnessInputBusy,
		},
		{
			name:    "Gemini trust owns input",
			harness: "gemini-cli",
			content: "Do you trust the files in this folder?\n● 1. Trust folder\n  2. Do not trust",
			state:   HarnessInputOnboarding,
		},
		{
			name:    "resolved Gemini trust before live composer",
			harness: "gemini-cli",
			content: "Do you trust the files in this folder?\n● 1. Trust folder\napproved\n│ >   Type your message or @path/to/file │\n? for shortcuts",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "OpenCode structural composer owns tail",
			harness: "opencode-cli",
			content: "response\n> Type your message",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "OpenCode output containing angle marker is not a composer",
			harness: "opencode-cli",
			content: "build > ready\nWorking on request",
			state:   HarnessInputBusy,
		},
		{
			name:    "stale OpenCode composer before working view",
			harness: "opencode-cli",
			content: "> Type your message\nWorking on request",
			state:   HarnessInputBusy,
		},
		{
			name:    "OpenCode queued AGM paste is positively identified",
			harness: "opencode-cli",
			content: "> [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-004 | Sent: 2026-07-21T12:00:00Z]\nrecover",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "historical OpenCode paste followed by output suppresses recovery",
			harness: "opencode-cli",
			content: "> [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-004 | Sent: 2026-07-21T12:00:00Z]\nrecover\nordinary active-work output",
			state:   HarnessInputBusy,
		},
		{
			name:    "historical OpenCode paste cannot occupy current empty composer",
			harness: "opencode-cli",
			content: "> [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-004 | Sent: 2026-07-21T12:00:00Z]\nrecover\nshort response\n>",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Pi managed ready status owns tail",
			harness: "pi-cli",
			content: "/work • pi-worker\nAGM plan/ready launch-current",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "Pi managed permission prompt wins over ready status",
			harness: "pi-cli",
			content: "AGM permission required\nAllow bash?\nAGM default/ready",
			state:   HarnessInputPermission,
		},
		{
			name:    "stale Pi ready status before working status",
			harness: "pi-cli",
			content: "AGM plan/ready launch-old\nAGM plan/working launch-current",
			state:   HarnessInputProcessing,
		},
		{
			name:    "Pi queued AGM paste is positively identified",
			harness: "pi-cli",
			content: "[Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-005 | Sent: 2026-07-21T12:00:00Z]\nrecover\n/work • pi-worker\n0.0%/0 (auto) AGM plan/ready launch-current",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "Pi queued human paste remains generic busy",
			harness: "pi-cli",
			content: "[Pasted text #1 +1 line]\nplease preserve my draft\nAGM plan/ready launch-current",
			state:   HarnessInputBusy,
		},
		{
			name:    "historical Pi paste cannot occupy current managed ready state",
			harness: "pi-cli",
			content: "[Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-005 | Sent: 2026-07-21T12:00:00Z]\nrecover\nshort response\nAGM plan/ready launch-current",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "historical Pi paste followed by output suppresses recovery",
			harness: "pi-cli",
			content: "[Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-005 | Sent: 2026-07-21T12:00:00Z]\nrecover\nAGM plan/ready launch-current\nordinary active-work output",
			state:   HarnessInputBusy,
		},
		{
			name:    "AGY survey wins over bare prompt",
			harness: "agy",
			content: ">\nHow's the CLI experience so far?\n[1] Good [2] Fine [3] Bad [0] Skip",
			state:   HarnessInputOverlay,
		},
		{
			name:    "resolved AGY survey before live composer",
			harness: "agy",
			content: "How's the CLI experience so far?\n[1] Good [2] Fine [3] Bad [0] Skip\nthanks\n>",
			ready:   true,
			state:   HarnessInputReady,
		},
		{
			name:    "stale AGY composer before working output",
			harness: "agy",
			content: ">\nprocessing request\nresponse chunk",
			state:   HarnessInputBusy,
		},
		{
			name:    "AGY queued AGM paste is positively identified",
			harness: "agy",
			content: "> [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-006 | Sent: 2026-07-21T12:00:00Z]\nrecover",
			state:   HarnessInputQueuedAGM,
		},
		{
			name:    "AGY queued human paste remains generic busy",
			harness: "agy",
			content: "> [Pasted text #1 +1 line]\nplease preserve my draft",
			state:   HarnessInputBusy,
		},
		{
			name:    "historical AGY paste followed by output suppresses recovery",
			harness: "agy",
			content: "> [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-006 | Sent: 2026-07-21T12:00:00Z]\nrecover\nordinary active-work output",
			state:   HarnessInputBusy,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ready, state, err := ClassifyHarnessInput(tt.content, tt.harness)
			if err != nil {
				t.Fatalf("ClassifyHarnessInput() error = %v", err)
			}
			if ready != tt.ready || state != tt.state {
				t.Fatalf("ClassifyHarnessInput() = (%v, %q), want (%v, %q)", ready, state, tt.ready, tt.state)
			}
		})
	}
}

func TestQueuedComposerPayloadPromptGlyphsRemainBoundToPasteAnchor(t *testing.T) {
	codexPayload := "[From: orchestrator | ID: 1774872000000-orchestr-002 | Sent: 2026-07-21T12:00:00Z]\n› explain this glyph without moving the anchor"
	tests := []struct {
		name    string
		harness string
		content string
	}{
		{
			name:    "Claude",
			harness: "claude-code",
			content: "response\n❯ [Pasted text #1 +2 lines]\n[From: orchestrator | ID: 1774872000000-orchestr-001 | Sent: 2026-07-21T12:00:00Z]\n❯ explain this glyph without moving the anchor\n────────────────\n? for shortcuts",
		},
		{
			name:    "Codex",
			harness: "codex-cli",
			content: fmt.Sprintf("│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\n╰──────────────────────────╯\n› [Pasted Content %d chars]\n%s", utf8.RuneCountInString(codexPayload), codexPayload),
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ready, state, err := ClassifyHarnessInput(testCase.content, testCase.harness)
			if err != nil {
				t.Fatalf("ClassifyHarnessInput() error = %v", err)
			}
			if ready || state != HarnessInputQueuedAGM {
				t.Fatalf("ClassifyHarnessInput() = (%t, %q), want (false, %q)", ready, state, HarnessInputQueuedAGM)
			}
		})
	}
}

func TestClassifyHarnessInputDistinguishesHumanDraftFromNativeProcessingTail(t *testing.T) {
	tests := []struct {
		name    string
		harness string
		content string
		state   string
	}{
		{
			name:    "Codex native active tail",
			harness: "codex-cli",
			content: "› compact this session\n• Working (3s • esc to interrupt)\ngpt-5.6 xhigh · ~/src/project",
			state:   HarnessInputProcessing,
		},
		{
			name:    "Codex human draft containing Working",
			harness: "codex-cli",
			content: "› Working on the release notes\ngpt-5.6 xhigh · ~/src/project",
			state:   HarnessInputBusy,
		},
		{
			name:    "Codex arbitrary Working prose lacks native control",
			harness: "codex-cli",
			content: "Working (3s)\ngpt-5.6 xhigh · ~/src/project",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude native active tail",
			harness: "claude-code",
			content: "❯ /compact\n✻ Compacting… (2s · esc to interrupt)\nvbonnet@mac:/private/tmp/wd",
			state:   HarnessInputProcessing,
		},
		{
			name:    "Claude human draft is not active evidence",
			harness: "claude-code",
			content: "❯ Working on compaction\n────────────────\n? for shortcuts",
			state:   HarnessInputBusy,
		},
		{
			name:    "Claude native-looking history without current footer fails closed",
			harness: "claude-code",
			content: "✻ Working… (2s · esc to interrupt)\nordinary model output",
			state:   HarnessInputBusy,
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			ready, state, err := ClassifyHarnessInput(testCase.content, testCase.harness)
			if err != nil {
				t.Fatalf("ClassifyHarnessInput() error = %v", err)
			}
			if ready || state != testCase.state {
				t.Fatalf("ClassifyHarnessInput() = (%t, %q), want (false, %q)", ready, state, testCase.state)
			}
		})
	}
}

func TestCheckExpectedHarnessInputForPaneDoesNotFollowActivePaneFocusChange(t *testing.T) {
	const targetPane = "%7"
	activePane := activePaneTarget{ID: "%8", RootPID: 800}
	runtime := harnessInputObservationRuntime{
		resolveActive: func(context.Context, string) (activePaneTarget, bool, error) {
			t.Fatalf("exact-target observation followed active pane %q", activePane.ID)
			return activePaneTarget{}, false, nil
		},
		resolvePane: func(_ context.Context, paneID string) (activePaneTarget, bool, error) {
			if paneID != targetPane {
				t.Fatalf("resolvePane(%q), want %q despite active pane %q", paneID, targetPane, activePane.ID)
			}
			return activePaneTarget{ID: targetPane, RootPID: 700}, true, nil
		},
		liveness: func(_ context.Context, pane activePaneTarget, harness string) (PaneLiveness, error) {
			if pane.ID != targetPane || harness != "codex-cli" {
				t.Fatalf("liveness target = %#v/%q", pane, harness)
			}
			return PaneLiveness{SessionExists: true, HarnessAlive: true, HarnessPID: 701}, nil
		},
		capture: func(_ context.Context, paneID string) (string, error) {
			if paneID != targetPane {
				t.Fatalf("capture target = %q, want %q", paneID, targetPane)
			}
			return "│ >_ OpenAI Codex (vtest) │\n│ model: gpt-5.6 /model to change │\n›", nil
		},
	}

	got, err := checkExpectedHarnessInputForPane(t.Context(), targetPane, 701, "codex-cli", runtime)
	if err != nil {
		t.Fatalf("CheckExpectedHarnessInputForPane() error = %v", err)
	}
	if !got.Ready || got.TargetPane != targetPane || got.TargetPID != 701 {
		t.Fatalf("exact-target readiness = %#v, want ready on %s/701", got, targetPane)
	}
}

func TestCheckExpectedHarnessInputForPaneRejectsReplacedForegroundHarnessPID(t *testing.T) {
	captured := false
	runtime := harnessInputObservationRuntime{
		resolvePane: func(context.Context, string) (activePaneTarget, bool, error) {
			return activePaneTarget{ID: "%7", RootPID: 700}, true, nil
		},
		liveness: func(context.Context, activePaneTarget, string) (PaneLiveness, error) {
			return PaneLiveness{SessionExists: true, HarnessAlive: true, HarnessPID: 702}, nil
		},
		capture: func(context.Context, string) (string, error) {
			captured = true
			return "", nil
		},
	}

	got, err := checkExpectedHarnessInputForPane(t.Context(), "%7", 701, "codex-cli", runtime)
	if err != nil {
		t.Fatalf("CheckExpectedHarnessInputForPane() error = %v", err)
	}
	if got.Ready || got.State != HarnessInputNotFound || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("replaced foreground harness readiness = %#v, want NOT_FOUND on original target", got)
	}
	if captured {
		t.Fatal("replaced foreground harness PID was allowed to contribute pane content")
	}
}

func TestCheckExpectedHarnessInputForPaneRejectsDeletedPane(t *testing.T) {
	runtime := harnessInputObservationRuntime{
		resolvePane: func(context.Context, string) (activePaneTarget, bool, error) {
			return activePaneTarget{}, false, nil
		},
		liveness: func(context.Context, activePaneTarget, string) (PaneLiveness, error) {
			t.Fatal("deleted pane reached liveness scan")
			return PaneLiveness{}, nil
		},
		capture: func(context.Context, string) (string, error) {
			t.Fatal("deleted pane reached capture")
			return "", nil
		},
	}

	got, err := checkExpectedHarnessInputForPane(t.Context(), "%7", 701, "codex-cli", runtime)
	if err != nil {
		t.Fatalf("CheckExpectedHarnessInputForPane() error = %v", err)
	}
	if got.Ready || got.State != HarnessInputNotFound || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("deleted pane readiness = %#v, want NOT_FOUND on original target", got)
	}
}

func TestHarnessStartupAdvanceKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		harness string
		content string
		want    []string
	}{
		{name: "Gemini trust selection", harness: "gemini-cli", content: "Do you trust the files in this folder?", want: []string{"1", "Enter"}},
		{name: "Codex model upgrade", harness: "codex-cli", content: "Choose how you'd like Codex to proceed", want: []string{"Down", "Enter"}},
		{name: "Codex update selector", harness: "codex-cli", content: "Update available!\nUpdate now\nSkip until next version", want: []string{"Down", "Enter"}},
		{name: "AGY survey", harness: "agy", content: "How's the CLI experience so far?\n[0] Skip", want: []string{"0"}},
		{name: "default trust selection", harness: "claude-code", content: "Do you trust the files in this folder?", want: []string{"Enter"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := harnessStartupAdvanceKeys(tt.harness, tt.content); !slices.Equal(got, tt.want) {
				t.Fatalf("harnessStartupAdvanceKeys(%q) = %#v, want %#v", tt.harness, got, tt.want)
			}
		})
	}
}

func TestCanAdvanceHarnessStartup(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   string
		harness string
		content string
		want    bool
	}{
		{
			name:    "Claude selected affirmative may advance",
			state:   HarnessInputOnboarding,
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n❯ 1. Yes, I trust this folder\n  2. No, exit\nEnter to confirm",
			want:    true,
		},
		{
			name:    "Claude selected negative may not advance",
			state:   HarnessInputOnboarding,
			harness: "claude-code",
			content: "Quick safety check: Is this a project you created or one you trust?\n  1. Yes, I trust this folder\n❯ 2. No, exit\nEnter to confirm",
			want:    false,
		},
		{
			name:    "other onboarding keeps existing advancement",
			state:   HarnessInputOnboarding,
			harness: "codex-cli",
			content: "Do you trust the contents of this directory?",
			want:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canAdvanceHarnessStartup(tt.state, tt.harness, tt.content); got != tt.want {
				t.Fatalf("canAdvanceHarnessStartup(%q, %q) = %t, want %t", tt.state, tt.harness, got, tt.want)
			}
		})
	}
}

func TestClassifyHarnessInputRejectsStalePromptOutsideTail(t *testing.T) {
	content := "❯\n" + strings.Repeat("historical output\n", 13)
	ready, state, err := ClassifyHarnessInput(content, "claude-code")
	if err != nil {
		t.Fatalf("ClassifyHarnessInput() error = %v", err)
	}
	if ready || state != HarnessInputBusy {
		t.Fatalf("stale prompt = (%v, %q), want busy", ready, state)
	}
}

func TestExpectedHarnessMatcherRejectsWrongProcess(t *testing.T) {
	procs := []ProcEntry{
		{PID: 10, PPID: 1, Comm: "zsh"},
		{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "S+", Comm: "agy"},
	}
	codex := classifyPaneLivenessProcesses([]int{10}, procs, expectedHarnessProcessMatcher("codex-cli"))
	if codex.HarnessAlive {
		t.Fatalf("AGY process classified as Codex: %#v", codex)
	}
	agy := classifyPaneLivenessProcesses([]int{10}, procs, expectedHarnessProcessMatcher("agy"))
	if !agy.HarnessAlive {
		t.Fatalf("AGY process not detected: %#v", agy)
	}
	piProcs := []ProcEntry{
		{PID: 10, PPID: 1, Comm: "zsh"},
		{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "S+", Comm: "agy"},
		{PID: 12, PPID: 10, PGID: 12, TPGID: 12, State: "S+", Comm: "pi", Args: "pi --session-id native"},
	}
	pi := classifyPaneLivenessProcesses([]int{10}, piProcs, expectedHarnessProcessMatcher("pi-cli"))
	if !pi.HarnessAlive {
		t.Fatalf("Pi process not detected: %#v", pi)
	}
}

func TestExpectedHarnessMatcherAcceptsIdentifiedNodeBackedHarness(t *testing.T) {
	t.Parallel()

	tests := []struct {
		harness string
		args    string
	}{
		{harness: "claude-code", args: "env NODE_ENV=production /usr/local/bin/node /opt/node_modules/@anthropic-ai/claude-code/cli.js"},
		{harness: "codex-cli", args: "/usr/local/bin/node --enable-source-maps /opt/node_modules/@openai/codex/bin/codex.js --quiet"},
		{harness: "gemini-cli", args: `/usr/local/bin/node "/opt/My Tools/node_modules/@google/gemini-cli/dist/index.js"`},
		{harness: "opencode-cli", args: "/usr/local/bin/node /opt/node_modules/opencode-ai/bin/opencode.js"},
		{harness: "pi-cli", args: "/usr/local/bin/node /opt/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"},
	}
	for _, tt := range tests {
		procs := []ProcEntry{
			{PID: 10, PPID: 1, Comm: "zsh"},
			{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "S+", Comm: "MainThread", Args: tt.args},
		}
		got := classifyPaneLivenessProcesses([]int{10}, procs, expectedHarnessProcessMatcher(tt.harness))
		if !got.HarnessAlive {
			t.Errorf("identified Node-backed %s liveness = %#v, want alive", tt.harness, got)
		}
	}
}

func TestExpectedHarnessMatcherRejectsUnrelatedNodeProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		harness string
		args    string
	}{
		{harness: "claude-code", args: "/usr/local/bin/node /srv/worker.js /tmp/@anthropic-ai/claude-code/cli.js"},
		{harness: "codex-cli", args: "/usr/local/bin/node /srv/worker.js /tmp/codex.js"},
		{harness: "gemini-cli", args: "/usr/local/bin/node --require /tmp/@google/gemini-cli.js /srv/worker.js"},
		{harness: "opencode-cli", args: "/usr/local/bin/node --trace-warnings /srv/worker.js /tmp/opencode.js"},
		{harness: "pi-cli", args: "/usr/local/bin/node /srv/worker.js /opt/node_modules/@earendil-works/pi-coding-agent/dist/cli.js"},
	}
	for _, tt := range tests {
		procs := []ProcEntry{
			{PID: 10, PPID: 1, Comm: "zsh", Args: "zsh"},
			{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "S+", Comm: "MainThread", Args: tt.args},
		}
		got := classifyPaneLivenessProcesses([]int{10}, procs, expectedHarnessProcessMatcher(tt.harness))
		if got.HarnessAlive {
			t.Errorf("unrelated Node process %q classified as %s: %#v", tt.args, tt.harness, got)
		}
	}
}

func TestExpectedHarnessMatcherRequiresForegroundTerminalOwnership(t *testing.T) {
	t.Parallel()

	for _, process := range []ProcEntry{
		{PID: 11, PPID: 10, PGID: 11, TPGID: 10, State: "S", Comm: "claude"},
		{PID: 11, PPID: 10, PGID: 11, TPGID: 11, State: "T+", Comm: "claude"},
	} {
		procs := []ProcEntry{{PID: 10, PPID: 1, PGID: 10, TPGID: 10, State: "S+", Comm: "zsh"}, process}
		got := classifyPaneLivenessProcesses([]int{10}, procs, expectedHarnessProcessMatcher("claude-code"))
		if got.HarnessAlive {
			t.Errorf("non-foreground Claude process classified alive: %#v", process)
		}
	}
}

func TestInputDeliveryAllowedOverridesOnlyPositivelyIdentifiedAGMQueue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		readiness HarnessInputReadiness
		force     bool
		allowed   bool
		forced    bool
	}{
		{name: "ready", readiness: HarnessInputReadiness{Ready: true, State: HarnessInputReady}, allowed: true},
		{name: "ready with force", readiness: HarnessInputReadiness{Ready: true, State: HarnessInputReady}, force: true, allowed: true},
		{name: "generic busy without policy", readiness: HarnessInputReadiness{State: HarnessInputBusy}},
		{name: "generic busy with policy", readiness: HarnessInputReadiness{State: HarnessInputBusy}, force: true},
		{name: "queued AGM without policy", readiness: HarnessInputReadiness{State: HarnessInputQueuedAGM}},
		{name: "queued AGM with policy", readiness: HarnessInputReadiness{State: HarnessInputQueuedAGM}, force: true, allowed: true, forced: true},
		{name: "permission with force", readiness: HarnessInputReadiness{State: HarnessInputPermission}, force: true},
		{name: "overlay with force", readiness: HarnessInputReadiness{State: HarnessInputOverlay}, force: true},
		{name: "onboarding with force", readiness: HarnessInputReadiness{State: HarnessInputOnboarding}, force: true},
		{name: "wrong harness with force", readiness: HarnessInputReadiness{State: HarnessInputWrongHarness}, force: true},
		{name: "missing with force", readiness: HarnessInputReadiness{State: HarnessInputNotFound}, force: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			allowed, forced := inputDeliveryAllowed(tt.readiness, InputDeliveryOptions{AllowQueuedAGM: tt.force})
			if allowed != tt.allowed || forced != tt.forced {
				t.Fatalf("inputDeliveryAllowed() = (%t, %t), want (%t, %t)", allowed, forced, tt.allowed, tt.forced)
			}
		})
	}
}

func TestCheckExpectedHarnessInputAndSendHonorsCancellationDuringTmuxLockContention(t *testing.T) {
	ReleaseTmuxLock()
	stateDir := t.TempDir()
	t.Setenv("AGM_STATE_DIR", stateDir)
	external, err := lock.New(filepath.Join(stateDir, "tmux-server.lock"))
	if err != nil {
		t.Fatalf("create external tmux lock: %v", err)
	}
	if err := external.TryLock(); err != nil {
		t.Fatalf("acquire external tmux lock: %v", err)
	}
	t.Cleanup(func() {
		_ = external.Unlock()
		_ = ReleaseTmuxLock()
	})

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		_, callErr := CheckExpectedHarnessInputAndSend(ctx, "contended", "codex-cli", "/compact", InputDeliveryOptions{})
		done <- callErr
	}()

	deadline := time.Now().Add(time.Second)
	for TmuxConcurrentOps() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if TmuxConcurrentOps() == 0 {
		cancel()
		t.Fatal("atomic delivery never reached the contended tmux lock")
	}
	select {
	case callErr := <-done:
		cancel()
		t.Fatalf("contended delivery returned before cancellation: %v", callErr)
	default:
	}

	cancel()
	select {
	case callErr := <-done:
		if !errors.Is(callErr, context.Canceled) {
			t.Fatalf("CheckExpectedHarnessInputAndSend() error = %v, want context.Canceled", callErr)
		}
	case <-time.After(time.Second):
		t.Fatal("CheckExpectedHarnessInputAndSend wedged on flock after cancellation")
	}
	if got := TmuxConcurrentOps(); got != 0 {
		t.Fatalf("tmux semaphore count after cancellation = %d, want 0", got)
	}
}

func TestAtomicExpectedHarnessDeliveryReprovesExactTargetBeforeSend(t *testing.T) {
	delivered := false
	runtime := harnessInputDeliveryRuntime{
		check: func(context.Context, string, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		recheckExact: func(_ context.Context, paneID string, harnessPID int, harness string) (HarnessInputReadiness, error) {
			if paneID != "%7" || harnessPID != 701 || harness != "codex-cli" {
				t.Fatalf("exact recheck = %q/%d/%q", paneID, harnessPID, harness)
			}
			return HarnessInputReadiness{State: HarnessInputNotFound, TargetPane: paneID, TargetPID: harnessPID}, nil
		},
		deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error {
			delivered = true
			return nil
		},
	}

	got, err := checkExpectedHarnessInputAndSendLocked(t.Context(), "session", "codex-cli", "/compact", InputDeliveryOptions{}, runtime)
	if err != nil {
		t.Fatalf("atomic delivery error = %v", err)
	}
	if delivered {
		t.Fatal("delivery proceeded after exact harness PID was no longer present")
	}
	if got.Ready || got.State != HarnessInputNotFound || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("recheck outcome = %#v, want non-ready original target", got)
	}
}

func TestAtomicExpectedHarnessDeliveryPreservesSubmissionUncertainty(t *testing.T) {
	ackLost := errors.New("tmux acknowledgement lost")
	strictConfirmation := false
	rawBracketedPaste := false
	runtime := harnessInputDeliveryRuntime{
		check: func(context.Context, string, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		recheckExact: func(context.Context, string, int, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		deliver: func(_ context.Context, _ HarnessInputReadiness, _, _ string, requireObservedSubmission, rawBracketed bool) error {
			strictConfirmation = requireObservedSubmission
			rawBracketedPaste = rawBracketed
			return MarkPromptSubmissionUncertain(ackLost)
		},
	}

	got, err := checkExpectedHarnessInputAndSendLocked(t.Context(), "session", "codex-cli", "/compact", InputDeliveryOptions{
		RequireSubmissionConfirmation: true,
		RawBracketedPaste:             true,
	}, runtime)
	if !errors.Is(err, ackLost) {
		t.Fatalf("atomic delivery error = %v, want lost acknowledgement", err)
	}
	if got.Ready || !got.MayHaveStarted || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("uncertain submission outcome = %#v, want may-have-started exact target", got)
	}
	if !strictConfirmation {
		t.Fatal("atomic delivery did not propagate strict post-Enter confirmation")
	}
	if !rawBracketedPaste {
		t.Fatal("atomic delivery did not propagate raw bracketed paste")
	}
}

func TestAtomicExpectedHarnessDeliveryStrictlyReprovesExactTargetAfterSubmit(t *testing.T) {
	postSubmitProofs := []struct {
		name                     string
		harness                  string
		observation              HarnessInputReadiness
		wantPostSubmitProcessing bool
	}{
		{
			name:                     "native processing",
			harness:                  "codex-cli",
			observation:              HarnessInputReadiness{State: HarnessInputProcessing, TargetPane: "%7", TargetPID: 701},
			wantPostSubmitProcessing: true,
		},
		{
			name:        "live ready",
			harness:     "codex-cli",
			observation: HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701},
		},
	}
	for _, proof := range postSubmitProofs {
		t.Run(proof.name, func(t *testing.T) {
			var events []string
			rechecks := 0
			runtime := harnessInputDeliveryRuntime{
				check: func(context.Context, string, string) (HarnessInputReadiness, error) {
					events = append(events, "check")
					return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
				},
				recheckExact: func(_ context.Context, paneID string, harnessPID int, harness string) (HarnessInputReadiness, error) {
					rechecks++
					events = append(events, fmt.Sprintf("recheck-%d", rechecks))
					if paneID != "%7" || harnessPID != 701 || harness != proof.harness {
						t.Fatalf("exact recheck = %q/%d/%q", paneID, harnessPID, harness)
					}
					if rechecks == 1 {
						return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: paneID, TargetPID: harnessPID}, nil
					}
					return proof.observation, nil
				},
				deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error {
					events = append(events, "deliver")
					return nil
				},
			}

			got, err := checkExpectedHarnessInputAndSendLocked(t.Context(), "session", proof.harness, "/compact", InputDeliveryOptions{
				RequireSubmissionConfirmation: true,
			}, runtime)
			if err != nil {
				t.Fatalf("strict atomic delivery error = %v", err)
			}
			if !got.Ready || got.MayHaveStarted || got.TargetPane != "%7" || got.TargetPID != 701 ||
				got.PostSubmitProcessing != proof.wantPostSubmitProcessing {
				t.Fatalf("strict atomic delivery = %#v, want confirmed exact target", got)
			}
			wantEvents := []string{"check", "recheck-1", "deliver", "recheck-2"}
			if !slices.Equal(events, wantEvents) {
				t.Fatalf("strict atomic delivery events = %#v, want %#v", events, wantEvents)
			}
		})
	}
}

func TestAtomicExpectedHarnessDeliveryTreatsPostSubmitReproofFailureAsUncertain(t *testing.T) {
	observationErr := errors.New("post-submit tmux observation failed")
	tests := []struct {
		name     string
		observed HarnessInputReadiness
		err      error
	}{
		{
			name:     "generic busy after command-shaped human draft",
			observed: HarnessInputReadiness{State: HarnessInputBusy, TargetPane: "%7", TargetPID: 701},
		},
		{
			name:     "foreground PID changed",
			observed: HarnessInputReadiness{State: HarnessInputProcessing, TargetPane: "%7", TargetPID: 702},
		},
		{
			name:     "expected harness disappeared",
			observed: HarnessInputReadiness{State: HarnessInputWrongHarness, TargetPane: "%7"},
		},
		{
			name:     "pane disappeared",
			observed: HarnessInputReadiness{State: HarnessInputNotFound, TargetPane: "%7", TargetPID: 701},
		},
		{
			name:     "queued AGM composer is not submission proof",
			observed: HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: "%7", TargetPID: 701},
		},
		{
			name:     "permission prompt is not submission proof",
			observed: HarnessInputReadiness{State: HarnessInputPermission, TargetPane: "%7", TargetPID: 701},
		},
		{
			name:     "overlay is not submission proof",
			observed: HarnessInputReadiness{State: HarnessInputOverlay, TargetPane: "%7", TargetPID: 701},
		},
		{
			name:     "onboarding is not submission proof",
			observed: HarnessInputReadiness{State: HarnessInputOnboarding, TargetPane: "%7", TargetPID: 701},
		},
		{
			name: "observation failed",
			err:  observationErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rechecks := 0
			deliveries := 0
			runtime := harnessInputDeliveryRuntime{
				check: func(context.Context, string, string) (HarnessInputReadiness, error) {
					return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
				},
				recheckExact: func(context.Context, string, int, string) (HarnessInputReadiness, error) {
					rechecks++
					if rechecks == 1 {
						return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
					}
					return test.observed, test.err
				},
				deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error {
					deliveries++
					return nil
				},
			}

			got, err := checkExpectedHarnessInputAndSendLocked(t.Context(), "session", "codex-cli", "/compact", InputDeliveryOptions{
				RequireSubmissionConfirmation: true,
			}, runtime)
			if err == nil || !PromptSubmissionMayHaveOccurred(err) {
				t.Fatalf("post-submit identity loss error = %v, want marked submission uncertainty", err)
			}
			if test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("post-submit observation error = %v, want cause %v", err, test.err)
			}
			if got.Ready || !got.MayHaveStarted || got.TargetPane != "%7" || got.TargetPID != 701 {
				t.Fatalf("post-submit identity loss result = %#v, want uncertain original receipt", got)
			}
			if deliveries != 1 || rechecks != 2 {
				t.Fatalf("deliveries/rechecks = %d/%d, want 1/2", deliveries, rechecks)
			}
		})
	}
}

func TestAtomicExpectedHarnessDeliveryLegacySkipsPostSubmitIdentityReproof(t *testing.T) {
	rechecks := 0
	runtime := harnessInputDeliveryRuntime{
		check: func(context.Context, string, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		recheckExact: func(context.Context, string, int, string) (HarnessInputReadiness, error) {
			rechecks++
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error { return nil },
	}

	got, err := checkExpectedHarnessInputAndSendLocked(t.Context(), "session", "codex-cli", "ordinary prompt", InputDeliveryOptions{}, runtime)
	if err != nil {
		t.Fatalf("legacy atomic delivery error = %v", err)
	}
	if !got.Ready || rechecks != 1 {
		t.Fatalf("legacy atomic delivery = %#v with %d rechecks, want one pre-submit recheck", got, rechecks)
	}
}

func TestAtomicExpectedHarnessDeliveryRequiresStableSessionBinding(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		initialStable string
		recheckStable string
		wantDelivered bool
	}{
		{name: "matching binding", initialStable: "stable-session", recheckStable: "stable-session", wantDelivered: true},
		{name: "missing binding"},
		{name: "wrong binding", initialStable: "replacement-session"},
		{name: "replacement before send", initialStable: "stable-session", recheckStable: "replacement-session"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			deliveries := 0
			rechecks := 0
			runtime := harnessInputDeliveryRuntime{
				check: func(context.Context, string, string) (HarnessInputReadiness, error) {
					return HarnessInputReadiness{
						Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPanePID: 77, TargetPID: 701,
						HarnessStartTime: "Thu Aug 27 07:00:00 2026",
						TargetSessionID:  "$1", StableSessionID: test.initialStable,
					}, nil
				},
				recheckExact: func(context.Context, string, int, string) (HarnessInputReadiness, error) {
					rechecks++
					return HarnessInputReadiness{
						Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPanePID: 77, TargetPID: 701,
						HarnessStartTime: "Thu Aug 27 07:00:00 2026",
						TargetSessionID:  "$1", StableSessionID: test.recheckStable,
					}, nil
				},
				deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error {
					deliveries++
					return nil
				},
			}

			got, err := checkExpectedHarnessInputAndSendLocked(
				t.Context(), "session", "codex-cli", "/compact",
				InputDeliveryOptions{ExpectedStableSessionID: "stable-session"}, runtime,
			)
			if test.wantDelivered {
				if err != nil || !got.Ready || deliveries != 1 || rechecks != 1 {
					t.Fatalf("matching stable delivery = %#v, %v; deliveries/rechecks=%d/%d", got, err, deliveries, rechecks)
				}
				return
			}
			if err == nil || got.Ready || deliveries != 0 {
				t.Fatalf("mismatched stable delivery = %#v, %v; deliveries=%d, want fail before mutation", got, err, deliveries)
			}
		})
	}
}

func TestAtomicExpectedHarnessDeliveryPreservesSendOnLockReleaseFailure(t *testing.T) {
	t.Parallel()

	unlockErr := errors.New("unlock failed")
	runtime := harnessInputDeliveryRuntime{
		check: func(context.Context, string, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		recheckExact: func(context.Context, string, int, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		deliver: func(context.Context, HarnessInputReadiness, string, string, bool, bool) error { return nil },
	}
	withUnlockFailure := func(_ context.Context, fn func() error) error {
		if err := fn(); err != nil {
			return err
		}
		return unlockErr
	}

	got, err := checkExpectedHarnessInputAndSendAtBoundary(
		t.Context(), "session", "codex-cli", "/compact", InputDeliveryOptions{}, runtime, withUnlockFailure,
	)
	if !errors.Is(err, unlockErr) || !PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("lock release error = %v, want marked submission uncertainty preserving cause", err)
	}
	if got.Ready || !got.MayHaveStarted || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("lock release result = %#v, want uncertain exact delivery receipt", got)
	}
}

func TestQueuedAGMRecoveryClearsBeforeReplacement(t *testing.T) {
	t.Parallel()

	var events []string
	runtime := queuedAGMRecoveryRuntime{
		sendKey: func(_ context.Context, pane, key string) error {
			events = append(events, "key:"+pane+":"+key)
			return nil
		},
		wait: func(_ context.Context, delay time.Duration) error {
			events = append(events, "wait:"+delay.String())
			return nil
		},
		recheck: func() (HarnessInputReadiness, error) {
			events = append(events, "recheck")
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701, Content: "empty composer"}, nil
		},
		deliver: func(_ context.Context, pane, command string) (HarnessInputReadiness, error) {
			events = append(events, "deliver:"+pane+":"+command)
			return HarnessInputReadiness{}, nil
		},
	}

	got, err := replaceQueuedAGMInputLocked(context.Background(), "%7", 701, "replacement", runtime)
	if err != nil {
		t.Fatalf("replaceQueuedAGMInputLocked() error = %v", err)
	}
	want := []string{
		"key:%7:C-c", "wait:200ms", "key:%7:C-u", "wait:100ms",
		"key:%7:C-a", "key:%7:C-k", "wait:300ms", "recheck",
		"deliver:%7:replacement",
	}
	if !slices.Equal(events, want) {
		t.Fatalf("replacement events = %#v, want %#v", events, want)
	}
	if !got.Ready || got.State != HarnessInputReady || !got.Forced || got.TargetPane != "%7" || got.TargetPID != 701 {
		t.Fatalf("replacement readiness = %#v, want forced ready on %%7", got)
	}
}

func TestQueuedAGMRecoveryDoesNotReplaceUntilExactPaneIsEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		recheck HarnessInputReadiness
	}{
		{name: "queue remains", recheck: HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: "%7", TargetPID: 701}},
		{name: "human input appears", recheck: HarnessInputReadiness{State: HarnessInputBusy, TargetPane: "%7", TargetPID: 701}},
		{name: "active pane changes", recheck: HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%8", TargetPID: 801}},
		{name: "pane process changes", recheck: HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 702}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delivered := false
			runtime := queuedAGMRecoveryRuntime{
				sendKey: func(context.Context, string, string) error { return nil },
				wait:    func(context.Context, time.Duration) error { return nil },
				recheck: func() (HarnessInputReadiness, error) { return tt.recheck, nil },
				deliver: func(context.Context, string, string) (HarnessInputReadiness, error) {
					delivered = true
					return HarnessInputReadiness{}, nil
				},
			}
			got, err := replaceQueuedAGMInputLocked(context.Background(), "%7", 701, "replacement", runtime)
			if err == nil {
				t.Fatal("replaceQueuedAGMInputLocked() error = nil, want failed closed")
			}
			if delivered {
				t.Fatal("replacement was delivered before exact empty-composer proof")
			}
			if got.Ready {
				t.Fatalf("failed recovery readiness = %#v, want Ready=false", got)
			}
		})
	}
}

func TestQueuedAGMRecoveryDoesNotReportReadyWhenReplacementFails(t *testing.T) {
	t.Parallel()

	runtime := queuedAGMRecoveryRuntime{
		sendKey: func(context.Context, string, string) error { return nil },
		wait:    func(context.Context, time.Duration) error { return nil },
		recheck: func() (HarnessInputReadiness, error) {
			return HarnessInputReadiness{Ready: true, State: HarnessInputReady, TargetPane: "%7", TargetPID: 701}, nil
		},
		deliver: func(context.Context, string, string) (HarnessInputReadiness, error) {
			return HarnessInputReadiness{}, errors.New("paste failed")
		},
	}
	got, err := replaceQueuedAGMInputLocked(context.Background(), "%7", 701, "replacement", runtime)
	if err == nil || !strings.Contains(err.Error(), "paste failed") {
		t.Fatalf("replaceQueuedAGMInputLocked() error = %v, want paste failure", err)
	}
	if got.Ready {
		t.Fatalf("failed replacement readiness = %#v, want Ready=false", got)
	}
}
