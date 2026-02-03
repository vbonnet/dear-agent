package arxiv

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidArxivID indicates that the arxiv ID could not be extracted from URL
	ErrInvalidArxivID = errors.New("invalid arxiv ID")

	// ErrPaperNotFound indicates that the paper was not found on arxiv
	ErrPaperNotFound = errors.New("paper not found on arxiv")

	// ErrPDFExtractionFailed indicates that PDF text extraction failed
	ErrPDFExtractionFailed = errors.New("PDF text extraction failed")

	// ErrPdftotextNotFound indicates that pdftotext command is not available
	ErrPdftotextNotFound = errors.New("pdftotext not found")
)

// ExtractionError represents an error during arxiv paper extraction
type ExtractionError struct {
	URL             string
	Operation       string
	Err             error
	Troubleshooting []string
}

// Error returns the formatted error message
func (e *ExtractionError) Error() string {
	msg := fmt.Sprintf("Failed to %s\n", e.Operation)
	msg += fmt.Sprintf("  URL: %s\n", e.URL)
	msg += fmt.Sprintf("  Reason: %v\n", e.Err)

	if len(e.Troubleshooting) > 0 {
		msg += "  Troubleshooting:\n"
		for _, tip := range e.Troubleshooting {
			msg += fmt.Sprintf("    - %s\n", tip)
		}
	}

	return msg
}

// Unwrap returns the underlying error
func (e *ExtractionError) Unwrap() error {
	return e.Err
}

// NewInvalidArxivIDError creates an error for invalid arxiv ID
func NewInvalidArxivIDError(url string) *ExtractionError {
	return &ExtractionError{
		URL:       url,
		Operation: "extract arxiv paper ID from URL",
		Err:       ErrInvalidArxivID,
		Troubleshooting: []string{
			"Verify URL format (e.g., https://arxiv.org/abs/2601.20802)",
			"Check that the arxiv ID is in format YYMM.NNNNN",
			"Try accessing the URL in a web browser",
		},
	}
}

// NewPaperNotFoundError creates an error for paper not found
func NewPaperNotFoundError(paperID string) *ExtractionError {
	return &ExtractionError{
		URL:       fmt.Sprintf("https://arxiv.org/abs/%s", paperID),
		Operation: "fetch arxiv paper",
		Err:       ErrPaperNotFound,
		Troubleshooting: []string{
			"Verify the arxiv ID is correct: " + paperID,
			"Check if the paper exists on arxiv.org",
			"Try a different arxiv paper",
		},
	}
}

// NewPDFExtractionError creates an error for PDF extraction failure
func NewPDFExtractionError(paperID string, reason error) *ExtractionError {
	return &ExtractionError{
		URL:       fmt.Sprintf("https://arxiv.org/pdf/%s.pdf", paperID),
		Operation: "extract text from PDF",
		Err:       fmt.Errorf("%w: %v", ErrPDFExtractionFailed, reason),
		Troubleshooting: []string{
			"Install pdftotext for better extraction: sudo apt-get install poppler-utils",
			"Check that the PDF is not corrupted",
			"Tool will fall back to abstract-only mode",
		},
	}
}
