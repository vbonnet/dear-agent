// Package errors provides structured error types for devlog operations.
//
// It includes custom error types with operation and path context tracking,
// sentinel errors for common failure cases, and exit codes for CLI usage.
//
// The DevlogError type implements error unwrapping (errors.Unwrap) to support
// Go 1.13+ error inspection with errors.Is() and errors.As().
//
// Usage:
//
//	err := errors.WrapPath("load config", "/path/to/file", ErrConfigNotFound)
//	if errors.Is(err, errors.ErrConfigNotFound) {
//	    // Handle config not found
//	}
package errors

import (
	"errors"
	"fmt"
)

// DevlogError is the base error type for devlog operations
type DevlogError struct {
	Op   string // Operation that failed (e.g., "load config", "git pull")
	Path string // File/directory path (optional)
	Err  error  // Underlying error
}

// Error implements the error interface
func (e *DevlogError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s failed for %s: %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("%s failed: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error for errors.Is/As support
func (e *DevlogError) Unwrap() error {
	return e.Err
}

// Sentinel errors for common failure cases
var (
	ErrConfigNotFound = errors.New("config file not found")
	ErrConfigInvalid  = errors.New("config is invalid")
	ErrRepoNotFound   = errors.New("repo not found in config")
	ErrGitFailed      = errors.New("git operation failed")
)

// Exit codes for CLI
const (
	ExitSuccess      = 0
	ExitGeneralError = 1
	ExitConfigError  = 2
	ExitGitError     = 3
)

// Wrap creates a DevlogError wrapping an existing error
func Wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return &DevlogError{
		Op:  op,
		Err: err,
	}
}

// WrapPath creates a DevlogError with both operation and path context
func WrapPath(op, path string, err error) error {
	if err == nil {
		return nil
	}
	return &DevlogError{
		Op:   op,
		Path: path,
		Err:  err,
	}
}

// Is reports whether err or any error in its chain matches target.
// This is a convenience wrapper around errors.Is from the standard library.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's chain that matches target type.
// This is a convenience wrapper around errors.As from the standard library.
func As(err error, target any) bool {
	return errors.As(err, target)
}
