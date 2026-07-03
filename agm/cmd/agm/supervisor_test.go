package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// fakeSupervisorEnv is a stub supervisorEnv for unit tests.
type fakeSupervisorEnv struct {
	envs  map[string]string
	paths map[string]string
}

func (f fakeSupervisorEnv) Getenv(key string) string { return f.envs[key] }
func (f fakeSupervisorEnv) LookPath(bin string) (string, error) {
	if p, ok := f.paths[bin]; ok {
		return p, nil
	}
	return "", fmt.Errorf("fake: not on PATH: %s", bin)
}

// noCredsPath returns a path under the test's temp dir that does not exist, so
// the OAuth presence guard's credentials-file lookup is isolated from the
// host's real ~/.claude/.credentials.json.
func noCredsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "no-credentials.json")
}

// writeFreshCreds writes a credentials.json with a non-expired token and
// returns its path.
func writeFreshCreds(t *testing.T, token string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	expires := time.Now().Add(time.Hour).UnixMilli()
	body := fmt.Sprintf(`{"claudeAiOauth":{"accessToken":%q,"expiresAt":%d}}`, token, expires)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

func TestCheckSupervisorEnvRefusesAPIKey(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{
		"ANTHROPIC_API_KEY":       "sk-fake",
		"CLAUDE_CODE_OAUTH_TOKEN": "oauth-token",
	}}
	err := checkSupervisorEnv(env, false, noCredsPath(t))
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !errors.Is(err, errToSRefusal) {
		t.Errorf("expected errToSRefusal, got %v", err)
	}
}

func TestCheckSupervisorEnvRequiresOAuth(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{}}
	err := checkSupervisorEnv(env, false, noCredsPath(t))
	if err == nil {
		t.Fatal("expected refusal, got nil")
		return
	}
	if !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Errorf("error = %q, want mention of CLAUDE_CODE_OAUTH_TOKEN", err)
	}
}

func TestCheckSupervisorEnvSkipFlag(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{}}
	if err := checkSupervisorEnv(env, true, noCredsPath(t)); err != nil {
		t.Errorf("--skip-oauth-check should bypass, got %v", err)
	}
}

func TestCheckSupervisorEnvAPIKeyWinsOverSkipFlag(t *testing.T) {
	// Even with skip-oauth-check, the API-key guard still applies: that's
	// the invariant we never want to bypass.
	env := fakeSupervisorEnv{envs: map[string]string{"ANTHROPIC_API_KEY": "sk-bad"}}
	err := checkSupervisorEnv(env, true, noCredsPath(t))
	if err == nil {
		t.Fatal("API-key guard must not be bypassed by --skip-oauth-check")
	}
	if !errors.Is(err, errToSRefusal) {
		t.Errorf("expected errToSRefusal, got %v", err)
	}
}

func TestCheckSupervisorEnvOK(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{
		"CLAUDE_CODE_OAUTH_TOKEN": "oauth-token",
	}}
	if err := checkSupervisorEnv(env, false, noCredsPath(t)); err != nil {
		t.Errorf("happy path errored: %v", err)
	}
}

func TestCheckSupervisorEnvAcceptsFileToken(t *testing.T) {
	// ce-dzhz: a supervisor with a valid credentials file but no
	// CLAUDE_CODE_OAUTH_TOKEN env var must be allowed to start — the env var
	// goes stale after the file auto-refreshes, so file-only auth is valid.
	env := fakeSupervisorEnv{envs: map[string]string{}}
	if err := checkSupervisorEnv(env, false, writeFreshCreds(t, "file-token")); err != nil {
		t.Errorf("file-based OAuth should satisfy the guard, got %v", err)
	}
}

func TestHeartbeatRoundTrip(t *testing.T) {
	// Redirect HOME so supervisor state lands in a test-scoped dir.
	home := t.TempDir()
	t.Setenv("HOME", home)

	rec := heartbeatRecord{
		ID:          "test-sup",
		PrimaryFor:  "peer-a",
		TertiaryFor: "peer-b",
		LastBeatUTC: time.Now().UTC().Round(time.Millisecond),
		PID:         12345,
	}
	path, err := heartbeatPath(rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeHeartbeatRecord(path, rec); err != nil {
		t.Fatalf("writeHeartbeatRecord: %v", err)
	}
	// Directory structure.
	wantDir := filepath.Join(home, ".agm", "supervisors", "test-sup")
	if _, err := os.Stat(wantDir); err != nil {
		t.Errorf("expected state dir %s to exist: %v", wantDir, err)
	}

	got, err := readHeartbeatRecord("test-sup")
	if err != nil {
		t.Fatalf("readHeartbeatRecord: %v", err)
	}
	if got == nil {
		t.Fatal("readHeartbeatRecord returned nil for just-written record")
		return
	}
	if got.ID != rec.ID || got.PrimaryFor != rec.PrimaryFor ||
		got.TertiaryFor != rec.TertiaryFor || got.PID != rec.PID {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, rec)
	}
}

func TestReadHeartbeatMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	got, err := readHeartbeatRecord("never-heartbeated")
	if err != nil {
		t.Errorf("readHeartbeatRecord(missing): %v", err)
	}
	if got != nil {
		t.Errorf("readHeartbeatRecord(missing) = %+v, want nil", got)
	}
}

func TestScrubAPIKey(t *testing.T) {
	before := []string{
		"PATH=/bin",
		"ANTHROPIC_API_KEY=sk-leak",
		"OTHER=value",
		"ANTHROPIC_API_KEY_SUFFIX=ok", // not the exact prefix; should remain
	}
	after := scrubAPIKey(before)

	if slices.Contains(after, "ANTHROPIC_API_KEY=sk-leak") {
		t.Error("scrubAPIKey failed to remove the canonical env var")
	}
	if !slices.Contains(after, "PATH=/bin") {
		t.Error("scrubAPIKey dropped an unrelated env var")
	}
	if !slices.Contains(after, "OTHER=value") {
		t.Error("scrubAPIKey dropped an unrelated env var")
	}
	// Conservative match is intentional: ANTHROPIC_API_KEY_SUFFIX is a
	// different variable and should survive.
	if !slices.Contains(after, "ANTHROPIC_API_KEY_SUFFIX=ok") {
		t.Error("scrubAPIKey incorrectly removed a prefix-matching but distinct var")
	}
}

func TestSyncVroomHeartbeatFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "vroom-hb")
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	if err := syncVroomHeartbeatFile(dir, "meta-o", ts); err != nil {
		t.Fatalf("syncVroomHeartbeatFile: %v", err)
	}

	// Directory must have been created.
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("dir not created: %v", err)
	}

	// File must exist with correct content.
	data, err := os.ReadFile(filepath.Join(dir, "meta-o.json"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	var got vroomHeartbeatFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Role != "meta-orchestrator" {
		t.Errorf("role = %q, want %q", got.Role, "meta-orchestrator")
	}
	if got.ISO != "2026-06-01T12:00:00Z" {
		t.Errorf("iso = %q, want %q", got.ISO, "2026-06-01T12:00:00Z")
	}
	wantTS := float64(ts.UnixMilli()) / 1e3
	if got.TS != wantTS {
		t.Errorf("ts = %v, want %v", got.TS, wantTS)
	}
}

func TestSyncHeartbeatFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Write internal heartbeat records for two supervisors.
	ts1 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	for id, ts := range map[string]time.Time{"meta-o": ts1, "orch": ts2} {
		path, err := heartbeatPath(id)
		if err != nil {
			t.Fatalf("heartbeatPath(%s): %v", id, err)
		}
		if err := writeHeartbeatRecord(path, heartbeatRecord{
			ID:          id,
			LastBeatUTC: ts,
		}); err != nil {
			t.Fatalf("writeHeartbeatRecord(%s): %v", id, err)
		}
	}

	// Write a supervisor dir with no heartbeat file — should be skipped.
	missingDir := filepath.Join(home, ".agm", "supervisors", "never-beat")
	if err := os.MkdirAll(missingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	vroomDir := filepath.Join(home, "vroom-hb")
	if err := SyncHeartbeatFiles(vroomDir); err != nil {
		t.Fatalf("SyncHeartbeatFiles: %v", err)
	}

	// Verify meta-o and orch files were written.
	for id, wantRole := range map[string]string{
		"meta-o": "meta-orchestrator",
		"orch":   "orchestrator",
	} {
		data, err := os.ReadFile(filepath.Join(vroomDir, id+".json"))
		if err != nil {
			t.Fatalf("read %s.json: %v", id, err)
		}
		var got vroomHeartbeatFile
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", id, err)
		}
		if got.Role != wantRole {
			t.Errorf("%s: role = %q, want %q", id, got.Role, wantRole)
		}
	}

	// never-beat supervisor must NOT have a flat file.
	if _, err := os.Stat(filepath.Join(vroomDir, "never-beat.json")); !errors.Is(err, os.ErrNotExist) {
		t.Error("never-beat.json should not have been written")
	}
}

func TestSyncHeartbeatFilesNoSupervisors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	vroomDir := filepath.Join(home, "vroom-hb")
	// Must not error when ~/.agm/supervisors doesn't exist.
	if err := SyncHeartbeatFiles(vroomDir); err != nil {
		t.Fatalf("SyncHeartbeatFiles (no supervisors dir): %v", err)
	}
}

func TestSupervisorRole(t *testing.T) {
	cases := []struct{ id, want string }{
		{"meta-o", "meta-orchestrator"},
		{"orch", "orchestrator"},
		{"overseer", "overseer"},
		{"custom", "custom"},
	}
	for _, c := range cases {
		if got := supervisorRole(c.id); got != c.want {
			t.Errorf("supervisorRole(%q) = %q, want %q", c.id, got, c.want)
		}
	}
}

// row is a small builder for annotateMeshRecovery test cases.
func row(id string, stale bool, primaryFor, tertiaryFor string) supervisorRow {
	return supervisorRow{
		ID:    id,
		Stale: stale,
		Record: &heartbeatRecord{
			ID:          id,
			PrimaryFor:  primaryFor,
			TertiaryFor: tertiaryFor,
		},
	}
}

func findRow(rows []supervisorRow, id string) supervisorRow {
	for _, r := range rows {
		if r.ID == id {
			return r
		}
	}
	panic(fmt.Sprintf("supervisor row %q not found", id))
}

// TestAnnotateMeshRecoveryBothStale is the ce-2qbx incident: meta-o is
// primary-for orch and orch is primary-for meta-o, and both are stale. Neither
// can re-drive the other, so both are quorum-lost and the mesh cannot self-heal.
func TestAnnotateMeshRecoveryBothStale(t *testing.T) {
	rows := []supervisorRow{
		row("vroom-meta-orchestrator", true, "vroom-orchestrator", ""),
		row("vroom-orchestrator", true, "vroom-meta-orchestrator", ""),
	}
	quorumLost := annotateMeshRecovery(rows)
	if !quorumLost {
		t.Fatal("expected quorumLost=true when a stale pair are each other's only recoverer")
	}
	for _, id := range []string{"vroom-meta-orchestrator", "vroom-orchestrator"} {
		r := findRow(rows, id)
		if r.Recoverable {
			t.Errorf("%s: Recoverable=true, want false", id)
		}
		if !r.QuorumLost {
			t.Errorf("%s: QuorumLost=false, want true", id)
		}
	}
}

// TestAnnotateMeshRecoveryPairStaleOverseerLive: the same stale pair, but a
// fresh overseer is tertiary-for both. The pair is recoverable (overseer can
// re-drive them) and NOT quorum-lost — the mesh is expected to self-heal.
func TestAnnotateMeshRecoveryPairStaleOverseerLive(t *testing.T) {
	rows := []supervisorRow{
		row("vroom-meta-orchestrator", true, "vroom-orchestrator", ""),
		row("vroom-orchestrator", true, "vroom-meta-orchestrator", ""),
		{ID: "vroom-overseer", Stale: false, Record: &heartbeatRecord{
			ID:          "vroom-overseer",
			TertiaryFor: "vroom-meta-orchestrator",
		}},
	}
	// overseer is tertiary-for meta-o only; make it tertiary-for orch too via a
	// second record field is not possible, so cover orch through primary-for.
	rows[2].Record.PrimaryFor = "vroom-orchestrator"

	quorumLost := annotateMeshRecovery(rows)
	if quorumLost {
		t.Fatal("expected quorumLost=false when a live overseer can recover the stale pair")
	}
	for _, id := range []string{"vroom-meta-orchestrator", "vroom-orchestrator"} {
		r := findRow(rows, id)
		if !r.Recoverable {
			t.Errorf("%s: Recoverable=false, want true (overseer is live)", id)
		}
		if r.QuorumLost {
			t.Errorf("%s: QuorumLost=true, want false", id)
		}
	}
}

// TestAnnotateMeshRecoverySingleStaleLiveRecoverer: one stale supervisor whose
// sole recoverer is fresh — recoverable, no quorum loss.
func TestAnnotateMeshRecoverySingleStaleLiveRecoverer(t *testing.T) {
	rows := []supervisorRow{
		row("A", true, "", ""),
		row("B", false, "A", ""), // B (fresh) is primary-for A
	}
	if annotateMeshRecovery(rows) {
		t.Fatal("expected quorumLost=false")
	}
	a := findRow(rows, "A")
	if !a.Recoverable || a.QuorumLost {
		t.Errorf("A: Recoverable=%v QuorumLost=%v, want true/false", a.Recoverable, a.QuorumLost)
	}
	if !slices.Contains(a.Recoverers, "B") {
		t.Errorf("A.Recoverers = %v, want to contain B", a.Recoverers)
	}
}

// TestAnnotateMeshRecoveryAllFresh: nobody stale → no quorum loss, everyone
// trivially recoverable.
func TestAnnotateMeshRecoveryAllFresh(t *testing.T) {
	rows := []supervisorRow{
		row("A", false, "B", ""),
		row("B", false, "A", ""),
	}
	if annotateMeshRecovery(rows) {
		t.Fatal("expected quorumLost=false when all fresh")
	}
	for _, id := range []string{"A", "B"} {
		if r := findRow(rows, id); !r.Recoverable {
			t.Errorf("%s: Recoverable=false, want true", id)
		}
	}
}

// TestAnnotateMeshRecoveryLoneStaleNoRecoverer: a stale supervisor nobody lists
// as primary/tertiary has no recoverer at all → quorum-lost.
func TestAnnotateMeshRecoveryLoneStaleNoRecoverer(t *testing.T) {
	rows := []supervisorRow{row("solo", true, "", "")}
	if !annotateMeshRecovery(rows) {
		t.Fatal("expected quorumLost=true for a stale supervisor with no recoverers")
	}
	r := findRow(rows, "solo")
	if r.Recoverable || !r.QuorumLost {
		t.Errorf("solo: Recoverable=%v QuorumLost=%v, want false/true", r.Recoverable, r.QuorumLost)
	}
	if len(r.Recoverers) != 0 {
		t.Errorf("solo.Recoverers = %v, want empty", r.Recoverers)
	}
}

// TestEmitSupervisorStatusTableRecoveryColumn checks the human table renders
// the RECOVERY column with QUORUM-LOST for an unrecoverable stale supervisor.
func TestEmitSupervisorStatusTableRecoveryColumn(t *testing.T) {
	rows := []supervisorRow{
		row("vroom-meta-orchestrator", true, "vroom-orchestrator", ""),
		row("vroom-orchestrator", true, "vroom-meta-orchestrator", ""),
	}
	annotateMeshRecovery(rows)

	prev := supervisorStatusJSON
	supervisorStatusJSON = false
	defer func() { supervisorStatusJSON = prev }()

	cmd := &cobra.Command{}
	var out strings.Builder
	cmd.SetOut(&out)
	if err := emitSupervisorStatus(cmd, rows); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "RECOVERY") {
		t.Errorf("table header missing RECOVERY column:\n%s", got)
	}
	if !strings.Contains(got, "QUORUM-LOST") {
		t.Errorf("table missing QUORUM-LOST for the stale pair:\n%s", got)
	}
}

// TestEmitSupervisorStatusJSONFields checks the JSON payload carries the new
// mesh-recovery fields so an external watchdog can act on them.
func TestEmitSupervisorStatusJSONFields(t *testing.T) {
	rows := []supervisorRow{
		row("A", true, "", ""),
		row("B", false, "A", ""),
	}
	annotateMeshRecovery(rows)

	prev := supervisorStatusJSON
	supervisorStatusJSON = true
	defer func() { supervisorStatusJSON = prev }()

	cmd := &cobra.Command{}
	var out strings.Builder
	cmd.SetOut(&out)
	if err := emitSupervisorStatus(cmd, rows); err != nil {
		t.Fatal(err)
	}
	var decoded []supervisorRow
	if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
		t.Fatalf("unmarshal status JSON: %v\n%s", err, out.String())
	}
	a := findRow(decoded, "A")
	if !a.Recoverable {
		t.Errorf("A.recoverable = false, want true (B is live)")
	}
	if !slices.Contains(a.Recoverers, "B") {
		t.Errorf("A.recoverers = %v, want to contain B", a.Recoverers)
	}
	// Confirm the field name is present in the raw JSON (contract for callers).
	if !strings.Contains(out.String(), "\"quorum_lost\"") {
		t.Errorf("JSON missing quorum_lost field:\n%s", out.String())
	}
}

// TestAnnotateMeshRecoveryNilRecordPeerIsNoRecoverer: a fresh peer with no
// heartbeat record (never wrote primary_for/tertiary_for) contributes no
// recoverer edge, so a stale supervisor it does not cover stays quorum-lost.
func TestAnnotateMeshRecoveryNilRecordPeerIsNoRecoverer(t *testing.T) {
	rows := []supervisorRow{
		row("A", true, "", ""),
		{ID: "B", Stale: false, Record: nil}, // fresh but declares no edges
	}
	if !annotateMeshRecovery(rows) {
		t.Fatal("expected quorumLost=true: B declares no primary/tertiary edge to A")
	}
	if r := findRow(rows, "A"); r.Recoverable {
		t.Error("A should be unrecoverable: no peer declares itself its recoverer")
	}
}
