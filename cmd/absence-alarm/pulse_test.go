package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func fixedProbes() probes {
	return probes{
		now: func() time.Time { return t0 },
	}
}

// AA-01: stale mtime is ABSENT.
func TestFileMtimePulse_StaleIsAbsent(t *testing.T) {
	pr := fixedProbes()
	pr.statMtime = func(string) (time.Time, bool, error) { return t0.Add(-3 * time.Hour), true, nil }
	p := Pulse{Name: "spans", Type: PulseFileMtime, Path: "/x/spans.jsonl", Window: "1h", window: time.Hour}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
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
	pr.statMtime = func(string) (time.Time, bool, error) { return t0.Add(-10 * time.Minute), true, nil }
	p := Pulse{Name: "spans", Type: PulseFileMtime, Path: "/x/spans.jsonl", Window: "1h", window: time.Hour}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
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
	pr.statMtime = func(string) (time.Time, bool, error) { return time.Time{}, false, nil }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "does not exist") {
		t.Fatalf("got %s %q, want absent with missing-file reason", res.Status, res.Reason)
	}
}

// AA-05: an unreadable stat is UNDETERMINED, not OK.
func TestFileMtimePulse_StatErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.statMtime = func(string) (time.Time, bool, error) { return time.Time{}, false, errors.New("permission denied") }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
	if !res.Status.alarming() {
		t.Error("undetermined must alarm")
	}
}

// AA-06: a future mtime beyond skew tolerance is UNDETERMINED.
func TestFileMtimePulse_FutureMtimeIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.statMtime = func(string) (time.Time, bool, error) { return t0.Add(10 * time.Minute), true, nil }
	p := Pulse{Name: "hb", Type: PulseFileMtime, Path: "/x/hb.json", Window: "1h", window: time.Hour}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined for a future mtime", res.Status)
	}
}

// AA-03: label missing from the launchd listing is ABSENT; present label is PRESENT.
func TestLaunchdPulse(t *testing.T) {
	listing := "123\t0\tcom.dear-agent.disk-watchdog\n-\t0\tcom.dear-agent.loop-tick\n"
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	res := evaluatePulse(context.Background(), p, pr, listing, nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "not loaded") {
		t.Fatalf("got %s %q, want absent not-loaded", res.Status, res.Reason)
	}
	p.Label = "com.dear-agent.disk-watchdog"
	if res := evaluatePulse(context.Background(), p, pr, listing, nil); res.Status != StatusPresent {
		t.Fatalf("loaded label: status = %s, want present", res.Status)
	}
}

// AA-03: a label must match the whole column, not a substring of another label.
func TestLaunchdPulse_NoSubstringMatch(t *testing.T) {
	listing := "123\t0\tcom.dear-agent.mergeloop-canary\n"
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	if res := evaluatePulse(context.Background(), p, pr, listing, nil); res.Status != StatusAbsent {
		t.Fatalf("status = %s, want absent (substring must not count)", res.Status)
	}
}

// AA-05: an unobtainable launchd listing is UNDETERMINED.
func TestLaunchdPulse_ListErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	p := Pulse{Name: "mergeloop-loaded", Type: PulseLaunchdLoaded, Label: "com.dear-agent.mergeloop"}
	res := evaluatePulse(context.Background(), p, pr, "", errors.New("launchctl exploded"))
	if res.Status != StatusUndetermined {
		t.Fatalf("status = %s, want undetermined", res.Status)
	}
}

// AA-04: non-zero exit is ABSENT with the exit status; zero exit is PRESENT.
func TestCommandPulse(t *testing.T) {
	pr := fixedProbes()
	pr.runCommand = func(_ context.Context, argv []string) (int, error) { return 1, nil }
	p := Pulse{Name: "main-merge", Type: PulseCommand, Command: []string{"check-merge"}}
	res := evaluatePulse(context.Background(), p, pr, "", nil)
	if res.Status != StatusAbsent || !strings.Contains(res.Reason, "exited 1") {
		t.Fatalf("got %s %q, want absent exited-1", res.Status, res.Reason)
	}
	pr.runCommand = func(_ context.Context, argv []string) (int, error) { return 0, nil }
	if res := evaluatePulse(context.Background(), p, pr, "", nil); res.Status != StatusPresent {
		t.Fatalf("zero exit: status = %s, want present", res.Status)
	}
}

// AA-05: a command that cannot start is UNDETERMINED.
func TestCommandPulse_StartErrorIsUndetermined(t *testing.T) {
	pr := fixedProbes()
	pr.runCommand = func(_ context.Context, argv []string) (int, error) { return -1, errors.New("no such binary") }
	p := Pulse{Name: "main-merge", Type: PulseCommand, Command: []string{"check-merge"}}
	if res := evaluatePulse(context.Background(), p, pr, "", nil); res.Status != StatusUndetermined {
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
		{"duplicate name", `{"pulses":[{"name":"a","type":"launchd_loaded","label":"x"},{"name":"a","type":"launchd_loaded","label":"y"}]}`, "duplicate pulse name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "pulses.json")
			if err := os.WriteFile(path, []byte(tc.doc), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := loadPulseConfig(path)
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
		{"name":"merge","type":"command","command":["true"]}]}`
	path := filepath.Join(t.TempDir(), "pulses.json")
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	pulses, err := loadPulseConfig(path)
	if err != nil {
		t.Fatalf("loadPulseConfig: %v", err)
	}
	if len(pulses) != 3 {
		t.Fatalf("len = %d, want 3", len(pulses))
	}
	if strings.HasPrefix(pulses[0].Path, "~") {
		t.Errorf("path %q not home-expanded", pulses[0].Path)
	}
	if pulses[0].window != 24*time.Hour {
		t.Errorf("window = %s, want 24h", pulses[0].window)
	}
}
