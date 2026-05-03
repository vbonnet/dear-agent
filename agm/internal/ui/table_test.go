package ui

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

func TestSelectLayout(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		expected LayoutMode
	}{
		{"very narrow", 40, LayoutMinimal},
		{"narrow boundary", 79, LayoutMinimal},
		{"compact start", 80, LayoutCompact},
		{"compact mid", 90, LayoutCompact},
		{"compact boundary", 99, LayoutCompact},
		{"full start", 100, LayoutFull},
		{"wide terminal", 200, LayoutFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := selectLayout(tt.width)
			if got != tt.expected {
				t.Errorf("selectLayout(%d) = %d, want %d", tt.width, got, tt.expected)
			}
		})
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		maxLen   int
		expected string
	}{
		{"short path unchanged", "/tmp", 10, "/tmp"},
		{"exact length", "abcde", 5, "abcde"},
		{"truncated with ellipsis", "/home/user/very/long/path/to/project", 20, "...ong/path/to/project"}, // len("...") + 17 = 20
		{"empty path", "", 10, ""},
		{"maxLen 4", "abcdef", 4, "...f"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncatePath(tt.path, tt.maxLen)
			if len(tt.path) <= tt.maxLen {
				if got != tt.expected {
					t.Errorf("truncatePath(%q, %d) = %q, want %q", tt.path, tt.maxLen, got, tt.expected)
				}
			} else {
				// Truncated: starts with "..."
				if !strings.HasPrefix(got, "...") {
					t.Errorf("truncatePath(%q, %d) = %q, expected prefix '...'", tt.path, tt.maxLen, got)
				}
			}
		})
	}
}

func TestCompactPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot get home dir")
	}

	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{"home subdir", homeDir + "/projects/test", "~/projects/test"},
		{"home exact", homeDir, "~"},
		{"non-home path", "/tmp/test", "/tmp/test"},
		{"empty path", "", ""},
		{"root path", "/", "/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compactPath(tt.path)
			if got != tt.expected {
				t.Errorf("compactPath(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestExtractShortUUID(t *testing.T) {
	tests := []struct {
		name     string
		uuid     string
		expected string
	}{
		{"standard uuid", "abc12345-def6-7890-abcd-ef1234567890", "abc12345"},
		{"short uuid", "abc", "abc"},
		{"empty uuid", "", "-"},
		{"no dashes", "abcdef1234567890", "abcdef1234567890"},
		{"leading dash", "-abc", "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractShortUUID(tt.uuid)
			if got != tt.expected {
				t.Errorf("extractShortUUID(%q) = %q, want %q", tt.uuid, got, tt.expected)
			}
		})
	}
}

func TestFormatTime(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		contains string
	}{
		{"minutes ago", now.Add(-30 * time.Minute), "m ago"},
		{"hours ago", now.Add(-5 * time.Hour), "h ago"},
		{"days ago", now.Add(-3 * 24 * time.Hour), "d ago"},
		{"old date format", now.Add(-30 * 24 * time.Hour), "-"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTime(tt.input)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("formatTime() = %q, want it to contain %q", got, tt.contains)
			}
		})
	}
}

func TestFormatTimeCompact(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{"just now", now.Add(-10 * time.Second), "now"},
		{"minutes", now.Add(-5 * time.Minute), "5m ago"},
		{"hours", now.Add(-3 * time.Hour), "3h ago"},
		{"days", now.Add(-2 * 24 * time.Hour), "2d ago"},
		{"weeks", now.Add(-14 * 24 * time.Hour), "2w ago"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimeCompact(tt.input)
			if got != tt.expected {
				t.Errorf("formatTimeCompact() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFormatTimeCompact_OldDate(t *testing.T) {
	old := time.Now().Add(-60 * 24 * time.Hour)
	got := formatTimeCompact(old)
	// Should be formatted as "Jan 02"
	if !strings.Contains(got, " ") {
		t.Errorf("formatTimeCompact for old date = %q, expected 'Mon DD' format", got)
	}
}

func TestGroupByStatus(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"active-1": true,
			"active-2": true,
		},
	}

	m1 := createTestManifest("id-1", "active-1")
	m2 := createTestManifest("id-2", "active-2")
	m3 := createTestManifest("id-3", "stopped-1")
	m4 := createTestManifest("id-4", "archived-1")
	m4.Lifecycle = manifest.LifecycleArchived

	manifests := []*manifest.Manifest{m1, m2, m3, m4}
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmuxMock)
	groups := groupByStatus(manifests, statuses)

	// Active sessions (attached or detached)
	activeCount := len(groups["attached"]) + len(groups["detached"])
	if activeCount != 2 {
		t.Errorf("expected 2 active sessions, got %d", activeCount)
	}

	if len(groups["stopped"]) != 1 {
		t.Errorf("expected 1 stopped session, got %d", len(groups["stopped"]))
	}

	if len(groups["archived"]) != 1 {
		t.Errorf("expected 1 archived session, got %d", len(groups["archived"]))
	}
}

func TestSortGroups(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"zebra": true,
			"alpha": true,
		},
	}

	m1 := createTestManifest("id-1", "zebra")
	m2 := createTestManifest("id-2", "alpha")
	m3 := createTestManifest("id-3", "zeta-stopped")
	m4 := createTestManifest("id-4", "beta-stopped")

	manifests := []*manifest.Manifest{m1, m2, m3, m4}
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmuxMock)
	groups := groupByStatus(manifests, statuses)

	sortGroups(groups, statuses)

	// Stopped group should be alphabetically sorted
	if len(groups["stopped"]) >= 2 {
		if strings.ToLower(groups["stopped"][0].Name) > strings.ToLower(groups["stopped"][1].Name) {
			t.Errorf("stopped group not sorted: %s before %s",
				groups["stopped"][0].Name, groups["stopped"][1].Name)
		}
	}
}

func TestShouldShowTmuxColumn(t *testing.T) {
	m1 := createTestManifest("id-1", "session-1")
	m1.Tmux.SessionName = "session-1"

	m2 := createTestManifest("id-2", "session-2")
	m2.Tmux.SessionName = "different-tmux"

	// Currently always returns false
	if shouldShowTmuxColumn([]*manifest.Manifest{m1, m2}) {
		t.Error("shouldShowTmuxColumn should return false (disabled)")
	}

	if shouldShowTmuxColumn(nil) {
		t.Error("shouldShowTmuxColumn(nil) should return false")
	}

	if shouldShowTmuxColumn([]*manifest.Manifest{}) {
		t.Error("shouldShowTmuxColumn([]) should return false")
	}
}

func TestFormatJSON(t *testing.T) {
	m := createTestManifest("test-id", "test-session")
	manifests := []*manifest.Manifest{m}

	result, err := FormatJSON(manifests)
	if err != nil {
		t.Fatalf("FormatJSON returned error: %v", err)
	}

	// Should be valid JSON
	var parsed []interface{}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Errorf("FormatJSON output is not valid JSON: %v", err)
	}

	if len(parsed) != 1 {
		t.Errorf("expected 1 item in JSON array, got %d", len(parsed))
	}

	// Should contain session name
	if !strings.Contains(result, "test-session") {
		t.Error("FormatJSON output should contain session name")
	}
}

func TestFormatJSON_Empty(t *testing.T) {
	result, err := FormatJSON([]*manifest.Manifest{})
	if err != nil {
		t.Fatalf("FormatJSON returned error: %v", err)
	}
	if result != "[]" {
		t.Errorf("FormatJSON([]) = %q, want %q", result, "[]")
	}
}

func TestFormatTable_Empty(t *testing.T) {
	tmuxMock := &mockTmuxInterface{sessions: make(map[string]bool)}
	output := FormatTable(nil, tmuxMock)
	if !strings.Contains(output, "No sessions found") {
		t.Errorf("expected 'No sessions found' in output, got: %s", output)
	}
}

func TestFormatTable_WithSessions(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"my-session": true},
	}

	m := createTestManifest("id-1", "my-session")
	output := FormatTable([]*manifest.Manifest{m}, tmuxMock)

	if !strings.Contains(output, "Sessions Overview") {
		t.Error("expected 'Sessions Overview' header")
	}
	if !strings.Contains(output, "my-session") {
		t.Error("expected session name in output")
	}
}

func TestFormatTableLegacy_Empty(t *testing.T) {
	tmuxMock := &mockTmuxInterface{sessions: make(map[string]bool)}
	output := FormatTableLegacy(nil, tmuxMock)
	// Legacy format with nil should handle gracefully (empty manifests slice)
	// Header should still be present
	if !strings.Contains(output, "NAME") {
		t.Error("expected header in legacy output")
	}
}

func TestFormatTableLegacy_WithSessions(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"legacy-test": true},
	}

	m := createTestManifest("id-1", "legacy-test")
	m.Harness = "claude"
	output := FormatTableLegacy([]*manifest.Manifest{m}, tmuxMock)

	if !strings.Contains(output, "legacy-test") {
		t.Error("expected session name in legacy output")
	}
	if !strings.Contains(output, "claude") {
		t.Error("expected agent name in legacy output")
	}
}

func TestGetStatusSymbol(t *testing.T) {
	// Ensure global config is default (no screen reader)
	cfg := DefaultConfig()
	cfg.UI.ScreenReader = false
	SetGlobalConfig(cfg)
	t.Setenv("AGM_SCREEN_READER", "") // restored on test cleanup
	os.Unsetenv("AGM_SCREEN_READER")

	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"attached", "attached", "●"},
		{"detached", "detached", "◐"},
		{"stopped", "stopped", "○"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusSymbol(tt.status)
			if got != tt.expected {
				t.Errorf("getStatusSymbol(%q) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestGetStatusSymbol_ScreenReader(t *testing.T) {
	cfg := DefaultConfig()
	cfg.UI.ScreenReader = true
	SetGlobalConfig(cfg)
	defer func() {
		cfg.UI.ScreenReader = false
		SetGlobalConfig(cfg)
	}()

	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"attached", "attached", "[ATTACHED]"},
		{"detached", "detached", "[DETACHED]"},
		{"stopped", "stopped", "[STOPPED]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getStatusSymbol(tt.status)
			if got != tt.expected {
				t.Errorf("getStatusSymbol(%q) = %q, want %q", tt.status, got, tt.expected)
			}
		})
	}
}

func TestRenderGroupHeader(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		count    int
		contains string
	}{
		{"active header", "active", 3, "ACTIVE"},
		{"stopped header", "stopped", 2, "Stopped"},
		{"attached header", "attached", 1, "Attached"},
		{"detached header", "detached", 5, "Detached"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderGroupHeader(tt.status, tt.count)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("renderGroupHeader(%q, %d) = %q, want it to contain %q",
					tt.status, tt.count, got, tt.contains)
			}
		})
	}
}

func TestCalculateMaxColumnWidths(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"short": true, "a-longer-session-name": true},
	}

	m1 := createTestManifest("id-1", "short")
	m1.Harness = "claude"
	m2 := createTestManifest("id-2", "a-longer-session-name")
	m2.Harness = "gemini"

	manifests := []*manifest.Manifest{m1, m2}
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmuxMock)
	groups := groupByStatus(manifests, statuses)

	widths := calculateMaxColumnWidths(groups, statuses, false)

	// Name width should accommodate the longest name
	if widths.name < len("a-longer-session-name") {
		t.Errorf("name width %d should be >= %d", widths.name, len("a-longer-session-name"))
	}

	// Minimum widths enforced
	if widths.uuid < 4 {
		t.Errorf("uuid width %d should be >= 4", widths.uuid)
	}
	if widths.workspace < 9 {
		t.Errorf("workspace width %d should be >= 9", widths.workspace)
	}
	if widths.agent < 5 {
		t.Errorf("agent width %d should be >= 5", widths.agent)
	}
	if widths.project < 7 {
		t.Errorf("project width %d should be >= 7", widths.project)
	}
}
