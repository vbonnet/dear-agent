package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type fakeSandboxGCStore struct {
	sessions []*manifest.Manifest
	listErr  error
	closeErr error
}

func (s *fakeSandboxGCStore) ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.sessions, nil
}

func (s *fakeSandboxGCStore) Close() error {
	return s.closeErr
}

func restoreSandboxGCDepsForTest(t *testing.T) {
	t.Helper()
	oldConfigs := sandboxGCStoreConfigs
	oldOpen := openSandboxGCStore
	oldLog := logSandboxGCEntry
	t.Cleanup(func() {
		sandboxGCStoreConfigs = oldConfigs
		openSandboxGCStore = oldOpen
		logSandboxGCEntry = oldLog
	})
	logSandboxGCEntry = func(entry gclog.Entry) {}
}

func TestSandboxGCLiveSessionIDsSkipsMissingConfiguredDatabase(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	configs := []*dolt.Config{
		{Workspace: "personal", Database: "personal"},
		{Workspace: "oss", Database: "oss"},
	}
	sandboxGCStoreConfigs = func() ([]*dolt.Config, error) { return configs, nil }
	openSandboxGCStore = func(config *dolt.Config) (sandboxGCSessionStore, error) {
		if config.Workspace == "personal" {
			return nil, errors.New("Error 1105 (HY000): database not found: personal")
		}
		return &fakeSandboxGCStore{sessions: []*manifest.Manifest{
			{SessionID: "live-session"},
			{SessionID: "archived-session", Lifecycle: manifest.LifecycleArchived},
		}}, nil
	}

	live, warnings, err := sandboxGCLiveSessionIDs()
	if err != nil {
		t.Fatalf("sandboxGCLiveSessionIDs() error = %v", err)
	}
	if !live["live-session"] {
		t.Fatalf("live sessions = %v, want live-session", live)
	}
	if live["archived-session"] {
		t.Fatalf("live sessions = %v, archived session must not be live", live)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `workspace "personal" skipped`) {
		t.Fatalf("warnings = %v, want personal skipped warning", warnings)
	}
}

func TestSandboxGCLiveSessionIDsFailsWhenOnlyMissingStores(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	sandboxGCStoreConfigs = func() ([]*dolt.Config, error) {
		return []*dolt.Config{{Workspace: "personal", Database: "personal"}}, nil
	}
	openSandboxGCStore = func(config *dolt.Config) (sandboxGCSessionStore, error) {
		return nil, errors.New("database not found: personal")
	}

	_, warnings, err := sandboxGCLiveSessionIDs()
	if err == nil {
		t.Fatal("sandboxGCLiveSessionIDs() error = nil, want fail-closed error")
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want skipped database warning", warnings)
	}
}

func TestSandboxGCLiveSessionIDsFailsWhenReachableStoresAreEmpty(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	sandboxGCStoreConfigs = func() ([]*dolt.Config, error) {
		return []*dolt.Config{{Workspace: "oss", Database: "oss"}}, nil
	}
	openSandboxGCStore = func(config *dolt.Config) (sandboxGCSessionStore, error) {
		return &fakeSandboxGCStore{}, nil
	}

	if _, _, err := sandboxGCLiveSessionIDs(); err == nil {
		t.Fatal("sandboxGCLiveSessionIDs() error = nil, want zero-session fail-closed error")
	}
}

func TestIsMissingDoltDatabaseError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "dolt message", err: errors.New("database not found: personal"), want: true},
		{name: "mysql message", err: errors.New("Error 1049: Unknown database 'personal'"), want: true},
		{name: "other database", err: errors.New("database not found: oss"), want: false},
		{name: "connection refused", err: errors.New("connect: connection refused"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingDoltDatabaseError(tt.err, "personal"); got != tt.want {
				t.Fatalf("isMissingDoltDatabaseError() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A skipped workspace contributes none of its live session IDs to the
// inventory, so every sandbox it owns looks unowned and can pass the remaining
// safety gates. Reaping on that partial knowledge deletes a live session's
// sandbox because the store proving it is live could not be read.
func TestEffectiveSandboxGCReapRefusesPartialInventory(t *testing.T) {
	tests := []struct {
		name       string
		requested  bool
		warnings   []string
		wantReap   bool
		wantNotice bool
	}{
		{name: "complete inventory reaps", requested: true, wantReap: true},
		{
			name:       "one skipped workspace refuses",
			requested:  true,
			warnings:   []string{`workspace "personal" skipped`},
			wantReap:   false,
			wantNotice: true,
		},
		{
			name:       "several skipped workspaces refuse",
			requested:  true,
			warnings:   []string{"a skipped", "b skipped"},
			wantReap:   false,
			wantNotice: true,
		},
		{
			name:      "dry run stays a dry run without a notice",
			requested: false,
			warnings:  []string{`workspace "personal" skipped`},
			wantReap:  false,
		},
		{name: "dry run with complete inventory", requested: false, wantReap: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reap, notice := effectiveSandboxGCReap(tt.requested, tt.warnings)
			if reap != tt.wantReap {
				t.Errorf("reap = %v, want %v", reap, tt.wantReap)
			}
			if (notice != "") != tt.wantNotice {
				t.Errorf("notice = %q, want notice present = %v", notice, tt.wantNotice)
			}
			if tt.wantNotice && !strings.Contains(notice, "refusing to reap") {
				t.Errorf("notice = %q, want it to say why the reap was refused", notice)
			}
		})
	}
}

// The scheduler that launched this sweep must be stamped on every record it
// writes. Without it, a reader cannot tell the hourly schedule's heartbeats
// from the ones some other component's remediation triggered on its own
// behalf — and a reader that grades the schedule on its own sweeps reports a
// dead schedule as alive (cmd/disk-watchdog, DW-27).
func TestLogSandboxGCEntryStampsTheDeclaredSource(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "declared runner is stamped", env: "disk-watchdog", want: "disk-watchdog"},
		{name: "surrounding whitespace is trimmed", env: "  disk-watchdog\n", want: "disk-watchdog"},
		{name: "undeclared runner stays empty", env: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreSandboxGCDepsForTest(t)
			var got gclog.Entry
			logSandboxGCEntry = func(entry gclog.Entry) { got = entry }
			t.Setenv(sandboxGCSourceEnv, tt.env)

			logSandboxGCEntryTagged(gclog.Entry{Operation: "sandbox_gc_completed"})

			if got.Source != tt.want {
				t.Errorf("Source = %q, want %q", got.Source, tt.want)
			}
		})
	}
}

// An explicit source on the entry wins over the environment, so a caller that
// knows better is never overwritten by an inherited variable.
func TestLogSandboxGCEntryKeepsAnExplicitSource(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	var got gclog.Entry
	logSandboxGCEntry = func(entry gclog.Entry) { got = entry }
	t.Setenv(sandboxGCSourceEnv, "disk-watchdog")

	logSandboxGCEntryTagged(gclog.Entry{Operation: "sandbox_gc_completed", Source: "launchd"})

	if got.Source != "launchd" {
		t.Errorf("Source = %q, want the explicit %q", got.Source, "launchd")
	}
}
