package arxiv

import (
	"errors"
	"strings"
	"testing"
)

func TestExtractionError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ExtractionError
		contains []string
	}{
		{
			name: "basic error",
			err: &ExtractionError{
				URL:       "https://arxiv.org/abs/2601.20802",
				Operation: "fetch metadata",
				Err:       errors.New("connection failed"),
				Troubleshooting: []string{
					"Check internet connection",
					"Verify URL is accessible",
				},
			},
			contains: []string{
				"Failed to fetch metadata",
				"URL: https://arxiv.org/abs/2601.20802",
				"Reason: connection failed",
				"Troubleshooting:",
				"Check internet connection",
				"Verify URL is accessible",
			},
		},
		{
			name: "no troubleshooting",
			err: &ExtractionError{
				URL:             "https://arxiv.org/abs/invalid",
				Operation:       "parse URL",
				Err:             ErrInvalidArxivID,
				Troubleshooting: nil,
			},
			contains: []string{
				"Failed to parse URL",
				"URL: https://arxiv.org/abs/invalid",
				"Reason: invalid arxiv ID",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errMsg := tt.err.Error()
			for _, substr := range tt.contains {
				if !strings.Contains(errMsg, substr) {
					t.Errorf("Error() missing substring %q\nGot:\n%s", substr, errMsg)
				}
			}
		})
	}
}

func TestExtractionError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	err := &ExtractionError{
		URL:       "https://arxiv.org/abs/2601.20802",
		Operation: "test",
		Err:       innerErr,
	}

	if !errors.Is(err, innerErr) {
		t.Error("Unwrap() should expose inner error")
	}
}

func TestNewInvalidArxivIDError(t *testing.T) {
	url := "https://example.com/invalid"
	err := NewInvalidArxivIDError(url)

	if err.URL != url {
		t.Errorf("URL = %q, want %q", err.URL, url)
	}

	if !errors.Is(err, ErrInvalidArxivID) {
		t.Error("Should wrap ErrInvalidArxivID")
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Should include troubleshooting tips")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "extract arxiv paper ID") {
		t.Errorf("Error message should mention operation: %s", errMsg)
	}
}

func TestNewPaperNotFoundError(t *testing.T) {
	paperID := "9999.99999"
	err := NewPaperNotFoundError(paperID)

	if !strings.Contains(err.URL, paperID) {
		t.Errorf("URL should contain paper ID %q, got %q", paperID, err.URL)
	}

	if !errors.Is(err, ErrPaperNotFound) {
		t.Error("Should wrap ErrPaperNotFound")
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Should include troubleshooting tips")
	}
}

func TestNewPDFExtractionError(t *testing.T) {
	paperID := "2601.20802"
	reason := errors.New("pdftotext failed")
	err := NewPDFExtractionError(paperID, reason)

	if !strings.Contains(err.URL, paperID) {
		t.Errorf("URL should contain paper ID %q, got %q", paperID, err.URL)
	}

	if !strings.Contains(err.URL, ".pdf") {
		t.Errorf("URL should be PDF URL, got %q", err.URL)
	}

	if !errors.Is(err, ErrPDFExtractionFailed) {
		t.Error("Should wrap ErrPDFExtractionFailed")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "pdftotext failed") {
		t.Errorf("Error should include reason: %s", errMsg)
	}

	if len(err.Troubleshooting) == 0 {
		t.Error("Should include troubleshooting tips")
	}
}

func TestErrorConstants(t *testing.T) {
	// Verify error constants are defined and distinct
	errors := []error{
		ErrInvalidArxivID,
		ErrPaperNotFound,
		ErrPDFExtractionFailed,
		ErrPdftotextNotFound,
	}

	for i, err1 := range errors {
		if err1 == nil {
			t.Errorf("Error constant %d is nil", i)
		}
		for j, err2 := range errors {
			if i != j && err1 == err2 {
				t.Errorf("Error constants %d and %d are identical", i, j)
			}
		}
	}
}
