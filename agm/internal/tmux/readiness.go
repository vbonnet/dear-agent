package tmux

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// HarnessInputReady means the harness can safely receive input.
	HarnessInputReady = "YES"
	// HarnessInputBusy means the harness does not currently own an empty composer.
	HarnessInputBusy = "QUEUE"
	// HarnessInputQueuedAGM means a queued-input marker and complete AGM message
	// header positively identify a stuck AGM paste in the current logical composer.
	HarnessInputQueuedAGM = "QUEUED_AGM"
	// HarnessInputPermission means a permission decision currently owns input.
	HarnessInputPermission = "PERMISSION"
	// HarnessInputOverlay means a harness overlay currently owns input.
	HarnessInputOverlay = "OVERLAY"
	// HarnessInputOnboarding means a documented first-run prompt currently owns input.
	HarnessInputOnboarding = "ONBOARDING"
	// HarnessInputNotFound means the exact tmux session does not exist.
	HarnessInputNotFound = "NOT_FOUND"
	// HarnessInputWrongHarness means the expected harness process is not alive.
	HarnessInputWrongHarness = "WRONG_HARNESS"
)

// HarnessInputReadiness is the harness-specific, fail-closed verdict shared by
// every surface before it sends input to a tmux pane.
type HarnessInputReadiness struct {
	Ready      bool
	State      string
	Content    string
	TargetPane string
	Forced     bool
}

// InputDeliveryOptions controls narrowly scoped exceptions inside the tmux
// mutation boundary. AllowQueuedAGM accepts only a positively identified stuck
// AGM paste after the expected foreground harness and exact pane are proved;
// the implementation clears and re-proves that exact composer before replacing
// the queued input.
type InputDeliveryOptions struct {
	AllowQueuedAGM bool
}

// CheckExpectedHarnessInput proves that the exact session exists, an expected
// harness process is alive, and that harness currently owns the input composer.
// A stale prompt rendered by a dead or different process is never sufficient.
func CheckExpectedHarnessInput(ctx context.Context, sessionName, harness string) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateReadinessHarness(harness); err != nil {
		return HarnessInputReadiness{}, err
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	pane, exists, err := resolveActivePaneTarget(scanCtx, sessionName, GetSocketPath())
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !exists {
		return HarnessInputReadiness{State: HarnessInputNotFound}, nil
	}
	liveness, err := checkExpectedHarnessLivenessForPane(scanCtx, pane, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	if !liveness.HarnessAlive {
		return HarnessInputReadiness{State: HarnessInputWrongHarness, TargetPane: pane.ID}, nil
	}
	styledContent, err := CapturePaneLogicalANSIOutputTargetContext(ctx, pane.ID)
	if err != nil {
		return HarnessInputReadiness{}, fmt.Errorf("capture expected %s pane: %w", harness, err)
	}
	ready, state, err := ClassifyHarnessInput(styledContent, harness)
	if err != nil {
		return HarnessInputReadiness{}, err
	}
	return HarnessInputReadiness{Ready: ready, State: state, Content: stripANSI(styledContent), TargetPane: pane.ID}, nil
}

// CheckExpectedHarnessInputAndSend serializes the readiness observation and
// exact-pane delivery under the same tmux mutation lock. A non-ready result
// never sends input unless options explicitly allow a positively identified
// stuck AGM paste; a ready result is returned only after delivery succeeds.
func CheckExpectedHarnessInputAndSend(ctx context.Context, sessionName, harness, command string, options InputDeliveryOptions) (HarnessInputReadiness, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return HarnessInputReadiness{}, fmt.Errorf("tmux concurrency limit reached: %w", err)
	}
	defer releaseTmuxSemaphore()

	var readiness HarnessInputReadiness
	err := withTmuxLock(func() error {
		var err error
		readiness, err = CheckExpectedHarnessInput(ctx, sessionName, harness)
		if err != nil {
			return err
		}
		allowed, forced := inputDeliveryAllowed(readiness, options)
		if !allowed {
			return nil
		}
		if !isPaneID(readiness.TargetPane) {
			return fmt.Errorf("ready harness returned invalid tmux pane ID %q", readiness.TargetPane)
		}
		if forced {
			readiness.Ready = false
			readiness.Forced = true
			recovered, recoveryErr := replaceQueuedAGMInputLocked(ctx, readiness.TargetPane, command, queuedAGMRecoveryRuntime{
				sendKey: sendReadinessKey,
				wait:    sleepWithContext,
				recheck: func() (HarnessInputReadiness, error) {
					return CheckExpectedHarnessInput(ctx, sessionName, harness)
				},
				deliver: func(ctx context.Context, targetPane, command string) error {
					return sendCommandToTargetForHarnessLocked(ctx, targetPane, command, harness)
				},
			})
			if recoveryErr != nil {
				return recoveryErr
			}
			readiness = recovered
			return nil
		}
		return sendCommandToTargetForHarnessLocked(ctx, readiness.TargetPane, command, harness)
	})
	return readiness, err
}

type queuedAGMRecoveryRuntime struct {
	sendKey func(context.Context, string, string) error
	wait    func(context.Context, time.Duration) error
	recheck func() (HarnessInputReadiness, error)
	deliver func(context.Context, string, string) error
}

// replaceQueuedAGMInputLocked clears one positively identified AGM-owned
// composer and proves that the same exact pane now owns an empty composer before
// delivering its replacement. The caller must hold the tmux mutation lock.
func replaceQueuedAGMInputLocked(ctx context.Context, targetPane, command string, runtime queuedAGMRecoveryRuntime) (HarnessInputReadiness, error) {
	clearSteps := []struct {
		key   string
		pause time.Duration
	}{
		{key: "C-c", pause: 200 * time.Millisecond},
		{key: "C-u", pause: 100 * time.Millisecond},
		{key: "C-a"},
		{key: "C-k", pause: 300 * time.Millisecond},
	}
	for _, step := range clearSteps {
		if err := runtime.sendKey(ctx, targetPane, step.key); err != nil {
			return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, Forced: true}, fmt.Errorf("clear queued AGM input with %s: %w", step.key, err)
		}
		if step.pause > 0 {
			if err := runtime.wait(ctx, step.pause); err != nil {
				return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, Forced: true}, fmt.Errorf("wait for queued AGM input clear: %w", err)
			}
		}
	}

	cleared, err := runtime.recheck()
	if err != nil {
		return HarnessInputReadiness{State: HarnessInputQueuedAGM, TargetPane: targetPane, Forced: true}, fmt.Errorf("recheck cleared queued AGM input: %w", err)
	}
	cleared.Forced = true
	if cleared.TargetPane != targetPane {
		cleared.Ready = false
		return cleared, fmt.Errorf("queued AGM input moved from verified pane %q to %q while clearing", targetPane, cleared.TargetPane)
	}
	if !cleared.Ready || cleared.State != HarnessInputReady {
		cleared.Ready = false
		return cleared, fmt.Errorf("queued AGM input was not cleared on verified pane %q: state %s", targetPane, cleared.State)
	}
	if err := runtime.deliver(ctx, targetPane, command); err != nil {
		cleared.Ready = false
		return cleared, fmt.Errorf("deliver replacement to cleared pane %q: %w", targetPane, err)
	}
	return cleared, nil
}

func inputDeliveryAllowed(readiness HarnessInputReadiness, options InputDeliveryOptions) (allowed, forced bool) {
	if readiness.Ready {
		return true, false
	}
	if options.AllowQueuedAGM && readiness.State == HarnessInputQueuedAGM {
		return true, true
	}
	return false, false
}

// ClassifyHarnessInput is the pure composer classifier. Readiness is scoped to
// the configured harness. Queue identity uses the complete joined logical
// composer; blockers and empty-composer readiness remain scoped to the pane
// tail that currently owns input.
func ClassifyHarnessInput(content, harness string) (bool, string, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return false, "", err
	}
	styledTail := paneRawInputTail(content, 12)
	tail := stripANSI(styledTail)
	queuedInput, _ := classifyCurrentQueuedInput(content, harness)

	// Pi's managed ready footer remains visible while its native confirmation
	// dialog owns input. Treat that dialog as authoritative before consulting
	// the footer so automated input can never become an authorization answer.
	if harness == "pi-cli" && hasPiManagedPermissionPrompt(tail) {
		return false, HarnessInputPermission, nil
	}

	var ready bool
	switch harness {
	case "claude-code":
		ready = hasTailOwnedClaudeComposer(styledTail)
	case "codex-cli":
		ready = isCodexInputComposerReady(tail)
	case "agy":
		ready = hasTailOwnedAgyComposer(tail)
	case "gemini-cli":
		ready = hasTailOwnedGeminiComposer(tail)
	case "opencode-cli":
		ready = hasTailOwnedOpenCodeComposer(tail)
	case "pi-cli":
		ready = containsPiReadyPattern(tail)
	}
	if ready && queuedInput == QueuedInputNone {
		return true, HarnessInputReady, nil
	}
	if hasInputOverlay(tail, harness) {
		return false, HarnessInputOverlay, nil
	}
	if hasOnboardingPrompt(tail, harness) {
		return false, HarnessInputOnboarding, nil
	}
	if hasTailOwnedPermissionPrompt(tail) {
		return false, HarnessInputPermission, nil
	}
	if queuedInput == QueuedInputAGM {
		return false, HarnessInputQueuedAGM, nil
	}
	return false, HarnessInputBusy, nil
}

// classifyCurrentQueuedInput scopes queue evidence to the registered harness's
// latest composer. It downgrades an otherwise valid AGM header to human-owned
// input unless the marker is on the current occupied composer and idle chrome
// still owns the pane tail.
func classifyCurrentQueuedInput(content, harness string) (QueuedInputType, string) {
	region, anchorLine, ok := currentComposerInputRegion(content, harness)
	if !ok {
		return QueuedInputNone, ""
	}
	inputType, description := ClassifyQueuedInput(stripANSI(region))
	if inputType != QueuedInputAGM {
		if harness == "pi-cli" && inputType != QueuedInputNone && piQueuedMarkerIsHistorical(region) {
			return QueuedInputNone, ""
		}
		return inputType, description
	}
	if harness == "pi-cli" && piQueuedMarkerIsHistorical(region) {
		return QueuedInputNone, ""
	}
	if !hasQueuedInputMarker(stripANSI(anchorLine)) || !queuedComposerOwnsTail(region, content, harness) {
		return QueuedInputHuman, "session has human input in progress - not sending. Retry later"
	}
	return inputType, description
}

func currentComposerInputRegion(content, harness string) (region, anchorLine string, ok bool) {
	lines := strings.Split(content, "\n")
	anchor := -1
	for i, line := range lines {
		plain := strings.TrimSpace(stripANSI(line))
		if isCurrentComposerAnchor(plain, harness) {
			candidate := strings.Join(lines[i:], "\n")
			// Once a structural pasted-content marker owns the pane tail, all
			// following lines are payload. Prompt glyphs inside that payload must
			// not replace the composer anchor that introduced it.
			if hasQueuedInputMarker(plain) && queuedComposerOwnsTail(candidate, content, harness) {
				return candidate, lines[i], true
			}
			anchor = i
		}
	}
	if anchor < 0 {
		return "", "", false
	}
	return strings.Join(lines[anchor:], "\n"), lines[anchor], true
}

func isCurrentComposerAnchor(line, harness string) bool {
	switch harness {
	case "claude-code":
		return strings.HasPrefix(line, "❯")
	case "codex-cli":
		return isCodexComposerAnchor(line)
	case "agy":
		return line == ">" || strings.HasPrefix(line, "> ") && hasQueuedInputMarker(line)
	case "gemini-cli":
		return isGeminiComposerAnchor(line)
	case "opencode-cli":
		return isOpenCodeComposerAnchor(line)
	case "pi-cli":
		return hasQueuedInputMarker(line)
	default:
		return false
	}
}

func isCodexComposerAnchor(line string) bool {
	return line == "›" || strings.HasPrefix(line, "› ") || line == ">" || strings.HasPrefix(line, "> [Pasted Content")
}

func queuedComposerOwnsTail(region, content, harness string) bool {
	plainRegion := stripANSI(region)
	if hasActiveSpinner(plainRegion) || strings.Contains(strings.ToLower(plainRegion), "esc to interrupt") {
		return false
	}
	lines := strings.Split(strings.TrimSpace(plainRegion), "\n")
	if len(lines) == 0 {
		return false
	}
	switch harness {
	case "claude-code":
		return hasTerminalIdleFooter(lines, isClaudeIdleFooter) && queuedPastePayloadOwnsTail(lines, isClaudeIdleComposerChrome)
	case "codex-cli":
		// Codex keeps its model footer visible while a turn is active, and a
		// queued paste replaces the empty cursor that would otherwise prove idle
		// ownership. Only the compact first-turn composer can therefore prove the
		// queue was pasted before any work began; post-turn queues fail closed.
		return codexInitialQueuedComposerOwnsTail(stripANSI(content))
	case "agy":
		return queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "gemini-cli":
		return hasTerminalIdleFooter(lines, isGeminiIdleFooter) && queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "opencode-cli":
		return queuedPastePayloadOwnsTail(lines, isTerminalIdleChrome)
	case "pi-cli":
		return hasTerminalIdleFooter(lines, isPiReadyFooter) && queuedPastePayloadOwnsTail(lines, isPiIdleComposerChrome)
	}
	return false
}

var queuedPastedTextLinePattern = regexp.MustCompile(`\[Pasted text(?: #\d+)? \+(\d+) lines?\]`)
var codexQueuedPasteCharPattern = regexp.MustCompile(`^[›>]\s+\[Pasted Content (\d+) chars\]$`)

func queuedPastePayloadOwnsTail(lines []string, isChrome func(string) bool) bool {
	if len(lines) == 0 {
		return false
	}
	matches := queuedPastedTextLinePattern.FindStringSubmatch(lines[0])
	if matches == nil {
		return false
	}
	want, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	payload := lines[1:]
	for len(payload) > 0 && isChrome(payload[len(payload)-1]) {
		payload = payload[:len(payload)-1]
	}
	return len(payload) == want
}

func hasTerminalIdleFooter(lines []string, isFooter func(string) bool) bool {
	for _, line := range slices.Backward(lines) {
		if isDecorativeChromeLine(line) {
			continue
		}
		return isFooter(line)
	}
	return false
}

func isClaudeIdleFooter(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(stripANSI(line)))
	for _, marker := range []string{"? for shortcuts", "shift+tab to cycle", "bypass permissions on", "accept edits on", "plan mode on"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func isGeminiIdleFooter(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(stripANSI(line)))
	return strings.Contains(lower, "? for shortcuts") || strings.Contains(lower, "sandbox") && strings.Contains(lower, "gemini-")
}

func isPiIdleComposerChrome(line string) bool {
	plain := strings.TrimSpace(stripANSI(line))
	if isDecorativeChromeLine(plain) {
		return true
	}
	return PiManagedState(plain) != "" || strings.Contains(plain, " • pi-") || strings.Contains(plain, "%/")
}

func isPiReadyFooter(line string) bool {
	return PiManagedState(stripANSI(line)) == "ready"
}

func isDecorativeChromeLine(line string) bool {
	line = strings.TrimSpace(stripANSI(line))
	return line == "" || strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == ""
}

func isGeminiComposerAnchor(line string) bool {
	line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
	return strings.HasPrefix(line, ">") && (strings.Contains(strings.ToLower(line), "type your message") || hasQueuedInputMarker(line))
}

func isOpenCodeComposerAnchor(line string) bool {
	line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
	switch line {
	case ">", ">>", "❯":
		return true
	}
	lower := strings.ToLower(line)
	if strings.HasPrefix(line, ">") && (strings.Contains(lower, "type your message") || strings.Contains(lower, "type here") || hasQueuedInputMarker(line)) {
		return true
	}
	return strings.HasPrefix(line, "❯") && (line == "❯" || hasQueuedInputMarker(line))
}

func piQueuedMarkerIsHistorical(region string) bool {
	lines := strings.Split(strings.TrimSpace(stripANSI(region)), "\n")
	return hasTerminalIdleFooter(lines, isPiReadyFooter) && !queuedPastePayloadOwnsTail(lines, isPiIdleComposerChrome)
}

func codexInitialQueuedComposerOwnsTail(content string) bool {
	// capture-pane -p appends one framing newline. Remove only that terminator:
	// trailing whitespace inside the queued payload contributes to Codex's
	// native pasted-character extent and must remain observable.
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, line := range lines {
		if !strings.Contains(line, CodexPromptPatterns[0]) {
			continue
		}
		for j := i + 1; j < len(lines) && j <= i+4; j++ {
			if !strings.Contains(lines[j], CodexPromptPatterns[1]) {
				continue
			}
			for k, tailLine := range lines[j+1:] {
				candidate := strings.TrimSpace(tailLine)
				switch {
				case candidate == "":
				case strings.HasPrefix(candidate, "│") && strings.HasSuffix(candidate, "│"):
				case strings.HasPrefix(candidate, "╰") && strings.HasSuffix(candidate, "╯"):
				case (strings.HasPrefix(candidate, "› ") || strings.HasPrefix(candidate, "> ")) && hasQueuedInputMarker(candidate):
					return codexQueuedPastePayloadOwnsTail(lines[j+1+k:])
				default:
					return false
				}
			}
		}
	}
	return false
}

func codexQueuedPastePayloadOwnsTail(lines []string) bool {
	if len(lines) < 2 {
		return false
	}
	matches := codexQueuedPasteCharPattern.FindStringSubmatch(strings.TrimSpace(lines[0]))
	if matches == nil {
		return false
	}
	want, err := strconv.Atoi(matches[1])
	if err != nil {
		return false
	}
	return utf8.RuneCountInString(strings.Join(lines[1:], "\n")) == want
}

// CheckExpectedHarnessLiveness scans the exact session's process tree and
// accepts only processes compatible with the configured harness. Node-backed
// harnesses must carry a harness-specific script/package identity in argv;
// an unrelated Node descendant is not liveness proof.
func CheckExpectedHarnessLiveness(ctx context.Context, sessionName, harness string) (PaneLiveness, error) {
	if err := validateReadinessHarness(harness); err != nil {
		return PaneLiveness{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	scanCtx, cancel := context.WithTimeout(ctx, livenessScanTimeout)
	defer cancel()
	pane, exists, err := resolveActivePaneTarget(scanCtx, sessionName, GetSocketPath())
	if err != nil {
		return PaneLiveness{}, err
	}
	if !exists {
		return PaneLiveness{SessionExists: false}, nil
	}
	return checkExpectedHarnessLivenessForPane(scanCtx, pane, harness)
}

func checkExpectedHarnessLivenessForPane(ctx context.Context, pane activePaneTarget, harness string) (PaneLiveness, error) {
	procs, err := readProcessTableWithArgs(ctx)
	if err != nil {
		return PaneLiveness{}, err
	}
	return classifyPaneLivenessProcesses([]int{pane.RootPID}, procs, expectedHarnessProcessMatcher(harness)), nil
}

// WaitForExpectedHarnessReady owns startup readiness for shared operations.
// It permits documented first-run transitions, but otherwise only observes.
func WaitForExpectedHarnessReady(ctx context.Context, sessionName, harness string, timeout time.Duration) error {
	if err := validateReadinessHarness(harness); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	observedHarness := false
	advanced := make(map[string]bool)
	for {
		readiness, err := CheckExpectedHarnessInput(waitCtx, sessionName, harness)
		if err != nil {
			if waitCtx.Err() != nil {
				return fmt.Errorf("wait for %s readiness in %q: %w", harness, sessionName, waitCtx.Err())
			}
			return fmt.Errorf("check %s readiness in %q: %w", harness, sessionName, err)
		}
		ready, err := handleHarnessStartupState(waitCtx, sessionName, harness, readiness, &observedHarness, advanced)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		select {
		case <-waitCtx.Done():
			return fmt.Errorf("timeout waiting for %s readiness in %q (last state %s): %w", harness, sessionName, readiness.State, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func handleHarnessStartupState(
	ctx context.Context,
	sessionName, harness string,
	readiness HarnessInputReadiness,
	observedHarness *bool,
	advanced map[string]bool,
) (bool, error) {
	switch readiness.State {
	case HarnessInputReady:
		*observedHarness = true
		return true, nil
	case HarnessInputNotFound:
		return false, fmt.Errorf("tmux session %q disappeared while waiting for %s readiness", sessionName, harness)
	case HarnessInputWrongHarness:
		if *observedHarness {
			return false, fmt.Errorf("expected %s process stopped in tmux session %q after startup", harness, sessionName)
		}
	case HarnessInputOnboarding, HarnessInputOverlay:
		*observedHarness = true
		if !canAdvanceHarnessStartup(readiness.State, harness, readiness.Content) {
			return false, nil
		}
		transition := readiness.State + ":" + onboardingKind(readiness.Content, harness)
		if !advanced[transition] {
			if err := advanceHarnessStartup(ctx, readiness.TargetPane, harness, readiness.Content); err != nil {
				return false, fmt.Errorf("advance %s startup in %q: %w", harness, sessionName, err)
			}
			advanced[transition] = true
		}
	default:
		*observedHarness = true
	}
	return false, nil
}

func validateReadinessHarness(harness string) error {
	switch harness {
	case "claude-code", "codex-cli", "agy", "gemini-cli", "opencode-cli", "pi-cli":
		return nil
	default:
		return fmt.Errorf("unsupported harness readiness check %q", harness)
	}
}

func expectedHarnessProcessMatcher(harness string) func(ProcEntry) bool {
	return func(process ProcEntry) bool {
		if !processOwnsForegroundTerminal(process) {
			return false
		}
		base := filepath.Base(strings.TrimSpace(process.Comm))
		return processBaseMatchesHarness(base, harness) ||
			nodeProcessMatchesHarness(process.Args, harness)
	}
}

// processOwnsForegroundTerminal rejects a matching harness that is merely a
// stopped or background descendant. Only a process in the terminal's current
// foreground process group can own the composer that will receive input.
func processOwnsForegroundTerminal(process ProcEntry) bool {
	return process.PGID > 0 && process.TPGID == process.PGID &&
		!strings.Contains(strings.ToUpper(process.State), "T")
}

func processBaseMatchesHarness(base, harness string) bool {
	switch harness {
	case "claude-code":
		return base == "claude" || isClaudeProcess(base)
	case "codex-cli":
		return base == "codex"
	case "agy":
		return base == "agy"
	case "gemini-cli":
		return base == "gemini"
	case "opencode-cli":
		return base == "opencode"
	case "pi-cli":
		return base == "pi"
	default:
		return false
	}
}

func nodeProcessMatchesHarness(args, harness string) bool {
	if harness == "pi-cli" {
		return isPiProcessCommand(args)
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return false
	}
	executable := 0
	if filepath.Base(strings.Trim(fields[0], "'\"")) == "env" {
		executable++
		for executable < len(fields) && strings.Contains(fields[executable], "=") {
			executable++
		}
	}
	if executable >= len(fields) || filepath.Base(strings.Trim(fields[executable], "'\"")) != "node" {
		return false
	}
	script, mustResolve, ok := nodeScriptArgument(args, fields, executable)
	if !ok {
		return false
	}
	if mustResolve {
		resolved, err := filepath.EvalSymlinks(script)
		if err != nil {
			return false
		}
		script = resolved
	}
	script = strings.ToLower(filepath.ToSlash(script))
	patterns := map[string][]string{
		"claude-code":  {"@anthropic-ai/claude-code", "/claude-code/", "/bin/claude", "claude.js"},
		"codex-cli":    {"@openai/codex", "/codex/bin/", "/bin/codex", "codex.js"},
		"gemini-cli":   {"@google/gemini-cli", "/gemini-cli/", "/bin/gemini", "gemini.js"},
		"opencode-cli": {"opencode-ai", "/opencode/bin/", "/bin/opencode", "opencode.js"},
	}
	for _, pattern := range patterns[harness] {
		if strings.Contains(script, pattern) {
			return true
		}
	}
	return false
}

func paneRawInputTail(content string, maxLines int) string {
	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func hasTailOwnedClaudeComposer(content string) bool {
	lines := strings.Split(content, "\n")
	promptIndex := -1
	for i, line := range lines {
		plainLine := strings.TrimSpace(stripANSI(line))
		if plainLine == "❯" || strings.HasPrefix(plainLine, "❯") && HasGhostTextInANSI(line) {
			promptIndex = i
		}
	}
	if promptIndex < 0 {
		return false
	}
	for _, line := range lines[promptIndex+1:] {
		if !isClaudeIdleComposerChrome(line) {
			return false
		}
	}
	return true
}

func isClaudeIdleComposerChrome(line string) bool {
	line = strings.TrimSpace(stripANSI(line))
	if line == "" {
		return true
	}
	if strings.Trim(line, "─━┄┈╌╍ ") == "" {
		return true
	}
	lower := strings.ToLower(line)
	if strings.Contains(lower, "esc to interrupt") {
		return false
	}
	for _, marker := range []string{"? for shortcuts", "shift+tab to cycle", "bypass permissions on", "accept edits on", "plan mode on"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func hasTailOwnedGeminiComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		return strings.HasPrefix(line, ">") && strings.Contains(strings.ToLower(line), "type your message")
	})
}

func hasTailOwnedAgyComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		return strings.TrimSpace(line) == ">"
	})
}

func hasTailOwnedOpenCodeComposer(content string) bool {
	return hasTailOwnedTextComposer(content, func(line string) bool {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		switch line {
		case ">", ">>", "❯":
			return true
		}
		lower := strings.ToLower(line)
		return strings.HasPrefix(line, ">") &&
			(strings.Contains(lower, "type your message") || strings.Contains(lower, "type here"))
	})
}

func hasTailOwnedTextComposer(content string, isComposer func(string) bool) bool {
	lines := strings.Split(content, "\n")
	composerIndex := -1
	for i, line := range lines {
		if isComposer(line) {
			composerIndex = i
		}
	}
	if composerIndex < 0 {
		return false
	}
	for _, line := range lines[composerIndex+1:] {
		if !isTerminalIdleChrome(line) {
			return false
		}
	}
	return true
}

func isTerminalIdleChrome(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	lower := strings.ToLower(line)
	for _, marker := range []string{"? for shortcuts", "shift+tab to", "accept edits"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "sandbox") && strings.Contains(lower, "gemini-")
}

func isCodexInputComposerReady(content string) bool {
	return IsCodexComposerReady(content)
}

var permissionChoicePattern = regexp.MustCompile(`(?i)^\s*(?:[❯>•]\s*(?:\d+[.)]\s*)?|\d+[.)]\s+)(yes|no|allow(?:\s+once|\s+always)?|deny|approve|reject|cancel|don't allow)\b`)

func hasTailOwnedPermissionPrompt(content string) bool {
	lines := strings.Split(content, "\n")
	anchor := -1
	for i, line := range lines {
		lower := strings.ToLower(line)
		for _, marker := range []string{
			"do you want to proceed?",
			"allow this command",
			"allow this action",
			"approve this action",
		} {
			if strings.Contains(lower, marker) {
				anchor = i
				break
			}
		}
	}
	if anchor >= 0 && permissionChoicesOwnTail(lines[anchor+1:]) {
		return true
	}
	return unanchoredPermissionChoicesOwnTail(lines)
}

func hasPiManagedPermissionPrompt(content string) bool {
	return strings.Contains(strings.ToLower(content), "agm permission required") ||
		hasTailOwnedPermissionPrompt(content)
}

func permissionChoicesOwnTail(lines []string) bool {
	hasChoice := false
	for _, line := range lines {
		if _, ok := permissionChoiceKind(line); ok {
			hasChoice = true
			continue
		}
		if isPermissionPromptChrome(line) {
			continue
		}
		return false
	}
	return hasChoice
}

func unanchoredPermissionChoicesOwnTail(lines []string) bool {
	var allow, deny, structured bool
	for _, line := range slices.Backward(lines) {
		if isPermissionPromptChrome(line) {
			continue
		}
		kind, ok := permissionChoiceKind(line)
		if !ok {
			break
		}
		structured = true
		switch kind {
		case "yes", "allow", "approve":
			allow = true
		case "no", "deny", "reject", "cancel", "don't allow":
			deny = true
		}
	}
	return structured && allow && deny
}

func permissionChoiceKind(line string) (string, bool) {
	match := permissionChoicePattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(match) != 2 {
		return "", false
	}
	kind := strings.ToLower(match[1])
	if strings.HasPrefix(kind, "allow") {
		kind = "allow"
	}
	return kind, true
}

func isPermissionPromptChrome(line string) bool {
	line = strings.TrimSpace(line)
	if line == "" {
		return true
	}
	if strings.Trim(line, "─━┄┈╌╍═│┃┆┊╎╏┌┐└┘├┤┬┴┼╭╮╰╯ ") == "" {
		return true
	}
	lower := strings.ToLower(line)
	return strings.Contains(lower, "enter to confirm") ||
		strings.Contains(lower, "esc to cancel") ||
		strings.Contains(lower, "use arrow keys")
}

func hasInputOverlay(content, harness string) bool {
	if strings.Contains(content, "Background tasks") && strings.Contains(content, "to close") {
		return true
	}
	return harness == "agy" && ContainsAgySurveyPrompt(content)
}

func hasOnboardingPrompt(content, harness string) bool {
	switch harness {
	case "claude-code":
		return containsTrustPromptPattern(content)
	case "codex-cli":
		return containsCodexTrustPromptPattern(content) || containsCodexModelUpgradePromptPattern(content)
	case "agy":
		return containsAgyTrustPromptPattern(content)
	case "gemini-cli":
		return containsGeminiTrustPromptPattern(content)
	default:
		return false
	}
}

func containsGeminiTrustPromptPattern(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "do you trust the files in this folder") ||
		strings.Contains(lower, "do you trust this folder")
}

func onboardingKind(content, harness string) string {
	if harness == "codex-cli" && containsCodexModelUpgradePromptPattern(content) {
		return "model-upgrade"
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return "survey"
	}
	return "trust"
}

func canAdvanceHarnessStartup(state, harness, content string) bool {
	return state == HarnessInputOnboarding ||
		(state == HarnessInputOverlay && harness == "agy" && ContainsAgySurveyPrompt(content))
}

func advanceHarnessStartup(ctx context.Context, targetPane, harness, content string) error {
	if !isPaneID(targetPane) {
		return fmt.Errorf("missing verified pane for %s startup transition", harness)
	}
	for _, key := range harnessStartupAdvanceKeys(harness, content) {
		if err := sendReadinessKey(ctx, targetPane, key); err != nil {
			return err
		}
	}
	return nil
}

func harnessStartupAdvanceKeys(harness, content string) []string {
	if harness == "codex-cli" && containsCodexModelUpgradePromptPattern(content) {
		return []string{"Down", "Enter"}
	}
	if harness == "agy" && ContainsAgySurveyPrompt(content) {
		return []string{"0"}
	}
	if harness == "gemini-cli" && containsGeminiTrustPromptPattern(content) {
		return []string{"1", "Enter"}
	}
	return []string{"Enter"}
}

func sendReadinessKey(ctx context.Context, targetPane, key string) error {
	_, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", GetSocketPath(), "send-keys", "-t", targetPane, key)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
