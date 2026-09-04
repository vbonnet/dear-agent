package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// deathSentinelName marks an in-progress token-family death. Its presence means
// the operator has already been alerted for this episode, so the 30-minute
// cadence does not re-notify every tick; a successful refresh clears it.
const (
	deathSentinelName     = "token-family-dead"
	defaultSentinelMaxAge = 24 * time.Hour
)

// clearCadenceSentinel re-arms the next cadence alert after an operator has
// explicitly cleared quarantine. A missing sentinel means there is nothing to
// re-arm and is therefore successful.
func cadenceSentinelName(quarantinePath, credentialsPath string) string {
	if quarantinePath != "" {
		sum := sha256.Sum256([]byte(quarantinePath))
		return fmt.Sprintf("%s-%x", deathSentinelName, sum[:8])
	}
	canonCreds := canonicalCredentialsPath(credentialsPath)
	if canonCreds == "" || canonCreds == canonicalCredentialsPath("") {
		return deathSentinelName
	}
	sum := sha256.Sum256([]byte(canonCreds))
	return fmt.Sprintf("%s-%x", deathSentinelName, sum[:8])
}

func clearCadenceSentinel(stateDir, name string) error {
	err := os.Remove(filepath.Join(stateDir, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func writeCadenceStop(credentialsPath string) error {
	return (auth.OAuthResolver{CredentialsPath: canonicalCredentialsPath(credentialsPath)}).
		WriteRefreshStop("refresh outcome unknown; operator must clear quarantine")
}

func clearCadenceStop(credentialsPath string) error {
	return (auth.OAuthResolver{CredentialsPath: canonicalCredentialsPath(credentialsPath)}).ClearRefreshStop()
}

func cadenceStopped(credentialsPath string) (bool, error) {
	return (auth.OAuthResolver{CredentialsPath: canonicalCredentialsPath(credentialsPath)}).RefreshStopped()
}

// cadenceExit adapts a run's exit code for the unattended launchd cadence job.
//
// Two problems it solves, both observed on 2026-07-19 (ce-77ip):
//
//  1. A dead token family was invisible. The launchd job wrote "token family is
//     dead" to a log file nobody tails, so the operator discovered the outage
//     by failing to authenticate hours later. Now it raises a desktop
//     notification the moment the family dies.
//
//  2. The job removed itself from the schedule. launchd throttles a
//     StartInterval job that exits non-zero quickly and repeatedly; after the
//     family died, this job exited 2 every 30 minutes until launchd stopped
//     running it entirely -- so when the operator did re-authenticate, nothing
//     was refreshing the credentials any more and the next death was
//     guaranteed. Reporting success keeps the cadence alive so it resumes
//     automatically after the next `claude /login`.
//
// The real exit code still reaches the audit log and stderr; only the process
// status is flattened, and only for the cadence caller.
// pruneCadenceAlerts encapsulates the maxAge check, pruning, and stderr reporting.
func pruneCadenceAlerts(stateDir string, maxAge time.Duration, keepSentinel string, stderr io.Writer) {
	if maxAge > 0 {
		if _, err := pruneCadenceSentinels(stateDir, maxAge, keepSentinel); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not prune cadence sentinels: %v\n", err)
		}
	}
}

func cadenceExit(code int, stateDir, sentinelName, quarantinePath, credentialsPath, tokenFP string, stderr io.Writer, maxAge time.Duration) int {
	switch code {
	case exitTokenFamilyDead:
		notifyCadenceOnce(stateDir, sentinelName, quarantinePath, credentialsPath, tokenFP, "dead",
			"Claude auth DOWN", "OAuth token family is dead. Run: claude /login")
		fmt.Fprintf(stderr, "token-refresher: cadence mode: reporting success so launchd keeps the schedule.\n")
		pruneCadenceAlerts(stateDir, maxAge, sentinelName, stderr)
		return exitOK

	case exitQuarantined:
		// A near-miss, and the alert that matters most: the family is still
		// alive precisely because we declined to replay the token. Reuse the
		// death sentinel so one episode raises one notification.
		notifyCadenceOnce(stateDir, sentinelName, quarantinePath, credentialsPath, tokenFP, "quarantined",
			"Claude auth AT RISK",
			"Refresh outcome unknown; token quarantined to protect the family. Check token-refresher -check")
		fmt.Fprintf(stderr, "token-refresher: cadence mode: reporting success so launchd keeps the schedule.\n")
		pruneCadenceAlerts(stateDir, maxAge, sentinelName, stderr)
		return exitOK

	case exitOK:
		// Family is healthy again; arm the alert for the next episode.
		if err := clearCadenceSentinel(stateDir, sentinelName); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not clear cadence alert state: %v\n", err)
		}
		pruneCadenceAlerts(stateDir, maxAge, "", stderr)
		return exitOK
	}

	pruneCadenceAlerts(stateDir, maxAge, "", stderr)
	return code
}

// pruneCadenceSentinels removes dead-family sentinels in stateDir that are older
// than maxAge, excluding keepSentinel to preserve alerts for in-progress episodes.
// If maxAge <= 0, pruning is disabled and returns (0, nil).
// It returns the count of removed sentinels and the first encountered error.
func pruneCadenceSentinels(stateDir string, maxAge time.Duration, keepSentinel string) (int, error) {
	if maxAge <= 0 || stateDir == "" {
		return 0, nil
	}
	cleanState := filepath.Clean(stateDir)
	if cleanState == "/" || cleanState == filepath.Clean(os.TempDir()) {
		return 0, nil
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("read state directory: %w", err)
	}
	cutoff := time.Now().Add(-maxAge)
	pruned := 0
	var firstErr error
	for _, entry := range entries {
		removed, err := pruneCadenceEntry(stateDir, entry, cutoff, keepSentinel)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		if removed {
			pruned++
		}
	}
	return pruned, firstErr
}

func pruneCadenceEntry(stateDir string, entry os.DirEntry, cutoff time.Time, keepSentinel string) (bool, error) {
	if !entry.Type().IsRegular() {
		return false, nil
	}
	name := entry.Name()
	if name == keepSentinel || !isCadenceSentinel(name) {
		return false, nil
	}
	target := filepath.Join(stateDir, name)
	info, err := entry.Info()
	if err != nil {
		return false, fmt.Errorf("get entry info for %s: %w", name, err)
	}
	if !info.ModTime().Before(cutoff) || isActiveSentinel(target) {
		return false, nil
	}
	currentInfo, err := os.Lstat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat sentinel %s before removal: %w", name, err)
	}
	if !currentInfo.ModTime().Before(cutoff) || !os.SameFile(info, currentInfo) || !currentInfo.ModTime().Equal(info.ModTime()) || isActiveSentinel(target) {
		return false, nil
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("remove sentinel %s: %w", name, err)
	}
	return true, nil
}

func isCadenceSentinel(name string) bool {
	if name == deathSentinelName {
		return true
	}
	prefix := deathSentinelName + "-"
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	if len(suffix) != 16 {
		return false
	}
	for i := 0; i < len(suffix); i++ {
		c := suffix[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

type sentinelRecord struct {
	Timestamp       string `json:"timestamp"`
	QuarantinePath  string `json:"quarantine_path,omitempty"`
	CredentialsPath string `json:"credentials_path,omitempty"`
	Fingerprint     string `json:"fingerprint,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
}

func parseSentinelData(data []byte) (sentinelRecord, bool) {
	var rec sentinelRecord
	if err := json.Unmarshal(data, &rec); err == nil && rec.Timestamp != "" {
		return rec, true
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return sentinelRecord{}, false
	}
	rec.Timestamp = strings.TrimSpace(lines[0])
	if len(lines) >= 2 {
		rec.QuarantinePath = filepath.Clean(strings.TrimSpace(lines[1]))
	}
	if len(lines) >= 3 {
		rec.CredentialsPath = strings.TrimSpace(lines[2])
	}
	if len(lines) >= 4 {
		rec.Fingerprint = strings.TrimSpace(lines[3])
	}
	return rec, true
}

func sentinelMatchesEpisode(rec sentinelRecord, credPath, tokenFP string) bool {
	if tokenFP == "" {
		tokenFP, _ = credentialsFingerprint(credPath)
	}
	if tokenFP == "" {
		return true
	}
	if rec.Fingerprint == "" {
		return false
	}
	return tokenFP == rec.Fingerprint
}

func isQuarantineActive(quarPath, credPath string) bool {
	if quarPath == "" || quarPath == "." {
		return false
	}
	// #nosec G703 -- path was recorded by local token-refresher to track its active quarantine marker.
	info, err := os.Lstat(quarPath)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	resolver := auth.OAuthResolver{
		CredentialsPath: canonicalCredentialsPath(credPath),
		QuarantinePath:  quarPath,
	}
	_, _, _, active := resolver.QuarantineStatus()
	return active
}

func isCadenceStoppedFor(target, credPath string) bool {
	if filepath.Base(target) != deathSentinelName && credPath == "" {
		return false
	}
	stopped, err := cadenceStopped(credPath)
	return err == nil && stopped
}

func readSentinelRecord(path string) (sentinelRecord, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return sentinelRecord{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sentinelRecord{}, false
	}
	return parseSentinelData(data)
}

func isDeadSentinel(rec sentinelRecord) bool {
	if rec.Outcome == "dead" {
		return true
	}
	return rec.Outcome == "" && (rec.QuarantinePath == "" || rec.QuarantinePath == ".")
}

func isActiveSentinel(target string) bool {
	rec, ok := readSentinelRecord(target)
	if !ok || rec.Fingerprint == "" || rec.CredentialsPath == "" {
		return false
	}
	currentFP, _ := credentialsFingerprint(rec.CredentialsPath)
	if currentFP == "" || currentFP != rec.Fingerprint {
		return false
	}
	if isDeadSentinel(rec) {
		return true
	}
	if rec.QuarantinePath != "" && rec.QuarantinePath != "." {
		return isQuarantineActive(rec.QuarantinePath, rec.CredentialsPath) || isCadenceStoppedFor(target, rec.CredentialsPath)
	}
	return false
}

// notifyCadenceOnce records the episode after its first best-effort alert.
// Every cadence failure path uses this helper so a durable quarantine/stop does
// not notify again on every launchd tick.
func notifyCadenceOnce(stateDir, sentinelName, quarantinePath, credentialsPath, tokenFP, outcome, title, message string) {
	sentinel := filepath.Join(stateDir, sentinelName)
	if tokenFP == "" {
		tokenFP, _ = credentialsFingerprint(credentialsPath)
	}
	if info, err := os.Lstat(sentinel); err == nil && !info.Mode().IsRegular() {
		_ = os.Remove(sentinel)
	} else if rec, ok := readSentinelRecord(sentinel); ok {
		if sentinelMatchesEpisode(rec, credentialsPath, tokenFP) {
			now := time.Now()
			_ = os.Chtimes(sentinel, now, now)
			return
		}
	}
	notifyOperator(title, message)
	if err := ensureSecureStateDir(stateDir); err == nil {
		rec := sentinelRecord{
			Timestamp:       time.Now().UTC().Format(time.RFC3339),
			QuarantinePath:  quarantinePath,
			CredentialsPath: credentialsPath,
			Fingerprint:     tokenFP,
			Outcome:         outcome,
		}
		if payload, err := json.Marshal(rec); err == nil {
			_ = os.WriteFile(sentinel, append(payload, '\n'), 0o600)
		}
	}
}

// notifyNotifyTimeout bounds the osascript call. A launchd job runs without a
// GUI session attached, so `osascript` can block indefinitely when Notification
// Center is unavailable or wedged. An unbounded call there would hang the very
// refresher this file exists to keep alive, so the alert is given a deadline
// and abandoned if it misses it.
const notifyTimeout = 5 * time.Second

// notifyOperator raises a macOS notification. Alerting is best-effort: a
// refresh must never fail or stall because the notification did not render.
func notifyOperator(title, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	script := fmt.Sprintf("display notification %q with title %q sound name \"Basso\"", message, title)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	_ = cmd.Run()
}

func isSecureStateDir(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(stat.Uid) == os.Getuid() && info.Mode().Perm() == 0o700
	}
	return false
}

func fallbackStateDir() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("dear-agent-%d", os.Getuid()))
}

func isFallbackStateDir(clean string) bool {
	return clean == filepath.Clean(fallbackStateDir()) ||
		(strings.HasPrefix(clean, filepath.Clean(os.TempDir())) && strings.HasPrefix(filepath.Base(clean), fmt.Sprintf("dear-agent-%d", os.Getuid())))
}

func isWritableDir(dir string) error {
	if err := syscall.Access(dir, 2); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	_ = probe.Close()
	_ = os.Remove(probe.Name())
	return nil
}

func ensureSecureStateDir(stateDir string) error {
	if stateDir == "" {
		return errors.New("state directory is empty")
	}
	clean := filepath.Clean(stateDir)
	info, err := os.Lstat(clean)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("state directory %s is not a directory or is a symlink", clean)
		}
		if isFallbackStateDir(clean) && !isSecureStateDir(clean) {
			return fmt.Errorf("fallback state directory %s has untrusted ownership or permissions", clean)
		}
		if info.Mode().Perm()&0o002 != 0 {
			return fmt.Errorf("state directory %s is world-writable", clean)
		}
		if err := isWritableDir(clean); err != nil {
			return fmt.Errorf("state directory %s is not writable: %w", clean, err)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(clean, 0o700); err != nil {
		return err
	}
	if isFallbackStateDir(clean) && !isSecureStateDir(clean) {
		return fmt.Errorf("fallback state directory %s has untrusted ownership or permissions", clean)
	}
	if err := isWritableDir(clean); err != nil {
		return fmt.Errorf("state directory %s is not writable: %w", clean, err)
	}
	return nil
}

// defaultStateDir is where the sentinel lives, alongside the audit log.
func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err == nil {
		return filepath.Join(home, ".local", "state", "dear-agent")
	}
	return fallbackStateDir()
}
