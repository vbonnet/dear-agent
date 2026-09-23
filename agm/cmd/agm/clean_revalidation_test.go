package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

type strictCleanupChecker struct {
	active bool
	err    error
	names  []string
}

func (c *strictCleanupChecker) HasSessionStrict(_ context.Context, name string) (bool, error) {
	c.names = append(c.names, name)
	return c.active, c.err
}

func cleanupFixture(t *testing.T, lifecycle string) (*dolt.Adapter, *ui.Session, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	previousCfg := cfg
	cfg = &config.Config{SessionsDir: filepath.Join(root, "sessions")}
	t.Cleanup(func() { cfg = previousCfg })

	adapter, err := dolt.NewSQLiteAdapter(filepath.Join(root, "agm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adapter.Close() })

	old := time.Now().AddDate(0, 0, -180)
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "selected-id",
		Name:          "selected-name",
		Harness:       "agy",
		Lifecycle:     lifecycle,
		CreatedAt:     old,
		UpdatedAt:     old,
		Context:       manifest.Context{Project: root},
		Tmux:          manifest.Tmux{SessionName: "selected-pane"},
	}
	if err := adapter.CreateSession(m); err != nil {
		t.Fatal(err)
	}
	selectedManifest, err := adapter.GetSession(m.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	dir := getSessionDir(m.SessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sentinel"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	return adapter, &ui.Session{Manifest: selectedManifest, UpdatedAt: selectedManifest.UpdatedAt}, dir
}

func TestCleanupSelection_UnchangedTargetsComplete(t *testing.T) {
	for _, tc := range []struct {
		name, lifecycle string
		action          cleanupAction
		threshold       int
	}{
		{"archive", manifest.LifecycleLegacy, cleanupArchive, 30},
		{"delete", manifest.LifecycleArchived, cleanupDelete, 90},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, selected, dir := cleanupFixture(t, tc.lifecycle)
			checker := &strictCleanupChecker{}
			applied, reason, err := applyCleanupSelection(context.Background(), adapter, selected,
				tc.action, tc.threshold, checker)
			if err != nil || !applied || reason != "" {
				t.Fatalf("cleanup = (%t, %q, %v), want applied", applied, reason, err)
			}
			if len(checker.names) != 1 || checker.names[0] != "selected-pane" {
				t.Fatalf("strict tmux probes = %v, want selected-pane", checker.names)
			}
			if tc.action == cleanupArchive {
				stored, err := adapter.GetSession(selected.SessionID)
				if err != nil || stored.Lifecycle != manifest.LifecycleArchived {
					t.Fatalf("archive lifecycle = %v, err = %v", stored, err)
				}
			} else if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Fatalf("selected delete directory still exists or stat failed: %v", err)
			}
		})
	}
}

func TestCleanupSelection_ChangedArchivedSessionPreservesDirectory(t *testing.T) {
	adapter, selected, dir := cleanupFixture(t, manifest.LifecycleArchived)
	current, err := adapter.GetSession(selected.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	current.Name = "changed-after-prompt"
	if err := adapter.UpdateSession(current); err != nil {
		t.Fatal(err)
	}
	checker := &strictCleanupChecker{}
	applied, reason, err := applyCleanupSelection(context.Background(), adapter, selected,
		cleanupDelete, 90, checker)
	if err != nil || applied || !strings.Contains(reason, "changed") {
		t.Fatalf("cleanup = (%t, %q, %v), want changed skip", applied, reason, err)
	}
	if len(checker.names) != 0 {
		t.Fatalf("probed stale selection: %v", checker.names)
	}
	if _, err := os.Stat(filepath.Join(dir, "sentinel")); err != nil {
		t.Fatalf("changed session directory was removed: %v", err)
	}
}

func TestCleanupEligibility_CatchesChangeWithoutTimestampAdvance(t *testing.T) {
	updated := time.Now().AddDate(0, 0, -180)
	parent := "parent-id"
	selectedManifest := &manifest.Manifest{
		SessionID: "child-id", Lifecycle: manifest.LifecycleArchived,
		UpdatedAt: updated, ParentSessionID: &parent,
	}
	selected := &ui.Session{Manifest: selectedManifest, UpdatedAt: updated}
	current := *selectedManifest
	current.ParentSessionID = nil // DetachChild does not advance updated_at.
	reason, err := cleanupEligibility(&current, selected, cleanupDelete, 90)
	if err != nil || !strings.Contains(reason, "changed") {
		t.Fatalf("unversioned parent change = (%q, %v), want changed skip", reason, err)
	}
}

func TestCleanupSelection_RejectsIDsOutsideSessionsDirectory(t *testing.T) {
	root := t.TempDir()
	previousCfg := cfg
	cfg = &config.Config{SessionsDir: filepath.Join(root, "sessions")}
	t.Cleanup(func() { cfg = previousCfg })
	sentinel := filepath.Join(root, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{".", "..", "../other", "nested/id", root} {
		t.Run(strings.ReplaceAll(id, "/", "_"), func(t *testing.T) {
			selected := &ui.Session{Manifest: &manifest.Manifest{SessionID: id}}
			applied, _, err := applyCleanupSelection(context.Background(), nil, selected,
				cleanupDelete, 0, &strictCleanupChecker{})
			if applied || err == nil || !strings.Contains(err.Error(), "single path component") {
				t.Fatalf("unsafe ID %q = (%t, %v), want rejection", id, applied, err)
			}
			if _, err := os.Stat(sentinel); err != nil {
				t.Fatalf("unsafe ID %q removed sibling file: %v", id, err)
			}
		})
	}
}

func TestCleanupSelection_StrictTmuxProbeFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		active bool
		err    error
	}{
		{"active", true, nil},
		{"socket failure", false, errors.New("socket inaccessible")},
	} {
		for _, action := range []cleanupAction{cleanupArchive, cleanupDelete} {
			t.Run(tc.name+"/"+string(rune('0'+action)), func(t *testing.T) {
				lifecycle := manifest.LifecycleLegacy
				if action == cleanupDelete {
					lifecycle = manifest.LifecycleArchived
				}
				adapter, selected, dir := cleanupFixture(t, lifecycle)
				checker := &strictCleanupChecker{active: tc.active, err: tc.err}
				applied, reason, err := applyCleanupSelection(context.Background(), adapter, selected,
					action, 30, checker)
				if applied || (tc.err != nil && err == nil) || (tc.err == nil && reason == "") {
					t.Fatalf("cleanup = (%t, %q, %v), want no mutation", applied, reason, err)
				}
				stored, getErr := adapter.GetSession(selected.SessionID)
				if getErr != nil || stored.Lifecycle != lifecycle {
					t.Fatalf("lifecycle changed: %v, err = %v", stored, getErr)
				}
				if _, statErr := os.Stat(filepath.Join(dir, "sentinel")); statErr != nil {
					t.Fatalf("directory changed: %v", statErr)
				}
			})
		}
	}
}

func TestCleanupSelection_LockReloadRejectsChangeDuringPrompt(t *testing.T) {
	adapter, selected, dir := cleanupFixture(t, manifest.LifecycleLegacy)
	held := make(chan struct{})
	release := make(chan struct{})
	lockDone := make(chan error, 1)
	go func() {
		lockDone <- ops.WithSessionLock(selected.SessionID, func() error {
			close(held)
			<-release
			current, err := adapter.GetSession(selected.SessionID)
			if err != nil {
				return err
			}
			current.Name = "resumed-during-prompt"
			return adapter.UpdateSession(current)
		})
	}()
	<-held
	started := make(chan struct{})
	type cleanupResult struct {
		applied bool
		reason  string
		err     error
	}
	done := make(chan cleanupResult, 1)
	checker := &strictCleanupChecker{}
	go func() {
		close(started)
		applied, reason, err := applyCleanupSelection(context.Background(), adapter, selected,
			cleanupArchive, 30, checker)
		done <- cleanupResult{applied, reason, err}
	}()
	<-started
	select {
	case result := <-done:
		t.Fatalf("cleanup crossed held lifecycle lock: %+v", result)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-lockDone; err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.err != nil || result.applied || !strings.Contains(result.reason, "changed") {
		t.Fatalf("cleanup = %+v, want changed skip", result)
	}
	stored, err := adapter.GetSession(selected.SessionID)
	if err != nil || stored.Lifecycle != manifest.LifecycleLegacy {
		t.Fatalf("lifecycle changed: %v, err = %v", stored, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sentinel")); err != nil {
		t.Fatalf("session directory changed: %v", err)
	}
}

// A canceled batch must not report success.
//
// SIGINT and SIGTERM cancel the root command context, and
// applyCleanupSelection surfaces that as an ordinary error. While both loops
// treated it as a per-item warning, they ran to completion and the command
// printed "Cleanup complete" and returned nil, so an interrupted batch exited
// successfully while silently leaving every remaining selection untouched.
func TestCleanupCanceledIsNotAPerItemWarning(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()

	if !cleanupCanceled(canceled, nil) {
		t.Error("a canceled context alone must count as cancellation")
	}
	if !cleanupCanceled(context.Background(), context.Canceled) {
		t.Error("a wrapped context.Canceled must count as cancellation")
	}
	if !cleanupCanceled(context.Background(), fmt.Errorf("lock: %w", context.DeadlineExceeded)) {
		t.Error("a wrapped deadline error must count as cancellation")
	}
	if cleanupCanceled(context.Background(), errors.New("reload session: no such row")) {
		t.Error("an ordinary per-item failure must not count as cancellation")
	}

	err := cleanupCancellation(canceled, nil, 2, 1)
	if err == nil {
		t.Fatal("an interrupted batch must return a non-nil error so the exit status says so")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("cleanupCancellation() = %v, want it to wrap the cancellation cause", err)
	}
}
