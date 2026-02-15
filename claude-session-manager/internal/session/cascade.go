package session

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/db"
)

// CascadeAction represents the action to take on child sessions when parent terminates
type CascadeAction string

const (
	CascadeTerminate CascadeAction = "terminate" // Terminate all children
	CascadeSkip      CascadeAction = "skip"      // Leave children running
	CascadeDetach    CascadeAction = "detach"    // Detach from parent (set parent_session_id = NULL)
)

// Integration Example:
//
// To integrate cascade termination into session archival/exit flow:
//
//   // Before archiving/terminating a session:
//   action, err := session.PromptCascadeTermination(database, parentSessionID)
//   if err != nil {
//       return fmt.Errorf("failed to prompt cascade: %w", err)
//   }
//
//   // Execute the chosen action
//   if err := session.ExecuteCascadeTermination(database, parentSessionID, action); err != nil {
//       return fmt.Errorf("failed to execute cascade: %w", err)
//   }
//
//   // Now archive/terminate the parent session
//   // ... existing termination logic ...

// PromptCascadeTermination prompts the user to choose what to do with child sessions
// when a parent session terminates. Returns the chosen CascadeAction.
func PromptCascadeTermination(database *db.DB, parentID string) (CascadeAction, error) {
	return promptCascadeTerminationWithReader(database, parentID, os.Stdin)
}

// promptCascadeTerminationWithReader allows injecting an io.Reader for testing
func promptCascadeTerminationWithReader(database *db.DB, parentID string, reader io.Reader) (CascadeAction, error) {
	if database == nil {
		return "", fmt.Errorf("database cannot be nil")
	}
	if parentID == "" {
		return "", fmt.Errorf("parentID cannot be empty")
	}

	// Get children count
	children, err := database.GetChildren(parentID)
	if err != nil {
		return "", fmt.Errorf("failed to get children: %w", err)
	}

	// If no children, skip cascade logic
	if len(children) == 0 {
		return CascadeSkip, nil
	}

	// Show prompt to user
	fmt.Printf("Session has %d child session(s). Terminate children? [Y/n/keep]: ", len(children))

	// Read user input
	bufReader := bufio.NewReader(reader)
	input, err := bufReader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("failed to read user input: %w", err)
	}

	// Trim whitespace and convert to lowercase
	input = strings.TrimSpace(strings.ToLower(input))

	// Parse user choice
	return parseCascadeInput(input)
}

// parseCascadeInput parses user input and returns the corresponding CascadeAction
func parseCascadeInput(input string) (CascadeAction, error) {
	switch input {
	case "", "y", "yes":
		return CascadeTerminate, nil
	case "n", "no":
		return CascadeSkip, nil
	case "keep":
		return CascadeDetach, nil
	default:
		return "", fmt.Errorf("invalid input: %s (expected Y/n/keep)", input)
	}
}

// ExecuteCascadeTermination executes the chosen cascade action on child sessions
func ExecuteCascadeTermination(database *db.DB, parentID string, action CascadeAction) error {
	if database == nil {
		return fmt.Errorf("database cannot be nil")
	}
	if parentID == "" {
		return fmt.Errorf("parentID cannot be empty")
	}

	// Get children
	children, err := database.GetChildren(parentID)
	if err != nil {
		return fmt.Errorf("failed to get children: %w", err)
	}

	// If no children, nothing to do
	if len(children) == 0 {
		return nil
	}

	switch action {
	case CascadeTerminate:
		// Terminate all children by setting lifecycle to "archived"
		for _, child := range children {
			child.Lifecycle = "archived"
			if err := database.UpdateSession(child); err != nil {
				return fmt.Errorf("failed to terminate child session %s: %w", child.SessionID, err)
			}
		}
		return nil

	case CascadeSkip:
		// Do nothing - leave children as-is
		return nil

	case CascadeDetach:
		// Detach children by setting parent_session_id to NULL
		// We need to use raw SQL for this since UpdateSession doesn't support setting parent_session_id
		conn := database.Conn()
		for _, child := range children {
			query := `UPDATE sessions SET parent_session_id = NULL WHERE session_id = ?`
			result, err := conn.Exec(query, child.SessionID)
			if err != nil {
				return fmt.Errorf("failed to detach child session %s: %w", child.SessionID, err)
			}

			rowsAffected, err := result.RowsAffected()
			if err != nil {
				return fmt.Errorf("failed to get rows affected for child %s: %w", child.SessionID, err)
			}

			if rowsAffected == 0 {
				return fmt.Errorf("child session not found: %s", child.SessionID)
			}
		}
		return nil

	default:
		return fmt.Errorf("invalid cascade action: %s", action)
	}
}
