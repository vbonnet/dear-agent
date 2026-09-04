package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func TestNotifyCadenceOnce_ClaimsEpisodeAtomicallyConcurrent(t *testing.T) {
	stateDir := t.TempDir()
	credsPath := credsWithRefreshToken(t, "rt-concurrent")
	fp, _ := credentialsFingerprint(credsPath)

	done := make(chan bool, 2)
	for range 2 {
		go func() {
			notifyCadenceOnce(stateDir, deathSentinelName, "", credsPath, fp, "dead", "Alert", "Msg")
			done <- true
		}()
	}
	<-done
	<-done

	sentinel := filepath.Join(stateDir, deathSentinelName)
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	rec, ok := parseSentinelData(data)
	if !ok {
		t.Fatalf("parseSentinelData failed on payload: %s", string(data))
	}
	if rec.Fingerprint != fp {
		t.Errorf("got fingerprint %q, want %q", rec.Fingerprint, fp)
	}
}

func TestIsActiveSentinel_SkipsFIFOCredentialsWithoutHanging(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "creds-fifo")
	if err := syscall.Mkfifo(fifoPath, 0o600); err != nil {
		t.Skipf("cannot make FIFO on this platform: %v", err)
	}
	sentinel := filepath.Join(dir, deathSentinelName)
	rec := sentinelRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		CredentialsPath: fifoPath,
		Fingerprint:     "dummy-fp",
		Outcome:         "dead",
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if isActiveSentinel(sentinel) {
		t.Errorf("isActiveSentinel returned true for FIFO credentials path %s", fifoPath)
	}
}

func TestEnsureSecureStateDir_RejectsGroupWritableDirectory(t *testing.T) {
	dir := t.TempDir()
	gwDir := filepath.Join(dir, "group-writable")
	if err := os.Mkdir(gwDir, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(gwDir, 0o770); err != nil {
		t.Fatal(err)
	}

	err := ensureSecureStateDir(gwDir)
	if err == nil {
		t.Errorf("ensureSecureStateDir should reject group-writable directory %s", gwDir)
	} else if !strings.Contains(err.Error(), "group- or world-writable") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestIsActiveSentinel_PreservesDisabledQuarantineWithActiveStop(t *testing.T) {
	dir := t.TempDir()
	credsPath := credsWithRefreshToken(t, "rt-disabled-quar")
	fp, _ := credentialsFingerprint(credsPath)

	stopPath := credsPath + ".refresh-stop"
	if err := os.WriteFile(stopPath, []byte("durable stop active\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	sentinel := filepath.Join(dir, deathSentinelName)
	rec := sentinelRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		CredentialsPath: credsPath,
		QuarantinePath:  "",
		Fingerprint:     fp,
		Outcome:         "quarantined",
	}
	payload, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	if !isActiveSentinel(sentinel) {
		t.Error("isActiveSentinel should return true for disabled-quarantine job with active durable stop")
	}
}

func TestEnsureSecureStateDir_RejectsSymlinkFallbackWithoutFollowing(t *testing.T) {
	dir := t.TempDir()
	targetDir := filepath.Join(dir, "target")
	if err := os.Mkdir(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkPath := filepath.Join(dir, "symlink-to-target")
	if err := os.Symlink(targetDir, symlinkPath); err != nil {
		t.Fatal(err)
	}

	err := ensureSecureStateDir(symlinkPath)
	if err == nil {
		t.Errorf("ensureSecureStateDir should reject symlink state directory: %s", symlinkPath)
	}

	targetInfo, err := os.Lstat(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	if targetInfo.Mode().Perm() != 0o755 {
		t.Errorf("target directory permissions were changed to %v; ensureSecureStateDir followed symlink", targetInfo.Mode().Perm())
	}
}

func TestNotifyCadenceOnce_CanonicalizesRelativeQuarantinePath(t *testing.T) {
	stateDir := t.TempDir()
	credsPath := credsWithRefreshToken(t, "rt-relative-quar")
	fp, _ := credentialsFingerprint(credsPath)

	notifyCadenceOnce(stateDir, deathSentinelName, "relative/quar.json", credsPath, fp, "quarantined", "Title", "Msg")

	sentinel := filepath.Join(stateDir, deathSentinelName)
	data, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	rec, ok := parseSentinelData(data)
	if !ok {
		t.Fatalf("parseSentinelData failed on payload: %s", string(data))
	}
	if !filepath.IsAbs(rec.QuarantinePath) {
		t.Errorf("expected absolute quarantine path in sentinel, got %q", rec.QuarantinePath)
	}
}

func TestPruneCadenceSentinels_HoldsLockCoordinatesWithClaim(t *testing.T) {
	stateDir := t.TempDir()
	sentinelName := deathSentinelName
	sentinelPath := filepath.Join(stateDir, sentinelName)

	credsPath := credsWithRefreshToken(t, "rt-stale")
	fp, _ := credentialsFingerprint(credsPath)

	staleRec := sentinelRecord{
		Timestamp:       time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		CredentialsPath: credsPath,
		Fingerprint:     "old-fp-no-longer-matches",
		Outcome:         "quarantined",
		QuarantinePath:  filepath.Join(stateDir, "nonexistent.json"),
	}
	payload, err := json.Marshal(staleRec)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinelPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(sentinelPath, oldTime, oldTime)

	lockF, err := os.OpenFile(sentinelPath, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lockF.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	pruneDone := make(chan int, 1)
	go func() {
		pruned, _ := pruneCadenceSentinels(stateDir, 1*time.Hour, "")
		pruneDone <- pruned
	}()

	time.Sleep(50 * time.Millisecond)

	newRec := sentinelRecord{
		Timestamp:       time.Now().UTC().Format(time.RFC3339),
		CredentialsPath: credsPath,
		Fingerprint:     fp,
		Outcome:         "dead",
	}
	newPayload, _ := json.Marshal(newRec)
	_ = lockF.Truncate(0)
	_, _ = lockF.Seek(0, io.SeekStart)
	_, _ = lockF.Write(newPayload)
	now := time.Now()
	_ = os.Chtimes(sentinelPath, now, now)

	_ = syscall.Flock(int(lockF.Fd()), syscall.LOCK_UN)
	_ = lockF.Close()

	select {
	case pruned := <-pruneDone:
		if pruned != 0 {
			t.Errorf("expected 0 pruned sentinels, got %d", pruned)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pruneCadenceSentinels")
	}

	if _, err := os.Stat(sentinelPath); err != nil {
		t.Errorf("sentinel was pruned despite concurrent claim: %v", err)
	}
}
