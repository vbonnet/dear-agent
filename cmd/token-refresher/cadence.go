package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
func cadenceSentinelName(quarantinePath string) string {
	if quarantinePath == "" {
		return deathSentinelName
	}
	sum := sha256.Sum256([]byte(quarantinePath))
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
func cadenceExit(code int, stateDir, sentinelName string, stderr io.Writer, maxAge time.Duration) int {
	if maxAge > 0 {
		if _, err := pruneCadenceSentinels(stateDir, maxAge, sentinelName); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not prune cadence sentinels: %v\n", err)
		}
	}
	switch code {
	case exitTokenFamilyDead:
		notifyCadenceOnce(stateDir, sentinelName,
			"Claude auth DOWN", "OAuth token family is dead. Run: claude /login")
		fmt.Fprintf(stderr, "token-refresher: cadence mode — reporting success so launchd keeps the schedule.\n")
		return exitOK

	case exitQuarantined:
		// A near-miss, and the alert that matters most: the family is still
		// alive precisely because we declined to replay the token. Reuse the
		// death sentinel so one episode raises one notification.
		notifyCadenceOnce(stateDir, sentinelName,
			"Claude auth AT RISK",
			"Refresh outcome unknown; token quarantined to protect the family. Check token-refresher -check")
		fmt.Fprintf(stderr, "token-refresher: cadence mode — reporting success so launchd keeps the schedule.\n")
		return exitOK

	case exitOK:
		// Family is healthy again; arm the alert for the next episode.
		if err := clearCadenceSentinel(stateDir, sentinelName); err != nil {
			fmt.Fprintf(stderr, "token-refresher: could not clear cadence alert state: %v\n", err)
		}
		return exitOK
	}

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
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == keepSentinel || !isCadenceSentinel(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("get entry info for %s: %w", name, err)
			}
			continue
		}
		if info.ModTime().Before(cutoff) {
			target := filepath.Join(stateDir, name)
			if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
				if firstErr == nil {
					firstErr = fmt.Errorf("remove sentinel %s: %w", name, err)
				}
			} else {
				pruned++
			}
		}
	}
	return pruned, firstErr
}

func isCadenceSentinel(name string) bool {
	return name == deathSentinelName || strings.HasPrefix(name, deathSentinelName+"-")
}

// notifyCadenceOnce records the episode after its first best-effort alert.
// Every cadence failure path uses this helper so a durable quarantine/stop does
// not notify again on every launchd tick.
func notifyCadenceOnce(stateDir, sentinelName, title, message string) {
	sentinel := filepath.Join(stateDir, sentinelName)
	if _, err := os.Stat(sentinel); err == nil {
		return
	}
	notifyOperator(title, message)
	if err := os.MkdirAll(stateDir, 0o700); err == nil {
		stamp := time.Now().UTC().Format(time.RFC3339)
		_ = os.WriteFile(sentinel, []byte(stamp+"\n"), 0o600)
	}
}

// notifyNotifyTimeout bounds the osascript call. A launchd job runs without a
// GUI session attached, so `osascript` can block indefinitely when Notification
// Center is unavailable or wedged. An unbounded call there would hang the very
// refresher this file exists to keep alive, so the alert is given a deadline
// and abandoned if it misses it.
const notifyTimeout = 5 * time.Second

// notifyOperator raises a macOS notification. Alerting is best-effort: a
// refresh must never fail — or stall — because the notification did not render.
func notifyOperator(title, message string) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()

	script := fmt.Sprintf("display notification %q with title %q sound name \"Basso\"", message, title)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	_ = cmd.Run()
}

// defaultStateDir is where the sentinel lives, alongside the audit log.
func defaultStateDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return os.TempDir()
	}
	return filepath.Join(home, ".local", "state", "dear-agent")
}
