package deploy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readPulseNames(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	names := make([]string, 0, len(doc.Pulses))
	for _, p := range doc.Pulses {
		names = append(names, p.Name)
	}
	return names
}

// The deployed pulse config is absent-only: once a host has one, no install
// path ever rewrites it. A new required pulse therefore reaches the repository
// and never reaches the running alarm, which is how a monitor ends up blind to
// something the registry believes it watches.
func TestMergeRequiredPulses_AddsMissingAndKeepsCustomisation(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "absence-alarm-pulses.json")
	defaults := filepath.Join(dir, "pulses.json")

	// The host has an operator-widened window and a pulse of its own.
	if err := os.WriteFile(host, []byte(`{"pulses":[
	  {"name":"disk-watchdog-tick","type":"file_mtime","path":"~/x.log","window":"6h"},
	  {"name":"operator-local","type":"file_mtime","path":"~/local.log","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"disk-watchdog-tick","type":"file_mtime","path":"~/x.log","window":"1h"},
	  {"name":"sandbox-gc-tick","type":"file_mtime","path":"~/gc.log","window":"2h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults)
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 1 || added[0] != "sandbox-gc-tick" {
		t.Errorf("added = %v, want [sandbox-gc-tick]", added)
	}

	names := readPulseNames(t, host)
	want := map[string]bool{"disk-watchdog-tick": true, "operator-local": true, "sandbox-gc-tick": true}
	if len(names) != 3 {
		t.Fatalf("pulses = %v, want 3", names)
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected pulse %q", n)
		}
	}

	// The operator's widened window must survive: this merges in what is
	// missing, it does not reassert the defaults over local choices.
	raw, _ := os.ReadFile(host)
	var doc struct {
		Pulses []map[string]any `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	for _, p := range doc.Pulses {
		if p["name"] == "disk-watchdog-tick" && p["window"] != "6h" {
			t.Errorf("operator window overwritten: window = %v, want 6h", p["window"])
		}
	}
}

// Running it twice must change nothing the second time.
func TestMergeRequiredPulses_Idempotent(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	body := `{"pulses":[{"name":"a","type":"file_mtime","path":"~/a","window":"1h"}]}`
	if err := os.WriteFile(host, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults)
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v on an already-current host, want none", added)
	}
	before, _ := os.ReadFile(host)
	if _, err := MergeRequiredPulses(host, defaults); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after, _ := os.ReadFile(host)
	if string(before) != string(after) {
		t.Error("second run rewrote the file")
	}
}

// A host with no config yet gets the defaults verbatim.
func TestMergeRequiredPulses_SeedsAbsentHostConfig(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "nested", "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(defaults, []byte(
		`{"pulses":[{"name":"a","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults)
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 1 || added[0] != "a" {
		t.Errorf("added = %v, want [a]", added)
	}
	if got := readPulseNames(t, host); len(got) != 1 || got[0] != "a" {
		t.Errorf("seeded pulses = %v", got)
	}
}

// A corrupt host config must fail loudly rather than being silently replaced:
// overwriting it would discard operator configuration this cannot read.
func TestMergeRequiredPulses_CorruptHostConfigRefuses(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(`{ not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(
		`{"pulses":[{"name":"a","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeRequiredPulses(host, defaults); err == nil {
		t.Fatal("a corrupt host config was accepted; it would have been overwritten")
	}
	raw, _ := os.ReadFile(host)
	if string(raw) != `{ not json` {
		t.Error("the corrupt host config was modified")
	}
}

// A pulse an operator deliberately removed must stay removed.
//
// Without a record of what was previously installed, "missing from the host"
// is ambiguous: it means either "this host predates the pulse" or "the operator
// turned it off". Re-adding on every sync silently reactivates probes someone
// switched off, which produces exactly the unwanted alarms and recovery actions
// the preserve-customization promise is supposed to prevent.
func TestMergeRequiredPulses_DoesNotResurrectRemovedPulses(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"noisy","type":"file_mtime","path":"~/b","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"noisy","type":"file_mtime","path":"~/b","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// First sync adopts the current defaults and records them.
	if _, err := MergeRequiredPulses(host, defaults); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	// The operator switches "noisy" off.
	if err := os.WriteFile(host, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("re-added %v; the operator removed those deliberately", added)
	}
	if got := readPulseNames(t, host); len(got) != 1 || got[0] != "keep" {
		t.Errorf("host pulses = %v, want [keep]", got)
	}
}

// A genuinely new default, never seen by this host, is still installed.
func TestMergeRequiredPulses_StillAddsGenuinelyNewDefaults(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(
		`{"pulses":[{"name":"keep","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(
		`{"pulses":[{"name":"keep","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeRequiredPulses(host, defaults); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	// A later release adds a pulse this host has never seen.
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"brand-new","type":"file_mtime","path":"~/c","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := MergeRequiredPulses(host, defaults)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if len(added) != 1 || added[0] != "brand-new" {
		t.Errorf("added = %v, want [brand-new]", added)
	}
}
