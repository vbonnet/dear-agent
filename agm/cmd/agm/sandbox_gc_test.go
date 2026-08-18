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
