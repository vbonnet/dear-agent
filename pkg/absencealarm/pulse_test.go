package absencealarm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func fixedProbes() Probes {
	return Probes{
		Now: func() time.Time { return t0 },
	}
}

// AA-01: stale mtime is ABSENT.
func TestFileMtimePulse_StaleIsAbsent(t *testing.T) {
	pr := fixedProbes()
	pr.StatMtime = func(string) (time.Time, bool, error) { return t0.Add(-3 * time.Hour), true, nil }
	p := Pulse{Name: "spans", Type: PulseFileMtime, Path: "/x/spans.jsonl", Window: "1h", window: time.Hour}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent {
		t.Fatalf("status = %s, want absent", res.Status)
	}
	if !strings.Contains(res.Reason, "window 1h") {
		t.Errorf("reason %q does not name the window", res.Reason)
	}
}

// AA-01: fresh mtime is PRESENT.
func TestFileMtimePulse_FreshIsPresent(t *testing.T) {
	pr := fixedProbes()
	pr.StatMtime = func(string) (time.Time, bool, error) { return t0.Add(-10 * time.Minute), true, nil }
	p := Pulse{Name: "spans", Type: PulseFileMtime, Path: "/x/spans.jsonl", Window: "1h", window: time.Hour}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusPresent {
		t.Fatalf("status = %s, want present", res.Status)
	}
	if !res.Evidence.Equal(t0.Add(-10 * time.Minute)) {
		t.Errorf("evidence = %s, want the mtime", res.Evidence)
	}
}

// AA-02: missing file is ABSENT with a missing-file reason.
func TestFileMtimePulse_MissingIsAbsent(t *testing.T) {
	pr := fixedProbes()
	pr.StatMtime = func(string) (time.Time, bool, error) { return time.Time{}, false, nil }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "does not exist") {
		t.Fatalf("got %s %q, want absent with missing-file reason", res.Status, res.Reason)
	}
}

// AA-05: an unreadable stat is UNDETERMINED, not OK.
func TestFileMtimePulse_StatErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.StatMtime = func(string) (time.Time, bool, error) { return time.Time{}, false, errors.New("permission denied") }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
	if !res.Status.Alarming() {
		t.Error("undetermined must alarm")
	}
}

// AA-06: a future mtime beyond skew tolerance is UNDETERMINED.
func TestFileMtimePulse_FutureMtimeIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.StatMtime = func(string) (time.Time, bool, error) { return t0.Add(10 * time.Minute), true, nil }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined for a future mtime", res.Status)
	}
}

// AA-03: label missing from the launchd listing is ABSENT; present label is PRESENT.
func TestLaunchdPulse(t *testing.T) {
	listing := "123\t0\tcom.dear-agent.disk-watchdog\n-\t0\tcom.dear-agent.loop-tick\n"
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	res := EvaluatePulse(context.Background(), p, pr, listing, nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "not loaded") {
		t.Fatalf("got %s %q, want absent not-loaded", res.Status, res.Reason)
	}
	p.Label = "com.dear-agent.disk-watchdog"
	if res := EvaluatePulse(context.Background(), p, pr, listing, nil); res.Status != StatusPresent {
		t.Fatalf("loaded label: status = %s, want present", res.Status)
	}
}

// AA-03: a label must match the whole column, not a substring of another label.
func TestLaunchdPulse_NoSubstringMatch(t *testing.T) {
	listing := "123\t0\tcom.dear-agent.mergeloop-canary\n"
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	if res := EvaluatePulse(context.Background(), p, pr, listing, nil); res.Status != StatusAbsent {
		t.Fatalf("status = %s, want absent (substring must not count)", res.Status)
	}
}

// AA-05: an unobtainable launchd listing is UNDETERMINED.
func TestLaunchdPulse_ListErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	res := EvaluatePulse(context.Background(), p, pr, "", errors.New("launchctl exploded"))
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
}

// AA-04: non-zero exit is ABSENT with the exit status; zero exit is PRESENT.
func TestCommandPulse(t *testing.T) {
	pr := fixedProbes()
	pr.RunCommand = func(_ context.Context, argv []string) (int, string, error) { return 1, "", nil }
	p := Pulse{Name: "main-merge", Type: PulseCommand, Command: []string{"check-merge"}}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "exited 1") {
		t.Fatalf("got %s %q, want absent exited-1", res.Status, res.Reason)
	}
	pr.RunCommand = func(_ context.Context, argv []string) (int, string, error) { return 0, "", nil }
	if res := EvaluatePulse(context.Background(), p, pr, "", nil); res.Status != StatusPresent {
		t.Fatalf("zero exit: status = %s, want present", res.Status)
	}
}

// AA-05: a command that cannot start is UNDETERMINED.
func TestCommandPulse_StartErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.RunCommand = func(_ context.Context, argv []string) (int, string, error) {
		return -1, "", errors.New("no such binary")
	}
	p := Pulse{Name: "main-merge", Type: PulseCommand, Command: []string{"check-merge"}}
	if res := EvaluatePulse(context.Background(), p, pr, "", nil); res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
}

// AA-19: config validation refuses unknown types, missing fields, bad and
// non-positive windows, duplicate names, and an empty pulse list.
func TestLoadPulseConfig_Validation(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"empty", `{"pulses":[]}`, "no pulses"},
		{"unknown type", `{"pulses":[{"name":"a","type":"nope"}]}`, "unknown type"},
		{"mtime missing path", `{"pulses":[{"name":"a","type":"file_mtime","window":"1h"}]}`, "requires path"},
		{"mtime missing window", `{"pulses":[{"name":"a","type":"file_mtime","path":"/x"}]}`, "requires window"},
		{"bad window", `{"pulses":[{"name":"a","type":"file_mtime","path":"/x","window":"soon"}]}`, "bad window"},
		{"negative window", `{"pulses":[{"name":"a","type":"file_mtime","path":"/x","window":"-1h"}]}`, "must be positive"},
		{"launchd missing label", `{"pulses":[{"name":"a","type":"launchd_loaded"}]}`, "requires label"},
		{"command empty", `{"pulses":[{"name":"a","type":"command"}]}`, "non-empty command"},
		{"json_timestamp missing path", `{"pulses":[{"name":"a","type":"json_timestamp","field":"ts","window":"1h"}]}`, "requires path"},
		{"json_timestamp missing field", `{"pulses":[{"name":"a","type":"json_timestamp","path":"/x","window":"1h"}]}`, "requires field"},
		{"json_timestamp missing window", `{"pulses":[{"name":"a","type":"json_timestamp","path":"/x","field":"ts"}]}`, "requires window"},
		{"json_timestamp bad window", `{"pulses":[{"name":"a","type":"json_timestamp","path":"/x","field":"ts","window":"soon"}]}`, "bad window"},
		{"json_timestamp negative window", `{"pulses":[{"name":"a","type":"json_timestamp","path":"/x","field":"ts","window":"-1h"}]}`, "must be positive"},
		{"duplicate name", `{"pulses":[{"name":"a","type":"launchd_loaded","label":"x"},{"name":"a","type":"launchd_loaded","label":"y"}]}`, "duplicate pulse name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pulses.json")
			if err := os.WriteFile(path, []byte(tc.doc), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadPulseConfig(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoadPulseConfig_Valid(t *testing.T) {
	doc := `{"pulses":[
		{"name":"spans","type":"file_mtime","path":"~/x/spans.jsonl","window":"24h","expect":"a fresh OTel span file"},
		{"name":"mergeloop","type":"launchd_loaded","label":"com.dear-agent.mergeloop"},
		{"name":"merge","type":"command","command":["true"]},
		{"name":"supervisor","type":"json_timestamp","path":"~/hb.json","field":"last_beat_utc","window":"10m"}]}`
	path := filepath.Join(t.TempDir(), "pulses.json")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	pulses, err := LoadPulseConfig(path)
	if err != nil {
		t.Fatalf("LoadPulseConfig: %v", err)
	}
	if len(pulses) != 4 {
		t.Fatalf("len = %d, want 4", len(pulses))
	}
	if strings.HasPrefix(pulses[0].Path, "~") {
		t.Errorf("path %q not home-expanded", pulses[0].Path)
	}
	if pulses[0].window != 24*time.Hour {
		t.Errorf("window = %s, want 24h", pulses[0].window)
	}
}

// A stat on a wedged mount blocks forever. The probe deadline must still
// return, classifying the pulse UNDETERMINED, so later pulses, notifications,
// and the heartbeat are not held hostage by one path.
func TestEvaluatePulseBoundsAWedgedFileStat(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	probes := Probes{
		StatMtime: func(string) (time.Time, bool, error) {
			<-release // never returns within the deadline
			return time.Time{}, false, nil
		},
		Now: func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	done := make(chan Result, 1)
	go func() {
		done <- EvaluatePulse(ctx, Pulse{
			Name: "spans", Type: PulseFileMtime, Path: "/mnt/wedged/spans.jsonl", Window: "1h",
		}, probes, "", nil)
	}()

	select {
	case res := <-done:
		if res.Status != StatusUndetermined {
			t.Errorf("Status = %q, want UNDETERMINED for a stat that never returns", res.Status)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("EvaluatePulse did not return after the probe deadline expired")
	}
}

// AA-05: a stat hook that panics must classify the pulse UNDETERMINED, not
// take the process down. The bounded stat runs the hook on its own goroutine
// that deliberately outlives this call, so a panic there cannot be recovered
// by the caller and would kill the whole tick: no later pulses, no
// notifications, no heartbeat. That is the silent failure this alarm exists to
// detect, caused by the alarm itself.
func TestEvaluatePulseSurvivesAPanickingFileStat(t *testing.T) {
	probes := Probes{
		StatMtime: func(string) (time.Time, bool, error) {
			panic("stat hook blew up")
		},
		Now: func() time.Time { return t0 },
	}

	res := EvaluatePulse(context.Background(), Pulse{
		Name: "spans", Type: PulseFileMtime, Path: "/mnt/broken/spans.jsonl", Window: "1h",
	}, probes, "", nil)

	if res.Status != StatusUndetermined {
		t.Errorf("Status = %q, want UNDETERMINED for a stat hook that panics", res.Status)
	}
	if !strings.Contains(res.Reason, "stat hook blew up") {
		t.Errorf("Reason = %q, want it to carry the panic value", res.Reason)
	}
}

// AA-19 covers a pulse configuration that cannot be loaded, is empty, or is
// invalid. TestLoadPulseConfig_Validation already pins the invalid-pulse
// table, but the load failures ahead of validation were unproven: the config
// gates every pulse, so a tick that cannot read it must refuse loudly rather
// than come back with an empty pulse set, which would report all-clear and
// silence the alarm entirely.
func TestLoadPulseConfig_UnloadableRefuses(t *testing.T) {
	dir := t.TempDir()

	unreadable := filepath.Join(dir, "config")
	if err := os.Mkdir(unreadable, 0o755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	malformed := filepath.Join(dir, "malformed.json")
	if err := os.WriteFile(malformed, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"pulses":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, path, want string
	}{
		{"unreadable", unreadable, "read pulse config"},
		{"missing", filepath.Join(dir, "absent.json"), "read pulse config"},
		{"malformed", malformed, "parse pulse config"},
		{"no pulses", empty, "no pulses configured"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pulses, err := LoadPulseConfig(tc.path)
			if err == nil {
				t.Fatalf("LoadPulseConfig(%s) = nil error, want a refusal", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if pulses != nil {
				t.Errorf("pulses = %v, want nil so no tick treats this as all-clear", pulses)
			}
		})
	}
}

func TestCommandPulseCarriesProbeOutputIntoTheReason(t *testing.T) {
	// AA-16: a command probe already knows why it is unhappy, and its stdout is
	// written to be read by a responder. Reporting only "exited 1" throws that
	// away and reproduces, inside the alarm itself, the failure this whole
	// package exists to end: a monitor that says something is wrong without
	// saying what. The reason must carry the probe's own summary.
	pr := DefaultProbes()
	pr.RunCommand = func(_ context.Context, _ []string) (int, string, error) {
		return 1, "gate-health: SYSTEMIC GATE FAILURE\n  check:  govulncheck\n  likely fix: bump x/crypto\n", nil
	}
	p := Pulse{Name: "gate", Type: PulseCommand, Command: []string{"gate-health"}, Expect: "no systemic gate failure"}

	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent {
		t.Fatalf("status = %q, want %q", res.Status, StatusAbsent)
	}
	for _, want := range []string{"govulncheck", "bump x/crypto"} {
		if !strings.Contains(res.Reason, want) {
			t.Errorf("Reason missing %q, got:\n%s", want, res.Reason)
		}
	}
}

func TestCommandPulseReasonFallsBackToExitCodeWhenSilent(t *testing.T) {
	// AA-17: a probe that fails without saying anything still yields a reason
	// naming the command and its exit code.
	pr := DefaultProbes()
	pr.RunCommand = func(_ context.Context, _ []string) (int, string, error) { return 3, "   \n", nil }
	p := Pulse{Name: "quiet", Type: PulseCommand, Command: []string{"quiet-probe", "--json"}}

	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent {
		t.Fatalf("status = %q, want %q", res.Status, StatusAbsent)
	}
	if !strings.Contains(res.Reason, "quiet-probe --json") || !strings.Contains(res.Reason, "3") {
		t.Errorf("Reason = %q, want the command and exit code", res.Reason)
	}
}

func TestCommandPulseReasonIsBounded(t *testing.T) {
	// AA-18: probe output is untrusted in length. A runaway probe must not push
	// a megabyte into the escalation journal or a desktop notification, so the
	// captured summary is truncated with a marker rather than dropped.
	pr := DefaultProbes()
	pr.RunCommand = func(_ context.Context, _ []string) (int, string, error) {
		return 1, strings.Repeat("x", 10_000), nil
	}
	p := Pulse{Name: "loud", Type: PulseCommand, Command: []string{"loud-probe"}}

	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if len(res.Reason) > maxReasonBytes+200 {
		t.Fatalf("Reason is %d bytes, want it bounded near %d", len(res.Reason), maxReasonBytes)
	}
	if !strings.Contains(res.Reason, "truncated") {
		t.Errorf("Reason must mark truncation, got tail: %q", res.Reason[max(0, len(res.Reason)-80):])
	}
}

func TestCommandPulseReasonTruncatesOnRuneBoundary(t *testing.T) {
	// A probe output that exceeds maxReasonBytes must not split a multi-byte
	// UTF-8 rune in half.
	prefix := strings.Repeat("a", maxReasonBytes-1)
	multiByteStr := prefix + "€€€"
	pr := DefaultProbes()
	pr.RunCommand = func(_ context.Context, _ []string) (int, string, error) {
		return 1, multiByteStr, nil
	}
	p := Pulse{Name: "utf8", Type: PulseCommand, Command: []string{"utf8-probe"}}

	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if !utf8.ValidString(res.Reason) {
		t.Fatalf("Reason contains invalid UTF-8: %q", res.Reason)
	}
	if !strings.Contains(res.Reason, "truncated") {
		t.Errorf("Reason must mark truncation: %q", res.Reason)
	}
}

// AA-26: fresh json_timestamp is PRESENT.
func TestJSONTimestampPulse_FreshIsPresent(t *testing.T) {
	pr := fixedProbes()
	ts := t0.Add(-5 * time.Minute)
	data := []byte(`{"id":"vroom-overseer","last_beat_utc":"` + ts.Format(time.RFC3339Nano) + `"}`)
	pr.ReadFile = func(string) ([]byte, error) { return data, nil }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusPresent {
		t.Fatalf("status = %s, want present", res.Status)
	}
	if !res.Evidence.Equal(ts) {
		t.Errorf("evidence = %s, want %s", res.Evidence, ts)
	}
}

// AA-26: stale json_timestamp is ABSENT.
func TestJSONTimestampPulse_StaleIsAbsent(t *testing.T) {
	pr := fixedProbes()
	ts := t0.Add(-30 * time.Minute)
	data := []byte(`{"last_beat_utc":"` + ts.Format(time.RFC3339) + `"}`)
	pr.ReadFile = func(string) ([]byte, error) { return data, nil }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent {
		t.Fatalf("status = %s, want absent", res.Status)
	}
	if !strings.Contains(res.Reason, "window 10m") {
		t.Errorf("reason %q does not name the window", res.Reason)
	}
}

// AA-27: missing file for json_timestamp is ABSENT with missing-file reason.
func TestJSONTimestampPulse_MissingIsAbsent(t *testing.T) {
	pr := fixedProbes()
	pr.ReadFile = func(string) ([]byte, error) { return nil, os.ErrNotExist }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "does not exist") {
		t.Fatalf("got %s %q, want absent with does not exist", res.Status, res.Reason)
	}
}

// AA-28: unreadable file is UNDETERMINED.
func TestJSONTimestampPulse_ReadErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.ReadFile = func(string) ([]byte, error) { return nil, errors.New("permission denied") }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
}

// AA-28: invalid JSON is UNDETERMINED.
func TestJSONTimestampPulse_InvalidJSONIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.ReadFile = func(string) ([]byte, error) { return []byte("not valid json"), nil }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined || !strings.Contains(res.Reason, "parsing JSON") {
		t.Fatalf("got %s %q, want undetermined with parsing JSON reason", res.Status, res.Reason)
	}
}

// AA-28: missing field in JSON is UNDETERMINED.
func TestJSONTimestampPulse_MissingFieldIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.ReadFile = func(string) ([]byte, error) { return []byte(`{"other_field":123}`), nil }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined || !strings.Contains(res.Reason, "not found") {
		t.Fatalf("got %s %q, want undetermined with not found reason", res.Status, res.Reason)
	}
}

// AA-06: timestamp in the future beyond tolerance is UNDETERMINED.
func TestJSONTimestampPulse_FutureTimestampIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	futureTS := t0.Add(10 * time.Minute)
	data := []byte(`{"last_beat_utc":"` + futureTS.Format(time.RFC3339) + `"}`)
	pr.ReadFile = func(string) ([]byte, error) { return data, nil }
	p := Pulse{Name: "overseer", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "last_beat_utc", Window: "10m", window: 10 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined || !strings.Contains(res.Reason, "future") {
		t.Fatalf("got %s %q, want undetermined with future timestamp reason", res.Status, res.Reason)
	}
}

// Test nested field and numeric/unix epoch formats.
func TestJSONTimestampPulse_NestedFieldAndFormats(t *testing.T) {
	pr := fixedProbes()
	ts := t0.Add(-2 * time.Minute)

	// Nested object with RFC3339
	data := []byte(`{"meta":{"deep":{"ts":"` + ts.Format(time.RFC3339) + `"}}}`)
	pr.ReadFile = func(string) ([]byte, error) { return data, nil }
	p := Pulse{Name: "nested", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "meta.deep.ts", Window: "5m", window: 5 * time.Minute}
	res := EvaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusPresent {
		t.Fatalf("nested status = %s, want present", res.Status)
	}

	// DateTime string format
	pr.ReadFile = func(string) ([]byte, error) {
		return []byte(`{"ts":"` + ts.Format("2006-01-02 15:04:05") + `"}`), nil
	}
	res = EvaluatePulse(context.Background(), Pulse{Name: "dt", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "ts", Window: "5m", window: 5 * time.Minute}, pr, "", nil)
	if res.Status != StatusPresent {
		t.Fatalf("datetime status = %s, want present", res.Status)
	}

	// Unix epoch integer seconds
	epochSec := ts.Unix()
	pr.ReadFile = func(string) ([]byte, error) {
		return []byte(`{"epoch":` + time.Unix(epochSec, 0).Format("1136239445") + `}`), nil
	}
	// Let's use a json payload with the integer
	pr.ReadFile = func(string) ([]byte, error) {
		return []byte(`{"epoch":1788263880}`), nil
	}
	res = EvaluatePulse(context.Background(), Pulse{Name: "epoch", Type: PulseJSONTimestamp, Path: "/x/hb.json", Field: "epoch", Window: "5m", window: 5 * time.Minute}, pr, "", nil)
	if res.Status != StatusPresent {
		t.Fatalf("epoch status = %s, want present", res.Status)
	}
}

// Bounded read test: A hung read on a wedged mount must not block past deadline.
func TestEvaluatePulseBoundsAWedgedReadFile(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	probes := Probes{
		ReadFile: func(string) ([]byte, error) {
			<-release
			return nil, nil
		},
		Now: func() time.Time { return t0 },
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	p := Pulse{Name: "slow", Type: PulseJSONTimestamp, Path: "/wedged/hb.json", Field: "ts", Window: "1h", window: time.Hour}
	res := EvaluatePulse(ctx, p, probes, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
	if !strings.Contains(res.Reason, "probe deadline") {
		t.Errorf("reason %q must mention probe deadline", res.Reason)
	}
}
