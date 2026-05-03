package reflection

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Store handles persistence of reflections as .ai.md files
type Store struct {
	reflectionPath string
	detector       *FailureDetector // Task 1.1.2: Auto-detect failures
}

// NewStore creates a new reflection store
func NewStore(reflectionPath string) *Store {
	return &Store{
		reflectionPath: reflectionPath,
		detector:       NewFailureDetector(), // Task 1.1.2: Initialize failure detector
	}
}

// Save saves a reflection as an engram
func (s *Store) Save(r *Reflection) error {
	// Create reflections directory if needed
	if err := os.MkdirAll(s.reflectionPath, 0o700); err != nil {
		return fmt.Errorf("failed to create reflections directory: %w", err)
	}

	// Generate filename from timestamp and session ID
	filename := fmt.Sprintf("%s-%s.ai.md",
		r.Timestamp.Format("2006-01-02-15-04-05"),
		r.SessionID[:8])
	path := filepath.Join(s.reflectionPath, filename)

	// Build frontmatter
	frontmatter := map[string]interface{}{
		"type":        "strategy", // Reflections are strategies (procedural learning)
		"title":       fmt.Sprintf("Reflection: %s", r.Trigger.Description),
		"description": r.Trigger.Description,
		"tags":        append([]string{"reflections", string(r.Trigger.Type)}, r.Tags...),
		"modified":    r.Timestamp,
		"session_id":  r.SessionID,
	}

	// Add failure tracking fields if present (Task 1.1.1: Mistake Notebook)
	if r.Outcome != "" {
		frontmatter["outcome"] = string(r.Outcome)
	}
	if r.ErrorCategory != "" {
		frontmatter["error_category"] = string(r.ErrorCategory)
	}
	if r.LessonLearned != "" {
		frontmatter["lesson_learned"] = r.LessonLearned
	}

	// Marshal frontmatter
	fmData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Build full content
	var contentBuilder string
	contentBuilder = fmt.Sprintf("---\n%s---\n\n# %s\n\n%s\n\n## Trigger\n\n%s\n\n",
		string(fmData),
		r.Trigger.Description,
		r.Learning,
		r.Trigger.Description)

	// Add failure tracking section if this is a failure (Task 1.1.1)
	if r.Outcome == OutcomeFailure && r.LessonLearned != "" {
		contentBuilder += fmt.Sprintf("## Lesson Learned\n\n%s\n\n**Error Category**: %s\n\n",
			r.LessonLearned,
			r.ErrorCategory)
	}

	// Add session metrics
	contentBuilder += fmt.Sprintf("## Session Metrics\n\n- Duration: %s\n- Files modified: %d\n- Lines changed: %d\n- Commands executed: %d\n- Errors encountered: %d\n",
		r.Metrics.Duration.String(),
		r.Metrics.FilesModified,
		r.Metrics.LinesChanged,
		r.Metrics.CommandsExecuted,
		r.Metrics.ErrorsEncountered)

	content := contentBuilder

	// Write file
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write reflection: %w", err)
	}

	return nil
}

// SaveWithAutoDetect saves a reflection with automatic failure detection
// (Task 1.1.2: Main entry point for auto-tagging failures)
func (s *Store) SaveWithAutoDetect(r *Reflection, context DetectionContext) error {
	// Auto-detect and enrich reflection with failure tracking fields
	s.detector.EnrichReflection(r, context)

	// Save the enriched reflection
	return s.Save(r)
}

// List returns all reflections
func (s *Store) List() ([]*Reflection, error) {
	entries, err := os.ReadDir(s.reflectionPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var reflections []*Reflection
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// See https://github.com/vbonnet/dear-agent/engram/issues/6 for reflection file parsing implementation
		// For now just return empty list
	}

	return reflections, nil
}
