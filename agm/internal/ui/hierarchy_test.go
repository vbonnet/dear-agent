package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/db"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

// mockTmuxInterface implements session.TmuxInterface for testing
type mockTmuxInterface struct {
	sessions map[string]bool // Map of session names to active status
}

func (m *mockTmuxInterface) ListSessions() ([]string, error) {
	var sessions []string
	for name, active := range m.sessions {
		if active {
			sessions = append(sessions, name)
		}
	}
	return sessions, nil
}

func (m *mockTmuxInterface) ListSessionsWithInfo() ([]session.SessionInfo, error) {
	var sessions []session.SessionInfo
	for name, active := range m.sessions {
		if active {
			sessions = append(sessions, session.SessionInfo{
				Name:            name,
				AttachedClients: 0,
			})
		}
	}
	return sessions, nil
}

func (m *mockTmuxInterface) HasSession(name string) (bool, error) {
	return m.sessions[name], nil
}

func (m *mockTmuxInterface) GetSessionInfo(name string) (*session.SessionInfo, error) {
	if m.sessions[name] {
		return &session.SessionInfo{
			Name:            name,
			AttachedClients: 0,
		}, nil
	}
	return nil, nil
}

func (m *mockTmuxInterface) SessionInfo(name string) (*session.SessionInfo, error) {
	return m.GetSessionInfo(name)
}

func (m *mockTmuxInterface) CreateSession(name, workingDir string) error {
	m.sessions[name] = true
	return nil
}

func (m *mockTmuxInterface) ListClients(sessionName string) ([]session.ClientInfo, error) {
	// Return empty list for testing
	return []session.ClientInfo{}, nil
}

func (m *mockTmuxInterface) KillSession(name string) error {
	delete(m.sessions, name)
	return nil
}

func (m *mockTmuxInterface) AttachSession(name string) error {
	return nil
}

func (m *mockTmuxInterface) NewSession(name string, opts ...interface{}) error {
	m.sessions[name] = true
	return nil
}

func (m *mockTmuxInterface) SendKeys(target string, keys string) error {
	return nil
}

func (m *mockTmuxInterface) GetWindowCount(name string) (int, error) {
	return 1, nil
}

// Helper function to create a test manifest
func createTestManifest(sessionID, name string) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID:     sessionID,
		Name:          name,
		SchemaVersion: "3.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "~/test-project",
			Purpose: "Test session",
			Tags:    []string{"test"},
		},
		Claude: manifest.Claude{
			UUID: sessionID,
		},
		Tmux: manifest.Tmux{
			SessionName: name,
		},
	}
}

// TestFormatTableWithHierarchy_NoSessions tests rendering with no sessions
func TestFormatTableWithHierarchy_NoSessions(t *testing.T) {
	tmuxMock := &mockTmuxInterface{sessions: make(map[string]bool)}
	output := FormatTableWithHierarchy([]*db.SessionNode{}, tmuxMock)

	if !strings.Contains(output, "No sessions found") {
		t.Errorf("Expected 'No sessions found' in output, got: %s", output)
	}
}

// TestFormatTableWithHierarchy_SingleRootSession tests rendering with one root session
func TestFormatTableWithHierarchy_SingleRootSession(t *testing.T) {
	tmuxMock := &mockTmuxInterface{sessions: map[string]bool{"root-session": true}}

	rootSession := createTestManifest("root-id", "root-session")

	nodes := []*db.SessionNode{
		{
			Session:  rootSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	output := FormatTableWithHierarchy(nodes, tmuxMock)

	// Check that the session name appears
	if !strings.Contains(output, "root-session") {
		t.Errorf("Expected session name 'root-session' in output, got: %s", output)
	}

	// Check that overview header exists
	if !strings.Contains(output, "Sessions Overview") {
		t.Errorf("Expected 'Sessions Overview' header in output")
	}

	// Check that total count is correct
	if !strings.Contains(output, "(1 total)") {
		t.Errorf("Expected '(1 total)' in output, got: %s", output)
	}
}

// TestFormatTableWithHierarchy_ParentWithChildren tests rendering with parent-child relationships
func TestFormatTableWithHierarchy_ParentWithChildren(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"parent-session": true,
			"child-1":        false,
			"child-2":        true,
		},
	}

	parentSession := createTestManifest("parent-id", "parent-session")
	child1Session := createTestManifest("child-1-id", "child-1")
	child2Session := createTestManifest("child-2-id", "child-2")

	nodes := []*db.SessionNode{
		{
			Session: parentSession,
			Depth:   0,
			Children: []*db.SessionNode{
				{
					Session:  child1Session,
					Depth:    1,
					Children: []*db.SessionNode{},
					IsLast:   false,
				},
				{
					Session:  child2Session,
					Depth:    1,
					Children: []*db.SessionNode{},
					IsLast:   true,
				},
			},
			IsLast: true,
		},
	}

	output := FormatTableWithHierarchy(nodes, tmuxMock)

	// Check that all session names appear
	if !strings.Contains(output, "parent-session") {
		t.Errorf("Expected 'parent-session' in output")
	}
	if !strings.Contains(output, "child-1") {
		t.Errorf("Expected 'child-1' in output")
	}
	if !strings.Contains(output, "child-2") {
		t.Errorf("Expected 'child-2' in output")
	}

	// Check for tree characters (├─ or └─)
	if !strings.Contains(output, "├─") && !strings.Contains(output, "└─") {
		t.Errorf("Expected tree characters (├─ or └─) in output, got: %s", output)
	}

	// Check that total count is correct
	if !strings.Contains(output, "(3 total)") {
		t.Errorf("Expected '(3 total)' in output")
	}

	// Check that children count is displayed
	if !strings.Contains(output, "2 children") && !strings.Contains(output, "(2)") {
		t.Errorf("Expected children count in output")
	}
}

// TestFormatTableWithHierarchy_DeepHierarchy tests rendering with multiple levels
func TestFormatTableWithHierarchy_DeepHierarchy(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"root":       true,
			"child":      true,
			"grandchild": false,
		},
	}

	rootSession := createTestManifest("root-id", "root")
	childSession := createTestManifest("child-id", "child")
	grandchildSession := createTestManifest("grandchild-id", "grandchild")

	nodes := []*db.SessionNode{
		{
			Session: rootSession,
			Depth:   0,
			Children: []*db.SessionNode{
				{
					Session: childSession,
					Depth:   1,
					Children: []*db.SessionNode{
						{
							Session:  grandchildSession,
							Depth:    2,
							Children: []*db.SessionNode{},
							IsLast:   true,
						},
					},
					IsLast: true,
				},
			},
			IsLast: true,
		},
	}

	output := FormatTableWithHierarchy(nodes, tmuxMock)

	// Check that all session names appear
	expectedNames := []string{"root", "child", "grandchild"}
	for _, name := range expectedNames {
		if !strings.Contains(output, name) {
			t.Errorf("Expected '%s' in output", name)
		}
	}

	// Check for tree characters indicating hierarchy
	if !strings.Contains(output, "└─") {
		t.Errorf("Expected tree characters (└─) in output for hierarchical structure")
	}

	// Check that total count is correct
	if !strings.Contains(output, "(3 total)") {
		t.Errorf("Expected '(3 total)' in output")
	}
}

// TestFormatTableWithHierarchy_MultipleRoots tests rendering with multiple root sessions
func TestFormatTableWithHierarchy_MultipleRoots(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"root-1": true,
			"root-2": false,
		},
	}

	root1Session := createTestManifest("root-1-id", "root-1")
	root2Session := createTestManifest("root-2-id", "root-2")

	nodes := []*db.SessionNode{
		{
			Session:  root1Session,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   false,
		},
		{
			Session:  root2Session,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	output := FormatTableWithHierarchy(nodes, tmuxMock)

	// Check that both root sessions appear
	if !strings.Contains(output, "root-1") {
		t.Errorf("Expected 'root-1' in output")
	}
	if !strings.Contains(output, "root-2") {
		t.Errorf("Expected 'root-2' in output")
	}

	// Check that total count is correct
	if !strings.Contains(output, "(2 total)") {
		t.Errorf("Expected '(2 total)' in output")
	}
}

// TestFormatTableWithHierarchy_StatusIndicators tests that status symbols are rendered
func TestFormatTableWithHierarchy_StatusIndicators(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{
			"active-session":  true,
			"stopped-session": false,
		},
	}

	activeSession := createTestManifest("active-id", "active-session")
	stoppedSession := createTestManifest("stopped-id", "stopped-session")

	nodes := []*db.SessionNode{
		{
			Session:  activeSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   false,
		},
		{
			Session:  stoppedSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	output := FormatTableWithHierarchy(nodes, tmuxMock)

	// Check that status symbols appear (● for active, ○ for stopped, etc.)
	// Note: We can't check for specific symbols without mocking the terminal width,
	// but we can verify the session names are there
	if !strings.Contains(output, "active-session") {
		t.Errorf("Expected 'active-session' in output")
	}
	if !strings.Contains(output, "stopped-session") {
		t.Errorf("Expected 'stopped-session' in output")
	}
}

// TestRenderHierarchyMinimal tests the minimal layout rendering
func TestRenderHierarchyMinimal(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"test-session": true},
	}

	testSession := createTestManifest("test-id", "test-session")
	nodes := []*db.SessionNode{
		{
			Session:  testSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	// Compute statuses
	statuses := session.ComputeStatusBatchWithInfo([]*manifest.Manifest{testSession}, tmuxMock)

	// Mock activity map
	activityMap := map[string]string{"test-session": "5m ago"}

	output := renderHierarchyMinimal(nodes, statuses, activityMap)

	if !strings.Contains(output, "test-session") {
		t.Errorf("Expected 'test-session' in minimal output")
	}
}

// TestRenderHierarchyCompact tests the compact layout rendering
func TestRenderHierarchyCompact(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"test-session": true},
	}

	testSession := createTestManifest("test-id", "test-session")
	nodes := []*db.SessionNode{
		{
			Session:  testSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	// Compute statuses
	statuses := session.ComputeStatusBatchWithInfo([]*manifest.Manifest{testSession}, tmuxMock)

	// Mock activity map
	activityMap := map[string]string{"test-session": "5m ago"}

	output := renderHierarchyCompact(nodes, statuses, activityMap)

	if !strings.Contains(output, "test-session") {
		t.Errorf("Expected 'test-session' in compact output")
	}
}

// TestRenderHierarchyFull tests the full layout rendering
func TestRenderHierarchyFull(t *testing.T) {
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"test-session": true},
	}

	testSession := createTestManifest("test-id", "test-session")
	nodes := []*db.SessionNode{
		{
			Session:  testSession,
			Depth:    0,
			Children: []*db.SessionNode{},
			IsLast:   true,
		},
	}

	// Compute statuses
	statuses := session.ComputeStatusBatchWithInfo([]*manifest.Manifest{testSession}, tmuxMock)

	// Calculate widths
	widths := calculateMaxColumnWidthsFlat([]*manifest.Manifest{testSession}, false)

	// Mock activity map
	activityMap := map[string]string{"test-session": "5m ago"}

	output := renderHierarchyFull(nodes, statuses, false, widths, activityMap)

	if !strings.Contains(output, "test-session") {
		t.Errorf("Expected 'test-session' in full output")
	}
}

// TestColumnWidthsMinimumHeaderWidth tests that column widths are at least as wide as their headers
// This prevents misalignment when data is shorter than header text (e.g., "oss" vs "WORKSPACE")
func TestColumnWidthsMinimumHeaderWidth(t *testing.T) {
	// Create test sessions with very short workspace names
	session1 := createTestManifest("id-1", "session-1")
	session1.Workspace = "oss" // 3 chars, shorter than "WORKSPACE" (9 chars)
	session1.Harness = "c"     // 1 char, shorter than "HARNESS" header

	session2 := createTestManifest("id-2", "s2")
	session2.Workspace = "a"       // 1 char, shorter than "WORKSPACE" (9 chars)
	session2.Harness = "codex-cli" // harness value

	manifests := []*manifest.Manifest{session1, session2}

	// Calculate widths
	groups := map[string][]*manifest.Manifest{
		"active": manifests,
	}
	tmuxMock := &mockTmuxInterface{
		sessions: map[string]bool{"session-1": true, "s2": true},
	}
	statuses := session.ComputeStatusBatchWithInfo(manifests, tmuxMock)
	widths := calculateMaxColumnWidths(groups, statuses, false)

	// Verify minimum widths match header text
	if widths.name < 4 {
		t.Errorf("Expected name width >= 4 (\"NAME\"), got %d", widths.name)
	}
	if widths.uuid < 4 {
		t.Errorf("Expected uuid width >= 4 (\"UUID\"), got %d", widths.uuid)
	}
	if widths.workspace < 9 {
		t.Errorf("Expected workspace width >= 9 (\"WORKSPACE\"), got %d", widths.workspace)
	}
	if widths.agent < 5 {
		t.Errorf("Expected agent width >= 5 (\"AGENT\"), got %d", widths.agent)
	}
	if widths.project < 7 {
		t.Errorf("Expected project width >= 7 (\"PROJECT\"), got %d", widths.project)
	}
}
