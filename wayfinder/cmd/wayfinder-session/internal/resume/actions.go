// Package resume provides resume-related functionality.
package resume

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// resume handles [R] Resume action
// Implements FR3 Resume requirement
func resume(dir string, state DirectoryState) error {
	var st *status.Status
	var err error

	switch state {
	case StateW0Only:
		// W0 exists but no STATUS → create new STATUS
		st = status.New(dir)

		// Mark W0 as completed (per FR3 W0-only behavior)
		st.UpdatePhase("W0", status.PhaseStatusCompleted, "W0 charter found, resuming project")
		st.CurrentPhase = "D1" // Next phase after W0

		fmt.Println("✅ Resumed project. Created STATUS from existing W0.")

	case StateStatusOnly, StateBothW0AndStatus:
		// STATUS exists → load existing
		st, err = status.ReadFrom(dir)
		if err != nil {
			return fmt.Errorf("failed to read existing STATUS: %w", err)
		}

		// Determine current phase from STATUS
		currentPhase, err := st.NextPhase()
		if err != nil {
			// Already at final phase or invalid state - keep as is
			currentPhase = st.CurrentPhase
		}
		st.CurrentPhase = currentPhase

		if state == StateBothW0AndStatus {
			fmt.Printf("✅ Resumed project. W0 complete, ready for %s.\n", currentPhase)
		} else {
			fmt.Printf("✅ Resumed project at phase %s.\n", currentPhase)
		}

	case StateEmpty:
		// Empty directory → fresh start (shouldn't reach here, but handle gracefully)
		st = status.New(dir)
		fmt.Println("✅ Starting new project in empty directory.")

	case StateNonResumable:
		return fmt.Errorf("cannot resume from state: %s", state)
	}

	// Write updated STATUS
	if err := st.WriteTo(dir); err != nil {
		return fmt.Errorf("failed to write STATUS file: %w", err)
	}

	return nil
}

// createInDifferentLocation handles [N] New action
// Implements FR4 New Name requirement
func createInDifferentLocation(_ string, currentDir string, reader io.Reader) error {
	bufReader := bufio.NewReader(reader)

	// Prompt for new project name
	fmt.Print("\nEnter new project name: ")
	newName, err := readUserInput(bufReader)
	if err != nil {
		return fmt.Errorf("failed to read project name: %w", err)
	}

	// Validate project name (non-empty, valid directory name)
	newName = strings.TrimSpace(newName)
	if newName == "" {
		return fmt.Errorf("project name cannot be empty")
	}

	// Security: Validate project name (implement T1 mitigation from S6)
	if err := validateProjectName(newName); err != nil {
		return err
	}

	// Prompt for location choice
	fmt.Println("\nChoose directory location:")
	fmt.Println("  [C] Current directory + subdirectory (" + newName + "/)")
	fmt.Println("  [O] Other location (enter path)")
	fmt.Print("Choice [C/O]: ")

	locChoice, err := readUserInput(bufReader)
	if err != nil {
		return fmt.Errorf("failed to read location choice: %w", err)
	}

	var newPath string
	switch strings.ToUpper(locChoice) {
	case "C":
		// Create in current directory + subdirectory
		newPath = filepath.Join(currentDir, newName)

	case "O":
		// Prompt for other path
		fmt.Print("Enter directory path: ")
		customPath, err := readUserInput(bufReader)
		if err != nil {
			return fmt.Errorf("failed to read directory path: %w", err)
		}

		customPath = strings.TrimSpace(customPath)
		if customPath == "" {
			return fmt.Errorf("directory path cannot be empty")
		}

		// Use custom path directly (user provides full path)
		newPath = customPath

	default:
		return fmt.Errorf("invalid location choice: %s (expected C or O)", locChoice)
	}

	// Create directory if needed
	if err := os.MkdirAll(newPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", newPath, err)
	}

	// Create new STATUS in new location
	st := status.New(newPath)
	if err := st.WriteTo(newPath); err != nil {
		return fmt.Errorf("failed to write STATUS to %s: %w", newPath, err)
	}

	fmt.Printf("✅ Created new project in %s\n", newPath)
	return nil
}

// abort handles [A] Abort action
// Implements FR5 Abort requirement
func abort() error {
	fmt.Println("Aborted. No changes made.")
	return ErrUserAborted
}

// validateProjectName validates project name for security
// Implements T1 path traversal mitigation from S6 threat model
func validateProjectName(name string) error {
	// Reject path traversal attempts
	if strings.Contains(name, "..") {
		return fmt.Errorf("invalid project name: cannot contain '..'")
	}

	// Reject absolute paths
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return fmt.Errorf("invalid project name: cannot contain path separators")
	}

	// Reject empty or whitespace-only names
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("invalid project name: cannot be empty")
	}

	return nil
}
