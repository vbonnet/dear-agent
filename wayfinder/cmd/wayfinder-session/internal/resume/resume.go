package resume

import (
	"fmt"
	"os"
)

// Detect scans directory and handles resume flow if needed.
// Returns true if session should continue (resume/new/empty), false if aborted.
//
// This is the single public entry point for the resume package.
// It orchestrates: detect → prompt → action flow.
//
// Behavior:
// - Empty directory → return true (no prompt)
// - Non-resumable → return error with clear message
// - Resumable → show menu, handle user choice
//   - Resume → load existing STATUS, return true
//   - New → prompt for new location, return false (caller should not continue)
//   - Abort → return false
func Detect(dir string, projectName string) (shouldContinue bool, err error) {
	// Step 1: Scan directory
	files, err := scanDirectory(dir)
	if err != nil {
		// Permission denied or other IO error
		return false, fmt.Errorf("cannot access directory %s: %w\n"+
			"Check directory permissions or run with appropriate access", dir, err)
	}

	// Step 2: Classify state
	result := classifyState(files)

	// Step 3: Handle based on state
	switch result.State {
	case StateEmpty:
		// Empty directory → no prompt, continue with fresh start
		return true, nil

	case StateNonResumable:
		// Directory has files beyond W0/STATUS → error
		return false, fmt.Errorf("%w: %v\n"+
			"Use --force to overwrite, or choose different directory",
			ErrNonResumable, result.VisibleFiles)

	case StateW0Only, StateStatusOnly, StateBothW0AndStatus:
		// Resumable directory → show menu
		choice, err := showMenu(result.State, dir, getStdin())
		if err != nil {
			return false, fmt.Errorf("failed to show menu: %w", err)
		}

		// Step 4: Handle user's choice
		switch choice {
		case ChoiceResume:
			// Resume existing project
			if err := resume(dir, result.State); err != nil {
				return false, fmt.Errorf("failed to resume: %w", err)
			}
			return true, nil

		case ChoiceNew:
			// Create in different location
			if err := createInDifferentLocation(projectName, dir, getStdin()); err != nil {
				return false, fmt.Errorf("failed to create in different location: %w", err)
			}
			// New session created elsewhere, caller should not continue in original dir
			return false, nil

		case ChoiceAbort:
			// User aborted
			return false, abort()

		default:
			return false, fmt.Errorf("unknown menu choice: %s", choice)
		}

	default:
		return false, fmt.Errorf("unknown directory state: %s", result.State)
	}
}

// isSafeToWrite checks if path is safe to write (not a symlink)
// Implements T4 symlink attack mitigation from S6 threat model
func isSafeToWrite(path string) error {
	info, err := os.Lstat(path) // Don't follow symlinks
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist, safe to write
		}
		return fmt.Errorf("failed to check file: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to write to symlink: %s", path)
	}

	return nil
}
