package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/vbonnet/dear-agent/pkg/absencealarm"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type runEnv struct {
	dir       string
	config    string
	state     string
	snooze    string
	journal   string
	heartbeat string
	notices   []string
	notifyErr error
}

func newRunEnv(t *testing.T, configDoc string) *runEnv {
	t.Helper()
	dir := t.TempDir()
	e := &runEnv{
		dir:       dir,
		config:    filepath.Join(dir, "pulses.json"),
		state:     filepath.Join(dir, "state.json"),
		snooze:    filepath.Join(dir, "snooze.json"),
		journal:   filepath.Join(dir, "journal.jsonl"),
		heartbeat: filepath.Join(dir, "heartbeat.json"),
	}
	if err := os.WriteFile(e.config, []byte(configDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	return e
}

func (e *runEnv) args(extra ...string) []string {
	base := []string{
		"--config", e.config, "--state", e.state, "--snooze", e.snooze,
		"--journal", e.journal, "--heartbeat", e.heartbeat,
	}
	return append(base, extra...)
}

func (e *runEnv) notifier() notifier {
	return func(_ context.Context, title, body string) error {
		e.notices = append(e.notices, title+" | "+body)
		return e.notifyErr
	}
}

const oneStalePulse = `{"pulses":[{"name":"spans","type":"file_mtime","path":"%s","window":"1h","expect":"a fresh OTel span file"}]}`

func stalePulseEnv(t *testing.T) *runEnv {
	t.Helper()
	dir := t.TempDir()
	target := filepath.Join(dir, "spans.jsonl")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-3 * time.Hour)
	if err := os.Chtimes(target, old, old); err != nil {
		t.Fatal(err)
	}
	doc := strings.ReplaceAll(oneStalePulse, "%s", target)
	return newRunEnv(t, doc)
}

// AA-07 + AA-09 + AA-10 + AA-16: an absent pulse exits 1, journals, notifies
// on transition, and the tick writes a heartbeat.
func TestRun_AbsentPulseAlarms(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errb.String())
	}
	if len(e.notices) != 1 || !strings.Contains(e.notices[0], "ABSENT: spans") {
		t.Fatalf("notices = %v, want one ABSENT notice", e.notices)
	}
	raw, err := os.ReadFile(e.journal)
	if err != nil {
		t.Fatalf("journal not written: %v", err)
	}
	var rec absencealarm.JournalRecord
	if err := json.Unmarshal(bytes.TrimSpace(raw), &rec); err != nil {
		t.Fatalf("journal not JSONL: %v", err)
	}
	if rec.Pulse != "spans" || rec.Status != absencealarm.StatusAbsent || rec.Misses != 1 {
		t.Errorf("journal record = %+v", rec)
	}
	var hb absencealarm.Heartbeat
	hraw, err := os.ReadFile(e.heartbeat)
	if err != nil {
		t.Fatalf("heartbeat not written: %v", err)
	}
	if err := json.Unmarshal(hraw, &hb); err != nil || len(hb.Results) != 1 {
		t.Fatalf("heartbeat = %s err=%v", hraw, err)
	}
}

// AA-10 + AA-11: the second tick within the backoff window does not
// re-notify but still journals and exits 1.
func TestRun_SecondTickDedupsNotification(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if len(e.notices) != 1 {
		t.Fatalf("notices = %v, want exactly one (deduped)", e.notices)
	}
	raw, _ := os.ReadFile(e.journal)
	if lines := strings.Count(strings.TrimSpace(string(raw)), "\n") + 1; lines != 2 {
		t.Errorf("journal lines = %d, want 2 (every alarming tick journals)", lines)
	}
}

// AA-12: recovery notifies once.
func TestRun_RecoveryNotifies(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())

	// Freshen the watched file: the pulse returns.
	var cfg absencealarm.PulseConfig
	raw, _ := os.ReadFile(e.config)
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(cfg.Pulses[0].Path, now, now); err != nil {
		t.Fatal(err)
	}
	code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 after recovery", code)
	}
	if len(e.notices) != 2 || !strings.Contains(e.notices[1], "RECOVERED: spans") {
		t.Fatalf("notices = %v, want ABSENT then RECOVERED", e.notices)
	}
}

// AA-08 + AA-13: a validly snoozed absent pulse exits 0 and stays quiet.
func TestRun_SnoozedPulseIsQuiet(t *testing.T) {
	e := stalePulseEnv(t)
	until := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	doc := `[{"pulse":"spans","until":"` + until + `","reason":"migration"}]`
	if err := os.WriteFile(e.snooze, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	if code != 0 {
		t.Fatalf("exit = %d, want 0 for a snoozed pulse", code)
	}
	if len(e.notices) != 0 {
		t.Fatalf("notices = %v, want none", e.notices)
	}
	if !strings.Contains(out.String(), "snoozed") {
		t.Errorf("report does not show the snooze:\n%s", out.String())
	}
}

// AA-14: an expiry-less snooze is a usage error.
func TestRun_SnoozeWithoutExpiryExits2(t *testing.T) {
	e := stalePulseEnv(t)
	if err := os.WriteFile(e.snooze, []byte(`[{"pulse":"spans"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// AA-15: an expired snooze does not silence the alarm and is named in it.
func TestRun_ExpiredSnoozeAlarms(t *testing.T) {
	e := stalePulseEnv(t)
	until := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	doc := `[{"pulse":"spans","until":"` + until + `","reason":"was migrating"}]`
	if err := os.WriteFile(e.snooze, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if len(e.notices) != 1 || !strings.Contains(e.notices[0], "snooze expired") {
		t.Fatalf("notices = %v, want alarm naming the expired snooze", e.notices)
	}
}

// AA-17: a notification failure is reported and does not change the exit code.
func TestRun_NotifyFailureKeepsExitCode(t *testing.T) {
	e := stalePulseEnv(t)
	e.notifyErr = errors.New("osascript unavailable")
	var out, errb bytes.Buffer
	if code := run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 1 {
		t.Fatalf("exit = %d, want 1 despite notify failure", code)
	}
	if !strings.Contains(errb.String(), "notify") {
		t.Errorf("stderr does not report the notify failure: %s", errb.String())
	}
}

// AA-19: a missing config is a usage error, not a quiet OK.
func TestRun_MissingConfigExits2(t *testing.T) {
	e := newRunEnv(t, `{"pulses":[{"name":"a","type":"command","command":["true"]}]}`)
	var out, errb bytes.Buffer
	args := e.args()
	args[1] = filepath.Join(e.dir, "no-such-config.json")
	if code := run(args, &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
}

// AA-20: dry-run alarms in the report but performs no side effects.
func TestRun_DryRunHasNoSideEffects(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	if code := run(e.args("--dry-run"), &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if len(e.notices) != 0 {
		t.Errorf("dry run notified: %v", e.notices)
	}
	for _, p := range []string{e.state, e.journal, e.heartbeat} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("dry run wrote %s", p)
		}
	}
}

// AA-21 + AA-22: JSON mode emits one object listing every pulse once.
func TestRun_JSONReport(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	run(e.args("--json"), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	var rep report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("stdout is not one JSON object: %v\n%s", err, out.String())
	}
	if len(rep.Results) != 1 || rep.Alarming != 1 || rep.Results[0].Name != "spans" {
		t.Fatalf("report = %+v", rep)
	}
}

// AA-23: an unwritable journal is reported on stderr and the exit code holds.
func TestRun_UnwritableJournalKeepsExitCode(t *testing.T) {
	e := stalePulseEnv(t)
	// A journal path whose parent is a file cannot be created.
	blocker := filepath.Join(e.dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := e.args()
	for i, a := range args {
		if a == e.journal {
			args[i] = filepath.Join(blocker, "journal.jsonl")
		}
	}
	var out, errb bytes.Buffer
	if code := run(args, &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if !strings.Contains(errb.String(), "journal") {
		t.Errorf("stderr does not report the journal failure: %s", errb.String())
	}
}

// AA-24: a hung command pulse must not disable the monitor. The probe is
// bounded, its expiry classifies as UNDETERMINED rather than ABSENT (a check
// that never finished is not evidence the subject is missing), and every later
// pulse is still evaluated and still reaches the heartbeat.
func TestRun_HungProbeDoesNotDisableTheTick(t *testing.T) {
	dir := t.TempDir()
	fresh := filepath.Join(dir, "fresh.log")
	if err := os.WriteFile(fresh, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := `{"pulses":[
	  {"name":"hangs","type":"command","command":["hang"],"expect":"a probe that returns"},
	  {"name":"after","type":"file_mtime","path":"` + fresh + `","window":"1h","expect":"a fresh file"}
	]}`
	e := newRunEnv(t, doc)

	probes := absencealarm.DefaultProbes()
	var sawDeadline bool
	probes.RunCommand = func(ctx context.Context, _ []string) (int, error) {
		if _, ok := ctx.Deadline(); ok {
			sawDeadline = true
		}
		// Stand in for a process that outlives its budget: block until the
		// probe's own context is cancelled, then report as CommandContext does.
		<-ctx.Done()
		return -1, ctx.Err()
	}

	done := make(chan int, 1)
	var out, errb bytes.Buffer
	go func() { done <- run(e.args("--probe-timeout", "150ms"), &out, &errb, probes, e.notifier()) }()

	var code int
	select {
	case code = <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("run did not return: a hung probe stalled the whole tick (AA-24)")
	}

	if !sawDeadline {
		t.Error("command probe ran with no deadline (AA-24)")
	}
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr: %s", code, errb.String())
	}

	var hb absencealarm.Heartbeat
	hraw, err := os.ReadFile(e.heartbeat)
	if err != nil {
		t.Fatalf("heartbeat not written after a hung probe (AA-24): %v", err)
	}
	if err := json.Unmarshal(hraw, &hb); err != nil {
		t.Fatalf("heartbeat unreadable: %v", err)
	}
	if len(hb.Results) != 2 {
		t.Fatalf("heartbeat has %d results, want 2: the pulse after the hung one was skipped (AA-24)", len(hb.Results))
	}
	byName := map[string]absencealarm.Status{}
	for _, r := range hb.Results {
		byName[r.Name] = r.Status
	}
	if got := byName["hangs"]; got != absencealarm.StatusUndetermined {
		t.Errorf("hung probe status = %q, want UNDETERMINED: a check that never finished is not proof of absence (AA-24)", got)
	}
	if got := byName["after"]; got != absencealarm.StatusPresent {
		t.Errorf("pulse after the hung one = %q, want PRESENT", got)
	}
}

// An exhausted tick budget is itself an alarm condition. Dispatching on the
// expired tick context would hand the notifier a already-canceled context, so
// DesktopDispatcher's exec.CommandContext would fail immediately and the
// operator would lose exactly the notification the timeout should produce.
func TestRun_NotificationSurvivesAnExpiredTickBudget(t *testing.T) {
	e := stalePulseEnv(t)

	var notifyCtxErr error
	notifyCtx := func(ctx context.Context, title, body string) error {
		notifyCtxErr = ctx.Err()
		e.notices = append(e.notices, title+" | "+body)
		return nil
	}

	var out, errb bytes.Buffer
	// A tick budget this small is already expired by the time any pulse is
	// classified, which is the scenario the reviewer described.
	code := run(e.args("--tick-timeout", "1ns"), &out, &errb, absencealarm.DefaultProbes(), notifyCtx)

	if len(e.notices) == 0 {
		t.Fatalf("no notification dispatched after the tick budget expired; stderr=%s", errb.String())
	}
	if notifyCtxErr != nil {
		t.Errorf("notifier received a context already in error: %v", notifyCtxErr)
	}
	if code == 0 {
		t.Errorf("exit code = 0, want non-zero for an alarming pulse")
	}
}

// Unexpected positional arguments before or after flags exit 2.
func TestRun_UnexpectedPositionalExits2(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	args := append([]string{"accidental"}, e.args()...)
	if code := run(args, &out, &errb, absencealarm.DefaultProbes(), e.notifier()); code != 2 {
		t.Fatalf("exit = %d, want 2 for unexpected positionals", code)
	}
	if !strings.Contains(errb.String(), "unexpected positional argument") {
		t.Errorf("stderr = %q, want unexpected positional error", errb.String())
	}
}

// AA-21: Human report includes window and evidence when populated.
func TestRun_HumanReportIncludesWindowAndEvidence(t *testing.T) {
	e := stalePulseEnv(t)
	var out, errb bytes.Buffer
	run(e.args(), &out, &errb, absencealarm.DefaultProbes(), e.notifier())
	reportStr := out.String()
	if !strings.Contains(reportStr, "window 1h") {
		t.Errorf("human report missing window: %s", reportStr)
	}
	if !strings.Contains(reportStr, "[evidence") {
		t.Errorf("human report missing evidence: %s", reportStr)
	}
}
