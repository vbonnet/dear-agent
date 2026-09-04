package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The bug this guards: after the family died, the launchd job exited 2 every
// 30 minutes until launchd throttled it off the schedule entirely, so nothing
// refreshed the credentials even after the operator re-authenticated.
func TestCadenceExit_TokenFamilyDeathReportsSuccess(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr, defaultSentinelMaxAge); got != exitOK {
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

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr, defaultSentinelMaxAge)
	first, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr, defaultSentinelMaxAge)
	second, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	if string(first) != string(second) {
		t.Error("sentinel rewritten on the second tick — the operator would be re-alerted every 30 minutes")
	}
}

func TestNotifyCadenceOnce_StampsNonStandardFailurePath(t *testing.T) {
	dir := t.TempDir()
	notifyCadenceOnce(dir, deathSentinelName, "test title", "test message")

	sentinel := filepath.Join(dir, deathSentinelName)
	first, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	notifyCadenceOnce(dir, deathSentinelName, "test title", "test message")
	second, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel after second alert: %v", err)
	}
	if string(first) != string(second) {
		t.Error("non-standard cadence alert rewrote its sentinel on the second tick")
	}
}

// A successful refresh must re-arm the alert so the NEXT death notifies again.
func TestCadenceExit_SuccessClearsSentinel(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	var stderr bytes.Buffer

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, &stderr, defaultSentinelMaxAge)
	cadenceExit(exitOK, dir, deathSentinelName, &stderr, defaultSentinelMaxAge)

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

func TestCadenceStopPersistsBesideCredentialsAndRearmsExplicitly(t *testing.T) {
	credentials := writeCreds(t, "access", staleMs(), "refresh")
	if err := writeCadenceStop(credentials); err != nil {
		t.Fatal(err)
	}
	if stopped, err := cadenceStopped(credentials); err != nil || !stopped {
		t.Fatalf("cadence stop = (%v, %v), want (true, nil)", stopped, err)
	}
	if err := clearCadenceStop(credentials); err != nil {
		t.Fatal(err)
	}
	if stopped, err := cadenceStopped(credentials); err != nil || stopped {
		t.Fatalf("cadence stop after clear = (%v, %v), want (false, nil)", stopped, err)
	}
}

// Only the dead-family code is flattened; other failures still surface.
func TestCadenceExit_PassesThroughOtherFailures(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitNotPersisted, dir, deathSentinelName, &stderr, defaultSentinelMaxAge); got != exitNotPersisted {
		t.Errorf("cadenceExit(not persisted) = %d, want %d", got, exitNotPersisted)
	}
	if got := cadenceExit(exitError, dir, deathSentinelName, &stderr, defaultSentinelMaxAge); got != exitError {
		t.Errorf("cadenceExit(generic error) = %d, want %d", got, exitError)
	}
}

func TestPruneCadenceSentinels_RemovesStaleKeepsFresh(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	stale1 := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(stale1, []byte("stale1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale1, now.Add(-25*time.Hour), now.Add(-25*time.Hour))

	stale2 := filepath.Join(dir, deathSentinelName+"-0123456789abcdef")
	if err := os.WriteFile(stale2, []byte("stale2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale2, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	fresh := filepath.Join(dir, deathSentinelName+"-fedcba9876543210")
	if err := os.WriteFile(fresh, []byte("fresh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(fresh, now.Add(-1*time.Hour), now.Add(-1*time.Hour))

	unrelated := filepath.Join(dir, "refresh-token-quarantine.json")
	if err := os.WriteFile(unrelated, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(unrelated, now.Add(-100*time.Hour), now.Add(-100*time.Hour))

	pruned, err := pruneCadenceSentinels(dir, 24*time.Hour)
	if err != nil {
		t.Fatalf("pruneCadenceSentinels failed: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	if _, err := os.Stat(stale1); !os.IsNotExist(err) {
		t.Errorf("stale sentinel %s still exists", stale1)
	}
	if _, err := os.Stat(stale2); !os.IsNotExist(err) {
		t.Errorf("stale sentinel %s still exists", stale2)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh sentinel %s was unexpectedly removed: %v", fresh, err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated file %s was unexpectedly removed: %v", unrelated, err)
	}
}

func TestPruneCadenceSentinels_DisabledWhenZeroOrNegative(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	stale := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(stale, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	pruned, err := pruneCadenceSentinels(dir, 0)
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) when disabled, got (%d, %v)", pruned, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("sentinel should not be pruned when maxAge=0: %v", err)
	}

	pruned, err = pruneCadenceSentinels(dir, -10*time.Minute)
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) when negative, got (%d, %v)", pruned, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("sentinel should not be pruned when maxAge<0: %v", err)
	}
}

func TestPruneCadenceSentinels_NonexistentDir(t *testing.T) {
	pruned, err := pruneCadenceSentinels("/nonexistent/state/dir", 24*time.Hour)
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) for nonexistent dir, got (%d, %v)", pruned, err)
	}
}

func TestCadenceExit_PrunesStaleSentinels(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	stale := filepath.Join(dir, deathSentinelName+"-stale1234567890")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	var stderr bytes.Buffer
	cadenceExit(exitOK, dir, deathSentinelName, &stderr, 24*time.Hour)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale sentinel survived cadenceExit")
	}
}
