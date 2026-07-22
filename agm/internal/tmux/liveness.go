package tmux

// Harness-process liveness for tmux panes (ce-axsr).
//
// `tmux has-session` and a fresh heartbeat file are both false-green signals:
// the tmux session keeps existing after the harness (claude/codex/agy/…)
// exits and the pane falls back to a bare shell, and an orphaned writer
// process can keep a heartbeat file fresh long after the harness died
// (ce-qkf7: an orphaned agm child wrote meta-o's heartbeat for ~3h while the
// only pane processes were zsh + that agm child).
//
// Real liveness is "a harness process is actually running in the pane's
// process tree". This file owns that scan: resolve the session's pane PIDs,
// read the process table once, walk the descendant tree, and classify. The
// classification core is pure (no tmux, no ps) so it is table-testable with
// fakes.

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// livenessScanTimeout bounds the tmux + ps round-trips for one scan.
const livenessScanTimeout = 2 * time.Second

// ProcEntry is one row of a process table. PGID, TPGID, and State are populated
// for input-readiness scans, which must distinguish a foreground harness from
// a stopped or background descendant. Comm may be a bare name or a full path.
type ProcEntry struct {
	PID   int
	PPID  int
	PGID  int
	TPGID int
	State string
	Comm  string
	Args  string
}

// procCommandEntry is one process-table row with the full command line. Pi's
// npm entrypoint is a Node script, so comm alone is not always sufficient to
// distinguish it from another Node-hosted harness.
type procCommandEntry struct {
	PID           int
	PPID          int
	Command       string
	Args          []string
	ArgvInspected bool
}

// PaneLiveness is the verdict of a harness-process liveness scan for one
// tmux session.
type PaneLiveness struct {
	// SessionExists reports whether the tmux session (and at least one pane)
	// was found at all. When false, every other field is zero-valued: the
	// scan can prove nothing about a session it cannot see.
	SessionExists bool
	// HarnessAlive reports whether a harness process (claude, codex, agy,
	// opencode, pi, node, …) is running anywhere in a pane's descendant process
	// tree.
	// This is the only signal that proves the session is genuinely alive.
	HarnessAlive bool
	// ZombieWriter reports the ce-qkf7 failure mode: no harness process is
	// alive, but an agm process is still running in the pane tree — the
	// likely orphaned writer keeping a heartbeat file falsely fresh.
	ZombieWriter bool
	// RestartableShell reports that the session has exactly one pane and every
	// process in its descendant tree is a plain interactive shell. Callers may
	// safely deliver a cold-resume command only when this positive proof is true.
	RestartableShell bool
	// Evidence is a human-readable summary of the pane's descendant process
	// names, so callers can say WHY a session was classified dead.
	Evidence string
}

// harnessComms is the set of process names that count as a live harness
// foreground. "node" is included because Node-based CLIs (Codex, some Claude
// installs) report as node.
var harnessComms = map[string]bool{
	"claude":   true,
	"codex":    true,
	"agy":      true,
	"node":     true,
	"gemini":   true,
	"opencode": true,
	"pi":       true,
}

// IsHarnessComm reports whether a process comm value names a known harness
// binary. Comm may be a full path; only the base name is matched. Claude
// Code's semver process names (e.g. "2.1.50", macOS "2_1_195") also count.
func IsHarnessComm(comm string) bool {
	base := filepath.Base(strings.TrimSpace(comm))
	if harnessComms[base] {
		return true
	}
	return isClaudeProcess(base)
}

// ParsePSTable parses `ps -axo pid=,ppid=,comm=` output into ProcEntry rows.
// Comm is split at the first whitespace after the two numeric columns only,
// so command paths containing spaces survive intact. Malformed lines are
// skipped.
func ParsePSTable(out string) []ProcEntry {
	var entries []ProcEntry
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// pid
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:idx])
		if err != nil {
			continue
		}
		rest := strings.TrimSpace(line[idx:])
		// ppid
		idx = strings.IndexAny(rest, " \t")
		if idx < 0 {
			continue
		}
		ppid, err := strconv.Atoi(rest[:idx])
		if err != nil {
			continue
		}
		comm := strings.TrimSpace(rest[idx:])
		if comm == "" {
			continue
		}
		entries = append(entries, ProcEntry{PID: pid, PPID: ppid, Comm: comm})
	}
	return entries
}

// ParsePSForegroundTable parses the process identity and terminal-ownership
// fields used by readiness scans. The first five columns are fixed-width
// scalar fields; the remainder is the comm value and may contain spaces.
func ParsePSForegroundTable(out string) []ProcEntry {
	var entries []ProcEntry
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, pidErr := strconv.Atoi(fields[0])
		ppid, ppidErr := strconv.Atoi(fields[1])
		pgid, pgidErr := strconv.Atoi(fields[2])
		tpgid, tpgidErr := strconv.Atoi(fields[3])
		if pidErr != nil || ppidErr != nil || pgidErr != nil || tpgidErr != nil {
			continue
		}
		entries = append(entries, ProcEntry{
			PID: pid, PPID: ppid, PGID: pgid, TPGID: tpgid,
			State: fields[4], Comm: strings.Join(fields[5:], " "),
		})
	}
	return entries
}

// ClassifyPaneLiveness is the pure classification core: given the session's
// pane PIDs and a process table, it walks the full descendant tree of each
// pane (not just direct children — a harness that crashed and was resumed
// runs as a grandchild under a shell) and classifies the session.
//
// isHarness decides which comm values count as a live harness; pass
// IsHarnessComm for the standard set. The pane processes themselves (the
// shells tmux spawned) are included in the walk, so a pane whose root
// process IS the harness classifies as alive.
func ClassifyPaneLiveness(panePIDs []int, procs []ProcEntry, isHarness func(comm string) bool) PaneLiveness {
	return classifyPaneLivenessProcesses(panePIDs, procs, func(process ProcEntry) bool {
		return isHarness(process.Comm)
	})
}

func classifyPaneLivenessProcesses(panePIDs []int, procs []ProcEntry, isHarness func(ProcEntry) bool) PaneLiveness {
	if len(panePIDs) == 0 {
		return PaneLiveness{SessionExists: false}
	}
	children := make(map[int][]ProcEntry, len(procs))
	byPID := make(map[int]ProcEntry, len(procs))
	for _, p := range procs {
		children[p.PPID] = append(children[p.PPID], p)
		byPID[p.PID] = p
	}

	verdict := PaneLiveness{SessionExists: true, RestartableShell: len(panePIDs) == 1}
	var comms []string
	seen := make(map[int]bool)
	processSeen := false
	queue := make([]int, 0, len(panePIDs))
	for _, pid := range panePIDs {
		if p, ok := byPID[pid]; ok {
			queue = append(queue, pid)
			comms = append(comms, filepath.Base(p.Comm))
		}
	}
	for i := 0; i < len(queue); i++ {
		pid := queue[i]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		p, ok := byPID[pid]
		if !ok {
			continue
		}
		processSeen = true
		observePaneProcess(&verdict, p, isHarness)
		for _, c := range children[pid] {
			if !seen[c.PID] {
				queue = append(queue, c.PID)
				comms = append(comms, filepath.Base(c.Comm))
			}
		}
	}
	// A zombie writer only matters as a verdict when no harness is alive:
	// with a live harness, an agm process in the tree is just normal tooling.
	if verdict.HarnessAlive {
		verdict.ZombieWriter = false
	}
	if !processSeen {
		verdict.RestartableShell = false
	}
	const maxEvidence = 200
	verdict.Evidence = strings.Join(comms, ",")
	// Truncate on a rune boundary: comm values may be paths containing
	// multi-byte UTF-8, and slicing by byte index could produce invalid UTF-8.
	if runes := []rune(verdict.Evidence); len(runes) > maxEvidence {
		verdict.Evidence = string(runes[:maxEvidence]) + "..."
	}
	return verdict
}

func observePaneProcess(verdict *PaneLiveness, process ProcEntry, isHarness func(ProcEntry) bool) {
	if !IsShellCommand(process.Comm) {
		verdict.RestartableShell = false
	}
	if isHarness(process) {
		verdict.HarnessAlive = true
	} else if filepath.Base(process.Comm) == "agm" {
		verdict.ZombieWriter = true
	}
}

// ParsePSArgsTable parses `ps -axo pid=,args=` output into full command lines
// keyed by PID. Malformed rows are skipped so a partial process table cannot
// accidentally attach arguments to the wrong process.
func ParsePSArgsTable(out string) map[int]string {
	entries := make(map[int]string)
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.IndexAny(line, " \t")
		if idx < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:idx])
		if err != nil {
			continue
		}
		args := strings.TrimSpace(line[idx:])
		if args != "" {
			entries[pid] = args
		}
	}
	return entries
}

type activePaneTarget struct {
	ID      string
	RootPID int
}

// resolveActivePaneTarget resolves the one pane that an unqualified tmux
// session target would currently receive input in. Callers retain its pane ID
// so later capture and delivery cannot drift if pane focus changes.
func resolveActivePaneTarget(ctx context.Context, sessionName, socketPath string) (activePaneTarget, bool, error) {
	normalized := NormalizeTmuxSessionName(sessionName)
	exists, err := tmuxSessionExistsOnSocket(ctx, normalized, socketPath)
	if err != nil || !exists {
		return activePaneTarget{}, exists, err
	}
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-t", FormatSessionTarget(normalized), "-f", "#{pane_active}", "-F", "#{pane_id}\t#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return activePaneTarget{}, true, fmt.Errorf("resolve active tmux pane: %w", err)
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) != 2 || !isPaneID(fields[0]) {
		return activePaneTarget{}, true, fmt.Errorf("invalid active tmux pane identity %q", strings.TrimSpace(string(out)))
	}
	pid, err := strconv.Atoi(fields[1])
	if err != nil || pid <= 0 {
		return activePaneTarget{}, true, fmt.Errorf("invalid active tmux pane PID %q", fields[1])
	}
	return activePaneTarget{ID: fields[0], RootPID: pid}, true, nil
}

// listPanePIDs returns the pane root PIDs for sessionName on socketPath.
// A missing session returns (nil, nil) — absence is a verdict, not an error.
// Only a clean non-zero exit from tmux ("no such session") counts as absence:
// a timeout, a missing tmux binary, or any other execution failure returns an
// error so callers fail safe instead of misreading "could not check" as
// "session is dead".
func listPanePIDs(ctx context.Context, sessionName, socketPath string) ([]int, error) {
	normalized := NormalizeTmuxSessionName(sessionName)
	exists, err := tmuxSessionExistsOnSocket(ctx, normalized, socketPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-s", "-t", FormatSessionTarget(normalized), "-F", "#{pane_pid}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tmux list-panes: %w", err)
	}
	var pids []int
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, convErr := strconv.Atoi(line)
		if convErr != nil {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

func tmuxSessionExistsOnSocket(ctx context.Context, normalizedName, socketPath string) (bool, error) {
	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "has-session", "-t", FormatSessionTarget(normalizedName))
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return false, fmt.Errorf("tmux has-session timed out: %w", ctx.Err())
	}
	return tmuxSessionExistenceResult(normalizedName, output, err)
}

func tmuxSessionExistenceResult(normalizedName string, output []byte, err error) (bool, error) {
	if err == nil {
		return true, nil
	}
	if isMissingSessionOutput(output) {
		return false, nil
	}
	return false, tmuxCommandError("check session", normalizedName, output, err)
}

// readProcessTable runs `ps -axo pid=,ppid=,comm=` and parses the result.
func readProcessTable(ctx context.Context) ([]ProcEntry, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps: %w", err)
	}
	return ParsePSTable(string(out)), nil
}

func readProcessCommandTable(ctx context.Context) ([]procCommandEntry, error) {
	cmd := exec.CommandContext(ctx, "ps", "-ww", "-axo", "pid=,ppid=,command=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps command table: %w", err)
	}
	entries := parsePSCommandTable(string(out))
	if err := inspectNodeProcessArgs(ctx, entries, readProcessArgv); err != nil {
		return nil, err
	}
	return entries, nil
}

func inspectNodeProcessArgs(ctx context.Context, entries []procCommandEntry, readArgv func(int) ([]string, error)) error {
	for index := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("read process argv: %w", err)
		}
		if processCommandExecutable(entries[index].Command) != "node" {
			continue
		}
		entries[index].ArgvInspected = true
		args, argvErr := readArgv(entries[index].PID)
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("read process argv: %w", err)
		}
		if argvErr == nil {
			entries[index].Args = args
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("read process argv: %w", err)
	}
	return nil
}

func processCommandExecutable(command string) string {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return ""
	}
	executable := 0
	if filepath.Base(strings.Trim(fields[0], "'\"")) == "env" {
		executable++
		for executable < len(fields) && strings.Contains(fields[executable], "=") {
			executable++
		}
	}
	if executable >= len(fields) {
		return ""
	}
	return filepath.Base(strings.Trim(fields[executable], "'\""))
}

func parsePSCommandTable(out string) []procCommandEntry {
	var entries []procCommandEntry
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		first := strings.IndexAny(line, " \t")
		if first < 0 {
			continue
		}
		pid, err := strconv.Atoi(line[:first])
		if err != nil {
			continue
		}
		rest := strings.TrimSpace(line[first:])
		second := strings.IndexAny(rest, " \t")
		if second < 0 {
			continue
		}
		ppid, err := strconv.Atoi(rest[:second])
		if err != nil {
			continue
		}
		command := strings.TrimSpace(rest[second:])
		if command == "" {
			continue
		}
		entries = append(entries, procCommandEntry{PID: pid, PPID: ppid, Command: command})
	}
	return entries
}

func isPiProcessCommand(command string) bool {
	return isPiProcessCommandWithResolver(command, filepath.EvalSymlinks)
}

func isPiProcessCommandWithResolver(command string, resolve func(string) (string, error)) bool {
	fields := strings.Fields(command)
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
	if executable >= len(fields) {
		return false
	}
	base := filepath.Base(strings.Trim(fields[executable], "'\""))
	if base == "pi" {
		return true
	}
	if base != "node" {
		return false
	}
	script, mustResolve, ok := nodeScriptArgument(command, fields, executable)
	if !ok {
		return false
	}
	if !mustResolve && isPiPackageEntry(script) {
		return true
	}
	resolved, err := resolve(script)
	return err == nil && isPiPackageEntry(resolved)
}

func isPiProcessArgsWithResolver(args []string, resolve func(string) (string, error)) bool {
	if len(args) == 0 {
		return false
	}
	executable, ok := executableArg(args)
	if !ok {
		return false
	}
	base := filepath.Base(args[executable])
	if base == "pi" {
		return true
	}
	if base != "node" {
		return false
	}
	for index := executable + 1; index < len(args); index++ {
		token := args[index]
		if token == "--" {
			index++
			if index >= len(args) {
				return false
			}
			return isPiScriptPath(args[index], resolve)
		}
		if !strings.HasPrefix(token, "-") {
			return isPiScriptPath(token, resolve)
		}
		name, _, hasInlineValue := strings.Cut(token, "=")
		if nodeNonScriptOptions[name] {
			return false
		}
		if hasInlineValue {
			continue
		}
		if nodeValueOptions[name] {
			index++
			if index >= len(args) {
				return false
			}
			continue
		}
		if !nodeFlagOptions[name] {
			return false
		}
	}
	return false
}

func executableArg(args []string) (int, bool) {
	executable := 0
	if filepath.Base(args[0]) != "env" {
		return executable, true
	}
	executable++
	for executable < len(args) && strings.Contains(args[executable], "=") {
		executable++
	}
	return executable, executable < len(args)
}

func isPiScriptPath(script string, resolve func(string) (string, error)) bool {
	if isPiPackageEntry(script) {
		return true
	}
	resolved, err := resolve(script)
	return err == nil && isPiPackageEntry(resolved)
}

// nodeScriptArgument recovers Node's entry-script argument from ps command
// text. ps flattens argv with spaces, so an unquoted npm prefix such as
// "/Users/me/My Projects" cannot be reconstructed with strings.Fields. The
// canonical package suffix gives us a bounded endpoint while the start stays
// anchored to Node's first non-option argument. An earlier JavaScript entry
// makes the reconstructed path require filesystem resolution so a generic
// Node worker cannot smuggle a later Pi path in its ordinary arguments.
func nodeScriptArgument(command string, fields []string, executable int) (script string, mustResolve, ok bool) {
	tail, ok := commandTailAfterField(command, fields, executable)
	if !ok {
		return "", false, false
	}
	tail, ok = trimNodeRuntimeOptions(tail)
	if !ok {
		return "", false, false
	}
	if script, quoted := quotedNodeScript(tail); quoted {
		return script, false, true
	}
	if strings.Contains(filepath.ToSlash(tail), piPackageEntrySuffix) {
		return unquotedPiPackageEntry(tail)
	}
	return strings.Trim(strings.Fields(tail)[0], "'\""), false, true
}

func commandTailAfterField(command string, fields []string, fieldIndex int) (string, bool) {
	offset := 0
	for i := 0; i <= fieldIndex; i++ {
		index := strings.Index(command[offset:], fields[i])
		if index < 0 {
			return "", false
		}
		offset += index + len(fields[i])
	}
	return strings.TrimSpace(command[offset:]), true
}

var nodeFlagOptions = map[string]bool{
	"--abort-on-uncaught-exception":           true,
	"--disallow-code-generation-from-strings": true,
	"--enable-source-maps":                    true,
	"--expose-gc":                             true,
	"--frozen-intrinsics":                     true,
	"--inspect":                               true,
	"--inspect-brk":                           true,
	"--inspect-wait":                          true,
	"--jitless":                               true,
	"--no-deprecation":                        true,
	"--no-warnings":                           true,
	"--openssl-legacy-provider":               true,
	"--openssl-shared-config":                 true,
	"--pending-deprecation":                   true,
	"--preserve-symlinks":                     true,
	"--preserve-symlinks-main":                true,
	"--throw-deprecation":                     true,
	"--trace-deprecation":                     true,
	"--trace-exit":                            true,
	"--trace-uncaught":                        true,
	"--trace-warnings":                        true,
	"--use-bundled-ca":                        true,
	"--use-openssl-ca":                        true,
	"--use-system-ca":                         true,
	"--zero-fill-buffers":                     true,
}

var nodeValueOptions = map[string]bool{
	"-C":                    true,
	"-r":                    true,
	"--conditions":          true,
	"--experimental-loader": true,
	"--import":              true,
	"--inspect-port":        true,
	"--debug-port":          true,
	"--loader":              true,
	"--require":             true,
}

var nodeNonScriptOptions = map[string]bool{
	"-":                 true,
	"-e":                true,
	"-i":                true,
	"-p":                true,
	"--check":           true,
	"--completion-bash": true,
	"--eval":            true,
	"--help":            true,
	"--interactive":     true,
	"--print":           true,
	"--prof-process":    true,
	"--run":             true,
	"--version":         true,
	"--v8-options":      true,
}

// trimNodeRuntimeOptions locates Node's script argument without allowing an
// option value to masquerade as that script. Known boolean runtime flags are
// skipped, known preload options consume their following value, evaluator and
// other non-script modes are rejected, and unknown options fail closed.
func trimNodeRuntimeOptions(tail string) (string, bool) {
	for {
		token, remainder, ok := commandToken(tail)
		if !ok {
			return "", false
		}
		if token == "--" {
			return strings.TrimSpace(remainder), strings.TrimSpace(remainder) != ""
		}
		if !strings.HasPrefix(token, "-") {
			return tail, true
		}
		name, _, hasInlineValue := strings.Cut(token, "=")
		if nodeNonScriptOptions[name] {
			return "", false
		}
		if hasInlineValue {
			// The value is bound to this option, so it cannot be mistaken for
			// the later script argument. This also safely supports V8 and
			// future Node options that follow the --name=value convention.
			tail = remainder
			continue
		}
		if nodeValueOptions[name] {
			remainder, ok = consumeNodeOptionValue(name, remainder)
			if !ok {
				return "", false
			}
			tail = remainder
			continue
		}
		if !nodeFlagOptions[name] {
			return "", false
		}
		tail = remainder
	}
}

var nodeModulePathOptions = map[string]bool{
	"-r":                    true,
	"--experimental-loader": true,
	"--import":              true,
	"--loader":              true,
	"--require":             true,
}

// consumeNodeOptionValue handles ps output that flattened a whitespace-bearing
// preload path. Module-valued options have a bounded file suffix, so consume
// through the last such suffix before the canonical Pi entrypoint; otherwise
// retain Node's ordinary one-token semantics.
func consumeNodeOptionValue(name, input string) (string, bool) {
	if !nodeModulePathOptions[name] || input == "" || input[0] == '\'' || input[0] == '"' {
		_, remainder, ok := commandToken(input)
		return remainder, ok
	}
	limit := strings.Index(filepath.ToSlash(input), piPackageEntrySuffix)
	if limit < 0 {
		_, remainder, ok := commandToken(input)
		return remainder, ok
	}
	valueEnd := -1
	for _, suffix := range []string{".cjs", ".mjs", ".js", ".node"} {
		search := input[:limit]
		for offset := 0; offset < len(search); {
			index := strings.Index(search[offset:], suffix)
			if index < 0 {
				break
			}
			end := offset + index + len(suffix)
			if end == len(search) || search[end] == ' ' || search[end] == '\t' {
				valueEnd = max(valueEnd, end)
			}
			offset = end
		}
	}
	if valueEnd >= 0 {
		return strings.TrimSpace(input[valueEnd:]), true
	}
	_, remainder, ok := commandToken(input)
	return remainder, ok
}

func commandToken(input string) (token, remainder string, ok bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", false
	}
	if input[0] == '\'' || input[0] == '"' {
		quote := input[0]
		if end := strings.IndexByte(input[1:], quote); end >= 0 {
			return input[1 : end+1], strings.TrimSpace(input[end+2:]), true
		}
		return "", "", false
	}
	if end := strings.IndexAny(input, " \t"); end >= 0 {
		return input[:end], strings.TrimSpace(input[end:]), true
	}
	return input, "", true
}

func quotedNodeScript(tail string) (string, bool) {
	if tail[0] == '\'' || tail[0] == '"' {
		quote := tail[0]
		if end := strings.IndexByte(tail[1:], quote); end >= 0 {
			return tail[1 : end+1], true
		}
	}
	return "", false
}

const piPackageEntrySuffix = "/@earendil-works/pi-coding-agent/dist/cli.js"

func unquotedPiPackageEntry(tail string) (script string, mustResolve, ok bool) {
	normalized := filepath.ToSlash(tail)
	index := strings.Index(normalized, piPackageEntrySuffix)
	end := index + len(piPackageEntrySuffix)
	if index < 0 || (end < len(normalized) && normalized[end] != ' ' && normalized[end] != '\t') {
		return "", false, false
	}
	candidate := strings.Trim(normalized[:end], "'\"")
	if strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "./") || strings.HasPrefix(candidate, "../") {
		return candidate, strings.ContainsAny(strings.TrimSpace(normalized[:index]), " \t"), true
	}
	return "", false, false
}

// IsPiProcessCommand is the pure process-identity contract used by BDD. A
// direct Pi executable is accepted; a generic Node process is accepted only
// when its entry script is the installed Pi package CLI.
func IsPiProcessCommand(command string) bool {
	return isPiProcessCommand(command)
}

func isPiPackageEntry(path string) bool {
	normalized := filepath.ToSlash(filepath.Clean(path))
	return strings.HasSuffix(normalized, piPackageEntrySuffix)
}

func piProcessInPaneTree(panePIDs []int, procs []procCommandEntry) bool {
	children := make(map[int][]int, len(procs))
	byPID := make(map[int]procCommandEntry, len(procs))
	for _, process := range procs {
		children[process.PPID] = append(children[process.PPID], process.PID)
		byPID[process.PID] = process
	}
	queue := append([]int(nil), panePIDs...)
	seen := make(map[int]bool, len(procs))
	for len(queue) > 0 {
		pid := queue[0]
		queue = queue[1:]
		if seen[pid] {
			continue
		}
		seen[pid] = true
		process, ok := byPID[pid]
		if !ok {
			continue
		}
		if process.ArgvInspected {
			if isPiProcessArgsWithResolver(process.Args, filepath.EvalSymlinks) {
				return true
			}
		} else if isPiProcessCommand(process.Command) {
			return true
		}
		queue = append(queue, children[pid]...)
	}
	return false
}

func readProcessTableWithArgs(ctx context.Context) ([]ProcEntry, error) {
	cmd := exec.CommandContext(ctx, "ps", "-axo", "pid=,ppid=,pgid=,tpgid=,stat=,comm=")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps foreground table: %w", err)
	}
	procs := ParsePSForegroundTable(string(out))
	cmd = exec.CommandContext(ctx, "ps", "-axo", "pid=,args=")
	out, err = cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ps args: %w", err)
	}
	argsByPID := ParsePSArgsTable(string(out))
	for i := range procs {
		procs[i].Args = argsByPID[procs[i].PID]
	}
	return procs, nil
}

// CheckPaneLiveness runs the real harness-liveness scan for sessionName on
// socketPath: pane PIDs via tmux, one ps snapshot, then the pure classifier
// with the standard harness set. A missing session returns
// SessionExists=false with a nil error — that IS the verdict.
func CheckPaneLiveness(sessionName, socketPath string) (PaneLiveness, error) {
	return CheckPaneLivenessContext(context.Background(), sessionName, socketPath)
}

// CheckPaneLivenessContext runs the liveness scan under the caller's lifetime
// while retaining the package timeout as an upper bound.
func CheckPaneLivenessContext(parent context.Context, sessionName, socketPath string) (PaneLiveness, error) {
	ctx, cancel := context.WithTimeout(parent, livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil {
		return PaneLiveness{}, err
	}
	if len(pids) == 0 {
		return PaneLiveness{SessionExists: false}, nil
	}
	procs, err := readProcessTable(ctx)
	if err != nil {
		return PaneLiveness{}, err
	}
	return ClassifyPaneLiveness(pids, procs, IsHarnessComm), nil
}

// CheckPaneLivenessBatch scans many sessions with a constant number of
// subprocesses: ONE `tmux list-panes -a` (all panes on the server, tagged
// with their session name) and ONE `ps` snapshot, then the pure classifier
// per requested session. The result map is keyed by the caller's original
// session names; a requested session with no panes on the server reports
// SessionExists=false. Use this instead of per-session CheckPaneLiveness in
// list paths, where N sessions would otherwise mean 3N subprocesses.
func CheckPaneLivenessBatch(sessionNames []string, socketPath string) (map[string]PaneLiveness, error) {
	ctx, cancel := context.WithTimeout(context.Background(), livenessScanTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-a", "-F", "#{session_name}\t#{pane_pid}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tmux list-panes -a timed out: %w", ctx.Err())
		}
		return nil, tmuxCommandError("list panes", "all sessions", out, err)
	}

	pidsBySession := make(map[string][]int)
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, "\t")
		if idx < 0 {
			continue
		}
		sessionName := line[:idx]
		pid, convErr := strconv.Atoi(strings.TrimSpace(line[idx+1:]))
		if convErr != nil {
			continue
		}
		pidsBySession[sessionName] = append(pidsBySession[sessionName], pid)
	}

	procs, err := readProcessTable(ctx)
	if err != nil {
		return nil, err
	}

	results := make(map[string]PaneLiveness, len(sessionNames))
	for _, name := range sessionNames {
		pids := pidsBySession[NormalizeTmuxSessionName(name)]
		if len(pids) == 0 {
			results[name] = PaneLiveness{SessionExists: false}
			continue
		}
		results[name] = ClassifyPaneLiveness(pids, procs, IsHarnessComm)
	}
	return results, nil
}

// CheckProcessInPaneTree reports whether a process named processName (exact
// comm or comm base-name match) is running anywhere in the descendant process
// tree of sessionName's panes. It preserves scan errors so lifecycle callers
// can fail safe instead of injecting a command when liveness is unknown.
func CheckProcessInPaneTree(sessionName, socketPath, processName string) (bool, error) {
	return IsProcessInPaneTreeContext(context.Background(), sessionName, socketPath, processName)
}

// IsPiProcessInPaneTree reports whether the pane tree contains Pi's native
// executable or its documented npm Node entrypoint.
func IsPiProcessInPaneTree(sessionName, socketPath string) (bool, error) {
	return IsPiProcessInPaneTreeContext(context.Background(), sessionName, socketPath)
}

// IsPiProcessInPaneTreeContext distinguishes Pi from other Node-hosted
// harnesses by inspecting live descendant argv values under the caller's
// lifetime. Tmux pane command metadata is deliberately not identity evidence:
// remain-on-exit preserves it after the process has died. A generic node comm
// is never sufficient proof either.
func IsPiProcessInPaneTreeContext(parent context.Context, sessionName, socketPath string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil || len(pids) == 0 {
		return false, err
	}
	procs, err := readProcessCommandTable(ctx)
	if err != nil {
		return false, err
	}
	return piProcessInPaneTree(pids, procs), nil
}

// IsProcessInPaneTree reports whether a process named processName (exact comm
// or comm base-name match) is running anywhere in the descendant process tree
// of sessionName's panes. Any failure reports false for compatibility with
// existing best-effort liveness callers.
func IsProcessInPaneTree(sessionName, socketPath, processName string) bool {
	running, err := IsProcessInPaneTreeContext(context.Background(), sessionName, socketPath, processName)
	if err != nil {
		return false
	}
	return running
}

// IsProcessInPaneTreeContext is the cancellation-aware process-tree scan used
// by command transactions that must not outlive their caller.
func IsProcessInPaneTreeContext(parent context.Context, sessionName, socketPath, processName string) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, livenessScanTimeout)
	defer cancel()

	pids, err := listPanePIDs(ctx, sessionName, socketPath)
	if err != nil || len(pids) == 0 {
		return false, err
	}
	// A matching pane foreground command is already exact positive evidence and
	// avoids requiring an OS-wide process-table scan for the common case. This
	// also keeps delivery checks usable in nested sandboxes where tmux is visible
	// but `ps -axo` is intentionally unavailable. A miss still falls through to
	// the full descendant-tree scan so wrapper shells and crash-resume trees work.
	command := exec.CommandContext(ctx, "tmux", "-S", socketPath, "list-panes", "-s", "-t", FormatSessionTarget(NormalizeTmuxSessionName(sessionName)), "-F", "#{pane_current_command}")
	if output, commandErr := command.Output(); commandErr == nil && paneCommandMatchesProcess(string(output), processName) {
		return true, nil
	}
	procs, err := readProcessTable(ctx)
	if err != nil {
		return false, err
	}
	verdict := ClassifyPaneLiveness(pids, procs, func(comm string) bool {
		return comm == processName || filepath.Base(comm) == processName
	})
	return verdict.HarnessAlive, nil
}

func paneCommandMatchesProcess(output, processName string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		command := strings.TrimSpace(line)
		if command == processName || filepath.Base(command) == processName {
			return true
		}
	}
	return false
}
