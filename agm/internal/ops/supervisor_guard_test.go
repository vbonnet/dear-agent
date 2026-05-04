package ops

import (
	"errors"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestIsSupervisorSession(t *testing.T) {
	tests := []struct {
		name     string
		sessName string
		want     bool
	}{
		// orchestrator patterns
		{"exact orchestrator", "orchestrator", true},
		{"orchestrator-v3", "orchestrator-v3", true},
		{"meta-orchestrator", "meta-orchestrator", true},
		{"meta-orchestrator-v2", "meta-orchestrator-v2", true},
		{"my-orchestrator-session", "my-orchestrator-session", true},

		// overseer patterns
		{"exact overseer", "overseer", true},
		{"overseer-v12", "overseer-v12", true},
		{"my-overseer", "my-overseer", true},

		// meta- patterns
		{"meta-scheduler", "meta-scheduler", true},
		{"meta-planner", "meta-planner", true},

		// case insensitive
		{"ORCHESTRATOR uppercase", "ORCHESTRATOR", true},
		{"Overseer mixed", "Overseer-Main", true},
		{"META-test", "META-test", true},

		// non-supervisor sessions
		{"regular worker", "impl-feature-xyz", false},
		{"worker with numbers", "worker-123", false},
		{"random name", "fix-login-bug", false},
		{"metadata (no dash)", "metadata-processor", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsSupervisorSession(tt.sessName)
			if got != tt.want {
				t.Errorf("IsSupervisorSession(%q) = %v, want %v", tt.sessName, got, tt.want)
			}
		})
	}
}

func TestSupervisorPatterns(t *testing.T) {
	patterns := SupervisorPatterns()
	if len(patterns) == 0 {
		t.Fatal("SupervisorPatterns() returned empty slice")
	}

	// Verify expected patterns are present
	expected := map[string]bool{"orchestrator": false, "overseer": false, "meta-": false}
	for _, p := range patterns {
		if _, ok := expected[p]; ok {
			expected[p] = true
		}
	}
	for p, found := range expected {
		if !found {
			t.Errorf("expected pattern %q not found in SupervisorPatterns()", p)
		}
	}
}

func TestArchiveSession_SupervisorProtection(t *testing.T) {
	tests := []struct {
		name      string
		sessName  string
		force     bool
		wantError bool
		errorType string
	}{
		{"blocks orchestrator", "orchestrator-main", false, true, "archive/supervisor_protected"},
		{"blocks meta-orchestrator", "meta-orchestrator-v2", false, true, "archive/supervisor_protected"},
		{"blocks overseer", "overseer-v12", false, true, "archive/supervisor_protected"},
		{"blocks meta-scheduler", "meta-scheduler", false, true, "archive/supervisor_protected"},
		{"allows with force", "orchestrator-main", true, false, ""},
		{"allows regular session", "impl-feature-xyz", false, false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newManifest("id-"+tt.sessName, tt.sessName, "~/project")
			m.State = manifest.StateDone
			m.UpdatedAt = time.Now().Add(-48 * time.Hour)
			// Clear tmux session name to avoid tmux check
			m.Tmux.SessionName = ""

			ctx := testCtx([]*manifest.Manifest{m})

			_, err := ArchiveSession(ctx, &ArchiveSessionRequest{
				Identifier: tt.sessName,
				Force:      tt.force,
			})

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error for supervisor session %q, got nil", tt.sessName)
				}
				var opErr *OpError
				if !errors.As(err, &opErr) {
					t.Fatalf("expected *OpError, got %T: %v", err, err)
				}
				if opErr.Type != tt.errorType {
					t.Errorf("expected error type %q, got %q", tt.errorType, opErr.Type)
				}
			} else if err != nil {
				// Allow verification errors (e.g., directory not found) — we only
				// care that the supervisor guard didn't fire
				var opErr *OpError
				if errors.As(err, &opErr) && opErr.Type == "archive/supervisor_protected" {
					t.Fatalf("unexpected supervisor protection error for %q", tt.sessName)
				}
			}
		})
	}
}

func TestGC_SkipsSupervisorSessions(t *testing.T) {
	now := time.Now()
	old := now.Add(-48 * time.Hour)

	sessions := []*manifest.Manifest{
		gcManifest("id-1", "meta-scheduler", manifest.StateDone, old),
		gcManifest("id-2", "orchestrator-v3", manifest.StateDone, old),
		gcManifest("id-3", "overseer-v12", manifest.StateDone, old),
		gcManifest("id-4", "regular-worker", manifest.StateDone, old),
	}

	ctx := testCtx(sessions) // no active tmux

	result, err := GC(ctx, &GCRequest{Force: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only regular-worker should be archived
	if result.Archived != 1 {
		t.Errorf("expected 1 archived, got %d", result.Archived)
	}

	for _, s := range result.Sessions {
		switch s.Name {
		case "meta-scheduler", "orchestrator-v3", "overseer-v12":
			if s.Action != "skipped" {
				t.Errorf("%s should be skipped, got action=%s", s.Name, s.Action)
			}
			if s.Reason != GCSkipProtectedRole {
				t.Errorf("%s skip reason should be %s, got %s", s.Name, GCSkipProtectedRole, s.Reason)
			}
		case "regular-worker":
			if s.Action != "archived" {
				t.Errorf("regular-worker should be archived, got action=%s", s.Action)
			}
		}
	}
}
