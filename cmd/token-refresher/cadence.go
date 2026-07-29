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
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// deathSentinelName marks an in-progress token-family death. Its presence means
// the operator has already been alerted for this episode, so the 30-minute
// cadence does not re-notify every tick; a successful refresh clears it.
const deathSentinelName = "token-family-dead"

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
func cadenceExit(code int, stateDir, sentinelName string, stderr io.Writer) int {
	sentinel := filepath.Join(stateDir, sentinelName)

	switch code {
	case exitTokenFamilyDead:
		if _, err := os.Stat(sentinel); err != nil {
			// First tick of this death episode: alert, then record that we did.
			notifyOperator("Claude auth DOWN", "OAuth token family is dead. Run: claude /login")
			if err := os.MkdirAll(stateDir, 0o700); err == nil {
				stamp := time.Now().UTC().Format(time.RFC3339)
				_ = os.WriteFile(sentinel, []byte(stamp+"\n"), 0o600)
			}
		}
		fmt.Fprintf(stderr, "token-refresher: cadence mode — reporting success so launchd keeps the schedule.\n")
		return exitOK

	case exitQuarantined:
		// A near-miss, and the alert that matters most: the family is still
		// alive precisely because we declined to replay the token. Reuse the
		// death sentinel so one episode raises one notification.
		if _, err := os.Stat(sentinel); err != nil {
			notifyOperator("Claude auth AT RISK",
				"Refresh outcome unknown; token quarantined to protect the family. Check token-refresher -check")
			if err := os.MkdirAll(stateDir, 0o700); err == nil {
				stamp := time.Now().UTC().Format(time.RFC3339)
				_ = os.WriteFile(sentinel, []byte(stamp+"\n"), 0o600)
			}
		}
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
