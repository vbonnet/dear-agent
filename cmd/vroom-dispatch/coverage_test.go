package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestCoveragePureHelpers pins duration parsing, minimum selection, API-key scrubbing, and retry classification.
func TestParseDurationCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  time.Duration
	}{{"empty", "", 0}, {"dash", "-", 0}, {"minutes", "12m", 12 * time.Minute}, {"compound", "2h30m", 150 * time.Minute}, {"days", "3d", 72 * time.Hour}, {"invalid", "bad", 0}}
	for _, tc := range cases {
		input, want := tc.input, tc.want
		t.Run(tc.name, func(t *testing.T) {
			if got := parseDuration(input); got != want {
				t.Errorf("parseDuration(%q) = %s, want %s", input, got, want)
			}
		})
	}
}

func TestMinDurationChoosesSmaller(t *testing.T) {
	if minDuration(time.Second, 2*time.Second) != time.Second || minDuration(2*time.Second, time.Second) != time.Second {
		t.Fatal("minDuration did not choose the smaller duration")
	}
}

func TestScrubAPIKeyRemovesSecret(t *testing.T) {
	got := scrubAPIKey([]string{"A=1", "ANTHROPIC_API_KEY=secret", "B=2"})
	if strings.Join(got, "|") != "A=1|B=2" {
		t.Fatalf("scrubAPIKey = %v", got)
	}
}

func TestRetryableSpawnRefusalClassification(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want bool
	}{{"ordinary", "ordinary failure", false}, {"stagger", "spawn too soon", true}, {"tagged stagger", "• [spawn_stagger] spawn too soon", true}, {"other tag", "• [disk] spawn too soon", false}}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryableSpawnRefusal(tc.out); got != tc.want {
				t.Errorf("isRetryableSpawnRefusal(%q) = %v, want %v", tc.out, got, tc.want)
			}
		})
	}
}

func TestSpawnRetryDelayFallback(t *testing.T) {
	if got := spawnRetryDelay("governor pause without a boundary"); got != minSpawnInterval {
		t.Errorf("spawnRetryDelay fallback = %s", got)
	}
}

// TestCoverageStateHelpers pins tolerant state loading, atomic state saving, archive arguments, and trail tail output.
func TestStateSaveLoadAndArchiveArgs(t *testing.T) {
	home := t.TempDir()
	if got := loadState(home); got.Sessions == nil {
		t.Fatal("loadState returned nil sessions")
	}
	state := &sessionState{Sessions: map[string]sessionInfo{"worker": {Name: "worker", LoopSent: true}}}
	saveState(home, state)
	loaded := loadState(home)
	if !loaded.Sessions["worker"].LoopSent || loaded.UpdatedAt == "" {
		t.Fatalf("saved state not restored: %+v", loaded)
	}
	if got := sessionArchiveArgs(supervisor{Name: "sup"}); strings.Join(got, " ") != "session archive --async --workspace=oss --outcome crashed sup" {
		t.Errorf("sessionArchiveArgs = %v", got)
	}
	if got := loadState(filepath.Join(home, "invalid")); got.Sessions == nil {
		t.Fatal("invalid state did not get an empty session map")
	}
}

func TestPrintTrailTailRendersLastLinesAndHandlesMissing(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, "trail.jsonl")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := captureStdout(func() { printTrailTail(path, 2) }); got != "two\nthree\n" {
		t.Fatalf("tail output = %q", got)
	}
	if got := captureStdout(func() { printTrailTail(filepath.Join(home, "empty"), 2) }); got != "" {
		t.Fatalf("missing tail output = %q", got)
	}
	if err := os.WriteFile(filepath.Join(home, "empty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := captureStdout(func() { printTrailTail(filepath.Join(home, "empty"), 2) }); got != "" {
		t.Fatalf("empty tail output = %q", got)
	}
}

// TestCoverageFakeAGM pins exact session matching and the status fallback without invoking AGM.
func TestSessionMatchingIsExact(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) printf '%s\n' 'vroom-orchestrator vroom-orchestrator-worker-1';; supervisor) exit 1;; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if !isSessionAlive("vroom-orchestrator") || isSessionAlive("orchestrator") || isSessionAlive("worker") {
		t.Fatal("session matching did not enforce exact names")
	}
}

func TestPrintStatusNamesConfiguredSupervisors(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) printf '%s\n' 'vroom-orchestrator';; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got := captureStdout(func() {
		printStatus(&sessionState{Sessions: map[string]sessionInfo{"vroom-orchestrator": {LoopSent: true}}})
	})
	if !strings.Contains(got, "vroom-orchestrator") {
		t.Fatalf("status omitted session: %q", got)
	}
}

func TestShowStatusRendersFallback(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$1" in supervisor) exit 1;; session) printf '%s\n' 'vroom-orchestrator';; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := captureStdout(showStatus); !strings.Contains(got, "Session status:") {
		t.Fatalf("status did not render: %q", got)
	}
}

// TestCoverageSupervisorClassification pins dead, stale, auth-failed, and alive health outcomes.
func TestCoverageSupervisorClassification(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) printf '%s\n' 'sup';; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	orig := captureSupervisorPane
	t.Cleanup(func() { captureSupervisorPane = orig })
	sup := supervisor{Name: "sup", Harness: "codex-cli", TickInterval: time.Hour}
	captureSupervisorPane = func(string) (string, error) { return "codex login required", nil }
	if got := classifySupervisor(t.TempDir(), sup); got != healthAuthFailed {
		t.Errorf("auth classification = %v", got)
	}
	captureSupervisorPane = func(string) (string, error) { return "gpt-5 · /tmp\n›", nil }
	if got := classifySupervisor(t.TempDir(), sup); got != healthStale {
		t.Errorf("missing heartbeat classification = %v", got)
	}
	home := t.TempDir()
	path := filepath.Join(home, ".agm", "vroom", "heartbeat")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "sup.json"), []byte(time.Now().UTC().Format(time.RFC3339)), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := classifySupervisor(home, sup); got != healthAlive {
		t.Errorf("alive classification = %v", got)
	}
	if got := classifySupervisor(home, supervisor{Name: "missing", TickInterval: time.Hour}); got != healthDead {
		t.Errorf("dead classification = %v", got)
	}
}

// TestCoverageWorkerEscalation pins reset, nudge, diagnose, kill, and kill-failure paths for stuck workers.
func TestEscalateIfStuckPinsLadderAndResets(t *testing.T) {
	bin := t.TempDir()
	argv := filepath.Join(t.TempDir(), "argv")
	t.Setenv("AGM_ARGV_LOG", argv)
	writeFakeAGM(t, bin, `printf '%s\n' "$*" >> "$AGM_ARGV_LOG"; case "$2" in send|session) exit 0;; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agm", "vroom"), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := healthEntry{Name: "worker-1", State: "PERMISSION_PROMPT", LastUpdateAt: "x", TimeSinceLastUpdate: "20m"}
	ws := &workerState{}
	escalateIfStuck(home, entry, ws, true)
	if ws.escalationLevel != 1 || ws.staleFor != 1 {
		t.Fatalf("nudge state = %+v", *ws)
	}
	entry.TimeSinceLastUpdate = "35m"
	escalateIfStuck(home, entry, ws, true)
	if ws.escalationLevel != 2 {
		t.Fatalf("diagnose level = %d", ws.escalationLevel)
	}
	entry.TimeSinceLastUpdate = "50m"
	escalateIfStuck(home, entry, ws, true)
	if ws.escalationLevel != 3 || ws.staleFor != 3 {
		t.Fatalf("kill state = %+v", *ws)
	}
	recs := readTrailRecords(t, home)
	assertKinds(t, recs, []string{"dispatch.worker_nudged", "dispatch.worker_diagnosed", "dispatch.worker_killed_stuck"})
	if recs[2].Payload["stale_ticks"] != float64(3) {
		t.Fatalf("kill trail = %+v", recs[2].Payload)
	}
	entry.LastUpdateAt = "y"
	escalateIfStuck(home, entry, ws, true)
	if ws.escalationLevel != 0 || ws.staleFor != 0 {
		t.Fatalf("progress did not reset: %+v", *ws)
	}
	ws.escalationLevel, ws.staleFor = 2, 9
	escalateIfStuck(home, entry, ws, false)
	if ws.escalationLevel != 0 || ws.staleFor != 0 {
		t.Fatalf("missing worker did not reset: %+v", *ws)
	}
	ws.escalationLevel, ws.staleFor = 2, 2
	ws.lastSeenUpdateAt = "a"
	escalateIfStuck(home, healthEntry{Name: "healthy", State: "RUNNING", LastUpdateAt: "a", TimeSinceLastUpdate: "100h"}, ws, true)
	if ws.escalationLevel != 2 {
		t.Fatalf("healthy worker reached kill level: %+v", *ws)
	}
}

func TestDiagnoseEscalationActionsAndMessages(t *testing.T) {
	bin, log := loggingAGM(t, false)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agm", "vroom"), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := healthEntry{Name: "w", State: "PERMISSION_PROMPT", TimeSinceLastUpdate: "35m"}
	applyDiagnoseEscalation(home, entry, true)
	applyDiagnoseEscalation(home, entry, false)
	recs := readTrailRecords(t, home)
	if recs[0].Payload["action"] != "defer_nudge" || recs[1].Payload["action"] != "wrap_up" {
		t.Fatalf("actions = %+v", recs)
	}
	lines := readLines(t, log)
	if len(lines) != 2 || lines[0] == lines[1] {
		t.Fatalf("messages did not differ: %v", lines)
	}
}

func TestKillEscalationSuccessAndFailure(t *testing.T) {
	bin, log := loggingAGM(t, false)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agm", "vroom"), 0o700); err != nil {
		t.Fatal(err)
	}
	entry := healthEntry{Name: "w", State: "OFFLINE", TimeSinceLastUpdate: "50m"}
	applyKillEscalation(home, entry, &workerState{staleFor: 4})
	lines := readLines(t, log)
	if len(lines) != 1 || !strings.Contains(lines[0], "--confirmed-stuck") {
		t.Fatalf("kill argv = %v", lines)
	}
	writeFakeAGM(t, bin, `printf '%s\n' "$*" >> "$AGM_ARGV_LOG"; exit 1`)
	applyKillEscalation(home, entry, &workerState{staleFor: 5})
	recs := readTrailRecords(t, home)
	if recs[0].Kind != "dispatch.worker_killed_stuck" || recs[1].Kind != "dispatch.worker_kill_failed" || recs[1].Payload["error"] == "" {
		t.Fatalf("kill records = %+v", recs)
	}
}

// TestCoverageMonitorAndEscalation pins monitor startup and cancellation plus structured human escalation outcomes.
func TestRunHealthMonitorReturnsWhenCancelled(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) exit 1;; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".agm", "vroom"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := &sessionState{Sessions: make(map[string]sessionInfo)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { runHealthMonitor(ctx, home, state, defaultSupervisorModel); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled monitor blocked")
	}
	if recs := readTrailRecords(t, home); len(recs) != 2 || recs[0].Kind != "dispatch.started" || recs[1].Kind != "dispatch.shutdown" {
		t.Fatalf("monitor trail = %+v", recs)
	}
	origDesktop, origPush := desktopNotify, mcpPush
	t.Cleanup(func() { desktopNotify, mcpPush = origDesktop, origPush })
	desktopNotify = func(string) error { return errors.New("desktop unavailable") }
	mcpPush = func(string, string) (bool, error) { return false, errors.New("push unavailable") }
	escalateToHuman(home, "test", "message", map[string]any{"worker": "w"})
	data, err := os.ReadFile(filepath.Join(home, ".agm", "vroom", "dispatch-trail.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "dispatch.escalation.mcp_failed") {
		t.Fatalf("missing escalation trail: %s", data)
	}
}

// TestCoverageEscalationHelpers pins AppleScript escaping and no-session push fallback.
func TestCoverageEscalationHelpers(t *testing.T) {
	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) exit 1;; esac`)
	if err := os.WriteFile(filepath.Join(bin, "osascript"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if got := appleScriptString("a\n\\\""); got != `"a \\\""` {
		t.Errorf("appleScriptString = %q", got)
	}
	if got := firstActiveSupervisor(); got != "" {
		t.Errorf("firstActiveSupervisor with no live session = %q, want empty", got)
	}
	if sent, err := pushViaActiveSession(t.TempDir(), "hello"); sent || err != nil {
		t.Errorf("push fallback = %v, %v", sent, err)
	}
	argsLog := filepath.Join(bin, "osascript-args")
	if err := os.WriteFile(filepath.Join(bin, "osascript"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \""+argsLog+"\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := osascriptNotify("test"); err != nil {
		t.Fatal(err)
	}
	var got string
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(argsLog); err == nil {
			got = strings.TrimSpace(string(data))
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(got, "display notification") || !strings.Contains(got, "test") {
		t.Fatalf("osascript args = %q", got)
	}
	if osascriptArgs("hello")[0] != "-e" {
		t.Fatal("osascriptArgs missing -e")
	}
}

// TestFirstActiveSupervisorFindsALiveOne is the positive half of the pair
// above. Asserting only the empty result would be satisfied by a function that
// always returned empty, so this pins that a live session is actually found and
// that the first supervisor in declaration order wins.
func TestFirstActiveSupervisorFindsALiveOne(t *testing.T) {
	if len(supervisors) == 0 {
		t.Skip("no supervisors configured")
	}
	want := supervisors[0].Name

	bin := t.TempDir()
	writeFakeAGM(t, bin, `case "$2" in list) printf '%s\n' '`+want+`';; esac`)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if got := firstActiveSupervisor(); got != want {
		t.Errorf("firstActiveSupervisor = %q, want %q", got, want)
	}
}

func writeFakeAGM(t *testing.T, dir, body string) {
	t.Helper()
	path := filepath.Join(dir, "agm")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func loggingAGM(t *testing.T, fail bool) (string, string) {
	bin := t.TempDir()
	log := filepath.Join(bin, "argv")
	t.Setenv("AGM_ARGV_LOG", log)
	body := `printf '%s\n' "$*" >> "$AGM_ARGV_LOG"; case "$2" in send) exit 0;; session) exit 0;; esac`
	if fail {
		body = `printf '%s\n' "$*" >> "$AGM_ARGV_LOG"; exit 1`
	}
	writeFakeAGM(t, bin, body)
	return bin, log
}
func readTrailRecords(t *testing.T, home string) []trailRecord {
	t.Helper()
	f, err := os.Open(filepath.Join(home, ".agm", "vroom", "dispatch-trail.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []trailRecord
	s := bufio.NewScanner(f)
	for s.Scan() {
		var r trailRecord
		if err := json.Unmarshal(s.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	return out
}
func assertKinds(t *testing.T, got []trailRecord, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("trail length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Kind != want[i] {
			t.Fatalf("trail[%d] = %q, want %q", i, got[i].Kind, want[i])
		}
	}
}
func readLines(t *testing.T, path string) []string {
	t.Helper()
	return strings.Split(strings.TrimSpace(string(mustRead(t, path))), "\n")
}
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func captureStdout(fn func()) string {
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = old
	var b bytes.Buffer
	io.Copy(&b, r)
	r.Close()
	return b.String()
}
