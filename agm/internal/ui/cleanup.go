package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// CleanupMultiSelect shows multi-select UI for batch cleanup
func CleanupMultiSelect(sessions []*Session, cfg *Config) (*CleanupResult, error) {
	// Group sessions by status and age
	stopped := filterByAge(filterStopped(sessions), cfg.Defaults.CleanupThresholdDays)
	archived := filterByAge(filterArchived(sessions), cfg.Defaults.ArchiveThresholdDays)

	choices := buildCleanupOptions(stopped, archived)

	// Multi-select form
	var toArchive, toDelete []string

	groups := []*huh.Group{}

	// Add stopped sessions group if any
	if len(choices.stopped) > 0 {
		groups = append(groups, huh.NewGroup(
			huh.NewNote().
				Title("Stopped Sessions (>30 days)").
				Description("Suggested for archival"),
			huh.NewMultiSelect[string]().
				Title("Select sessions to archive:").
				Options(choices.stopped...).
				Value(&toArchive).
				Limit(20),
		))
	}

	// Add archived sessions group if any
	if len(choices.archived) > 0 {
		groups = append(groups, huh.NewGroup(
			huh.NewNote().
				Title("Archived Sessions (>90 days)").
				Description("Suggested for deletion"),
			huh.NewMultiSelect[string]().
				Title("Select sessions to delete:").
				Options(choices.archived...).
				Value(&toDelete).
				Limit(20),
		))
	}

	if len(groups) == 0 {
		return &CleanupResult{}, nil
	}

	form := huh.NewForm(groups...).WithTheme(getTheme(cfg.UI.Theme))

	if err := form.Run(); err != nil {
		return nil, err
	}

	return choices.result(toArchive, toDelete), nil
}

type cleanupOptions struct {
	stopped  []huh.Option[string]
	archived []huh.Option[string]
	byID     map[string]*Session
}

func buildCleanupOptions(stopped, archived []*Session) cleanupOptions {
	choices := cleanupOptions{
		stopped:  make([]huh.Option[string], len(stopped)),
		archived: make([]huh.Option[string], len(archived)),
		byID:     make(map[string]*Session, len(stopped)+len(archived)),
	}
	for i, s := range stopped {
		choices.stopped[i] = huh.NewOption(cleanupOptionLabel(s), s.SessionID)
		choices.byID[s.SessionID] = s
	}
	for i, s := range archived {
		choices.archived[i] = huh.NewOption(cleanupOptionLabel(s), s.SessionID)
		choices.byID[s.SessionID] = s
	}
	return choices
}

func cleanupOptionLabel(s *Session) string {
	return fmt.Sprintf("[ID: %q] %q", s.SessionID, formatCleanupOption(s))
}

func (choices cleanupOptions) result(toArchive, toDelete []string) *CleanupResult {
	result := &CleanupResult{
		ToArchive: make([]*Session, 0, len(toArchive)),
		ToDelete:  make([]*Session, 0, len(toDelete)),
	}

	for _, id := range toArchive {
		if s, ok := choices.byID[id]; ok {
			result.ToArchive = append(result.ToArchive, s)
		}
	}

	for _, id := range toDelete {
		if s, ok := choices.byID[id]; ok {
			result.ToDelete = append(result.ToDelete, s)
		}
	}

	return result
}

func filterStopped(sessions []*Session) []*Session {
	var result []*Session
	for _, s := range sessions {
		if s.Status == "stopped" {
			result = append(result, s)
		}
	}
	return result
}

func filterArchived(sessions []*Session) []*Session {
	var result []*Session
	for _, s := range sessions {
		if s.Lifecycle == manifest.LifecycleArchived {
			result = append(result, s)
		}
	}
	return result
}

func filterByAge(sessions []*Session, days int) []*Session {
	if days <= 0 {
		return sessions
	}

	cutoff := time.Now().AddDate(0, 0, -days)
	var result []*Session

	for _, s := range sessions {
		if s.UpdatedAt.Before(cutoff) {
			result = append(result, s)
		}
	}

	return result
}

func formatCleanupOption(s *Session) string {
	age := formatRelativeTime(s.UpdatedAt)
	project := s.Context.Project
	if len(project) > 30 {
		project = "..." + project[len(project)-27:]
	}
	return s.Name + " (" + age + ") - " + project
}
