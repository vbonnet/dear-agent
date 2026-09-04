package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// The bug this guards: after the family died, the launchd job exited 2 every
// 30 minutes until launchd throttled it off the schedule entirely, so nothing
// refreshed the credentials even after the operator re-authenticated.
func TestCadenceExit_TokenFamilyDeathReportsSuccess(t *testing.T) {
	dir := t.TempDir()
	var stderr bytes.Buffer

	if got := cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge); got != exitOK {
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

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge)
	first, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge)
	second, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	if string(first) != string(second) {
		t.Error("sentinel rewritten on the second tick: the operator would be re-alerted every 30 minutes")
	}
}

func TestNotifyCadenceOnce_StampsNonStandardFailurePath(t *testing.T) {
	dir := t.TempDir()
	notifyCadenceOnce(dir, deathSentinelName, "", "", "", "dead", "test title", "test message")

	sentinel := filepath.Join(dir, deathSentinelName)
	first, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}

	notifyCadenceOnce(dir, deathSentinelName, "", "", "", "dead", "test title", "test message")
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

	cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge)
	cadenceExit(exitOK, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge)

	if _, err := os.Stat(sentinel); !os.IsNotExist(err) {
		t.Error("sentinel survived a successful refresh: the next death would be silent")
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

	if got := cadenceExit(exitNotPersisted, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge); got != exitNotPersisted {
		t.Errorf("cadenceExit(not persisted) = %d, want %d", got, exitNotPersisted)
	}
	if got := cadenceExit(exitError, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge); got != exitError {
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

	pruned, err := pruneCadenceSentinels(dir, 24*time.Hour, "")
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

func TestPruneCadenceSentinels_PreservesKeepSentinel(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	activeSentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(activeSentinel, []byte("active\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(activeSentinel, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	staleSentinel := filepath.Join(dir, deathSentinelName+"-0123456789abcdef")
	if err := os.WriteFile(staleSentinel, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(staleSentinel, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	pruned, err := pruneCadenceSentinels(dir, 24*time.Hour, deathSentinelName)
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}
	if _, err := os.Stat(activeSentinel); err != nil {
		t.Errorf("active sentinel matching keepSentinel was pruned: %v", err)
	}
	if _, err := os.Stat(staleSentinel); !os.IsNotExist(err) {
		t.Errorf("stale sentinel was not pruned")
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

	pruned, err := pruneCadenceSentinels(dir, 0, "")
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) when disabled, got (%d, %v)", pruned, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("sentinel should not be pruned when maxAge=0: %v", err)
	}

	pruned, err = pruneCadenceSentinels(dir, -10*time.Minute, "")
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) when negative, got (%d, %v)", pruned, err)
	}
	if _, err := os.Stat(stale); err != nil {
		t.Errorf("sentinel should not be pruned when maxAge<0: %v", err)
	}
}

func TestPruneCadenceSentinels_NonexistentDir(t *testing.T) {
	pruned, err := pruneCadenceSentinels("/nonexistent/state/dir", 24*time.Hour, "")
	if err != nil || pruned != 0 {
		t.Fatalf("expected (0, nil) for nonexistent dir, got (%d, %v)", pruned, err)
	}
}

func TestPruneCadenceSentinels_IgnoresInvalidSentinelNames(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	// Valid sentinel that is stale and should be pruned.
	validStale := filepath.Join(dir, deathSentinelName+"-0123456789abcdef")
	if err := os.WriteFile(validStale, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(validStale, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	// Invalid sentinel names that must never be pruned as refresher state.
	invalidNames := []string{
		deathSentinelName + "-notes",
		deathSentinelName + "-12345",
		deathSentinelName + "-nothexcharacters",
		deathSentinelName + "-",
		deathSentinelName + "-0123456789abcdef0",
	}
	for _, name := range invalidNames {
		target := filepath.Join(dir, name)
		if err := os.WriteFile(target, []byte("preserve\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_ = os.Chtimes(target, now.Add(-48*time.Hour), now.Add(-48*time.Hour))
	}

	pruned, err := pruneCadenceSentinels(dir, 24*time.Hour, "")
	if err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}
	if _, err := os.Stat(validStale); !os.IsNotExist(err) {
		t.Errorf("valid stale sentinel was not pruned")
	}
	for _, name := range invalidNames {
		target := filepath.Join(dir, name)
		if _, err := os.Stat(target); err != nil {
			t.Errorf("invalid sentinel name %s was incorrectly pruned: %v", name, err)
		}
	}
}

func TestCadenceExit_PrunesStaleSentinels(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	stale := filepath.Join(dir, deathSentinelName+"-0123456789abcdef")
	if err := os.WriteFile(stale, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(stale, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	var stderr bytes.Buffer
	cadenceExit(exitOK, dir, deathSentinelName, "", "", "", &stderr, 24*time.Hour)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale sentinel survived cadenceExit")
	}
}

func TestCadenceExit_PreservesActiveSentinelSpanningMaxAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	activeSentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(activeSentinel, []byte("active\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(activeSentinel, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	var stderr bytes.Buffer
	// During an ongoing failure episode, cadenceExit must not delete the sentinel
	// being handled, which would re-alert the operator every maxAge.
	code := cadenceExit(exitTokenFamilyDead, dir, deathSentinelName, "", "", "", &stderr, 24*time.Hour)
	if code != exitOK {
		t.Errorf("cadenceExit = %d, want %d", code, exitOK)
	}
	if _, err := os.Stat(activeSentinel); err != nil {
		t.Errorf("active sentinel was pruned: %v", err)
	}
	// Sentinel content was not replaced (would indicate re-alerting).
	content, err := os.ReadFile(activeSentinel)
	if err != nil || string(content) != "active\n" {
		t.Errorf("sentinel rewritten during unresolved episode: %q", string(content))
	}
}

func writeQuarantineFile(path, token string) error {
	rec := map[string]any{
		"refresh_token_fp": auth.RefreshTokenFingerprint(token),
		"quarantined_at":   "2026-09-01T00:00:00Z",
		"reason":           "test",
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func TestCadenceExit_PreservesSentinelsForAllActiveJobs(t *testing.T) {
	stateDir := t.TempDir()
	quarDir := t.TempDir()
	now := time.Now()

	// Setup Job 1 quarantine and sentinel.
	c1 := writeCreds(t, "access1", freshMs(), "refresh1")
	fp1, _ := credentialsFingerprint(c1)
	q1 := filepath.Join(quarDir, "job1-quarantine.json")
	if err := writeQuarantineFile(q1, "refresh1"); err != nil {
		t.Fatal(err)
	}
	s1Name := cadenceSentinelName(q1, c1)
	s1 := filepath.Join(stateDir, s1Name)
	if err := os.WriteFile(s1, []byte("2026-09-01T00:00:00Z\n"+q1+"\n"+c1+"\n"+fp1+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(s1, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	// Setup Job 2 quarantine and sentinel.
	c2 := writeCreds(t, "access2", freshMs(), "refresh2")
	fp2, _ := credentialsFingerprint(c2)
	q2 := filepath.Join(quarDir, "job2-quarantine.json")
	if err := writeQuarantineFile(q2, "refresh2"); err != nil {
		t.Fatal(err)
	}
	s2Name := cadenceSentinelName(q2, c2)
	s2 := filepath.Join(stateDir, s2Name)
	if err := os.WriteFile(s2, []byte("2026-09-01T00:00:00Z\n"+q2+"\n"+c2+"\n"+fp2+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(s2, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	// Job 1 runs cadenceExit while both failures remain unresolved.
	var stderr bytes.Buffer
	code := cadenceExit(exitQuarantined, stateDir, s1Name, q1, c1, fp1, &stderr, 24*time.Hour)
	if code != exitOK {
		t.Errorf("cadenceExit = %d, want %d", code, exitOK)
	}

	// S1 must be preserved.
	if _, err := os.Stat(s1); err != nil {
		t.Errorf("Job 1 sentinel was pruned: %v", err)
	}
	// S2 must ALSO be preserved because Job 2's quarantine remains active on disk.
	if _, err := os.Stat(s2); err != nil {
		t.Errorf("Job 2 sentinel was pruned despite active quarantine: %v", err)
	}

	// Now simulate Job 2's token rotating on disk so its quarantine marker becomes stale (inactive).
	_ = writeCredsFile(c2, "access2-rotated", freshMs(), "refresh2-rotated")

	// Job 1 runs again: now that Job 2's quarantine is inactive/stale, S2 should be pruned.
	code = cadenceExit(exitQuarantined, stateDir, s1Name, q1, c1, fp1, &stderr, 24*time.Hour)
	if code != exitOK {
		t.Errorf("cadenceExit = %d, want %d", code, exitOK)
	}

	if _, err := os.Stat(s1); err != nil {
		t.Errorf("Job 1 sentinel was pruned: %v", err)
	}
	if _, err := os.Stat(s2); !os.IsNotExist(err) {
		t.Errorf("Job 2 sentinel survived after its quarantine became stale/inactive")
	}
}

func TestNotifyCadenceOnce_RefreshesSentinelModTimeOnActiveEpisode(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	sentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(sentinel, []byte("initial\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := now.Add(-10 * time.Hour)
	_ = os.Chtimes(sentinel, oldTime, oldTime)

	notifyCadenceOnce(dir, deathSentinelName, "", "", "", "dead", "test title", "test message")

	info, err := os.Stat(sentinel)
	if err != nil {
		t.Fatalf("stat sentinel: %v", err)
	}
	if !info.ModTime().After(oldTime.Add(9 * time.Hour)) {
		t.Errorf("sentinel ModTime was not refreshed: %v (wanted close to %v)", info.ModTime(), now)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "initial\n" {
		t.Errorf("sentinel content was modified: %q", string(content))
	}
}

func TestPruneCadenceSentinels_SkipsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	oldTime := time.Now().Add(-48 * time.Hour)

	// Subdirectory matching valid sentinel name shape.
	subDirSentinel := filepath.Join(dir, "token-family-dead-1111111111111111")
	if err := os.Mkdir(subDirSentinel, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(subDirSentinel, oldTime, oldTime)

	// Symlink matching valid sentinel name shape.
	symlinkSentinel := filepath.Join(dir, "token-family-dead-2222222222222222")
	targetFile := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(targetFile, []byte("regular\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(targetFile, symlinkSentinel); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// FIFO matching valid sentinel name shape.
	fifoSentinel := filepath.Join(dir, "token-family-dead-3333333333333333")
	if err := syscall.Mkfifo(fifoSentinel, 0o600); err == nil {
		defer os.Remove(fifoSentinel)
	}

	pruned, err := pruneCadenceSentinels(dir, 24*time.Hour, "")
	if err != nil {
		t.Fatalf("pruneCadenceSentinels failed: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0 non-regular entries pruned", pruned)
	}

	if _, err := os.Stat(subDirSentinel); err != nil {
		t.Errorf("subdirectory sentinel was improperly removed: %v", err)
	}
	if _, err := os.Lstat(symlinkSentinel); err != nil {
		t.Errorf("symlink sentinel was improperly removed: %v", err)
	}
}

func TestPruneCadenceSentinels_SkipsTempAndSystemRoot(t *testing.T) {
	pruned, err := pruneCadenceSentinels(os.TempDir(), 24*time.Hour, "")
	if err != nil {
		t.Fatalf("pruneCadenceSentinels on TempDir returned error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruneCadenceSentinels on TempDir pruned %d files, want 0", pruned)
	}

	pruned, err = pruneCadenceSentinels("/", 24*time.Hour, "")
	if err != nil {
		t.Fatalf("pruneCadenceSentinels on root returned error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruneCadenceSentinels on root pruned %d files, want 0", pruned)
	}
}

func TestNotifyCadenceOnce_NewEpisodeAlertsWhenTokenFingerprintRotates(t *testing.T) {
	stateDir := t.TempDir()
	sentinel := filepath.Join(stateDir, deathSentinelName)
	credsPath := credsWithRefreshToken(t, "rt-first")
	fpFirst, _ := credentialsFingerprint(credsPath)

	notifyCadenceOnce(stateDir, deathSentinelName, "", credsPath, fpFirst, "dead", "title1", "msg1")

	data1, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel after first notify: %v", err)
	}
	if !strings.Contains(string(data1), fpFirst) {
		t.Fatalf("sentinel missing initial fingerprint %q: %s", fpFirst, string(data1))
	}

	// Update credentials with rotated token representing a new episode.
	credsPath = credsWithRefreshToken(t, "rt-second")
	fpSecond, _ := credentialsFingerprint(credsPath)
	if fpFirst == fpSecond {
		t.Fatal("expected fingerprints to differ for different tokens")
	}

	notifyCadenceOnce(stateDir, deathSentinelName, "", credsPath, fpSecond, "dead", "title2", "msg2")

	data2, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel after second notify: %v", err)
	}
	if !strings.Contains(string(data2), fpSecond) {
		t.Fatalf("sentinel not updated with new episode fingerprint %q: %s", fpSecond, string(data2))
	}
}

func TestIsActiveSentinel_ExpiredEpisodeWithRotatedFingerprintReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "token-family-dead-0123456789abcdef")

	credsPath := credsWithRefreshToken(t, "rt-current")
	fpOld := "deadbeef0000"

	// Write sentinel recording a prior episode fingerprint and active stop marker.
	if err := writeCadenceStop(credsPath); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = clearCadenceStop(credsPath) }()

	stamp := time.Now().UTC().Format(time.RFC3339)
	lines := []string{stamp, "", credsPath, fpOld}
	if err := os.WriteFile(sentinel, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if isActiveSentinel(sentinel) {
		t.Error("isActiveSentinel = true for sentinel with rotated/mismatched fingerprint, want false")
	}

	// Now rewrite sentinel recording current fingerprint.
	fpCurrent, _ := credentialsFingerprint(credsPath)
	lines = []string{stamp, "", credsPath, fpCurrent}
	if err := os.WriteFile(sentinel, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !isActiveSentinel(sentinel) {
		t.Error("isActiveSentinel = false for sentinel with current matching fingerprint, want true")
	}
}

func TestDefaultStateDir_ScopedPerUserFallback(t *testing.T) {
	t.Setenv("HOME", "")
	dir := defaultStateDir()
	expectedSubdir := fmt.Sprintf("dear-agent-%d", os.Getuid())
	if !strings.Contains(dir, expectedSubdir) {
		t.Errorf("defaultStateDir fallback %q does not contain expected UID scoped subdir %q", dir, expectedSubdir)
	}
}

func TestCadenceExit_UnrelatedErrorPrunesStaleExpiredSentinel(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	var stderr bytes.Buffer

	staleSentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(staleSentinel, []byte("2020-01-01T00:00:00Z\n\n/nonexistent\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(staleSentinel, now.Add(-48*time.Hour), now.Add(-48*time.Hour))

	got := cadenceExit(exitError, dir, deathSentinelName, "", "", "", &stderr, defaultSentinelMaxAge)
	if got != exitError {
		t.Fatalf("cadenceExit = %d, want %d", got, exitError)
	}

	if _, err := os.Stat(staleSentinel); !os.IsNotExist(err) {
		t.Errorf("stale sentinel %s was not pruned on unrelated error: %v", staleSentinel, err)
	}
}

func TestNotifyCadenceOnce_SkipsNonRegularFile(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "cadence-fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("cannot make FIFO on this platform: %v", err)
	}

	notifyCadenceOnce(dir, "cadence-fifo", "", "", "", "dead", "title", "message")

	info, err := os.Lstat(fifoPath)
	if err != nil {
		t.Fatalf("stat after notifyCadenceOnce: %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Errorf("sentinel %s is not regular file: mode %v", fifoPath, info.Mode())
	}
}

func TestSentinelMatchesEpisode_EmptyRecordedFPReturnsFalse(t *testing.T) {
	credsPath := credsWithRefreshToken(t, "rt-legacy")
	if sentinelMatchesEpisode(sentinelRecord{}, credsPath, "") {
		t.Error("sentinelMatchesEpisode with empty recorded fingerprint should return false to trigger upgrade/re-alert")
	}
}

func TestCadenceSentinelName_ScopesDisabledQuarantineToCredentials(t *testing.T) {
	c1 := "/path/to/job1/creds.json"
	c2 := "/path/to/job2/creds.json"
	s1 := cadenceSentinelName("", c1)
	s2 := cadenceSentinelName("", c2)
	if s1 == s2 {
		t.Errorf("expected different sentinel names for different credentials under disabled quarantine, got %q", s1)
	}
	if !isCadenceSentinel(s1) || !isCadenceSentinel(s2) {
		t.Errorf("sentinel names must match cadence sentinel pattern: s1=%q s2=%q", s1, s2)
	}
}

func TestIsActiveSentinel_DeadFamilyPreservedWhileFingerprintMatches(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	credsPath := credsWithRefreshToken(t, "rt-dead")
	fp, _ := credentialsFingerprint(credsPath)

	stamp := time.Now().UTC().Format(time.RFC3339)
	lines := []string{stamp, "", credsPath, fp}
	if err := os.WriteFile(sentinel, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if !isActiveSentinel(sentinel) {
		t.Error("dead-family sentinel with matching fingerprint must be preserved across transport errors")
	}
}

func TestIsActiveSentinel_DeadFamilyPreservedWithConfiguredQuarantine(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "token-family-dead-12345678abcdef01")
	credsPath := credsWithRefreshToken(t, "rt-dead-quar")
	fp, _ := credentialsFingerprint(credsPath)

	rec := sentinelRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		QuarantinePath:  "/path/to/quarantine.json",
		CredentialsPath: credsPath,
		Fingerprint:     fp,
		Outcome:         "dead",
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	if !isActiveSentinel(sentinel) {
		t.Error("dead-family sentinel with configured quarantine path must be preserved while fingerprint matches")
	}
}

func TestIsActiveSentinel_OrphanedDisabledQuarantineExpires(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	nonExistentCreds := filepath.Join(dir, "deleted-creds.json")

	rec := sentinelRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		QuarantinePath:  "",
		CredentialsPath: nonExistentCreds,
		Fingerprint:     "deadbeef1234",
		Outcome:         "dead",
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, append(payload, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}

	if isActiveSentinel(sentinel) {
		t.Error("orphaned sentinel for deleted credentials file must be classified as inactive to allow pruning")
	}
}

func TestNotifyCadenceOnce_JSONSerializationHandlesNewlinesInPaths(t *testing.T) {
	dir := t.TempDir()
	sentinelName := "token-family-dead-0000000000000001"
	credsPath := credsWithRefreshToken(t, "rt-newline")
	quarPath := filepath.Join(dir, "quar\nwith\nnewline.json")

	notifyCadenceOnce(dir, sentinelName, quarPath, credsPath, "", "quarantined", "Title", "Message")

	sentinel := filepath.Join(dir, sentinelName)
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	rec, ok := parseSentinelData(data)
	if !ok {
		t.Fatalf("parseSentinelData failed on JSON payload: %s", string(data))
	}
	if rec.QuarantinePath != quarPath {
		t.Errorf("quarantine path with newlines not preserved: got %q, want %q", rec.QuarantinePath, quarPath)
	}
	fp, _ := credentialsFingerprint(credsPath)
	if rec.Fingerprint != fp {
		t.Errorf("fingerprint corrupted by newlines in paths: got %q, want %q", rec.Fingerprint, fp)
	}
	if rec.Outcome != "quarantined" {
		t.Errorf("outcome not preserved: got %q, want %q", rec.Outcome, "quarantined")
	}
}

func TestDefaultStateDir_IsSideEffectFree(t *testing.T) {
	t.Setenv("HOME", "")
	targetFallback := filepath.Join(os.TempDir(), fmt.Sprintf("dear-agent-%d", os.Getuid()))
	if _, err := os.Lstat(targetFallback); os.IsNotExist(err) {
		dir := defaultStateDir()
		if dir != targetFallback {
			t.Errorf("expected %s, got %s", targetFallback, dir)
		}
		if _, err := os.Lstat(targetFallback); !os.IsNotExist(err) {
			t.Errorf("defaultStateDir created %s on disk", targetFallback)
		}
	} else {
		dir := defaultStateDir()
		if dir != targetFallback {
			t.Errorf("expected %s, got %s", targetFallback, dir)
		}
	}
}

func TestEnsureSecureStateDir_RejectsInsecureFallback(t *testing.T) {
	insecureDir := filepath.Join(os.TempDir(), fmt.Sprintf("dear-agent-%d-insecure-%d", os.Getuid(), time.Now().UnixNano()))
	if err := os.Mkdir(insecureDir, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(insecureDir)
	if err := os.Chmod(insecureDir, 0o777); err != nil {
		t.Fatal(err)
	}

	if err := ensureSecureStateDir(insecureDir); err == nil {
		t.Errorf("ensureSecureStateDir should reject fallback directory with 0777 permissions: %s", insecureDir)
	}
}

func TestEnsureSecureStateDir_RejectsReadOnlyDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping chmod-based write denial test when running as root")
	}
	roDir := t.TempDir()
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chmod(roDir, 0o700)
	}()

	if err := ensureSecureStateDir(roDir); err == nil {
		t.Errorf("ensureSecureStateDir should reject non-writable directory: %s", roDir)
	}
}

func TestIsQuarantineActive_SkipsFIFOWithoutHanging(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "quarantine-fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("cannot make FIFO on this platform: %v", err)
	}
	credsPath := credsWithRefreshToken(t, "rt-fifo-test")

	if isQuarantineActive(fifoPath, credsPath) {
		t.Errorf("isQuarantineActive returned true for FIFO path %s", fifoPath)
	}
}

func TestPruneCadenceSentinels_PreservesSentinelIfModifiedBetweenInspectionAndUnlink(t *testing.T) {
	dir := t.TempDir()
	sentinel := filepath.Join(dir, deathSentinelName)
	if err := os.WriteFile(sentinel, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	oldTime := now.Add(-48 * time.Hour)
	_ = os.Chtimes(sentinel, oldTime, oldTime)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	newTime := now
	_ = os.Chtimes(sentinel, newTime, newTime)

	cutoff := now.Add(-24 * time.Hour)
	pruned, err := pruneCadenceEntry(dir, entries[0], cutoff, "")
	if err != nil {
		t.Fatalf("pruneCadenceEntry failed: %v", err)
	}
	if pruned {
		t.Errorf("sentinel modified between inspection and unlink was unexpectedly pruned")
	}
	if _, err := os.Stat(sentinel); os.IsNotExist(err) {
		t.Errorf("sentinel file was deleted despite being modified")
	}
}

func TestNotifyCadenceOnce_PersistsFailedTokenFingerprintEvenIfCredentialsRotate(t *testing.T) {
	stateDir := t.TempDir()
	credsPath := credsWithRefreshToken(t, "rt-initial-failed")
	fpInitial, _ := credentialsFingerprint(credsPath)

	_ = writeCredsFile(credsPath, "access-rotated", freshMs(), "rt-newly-rotated")
	fpRotated, _ := credentialsFingerprint(credsPath)
	if fpInitial == fpRotated {
		t.Fatal("expected different fingerprints for rotated tokens")
	}

	notifyCadenceOnce(stateDir, deathSentinelName, "", credsPath, fpInitial, "dead", "Alert", "Msg")

	sentinel := filepath.Join(stateDir, deathSentinelName)
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	rec, ok := parseSentinelData(data)
	if !ok {
		t.Fatalf("parseSentinelData failed on payload: %s", string(data))
	}
	if rec.Fingerprint != fpInitial {
		t.Errorf("sentinel recorded fingerprint %q, want failed token fingerprint %q", rec.Fingerprint, fpInitial)
	}
}
