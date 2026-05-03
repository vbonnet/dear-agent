package status

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

// New creates a new Status with default values
func New(projectPath string) *Status {
	now := time.Now()
	return &Status{
		SchemaVersion: SchemaVersion,
		SessionID:     uuid.New().String(),
		ProjectPath:   projectPath,
		StartedAt:     now,
		Status:        StatusInProgress,
		Phases:        []Phase{},
	}
}

// Read reads STATUS file from current directory
func Read() (*Status, error) {
	return ReadFrom(".")
}

// ReadFrom reads STATUS file from specified directory
func ReadFrom(dir string) (*Status, error) {
	path := filepath.Join(dir, StatusFilename)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", StatusFilename, err)
	}

	// Extract YAML frontmatter
	yamlContent, err := extractFrontmatter(string(data))
	if err != nil {
		return nil, err
	}

	var status Status
	if err := yaml.Unmarshal([]byte(yamlContent), &status); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &status, nil
}

// Write writes STATUS file to current directory
func (s *Status) Write() error {
	return s.WriteTo(".")
}

// WriteTo writes STATUS file to specified directory
func (s *Status) WriteTo(dir string) error {
	path := filepath.Join(dir, StatusFilename)

	// Marshal to YAML
	yamlData, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML: %w", err)
	}

	// Create content with YAML frontmatter + Markdown body
	content := fmt.Sprintf("---\n%s---\n\n# Wayfinder Session\n\n**Session ID**: %s\n**Project**: %s\n**Started**: %s\n**Status**: %s\n**Current Phase**: %s\n\n## Phase Progress\n\n%s\n",
		string(yamlData),
		s.SessionID,
		s.ProjectPath,
		s.StartedAt.Format("2006-01-02 15:04 MST"),
		s.Status,
		s.CurrentPhase,
		s.formatPhaseList(),
	)

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", StatusFilename, err)
	}

	return nil
}

// FindPhase finds a phase by name, returns nil if not found
func (s *Status) FindPhase(name string) *Phase {
	for i := range s.Phases {
		if s.Phases[i].Name == name {
			return &s.Phases[i]
		}
	}
	return nil
}

// AddPhase adds a new phase if it doesn't exist
func (s *Status) AddPhase(name string) *Phase {
	if phase := s.FindPhase(name); phase != nil {
		return phase
	}

	newPhase := Phase{
		Name:   name,
		Status: PhaseStatusPending,
	}
	s.Phases = append(s.Phases, newPhase)
	return &s.Phases[len(s.Phases)-1]
}

// UpdatePhase updates an existing phase or creates it
func (s *Status) UpdatePhase(name string, status string, outcome string) {
	phase := s.AddPhase(name)
	now := time.Now()

	phase.Status = status
	if status == PhaseStatusInProgress && phase.StartedAt == nil {
		phase.StartedAt = &now
	}
	if status == PhaseStatusCompleted && phase.CompletedAt == nil {
		phase.CompletedAt = &now
		phase.Outcome = outcome
	}

	// Update in slice (since we have a copy)
	for i := range s.Phases {
		if s.Phases[i].Name == name {
			s.Phases[i] = *phase
			break
		}
	}
}

// formatPhaseList creates a markdown checklist of phases
func (s *Status) formatPhaseList() string {
	// Defensive: handle nil Status
	if s == nil {
		return ""
	}

	var lines []string
	for _, phase := range s.Phases {
		var checkbox string
		switch phase.Status {
		case PhaseStatusCompleted:
			checkbox = "[x]"
		case PhaseStatusInProgress:
			checkbox = "[ ] **"
		default:
			checkbox = "[ ]"
		}

		var suffix string
		switch phase.Status {
		case PhaseStatusInProgress:
			suffix = "** (in progress)"
		case PhaseStatusCompleted:
			// Additional nil checks to prevent SIGSEGV
			if phase.CompletedAt != nil && phase.StartedAt != nil {
				duration := (*phase.CompletedAt).Sub(*phase.StartedAt)
				suffix = fmt.Sprintf(" (%s)", formatDuration(duration))
			}
		}

		lines = append(lines, fmt.Sprintf("- %s %s%s", checkbox, phase.Name, suffix))
	}

	// Add remaining phases as pending
	existing := make(map[string]bool)
	for _, phase := range s.Phases {
		existing[phase.Name] = true
	}
	version := s.GetVersion()
	for _, phaseName := range AllPhases(version) {
		if !existing[phaseName] {
			lines = append(lines, fmt.Sprintf("- [ ] %s", phaseName))
		}
	}

	return strings.Join(lines, "\n")
}

// extractFrontmatter extracts YAML between --- delimiters
func extractFrontmatter(content string) (string, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return "", fmt.Errorf("invalid frontmatter: must start with ---")
	}

	var yamlLines []string
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			return strings.Join(yamlLines, "\n"), nil
		}
		yamlLines = append(yamlLines, lines[i])
	}

	return "", fmt.Errorf("invalid frontmatter: missing closing ---")
}

// formatDuration formats a duration in human-readable form
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if minutes == 0 {
		return fmt.Sprintf("%dh", hours)
	}
	return fmt.Sprintf("%dh %dm", hours, minutes)
}

// NextPhase returns the next phase in sequence based on current_phase
// Returns error if:
// - Current phase is S11 (last phase)
// - Current phase is invalid
// Returns first phase if no current phase is set (v1: "W0", v2: "discovery.problem")
//
// IMPORTANT: Returns current phase if it's not completed yet.
// Only advances to next phase if current phase is marked as completed.
func (s *Status) NextPhase() (string, error) {
	version := s.GetVersion()
	allPhases := AllPhases(version)

	// Handle empty current phase
	if s.CurrentPhase == "" {
		return s.handleEmptyCurrentPhase(version, allPhases)
	}

	// Find and validate current phase
	currentIdx := s.findPhaseIndex(s.CurrentPhase, allPhases)
	if currentIdx == -1 {
		return "", fmt.Errorf("invalid current phase: %s", s.CurrentPhase)
	}

	// Check if at final phase
	finalPhaseName := s.getFinalPhaseName(version)
	if s.isAtFinalPhase(currentIdx, allPhases) {
		return "", fmt.Errorf("already at final phase %s", finalPhaseName)
	}

	// Check if current phase is completed
	if !s.isPhaseCompleted(s.CurrentPhase) {
		return s.CurrentPhase, nil
	}

	// Advance to next phase
	return s.advanceToNextPhase(currentIdx, allPhases, version, finalPhaseName)
}

// handleEmptyCurrentPhase returns the first phase when no current phase is set.
func (s *Status) handleEmptyCurrentPhase(version string, allPhases []string) (string, error) {
	if len(allPhases) > 0 {
		return allPhases[0], nil
	}
	// Fallback (should never happen with proper AllPhases implementation)
	if version == WayfinderV2 {
		return "CHARTER", nil
	}
	return "W0", nil
}

// findPhaseIndex finds the index of a phase in the phases list.
func (s *Status) findPhaseIndex(phaseName string, allPhases []string) int {
	for i, phase := range allPhases {
		if phase == phaseName {
			return i
		}
	}
	return -1
}

// getFinalPhaseName returns the final phase name based on version.
func (s *Status) getFinalPhaseName(version string) string {
	if version == WayfinderV2 {
		return "RETRO"
	}
	return "S11"
}

// isAtFinalPhase checks if we're at the final phase.
func (s *Status) isAtFinalPhase(currentIdx int, allPhases []string) bool {
	return currentIdx == len(allPhases)-1
}

// isPhaseCompleted checks if a phase is marked as completed or skipped.
func (s *Status) isPhaseCompleted(phaseName string) bool {
	for _, p := range s.Phases {
		if p.Name == phaseName {
			return p.Status == PhaseStatusCompleted || p.Status == PhaseStatusSkipped
		}
	}
	return false
}

// advanceToNextPhase advances to the next phase, skipping roadmap phases if needed.
func (s *Status) advanceToNextPhase(currentIdx int, allPhases []string, version string, finalPhaseName string) (string, error) {
	nextIdx := currentIdx + 1
	nextPhase := allPhases[nextIdx]

	// Skip roadmap phases if SkipRoadmap is true (v2 only)
	if version == WayfinderV2 && s.SkipRoadmap {
		nextIdx = s.skipRoadmapPhases(nextIdx, allPhases)
		if nextIdx >= len(allPhases) {
			return "", fmt.Errorf("already at final phase %s", finalPhaseName)
		}
		nextPhase = allPhases[nextIdx]
	}

	return nextPhase, nil
}

// skipRoadmapPhases skips the roadmap phase (SETUP in V2) and returns the next index.
func (s *Status) skipRoadmapPhases(startIdx int, allPhases []string) int {
	idx := startIdx
	// V2: SETUP is the roadmap phase (consolidated planning+breakdown+dependencies)
	// V1: S7 is the roadmap phase
	for idx < len(allPhases) && (allPhases[idx] == "SETUP" || allPhases[idx] == "S7") {
		idx++
	}
	return idx
}

// Load reads a status file from the specified directory
// This is a convenience function that wraps ReadFrom
func Load(projectPath string) (*Status, error) {
	return ReadFrom(projectPath)
}

// Save writes a status file to the specified directory
// This is a convenience function that wraps WriteTo
func Save(projectPath string, st *Status) error {
	return st.WriteTo(projectPath)
}

// StatusInterface is a common interface for both Status and StatusV2
// Allows commands to work with either version
type StatusInterface interface {
	UpdatePhase(phaseName string, phaseStatus string, outcome string)
	GetSessionID() string
	WriteTo(dir string) error
	GetVersion() string
	FindPhase(phaseName string) *Phase
	GetCurrentPhase() string
	SetCurrentPhase(phase string)
	GetStartedAt() time.Time
	GetSkipRoadmap() bool
}

// GetSessionID returns the session ID for V1 Status
func (s *Status) GetSessionID() string {
	return s.SessionID
}

// GetCurrentPhase returns the current phase for V1 Status
func (s *Status) GetCurrentPhase() string {
	return s.CurrentPhase
}

// GetStartedAt returns the started time for V1 Status
func (s *Status) GetStartedAt() time.Time {
	return s.StartedAt
}

// SetCurrentPhase sets the current phase for V1 Status
func (s *Status) SetCurrentPhase(phase string) {
	s.CurrentPhase = phase
}

// GetSkipRoadmap returns the skip_roadmap flag for V1 Status
func (s *Status) GetSkipRoadmap() bool {
	return s.SkipRoadmap
}

// DetectSchemaVersionFromDir detects the schema version from a directory
// Wrapper around DetectSchemaVersion that takes a directory path
func DetectSchemaVersionFromDir(dir string) (string, error) {
	path := filepath.Join(dir, StatusFilename)
	return DetectSchemaVersion(path)
}

// LoadAnyVersion reads WAYFINDER-STATUS.md and returns the appropriate struct
// Returns (*Status, nil) for V1 files
// Returns (*StatusV2, nil) for V2 files
// Returns StatusInterface which both types implement
func LoadAnyVersion(dir string) (StatusInterface, error) {
	version, err := DetectSchemaVersionFromDir(dir)
	if err != nil {
		return nil, err
	}

	if version == SchemaVersionV2 {
		return ParseV2FromDir(dir)
	}

	// Default to V1 (backward compatibility)
	return ReadFrom(dir)
}

// SaveAnyVersion writes WAYFINDER-STATUS.md using the appropriate format
// Accepts either *Status (V1) or *StatusV2 (V2)
func SaveAnyVersion(dir string, v interface{}) error {
	switch st := v.(type) {
	case *StatusV2:
		return WriteV2ToDir(st, dir)
	case *Status:
		return st.WriteTo(dir)
	default:
		return fmt.Errorf("unsupported status type: %T (must be *Status or *StatusV2)", v)
	}
}
