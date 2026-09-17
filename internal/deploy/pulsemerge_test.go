package deploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func validPulseConfig(names ...string) []byte {
	type fixturePulse struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Path   string `json:"path"`
		Window string `json:"window"`
	}
	doc := struct {
		Pulses []fixturePulse `json:"pulses"`
	}{Pulses: make([]fixturePulse, 0, len(names))}
	for _, name := range names {
		doc.Pulses = append(doc.Pulses, fixturePulse{
			Name: name, Type: "file_mtime", Path: "/tmp/" + name, Window: "1h",
		})
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return raw
}

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

	added, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults))
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

func TestMergeRequiredPulses_PreservesExactNumbersInsidePulseEntries(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	const exact = "9007199254740993"
	if err := os.WriteFile(host, []byte(`{"pulses":[{
  "name":"existing","type":"file_mtime","path":"~/existing","window":"1h",
  "operator_sequence":9007199254740993,
  "metadata":{"nested_sequence":9007199254740993}
}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
  {"name":"existing","type":"file_mtime","path":"~/existing","window":"1h"},
  {"name":"new","type":"file_mtime","path":"~/new","window":"1h"}
]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults))
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	requireNames(t, added, "new")

	raw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Pulses []map[string]any `json:"pulses"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if got, ok := doc.Pulses[0]["operator_sequence"].(json.Number); !ok || got.String() != exact {
		t.Fatalf("operator_sequence = %#v, want exact %s", doc.Pulses[0]["operator_sequence"], exact)
	}
	metadata, ok := doc.Pulses[0]["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata = %#v, want object", doc.Pulses[0]["metadata"])
	}
	if got, ok := metadata["nested_sequence"].(json.Number); !ok || got.String() != exact {
		t.Fatalf("nested_sequence = %#v, want exact %s", metadata["nested_sequence"], exact)
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

	added, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults))
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v on an already-current host, want none", added)
	}
	before, _ := os.ReadFile(host)
	if _, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults)); err != nil {
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

	added, err := MergeRequiredPulses(host, defaults, 0o600, allRequired(defaults))
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 1 || added[0] != "a" {
		t.Errorf("added = %v, want [a]", added)
	}
	if got := readPulseNames(t, host); len(got) != 1 || got[0] != "a" {
		t.Errorf("seeded pulses = %v", got)
	}
	info, err := os.Stat(host)
	if err != nil {
		t.Fatalf("stat seeded registry: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("seeded mode = %04o, want manifest mode 0600", got)
	}
}

func TestMergeRequiredPulses_PreservesExistingRegistryMode(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(
		`{"pulses":[{"name":"existing","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"existing","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"new","type":"file_mtime","path":"~/b","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults))
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 1 || added[0] != "new" {
		t.Fatalf("added = %v, want [new]", added)
	}
	info, err := os.Stat(host)
	if err != nil {
		t.Fatalf("stat merged registry: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("merged mode = %04o, want existing mode 0600", got)
	}
}

func TestMergeRequiredPulses_PreservesUnknownTopLevelFields(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	hostRaw := []byte(`{
	  "pulses": [{"name":"existing","type":"file_mtime","path":"~/a","window":"1h"}],
	  "generation": 9007199254740993,
	  "enabled": true,
	  "metadata": {"ratio":1.2300,"labels":{"owner":"ops"}},
	  "owners": ["ops",{"rank":2}],
	  "override": null
	}`)
	if err := os.WriteFile(host, hostRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"existing","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"new","type":"file_mtime","path":"~/b","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults)); err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	afterRaw, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(hostRaw, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(afterRaw, &after); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"generation", "enabled", "metadata", "owners", "override"} {
		if got, want := compactJSON(t, after[field]), compactJSON(t, before[field]); got != want {
			t.Errorf("top-level field %q changed: got %s, want %s", field, got, want)
		}
	}
	if got := readPulseNames(t, host); len(got) != 2 || got[0] != "existing" || got[1] != "new" {
		t.Errorf("merged pulses = %v, want [existing new]", got)
	}
}

func compactJSON(t *testing.T, raw []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := json.Compact(&out, raw); err != nil {
		t.Fatalf("compact JSON %q: %v", raw, err)
	}
	return out.String()
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
	if _, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults)); err == nil {
		t.Fatal("a corrupt host config was accepted; it would have been overwritten")
	}
	raw, _ := os.ReadFile(host)
	if string(raw) != `{ not json` {
		t.Error("the corrupt host config was modified")
	}
}

// An unrequired pulse an operator deliberately removed must stay removed.
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

	required := map[string]bool{"keep": true}
	// First sync adopts the current defaults and records them.
	if _, err := MergeRequiredPulses(host, defaults, 0o644, required); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	// The operator switches "noisy" off.
	if err := os.WriteFile(host, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults, 0o644, required)
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

func TestMergeRequiredPulses_RefusesRemovedPulseStillRequiredByJob(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	defaultsRaw := []byte(`{"pulses":[
  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"},
  {"name":"required","type":"file_mtime","path":"~/b","window":"1h"}
]}`)
	if err := os.WriteFile(defaults, defaultsRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(host, defaultsRaw, 0o644); err != nil {
		t.Fatal(err)
	}
	required := allRequired(defaults)
	if _, err := MergeRequiredPulses(host, defaults, 0o644, required); err != nil {
		t.Fatalf("adopt initial registry: %v", err)
	}

	removed := validPulseConfig("keep")
	if err := os.WriteFile(host, removed, 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := MergeRequiredPulses(host, defaults, 0o644, required)
	if err == nil || !strings.Contains(err.Error(), "live pulse registry: required") {
		t.Fatalf("merge error = %v, added = %v; want missing required-pulse refusal", err, added)
	}
	after, readErr := os.ReadFile(host)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(after, removed) {
		t.Fatalf("failed required-pulse check changed operator registry: got %s, want %s", after, removed)
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
	if _, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults)); err != nil {
		t.Fatalf("first merge: %v", err)
	}

	// A later release adds a pulse this host has never seen.
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"keep","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"brand-new","type":"file_mtime","path":"~/c","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := MergeRequiredPulses(host, defaults, 0o644, allRequired(defaults))
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if len(added) != 1 || added[0] != "brand-new" {
		t.Errorf("added = %v, want [brand-new]", added)
	}
}

// allRequired treats every pulse in the given defaults file as required, which
// is what the pre-narrowing tests assume.
func allRequired(defaultsPath string) map[string]bool {
	raw, err := os.ReadFile(defaultsPath)
	if err != nil {
		return map[string]bool{}
	}
	var doc struct {
		Pulses []struct {
			Name string `json:"name"`
		} `json:"pulses"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return map[string]bool{}
	}
	req := make(map[string]bool, len(doc.Pulses))
	for _, p := range doc.Pulses {
		req[p.Name] = true
	}
	return req
}

// A default pulse that no recovery job depends on is the operator's business.
// Installing it unattended is what would resurrect probes removed before the
// ledger existed, since on such a host "missing" and "removed" are the same
// observation.
func TestMergeRequiredPulses_LeavesUnrequiredDefaultsAlone(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(
		`{"pulses":[{"name":"needed","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"needed","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"optional-probe","type":"file_mtime","path":"~/b","window":"1h"},
	  {"name":"job-critical","type":"file_mtime","path":"~/c","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := MergeRequiredPulses(host, defaults, 0o644, map[string]bool{"needed": true, "job-critical": true})
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != 1 || added[0] != "job-critical" {
		t.Errorf("added = %v, want [job-critical]", added)
	}
	for _, n := range readPulseNames(t, host) {
		if n == "optional-probe" {
			t.Error("installed a default no recovery job depends on")
		}
	}
}

// PendingPulseMerges must report exactly what a sync would do, and write nothing.
func TestPendingPulseMerges_ReportsWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(
		`{"pulses":[{"name":"a","type":"file_mtime","path":"~/a","window":"1h"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"a","type":"file_mtime","path":"~/a","window":"1h"},
	  {"name":"b","type":"file_mtime","path":"~/b","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	req := map[string]bool{"a": true, "b": true}

	before, _ := os.ReadFile(host)
	pending, err := PendingPulseMerges(host, defaults, req)
	if err != nil {
		t.Fatalf("PendingPulseMerges: %v", err)
	}
	if len(pending) != 1 || pending[0] != "b" {
		t.Errorf("pending = %v, want [b]", pending)
	}
	after, _ := os.ReadFile(host)
	if string(before) != string(after) {
		t.Error("PendingPulseMerges wrote to the host registry")
	}

	added, err := MergeRequiredPulses(host, defaults, 0o644, req)
	if err != nil {
		t.Fatalf("MergeRequiredPulses: %v", err)
	}
	if len(added) != len(pending) || added[0] != pending[0] {
		t.Errorf("sync added %v but the preview said %v", added, pending)
	}
}

// The real registries must agree: every pulse a recovery job names is required.
func TestRequiredPulseNames_CoversDeployedRegistry(t *testing.T) {
	req, err := RequiredPulseNames(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("RequiredPulseNames: %v", err)
	}
	for _, want := range []string{"sandbox-gc-tick", "token-refresher-tick", "absence-alarm-heartbeat"} {
		if !req[want] {
			t.Errorf("%q is named by a recovery job but not reported as required", want)
		}
	}
}

func TestRequiredPulseNamesRendered_UsesRenderedJobRegistryBytes(t *testing.T) {
	req, err := RequiredPulseNamesRendered([]byte(`{"jobs":[
	  {"name":"custom","pulse":"custom-tick"},
	  {"name":"no-pulse"}
	]}`))
	if err != nil {
		t.Fatalf("RequiredPulseNamesRendered: %v", err)
	}
	if !req["custom-tick"] {
		t.Error("rendered custom pulse was not required")
	}
	for _, want := range []string{"sandbox-gc-tick", "token-refresher-tick", "absence-alarm-heartbeat"} {
		if req[want] {
			t.Errorf("built-in pulse %q leaked into an explicit replacement registry", want)
		}
	}
}

func TestPulseOperationsRejectUndefinedRequiredPulse(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := []byte(`{"pulses":[{"name":"defined-tick","type":"file_mtime","path":"~/defined","window":"1h"}]}`)
	before := []byte(`{"pulses":[]}`)
	if err := os.WriteFile(host, before, 0o640); err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{"orphan-tick": true}

	if _, err := PendingPulseMergesRendered(host, defaults, required); err == nil || !strings.Contains(err.Error(), "orphan-tick") {
		t.Fatalf("preview error = %v, want undefined required pulse", err)
	}
	if _, err := MergeRequiredPulsesRenderedResult(host, defaults, 0o644, required); err == nil || !strings.Contains(err.Error(), "orphan-tick") {
		t.Fatalf("merge error = %v, want undefined required pulse", err)
	}

	after, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed validation changed registry: got %q, want %q", after, before)
	}
	if info, err := os.Stat(host); err != nil {
		t.Fatal(err)
	} else if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("failed validation changed mode to %04o", got)
	}
	for _, suffix := range []string{".offered", ".offered.pending"} {
		if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
			t.Fatalf("failed validation wrote %s: %v", suffix, err)
		}
	}
}

func TestPulseOperationsRejectRuntimeInvalidProjectedRegistry(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	before := []byte(`{"pulses":[{"name":"required-tick","type":"file_mtime","path":"~/required"}]}`)
	if err := os.WriteFile(host, before, 0o640); err != nil {
		t.Fatal(err)
	}
	defaults := []byte(`{"pulses":[{"name":"required-tick","type":"file_mtime","path":"~/required","window":"1h"}]}`)
	required := map[string]bool{"required-tick": true}

	if _, err := PendingPulseMergesRendered(host, defaults, required); err == nil || !strings.Contains(err.Error(), "requires window") {
		t.Fatalf("preview error = %v, want runtime validation failure", err)
	}
	if _, err := MergeRequiredPulsesRenderedResult(host, defaults, 0o644, required); err == nil || !strings.Contains(err.Error(), "requires window") {
		t.Fatalf("merge error = %v, want runtime validation failure", err)
	}

	after, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatalf("failed validation changed registry: got %q, want %q", after, before)
	}
	for _, suffix := range []string{".offered", ".offered.pending"} {
		if _, err := os.Lstat(host + suffix); !os.IsNotExist(err) {
			t.Fatalf("failed validation wrote %s: %v", suffix, err)
		}
	}
}

// Concurrent merges must not lose additions. Two post-merge deployments from
// different worktrees can overlap, and the registry plus its ledger are a
// read-modify-write pair: an interleaving that drops one run's additions while
// both ledgers record them as offered would leave those pulses permanently
// missing and permanently believed installed.
func TestMergeRequiredPulses_ConcurrentRunsDoNotLoseAdditions(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(host, []byte(`{"pulses":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, []byte(`{"pulses":[
	  {"name":"p1","type":"file_mtime","path":"~/1","window":"1h"},
	  {"name":"p2","type":"file_mtime","path":"~/2","window":"1h"},
	  {"name":"p3","type":"file_mtime","path":"~/3","window":"1h"}
	]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	req := map[string]bool{"p1": true, "p2": true, "p3": true}

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := MergeRequiredPulses(host, defaults, 0o644, req); err != nil {
				t.Errorf("concurrent merge: %v", err)
			}
		})
	}
	wg.Wait()

	got := readPulseNames(t, host)
	seen := map[string]int{}
	for _, n := range got {
		seen[n]++
	}
	for _, want := range []string{"p1", "p2", "p3"} {
		switch seen[want] {
		case 1:
			// installed exactly once
		case 0:
			t.Errorf("%q was lost by concurrent merges", want)
		default:
			t.Errorf("%q installed %d times", want, seen[want])
		}
	}
}

func TestMergeRequiredPulses_ConcurrentFirstSeedsConverge(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "host.json")
	oldDefaults := []byte(`{"pulses":[{"name":"old","type":"file_mtime","path":"~/old","window":"1h"}]}`)
	newDefaults := []byte(`{"pulses":[{"name":"new","type":"file_mtime","path":"~/new","window":"1h"}]}`)

	var wg sync.WaitGroup
	created := make(chan bool, 16)
	for i := range 16 {
		defaults := oldDefaults
		required := map[string]bool{"old": true}
		if i%2 == 1 {
			defaults = newDefaults
			required = map[string]bool{"new": true}
		}
		wg.Go(func() {
			result, err := MergeRequiredPulsesRenderedResult(host, defaults, 0o644, required)
			if err != nil {
				t.Errorf("concurrent first seed: %v", err)
				return
			}
			created <- result.Created
		})
	}
	wg.Wait()
	close(created)
	createdCount := 0
	for wasCreated := range created {
		if wasCreated {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("created receipts = %d, want exactly one", createdCount)
	}

	registryNames := readPulseNames(t, host)
	ledgerNames := readPulseLedgerNames(t, host)
	slices.Sort(registryNames)
	slices.Sort(ledgerNames)
	requireNames(t, registryNames, "new", "old")
	requireNames(t, ledgerNames, "new", "old")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}

func TestInstallAbsenceAlarmLaunchAgentUsesLockedPulseSeed(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	makefile := string(raw)
	start := strings.Index(makefile, "install-absence-alarm-launchagent:")
	if start < 0 {
		t.Fatal("Makefile has no install-absence-alarm-launchagent target")
	}
	rest := makefile[start:]
	target, _, found := strings.Cut(rest, "\nuninstall-absence-alarm-launchagent:")
	if !found {
		t.Fatal("cannot find end of install-absence-alarm-launchagent target")
	}
	if strings.Contains(target, "cp deploy/absence-alarm/pulses.json") {
		t.Fatal("install target seeds the pulse registry outside the locked merger")
	}
	if got := strings.Count(target, "go run ./cmd/dear-deploy merge-pulses"); got != 1 {
		t.Fatalf("install target delegates to locked merger %d times, want exactly once", got)
	}
}

func TestMergeRequiredPulses_PreservesRegistrySymlinkAndTargetMode(t *testing.T) {
	dir := t.TempDir()
	managedDir := filepath.Join(dir, "managed")
	logicalDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managedDir, "pulses.json")
	host := filepath.Join(logicalDir, "pulses.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(target, validPulseConfig("existing"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(defaults, validPulseConfig("existing", "new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "managed", "pulses.json"), host); err != nil {
		t.Fatal(err)
	}

	pending, err := PendingPulseMerges(host, defaults, map[string]bool{"existing": true, "new": true})
	if err != nil {
		t.Fatalf("preview through symlink: %v", err)
	}
	requireNames(t, pending, "new")
	added, err := MergeRequiredPulses(host, defaults, 0o644, map[string]bool{"existing": true, "new": true})
	if err != nil {
		t.Fatalf("merge through symlink: %v", err)
	}
	requireNames(t, added, "new")

	linkInfo, err := os.Lstat(host)
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("logical registry is no longer a symlink: info=%v err=%v", linkInfo, err)
	}
	requireNames(t, readPulseNames(t, target), "existing", "new")
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("symlink target mode = %04o, want 0600", got)
	}
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	requirePathMissing(t, pulseLedgerPath(target))
}

func TestMergeRequiredPulses_RecoversInterruptedSymlinkTransaction(t *testing.T) {
	dir := t.TempDir()
	managedDir := filepath.Join(dir, "managed")
	logicalDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(managedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(logicalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(managedDir, "pulses.json")
	host := filepath.Join(logicalDir, "pulses.json")
	baseRaw := validPulseConfig("existing")
	defaultsRaw := validPulseConfig("existing", "new")
	if err := os.WriteFile(target, baseRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "managed", "pulses.json"), host); err != nil {
		t.Fatal(err)
	}

	injected := errors.New("injected logical ledger write failure")
	ops, failed := failFirstPulseLedgerWrite(host, injected)
	_, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
		ops,
	)
	if !errors.Is(err, injected) || !*failed {
		t.Fatalf("first merge error = %v, failed=%v; want injected ledger failure", err, *failed)
	}
	requireNames(t, readPulseNames(t, target), "existing", "new")
	requirePathMissing(t, pulseLedgerPath(host))
	if _, err := os.Stat(pulseLedgerTransactionPath(host)); err != nil {
		t.Fatalf("logical pending transaction missing: %v", err)
	}

	retryOps := defaultPulseFileOps()
	realWrite := retryOps.atomicWrite
	registryRewrite := false
	retryOps.atomicWrite = func(path string, content []byte, mode os.FileMode, wantHash string) error {
		if path == target {
			registryRewrite = true
			return errors.New("unexpected registry rewrite during reconciliation")
		}
		return realWrite(path, content, mode, wantHash)
	}
	added, err := mergeRequiredPulsesWithOps(
		host,
		defaultsRaw,
		0o644,
		map[string]bool{"existing": true, "new": true},
		retryOps,
	)
	if err != nil {
		t.Fatalf("retry reconciliation: %v", err)
	}
	if registryRewrite {
		t.Fatal("retry rewrote an already-activated symlink target")
	}
	if len(added) != 0 {
		t.Fatalf("retry added duplicate pulses: %v", added)
	}

	linkInfo, err := os.Lstat(host)
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("logical registry is no longer a symlink: info=%v err=%v", linkInfo, err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("symlink target mode = %04o, want 0600", got)
	}
	requireNames(t, readPulseNames(t, target), "existing", "new")
	requireNames(t, readPulseLedgerNames(t, host), "existing", "new")
	requirePathMissing(t, pulseLedgerTransactionPath(host))
	requirePathMissing(t, pulseLedgerPath(target))
}

func TestPulseMerge_DanglingRegistrySymlinkFailsWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	host := filepath.Join(dir, "pulses.json")
	target := filepath.Join(dir, "missing.json")
	defaults := filepath.Join(dir, "defaults.json")
	if err := os.WriteFile(defaults, validPulseConfig("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), host); err != nil {
		t.Fatal(err)
	}

	for _, operation := range []struct {
		name string
		run  func() error
	}{
		{
			name: "preview",
			run: func() error {
				_, err := PendingPulseMerges(host, defaults, map[string]bool{"new": true})
				return err
			},
		},
		{
			name: "merge",
			run: func() error {
				_, err := MergeRequiredPulses(host, defaults, 0o644, map[string]bool{"new": true})
				return err
			},
		},
	} {
		t.Run(operation.name, func(t *testing.T) {
			err := operation.run()
			if err == nil || !strings.Contains(err.Error(), "resolve pulse registry symlink") {
				t.Fatalf("error = %v, want dangling-symlink refusal", err)
			}
		})
	}

	linkInfo, err := os.Lstat(host)
	if err != nil || linkInfo.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dangling link was replaced: info=%v err=%v", linkInfo, err)
	}
	requirePathMissing(t, target)
	requirePathMissing(t, pulseLedgerPath(host))
	requirePathMissing(t, pulseLedgerTransactionPath(host))
}
