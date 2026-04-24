package main

import (
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

func TestCheckSupervisorEnvRefusesAPIKey(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{
		"ANTHROPIC_API_KEY":        "sk-fake",
		"CLAUDE_CODE_OAUTH_TOKEN":  "oauth-token",
	}}
	err := checkSupervisorEnv(env, false)
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !errors.Is(err, errToSRefusal) {
		t.Errorf("expected errToSRefusal, got %v", err)
	}
}

func TestCheckSupervisorEnvRequiresOAuth(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{}}
	err := checkSupervisorEnv(env, false)
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !strings.Contains(err.Error(), "CLAUDE_CODE_OAUTH_TOKEN") {
		t.Errorf("error = %q, want mention of CLAUDE_CODE_OAUTH_TOKEN", err)
	}
}

func TestCheckSupervisorEnvSkipFlag(t *testing.T) {
	env := fakeSupervisorEnv{envs: map[string]string{}}
	if err := checkSupervisorEnv(env, true); err != nil {
		t.Errorf("--skip-oauth-check should bypass, got %v", err)
	}
}

func TestCheckSupervisorEnvAPIKeyWinsOverSkipFlag(t *testing.T) {
	// Even with skip-oauth-check, the API-key guard still applies: that's
	// the invariant we never want to bypass.
	env := fakeSupervisorEnv{envs: map[string]string{"ANTHROPIC_API_KEY": "sk-bad"}}
	err := checkSupervisorEnv(env, true)
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
	if err := checkSupervisorEnv(env, false); err != nil {
		t.Errorf("happy path errored: %v", err)
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
