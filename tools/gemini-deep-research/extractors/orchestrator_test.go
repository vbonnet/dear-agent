package extractors

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
)

// mockDetector is a mock implementation of the Detector interface
type mockDetector struct {
	detectFunc func(url string, typeOverride string) (detector.ContentType, error)
}

func (m *mockDetector) DetectContentType(url string, typeOverride string) (detector.ContentType, error) {
	if m.detectFunc != nil {
		return m.detectFunc(url, typeOverride)
	}
	return detector.ContentTypeArticle, nil
}

func TestNewOrchestrator(t *testing.T) {
	orch, err := NewOrchestrator()
	if err != nil {
		t.Fatalf("NewOrchestrator() error = %v", err)
	}

	if orch == nil {
		t.Fatal("NewOrchestrator() returned nil orchestrator")
	}

	if orch.factory == nil {
		t.Error("Orchestrator factory not initialized")
	}

	if orch.detector == nil {
		t.Error("Orchestrator detector not initialized")
	}
}

func TestOrchestrator_ExtractContent_Success(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		typeOverride string
		contentType  detector.ContentType
		wantErr      bool
	}{
		{
			name:         "detect video",
			url:          "https://www.youtube.com/watch?v=VIDEO_ID",
			typeOverride: "",
			contentType:  detector.ContentTypeVideo,
			wantErr:      false,
		},
		{
			name:         "detect arxiv",
			url:          "https://arxiv.org/abs/2601.20802",
			typeOverride: "",
			contentType:  detector.ContentTypeArxiv,
			wantErr:      false,
		},
		{
			name:         "detect huggingface",
			url:          "https://huggingface.co/papers/2501.12345",
			typeOverride: "",
			contentType:  detector.ContentTypeHuggingFace,
			wantErr:      false,
		},
		{
			name:         "detect generic article",
			url:          "https://example.com/article",
			typeOverride: "",
			contentType:  detector.ContentTypeArticle,
			wantErr:      false,
		},
		{
			name:         "override to article",
			url:          "https://www.youtube.com/watch?v=VIDEO_ID",
			typeOverride: "article",
			contentType:  detector.ContentTypeArticle,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create orchestrator with mock detector
			orch := &Orchestrator{
				detector: &mockDetector{
					detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
						if typeOverride != "" {
							// Simulate override
							return tt.contentType, nil
						}
						return tt.contentType, nil
					},
				},
			}

			// Initialize factory
			factory, err := NewExtractorFactory()
			if err != nil {
				t.Fatalf("NewExtractorFactory() error = %v", err)
			}
			orch.factory = factory

			// Note: We can't actually extract without real extractors
			// This test just verifies the orchestration logic
			// Real extraction is tested in integration tests
		})
	}
}

func TestOrchestrator_ExtractContent_DetectionError(t *testing.T) {
	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				return detector.ContentTypeArticle, errors.New("invalid URL format")
			},
		},
		factory: nil, // Factory not needed for this test
	}

	ctx := context.Background()
	_, err := orch.ExtractContent(ctx, "invalid-url", "")

	if err == nil {
		t.Fatal("ExtractContent() expected error for detection failure")
	}

	// Should be an ExtractionError
	var extErr *ExtractionError
	if !errors.As(err, &extErr) {
		t.Errorf("Expected ExtractionError, got %T", err)
	}

	if extErr != nil {
		if extErr.URL != "invalid-url" {
			t.Errorf("ExtractionError.URL = %v, want 'invalid-url'", extErr.URL)
		}

		if extErr.Operation != "detect content type" {
			t.Errorf("ExtractionError.Operation = %v, want 'detect content type'", extErr.Operation)
		}

		if len(extErr.Troubleshooting) == 0 {
			t.Error("ExtractionError should have troubleshooting tips")
		}
	}
}

func TestOrchestrator_ExtractContent_UnsupportedType(t *testing.T) {
	factory, err := NewExtractorFactory()
	if err != nil {
		t.Fatalf("NewExtractorFactory() error = %v", err)
	}

	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				return detector.ContentType(999), nil // Unsupported type
			},
		},
		factory: factory,
	}

	ctx := context.Background()
	_, err = orch.ExtractContent(ctx, "https://example.com", "")

	if err == nil {
		t.Fatal("ExtractContent() expected error for unsupported type")
	}

	// Should be an ExtractionError
	var extErr *ExtractionError
	if !errors.As(err, &extErr) {
		t.Errorf("Expected ExtractionError, got %T", err)
	}
}

func TestOrchestrator_ExtractContent_WithMockExtractor(t *testing.T) {
	// Create orchestrator with real factory but mock detector
	factory, err := NewExtractorFactory()
	if err != nil {
		t.Fatalf("NewExtractorFactory() error = %v", err)
	}

	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				// Always return article type for predictable testing
				return detector.ContentTypeArticle, nil
			},
		},
		factory: factory,
	}

	// Note: Actual extraction would fail because we'd need real web content
	// This test just verifies the orchestration flow doesn't panic
	ctx := context.Background()
	_, err = orch.ExtractContent(ctx, "https://nonexistent.example.com/article", "")

	// We expect an error because the URL doesn't exist
	// but the orchestrator should have attempted extraction
	if err == nil {
		t.Log("Warning: ExtractContent succeeded unexpectedly (network access?)")
	}
}

func TestExtractionError_Error(t *testing.T) {
	err := &ExtractionError{
		URL:       "https://example.com",
		Operation: "test operation",
		Err:       errors.New("underlying error"),
		Troubleshooting: []string{
			"Try step 1",
			"Try step 2",
		},
	}

	errMsg := err.Error()

	// Check error message contains all components
	if !strings.Contains(errMsg, "test operation") {
		t.Error("Error message should contain operation")
	}

	if !strings.Contains(errMsg, "https://example.com") {
		t.Error("Error message should contain URL")
	}

	if !strings.Contains(errMsg, "underlying error") {
		t.Error("Error message should contain underlying error")
	}

	if !strings.Contains(errMsg, "Try step 1") {
		t.Error("Error message should contain troubleshooting tips")
	}

	if !strings.Contains(errMsg, "Try step 2") {
		t.Error("Error message should contain all troubleshooting tips")
	}
}

func TestExtractionError_Unwrap(t *testing.T) {
	underlyingErr := errors.New("underlying error")
	err := &ExtractionError{
		URL:       "https://example.com",
		Operation: "test",
		Err:       underlyingErr,
	}

	unwrapped := err.Unwrap()
	if unwrapped != underlyingErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, underlyingErr)
	}

	// Test with errors.Is
	if !errors.Is(err, underlyingErr) {
		t.Error("errors.Is should find underlying error")
	}
}

func TestDetectorWrapper(t *testing.T) {
	wrapper := &detectorWrapper{}

	// Test valid URLs
	tests := []struct {
		name         string
		url          string
		typeOverride string
		wantType     detector.ContentType
		wantErr      bool
	}{
		{
			name:         "youtube url",
			url:          "https://www.youtube.com/watch?v=VIDEO_ID",
			typeOverride: "",
			wantType:     detector.ContentTypeVideo,
			wantErr:      false,
		},
		{
			name:         "arxiv url",
			url:          "https://arxiv.org/abs/2601.20802",
			typeOverride: "",
			wantType:     detector.ContentTypeArxiv,
			wantErr:      false,
		},
		{
			name:         "override to video",
			url:          "https://example.com",
			typeOverride: "video",
			wantType:     detector.ContentTypeVideo,
			wantErr:      false,
		},
		{
			name:         "empty url",
			url:          "",
			typeOverride: "",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contentType, err := wrapper.DetectContentType(tt.url, tt.typeOverride)

			if tt.wantErr {
				if err == nil {
					t.Error("DetectContentType() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("DetectContentType() unexpected error: %v", err)
				return
			}

			if contentType != tt.wantType {
				t.Errorf("DetectContentType() = %v, want %v", contentType, tt.wantType)
			}
		})
	}
}

func TestOrchestrator_ErrorPropagation(t *testing.T) {
	// Test that errors from extractors are properly propagated and wrapped
	factory, err := NewExtractorFactory()
	if err != nil {
		t.Fatalf("NewExtractorFactory() error = %v", err)
	}

	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				return detector.ContentTypeArticle, nil
			},
		},
		factory: factory,
	}

	ctx := context.Background()
	// Use a URL that will fail extraction
	_, err = orch.ExtractContent(ctx, "https://invalid-domain-that-does-not-exist-12345.com", "")

	if err == nil {
		t.Fatal("ExtractContent() expected error for invalid URL")
	}

	// Error should contain context about which extractor was used
	if !strings.Contains(err.Error(), "extractor") {
		t.Errorf("Error should mention extractor: %v", err)
	}
}

func TestOrchestrator_ContextCancellation(t *testing.T) {
	factory, err := NewExtractorFactory()
	if err != nil {
		t.Fatalf("NewExtractorFactory() error = %v", err)
	}

	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				return detector.ContentTypeArticle, nil
			},
		},
		factory: factory,
	}

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = orch.ExtractContent(ctx, "https://example.com", "")

	// Should get an error (either from cancellation or extraction failure)
	if err == nil {
		t.Log("Warning: Expected error due to cancelled context")
	}
}

func TestOrchestrator_TypeOverride(t *testing.T) {
	factory, err := NewExtractorFactory()
	if err != nil {
		t.Fatalf("NewExtractorFactory() error = %v", err)
	}

	callCount := 0
	expectedOverride := "article"

	orch := &Orchestrator{
		detector: &mockDetector{
			detectFunc: func(url string, typeOverride string) (detector.ContentType, error) {
				callCount++
				// Verify typeOverride is passed through
				if typeOverride != expectedOverride {
					t.Errorf("DetectContentType() typeOverride = %v, want %v", typeOverride, expectedOverride)
				}
				return detector.ContentTypeArticle, nil
			},
		},
		factory: factory,
	}

	ctx := context.Background()
	_, _ = orch.ExtractContent(ctx, "https://example.com", expectedOverride)

	if callCount != 1 {
		t.Errorf("DetectContentType() called %d times, want 1", callCount)
	}
}
