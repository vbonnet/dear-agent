package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// The bug this guards: after the family died, the launchd job exited 2 every
// 30 minutes until launchd throttled it off the schedule entirely, so nothing
// refreshed the credentials even after the operator re-authenticated.
func TestCadenceExit_TokenFamilyDeathReportsSuccess(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr); got != exitOK {
		t.Errorf("cadenceExit(family dead) = %d, want %d so launchd keeps the schedule", got, exitOK)
	}
	if _, err := os.Stat(filepath.Join(dir, deathSentinelName)); err != nil {
		t.Errorf("expected a death sentinel to be written: %v", err)
	}
}

// The operator should be alerted once per episode, not every 30 minutes.
func TestCadenceExit_AlertsOncePerEpisode(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	var stderr bytes.Buffer

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr)
	first, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr)
	second, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	if string(first) != string(second) {
		t.Error("sentinel rewritten on the second tick — the operator would be re-alerted every 30 minutes")
	}
}

// A successful refresh must re-arm the alert so the NEXT death notifies again.
func TestCadenceExit_SuccessClearsSentinel(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	var stderr bytes.Buffer

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr)
	cadenceExit(exitOK, dir, deathSentinelName, &stderr)

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("sentinel survived a successful refresh — the next death would be silent")
	}
}

func TestClearCadenceSentinel(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(sentinel, []byte("alerted\n"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	if err := clearCadenceSentinel(dir, deathSentinelName); err != nil {
		t.Fatalf("clear cadence sentinel: %v", err)
	}
	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("sentinel survived explicit re-arm")
	}
	if err := clearCadenceSentinel(dir, deathSentinelName); err != nil {
		t.Fatalf("clearing an absent sentinel: %v", err)
	}
}

// Only the dead-family code is flattened; other failures still surface.
func TestCadenceExit_PassesThroughOtherFailures(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitNotPersisted, dir, deathSentinelName, &stderr); got != exitNotPersisted {
		t.Errorf("cadenceExit(not persisted) = %d, want %d", got, exitNotPersisted)
	}
	if got := cadenceExit(exitError, dir, deathSentinelName, &stderr); got != exitError {
		t.Errorf("cadenceExit(generic error) = %d, want %d", got, exitError)
	}
}
