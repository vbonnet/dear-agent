package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

type sandboxGCSessionStore interface {
	ListSessions(*dolt.SessionFilter) ([]*manifest.Manifest, error)
	Close() error
}

var (
	sandboxGCStoreConfigs = configuredSandboxGCStoreConfigs
	openSandboxGCStore    = func(config *dolt.Config) (sandboxGCSessionStore, error) {
		store, err := dolt.NewWithoutAutoStart(config)
		if err == nil || isMissingDoltDatabaseError(err, config.Database) {
			return store, err
		}
		// The endpoint was not reachable, so preserve the configured recovery
		// behavior for an offline Dolt server. A reachable endpoint reporting a
		// missing database never reaches this auto-start path.
		return dolt.New(config)
	}
)

func configuredSandboxGCStoreConfigs() ([]*dolt.Config, error) {
	path, err := getWorkspaceConfigPath()
	if err != nil {
		return nil, fmt.Errorf("resolve AGM workspace config path: %w", err)
	}
	return dolt.ConfiguredWorkspaceConfigsIncludingDisabledAt(path)
}

func constantLiveSessionIDs(live map[string]bool) func() (map[string]bool, error) {
	return func() (map[string]bool, error) {
		return live, nil
	}
}

func sandboxGCLiveSessionIDs() (map[string]bool, []string, error) {
	if os.Getenv("AGM_DB_PATH") != "" {
		return sandboxGCLiveSessionIDsFromSQLite()
	}
	return sandboxGCLiveSessionIDsFromDolt()
}

func sandboxGCLiveSessionIDsFromSQLite() (map[string]bool, []string, error) {
	store, err := getStorage()
	if err != nil {
		return nil, nil, err
	}
	sessions, listErr := store.ListSessions(nil)
	closeErr := store.Close()
	if listErr != nil {
		return nil, nil, fmt.Errorf("list sessions from SQLite store: %w", listErr)
	}
	if closeErr != nil {
		return nil, nil, fmt.Errorf("close SQLite session store: %w", closeErr)
	}
	if len(sessions) == 0 {
		return nil, nil, fmt.Errorf("SQLite session store returned zero sessions — refusing to treat all sandboxes as orphaned")
	}
	return liveSessionIDsFromManifests(sessions), nil, nil
}

func sandboxGCLiveSessionIDsFromDolt() (map[string]bool, []string, error) {
	configs, err := sandboxGCStoreConfigs()
	if err != nil {
		return nil, nil, err
	}
	live := make(map[string]bool)
	var warnings []string
	var totalSessions int
	var reachableStores int
	for _, config := range configs {
		store, err := openSandboxGCStore(config)
		if err != nil {
			if isMissingDoltDatabaseError(err, config.Database) {
				warnings = append(warnings, fmt.Sprintf(
					"workspace %q skipped: Dolt database %q does not exist",
					config.Workspace, config.Database,
				))
				continue
			}
			return nil, warnings, fmt.Errorf("open Dolt session store for workspace %q: %w", config.Workspace, err)
		}
		sessions, listErr := store.ListSessions(nil)
		closeErr := store.Close()
		if listErr != nil {
			return nil, warnings, fmt.Errorf("list sessions from workspace %q: %w", config.Workspace, listErr)
		}
		if closeErr != nil {
			return nil, warnings, fmt.Errorf("close Dolt session store for workspace %q: %w", config.Workspace, closeErr)
		}
		reachableStores++
		totalSessions += len(sessions)
		for sessionID := range liveSessionIDsFromManifests(sessions) {
			live[sessionID] = true
		}
	}
	if reachableStores == 0 {
		return nil, warnings, fmt.Errorf("no configured Dolt session stores were reachable")
	}
	if totalSessions == 0 {
		return nil, warnings, fmt.Errorf("configured Dolt session stores returned zero sessions — refusing to treat all sandboxes as orphaned")
	}
	return live, warnings, nil
}

func liveSessionIDsFromManifests(sessions []*manifest.Manifest) map[string]bool {
	live := make(map[string]bool)
	for _, session := range sessions {
		if session.Lifecycle != manifest.LifecycleArchived {
			live[session.SessionID] = true
		}
	}
	return live
}

func isMissingDoltDatabaseError(err error, database string) bool {
	if err == nil || database == "" {
		return false
	}
	msg := strings.ToLower(err.Error())
	db := strings.ToLower(database)
	return strings.Contains(msg, "database not found: "+db) ||
		strings.Contains(msg, "unknown database '"+db+"'") ||
		strings.Contains(msg, "unknown database \""+db+"\"")
}
