package ui

import (
	"time"

	"github.com/charmbracelet/huh"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// CleanupMultiSelect shows multi-select UI for batch cleanup
func CleanupMultiSelect(sessions []*Session, cfg *Config) (*CleanupResult, error) {
	// Group sessions by status and age
	stopped := filterByAge(filterStopped(sessions), cfg.Defaults.CleanupThresholdDays)
	archived := filterByAge(filterArchived(sessions), cfg.Defaults.ArchiveThresholdDays)

	// Build options
	stoppedOpts := make([]huh.Option[string], len(stopped))
	archivedOpts := make([]huh.Option[string], len(archived))
	sessionMap := make(map[string]*Session)

	for i, s := range stopped {
		label := formatCleanupOption(s)
		stoppedOpts[i] = huh.NewOption(label, s.Name)
		sessionMap[s.Name] = s
	}

	for i, s := range archived {
		label := formatCleanupOption(s)
		archivedOpts[i] = huh.NewOption(label, s.Name)
		sessionMap[s.Name] = s
	}

	// Multi-select form
	var toArchive, toDelete []string

	groups := []*huh.Group{}

	// Add stopped sessions group if any
	if len(stoppedOpts) > 0 {
		groups = append(groups, huh.NewGroup(
			huh.NewNote().
				Title("Stopped Sessions (>30 days)").
				Description("Suggested for archival"),
			huh.NewMultiSelect[string]().
				Title("Select sessions to archive:").
				Options(stoppedOpts...).
				Value(&toArchive).
				Limit(20),
		))
	}

	// Add archived sessions group if any
	if len(archivedOpts) > 0 {
		groups = append(groups, huh.NewGroup(
			huh.NewNote().
				Title("Archived Sessions (>90 days)").
				Description("Suggested for deletion"),
			huh.NewMultiSelect[string]().
				Title("Select sessions to delete:").
				Options(archivedOpts...).
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

	// Build result
	result := &CleanupResult{
		ToArchive: make([]*Session, 0, len(toArchive)),
		ToDelete:  make([]*Session, 0, len(toDelete)),
	}

	for _, name := range toArchive {
		if s, ok := sessionMap[name]; ok {
			result.ToArchive = append(result.ToArchive, s)
		}
	}

	for _, name := range toDelete {
		if s, ok := sessionMap[name]; ok {
			result.ToDelete = append(result.ToDelete, s)
		}
	}

	return result, nil
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
