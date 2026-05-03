// Package status provides status-related functionality.
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

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
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
		if phase.Status == PhaseStatusInProgress {
			suffix = "** (in progress)"
		} else if phase.Status == PhaseStatusCompleted && phase.CompletedAt != nil && phase.StartedAt != nil {
			duration := (*phase.CompletedAt).Sub(*phase.StartedAt)
			suffix = fmt.Sprintf(" (%s)", formatDuration(duration))
		}

		lines = append(lines, fmt.Sprintf("- %s %s%s", checkbox, phase.Name, suffix))
	}

	// Add remaining phases as pending
	existing := make(map[string]bool)
	for _, phase := range s.Phases {
		existing[phase.Name] = true
	}
	for _, phaseName := range AllPhases() {
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
// Returns "D1" if no current phase is set
func (s *Status) NextPhase() (string, error) {
	if s.CurrentPhase == "" {
		return "D1", nil // Start with D1 if no current phase
	}

	allPhases := AllPhases()
	currentIdx := -1

	// Find current phase index
	for i, phase := range allPhases {
		if phase == s.CurrentPhase {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return "", fmt.Errorf("invalid current phase: %s", s.CurrentPhase)
	}

	if currentIdx == len(allPhases)-1 {
		return "", fmt.Errorf("already at final phase S11")
	}

	return allPhases[currentIdx+1], nil
}
