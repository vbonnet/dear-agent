package evaluation

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// MockFileSystem implements FileSystem for testing.
type MockFileSystem struct {
	files map[string][]byte
	dirs  map[string]bool
}

func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string][]byte),
		dirs:  make(map[string]bool),
	}
}

func (mfs *MockFileSystem) WriteFile(path string, data []byte, perm os.FileMode) error {
	mfs.files[path] = data
	return nil
}

func (mfs *MockFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	if !mfs.dirs[path] {
		return nil, os.ErrNotExist
	}

	entries := []os.DirEntry{}
	prefix := path + "/"
	for filename := range mfs.files {
		if strings.HasPrefix(filename, prefix) {
			relPath := strings.TrimPrefix(filename, prefix)
			if !strings.Contains(relPath, "/") { // Only direct children
				entries = append(entries, &mockDirEntry{name: filepath.Base(filename)})
			}
		}
	}
	return entries, nil
}

func (mfs *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	mfs.dirs[path] = true
	return nil
}

func (mfs *MockFileSystem) FileExists(path string) bool {
	_, ok := mfs.files[path]
	return ok
}

func (mfs *MockFileSystem) GetFileContent(path string) ([]byte, bool) {
	data, ok := mfs.files[path]
	return data, ok
}

type mockDirEntry struct {
	name string
}

func (m *mockDirEntry) Name() string               { return m.name }
func (m *mockDirEntry) IsDir() bool                { return false }
func (m *mockDirEntry) Type() fs.FileMode          { return 0 }
func (m *mockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestDefaultValidator_ValidExample(t *testing.T) {
	validator := NewDefaultValidator()
	example := Example{
		ID:        "test-1",
		Prompt:    "What is the capital of France?",
		Response:  "The capital of France is Paris.",
		Score:     0.95,
		Timestamp: time.Now(),
		Source:    "production",
		Validated: true,
	}

	err := validator.Validate(example)
	if err != nil {
		t.Errorf("Expected valid example, got error: %v", err)
	}
}

func TestDefaultValidator_EmptyPrompt(t *testing.T) {
	validator := NewDefaultValidator()
	example := Example{
		ID:        "test-1",
		Prompt:    "",
		Response:  "Some response",
		Validated: true,
	}

	err := validator.Validate(example)
	if err == nil {
		t.Error("Expected error for empty prompt")
	}
}

func TestDefaultValidator_EmptyResponse(t *testing.T) {
	validator := NewDefaultValidator()
	example := Example{
		ID:        "test-1",
		Prompt:    "Some prompt",
		Response:  "",
		Validated: true,
	}

	err := validator.Validate(example)
	if err == nil {
		t.Error("Expected error for empty response")
	}
}

func TestDefaultValidator_NotValidated(t *testing.T) {
	validator := NewDefaultValidator()
	example := Example{
		ID:        "test-1",
		Prompt:    "Some prompt",
		Response:  "Some response",
		Validated: false,
	}

	err := validator.Validate(example)
	if err == nil {
		t.Error("Expected error for non-validated example")
	}
}

func TestDefaultValidator_PIIDetection(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		response string
		hasPII   bool
	}{
		{
			name:     "email in prompt",
			prompt:   "Contact me at john.doe@example.com",
			response: "OK",
			hasPII:   true,
		},
		{
			name:     "email in response",
			prompt:   "What is my email?",
			response: "Your email is jane@company.org",
			hasPII:   true,
		},
		{
			name:     "credit card number",
			prompt:   "My card is 1234-5678-9012-3456",
			response: "OK",
			hasPII:   true,
		},
		{
			name:     "SSN",
			prompt:   "My SSN is 123-45-6789",
			response: "OK",
			hasPII:   true,
		},
		{
			name:     "phone number",
			prompt:   "Call me at 555-123-4567",
			response: "OK",
			hasPII:   true,
		},
		{
			name:     "API key",
			prompt:   "Use api_key=sk_test_1234567890abcdef",
			response: "OK",
			hasPII:   true,
		},
		{
			name:     "clean content",
			prompt:   "What is the weather today?",
			response: "It is sunny with a high of 75 degrees.",
			hasPII:   false,
		},
	}

	validator := NewDefaultValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			example := Example{
				ID:        "test-1",
				Prompt:    tt.prompt,
				Response:  tt.response,
				Validated: true,
			}

			err := validator.Validate(example)
			if tt.hasPII && err == nil {
				t.Error("Expected error for PII content")
			}
			if !tt.hasPII && err != nil {
				t.Errorf("Expected no error for clean content, got: %v", err)
			}
		})
	}
}

func TestDefaultValidator_ErrorIndicators(t *testing.T) {
	tests := []struct {
		name     string
		response string
		hasError bool
	}{
		{
			name:     "error prefix",
			response: "Error: failed to process request",
			hasError: true,
		},
		{
			name:     "exception",
			response: "Exception: null pointer reference",
			hasError: true,
		},
		{
			name:     "failed to",
			response: "Failed to connect to database",
			hasError: true,
		},
		{
			name:     "panic",
			response: "Panic: runtime error",
			hasError: true,
		},
		{
			name:     "stack trace",
			response: "Stack trace: line 1, line 2",
			hasError: true,
		},
		{
			name:     "clean response",
			response: "The operation completed successfully.",
			hasError: false,
		},
	}

	validator := NewDefaultValidator()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			example := Example{
				ID:        "test-1",
				Prompt:    "Test prompt",
				Response:  tt.response,
				Validated: true,
			}

			err := validator.Validate(example)
			if tt.hasError && err == nil {
				t.Error("Expected error for response with error indicator")
			}
			if !tt.hasError && err != nil {
				t.Errorf("Expected no error for clean response, got: %v", err)
			}
		})
	}
}

func TestFeedbackLoop_UpdateGoldenDataset_NoExamples(t *testing.T) {
	fs := NewMockFileSystem()
	prCreator := &MockPRCreator{}
	fl := NewFeedbackLoop("test/golden", fs, prCreator, nil)

	count, err := fl.UpdateGoldenDataset(context.Background(), []Example{})
	if err != nil {
		t.Errorf("Expected no error for empty examples, got: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 added examples, got %d", count)
	}
}

func TestFeedbackLoop_UpdateGoldenDataset_ValidExamples(t *testing.T) {
	fs := NewMockFileSystem()
	prCreator := &MockPRCreator{}
	fl := NewFeedbackLoop("test/golden", fs, prCreator, nil)

	examples := []Example{
		{
			ID:        "ex1",
			Prompt:    "What is Go?",
			Response:  "Go is a programming language.",
			Score:     0.95,
			Timestamp: time.Now(),
			Source:    "production",
			Validated: true,
		},
		{
			ID:        "ex2",
			Prompt:    "Explain testing",
			Response:  "Testing ensures code quality.",
			Score:     0.90,
			Timestamp: time.Now(),
			Source:    "production",
			Validated: true,
		},
	}

	count, err := fl.UpdateGoldenDataset(context.Background(), examples)
	if err != nil {
		t.Fatalf("UpdateGoldenDataset failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 added examples, got %d", count)
	}

	// Verify files were created
	if len(fs.files) != 2 {
		t.Errorf("Expected 2 files created, got %d", len(fs.files))
	}

	// Verify PR was created
	if len(prCreator.PRs) != 1 {
		t.Errorf("Expected 1 PR created, got %d", len(prCreator.PRs))
	}

	pr := prCreator.PRs[0]
	if !strings.Contains(pr.Title, "2 production examples") {
		t.Errorf("PR title incorrect: %s", pr.Title)
	}
}

func TestFeedbackLoop_UpdateGoldenDataset_InvalidExamples(t *testing.T) {
	fs := NewMockFileSystem()
	prCreator := &MockPRCreator{}
	fl := NewFeedbackLoop("test/golden", fs, prCreator, nil)

	examples := []Example{
		{
			ID:        "ex1",
			Prompt:    "", // Invalid: empty prompt
			Response:  "Some response",
			Validated: true,
		},
		{
			ID:        "ex2",
			Prompt:    "Contact me at test@example.com", // Invalid: PII
			Response:  "OK",
			Validated: true,
		},
		{
			ID:        "ex3",
			Prompt:    "Test",
			Response:  "Error: something went wrong", // Invalid: error indicator
			Validated: true,
		},
	}

	count, err := fl.UpdateGoldenDataset(context.Background(), examples)
	if err == nil {
		t.Error("Expected error for all invalid examples")
	}

	if count != 0 {
		t.Errorf("Expected 0 added examples, got %d", count)
	}

	// Verify no files were created
	if len(fs.files) > 0 {
		t.Errorf("Expected 0 files created, got %d", len(fs.files))
	}
}

func TestFeedbackLoop_UpdateGoldenDataset_MixedValidity(t *testing.T) {
	fs := NewMockFileSystem()
	prCreator := &MockPRCreator{}
	fl := NewFeedbackLoop("test/golden", fs, prCreator, nil)

	examples := []Example{
		{
			ID:        "ex1",
			Prompt:    "Valid prompt",
			Response:  "Valid response",
			Validated: true,
		},
		{
			ID:        "ex2",
			Prompt:    "", // Invalid
			Response:  "Response",
			Validated: true,
		},
		{
			ID:        "ex3",
			Prompt:    "Another valid prompt",
			Response:  "Another valid response",
			Validated: true,
		},
	}

	count, err := fl.UpdateGoldenDataset(context.Background(), examples)
	if err != nil {
		t.Fatalf("UpdateGoldenDataset failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 valid examples added, got %d", count)
	}

	// Verify PR body mentions invalid count
	pr := prCreator.PRs[0]
	if !strings.Contains(pr.Body, "Invalid examples (skipped): 1") {
		t.Errorf("PR body should mention invalid examples: %s", pr.Body)
	}
}

func TestFeedbackLoop_UpdateGoldenDataset_NoPRCreator(t *testing.T) {
	fs := NewMockFileSystem()
	fl := NewFeedbackLoop("test/golden", fs, nil, nil) // No PR creator

	examples := []Example{
		{
			ID:        "ex1",
			Prompt:    "Test prompt",
			Response:  "Test response",
			Validated: true,
		},
	}

	count, err := fl.UpdateGoldenDataset(context.Background(), examples)
	if err != nil {
		t.Fatalf("UpdateGoldenDataset should not fail without PR creator: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 added example, got %d", count)
	}
}

func TestFeedbackLoop_GetExampleCount(t *testing.T) {
	fs := NewMockFileSystem()
	fl := NewFeedbackLoop("test/golden", fs, nil, nil)

	// Initially, directory doesn't exist
	count, err := fl.GetExampleCount()
	if err != nil {
		t.Errorf("GetExampleCount should not error for non-existent directory: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 examples, got %d", count)
	}

	// Add some examples
	fs.MkdirAll("test/golden", 0755)
	fs.WriteFile("test/golden/example1.json", []byte("{}"), 0644)
	fs.WriteFile("test/golden/example2.json", []byte("{}"), 0644)
	fs.WriteFile("test/golden/readme.txt", []byte("readme"), 0644) // Non-JSON file

	count, err = fl.GetExampleCount()
	if err != nil {
		t.Fatalf("GetExampleCount failed: %v", err)
	}

	if count != 2 {
		t.Errorf("Expected 2 JSON examples, got %d", count)
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "simple-name",
			expected: "simple-name",
		},
		{
			input:    "name with spaces",
			expected: "name_with_spaces",
		},
		{
			input:    "name/with/slashes",
			expected: "name_with_slashes",
		},
		{
			input:    "name@with#special$chars",
			expected: "name_with_special_chars",
		},
		{
			input:    strings.Repeat("a", 100), // Too long
			expected: strings.Repeat("a", 50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestFeedbackLoop_FileContentValidation(t *testing.T) {
	fs := NewMockFileSystem()
	prCreator := &MockPRCreator{}
	fl := NewFeedbackLoop("test/golden", fs, prCreator, nil)

	example := Example{
		ID:        "test-ex",
		Prompt:    "What is testing?",
		Response:  "Testing is verification.",
		Score:     0.92,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Source:    "production",
		Validated: true,
	}

	_, err := fl.UpdateGoldenDataset(context.Background(), []Example{example})
	if err != nil {
		t.Fatalf("UpdateGoldenDataset failed: %v", err)
	}

	// Find the created file
	var fileContent []byte
	for _, content := range fs.files {
		fileContent = content
		break
	}

	// Verify it's valid JSON
	var decoded Example
	if err := json.Unmarshal(fileContent, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal saved example: %v", err)
	}

	// Verify content matches
	if decoded.ID != example.ID {
		t.Errorf("ID mismatch: expected %s, got %s", example.ID, decoded.ID)
	}
	if decoded.Prompt != example.Prompt {
		t.Errorf("Prompt mismatch: expected %s, got %s", example.Prompt, decoded.Prompt)
	}
	if decoded.Response != example.Response {
		t.Errorf("Response mismatch: expected %s, got %s", example.Response, decoded.Response)
	}
}

func TestMockPRCreator(t *testing.T) {
	prCreator := &MockPRCreator{}

	url, err := prCreator.CreatePR(context.Background(), "Test PR", "Test body", []string{"file1.json"})
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if !strings.Contains(url, "/pull/1") {
		t.Errorf("Expected PR URL to contain /pull/1, got: %s", url)
	}

	if len(prCreator.PRs) != 1 {
		t.Errorf("Expected 1 PR record, got %d", len(prCreator.PRs))
	}

	pr := prCreator.PRs[0]
	if pr.Title != "Test PR" {
		t.Errorf("PR title mismatch: %s", pr.Title)
	}
	if pr.Body != "Test body" {
		t.Errorf("PR body mismatch: %s", pr.Body)
	}
	if len(pr.Files) != 1 || pr.Files[0] != "file1.json" {
		t.Errorf("PR files mismatch: %v", pr.Files)
	}
}

func TestExample_CaptureTraceID(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0af7651916cd43dd8448eb211c80319c")
	spanID, _ := trace.SpanIDFromHex("00f067aa0ba902b7")
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), sc)

	example := Example{ID: "test-1", Prompt: "hello", Response: "world"}
	example.CaptureTraceID(ctx)

	if example.TraceID != "0af7651916cd43dd8448eb211c80319c" {
		t.Errorf("expected trace_id 0af7651916cd43dd8448eb211c80319c, got %s", example.TraceID)
	}
}

func TestExample_CaptureTraceID_NoSpan(t *testing.T) {
	example := Example{ID: "test-1", Prompt: "hello", Response: "world"}
	example.CaptureTraceID(context.Background())

	if example.TraceID != "" {
		t.Errorf("expected empty trace_id without span context, got %s", example.TraceID)
	}
}

func TestExample_TraceID_JSON_Omitempty(t *testing.T) {
	example := Example{
		ID:        "test-1",
		Prompt:    "hello",
		Response:  "world",
		Validated: true,
	}

	data, err := json.Marshal(example)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if strings.Contains(string(data), "trace_id") {
		t.Error("expected trace_id to be omitted when empty")
	}

	example.TraceID = "abc123"
	data, err = json.Marshal(example)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	if !strings.Contains(string(data), `"trace_id":"abc123"`) {
		t.Errorf("expected trace_id in JSON, got: %s", string(data))
	}
}
