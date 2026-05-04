// Package pivot detects when a session pivots from implementation to planning/docs.
//
// It maintains a sliding window of recent file operations and classifies each
// file as "code" or "docs" based on extension. When the docs ratio exceeds a
// threshold, a pivot is detected.
package pivot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// WindowSize is the number of recent file operations to track.
const WindowSize = 10

// PivotThreshold is the fraction of docs operations that triggers a pivot alert.
// >50% docs in the window = pivot detected.
const PivotThreshold = 0.5

// docExtensions are file extensions classified as documentation/planning.
var docExtensions = map[string]bool{
	".md":      true,
	".mdx":     true,
	".txt":     true,
	".rst":     true,
	".adoc":    true,
	".wiki":    true,
	".org":     true,
	".textile": true,
}

// codeExtensions are file extensions classified as implementation code.
var codeExtensions = map[string]bool{
	".go":      true,
	".py":      true,
	".js":      true,
	".ts":      true,
	".tsx":     true,
	".jsx":     true,
	".rs":      true,
	".java":    true,
	".c":       true,
	".cpp":     true,
	".h":       true,
	".rb":      true,
	".sh":      true,
	".bash":    true,
	".zsh":     true,
	".cs":      true,
	".swift":   true,
	".kt":      true,
	".scala":   true,
	".zig":     true,
	".lua":     true,
	".r":       true,
	".sql":     true,
	".proto":   true,
	".graphql": true,
}

// FileKind classifies a file as code, docs, or other.
type FileKind string

// FileKind classification values.
const (
	KindCode  FileKind = "code"
	KindDocs  FileKind = "docs"
	KindOther FileKind = "other"
)

// FileEntry records a single file operation in the sliding window.
type FileEntry struct {
	Path      string    `json:"path"`
	Kind      FileKind  `json:"kind"`
	ToolName  string    `json:"tool_name"`
	Timestamp time.Time `json:"timestamp"`
}

// State holds the pivot detection state persisted between hook invocations.
type State struct {
	// Window is the sliding window of recent file operations.
	Window []FileEntry `json:"window"`
	// AlertedAt records when the last pivot alert was emitted.
	// Alerts are suppressed for CooldownDuration after the last alert.
	AlertedAt *time.Time `json:"alerted_at,omitempty"`
	// InitialKind records the dominant file kind from the first few operations,
	// establishing the session's expected work type.
	InitialKind FileKind `json:"initial_kind,omitempty"`
	// CodeCount is the total number of code file operations seen.
	CodeCount int `json:"code_count"`
	// DocsCount is the total number of docs file operations seen.
	DocsCount int `json:"docs_count"`
}

// CooldownDuration is the minimum time between pivot alerts.
const CooldownDuration = 5 * time.Minute

// ClassifyFile returns the FileKind for a given file path.
func ClassifyFile(path string) FileKind {
	ext := strings.ToLower(filepath.Ext(path))
	if docExtensions[ext] {
		return KindDocs
	}
	if codeExtensions[ext] {
		return KindCode
	}

	// Check for common doc filenames without extension
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "readme", "changelog", "contributing", "license", "authors":
		return KindDocs
	}

	return KindOther
}

// fileTools are tools that operate on files and should be tracked.
var fileTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"Read":         true,
	"NotebookEdit": true,
}

// IsFileOp returns true if the tool name is a file operation worth tracking.
func IsFileOp(toolName string) bool {
	return fileTools[toolName]
}

// IsWriteOp returns true if the tool name is a write/edit operation.
// Write operations carry more weight for pivot detection than reads.
func IsWriteOp(toolName string) bool {
	return toolName == "Edit" || toolName == "Write"
}

// DefaultStatePath returns the default pivot state file path.
func DefaultStatePath() string {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/tmp"
	}
	return filepath.Join(home, ".claude", "pivot-state.json")
}

// LoadState reads pivot state from disk.
func LoadState(path string) State {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}
	}
	return state
}

// SaveState writes pivot state to disk.
func SaveState(path string, state State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// RecordFileOp adds a file operation to the sliding window and updates counts.
func (s *State) RecordFileOp(path, toolName string) {
	kind := ClassifyFile(path)
	if kind == KindOther {
		return
	}

	entry := FileEntry{
		Path:      path,
		Kind:      kind,
		ToolName:  toolName,
		Timestamp: time.Now(),
	}

	s.Window = append(s.Window, entry)

	// Trim to window size
	if len(s.Window) > WindowSize {
		s.Window = s.Window[len(s.Window)-WindowSize:]
	}

	// Update global counts
	switch kind {
	case KindCode:
		s.CodeCount++
	case KindDocs:
		s.DocsCount++
	case KindOther:
		// Other file types don't affect code/docs counts
	}

	// Establish initial kind from first 3 operations
	if s.InitialKind == "" && (s.CodeCount+s.DocsCount) >= 3 {
		if s.CodeCount > s.DocsCount {
			s.InitialKind = KindCode
		} else if s.DocsCount > s.CodeCount {
			s.InitialKind = KindDocs
		}
	}
}

// PivotResult contains the result of a pivot check.
type PivotResult struct {
	// Detected is true if a pivot was detected.
	Detected bool
	// DocsRatio is the fraction of docs operations in the current window.
	DocsRatio float64
	// WindowSize is the number of entries in the current window.
	WindowSize int
	// Suppressed is true if the pivot was detected but alert is in cooldown.
	Suppressed bool
	// RecentDocFiles lists the doc files from the current window.
	RecentDocFiles []string
}

// CheckPivot analyzes the current window for a pivot from code to docs.
// A pivot is only detected when:
//  1. The session started with code (InitialKind == KindCode)
//  2. The window has enough entries (>= WindowSize/2)
//  3. The docs ratio exceeds PivotThreshold
//  4. At least some of the docs operations are writes (not just reads)
func (s *State) CheckPivot() PivotResult {
	result := PivotResult{}

	// Need initial kind established and it must be code
	if s.InitialKind != KindCode {
		return result
	}

	// Need minimum entries
	minEntries := WindowSize / 2
	if len(s.Window) < minEntries {
		return result
	}

	var docsCount, docsWriteCount int
	for _, e := range s.Window {
		if e.Kind == KindDocs {
			docsCount++
			if IsWriteOp(e.ToolName) {
				docsWriteCount++
			}
			result.RecentDocFiles = append(result.RecentDocFiles, e.Path)
		}
	}

	result.WindowSize = len(s.Window)
	result.DocsRatio = float64(docsCount) / float64(len(s.Window))

	// Must exceed threshold AND have at least one docs write
	if result.DocsRatio > PivotThreshold && docsWriteCount > 0 {
		result.Detected = true

		// Check cooldown
		if s.AlertedAt != nil && time.Since(*s.AlertedAt) < CooldownDuration {
			result.Suppressed = true
		}
	}

	return result
}

// MarkAlerted records that a pivot alert was emitted.
func (s *State) MarkAlerted() {
	now := time.Now()
	s.AlertedAt = &now
}
