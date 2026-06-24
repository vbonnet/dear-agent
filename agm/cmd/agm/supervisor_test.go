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
