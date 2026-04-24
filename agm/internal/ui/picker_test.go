package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestFormatRelativeTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{"zero time", time.Time{}, "never"},
		{"just now", now.Add(-10 * time.Second), "just now"},
		{"1 minute ago", now.Add(-1 * time.Minute), "1m ago"},
		{"30 minutes ago", now.Add(-30 * time.Minute), "30m ago"},
		{"59 minutes ago", now.Add(-59 * time.Minute), "59m ago"},
		{"1 hour ago", now.Add(-1 * time.Hour), "1h ago"},
		{"5 hours ago", now.Add(-5 * time.Hour), "5h ago"},
		{"23 hours ago", now.Add(-23 * time.Hour), "23h ago"},
		{"1 day ago", now.Add(-25 * time.Hour), "1d ago"},
		{"3 days ago", now.Add(-3 * 24 * time.Hour), "3d ago"},
		{"6 days ago", now.Add(-6 * 24 * time.Hour), "6d ago"},
		{"1 week ago", now.Add(-8 * 24 * time.Hour), "1w ago"},
		{"3 weeks ago", now.Add(-21 * 24 * time.Hour), "3w ago"},
		{"1 month ago", now.Add(-35 * 24 * time.Hour), "1mo ago"},
		{"6 months ago", now.Add(-180 * 24 * time.Hour), "6mo ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatRelativeTime(tt.input)
			if got != tt.expected {
				t.Errorf("formatRelativeTime() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatSessionOption(t *testing.T) {
	cfg := DefaultConfig()

	tests := []struct {
		name           string
		session        *Session
		showPaths      bool
		wantContains   []string
		wantNoContains []string
	}{
		{
			name: "basic session with status",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name: "test-session",
					Context: manifest.Context{
						Project: "/home/user/project",
					},
				},
				Status:    "active",
				UpdatedAt: time.Now().Add(-5 * time.Minute),
			},
			showPaths:    true,
			wantContains: []string{"test-session", "(active)", "5m ago"},
		},
		{
			name: "empty status shows unknown",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name:    "no-status",
					Context: manifest.Context{},
				},
				Status:    "",
				UpdatedAt: time.Now(),
			},
			showPaths:    false,
			wantContains: []string{"no-status", "(unknown)"},
		},
		{
			name: "long project path truncated",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name: "long-path",
					Context: manifest.Context{
						Project: "/home/user/very/long/path/to/some/deeply/nested/project/directory",
					},
				},
				Status:    "stopped",
				UpdatedAt: time.Now(),
			},
			showPaths:    true,
			wantContains: []string{"long-path", "(stopped)", "..."},
		},
		{
			name: "project paths hidden when disabled",
			session: &Session{
				Manifest: &manifest.Manifest{
					Name: "no-paths",
					Context: manifest.Context{
						Project: "/home/user/project",
					},
				},
				Status:    "active",
				UpdatedAt: time.Now(),
			},
			showPaths:      false,
			wantContains:   []string{"no-paths", "(active)"},
			wantNoContains: []string{"[/home"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localCfg := *cfg
			localCfg.UI.ShowProjectPaths = tt.showPaths
			got := formatSessionOption(tt.session, &localCfg)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatSessionOption() = %q, want it to contain %q", got, want)
				}
			}
			for _, noWant := range tt.wantNoContains {
				if strings.Contains(got, noWant) {
					t.Errorf("formatSessionOption() = %q, should NOT contain %q", got, noWant)
				}
			}
		})
	}
}

func TestFormatArchivedSessionOption(t *testing.T) {
	tests := []struct {
		name         string
		session      ArchivedSessionInfo
		wantContains []string
	}{
		{
			name: "full info",
			session: ArchivedSessionInfo{
				SessionID:  "abc-123",
				Name:       "archived-session",
				ArchivedAt: "2025-01-15",
				Tags:       []string{"feature", "auth"},
				Project:    "/home/user/project",
			},
			wantContains: []string{"archived-session", "2025-01-15", "feature, auth", "/home/user/project"},
		},
		{
			name: "minimal info",
			session: ArchivedSessionInfo{
				SessionID: "abc-123",
				Name:      "minimal",
			},
			wantContains: []string{"minimal"},
		},
		{
			name: "unknown archived date excluded",
			session: ArchivedSessionInfo{
				SessionID:  "abc-123",
				Name:       "unknown-date",
				ArchivedAt: "unknown",
			},
			wantContains: []string{"unknown-date"},
		},
		{
			name: "long project path truncated",
			session: ArchivedSessionInfo{
				SessionID: "abc-123",
				Name:      "truncated-project",
				Project:   "/home/user/very/long/path/to/some/deeply/nested/project/directory",
			},
			wantContains: []string{"truncated-project", "..."},
		},
		{
			name: "long tags truncated",
			session: ArchivedSessionInfo{
				SessionID: "abc-123",
				Name:      "long-tags",
				Tags:      []string{"very-long-tag-one", "very-long-tag-two", "very-long-tag-three"},
			},
			wantContains: []string{"long-tags", "..."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatArchivedSessionOption(tt.session)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("formatArchivedSessionOption() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestGetTheme(t *testing.T) {
	tests := []struct {
		name      string
		themeName string
	}{
		{"agm theme", "agm"},
		{"agm-light theme", "agm-light"},
		{"dracula theme", "dracula"},
		{"catppuccin theme", "catppuccin"},
		{"charm theme", "charm"},
		{"base theme", "base"},
		{"unknown falls back to agm", "nonexistent"},
		{"empty falls back to agm", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := getTheme(tt.themeName)
			if theme == nil {
				t.Errorf("getTheme(%q) returned nil", tt.themeName)
			}
		})
	}
}

func TestGetTheme_Exported(t *testing.T) {
	cfg := DefaultConfig()
	SetGlobalConfig(cfg)

	theme := GetTheme()
	if theme == nil {
		t.Error("GetTheme() returned nil")
	}
}

func TestSessionPicker_EmptySessions(t *testing.T) {
	cfg := DefaultConfig()
	_, err := SessionPicker(nil, cfg)
	if err == nil {
		t.Error("expected error for empty sessions")
	}
	if !strings.Contains(err.Error(), "no sessions available") {
		t.Errorf("expected 'no sessions available' error, got: %v", err)
	}
}

func TestArchivedSessionPicker_EmptySessions(t *testing.T) {
	_, err := ArchivedSessionPicker(nil)
	if err == nil {
		t.Error("expected error for empty sessions")
	}
	if !strings.Contains(err.Error(), "no sessions available") {
		t.Errorf("expected 'no sessions available' error, got: %v", err)
	}
}
