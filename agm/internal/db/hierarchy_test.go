package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// createTestSessionWithParent creates a test session with an optional parent
func createTestSessionWithParent(sessionID, parentID string) *manifest.Manifest {
	now := time.Now()
	session := &manifest.Manifest{
		SchemaVersion: "2.0",
		SessionID:     sessionID,
		Name:          "Test Session " + sessionID,
		CreatedAt:     now,
		UpdatedAt:     now,
		Lifecycle:     "",
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: "test-project",
			Purpose: "testing",
			Tags:    []string{"test"},
			Notes:   "Test notes",
		},
		Claude: manifest.Claude{
			UUID: "claude-uuid-" + sessionID,
		},
		Tmux: manifest.Tmux{
			SessionName: "tmux-" + sessionID,
		},
	}
	return session
}

// insertSessionWithParent inserts a session with a specific parent_session_id
func insertSessionWithParent(t *testing.T, db *DB, session *manifest.Manifest, parentID *string) {
	query := `
		INSERT INTO sessions (
			session_id, name, schema_version, created_at, updated_at, lifecycle,
			harness, model, context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, engram_enabled, engram_query,
			engram_ids, engram_loaded_at, engram_count, parent_session_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var parentVal interface{}
	if parentID != nil && *parentID != "" {
		parentVal = *parentID
	} else {
		parentVal = nil
	}

	_, err := db.conn.Exec(query,
		session.SessionID,
		session.Name,
		session.SchemaVersion,
		session.CreatedAt,
		session.UpdatedAt,
		session.Lifecycle,
		session.Harness,
		session.Model,
		session.Context.Project,
		session.Context.Purpose,
		"[]", // context_tags
		session.Context.Notes,
		session.Claude.UUID,
		session.Tmux.SessionName,
		0,   // engram_enabled
		"",  // engram_query
		nil, // engram_ids
		nil, // engram_loaded_at
		0,   // engram_count
		parentVal,
	)
	require.NoError(t, err, "failed to insert session with parent")
}

// TestGetChildren tests the GetChildren function
func TestGetChildren(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("get children of root session with no children", func(t *testing.T) {
		parent := createTestSessionWithParent("parent-no-children", "")
		insertSessionWithParent(t, db, parent, nil)

		children, err := db.GetChildren("parent-no-children")
		require.NoError(t, err)
		assert.NotNil(t, children)
		assert.Len(t, children, 0)
	})

	t.Run("get children with single child", func(t *testing.T) {
		parent := createTestSessionWithParent("parent-one-child", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "parent-one-child"
		child := createTestSessionWithParent("child-1", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		children, err := db.GetChildren("parent-one-child")
		require.NoError(t, err)
		assert.Len(t, children, 1)
		assert.Equal(t, "child-1", children[0].SessionID)
		assert.Equal(t, "Test Session child-1", children[0].Name)
	})

	t.Run("get children with multiple children", func(t *testing.T) {
		parent := createTestSessionWithParent("parent-multi-child", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "parent-multi-child"
		child1 := createTestSessionWithParent("child-m1", parentID)
		time.Sleep(1 * time.Millisecond) // Ensure different created_at times
		child2 := createTestSessionWithParent("child-m2", parentID)
		time.Sleep(1 * time.Millisecond)
		child3 := createTestSessionWithParent("child-m3", parentID)

		insertSessionWithParent(t, db, child1, &parentID)
		insertSessionWithParent(t, db, child2, &parentID)
		insertSessionWithParent(t, db, child3, &parentID)

		children, err := db.GetChildren("parent-multi-child")
		require.NoError(t, err)
		assert.Len(t, children, 3)

		// Verify order (should be by created_at ASC)
		assert.Equal(t, "child-m1", children[0].SessionID)
		assert.Equal(t, "child-m2", children[1].SessionID)
		assert.Equal(t, "child-m3", children[2].SessionID)
	})

	t.Run("get children with empty session_id fails", func(t *testing.T) {
		_, err := db.GetChildren("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})

	t.Run("get children of nonexistent session returns empty", func(t *testing.T) {
		children, err := db.GetChildren("nonexistent-session")
		require.NoError(t, err)
		assert.NotNil(t, children)
		assert.Len(t, children, 0)
	})

	t.Run("grandchildren are not returned", func(t *testing.T) {
		// Create parent -> child -> grandchild hierarchy
		parent := createTestSessionWithParent("grandparent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "grandparent"
		child := createTestSessionWithParent("child-gc", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		childID := "child-gc"
		grandchild := createTestSessionWithParent("grandchild", childID)
		insertSessionWithParent(t, db, grandchild, &childID)

		// Getting children of grandparent should only return direct child
		children, err := db.GetChildren("grandparent")
		require.NoError(t, err)
		assert.Len(t, children, 1)
		assert.Equal(t, "child-gc", children[0].SessionID)
	})
}

// TestGetParent tests the GetParent function
func TestGetParent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("get parent of root session returns nil", func(t *testing.T) {
		root := createTestSessionWithParent("root-session", "")
		insertSessionWithParent(t, db, root, nil)

		parent, err := db.GetParent("root-session")
		require.NoError(t, err)
		assert.Nil(t, parent)
	})

	t.Run("get parent of child session", func(t *testing.T) {
		parent := createTestSessionWithParent("parent-session", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "parent-session"
		child := createTestSessionWithParent("child-session", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		retrievedParent, err := db.GetParent("child-session")
		require.NoError(t, err)
		assert.NotNil(t, retrievedParent)
		assert.Equal(t, "parent-session", retrievedParent.SessionID)
		assert.Equal(t, "Test Session parent-session", retrievedParent.Name)
	})

	t.Run("get parent with empty session_id fails", func(t *testing.T) {
		_, err := db.GetParent("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})

	t.Run("get parent of nonexistent session fails", func(t *testing.T) {
		_, err := db.GetParent("nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("get parent when parent was deleted", func(t *testing.T) {
		// Create parent and child
		parent := createTestSessionWithParent("parent-to-delete", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "parent-to-delete"
		child := createTestSessionWithParent("orphaned-child", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		// Delete parent (ON DELETE SET NULL should set parent_session_id to NULL)
		err := db.DeleteSession("parent-to-delete")
		require.NoError(t, err)

		// Getting parent should return nil (orphaned)
		retrievedParent, err := db.GetParent("orphaned-child")
		require.NoError(t, err)
		assert.Nil(t, retrievedParent)
	})

	t.Run("multi-level hierarchy parent retrieval", func(t *testing.T) {
		// Create grandparent -> parent -> child
		grandparent := createTestSessionWithParent("grandparent-p", "")
		insertSessionWithParent(t, db, grandparent, nil)

		grandparentID := "grandparent-p"
		parent := createTestSessionWithParent("parent-p", grandparentID)
		insertSessionWithParent(t, db, parent, &grandparentID)

		parentID := "parent-p"
		child := createTestSessionWithParent("child-p", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		// Get parent of child should return parent, not grandparent
		retrievedParent, err := db.GetParent("child-p")
		require.NoError(t, err)
		assert.NotNil(t, retrievedParent)
		assert.Equal(t, "parent-p", retrievedParent.SessionID)
	})
}

// TestGetSessionTree tests the GetSessionTree function
func TestGetSessionTree(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("get tree for root session with no children", func(t *testing.T) {
		root := createTestSessionWithParent("tree-root-alone", "")
		insertSessionWithParent(t, db, root, nil)

		tree, err := db.GetSessionTree("tree-root-alone")
		require.NoError(t, err)
		assert.NotNil(t, tree)
		assert.NotNil(t, tree.Root)
		assert.Equal(t, "tree-root-alone", tree.Root.SessionID)
		assert.Nil(t, tree.Parent)
		assert.Len(t, tree.Children, 0)
		assert.Equal(t, 0, tree.Depth)
	})

	t.Run("get tree for root session with children", func(t *testing.T) {
		root := createTestSessionWithParent("tree-root-with-kids", "")
		insertSessionWithParent(t, db, root, nil)

		rootID := "tree-root-with-kids"
		child1 := createTestSessionWithParent("tree-child-1", rootID)
		child2 := createTestSessionWithParent("tree-child-2", rootID)
		insertSessionWithParent(t, db, child1, &rootID)
		insertSessionWithParent(t, db, child2, &rootID)

		tree, err := db.GetSessionTree("tree-root-with-kids")
		require.NoError(t, err)
		assert.NotNil(t, tree)
		assert.Equal(t, "tree-root-with-kids", tree.Root.SessionID)
		assert.Nil(t, tree.Parent)
		assert.Len(t, tree.Children, 2)
		assert.Equal(t, 0, tree.Depth)
	})

	t.Run("get tree for child session", func(t *testing.T) {
		parent := createTestSessionWithParent("tree-parent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "tree-parent"
		child := createTestSessionWithParent("tree-child", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		childID := "tree-child"
		grandchild := createTestSessionWithParent("tree-grandchild", childID)
		insertSessionWithParent(t, db, grandchild, &childID)

		tree, err := db.GetSessionTree("tree-child")
		require.NoError(t, err)
		assert.NotNil(t, tree)
		assert.Equal(t, "tree-child", tree.Root.SessionID)
		assert.NotNil(t, tree.Parent)
		assert.Equal(t, "tree-parent", tree.Parent.SessionID)
		assert.Len(t, tree.Children, 1)
		assert.Equal(t, "tree-grandchild", tree.Children[0].SessionID)
		assert.Equal(t, 1, tree.Depth) // Child is depth 1
	})

	t.Run("get tree calculates depth correctly", func(t *testing.T) {
		// Create a 3-level hierarchy
		level0 := createTestSessionWithParent("depth-l0", "")
		insertSessionWithParent(t, db, level0, nil)

		l0ID := "depth-l0"
		level1 := createTestSessionWithParent("depth-l1", l0ID)
		insertSessionWithParent(t, db, level1, &l0ID)

		l1ID := "depth-l1"
		level2 := createTestSessionWithParent("depth-l2", l1ID)
		insertSessionWithParent(t, db, level2, &l1ID)

		l2ID := "depth-l2"
		level3 := createTestSessionWithParent("depth-l3", l2ID)
		insertSessionWithParent(t, db, level3, &l2ID)

		// Check depth at each level
		tree0, err := db.GetSessionTree("depth-l0")
		require.NoError(t, err)
		assert.Equal(t, 0, tree0.Depth)

		tree1, err := db.GetSessionTree("depth-l1")
		require.NoError(t, err)
		assert.Equal(t, 1, tree1.Depth)

		tree2, err := db.GetSessionTree("depth-l2")
		require.NoError(t, err)
		assert.Equal(t, 2, tree2.Depth)

		tree3, err := db.GetSessionTree("depth-l3")
		require.NoError(t, err)
		assert.Equal(t, 3, tree3.Depth)
	})

	t.Run("get tree with empty session_id fails", func(t *testing.T) {
		_, err := db.GetSessionTree("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session_id cannot be empty")
	})

	t.Run("get tree for nonexistent session fails", func(t *testing.T) {
		_, err := db.GetSessionTree("nonexistent-tree")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "session not found")
	})

	t.Run("full tree structure", func(t *testing.T) {
		// Create a more complex tree:
		//   grandparent
		//   └── parent
		//       ├── child-1
		//       └── child-2 (focus)
		//           └── grandchild

		grandparent := createTestSessionWithParent("full-gp", "")
		insertSessionWithParent(t, db, grandparent, nil)

		gpID := "full-gp"
		parent := createTestSessionWithParent("full-p", gpID)
		insertSessionWithParent(t, db, parent, &gpID)

		pID := "full-p"
		child1 := createTestSessionWithParent("full-c1", pID)
		child2 := createTestSessionWithParent("full-c2", pID)
		insertSessionWithParent(t, db, child1, &pID)
		insertSessionWithParent(t, db, child2, &pID)

		c2ID := "full-c2"
		grandchild := createTestSessionWithParent("full-gc", c2ID)
		insertSessionWithParent(t, db, grandchild, &c2ID)

		// Get tree for child-2
		tree, err := db.GetSessionTree("full-c2")
		require.NoError(t, err)
		assert.NotNil(t, tree)
		assert.Equal(t, "full-c2", tree.Root.SessionID)
		assert.NotNil(t, tree.Parent)
		assert.Equal(t, "full-p", tree.Parent.SessionID)
		assert.Len(t, tree.Children, 1)
		assert.Equal(t, "full-gc", tree.Children[0].SessionID)
		assert.Equal(t, 2, tree.Depth) // full-c2 is at depth 2
	})
}

// TestCircularReference tests protection against circular parent references
func TestCircularReference(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("circular reference detection", func(t *testing.T) {
		// Create two sessions
		session1 := createTestSessionWithParent("circular-1", "")
		insertSessionWithParent(t, db, session1, nil)

		session2 := createTestSessionWithParent("circular-2", "")
		insertSessionWithParent(t, db, session2, nil)

		// Manually create circular reference: 1 -> 2 -> 1
		// This bypasses normal constraints and simulates DB corruption
		_, err := db.conn.Exec("UPDATE sessions SET parent_session_id = ? WHERE session_id = ?", "circular-2", "circular-1")
		require.NoError(t, err)
		_, err = db.conn.Exec("UPDATE sessions SET parent_session_id = ? WHERE session_id = ?", "circular-1", "circular-2")
		require.NoError(t, err)

		// GetSessionTree should detect the circular reference
		_, err = db.GetSessionTree("circular-1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "circular parent reference")
	})
}

// TestHierarchyWithDeletedParent tests behavior when parent is deleted
func TestHierarchyWithDeletedParent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("children persist when parent deleted", func(t *testing.T) {
		parent := createTestSessionWithParent("del-parent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "del-parent"
		child := createTestSessionWithParent("del-child", parentID)
		insertSessionWithParent(t, db, child, &parentID)

		// Delete parent
		err := db.DeleteSession("del-parent")
		require.NoError(t, err)

		// Child should still exist
		retrievedChild, err := db.GetSession("del-child")
		require.NoError(t, err)
		assert.NotNil(t, retrievedChild)

		// Child's parent should be nil (orphaned)
		parent, err = db.GetParent("del-child")
		require.NoError(t, err)
		assert.Nil(t, parent)

		// Tree should show depth 0 (now a root)
		tree, err := db.GetSessionTree("del-child")
		require.NoError(t, err)
		assert.Equal(t, 0, tree.Depth)
		assert.Nil(t, tree.Parent)
	})
}

// TestHierarchyDatabaseErrors tests database error scenarios
func TestHierarchyDatabaseErrors(t *testing.T) {
	t.Run("database closed error handling", func(t *testing.T) {
		db := setupTestDB(t)

		// Create a session
		session := createTestSessionWithParent("db-error-test", "")
		insertSessionWithParent(t, db, session, nil)

		// Close the database
		db.Close()

		// Operations should fail gracefully
		_, err := db.GetChildren("db-error-test")
		assert.Error(t, err)

		_, err = db.GetParent("db-error-test")
		assert.Error(t, err)

		_, err = db.GetSessionTree("db-error-test")
		assert.Error(t, err)
	})
}

// TestHierarchyEdgeCases tests edge cases
func TestHierarchyEdgeCases(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("session with null parent_session_id", func(t *testing.T) {
		// Explicitly test NULL parent_session_id
		session := createTestSessionWithParent("null-parent", "")
		insertSessionWithParent(t, db, session, nil)

		parent, err := db.GetParent("null-parent")
		require.NoError(t, err)
		assert.Nil(t, parent)

		tree, err := db.GetSessionTree("null-parent")
		require.NoError(t, err)
		assert.Nil(t, tree.Parent)
		assert.Equal(t, 0, tree.Depth)
	})

	t.Run("session with empty string parent_session_id", func(t *testing.T) {
		// Test empty string vs NULL
		session := createTestSessionWithParent("empty-parent", "")
		emptyString := ""
		insertSessionWithParent(t, db, session, &emptyString)

		parent, err := db.GetParent("empty-parent")
		require.NoError(t, err)
		assert.Nil(t, parent)
	})

	t.Run("children ordered by created_at", func(t *testing.T) {
		parent := createTestSessionWithParent("order-parent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "order-parent"

		// Create children with specific timestamps
		child3 := createTestSessionWithParent("order-c3", parentID)
		child3.CreatedAt = time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
		insertSessionWithParent(t, db, child3, &parentID)

		child1 := createTestSessionWithParent("order-c1", parentID)
		child1.CreatedAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		insertSessionWithParent(t, db, child1, &parentID)

		child2 := createTestSessionWithParent("order-c2", parentID)
		child2.CreatedAt = time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		insertSessionWithParent(t, db, child2, &parentID)

		children, err := db.GetChildren("order-parent")
		require.NoError(t, err)
		assert.Len(t, children, 3)
		// Should be ordered by created_at ASC
		assert.Equal(t, "order-c1", children[0].SessionID)
		assert.Equal(t, "order-c2", children[1].SessionID)
		assert.Equal(t, "order-c3", children[2].SessionID)
	})
}

// TestHierarchyIntegration tests integration scenarios
func TestHierarchyIntegration(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	t.Run("complete hierarchy workflow", func(t *testing.T) {
		// Create a realistic session hierarchy
		// Project root
		root := createTestSessionWithParent("project-root", "")
		insertSessionWithParent(t, db, root, nil)

		// Feature branches
		rootID := "project-root"
		feature1 := createTestSessionWithParent("feature-auth", rootID)
		feature2 := createTestSessionWithParent("feature-api", rootID)
		insertSessionWithParent(t, db, feature1, &rootID)
		insertSessionWithParent(t, db, feature2, &rootID)

		// Sub-tasks under feature-auth
		f1ID := "feature-auth"
		task1 := createTestSessionWithParent("task-login", f1ID)
		task2 := createTestSessionWithParent("task-signup", f1ID)
		insertSessionWithParent(t, db, task1, &f1ID)
		insertSessionWithParent(t, db, task2, &f1ID)

		// Verify root has 2 children
		rootChildren, err := db.GetChildren("project-root")
		require.NoError(t, err)
		assert.Len(t, rootChildren, 2)

		// Verify feature-auth has 2 children
		featureChildren, err := db.GetChildren("feature-auth")
		require.NoError(t, err)
		assert.Len(t, featureChildren, 2)

		// Verify task-login tree
		taskTree, err := db.GetSessionTree("task-login")
		require.NoError(t, err)
		assert.Equal(t, "task-login", taskTree.Root.SessionID)
		assert.Equal(t, "feature-auth", taskTree.Parent.SessionID)
		assert.Len(t, taskTree.Children, 0)
		assert.Equal(t, 2, taskTree.Depth)

		// Verify feature-auth tree
		featureTree, err := db.GetSessionTree("feature-auth")
		require.NoError(t, err)
		assert.Equal(t, "feature-auth", featureTree.Root.SessionID)
		assert.Equal(t, "project-root", featureTree.Parent.SessionID)
		assert.Len(t, featureTree.Children, 2)
		assert.Equal(t, 1, featureTree.Depth)
	})
}

// TestGetAllSessionsHierarchy tests the GetAllSessionsHierarchy function
func TestGetAllSessionsHierarchy(t *testing.T) {
	t.Run("empty database returns empty hierarchy", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.NotNil(t, nodes)
		assert.Len(t, nodes, 0)
	})

	t.Run("single root session", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		root := createTestSessionWithParent("single-root", "")
		insertSessionWithParent(t, db, root, nil)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)
		assert.Equal(t, "single-root", nodes[0].Session.SessionID)
		assert.Equal(t, 0, nodes[0].Depth)
		assert.Len(t, nodes[0].Children, 0)
	})

	t.Run("parent with children hierarchy", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		parent := createTestSessionWithParent("hier-parent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "hier-parent"
		child1 := createTestSessionWithParent("hier-child-1", parentID)
		child2 := createTestSessionWithParent("hier-child-2", parentID)
		insertSessionWithParent(t, db, child1, &parentID)
		insertSessionWithParent(t, db, child2, &parentID)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1) // Only 1 root node

		rootNode := nodes[0]
		assert.Equal(t, "hier-parent", rootNode.Session.SessionID)
		assert.Equal(t, 0, rootNode.Depth)
		assert.Len(t, rootNode.Children, 2)

		// Verify children
		for _, child := range rootNode.Children {
			assert.Equal(t, 1, child.Depth)
			assert.Len(t, child.Children, 0)
		}
	})

	t.Run("deep hierarchy multiple levels", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		root := createTestSessionWithParent("deep-root", "")
		insertSessionWithParent(t, db, root, nil)

		rootID := "deep-root"
		child := createTestSessionWithParent("deep-child", rootID)
		insertSessionWithParent(t, db, child, &rootID)

		childID := "deep-child"
		grandchild := createTestSessionWithParent("deep-grandchild", childID)
		insertSessionWithParent(t, db, grandchild, &childID)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)

		// Check root
		assert.Equal(t, "deep-root", nodes[0].Session.SessionID)
		assert.Equal(t, 0, nodes[0].Depth)
		assert.Len(t, nodes[0].Children, 1)

		// Check child
		childNode := nodes[0].Children[0]
		assert.Equal(t, "deep-child", childNode.Session.SessionID)
		assert.Equal(t, 1, childNode.Depth)
		assert.Len(t, childNode.Children, 1)

		// Check grandchild
		grandchildNode := childNode.Children[0]
		assert.Equal(t, "deep-grandchild", grandchildNode.Session.SessionID)
		assert.Equal(t, 2, grandchildNode.Depth)
		assert.Len(t, grandchildNode.Children, 0)
	})

	t.Run("multiple independent root sessions", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		root1 := createTestSessionWithParent("multi-root-1", "")
		root2 := createTestSessionWithParent("multi-root-2", "")
		insertSessionWithParent(t, db, root1, nil)
		insertSessionWithParent(t, db, root2, nil)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2)

		// Both should be root level (depth 0)
		for _, node := range nodes {
			assert.Equal(t, 0, node.Depth)
		}
	})

	t.Run("complex multi-tree hierarchy", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		// Create two separate hierarchies
		// Tree 1: root1 -> child1a, child1b
		// Tree 2: root2 -> child2a -> grandchild2a
		root1 := createTestSessionWithParent("complex-root1", "")
		insertSessionWithParent(t, db, root1, nil)

		r1ID := "complex-root1"
		child1a := createTestSessionWithParent("complex-child1a", r1ID)
		child1b := createTestSessionWithParent("complex-child1b", r1ID)
		insertSessionWithParent(t, db, child1a, &r1ID)
		insertSessionWithParent(t, db, child1b, &r1ID)

		root2 := createTestSessionWithParent("complex-root2", "")
		insertSessionWithParent(t, db, root2, nil)

		r2ID := "complex-root2"
		child2a := createTestSessionWithParent("complex-child2a", r2ID)
		insertSessionWithParent(t, db, child2a, &r2ID)

		c2aID := "complex-child2a"
		grandchild2a := createTestSessionWithParent("complex-gc2a", c2aID)
		insertSessionWithParent(t, db, grandchild2a, &c2aID)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 2) // Two root nodes

		// Verify tree structures
		for _, node := range nodes {
			if node.Session.SessionID == "complex-root1" {
				assert.Len(t, node.Children, 2)
				for _, child := range node.Children {
					assert.Equal(t, 1, child.Depth)
				}
			} else if node.Session.SessionID == "complex-root2" {
				assert.Len(t, node.Children, 1)
				assert.Equal(t, "complex-child2a", node.Children[0].Session.SessionID)
				assert.Len(t, node.Children[0].Children, 1)
				assert.Equal(t, "complex-gc2a", node.Children[0].Children[0].Session.SessionID)
			}
		}
	})

	t.Run("filter by lifecycle", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		active := createTestSessionWithParent("filter-active", "")
		insertSessionWithParent(t, db, active, nil)

		archived := createTestSessionWithParent("filter-archived", "")
		archived.Lifecycle = manifest.LifecycleArchived
		insertSessionWithParent(t, db, archived, nil)

		// No filter - should get both
		allNodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, allNodes, 2)

		// Filter for archived only
		filter := &SessionFilter{Lifecycle: manifest.LifecycleArchived}
		archivedNodes, err := db.GetAllSessionsHierarchy(filter)
		require.NoError(t, err)
		assert.Len(t, archivedNodes, 1)
		assert.Equal(t, "filter-archived", archivedNodes[0].Session.SessionID)
	})

	t.Run("IsLast flag is set correctly", func(t *testing.T) {
		db := setupTestDB(t)
		defer db.Close()

		parent := createTestSessionWithParent("islast-parent", "")
		insertSessionWithParent(t, db, parent, nil)

		parentID := "islast-parent"
		child1 := createTestSessionWithParent("islast-c1", parentID)
		child2 := createTestSessionWithParent("islast-c2", parentID)
		child3 := createTestSessionWithParent("islast-c3", parentID)
		insertSessionWithParent(t, db, child1, &parentID)
		time.Sleep(1 * time.Millisecond)
		insertSessionWithParent(t, db, child2, &parentID)
		time.Sleep(1 * time.Millisecond)
		insertSessionWithParent(t, db, child3, &parentID)

		nodes, err := db.GetAllSessionsHierarchy(nil)
		require.NoError(t, err)
		assert.Len(t, nodes, 1)

		children := nodes[0].Children
		assert.Len(t, children, 3)

		// First two should not be marked as last
		assert.False(t, children[0].IsLast)
		assert.False(t, children[1].IsLast)

		// Last one should be marked as last
		assert.True(t, children[2].IsLast)
	})
}
