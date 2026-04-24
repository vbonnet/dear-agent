package validator

import (
	"errors"
	"fmt"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// CheckChildrenComplete validates all child projects are complete.
// Returns error if any child is incomplete with details.
//
// This function performs recursive validation - if a child has children,
// they must also be complete for the child to be considered complete.
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - nil if all children complete (or no children exist)
//   - Error with list of incomplete children if validation fails
//
// Example error:
//
//	"child projects not complete: tasks/child-a, tasks/child-b"
func CheckChildrenComplete(dir string) error {
	return checkChildrenCompleteWithDepth(dir, 0)
}

// checkChildrenCompleteWithDepth implements recursive checking with depth tracking
func checkChildrenCompleteWithDepth(dir string, depth int) error {
	// Safety: prevent stack overflow from excessive nesting
	if depth > status.MaxNestingDepth {
		return status.ErrMaxDepthExceeded
	}

	// Check if directory has children
	if !status.HasChildren(dir) {
		return nil // No children = validation passes
	}

	// Get list of child projects
	children, err := status.ListChildren(dir)
	if err != nil {
		return fmt.Errorf("failed to list child projects: %w", err)
	}

	// Check each child
	var incompleteChildren []string

	for _, childName := range children {
		childPath := fmt.Sprintf("%s/tasks/%s", dir, childName)

		complete, err := isProjectCompleteWithDepth(childPath, depth+1)
		if err != nil {
			// If max depth exceeded, propagate immediately (critical error)
			if errors.Is(err, status.ErrMaxDepthExceeded) {
				return err
			}

			// Other errors: log but continue checking other children
			// This allows us to report all incomplete/errored children at once
			incompleteChildren = append(incompleteChildren, fmt.Sprintf("tasks/%s (error: %v)", childName, err))
			continue
		}

		if !complete {
			incompleteChildren = append(incompleteChildren, fmt.Sprintf("tasks/%s", childName))
		}
	}

	// If any children incomplete, return error
	if len(incompleteChildren) > 0 {
		return fmt.Errorf("child projects not complete: %s", strings.Join(incompleteChildren, ", "))
	}

	return nil
}

// GetIncompleteChildren returns list of child projects that are not complete.
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - Slice of incomplete child project names (relative paths: "tasks/child-a")
//   - Error if unable to check children
func GetIncompleteChildren(dir string) ([]string, error) {
	// Check if directory has children
	if !status.HasChildren(dir) {
		return []string{}, nil
	}

	// Get list of child projects
	children, err := status.ListChildren(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list child projects: %w", err)
	}

	var incomplete []string

	for _, childName := range children {
		childPath := fmt.Sprintf("%s/tasks/%s", dir, childName)

		complete, err := IsProjectComplete(childPath)
		if err != nil {
			// Include errored children as "incomplete"
			incomplete = append(incomplete, fmt.Sprintf("tasks/%s", childName))
			continue
		}

		if !complete {
			incomplete = append(incomplete, fmt.Sprintf("tasks/%s", childName))
		}
	}

	return incomplete, nil
}

// IsProjectComplete checks if a single project is complete.
// A project is complete if:
//   - Status field == "completed", OR
//   - All phases have Status == "completed"
//
// If project has children, they must also be complete (recursive check).
//
// Parameters:
//   - dir: Absolute path to project directory
//
// Returns:
//   - true if project is complete, false otherwise
//   - Error if unable to read project status
func IsProjectComplete(dir string) (bool, error) {
	return isProjectCompleteWithDepth(dir, 0)
}

// isProjectCompleteWithDepth implements recursive checking with depth tracking
func isProjectCompleteWithDepth(dir string, depth int) (bool, error) {
	// Safety: prevent stack overflow from excessive nesting
	if depth > status.MaxNestingDepth {
		return false, status.ErrMaxDepthExceeded
	}

	// Read project status
	st, err := status.ReadFrom(dir)
	if err != nil {
		return false, fmt.Errorf("failed to read project status: %w", err)
	}

	// Check 1: Is overall status "completed"?
	if st.Status == status.StatusCompleted {
		// Still need to verify children are complete
		if status.HasChildren(dir) {
			err := checkChildrenCompleteWithDepth(dir, depth)
			if err != nil {
				// Propagate critical errors like max depth exceeded
				if errors.Is(err, status.ErrMaxDepthExceeded) {
					return false, err
				}
				return false, nil // Children incomplete = project incomplete
			}
		}
		return true, nil
	}

	// Check 2: Are all phases completed?
	allPhasesComplete := true
	if len(st.Phases) == 0 {
		allPhasesComplete = false
	}

	for _, phase := range st.Phases {
		if phase.Status != status.PhaseStatusCompleted {
			allPhasesComplete = false
			break
		}
	}

	if !allPhasesComplete {
		return false, nil
	}

	// All phases complete, but still need to check children
	if status.HasChildren(dir) {
		err := checkChildrenCompleteWithDepth(dir, depth)
		if err != nil {
			// Propagate critical errors like max depth exceeded
			if err == status.ErrMaxDepthExceeded {
				return false, err
			}
			return false, nil // Children incomplete = project incomplete
		}
	}

	return true, nil
}
