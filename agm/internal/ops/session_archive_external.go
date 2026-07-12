package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/claudeui"
	"github.com/vbonnet/dear-agent/agm/internal/codexarchive"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// ExternalArchiveStatus describes whether a harness-specific external session
// representation was changed after AGM archived the manifest.
type ExternalArchiveStatus string

const (
	// ExternalArchiveArchived means the provider representation was archived.
	ExternalArchiveArchived ExternalArchiveStatus = "archived"
	// ExternalArchiveAlreadyArchived means the provider representation was already archived.
	ExternalArchiveAlreadyArchived ExternalArchiveStatus = "already_archived"
	// ExternalArchiveNotPresent means no exact provider representation was found.
	ExternalArchiveNotPresent ExternalArchiveStatus = "not_present"
	// ExternalArchiveSkipped means the selected harness has no archive adapter.
	ExternalArchiveSkipped ExternalArchiveStatus = "skipped"
	// ExternalArchiveFailed means the provider archive attempt did not complete.
	ExternalArchiveFailed ExternalArchiveStatus = "failed"
)

// ExternalArchiveOutcome reports one harness-specific archive attempt. The
// shared lifecycle always returns an outcome so CLI, MCP, GC, and reaper
// callers can expose the same audit trail without provider-specific logic.
type ExternalArchiveOutcome struct {
	Provider string                `json:"provider"`
	Status   ExternalArchiveStatus `json:"status"`
	Target   string                `json:"target,omitempty"`
	Detail   string                `json:"detail,omitempty"`
}

// ExternalSessionArchiver is the harness-neutral extension point for archive
// side effects. Implementations must not change the durable AGM lifecycle
// record; ArchiveSession persists that record before this boundary is invoked.
type ExternalSessionArchiver interface {
	ArchiveExternalSession(context.Context, *manifest.Manifest) []ExternalArchiveOutcome
}

type externalSessionArchiverFunc func(context.Context, *manifest.Manifest) []ExternalArchiveOutcome

func (f externalSessionArchiverFunc) ArchiveExternalSession(ctx context.Context, m *manifest.Manifest) []ExternalArchiveOutcome {
	return f(ctx, m)
}

// ArchiveExternalSession archives the exact external representation associated
// with m. It contains the provider-specific extensions used by every AGM
// archive caller; unknown harnesses are explicitly skipped rather than guessed.
func ArchiveExternalSession(ctx context.Context, m *manifest.Manifest) []ExternalArchiveOutcome {
	if m == nil {
		return []ExternalArchiveOutcome{{Provider: "unknown", Status: ExternalArchiveSkipped, Detail: "nil manifest"}}
	}

	switch m.Harness {
	case "codex-cli":
		return []ExternalArchiveOutcome{archiveCodexExternalSession(ctx, m)}
	case "claude-code", "":
		return []ExternalArchiveOutcome{archiveClaudeExternalSession(m)}
	default:
		return []ExternalArchiveOutcome{{Provider: m.Harness, Status: ExternalArchiveSkipped, Detail: "no external archive adapter"}}
	}
}

func archiveCodexExternalSession(ctx context.Context, m *manifest.Manifest) ExternalArchiveOutcome {
	result, err := codexarchive.ArchiveManifest(ctx, m)
	if err != nil {
		return ExternalArchiveOutcome{Provider: "codex", Status: ExternalArchiveFailed, Detail: err.Error()}
	}
	if result == nil || result.Skipped {
		return ExternalArchiveOutcome{Provider: "codex", Status: ExternalArchiveSkipped}
	}
	if result.AlreadyArchived {
		return ExternalArchiveOutcome{Provider: "codex", Status: ExternalArchiveAlreadyArchived, Target: result.Target}
	}
	return ExternalArchiveOutcome{Provider: "codex", Status: ExternalArchiveArchived, Target: result.Target}
}

func archiveClaudeExternalSession(m *manifest.Manifest) ExternalArchiveOutcome {
	if m.Claude.UUID == "" {
		return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveNotPresent, Detail: "manifest has no Claude UUID"}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveFailed, Target: m.Claude.UUID, Detail: err.Error()}
	}

	sessions, loadErrs, err := claudeui.FindByCLISessionID(claudeui.DefaultStoreRoot(home), m.Claude.UUID)
	if err != nil {
		if errors.Is(err, claudeui.ErrStoreNotFound) {
			return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveNotPresent, Target: m.Claude.UUID}
		}
		return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveFailed, Target: m.Claude.UUID, Detail: err.Error()}
	}
	if len(sessions) == 0 {
		detail := "no matching Claude desktop session"
		if len(loadErrs) > 0 {
			detail = fmt.Sprintf("%s (%d unreadable or unrecognized store record(s) skipped)", detail, len(loadErrs))
		}
		return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveNotPresent, Target: m.Claude.UUID, Detail: detail}
	}

	backupDir := filepath.Join(home, ".agm", "backups", "claude-ui-sessions", time.Now().UTC().Format("20060102T150405Z"))
	changed := 0
	for _, s := range sessions {
		wasChanged, _, setErr := s.SetArchived(true, true, backupDir)
		if setErr != nil {
			return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveFailed, Target: m.Claude.UUID, Detail: setErr.Error()}
		}
		if wasChanged {
			changed++
		}
	}
	if changed == 0 {
		return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveAlreadyArchived, Target: m.Claude.UUID}
	}
	detail := fmt.Sprintf("archived %d Claude desktop record(s)", changed)
	if len(loadErrs) > 0 {
		detail = fmt.Sprintf("%s (%d unreadable or unrecognized store record(s) skipped)", detail, len(loadErrs))
	}
	return ExternalArchiveOutcome{Provider: "claude", Status: ExternalArchiveArchived, Target: m.Claude.UUID, Detail: detail}
}

func archiveExternalForContext(opCtx *OpContext, m *manifest.Manifest) []ExternalArchiveOutcome {
	archiver := opCtx.ExternalSessionArchiver
	if archiver == nil {
		archiver = externalSessionArchiverFunc(ArchiveExternalSession)
	}
	return archiver.ArchiveExternalSession(context.Background(), m)
}
