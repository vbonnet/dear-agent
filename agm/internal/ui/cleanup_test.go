package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestFilterStopped(t *testing.T) {
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1"}, Status: "stopped"},
		{Manifest: &manifest.Manifest{Name: "s2"}, Status: "active"},
		{Manifest: &manifest.Manifest{Name: "s3"}, Status: "stopped"},
		{Manifest: &manifest.Manifest{Name: "s4"}, Status: "archived"},
	}

	got := filterStopped(sessions)
	if len(got) != 2 {
		t.Errorf("filterStopped() returned %d sessions, want 2", len(got))
	}
	for _, s := range got {
		if s.Status != "stopped" {
			t.Errorf("filterStopped() returned session with status %q", s.Status)
		}
	}
}

func TestFilterStopped_Empty(t *testing.T) {
	got := filterStopped(nil)
	if len(got) != 0 {
		t.Errorf("filterStopped(nil) = %d sessions, want 0", len(got))
	}

	got = filterStopped([]*Session{})
	if len(got) != 0 {
		t.Errorf("filterStopped([]) = %d sessions, want 0", len(got))
	}
}

func TestFilterStopped_NoneStopped(t *testing.T) {
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1"}, Status: "active"},
		{Manifest: &manifest.Manifest{Name: "s2"}, Status: "active"},
	}

	got := filterStopped(sessions)
	if len(got) != 0 {
		t.Errorf("filterStopped() = %d sessions, want 0 (none stopped)", len(got))
	}
}

func TestFilterArchived(t *testing.T) {
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1", Lifecycle: manifest.LifecycleArchived}, Status: "archived"},
		{Manifest: &manifest.Manifest{Name: "s2", Lifecycle: ""}, Status: "active"},
		{Manifest: &manifest.Manifest{Name: "s3", Lifecycle: manifest.LifecycleArchived}, Status: "archived"},
	}

	got := filterArchived(sessions)
	if len(got) != 2 {
		t.Errorf("filterArchived() returned %d sessions, want 2", len(got))
	}
	for _, s := range got {
		if s.Lifecycle != manifest.LifecycleArchived {
			t.Errorf("filterArchived() returned session with lifecycle %q", s.Lifecycle)
		}
	}
}

func TestFilterArchived_Empty(t *testing.T) {
	got := filterArchived(nil)
	if len(got) != 0 {
		t.Errorf("filterArchived(nil) = %d sessions, want 0", len(got))
	}
}

func TestFilterByAge(t *testing.T) {
	now := time.Now()
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "old"}, UpdatedAt: now.Add(-60 * 24 * time.Hour)},
		{Manifest: &manifest.Manifest{Name: "recent"}, UpdatedAt: now.Add(-5 * 24 * time.Hour)},
		{Manifest: &manifest.Manifest{Name: "very-old"}, UpdatedAt: now.Add(-120 * 24 * time.Hour)},
	}

	// Filter sessions older than 30 days
	got := filterByAge(sessions, 30)
	if len(got) != 2 {
		t.Errorf("filterByAge(30) = %d sessions, want 2", len(got))
	}

	// Verify only old sessions returned
	for _, s := range got {
		if s.Name == "recent" {
			t.Error("filterByAge should not include recent session")
		}
	}
}

func TestFilterByAge_ZeroDays(t *testing.T) {
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1"}, UpdatedAt: time.Now()},
		{Manifest: &manifest.Manifest{Name: "s2"}, UpdatedAt: time.Now()},
	}

	// Zero days should return all sessions
	got := filterByAge(sessions, 0)
	if len(got) != 2 {
		t.Errorf("filterByAge(0) = %d sessions, want 2 (all)", len(got))
	}
}

func TestFilterByAge_NegativeDays(t *testing.T) {
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1"}, UpdatedAt: time.Now()},
	}

	// Negative days should return all sessions
	got := filterByAge(sessions, -1)
	if len(got) != 1 {
		t.Errorf("filterByAge(-1) = %d sessions, want 1 (all)", len(got))
	}
}

func TestFilterByAge_Empty(t *testing.T) {
	got := filterByAge(nil, 30)
	if len(got) != 0 {
		t.Errorf("filterByAge(nil, 30) = %d sessions, want 0", len(got))
	}
}

func TestFilterByAge_AllRecent(t *testing.T) {
	now := time.Now()
	sessions := []*Session{
		{Manifest: &manifest.Manifest{Name: "s1"}, UpdatedAt: now.Add(-1 * 24 * time.Hour)},
		{Manifest: &manifest.Manifest{Name: "s2"}, UpdatedAt: now.Add(-2 * 24 * time.Hour)},
	}

	got := filterByAge(sessions, 30)
	if len(got) != 0 {
		t.Errorf("filterByAge(30) = %d sessions, want 0 (all recent)", len(got))
	}
}

func TestFormatCleanupOption(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		session      *Session
		wantContains []string
	}{
		{
			name: "basic session",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name: "test-session",
					Context: manifest.Context{
						Project: "/home/user/project",
					},
				},
				UpdatedAt: now.Add(-5 * time.Hour),
			},
			wantContains: []string{"test-session", "5h ago", "/home/user/project"},
		},
		{
			name: "long project path truncated",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name: "long-project",
					Context: manifest.Context{
						Project: "/home/user/very/long/path/to/deeply/nested/project/directory/src",
					},
				},
				UpdatedAt: now.Add(-2 * 24 * time.Hour),
			},
			wantContains: []string{"long-project", "2d ago", "..."},
		},
		{
			name: "empty project",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name:    "no-project",
					Context: manifest.Context{},
				},
				UpdatedAt: now.Add(-1 * time.Minute),
			},
			wantContains: []string{"no-project"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatCleanupOption(tt.session)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatCleanupOption() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestCleanupMultiSelect_NoEligible(t *testing.T) {
	cfg := DefaultConfig()

	// Recent sessions that don't meet threshold
	sessions := []*Session{
		{
			Manifest: &manifest.Manifest{Name: "s1"},
			Status:   "stopped",
			UpdatedAt: time.Now(),
		},
	}

	result, err := CleanupMultiSelect(sessions, cfg)
	if err != nil {
		t.Fatalf("CleanupMultiSelect returned error: %v", err)
	}
	if result == nil {
		t.Fatal("CleanupMultiSelect returned nil result")
	}
	if len(result.ToArchive) != 0 {
		t.Errorf("expected 0 ToArchive, got %d", len(result.ToArchive))
	}
	if len(result.ToDelete) != 0 {
		t.Errorf("expected 0 ToDelete, got %d", len(result.ToDelete))
	}
}

func TestCleanupMultiSelect_EmptySessions(t *testing.T) {
	cfg := DefaultConfig()

	result, err := CleanupMultiSelect([]*Session{}, cfg)
	if err != nil {
		t.Fatalf("CleanupMultiSelect returned error: %v", err)
	}
	if result == nil {
		t.Fatal("CleanupMultiSelect returned nil result")
	}
	if len(result.ToArchive) != 0 || len(result.ToDelete) != 0 {
		t.Error("expected empty result for empty sessions")
	}
}

func TestCleanupMultiSelect_NilSessions(t *testing.T) {
	cfg := DefaultConfig()

	result, err := CleanupMultiSelect(nil, cfg)
	if err != nil {
		t.Fatalf("CleanupMultiSelect returned error: %v", err)
	}
	if result == nil {
		t.Fatal("CleanupMultiSelect returned nil result")
	}
}
