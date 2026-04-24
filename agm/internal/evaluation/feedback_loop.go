package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// Example represents a test example with input/output pairs.
type Example struct {
	ID        string    `json:"id"`
	Prompt    string    `json:"prompt"`
	Response  string    `json:"response"`
	Score     float64   `json:"score"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"` // "production", "manual", etc.
	Validated bool      `json:"validated"`
	TraceID   string    `json:"trace_id,omitempty"`
}

// CaptureTraceID populates the TraceID field from the span context in ctx.
func (e *Example) CaptureTraceID(ctx context.Context) {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		e.TraceID = sc.TraceID().String()
	}
}

// FileSystem defines the interface for file operations (for testing).
type FileSystem interface {
	WriteFile(path string, data []byte, perm os.FileMode) error
	ReadDir(path string) ([]os.DirEntry, error)
	MkdirAll(path string, perm os.FileMode) error
}

// OSFileSystem implements FileSystem using standard library.
type OSFileSystem struct{}

// WriteFile writes data to a file at the specified path.
func (fs *OSFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// ReadDir reads the directory at the specified path.
func (fs *OSFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}

// MkdirAll creates a directory and all necessary parent directories.
func (fs *OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// PRCreator defines the interface for creating pull requests.
type PRCreator interface {
	CreatePR(ctx context.Context, title, body string, files []string) (string, error)
}

// FeedbackLoop manages the feedback loop from production to golden dataset.
type FeedbackLoop struct {
	goldenDir string
	fs        FileSystem
	prCreator PRCreator
	validator ExampleValidator
}

// ExampleValidator validates examples before adding to golden dataset.
type ExampleValidator interface {
	Validate(example Example) error
}

// DefaultValidator implements basic validation rules.
type DefaultValidator struct {
	piiPatterns []*regexp.Regexp
}

// NewDefaultValidator creates a validator with PII detection patterns.
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{
		piiPatterns: []*regexp.Regexp{
			// Email addresses
			regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),
			// Credit card numbers (simple pattern)
			regexp.MustCompile(`\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`),
			// Social Security Numbers
			regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
			// Phone numbers
			regexp.MustCompile(`\b\d{3}[-.]?\d{3}[-.]?\d{4}\b`),
			// API keys (common patterns)
			regexp.MustCompile(`(?i)(api[_-]?key|apikey|auth[_-]?token|secret)["\s:=]+[a-z0-9_-]{16,}`),
		},
	}
}

// Validate checks if an example is safe to add to the dataset.
func (dv *DefaultValidator) Validate(example Example) error {
	// Check for empty content
	if strings.TrimSpace(example.Prompt) == "" {
		return fmt.Errorf("example has empty prompt")
	}
	if strings.TrimSpace(example.Response) == "" {
		return fmt.Errorf("example has empty response")
	}

	// Check for PII in prompt
	for _, pattern := range dv.piiPatterns {
		if pattern.MatchString(example.Prompt) {
			return fmt.Errorf("prompt contains potential PII: %s", pattern.String())
		}
		if pattern.MatchString(example.Response) {
			return fmt.Errorf("response contains potential PII: %s", pattern.String())
		}
	}

	// Check for error indicators
	errorIndicators := []string{
		"error:",
		"exception:",
		"failed to",
		"panic:",
		"stack trace",
	}
	lowerResponse := strings.ToLower(example.Response)
	for _, indicator := range errorIndicators {
		if strings.Contains(lowerResponse, indicator) {
			return fmt.Errorf("response contains error indicator: %s", indicator)
		}
	}

	// Example must be validated
	if !example.Validated {
		return fmt.Errorf("example has not been validated")
	}

	return nil
}

// NewFeedbackLoop creates a new feedback loop manager.
func NewFeedbackLoop(goldenDir string, fs FileSystem, prCreator PRCreator, validator ExampleValidator) *FeedbackLoop {
	if validator == nil {
		validator = NewDefaultValidator()
	}
	return &FeedbackLoop{
		goldenDir: goldenDir,
		fs:        fs,
		prCreator: prCreator,
		validator: validator,
	}
}

// UpdateGoldenDataset adds production examples to the golden dataset.
// It validates examples before adding and creates a pull request with the changes.
func (fl *FeedbackLoop) UpdateGoldenDataset(ctx context.Context, prodExamples []Example) (int, error) {
	if len(prodExamples) == 0 {
		return 0, nil
	}

	// Ensure golden directory exists
	if err := fl.fs.MkdirAll(fl.goldenDir, 0755); err != nil {
		return 0, fmt.Errorf("failed to create golden directory: %w", err)
	}

	// Validate and filter examples
	validExamples := []Example{}
	invalidCount := 0
	for _, example := range prodExamples {
		if err := fl.validator.Validate(example); err != nil {
			invalidCount++
			continue
		}
		validExamples = append(validExamples, example)
	}

	if len(validExamples) == 0 {
		return 0, fmt.Errorf("no valid examples to add (%d invalid)", invalidCount)
	}

	// Write examples to files
	addedFiles := []string{}
	for _, example := range validExamples {
		filename := fmt.Sprintf("example_%s_%d.json",
			sanitizeFilename(example.ID),
			example.Timestamp.Unix())
		filepath := filepath.Join(fl.goldenDir, filename)

		data, err := json.MarshalIndent(example, "", "  ")
		if err != nil {
			return len(addedFiles), fmt.Errorf("failed to marshal example: %w", err)
		}

		if err := fl.fs.WriteFile(filepath, data, 0644); err != nil {
			return len(addedFiles), fmt.Errorf("failed to write example file: %w", err)
		}

		addedFiles = append(addedFiles, filepath)
	}

	// Create pull request
	if fl.prCreator != nil {
		title := fmt.Sprintf("Add %d production examples to golden dataset", len(validExamples))
		body := fmt.Sprintf(
			"This PR adds %d validated production examples to the golden dataset.\n\n"+
				"- Valid examples: %d\n"+
				"- Invalid examples (skipped): %d\n"+
				"- Source: production feedback loop\n"+
				"- Generated at: %s\n",
			len(validExamples), len(validExamples), invalidCount, time.Now().Format(time.RFC3339))

		if _, err := fl.prCreator.CreatePR(ctx, title, body, addedFiles); err != nil {
			return len(addedFiles), fmt.Errorf("failed to create PR: %w", err)
		}
	}

	return len(validExamples), nil
}

// GetExampleCount returns the number of examples in the golden dataset.
func (fl *FeedbackLoop) GetExampleCount() (int, error) {
	entries, err := fl.fs.ReadDir(fl.goldenDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to read golden directory: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			count++
		}
	}

	return count, nil
}

// sanitizeFilename removes characters that are invalid in filenames.
func sanitizeFilename(s string) string {
	// Replace invalid characters with underscores
	re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	sanitized := re.ReplaceAllString(s, "_")

	// Limit length
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}

	return sanitized
}

// MockPRCreator implements PRCreator for testing.
type MockPRCreator struct {
	PRs []PRRecord
}

// PRRecord stores information about a created PR.
type PRRecord struct {
	Title string
	Body  string
	Files []string
	URL   string
}

// CreatePR creates a mock pull request.
func (mpc *MockPRCreator) CreatePR(ctx context.Context, title, body string, files []string) (string, error) {
	url := fmt.Sprintf("https://github.com/example/repo/pull/%d", len(mpc.PRs)+1)
	mpc.PRs = append(mpc.PRs, PRRecord{
		Title: title,
		Body:  body,
		Files: files,
		URL:   url,
	})
	return url, nil
}
