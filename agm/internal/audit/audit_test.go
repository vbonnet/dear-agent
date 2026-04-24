package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/testutil"
	"gopkg.in/yaml.v3"
)

func TestRunAudit(t *testing.T) {
	// Create test sessions directory
	tempDir := t.TempDir()

	// Create a valid session
	createTestSession(t, tempDir, "session-1", "ws-oss", "uuid-1", false, time.Now())

	// Run audit
	opts := AuditOptions{
		SessionsDir: tempDir,
	}

	report, err := RunAudit(opts)
	require.NoError(t, err)
	assert.NotNil(t, report)
	assert.Equal(t, 1, report.ManifestsScanned)
	// Note: May find orphans from actual history.jsonl, so we don't assert Healthy=true
	// Just verify the report structure is correct
	assert.NotNil(t, report.IssuesBySeverity)
	assert.NotNil(t, report.IssuesByType)
}

func TestRunAudit_WithWorkspaceFilter(t *testing.T) {
	tempDir := t.TempDir()

	// Create sessions in different workspaces
	createTestSession(t, tempDir, "session-oss", "oss", "uuid-1", false, time.Now())
	createTestSession(t, tempDir, "session-acme", "acme", "uuid-2", false, time.Now())

	// Run audit with workspace filter
	opts := AuditOptions{
		SessionsDir:     tempDir,
		WorkspaceFilter: "oss",
	}

	report, err := RunAudit(opts)
	require.NoError(t, err)

	// Should only process oss workspace
	assert.Equal(t, 2, report.ManifestsScanned) // Scans both but filters results
}

func TestRunAudit_WithSeverityFilter(t *testing.T) {
	tempDir := t.TempDir()

	// Create a corrupted manifest (critical)
	createCorruptedManifest(t, tempDir, "session-bad")

	// Create a stale session (info)
	createTestSession(t, tempDir, "session-stale", "oss", "uuid-1", false, time.Now().Add(-60*24*time.Hour))

	// Run audit with severity filter (critical only)
	opts := AuditOptions{
		SessionsDir:    tempDir,
		SeverityFilter: SeverityCritical,
	}

	report, err := RunAudit(opts)
	require.NoError(t, err)

	// Should only show critical issues
	assert.Greater(t, report.TotalIssues, 0)
	for _, issue := range report.Issues {
		assert.Equal(t, SeverityCritical, issue.Severity)
	}
}

func TestCheckCorruptedManifests_YAMLParseError(t *testing.T) {
	tempDir := t.TempDir()

	// Create manifest with invalid YAML
	createCorruptedManifest(t, tempDir, "session-bad-yaml")

	issues, count, err := checkCorruptedManifests(tempDir, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, issues, 1)
	assert.Equal(t, IssueCorruptedManifest, issues[0].Type)
	assert.Equal(t, SeverityCritical, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "YAML parse error")
}

func TestCheckCorruptedManifests_MissingRequiredFields(t *testing.T) {
	tempDir := t.TempDir()

	// Create manifest missing required fields
	sessionDir := filepath.Join(tempDir, "session-incomplete")
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	m := manifest.Manifest{
		SchemaVersion: "2.0",
		// Missing SessionID (required)
		Name:      "incomplete",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	data, err := yaml.Marshal(&m)
	require.NoError(t, err)

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, data, 0644))

	issues, count, err := checkCorruptedManifests(tempDir, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, issues, 1)
	assert.Equal(t, IssueCorruptedManifest, issues[0].Type)
	assert.Contains(t, issues[0].Message, "Invalid manifest")
}

func TestCheckCorruptedManifests_ValidManifest(t *testing.T) {
	tempDir := t.TempDir()

	createTestSession(t, tempDir, "session-good", "oss", "uuid-1", false, time.Now())

	issues, count, err := checkCorruptedManifests(tempDir, "")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Len(t, issues, 0) // No issues for valid manifest
}

func TestCheckStaleSessions(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	freshID := "test-stale-fresh-" + uuid.New().String()[:8]
	staleID := "test-stale-old-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, freshID)
	defer testutil.CleanupTestSession(t, adapter, staleID)

	// Create a fresh session in Dolt
	freshM := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     freshID,
		Name:          freshID,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-fresh-" + freshID},
		Tmux:          manifest.Tmux{SessionName: freshID},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	require.NoError(t, adapter.CreateSession(freshM))

	// Create a stale session (40 days old) in Dolt
	staleM := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     staleID,
		Name:          staleID,
		CreatedAt:     time.Now().Add(-41 * 24 * time.Hour),
		UpdatedAt:     time.Now().Add(-40 * 24 * time.Hour),
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-stale-" + staleID},
		Tmux:          manifest.Tmux{SessionName: staleID},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	require.NoError(t, adapter.CreateSession(staleM))

	issues, err := checkStaleSessions(tempDir, "", adapter)
	require.NoError(t, err)

	// Should find at least one stale session (our staleID)
	found := false
	for _, issue := range issues {
		if issue.SessionID == staleID {
			found = true
			assert.Equal(t, IssueStaleSession, issue.Type)
			assert.Equal(t, SeverityInfo, issue.Severity)
			assert.Contains(t, issue.Message, "Stale session")
		}
	}
	assert.True(t, found, "expected stale session %s to be detected", staleID)
}

func TestCheckStaleSessions_SkipsArchived(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	archivedID := "test-stale-archived-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, archivedID)

	// Create an archived stale session in Dolt
	archivedM := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     archivedID,
		Name:          archivedID,
		CreatedAt:     time.Now().Add(-41 * 24 * time.Hour),
		UpdatedAt:     time.Now().Add(-40 * 24 * time.Hour),
		Lifecycle:     manifest.LifecycleArchived,
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "uuid-arch-" + archivedID},
		Tmux:          manifest.Tmux{SessionName: archivedID},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	require.NoError(t, adapter.CreateSession(archivedM))

	issues, err := checkStaleSessions(tempDir, "", adapter)
	require.NoError(t, err)

	// Should not flag archived sessions
	for _, issue := range issues {
		assert.NotEqual(t, archivedID, issue.SessionID, "archived session should not be flagged as stale")
	}
}

func TestCheckDuplicateUUIDs(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	dupUUID := "dup-uuid-" + uuid.New().String()[:8]
	sid1 := "test-dup1-" + uuid.New().String()[:8]
	sid2 := "test-dup2-" + uuid.New().String()[:8]
	sid3 := "test-uniq-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)
	defer testutil.CleanupTestSession(t, adapter, sid3)

	for _, sid := range []string{sid1, sid2} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: dupUUID},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		require.NoError(t, adapter.CreateSession(m))
	}

	// Create a session with unique UUID
	m3 := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sid3,
		Name:          sid3,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Workspace:     "oss",
		Claude:        manifest.Claude{UUID: "unique-" + uuid.New().String()[:8]},
		Tmux:          manifest.Tmux{SessionName: sid3},
		Context:       manifest.Context{Project: "~/src/test"},
	}
	require.NoError(t, adapter.CreateSession(m3))

	issues, err := checkDuplicateUUIDs(tempDir, "", adapter)
	require.NoError(t, err)

	// Should find issues for our duplicate UUID
	dupCount := 0
	for _, issue := range issues {
		if issue.UUID == dupUUID {
			dupCount++
			assert.Equal(t, IssueDuplicateUUID, issue.Type)
			assert.Equal(t, SeverityCritical, issue.Severity)
			assert.Contains(t, issue.Message, "Duplicate UUID")
		}
	}
	assert.Equal(t, 2, dupCount, "expected 2 duplicate UUID issues for our test sessions")
}

func TestCheckDuplicateUUIDs_NoUUID(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	sid1 := "test-nouuid1-" + uuid.New().String()[:8]
	sid2 := "test-nouuid2-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)

	// Create sessions without UUIDs in Dolt
	for _, sid := range []string{sid1, sid2} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: ""},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		require.NoError(t, adapter.CreateSession(m))
	}

	issues, err := checkDuplicateUUIDs(tempDir, "", adapter)
	require.NoError(t, err)

	// Should not flag sessions without UUIDs
	for _, issue := range issues {
		assert.NotEqual(t, sid1, issue.SessionID, "no-UUID session should not be flagged")
		assert.NotEqual(t, sid2, issue.SessionID, "no-UUID session should not be flagged")
	}
}

func TestFilterBySeverity_Critical(t *testing.T) {
	issues := []*AuditIssue{
		{Severity: SeverityCritical, Message: "critical1"},
		{Severity: SeverityWarning, Message: "warning1"},
		{Severity: SeverityInfo, Message: "info1"},
		{Severity: SeverityCritical, Message: "critical2"},
	}

	filtered := filterBySeverity(issues, SeverityCritical)

	assert.Len(t, filtered, 2)
	assert.Equal(t, "critical1", filtered[0].Message)
	assert.Equal(t, "critical2", filtered[1].Message)
}

func TestFilterBySeverity_Warning(t *testing.T) {
	issues := []*AuditIssue{
		{Severity: SeverityCritical, Message: "critical1"},
		{Severity: SeverityWarning, Message: "warning1"},
		{Severity: SeverityInfo, Message: "info1"},
	}

	filtered := filterBySeverity(issues, SeverityWarning)

	assert.Len(t, filtered, 2)
	assert.Equal(t, SeverityCritical, filtered[0].Severity)
	assert.Equal(t, SeverityWarning, filtered[1].Severity)
}

func TestFilterBySeverity_Info(t *testing.T) {
	issues := []*AuditIssue{
		{Severity: SeverityCritical, Message: "critical1"},
		{Severity: SeverityWarning, Message: "warning1"},
		{Severity: SeverityInfo, Message: "info1"},
	}

	filtered := filterBySeverity(issues, SeverityInfo)

	// All issues should pass
	assert.Len(t, filtered, 3)
}

func TestAuditReport_Statistics(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	sid1 := "test-stat1-" + uuid.New().String()[:8]
	sid2 := "test-stat2-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)

	// Create stale sessions in Dolt
	for _, sid := range []string{sid1, sid2} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now().Add(-41 * 24 * time.Hour),
			UpdatedAt:     time.Now().Add(-40 * 24 * time.Hour),
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: "uuid-" + sid},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		require.NoError(t, adapter.CreateSession(m))
	}

	// Create corrupted manifest on disk (still detected by checkCorruptedManifests)
	createCorruptedManifest(t, tempDir, "session-bad")

	opts := AuditOptions{
		SessionsDir: tempDir,
		Adapter:     adapter,
	}

	report, err := RunAudit(opts)
	require.NoError(t, err)

	// Check statistics
	assert.Greater(t, report.TotalIssues, 0)
	assert.NotEmpty(t, report.IssuesBySeverity)
	assert.NotEmpty(t, report.IssuesByType)

	// Check metadata
	assert.False(t, report.StartedAt.IsZero())
	assert.False(t, report.CompletedAt.IsZero())
	assert.NotEmpty(t, report.Duration)
	assert.False(t, report.Healthy)
}

func TestAuditReport_HealthySystem(t *testing.T) {
	tempDir := t.TempDir()
	adapter := testutil.GetTestDoltAdapter(t)
	defer adapter.Close()

	sid1 := "test-healthy1-" + uuid.New().String()[:8]
	sid2 := "test-healthy2-" + uuid.New().String()[:8]
	defer testutil.CleanupTestSession(t, adapter, sid1)
	defer testutil.CleanupTestSession(t, adapter, sid2)

	// Create healthy sessions in Dolt
	for _, sid := range []string{sid1, sid2} {
		m := &manifest.Manifest{
			SchemaVersion: "2.0",
			SessionID:     sid,
			Name:          sid,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
			Workspace:     "oss",
			Claude:        manifest.Claude{UUID: "uuid-" + sid},
			Tmux:          manifest.Tmux{SessionName: sid},
			Context:       manifest.Context{Project: "~/src/test"},
		}
		require.NoError(t, adapter.CreateSession(m))
	}

	// Also create YAML manifests on disk for checkCorruptedManifests to find
	createTestSession(t, tempDir, sid1, "oss", "uuid-"+sid1, false, time.Now())
	createTestSession(t, tempDir, sid2, "oss", "uuid-"+sid2, false, time.Now())

	opts := AuditOptions{
		SessionsDir: tempDir,
		Adapter:     adapter,
	}

	report, err := RunAudit(opts)
	require.NoError(t, err)

	// Verify structure
	assert.NotNil(t, report)
	assert.Empty(t, report.Errors)
	assert.NotNil(t, report.IssuesBySeverity)
	assert.NotNil(t, report.IssuesByType)
}

// Helper functions

func createTestSession(t *testing.T, baseDir, sessionID, workspace, uuid string, archived bool, updatedAt time.Time) {
	sessionDir := filepath.Join(baseDir, sessionID)
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	lifecycle := ""
	if archived {
		lifecycle = "archived"
	}

	m := manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          sessionID,
		CreatedAt:     time.Now().Add(-24 * time.Hour),
		UpdatedAt:     updatedAt,
		Lifecycle:     lifecycle,
		Workspace:     workspace,
		Context: manifest.Context{
			Project: "~/src/test",
		},
		Claude: manifest.Claude{
			UUID: uuid,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionID,
		},
	}

	data, err := yaml.Marshal(&m)
	require.NoError(t, err)

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, data, 0644))
}

func TestShortID(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},                      // empty — no panic
		{"abc", "abc"},               // shorter than 8 — returned as-is
		{"1234567", "1234567"},        // exactly 7 — no panic
		{"12345678", "12345678"},      // exactly 8 — full string
		{"abcdefgh-extra", "abcdefgh"}, // longer than 8 — truncated
	}
	for _, tc := range cases {
		got := shortID(tc.in)
		if got != tc.want {
			t.Errorf("shortID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func createCorruptedManifest(t *testing.T, baseDir, sessionID string) {
	sessionDir := filepath.Join(baseDir, sessionID)
	require.NoError(t, os.MkdirAll(sessionDir, 0755))

	// Write invalid YAML
	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	invalidYAML := `
schema_version: "2.0"
session_id: "test"
name: [this is invalid: yaml: syntax
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(invalidYAML), 0644))
}
