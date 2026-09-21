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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/internal/supervisorheartbeat"
	vroomsupervisor "github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
	"golang.org/x/sys/unix"
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
	if got := captureStdout(t, func() { printTrailTail(path, 2) }); got != "two\nthree\n" {
		t.Fatalf("tail output = %q", got)
	}
	if got := captureStdout(t, func() { printTrailTail(filepath.Join(home, "empty"), 2) }); got != "" {
		t.Fatalf("missing tail output = %q", got)
	}
	if err := os.WriteFile(filepath.Join(home, "empty"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := captureStdout(t, func() { printTrailTail(filepath.Join(home, "empty"), 2) }); got != "" {
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
	got := captureStdout(t, func() {
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
	// showStatus reads state and the dispatch trail from the home directory.
	// Without this it would read a developer's real ~/.agm/vroom files, which
	// makes the test non-hermetic and would put live session and trail
	// contents into the failure output.
	t.Setenv("HOME", t.TempDir())

	if got := captureStdout(t, showStatus); !strings.Contains(got, "Session status:") {
		t.Fatalf("status did not render: %q", got)
	}
}

// TestCoverageSupervisorClassification pins dead, stale, auth-failed, and alive health outcomes.
func TestCoverageSupervisorClassification(t *testing.T) {
	sup := supervisor{Name: "vroom-orchestrator", Harness: "codex-cli", TickInterval: time.Hour}
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	checkerForPane := func(pane string) supervisorHealthChecker {
		return supervisorHealthChecker{
			sessionAlive:  func(name string) bool { return name == sup.Name },
			capturePane:   func(string) (string, error) { return pane, nil },
			readHeartbeat: readSupervisorHeartbeat,
			now:           func() time.Time { return now },
		}
	}
	writeRecord := func(home, id string, beat time.Time) {
		t.Helper()
		store := supervisorheartbeat.New(filepath.Join(home, ".agm", "supervisors"))
		if err := store.Write(supervisorheartbeat.Record{
			ID:          id,
			PrimaryFor:  "vroom-overseer",
			TertiaryFor: "vroom-meta-orchestrator",
			LastBeatUTC: beat,
			PID:         os.Getpid(),
			TmuxSession: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	writeMirror := func(home string, beat time.Time) {
		t.Helper()
		dir := filepath.Join(home, ".agm", "vroom", "heartbeat")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]any{
			"ts":   float64(beat.UnixMilli()) / 1e3,
			"iso":  beat.UTC().Format(time.RFC3339),
			"role": "orchestrator",
		})
		if err != nil {
			t.Fatal(err)
		}
		member, ok := vroomsupervisor.Lookup(sup.Name)
		if !ok {
			t.Fatalf("legacy mirror fixture has no topology member for %q", sup.Name)
		}
		if err := os.WriteFile(filepath.Join(dir, member.Alias+".json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	freshHome := t.TempDir()
	writeRecord(freshHome, sup.Name, now)
	classify := func(checker supervisorHealthChecker, home string, candidate supervisor) supervisorHealth {
		t.Helper()
		health, err := checker.classify(home, candidate)
		if err != nil {
			t.Fatalf("classify(%q): %v", candidate.Name, err)
		}
		return health
	}
	if got := classify(checkerForPane("codex login required"), freshHome, sup); got != healthAuthFailed {
		t.Errorf("fresh heartbeat with auth failure classification = %v", got)
	}
	readyChecker := checkerForPane("gpt-5 · /tmp\n›")

	missingHome := t.TempDir()
	writeMirror(missingHome, now)
	if got := classify(readyChecker, missingHome, sup); got != healthStale {
		t.Errorf("fresh mirror with missing authoritative heartbeat classification = %v", got)
	}
	zeroHome := t.TempDir()
	zeroStore := supervisorheartbeat.New(filepath.Join(zeroHome, ".agm", "supervisors"))
	if err := zeroStore.Write(supervisorheartbeat.Record{ID: sup.Name}); err != nil {
		t.Fatal(err)
	}
	writeMirror(zeroHome, now)
	if got := classify(readyChecker, zeroHome, sup); got != healthStale {
		t.Errorf("fresh mirror with zero authoritative heartbeat classification = %v", got)
	}
	malformedHome := t.TempDir()
	malformedDir := filepath.Join(malformedHome, ".agm", "supervisors", sup.Name)
	if err := os.MkdirAll(malformedDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(malformedDir, "heartbeat.json"), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeMirror(malformedHome, now)
	malformedHealth, malformedErr := readyChecker.classify(malformedHome, sup)
	if malformedHealth != healthStale {
		t.Errorf("fresh mirror with malformed authoritative heartbeat classification = %v", malformedHealth)
	}
	if malformedErr == nil ||
		!strings.Contains(malformedErr.Error(), `read authoritative heartbeat "vroom-orchestrator"`) ||
		!strings.Contains(malformedErr.Error(), "unmarshal") {
		t.Errorf("malformed authoritative heartbeat diagnostic = %v", malformedErr)
	}
	agedHome := t.TempDir()
	writeRecord(agedHome, sup.Name, now.Add(-3*time.Hour))
	writeMirror(agedHome, now)
	if got := classify(readyChecker, agedHome, sup); got != healthStale {
		t.Errorf("fresh mirror with aged authoritative heartbeat classification = %v", got)
	}

	home := t.TempDir()
	writeRecord(home, sup.Name, now)
	writeMirror(home, now.Add(-24*time.Hour))
	if got := classify(readyChecker, home, sup); got != healthAlive {
		t.Errorf("fresh authoritative heartbeat with stale mirror classification = %v", got)
	}
	writeRecord(home, "missing", now)
	if got := classify(readyChecker, home, supervisor{Name: "missing", TickInterval: time.Hour}); got != healthDead {
		t.Errorf("fresh heartbeat with absent session classification = %v", got)
	}
	thresholdHome := t.TempDir()
	writeRecord(thresholdHome, sup.Name, now.Add(-2*sup.TickInterval))
	if got := classify(readyChecker, thresholdHome, sup); got != healthAlive {
		t.Errorf("heartbeat at exact threshold classification = %v", got)
	}
	writeRecord(thresholdHome, sup.Name, now.Add(-2*sup.TickInterval-time.Nanosecond))
	if got := classify(readyChecker, thresholdHome, sup); got != healthStale {
		t.Errorf("heartbeat beyond threshold classification = %v", got)
	}
}

func TestSupervisorHealthCheckerHarnessAuth(t *testing.T) {
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name, harness, pane string
		want                supervisorHealth
	}{
		{"Claude auth", "claude-code", "Error: 401 Unauthorized\nPlease run /login", healthAuthFailed},
		{"Codex auth", "codex-cli", "No OpenAI credentials found. Run `codex login` to continue.", healthAuthFailed},
		{"AGY auth", "agy", "Application Default Credentials unavailable; run gcloud auth application-default login", healthAuthFailed},
		{"other harness", "other", "codex login required", healthAlive},
		{"Codex ready", "codex-cli", "codex login required\n›\n\ngpt-5.6 xhigh · ~/src/project", healthAlive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := supervisorHealthChecker{
				sessionAlive: func(string) bool { return true },
				capturePane:  func(string) (string, error) { return tt.pane, nil },
				readHeartbeat: func(string, string) (*supervisorheartbeat.Record, error) {
					return &supervisorheartbeat.Record{LastBeatUTC: now}, nil
				},
				now: func() time.Time { return now },
			}
			got, err := checker.classify(t.TempDir(), supervisor{
				Name: "vroom-orchestrator", Harness: tt.harness, TickInterval: time.Hour,
			})
			if err != nil || got != tt.want {
				t.Fatalf("classify() = (%v, %v), want (%v, nil)", got, err, tt.want)
			}
		})
	}
}

func TestSupervisorHealthCheckerProbeOrder(t *testing.T) {
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		sessionUp  bool
		pane       string
		paneErr    error
		record     *supervisorheartbeat.Record
		wantHealth supervisorHealth
		wantCalls  string
	}{
		{"dead skips later probes", false, "", nil, nil, healthDead, "session"},
		{"auth skips heartbeat and clock", true, "codex login required", nil, nil, healthAuthFailed, "session,pane"},
		{"missing heartbeat skips clock", true, "", nil, nil, healthStale, "session,pane,heartbeat"},
		{"pane error does not hide heartbeat", true, "", errors.New("tmux unavailable"), nil, healthStale, "session,pane,heartbeat"},
		{"fresh heartbeat reads clock", true, "", nil, &supervisorheartbeat.Record{LastBeatUTC: now}, healthAlive, "session,pane,heartbeat,clock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			checker := supervisorHealthChecker{
				sessionAlive: func(name string) bool {
					calls = append(calls, "session")
					if name != "vroom-orchestrator" {
						t.Fatalf("session probe name = %q", name)
					}
					return tt.sessionUp
				},
				capturePane: func(name string) (string, error) {
					calls = append(calls, "pane")
					if name != "vroom-orchestrator" {
						t.Fatalf("pane probe name = %q", name)
					}
					return tt.pane, tt.paneErr
				},
				readHeartbeat: func(home, name string) (*supervisorheartbeat.Record, error) {
					calls = append(calls, "heartbeat")
					if home != "/test-home" || name != "vroom-orchestrator" {
						t.Fatalf("heartbeat probe args = (%q, %q)", home, name)
					}
					return tt.record, nil
				},
				now: func() time.Time {
					calls = append(calls, "clock")
					return now
				},
			}
			got, err := checker.classify("/test-home", supervisor{
				Name: "vroom-orchestrator", Harness: "codex-cli", TickInterval: time.Hour,
			})
			if err != nil || got != tt.wantHealth {
				t.Fatalf("classify() = (%v, %v), want (%v, nil)", got, err, tt.wantHealth)
			}
			if gotCalls := strings.Join(calls, ","); gotCalls != tt.wantCalls {
				t.Fatalf("probe calls = %q, want %q", gotCalls, tt.wantCalls)
			}
		})
	}
}

func TestSupervisorDiagnosticTrackerDebouncesUntilRecovery(t *testing.T) {
	tracker := newSupervisorDiagnosticTracker()
	readErr := errors.New("malformed heartbeat")
	if !tracker.shouldReportHeartbeatReadError("orchestrator", readErr) {
		t.Fatal("first heartbeat read error was suppressed")
	}
	if tracker.shouldReportHeartbeatReadError("orchestrator", readErr) {
		t.Fatal("identical heartbeat read error was reported twice")
	}
	if !tracker.shouldReportHeartbeatReadError("orchestrator", errors.New("permission denied")) {
		t.Fatal("changed heartbeat read error was suppressed")
	}
	if tracker.shouldReportHeartbeatReadError("orchestrator", nil) {
		t.Fatal("recovery should clear state without reporting an error")
	}
	if !tracker.shouldReportHeartbeatReadError("orchestrator", readErr) {
		t.Fatal("heartbeat read error after recovery was suppressed")
	}
}

func TestStaleSupervisorTrailDetailsIdentifyAuthoritativeRecord(t *testing.T) {
	details := staleSupervisorTrailDetails("vroom-orchestrator")
	want := map[string]any{
		"supervisor":       "vroom-orchestrator",
		"heartbeat_id":     "vroom-orchestrator",
		"heartbeat_source": "authoritative_agm_supervisor_record",
	}
	if !reflect.DeepEqual(details, want) {
		t.Fatalf("staleSupervisorTrailDetails() = %#v, want %#v", details, want)
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
	case <-time.After(30 * time.Second):
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
	argsPipe := filepath.Join(bin, "osascript-args")
	if err := unix.Mkfifo(argsPipe, 0o600); err != nil {
		t.Fatal(err)
	}
	pipeReader, err := os.OpenFile(argsPipe, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pipeReader.Close() })
	t.Setenv("OSASCRIPT_ARGS_PIPE", argsPipe)
	if err := os.WriteFile(filepath.Join(bin, "osascript"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$OSASCRIPT_ARGS_PIPE\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	argsRead := make(chan string, 1)
	readErr := make(chan error, 1)
	go func() {
		got, err := bufio.NewReader(pipeReader).ReadString('\n')
		if err != nil {
			readErr <- err
			return
		}
		argsRead <- strings.TrimSpace(got)
	}()
	if err := osascriptNotify("test"); err != nil {
		t.Fatal(err)
	}
	var got string
	select {
	case got = <-argsRead:
	case err := <-readErr:
		t.Fatalf("read osascript args: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("timed out waiting for asynchronous osascript invocation")
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
	// A scanner stops silently on a read error or an over-long line. Without
	// this check a truncated trail would read as a shorter-but-valid one and
	// the assertions would pass on incomplete evidence.
	if err := s.Err(); err != nil {
		t.Fatalf("reading trail: %v", err)
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

// captureStdout runs fn with os.Stdout redirected and returns what it printed.
//
// The read happens on its own goroutine: a synchronous read deadlocks as soon
// as fn writes more than the pipe buffer holds. os.Stdout is restored through
// a defer so a panicking fn cannot leave every later test writing into a dead
// pipe.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = old
		_ = r.Close()
	}()

	out := make(chan string, 1)
	go func() {
		var b bytes.Buffer
		_, _ = io.Copy(&b, r)
		out <- b.String()
	}()

	fn()
	_ = w.Close()
	return <-out
}
