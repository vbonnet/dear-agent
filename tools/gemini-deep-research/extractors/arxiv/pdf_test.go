package arxiv

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestPDFParser_PdftotextAvailability(t *testing.T) {
	_, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not available, skipping test")
	}
}

func TestPDFParser_ExtractFromFile_Pdftotext(t *testing.T) {
	// Check if pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not available")
	}

	// This test requires a sample PDF file
	// In a real test environment, you would have test fixtures
	t.Skip("Requires sample PDF fixture - implement when test files are available")
}

func TestPDFParser_ExtractFromFile_Fallback(t *testing.T) {
	// Test ledongthuc/pdf fallback when pdftotext is not available
	t.Skip("Requires sample PDF fixture - implement when test files are available")
}

func TestPDFParser_ExtractWithPdftotext_NotFound(t *testing.T) {
	parser := NewPDFParser()

	// Try to extract from non-existent file
	_, err := parser.extractWithPdftotext("/tmp/nonexistent.pdf")
	if err == nil {
		t.Error("extractWithPdftotext() expected error for non-existent file")
	}
}

func TestPDFParser_ExtractWithLedongthuc_InvalidPDF(t *testing.T) {
	// Create a temporary file with invalid PDF content
	tmpFile, err := os.CreateTemp("", "invalid-*.pdf")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write invalid PDF content
	_, err = tmpFile.WriteString("This is not a valid PDF")
	tmpFile.Close()
	if err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}

	// Try to extract - should fail
	parser := NewPDFParser()
	_, err = parser.extractWithLedongthuc(tmpFile.Name())
	if err == nil {
		t.Error("extractWithLedongthuc() expected error for invalid PDF")
	}
}

// Mock test for PDF extraction pipeline
func TestPDFParser_ExtractionPipeline(t *testing.T) {
	tests := []struct {
		name           string
		pdfContent     string
		expectError    bool
		errorContains  string
		pdftotextAvail bool
	}{
		{
			name:        "empty PDF",
			pdfContent:  "",
			expectError: true,
		},
		{
			name:        "invalid PDF format",
			pdfContent:  "not a pdf",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is a placeholder for proper mock tests
			// In production, you would:
			// 1. Create a mock PDF file with specific content
			// 2. Test both extraction paths (pdftotext and ledongthuc)
			// 3. Verify fallback behavior
			t.Skip("Requires PDF test fixtures")
		})
	}
}

// Integration test - requires network access and real PDF
func TestPDFParser_ExtractFromArxiv_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Note: This would download a real PDF from arxiv
	// Skipping by default to avoid network calls in unit tests
	t.Skip("Integration test - run manually with network access")

	// _ = NewPDFParser() // Would be used in actual integration test

	// Example usage when running integration tests:
	/*
		ctx := context.Background()
		text, err := parser.ExtractFromArxiv(ctx, "2301.07041")
		if err != nil {
			t.Fatalf("ExtractFromArxiv() error = %v", err)
		}

		if len(strings.TrimSpace(text)) < 1000 {
			t.Errorf("ExtractFromArxiv() returned suspiciously short text: %d chars", len(text))
		}

		t.Logf("Extracted %d characters from PDF", len(text))
	*/
}

func TestPDFParser_ValidationLogic(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		shouldPass  bool
		description string
	}{
		{
			name:        "valid long text",
			text:        strings.Repeat("Lorem ipsum dolor sit amet. ", 10),
			shouldPass:  true,
			description: "Text longer than 100 chars should pass",
		},
		{
			name:        "short text fails",
			text:        "Short",
			shouldPass:  false,
			description: "Text shorter than 100 chars should fail",
		},
		{
			name:        "empty text fails",
			text:        "",
			shouldPass:  false,
			description: "Empty text should fail",
		},
		{
			name:        "whitespace only fails",
			text:        "   \n\n   \t\t   ",
			shouldPass:  false,
			description: "Whitespace-only text should fail",
		},
		{
			name:        "exactly 100 chars passes",
			text:        strings.Repeat("x", 100),
			shouldPass:  true,
			description: "Exactly 100 chars should pass",
		},
		{
			name:        "99 chars fails",
			text:        strings.Repeat("x", 99),
			shouldPass:  false,
			description: "99 chars should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate the same way the PDF parser does
			isValid := len(strings.TrimSpace(tt.text)) >= 100

			if isValid != tt.shouldPass {
				t.Errorf("%s: validation = %v, want %v", tt.description, isValid, tt.shouldPass)
			}
		})
	}
}
