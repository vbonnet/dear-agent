package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// describeAssociationStorage returns a truthful, human-readable description of
// where an associated session is persisted, for display after `agm session
// associate`.
//
// AGM stores sessions in Dolt (the source of truth). Sessions created before
// the Dolt migration may additionally have an on-disk manifest.yaml. We only
// report a manifest file path when that file actually exists on disk;
// otherwise we report the Dolt storage location. This prevents printing a path
// to a manifest file that was never written — the historical "bogus path" bug
// where the create path mkdir'd an empty session-<name>/ directory and printed
// "Manifest: <that empty path>" even though the data lived only in Dolt.
func describeAssociationStorage(m *manifest.Manifest, workspace, manifestPath string) string {
	if manifestPath != "" {
		if info, err := os.Stat(manifestPath); err == nil && !info.IsDir() {
			return fmt.Sprintf("Manifest: %s", manifestPath)
		}
	}

	if workspace == "" {
		workspace = m.Workspace
	}
	if workspace == "" {
		workspace = "default"
	}

	return fmt.Sprintf("Session ID: %s\nStorage:    Dolt (workspace: %s)", m.SessionID, workspace)
}
