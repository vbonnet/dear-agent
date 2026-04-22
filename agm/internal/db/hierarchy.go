package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// SessionTree represents a session with its hierarchical relationships
type SessionTree struct {
	Root     *manifest.Manifest   // The session being queried
	Parent   *manifest.Manifest   // Parent session (nil if root)
	Children []*manifest.Manifest // Direct children
	Depth    int                  // 0 for root, 1 for children of root, etc.
}

// GetChildren returns all direct child sessions of the given session.
// Returns an empty slice if the session has no children.
func (db *DB) GetChildren(sessionID string) ([]*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	query := `
		SELECT session_id, name, schema_version, created_at, updated_at, lifecycle,
			harness, model, context_project, context_purpose, context_tags, context_notes,
			claude_uuid, tmux_session_name, engram_enabled, engram_query,
			engram_ids, engram_loaded_at, engram_count, parent_session_id
		FROM sessions
		WHERE parent_session_id = ?
		ORDER BY created_at ASC
	`

	rows, err := db.conn.Query(query, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query children: %w", err)
	}
	defer rows.Close()

	var children []*manifest.Manifest
	for rows.Next() {
		session, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		children = append(children, session)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating children rows: %w", err)
	}

	// Return empty slice instead of nil if no children found
	if children == nil {
		children = []*manifest.Manifest{}
	}

	return children, nil
}

// GetParent returns the parent session of the given session.
// Returns nil if the session has no parent (i.e., it's a root session).
// Returns an error if the session is not found.
func (db *DB) GetParent(sessionID string) (*manifest.Manifest, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Query for the parent_session_id from the database row
	var parentSessionID sql.NullString
	query := `SELECT parent_session_id FROM sessions WHERE session_id = ?`
	err := db.conn.QueryRow(query, sessionID).Scan(&parentSessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("session not found: %s", sessionID)
		}
		return nil, fmt.Errorf("failed to query parent_session_id: %w", err)
	}

	// If parent_session_id is NULL, this is a root session
	if !parentSessionID.Valid || parentSessionID.String == "" {
		return nil, nil
	}

	// Fetch the parent session
	parent, err := db.GetSession(parentSessionID.String)
	if err != nil {
		// If parent session doesn't exist (orphaned reference), return nil
		// This handles the case where parent was deleted with ON DELETE SET NULL
		if err.Error() == fmt.Sprintf("session not found") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get parent session: %w", err)
	}

	return parent, nil
}

// GetSessionTree returns the full hierarchical tree for a given session.
// It includes the session itself, its parent (if any), direct children, and calculates depth.
func (db *DB) GetSessionTree(sessionID string) (*SessionTree, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("session_id cannot be empty")
	}

	// Get the root session
	root, err := db.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// Get the parent session
	parent, err := db.GetParent(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get parent: %w", err)
	}

	// Get the children
	children, err := db.GetChildren(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get children: %w", err)
	}

	// Calculate depth by walking up the parent chain
	depth := 0
	currentID := sessionID
	visited := make(map[string]bool) // Prevent infinite loops in case of circular references

	for {
		if visited[currentID] {
			return nil, fmt.Errorf("circular parent reference detected for session %s", sessionID)
		}
		visited[currentID] = true

		var parentID sql.NullString
		query := `SELECT parent_session_id FROM sessions WHERE session_id = ?`
		err := db.conn.QueryRow(query, currentID).Scan(&parentID)
		if err != nil {
			if err == sql.ErrNoRows {
				break
			}
			return nil, fmt.Errorf("failed to calculate depth: %w", err)
		}

		if !parentID.Valid || parentID.String == "" {
			break
		}

		depth++
		currentID = parentID.String
	}

	return &SessionTree{
		Root:     root,
		Parent:   parent,
		Children: children,
		Depth:    depth,
	}, nil
}

// SessionNode represents a session in a hierarchical tree structure for rendering
type SessionNode struct {
	Session  *manifest.Manifest
	Depth    int
	Children []*SessionNode
	IsLast   bool // Whether this is the last child of its parent
}

// GetAllSessionsHierarchy returns all sessions organized in a hierarchical tree structure.
// Root sessions (those without parents) are at the top level.
// The sessions are ordered by creation time within each level.
func (db *DB) GetAllSessionsHierarchy(filter *SessionFilter) ([]*SessionNode, error) {
	// Get all sessions
	sessions, err := db.ListSessions(filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		return []*SessionNode{}, nil
	}

	// Build a map of session ID to session for quick lookup
	sessionMap := make(map[string]*manifest.Manifest)
	for _, s := range sessions {
		sessionMap[s.SessionID] = s
	}

	// Build a map of parent ID to children
	childrenMap := make(map[string][]*manifest.Manifest)
	var rootSessions []*manifest.Manifest

	for _, s := range sessions {
		// Get parent_session_id from database
		var parentSessionID sql.NullString
		query := `SELECT parent_session_id FROM sessions WHERE session_id = ?`
		err := db.conn.QueryRow(query, s.SessionID).Scan(&parentSessionID)
		if err != nil {
			if err == sql.ErrNoRows {
				continue
			}
			return nil, fmt.Errorf("failed to query parent_session_id: %w", err)
		}

		if !parentSessionID.Valid || parentSessionID.String == "" {
			// This is a root session
			rootSessions = append(rootSessions, s)
		} else {
			// This is a child session
			parentID := parentSessionID.String
			childrenMap[parentID] = append(childrenMap[parentID], s)
		}
	}

	// Recursively build the tree
	var buildTree func(session *manifest.Manifest, depth int) *SessionNode
	buildTree = func(session *manifest.Manifest, depth int) *SessionNode {
		node := &SessionNode{
			Session:  session,
			Depth:    depth,
			Children: []*SessionNode{},
			IsLast:   false,
		}

		// Add children
		children := childrenMap[session.SessionID]
		for i, child := range children {
			childNode := buildTree(child, depth+1)
			if i == len(children)-1 {
				childNode.IsLast = true
			}
			node.Children = append(node.Children, childNode)
		}

		return node
	}

	// Build tree for each root session
	var result []*SessionNode
	for i, root := range rootSessions {
		node := buildTree(root, 0)
		if i == len(rootSessions)-1 {
			node.IsLast = true
		}
		result = append(result, node)
	}

	return result, nil
}
