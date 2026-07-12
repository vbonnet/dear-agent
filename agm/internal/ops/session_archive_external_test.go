package ops

import (
	"context"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestArchiveSession_ArchivesExternalRepresentation(t *testing.T) {
	m := newManifest("id-1", "codex-worker", "~/project")
	m.Harness = "codex-cli"
	ctx := testCtx([]*manifest.Manifest{m})

	called := false
	ctx.ExternalSessionArchiver = externalSessionArchiverFunc(func(_ context.Context, got *manifest.Manifest) []ExternalArchiveOutcome {
		called = true
		if got.SessionID != m.SessionID {
			t.Errorf("archived external manifest = %q, want %q", got.SessionID, m.SessionID)
		}
		return []ExternalArchiveOutcome{{Provider: "codex", Status: ExternalArchiveArchived, Target: "thread-1"}}
	})

	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: m.SessionID})
	if err != nil {
		t.Fatalf("ArchiveSession() error = %v", err)
	}
	if !called {
		t.Fatal("external archive dispatcher was not called")
	}
	if len(result.ExternalArchives) != 1 {
		t.Fatalf("external outcomes = %#v, want one", result.ExternalArchives)
	}
	if got := result.ExternalArchives[0]; got.Status != ExternalArchiveArchived || got.Target != "thread-1" {
		t.Fatalf("external outcome = %#v", got)
	}
}

func TestArchiveSession_DryRunDoesNotArchiveExternalRepresentation(t *testing.T) {
	m := newManifest("id-1", "claude-worker", "~/project")
	ctx := testCtx([]*manifest.Manifest{m})
	ctx.DryRun = true
	ctx.ExternalSessionArchiver = externalSessionArchiverFunc(func(context.Context, *manifest.Manifest) []ExternalArchiveOutcome {
		t.Fatal("external archive dispatcher must not run during dry run")
		return nil
	})

	result, err := ArchiveSession(ctx, &ArchiveSessionRequest{Identifier: m.SessionID})
	if err != nil {
		t.Fatalf("ArchiveSession() error = %v", err)
	}
	if len(result.ExternalArchives) != 0 {
		t.Fatalf("dry-run external outcomes = %#v, want none", result.ExternalArchives)
	}
}
