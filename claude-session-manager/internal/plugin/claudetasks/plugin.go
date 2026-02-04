package claudetasks

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/roadmap"
)

// Plugin implements TaskManagerPlugin for Claude Code tasks via ROADMAP.md
type Plugin struct{}

// NewPlugin creates a new Claude tasks plugin instance
func NewPlugin() *Plugin {
	return &Plugin{}
}

// Metadata returns plugin information
func (p *Plugin) Metadata() plugin.PluginMetadata {
	return plugin.PluginMetadata{
		Name:        "claude-tasks",
		Version:     "1.0.0",
		Author:      "CSM Team",
		Description: "Claude Code task tracker via ROADMAP.md",
	}
}

// GetTasks retrieves all tasks from ROADMAP.md in the session directory
func (p *Plugin) GetTasks(sessionDir string) ([]plugin.Task, error) {
	roadmapPath := filepath.Join(sessionDir, "ROADMAP.md")

	// Check if ROADMAP.md exists
	if _, err := os.Stat(roadmapPath); os.IsNotExist(err) {
		return []plugin.Task{}, nil // No ROADMAP.md = no tasks (not an error)
	}

	// Parse ROADMAP.md
	beads, err := roadmap.ParseROADMAP(roadmapPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ROADMAP.md: %w", err)
	}

	// Convert beads to tasks
	tasks := make([]plugin.Task, 0, len(beads))
	for _, bead := range beads {
		tasks = append(tasks, plugin.Task{
			ID:          bead.ID,
			Title:       bead.Description,
			Description: bead.Description,
			Status:      bead.Status,
			Phase:       bead.Phase,
			Labels:      []string{bead.Phase}, // Use phase as label
			Metadata: map[string]string{
				"effort":     bead.Effort,
				"raw_status": bead.RawStatus,
				"source":     "roadmap",
			},
			CreatedAt: time.Now(), // ROADMAP doesn't track creation time
			UpdatedAt: time.Now(),
		})
	}

	return tasks, nil
}

// GetPhaseProgress calculates per-phase completion statistics from tasks
func (p *Plugin) GetPhaseProgress(sessionDir string) ([]plugin.PhaseStats, error) {
	tasks, err := p.GetTasks(sessionDir)
	if err != nil {
		return nil, err
	}

	// Group tasks by phase
	phaseMap := make(map[string]*plugin.PhaseStats)
	for _, task := range tasks {
		phase := task.Phase
		if phase == "" {
			phase = "phase-unknown"
		}

		if _, exists := phaseMap[phase]; !exists {
			phaseMap[phase] = &plugin.PhaseStats{
				Phase: phase,
			}
		}

		stats := phaseMap[phase]
		stats.Total++

		switch task.Status {
		case "pending":
			stats.Pending++
		case "in_progress":
			stats.InProgress++
		case "completed":
			stats.Completed++
		case "blocked":
			stats.Blocked++
		case "cancelled":
			stats.Cancelled++
		}
	}

	// Calculate percentages
	for _, stats := range phaseMap {
		if stats.Total > 0 {
			stats.Percentage = float64(stats.Completed) / float64(stats.Total) * 100.0
		}
	}

	// Convert map to slice
	result := make([]plugin.PhaseStats, 0, len(phaseMap))
	for _, stats := range phaseMap {
		result = append(result, *stats)
	}

	return result, nil
}

// SupportsSession checks if session has a ROADMAP.md file
func (p *Plugin) SupportsSession(sessionDir string) bool {
	roadmapPath := filepath.Join(sessionDir, "ROADMAP.md")
	_, err := os.Stat(roadmapPath)
	return err == nil
}
