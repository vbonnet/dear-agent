package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/agysession"
	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func enrichManifestWithAgyConversation(m *manifest.Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest cannot be nil")
	}
	workspaceCandidates := agyWorkspaceCandidates(m)
	if len(workspaceCandidates) == 0 {
		return fmt.Errorf("AGY session has no working directory")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("determine home directory: %w", err)
	}
	var meta *agysession.Metadata
	var lookupErrs []string
	for _, workspacePath := range workspaceCandidates {
		meta, err = agysession.FindLatestForWorkspace(homeDir, workspacePath)
		if err == nil {
			break
		}
		lookupErrs = append(lookupErrs, fmt.Sprintf("%s: %v", workspacePath, err))
	}
	if meta == nil {
		return fmt.Errorf("discover AGY conversation metadata: %s", strings.Join(lookupErrs, "; "))
	}
	m.Harness = "agy"
	m.WorkingDirectory = meta.WorkspacePath
	if m.Context.Project == "" {
		m.Context.Project = meta.WorkspacePath
	}
	m.Agy = &manifest.Agy{
		ConversationID: meta.ConversationID,
		WorkspacePath:  meta.WorkspacePath,
		ConversationDB: meta.ConversationDBPath,
		TranscriptPath: meta.TranscriptPath,
	}
	return nil
}

func agyWorkspaceCandidates(m *manifest.Manifest) []string {
	if m == nil {
		return nil
	}
	var candidates []string
	seen := make(map[string]struct{})
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		candidates = append(candidates, path)
	}
	// Context.Project is AGM's canonical project/worktree field. Prefer it over
	// WorkingDirectory, which may reflect the operator's current shell cwd during
	// manual association.
	add(m.Context.Project)
	add(m.WorkingDirectory)
	if m.Agy != nil {
		add(m.Agy.WorkspacePath)
	}
	return candidates
}

func associateSpawnedAgySession(sessionName string) {
	adapter, err := getStorage()
	if err != nil {
		debug.Log("AGY association skipped: failed to connect to Dolt: %v", err)
		return
	}
	defer func() { _ = adapter.Close() }()

	m, err := adapter.GetSessionByName(sessionName)
	if err != nil || m == nil {
		debug.Log("AGY association skipped: session %q not found in Dolt: %v", sessionName, err)
		return
	}
	if err := enrichManifestWithAgyConversation(m); err != nil {
		debug.Log("AGY association skipped: failed to discover AGY conversation metadata: %v", err)
		return
	}
	if err := persistAssociatedManifest(adapter, m); err != nil {
		debug.Log("AGY association skipped: failed to persist metadata: %v", err)
	}
}

func associateSpawnedAgySessionWithRetry(ctx context.Context, sessionName string, attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		adapter, err := getStorage()
		if err != nil {
			debug.Log("AGY association skipped: failed to connect to Dolt: %v", err)
			return nil
		}
		if err := ctx.Err(); err != nil {
			_ = adapter.Close()
			return err
		}

		m, getErr := adapter.GetSessionByName(sessionName)
		if getErr == nil && m != nil {
			enrichErr := enrichManifestWithAgyConversation(m)
			if enrichErr == nil {
				if err := ctx.Err(); err != nil {
					_ = adapter.Close()
					return err
				}
				persistErr := persistAssociatedManifest(adapter, m)
				if persistErr == nil {
					_ = adapter.Close()
					return nil
				}
				debug.Log("AGY association retry %d/%d: failed to persist metadata: %v", attempt, attempts, persistErr)
			} else {
				debug.Log("AGY association retry %d/%d: failed to discover AGY conversation metadata: %v", attempt, attempts, enrichErr)
			}
		} else {
			debug.Log("AGY association retry %d/%d: session %q not found in Dolt: %v", attempt, attempts, sessionName, getErr)
		}
		_ = adapter.Close()
		if attempt < attempts {
			if err := waitForAgyAssociationRetryDelay(ctx, delay); err != nil {
				return err
			}
		}
	}
	return nil
}

func waitForAgyAssociationRetryDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func enrichAssociatedAgyManifest(adapter *dolt.Adapter, m *manifest.Manifest) {
	if adapter == nil || m == nil {
		return
	}
	if err := enrichManifestWithAgyConversation(m); err != nil {
		debug.Log("AGY association metadata not captured: %v", err)
	}
}
