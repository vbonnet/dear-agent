// Package security provides utilities for secure file operations.
//
// This package prevents common security vulnerabilities including:
// - Path traversal attacks (../ sequences)
// - Symlink following vulnerabilities
// - File size DoS attacks
// - Invalid file extensions
// - Unsafe directory creation
//
// All diagram-as-code skills MUST use these utilities for file operations.
package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Security constants
const (
	// MaxDiagramSize is the maximum allowed size for diagram files (10MB)
	MaxDiagramSize = 10 * 1024 * 1024
)

// Allowed file extensions
var (
	// AllowedDiagramExtensions are the permitted diagram file extensions
	AllowedDiagramExtensions = map[string]bool{
		".d2":          true,
		".mmd":         true,
		".mermaid":     true,
		".dsl":         true,
		".structurizr": true,
		".puml":        true,
		".plantuml":    true,
	}

	// AllowedOutputExtensions are the permitted output file extensions
	AllowedOutputExtensions = map[string]bool{
		".svg":  true,
		".png":  true,
		".pdf":  true,
		".json": true,
		".txt":  true,
		".md":   true,
	}
)

// Error types
var (
	// ErrPathTraversal is returned when path traversal is detected
	ErrPathTraversal = errors.New("path traversal detected")

	// ErrSymlink is returned when an unauthorized symlink is detected
	ErrSymlink = errors.New("symlinks not allowed")

	// ErrFileSize is returned when a file exceeds size limits
	ErrFileSize = errors.New("file too large")

	// ErrInvalidExtension is returned when a file has an invalid extension
	ErrInvalidExtension = errors.New("invalid file extension")

	// ErrNotDirectory is returned when allowed base is not a directory
	ErrNotDirectory = errors.New("allowed base must be a directory")
)

// PathTraversalError wraps path traversal errors with context
type PathTraversalError struct {
	Path        string
	AllowedBase string
}

func (e *PathTraversalError) Error() string {
	return fmt.Sprintf("path %s escapes allowed directory %s", e.Path, e.AllowedBase)
}

func (e *PathTraversalError) Unwrap() error {
	return ErrPathTraversal
}

// SymlinkError wraps symlink errors with context
type SymlinkError struct {
	Path string
}

func (e *SymlinkError) Error() string {
	return fmt.Sprintf("symlinks not allowed: %s", e.Path)
}

func (e *SymlinkError) Unwrap() error {
	return ErrSymlink
}

// FileSizeError wraps file size errors with context
type FileSizeError struct {
	Size    int64
	MaxSize int64
}

func (e *FileSizeError) Error() string {
	return fmt.Sprintf("file too large: %d bytes (max %d)", e.Size, e.MaxSize)
}

func (e *FileSizeError) Unwrap() error {
	return ErrFileSize
}

// InvalidExtensionError wraps invalid extension errors with context
type InvalidExtensionError struct {
	Extension string
	Allowed   map[string]bool
}

func (e *InvalidExtensionError) Error() string {
	allowed := make([]string, 0, len(e.Allowed))
	for ext := range e.Allowed {
		allowed = append(allowed, ext)
	}
	return fmt.Sprintf("invalid extension: %s. Allowed: %s", e.Extension, strings.Join(allowed, ", "))
}

func (e *InvalidExtensionError) Unwrap() error {
	return ErrInvalidExtension
}

// ValidatePath validates that a path is within the allowed base directory.
//
// This function prevents path traversal attacks by ensuring the resolved
// path is within the allowed base directory. It also optionally prevents
// symlink following.
//
// Parameters:
//   - path: User-provided path to validate
//   - allowedBase: Base directory that path must be within
//   - followSymlinks: Whether to allow symlinks
//
// Returns the validated absolute path or an error.
func ValidatePath(path, allowedBase string, followSymlinks bool) (string, error) {
	// Resolve allowed base to absolute path
	baseAbs, err := filepath.Abs(allowedBase)
	if err != nil {
		return "", fmt.Errorf("resolving base path: %w", err)
	}

	// Ensure base directory exists
	baseInfo, err := os.Stat(baseAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("allowed base directory does not exist: %s", allowedBase)
		}
		return "", fmt.Errorf("checking base directory: %w", err)
	}

	if !baseInfo.IsDir() {
		return "", &PathTraversalError{Path: path, AllowedBase: allowedBase}
	}

	// Resolve the base through symlinks now that we know it exists.
	// On macOS /var → /private/var; without this the Rel check below produces
	// a false PathTraversalError when the user path is resolved but base is not.
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", fmt.Errorf("evaluating base symlinks: %w", err)
	}

	// Check for symlinks before resolving if not allowed
	if !followSymlinks {
		linkInfo, err := os.Lstat(path)
		if err == nil && linkInfo.Mode()&os.ModeSymlink != 0 {
			return "", &SymlinkError{Path: path}
		}
	}

	// Resolve path to absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	// Clean the path to resolve .. and . components
	cleanPath := filepath.Clean(absPath)

	// Evaluate symlinks for the path. For non-existent paths (e.g. files
	// about to be created) walk up to the first existing ancestor so the
	// resolved result is comparable to the resolved base.
	resolvedPath, err := resolveExisting(cleanPath)
	if err != nil {
		return "", fmt.Errorf("evaluating symlinks: %w", err)
	}

	// Validate path is within allowed base (both sides fully resolved)
	relPath, err := filepath.Rel(baseResolved, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}

	// Check if relative path escapes base (starts with ..)
	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", &PathTraversalError{Path: path, AllowedBase: allowedBase}
	}

	return resolvedPath, nil
}

// ValidateDiagramPath validates a diagram file path with extension checking.
//
// Parameters:
//   - path: Path to diagram file
//   - allowedBase: Base directory for diagrams
//
// Returns the validated absolute path or an error.
func ValidateDiagramPath(path, allowedBase string) (string, error) {
	// First validate path security
	validatedPath, err := ValidatePath(path, allowedBase, false)
	if err != nil {
		return "", err
	}

	// Then validate extension
	ext := strings.ToLower(filepath.Ext(validatedPath))
	if !AllowedDiagramExtensions[ext] {
		return "", &InvalidExtensionError{
			Extension: ext,
			Allowed:   AllowedDiagramExtensions,
		}
	}

	return validatedPath, nil
}

// ValidateOutputPath validates an output file path with extension checking.
//
// Parameters:
//   - path: Path to output file
//   - allowedBase: Base directory for outputs
//
// Returns the validated absolute path or an error.
func ValidateOutputPath(path, allowedBase string) (string, error) {
	// First validate path security
	validatedPath, err := ValidatePath(path, allowedBase, false)
	if err != nil {
		return "", err
	}

	// Then validate extension (allow empty extension)
	ext := strings.ToLower(filepath.Ext(validatedPath))
	if ext != "" && !AllowedOutputExtensions[ext] {
		return "", &InvalidExtensionError{
			Extension: ext,
			Allowed:   AllowedOutputExtensions,
		}
	}

	return validatedPath, nil
}

// SafeReadFile safely reads a file with size validation.
//
// Parameters:
//   - path: Path to file (must already be validated)
//   - maxSize: Maximum file size in bytes (0 for MaxDiagramSize)
//
// Returns the file contents or an error.
func SafeReadFile(path string, maxSize int64) (string, error) {
	if maxSize == 0 {
		maxSize = MaxDiagramSize
	}

	// Check file exists and get size
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}

	// Check file size before reading
	if info.Size() > maxSize {
		return "", &FileSizeError{
			Size:    info.Size(),
			MaxSize: maxSize,
		}
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// SafeCreateDirectory safely creates a directory with validation.
//
// Parameters:
//   - path: Directory path to create
//   - allowedBase: Base directory that path must be within
//   - mode: Directory permissions
//
// Returns the validated absolute path to the created directory or an error.
func SafeCreateDirectory(path, allowedBase string, mode os.FileMode) (string, error) {
	// Resolve allowed base to absolute path
	baseAbs, err := filepath.Abs(allowedBase)
	if err != nil {
		return "", fmt.Errorf("resolving base path: %w", err)
	}

	// Ensure base directory exists
	baseInfo, err := os.Stat(baseAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("allowed base directory does not exist: %s", allowedBase)
		}
		return "", fmt.Errorf("checking base directory: %w", err)
	}

	if !baseInfo.IsDir() {
		return "", &PathTraversalError{Path: path, AllowedBase: allowedBase}
	}

	// Resolve the base through symlinks (see ValidatePath — same macOS fix).
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return "", fmt.Errorf("evaluating base symlinks: %w", err)
	}

	// Check for symlink BEFORE resolving
	pathAbs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving path: %w", err)
	}

	if linkInfo, err := os.Lstat(pathAbs); err == nil {
		if linkInfo.Mode()&os.ModeSymlink != 0 {
			return "", &SymlinkError{Path: path}
		}
	}

	// Clean the path and resolve as much as possible (directory may not exist yet).
	cleanPath := filepath.Clean(pathAbs)
	resolvedClean, err := resolveExisting(cleanPath)
	if err != nil {
		return "", fmt.Errorf("evaluating path symlinks: %w", err)
	}

	// Validate path is within base (both sides fully resolved)
	relPath, err := filepath.Rel(baseResolved, resolvedClean)
	if err != nil {
		return "", fmt.Errorf("computing relative path: %w", err)
	}

	if strings.HasPrefix(relPath, "..") || filepath.IsAbs(relPath) {
		return "", &PathTraversalError{Path: path, AllowedBase: allowedBase}
	}

	// Create directory (use resolvedClean so the path matches the resolved base)
	if err := os.MkdirAll(resolvedClean, mode); err != nil {
		return "", fmt.Errorf("creating directory: %w", err)
	}

	return resolvedClean, nil
}

// IsSafePath checks if a path is safe without raising errors.
//
// Parameters:
//   - path: Path to check
//   - allowedBase: Base directory
//
// Returns true if the path is safe, false otherwise.
func IsSafePath(path, allowedBase string) bool {
	_, err := ValidatePath(path, allowedBase, false)
	return err == nil
}

// GetSafeTempDir returns a safe temporary directory for diagram operations.
//
// Returns the path to the system temporary directory.
func GetSafeTempDir() string {
	return os.TempDir()
}

// resolveExisting resolves as many leading path components as possible through
// filepath.EvalSymlinks, then appends the unresolved suffix. This handles
// paths that do not yet exist (e.g. a file about to be created) so they can
// be compared against a fully-resolved base directory.
//
// On macOS, os.TempDir() returns /var/… while EvalSymlinks resolves it to
// /private/var/… — without this, Rel comparisons produce false ../ prefixes.
func resolveExisting(path string) (string, error) {
	if _, err := os.Stat(path); err == nil {
		return filepath.EvalSymlinks(path)
	}
	parent := filepath.Dir(path)
	if parent == path {
		// Reached the filesystem root; return as-is.
		return path, nil
	}
	resolvedParent, err := resolveExisting(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}
