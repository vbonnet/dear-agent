package tmux

import (
	"context"
	"slices"
	"strings"
	"testing"
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

func TestInputDeliveryAllowedForceOverridesOnlyBusyComposer(t *testing.T) {
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
		{name: "busy without force", readiness: HarnessInputReadiness{State: HarnessInputBusy}},
		{name: "busy with force", readiness: HarnessInputReadiness{State: HarnessInputBusy}, force: true, allowed: true, forced: true},
		{name: "permission with force", readiness: HarnessInputReadiness{State: HarnessInputPermission}, force: true},
		{name: "overlay with force", readiness: HarnessInputReadiness{State: HarnessInputOverlay}, force: true},
		{name: "onboarding with force", readiness: HarnessInputReadiness{State: HarnessInputOnboarding}, force: true},
		{name: "wrong harness with force", readiness: HarnessInputReadiness{State: HarnessInputWrongHarness}, force: true},
		{name: "missing with force", readiness: HarnessInputReadiness{State: HarnessInputNotFound}, force: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			allowed, forced := inputDeliveryAllowed(tt.readiness, InputDeliveryOptions{AllowBusyComposer: tt.force})
			if allowed != tt.allowed || forced != tt.forced {
				t.Fatalf("inputDeliveryAllowed() = (%t, %t), want (%t, %t)", allowed, forced, tt.allowed, tt.forced)
			}
		})
	}
}
