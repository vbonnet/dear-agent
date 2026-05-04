package session

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

// CascadeAction represents the action to take on child sessions when parent terminates
type CascadeAction string

// CascadeAction values for child sessions when a parent terminates.
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
//   action, err := session.PromptCascadeTermination(adapter, parentSessionID)
//   if err != nil {
//       return fmt.Errorf("failed to prompt cascade: %w", err)
//   }
//
//   // Execute the chosen action
//   if err := session.ExecuteCascadeTermination(adapter, parentSessionID, action); err != nil {
//       return fmt.Errorf("failed to execute cascade: %w", err)
//   }
//
//   // Now archive/terminate the parent session
//   // ... existing termination logic ...

// PromptCascadeTermination prompts the user to choose what to do with child sessions
// when a parent session terminates. Returns the chosen CascadeAction.
func PromptCascadeTermination(adapter *dolt.Adapter, parentID string) (CascadeAction, error) {
	return promptCascadeTerminationWithReader(adapter, parentID, os.Stdin)
}

// promptCascadeTerminationWithReader allows injecting an io.Reader for testing
func promptCascadeTerminationWithReader(adapter *dolt.Adapter, parentID string, reader io.Reader) (CascadeAction, error) {
	if adapter == nil {
		return "", fmt.Errorf("adapter cannot be nil")
	}
	if parentID == "" {
		return "", fmt.Errorf("parentID cannot be empty")
	}

	// Get children count
	children, err := adapter.GetChildren(parentID)
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
func ExecuteCascadeTermination(adapter *dolt.Adapter, parentID string, action CascadeAction) error {
	if adapter == nil {
		return fmt.Errorf("adapter cannot be nil")
	}
	if parentID == "" {
		return fmt.Errorf("parentID cannot be empty")
	}

	// Get children
	children, err := adapter.GetChildren(parentID)
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
			if err := adapter.UpdateSession(child); err != nil {
				return fmt.Errorf("failed to terminate child session %s: %w", child.SessionID, err)
			}
		}
		return nil

	case CascadeSkip:
		// Do nothing - leave children as-is
		return nil

	case CascadeDetach:
		// Detach children by setting parent_session_id to NULL
		for _, child := range children {
			if err := adapter.DetachChild(child.SessionID); err != nil {
				return fmt.Errorf("failed to detach child session %s: %w", child.SessionID, err)
			}
		}
		return nil

	default:
		return fmt.Errorf("invalid cascade action: %s", action)
	}
}
