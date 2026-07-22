package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/readiness"
	"github.com/vbonnet/dear-agent/agm/internal/session"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	uuidpkg "github.com/vbonnet/dear-agent/agm/internal/uuid"
)

// associateSpawnedClaudeSession links a freshly-spawned Claude session to its
// AGM manifest and writes the readiness signal entirely from Go — with NO
// dependency on the /agm:agm-assoc plugin slash command resolving inside the
// spawned Claude TUI (ce-o1sg).
//
// Background: agm session new used to associate by sending /rename followed by
// /agm:agm-assoc into the live Claude pane (tmux.InitSequence). /agm:agm-assoc
// is a plugin slash command whose body just instructs the LLM to run
// `agm session associate <name> --create`. Routing a deterministic op through
// (plugin loaded → LLM follows steps → Bash allowed → not plan mode) is the
// fragility: in spawned / sandbox sessions the plugin is not loaded, so the
// slash command resolves to "Unknown command", association silently never runs,
// and the spawner only learns of it via a ready-file timeout (one per session).
//
// The tmux-name → AGM-session-UUID mapping already exists: createAndRegister-
// Manifest persisted it to Dolt before this runs. So the deterministic work is:
//
//  1. Best-effort backfill the Claude conversation UUID (so `agm session resume`
//     works immediately when history is already available). At spawn time the
//     UUID is usually NOT yet discoverable — Claude has not written its
//     conversation to ~/.claude/history.jsonl yet — so this is genuinely
//     best-effort and a miss is the expected, non-fatal case. The SessionStart
//     hook (agm-state-ready), which receives Claude's session_id
//     directly in its payload, backfills the UUID once Claude emits it. We never
//     block readiness on UUID detection.
//
//  2. Write the readiness ready-file UNCONDITIONALLY. This is the signal the
//     post-create flow blocks on; writing it from the spawner is what makes
//     readiness deterministic — it no longer depends on an in-session subprocess
//     ever running.
//
// All failures are non-fatal: the session is usable regardless, and downstream
// tooling (the SessionStart hook, `agm sync`) reconciles the UUID later. The
// manual recovery command remains `agm session associate <name> --create`.
func associateSpawnedClaudeSession(sessionName string) {
	debug.Phase("Deterministic Session Association")

	// Default manifest path used for the ready-file payload when we can't
	// resolve the canonical one (Dolt unavailable / manifest not found). It is
	// informational only; the ready-file's job is purely the readiness signal.
	manifestPath := filepath.Join(getSessionsDir(), sessionName, "manifest.yaml")

	adapter, err := getStorage()
	if err != nil {
		debug.Log("Deterministic association: storage unavailable (%v); writing ready-file only", err)
		writeAssociationReadyFile(sessionName, manifestPath)
		return
	}
	defer func() { _ = adapter.Close() }()

	m, resolvedPath, err := session.ResolveIdentifier(sessionName, getSessionsDir(), adapter)
	if err != nil {
		// The manifest is normally created by the shared ops lifecycle before
		// this runs; if it's somehow missing we still signal readiness so the
		// spawner doesn't hang, and let `agm sync` reconcile later.
		debug.Log("Deterministic association: manifest not found for %q (%v); writing ready-file anyway", sessionName, err)
		writeAssociationReadyFile(sessionName, manifestPath)
		return
	}
	if resolvedPath != "" {
		manifestPath = resolvedPath
	}

	backfillClaudeUUID(m, manifestPath, adapter)
	writeAssociationReadyFile(sessionName, manifestPath)
}

// backfillClaudeUUID makes a best-effort attempt to discover the Claude
// conversation UUID and persist it to the manifest. A miss is the expected,
// non-fatal case at spawn time (see associateSpawnedClaudeSession) — the
// SessionStart hook backfills the UUID once Claude emits its session_id.
func backfillClaudeUUID(m *manifest.Manifest, manifestPath string, adapter *dolt.Adapter) {
	if m.Claude.UUID != "" {
		debug.Log("Deterministic association: manifest already linked to Claude UUID %s", shortID(m.Claude.UUID))
		return
	}

	// Discover scans the AGM manifest, /rename history, and the projects dir.
	// Pass a search func that returns this manifest so Level 1 is satisfied.
	detectedUUID, err := uuidpkg.Discover(m.Tmux.SessionName, func(string) (*manifest.Manifest, error) {
		return m, nil
	}, false)
	if err != nil || detectedUUID == "" {
		debug.Log("Deterministic association: Claude UUID not yet discoverable (expected at spawn); SessionStart hook will backfill: %v", err)
		return
	}

	m.Claude.UUID = detectedUUID
	m.UpdatedAt = time.Now()
	if err := adapter.UpdateSession(m); err != nil {
		debug.Log("Deterministic association: failed to persist detected UUID: %v", err)
		return
	}
	_ = git.CommitManifest(manifestPath, "associate", m.Tmux.SessionName)
	debug.Log("Deterministic association: linked Claude UUID %s", shortID(detectedUUID))
}

// shortID returns the first 8 chars of an ID for log lines, guarding against
// unexpectedly short strings (corrupted/partial manifest state, or a discovery
// fallback that returns a non-canonical value) that would otherwise panic the
// spawner on the slice expression.
func shortID(id string) string {
	if len(id) < 8 {
		return id
	}
	return id[:8]
}

// writeAssociationReadyFile writes the readiness ready-file that the post-create
// flow waits on. This is the deterministic readiness signal — it depends on
// nothing inside the spawned Claude session.
func writeAssociationReadyFile(sessionName, manifestPath string) {
	if err := readiness.CreateReadyFile(sessionName, manifestPath); err != nil {
		debug.Log("Deterministic association: failed to write ready-file: %v", err)
		ui.PrintWarning(fmt.Sprintf("Failed to write readiness signal: %v", err))
		return
	}
	debug.Log("Deterministic association: ready-file written for session %s", sessionName)
}
