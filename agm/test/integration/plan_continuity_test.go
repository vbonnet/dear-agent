package integration

import (
	"os"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// End-to-end integration test for plan continuity workflow
// Tests the complete flow: planning session → execution session → resume preference

func getTestAdapter(t *testing.T) *dolt.Adapter {
	if os.Getenv("DOLT_TEST_INTEGRATION") == "" {
		t.Skip("Skipping integration test (set DOLT_TEST_INTEGRATION=1 to enable)")
	}

	// Set up test environment
	t.Setenv("WORKSPACE", "test")
	t.Setenv("DOLT_PORT", "3307")
	os.Unsetenv("DOLT_DATABASE") // Let it default to workspace name

	config, err := dolt.DefaultConfig()
	if err != nil {
		t.Fatalf("Failed to get default config: %v", err)
	}

	adapter, err := dolt.New(config)
	if err != nil {
		t.Fatalf("Failed to create adapter: %v", err)
	}

	// Apply migrations (includes migration 007 for parent_session_id)
	if err := adapter.ApplyMigrations(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	return adapter
}

func TestPlanExecuteResumeWorkflow(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Step 1: Create planning session
	t.Log("Step 1: Creating planning session")
	planningSession := &manifest.Manifest{
		SessionID:     "test-plan-" + timestamp,
		Name:          "open-viking", // User-provided name for planning session
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "~/src/project",
			Purpose: "Planning phase for open-viking feature",
			Tags:    []string{"planning", "feature"},
		},
		Claude: manifest.Claude{
			UUID: "593e6716-plan-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "open-viking-plan",
		},
	}

	if err := adapter.CreateSession(planningSession); err != nil {
		t.Fatalf("Failed to create planning session: %v", err)
	}
	defer adapter.DeleteSession(planningSession.SessionID)

	// Verify planning session created
	retrieved, err := adapter.GetSession(planningSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve planning session: %v", err)
	}
	if retrieved.Name != "open-viking" {
		t.Errorf("Expected planning session name 'open-viking', got '%s'", retrieved.Name)
	}

	// Step 2: Simulate "Clear Context and Execute Plan" - creates new session with different UUID
	t.Log("Step 2: Simulating execution session creation (Clear Context and Execute Plan)")
	// Wait 2 seconds to simulate timing gap
	time.Sleep(2 * time.Second)

	executionSession := &manifest.Manifest{
		SessionID:     "test-exec-" + timestamp,
		Name:          "Unknown", // Execution sessions start with name "Unknown"
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "~/src/project", // Same CWD as planning session
			Purpose: "Execution phase",
			Tags:    []string{"execution"},
		},
		Claude: manifest.Claude{
			UUID: "80c10f57-exec-uuid", // Different UUID
		},
		Tmux: manifest.Tmux{
			SessionName: "execution-session",
		},
	}

	if err := adapter.CreateSession(executionSession); err != nil {
		t.Fatalf("Failed to create execution session: %v", err)
	}
	defer adapter.DeleteSession(executionSession.SessionID)

	// Verify execution session has no parent initially
	if executionSession.ParentSessionID != nil {
		t.Error("Expected execution session to have no parent initially")
	}

	// Step 3: Run detection logic to find parent
	t.Log("Step 3: Detecting parent session")
	// Simulate the detect-plan-parent command logic
	// Search for sessions with same CWD, created 1-10 seconds before, with a name
	allSessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		t.Fatalf("Failed to list sessions: %v", err)
	}

	var detectedParent *manifest.Manifest
	for _, session := range allSessions {
		// Must have a name (not "Unknown")
		if session.Name == "" || session.Name == "Unknown" {
			continue
		}

		// Must match CWD
		if session.Context.Project != executionSession.Context.Project {
			continue
		}

		// Must be created before execution session
		timeDiff := executionSession.CreatedAt.Sub(session.CreatedAt)
		if timeDiff < 1*time.Second || timeDiff > 10*time.Second {
			continue
		}

		// Found a match
		detectedParent = session
		break
	}

	if detectedParent == nil {
		t.Fatal("Failed to detect parent session")
	}
	if detectedParent.SessionID != planningSession.SessionID {
		t.Errorf("Expected to detect planning session, got session: %s", detectedParent.SessionID)
	}

	// Step 4: Link execution session to parent
	t.Log("Step 4: Linking execution session to parent")
	executionSession.ParentSessionID = &detectedParent.SessionID

	// Simulate name inheritance (Unknown → parent-name-exec)
	executionSession.Name = detectedParent.Name + "-exec"

	if err := adapter.UpdateSession(executionSession); err != nil {
		t.Fatalf("Failed to update execution session: %v", err)
	}

	// Verify parent_session_id was set
	updated, err := adapter.GetSession(executionSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to retrieve updated execution session: %v", err)
	}
	if updated.ParentSessionID == nil || *updated.ParentSessionID != planningSession.SessionID {
		t.Error("Parent session ID not set correctly")
	}
	if updated.Name != "open-viking-exec" {
		t.Errorf("Expected name 'open-viking-exec', got '%s'", updated.Name)
	}

	// Step 5: Verify hierarchy methods work
	t.Log("Step 5: Verifying hierarchy methods")

	// GetParent should return planning session
	parent, err := adapter.GetParent(executionSession.SessionID)
	if err != nil {
		t.Fatalf("GetParent failed: %v", err)
	}
	if parent == nil {
		t.Fatal("Expected parent to be returned")
	}
	if parent.SessionID != planningSession.SessionID {
		t.Errorf("Expected parent ID '%s', got '%s'", planningSession.SessionID, parent.SessionID)
	}

	// GetChildren should return execution session
	children, err := adapter.GetChildren(planningSession.SessionID)
	if err != nil {
		t.Fatalf("GetChildren failed: %v", err)
	}
	if len(children) != 1 {
		t.Fatalf("Expected 1 child, got %d", len(children))
	}
	if children[0].SessionID != executionSession.SessionID {
		t.Errorf("Expected child ID '%s', got '%s'", executionSession.SessionID, children[0].SessionID)
	}

	// GetSessionTree should show correct depth
	tree, err := adapter.GetSessionTree(executionSession.SessionID)
	if err != nil {
		t.Fatalf("GetSessionTree failed: %v", err)
	}
	if tree.Depth != 1 {
		t.Errorf("Expected depth 1 for execution session, got %d", tree.Depth)
	}
	if tree.Parent == nil || tree.Parent.SessionID != planningSession.SessionID {
		t.Error("Expected parent in session tree")
	}

	// Step 6: Simulate resume preference logic
	t.Log("Step 6: Testing resume preference logic")
	// When user runs `agm session resume open-viking`, the resume logic should:
	// 1. Match by name "open-viking" → finds planning session
	// 2. Check if planning session has children
	// 3. Prefer most recent non-archived child (execution session)

	// Get children and find most recent non-archived child
	children, err = adapter.GetChildren(planningSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to get children for resume logic: %v", err)
	}

	var mostRecentChild *manifest.Manifest
	for _, child := range children {
		if child.Lifecycle != manifest.LifecycleArchived {
			if mostRecentChild == nil || child.UpdatedAt.After(mostRecentChild.UpdatedAt) {
				mostRecentChild = child
			}
		}
	}

	if mostRecentChild == nil {
		t.Fatal("Expected to find most recent child for resume")
	}
	if mostRecentChild.SessionID != executionSession.SessionID {
		t.Errorf("Expected resume to prefer execution session, got: %s", mostRecentChild.SessionID)
	}

	t.Log("✓ All workflow steps completed successfully")
	t.Logf("  Planning session: %s (name: %s)", planningSession.SessionID, planningSession.Name)
	t.Logf("  Execution session: %s (name: %s)", executionSession.SessionID, executionSession.Name)
	t.Logf("  Parent linked correctly: %v", executionSession.ParentSessionID != nil)
	t.Logf("  Name inherited: %s", executionSession.Name)
	t.Logf("  Resume would use: %s", mostRecentChild.SessionID)
}

func TestResumePreferenceWithArchivedChild(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create planning session
	planningSession := &manifest.Manifest{
		SessionID:     "test-archived-plan-" + timestamp,
		Name:          "archived-parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/archived",
		},
		Claude: manifest.Claude{
			UUID: "archived-plan-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "archived-plan-tmux",
		},
	}

	if err := adapter.CreateSession(planningSession); err != nil {
		t.Fatalf("Failed to create planning session: %v", err)
	}
	defer adapter.DeleteSession(planningSession.SessionID)

	time.Sleep(1 * time.Second)

	// Create archived child
	archivedChild := &manifest.Manifest{
		SessionID:       "test-archived-child1-" + timestamp,
		Name:            "archived-parent-exec-1",
		ParentSessionID: &planningSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Harness:         "claude-code",
		Lifecycle:       manifest.LifecycleArchived, // Archived
		Context: manifest.Context{
			Project: "/test/archived",
		},
		Claude: manifest.Claude{
			UUID: "archived-child1-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "archived-child1-tmux",
		},
	}

	if err := adapter.CreateSession(archivedChild); err != nil {
		t.Fatalf("Failed to create archived child: %v", err)
	}
	defer adapter.DeleteSession(archivedChild.SessionID)

	time.Sleep(1 * time.Second)

	// Create active child (no Lifecycle field means active)
	activeChild := &manifest.Manifest{
		SessionID:       "test-archived-child2-" + timestamp,
		Name:            "archived-parent-exec-2",
		ParentSessionID: &planningSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Harness:         "claude-code",
		// No Lifecycle field = active session
		Context: manifest.Context{
			Project: "/test/archived",
		},
		Claude: manifest.Claude{
			UUID: "archived-child2-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "archived-child2-tmux",
		},
	}

	if err := adapter.CreateSession(activeChild); err != nil {
		t.Fatalf("Failed to create active child: %v", err)
	}
	defer adapter.DeleteSession(activeChild.SessionID)

	// Resume logic should skip archived child and use active child
	children, err := adapter.GetChildren(planningSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to get children: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(children))
	}

	var mostRecentChild *manifest.Manifest
	for _, child := range children {
		if child.Lifecycle != manifest.LifecycleArchived {
			if mostRecentChild == nil || child.UpdatedAt.After(mostRecentChild.UpdatedAt) {
				mostRecentChild = child
			}
		}
	}

	if mostRecentChild == nil {
		t.Fatal("Expected to find active child")
	}
	if mostRecentChild.SessionID != activeChild.SessionID {
		t.Errorf("Expected resume to prefer active child, got: %s", mostRecentChild.SessionID)
	}
	if mostRecentChild.Lifecycle == manifest.LifecycleArchived {
		t.Error("Resume should not prefer archived child")
	}
}

func TestMultipleExecutionSessions(t *testing.T) {
	adapter := getTestAdapter(t)
	defer adapter.Close()

	timestamp := time.Now().Format("20060102-150405")

	// Create planning session
	planningSession := &manifest.Manifest{
		SessionID:     "test-multi-plan-" + timestamp,
		Name:          "multi-exec-parent",
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "/test/multi",
		},
		Claude: manifest.Claude{
			UUID: "multi-plan-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "multi-plan-tmux",
		},
	}

	if err := adapter.CreateSession(planningSession); err != nil {
		t.Fatalf("Failed to create planning session: %v", err)
	}
	defer adapter.DeleteSession(planningSession.SessionID)

	// Create first execution session
	time.Sleep(1 * time.Second)
	exec1 := &manifest.Manifest{
		SessionID:       "test-multi-exec1-" + timestamp,
		Name:            "multi-exec-parent-exec",
		ParentSessionID: &planningSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/multi",
		},
		Claude: manifest.Claude{
			UUID: "multi-exec1-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "multi-exec1-tmux",
		},
	}

	if err := adapter.CreateSession(exec1); err != nil {
		t.Fatalf("Failed to create exec1: %v", err)
	}
	defer adapter.DeleteSession(exec1.SessionID)

	// Create second execution session (more recent)
	time.Sleep(1 * time.Second)
	exec2 := &manifest.Manifest{
		SessionID:       "test-multi-exec2-" + timestamp,
		Name:            "multi-exec-parent-exec-2",
		ParentSessionID: &planningSession.SessionID,
		SchemaVersion:   "2.0",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now().Add(1 * time.Second), // More recent
		Harness:         "claude-code",
		Context: manifest.Context{
			Project: "/test/multi",
		},
		Claude: manifest.Claude{
			UUID: "multi-exec2-uuid",
		},
		Tmux: manifest.Tmux{
			SessionName: "multi-exec2-tmux",
		},
	}

	if err := adapter.CreateSession(exec2); err != nil {
		t.Fatalf("Failed to create exec2: %v", err)
	}
	defer adapter.DeleteSession(exec2.SessionID)

	// Resume logic should prefer most recent execution session
	children, err := adapter.GetChildren(planningSession.SessionID)
	if err != nil {
		t.Fatalf("Failed to get children: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("Expected 2 children, got %d", len(children))
	}

	var mostRecentChild *manifest.Manifest
	for _, child := range children {
		if child.Lifecycle != manifest.LifecycleArchived {
			if mostRecentChild == nil || child.UpdatedAt.After(mostRecentChild.UpdatedAt) {
				mostRecentChild = child
			}
		}
	}

	if mostRecentChild == nil {
		t.Fatal("Expected to find most recent child")
	}
	if mostRecentChild.SessionID != exec2.SessionID {
		t.Errorf("Expected resume to prefer exec2 (most recent), got: %s", mostRecentChild.SessionID)
	}
}
