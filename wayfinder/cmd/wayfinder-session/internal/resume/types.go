package resume

import "errors"

// DirectoryState represents the state of a project directory
type DirectoryState int

const (
	// StateEmpty indicates directory has no visible files (empty or only hidden files)
	StateEmpty DirectoryState = iota
	// StateW0Only indicates directory contains only W0 file(s)
	StateW0Only
	// StateStatusOnly indicates directory contains only WAYFINDER-STATUS.md
	StateStatusOnly
	// StateBothW0AndStatus indicates directory contains both W0 and STATUS files
	StateBothW0AndStatus
	// StateNonResumable indicates directory contains other files beyond W0/STATUS
	StateNonResumable
)

// String returns human-readable state name
func (s DirectoryState) String() string {
	switch s {
	case StateEmpty:
		return "empty"
	case StateW0Only:
		return "W0-only"
	case StateStatusOnly:
		return "STATUS-only"
	case StateBothW0AndStatus:
		return "W0+STATUS"
	case StateNonResumable:
		return "non-resumable"
	default:
		return "unknown"
	}
}

// MenuChoice represents user's choice from interactive menu
type MenuChoice int

const (
	// ChoiceResume indicates user chose to resume existing project
	ChoiceResume MenuChoice = iota
	// ChoiceNew indicates user chose to create project in different location
	ChoiceNew
	// ChoiceAbort indicates user chose to abort without changes
	ChoiceAbort
)

// String returns human-readable choice name
func (c MenuChoice) String() string {
	switch c {
	case ChoiceResume:
		return "resume"
	case ChoiceNew:
		return "new"
	case ChoiceAbort:
		return "abort"
	default:
		return "unknown"
	}
}

// DetectionResult holds the result of directory state detection
type DetectionResult struct {
	State        DirectoryState
	VisibleFiles []string // All visible files (excluding hidden files starting with '.')
	W0Files      []string // W0-charter.md or W0.md files found
	StatusFiles  []string // WAYFINDER-STATUS.md files found
}

// Errors
var (
	// ErrNonResumable indicates directory contains files beyond W0/STATUS
	ErrNonResumable = errors.New("directory contains non-resumable files")

	// ErrUserAborted indicates user chose to abort operation
	ErrUserAborted = errors.New("user aborted")

	// ErrPermissionDenied indicates insufficient permissions to access directory
	ErrPermissionDenied = errors.New("permission denied")
)
