package resume

import (
	"fmt"
	"os"
	"strings"
)

// scanDirectory reads directory and returns list of visible files
// Hidden files (starting with '.') are excluded per FR1 requirement
func scanDirectory(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s", ErrPermissionDenied, dir)
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var visibleFiles []string
	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// Skip hidden files (per FR1: "Files/directories starting with '.' MUST be ignored")
		if strings.HasPrefix(name, ".") {
			continue
		}

		visibleFiles = append(visibleFiles, name)
	}

	return visibleFiles, nil
}

// isW0File checks if filename matches W0 charter patterns
// Matches: W0-charter.md, W0.md (case-sensitive per Unix conventions)
func isW0File(name string) bool {
	return name == "W0-charter.md" || name == "W0.md"
}

// isStatusFile checks if filename matches STATUS file pattern
// Matches: WAYFINDER-STATUS.md (exact match, case-sensitive)
func isStatusFile(name string) bool {
	return name == "WAYFINDER-STATUS.md"
}

// classifyState determines directory state from list of visible files
// Implements FR1 resumable pattern detection
func classifyState(files []string) DetectionResult {
	result := DetectionResult{
		VisibleFiles: files,
		W0Files:      []string{},
		StatusFiles:  []string{},
	}

	// Categorize files
	for _, file := range files {
		if isW0File(file) {
			result.W0Files = append(result.W0Files, file)
		} else if isStatusFile(file) {
			result.StatusFiles = append(result.StatusFiles, file)
		}
	}

	hasW0 := len(result.W0Files) > 0
	hasStatus := len(result.StatusFiles) > 0
	hasOtherFiles := len(files) > len(result.W0Files)+len(result.StatusFiles)

	// Determine state per FR1 resumable patterns
	switch {
	case len(files) == 0:
		// Pattern 1: Empty directory (zero visible files)
		result.State = StateEmpty
	case hasOtherFiles:
		// Non-resumable: Directory has files beyond W0/STATUS
		result.State = StateNonResumable
	case hasW0 && hasStatus:
		// Pattern 4: Both W0 and STATUS (no other files)
		result.State = StateBothW0AndStatus
	case hasW0:
		// Pattern 2: W0-only (no other files)
		result.State = StateW0Only
	case hasStatus:
		// Pattern 3: STATUS-only (no other files)
		result.State = StateStatusOnly
	default:
		// Shouldn't reach here logically, but classify as non-resumable
		result.State = StateNonResumable
	}

	return result
}
