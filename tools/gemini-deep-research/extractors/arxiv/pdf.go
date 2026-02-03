package arxiv

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/ledongthuc/pdf"
)

// PDFParser handles PDF text extraction with fallback
type PDFParser struct {
	apiClient *APIClient
}

// NewPDFParser creates a new PDF parser
func NewPDFParser() *PDFParser {
	return &PDFParser{
		apiClient: NewAPIClient(),
	}
}

// ExtractFromArxiv downloads and extracts text from an arxiv PDF
func (p *PDFParser) ExtractFromArxiv(ctx context.Context, paperID string) (string, error) {
	// Download PDF
	pdfData, err := p.apiClient.DownloadPDF(ctx, paperID)
	if err != nil {
		return "", fmt.Errorf("download PDF: %w", err)
	}

	// Create temporary file for PDF
	tmpFile, err := os.CreateTemp("", "arxiv-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write PDF data to temp file
	if _, err := tmpFile.Write(pdfData); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("write PDF to temp file: %w", err)
	}
	tmpFile.Close()

	// Try pdftotext first (best quality)
	text, err := p.extractWithPdftotext(tmpFile.Name())
	if err == nil {
		return text, nil
	}

	// Fallback to ledongthuc/pdf library
	text, err = p.extractWithLedongthuc(tmpFile.Name())
	if err == nil {
		return text, nil
	}

	// Both methods failed
	return "", fmt.Errorf("all PDF extraction methods failed: pdftotext and ledongthuc/pdf")
}

// extractWithPdftotext extracts text using pdftotext command
func (p *PDFParser) extractWithPdftotext(pdfPath string) (string, error) {
	// Check if pdftotext is available
	if _, err := exec.LookPath("pdftotext"); err != nil {
		return "", ErrPdftotextNotFound
	}

	// Run pdftotext with layout preservation
	// Output to stdout using "-" as output file
	cmd := exec.Command("pdftotext", "-layout", pdfPath, "-")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("pdftotext execution failed: %w", err)
	}

	text := string(output)

	// Validate extracted text
	if len(strings.TrimSpace(text)) < 100 {
		return "", ErrPDFExtractionFailed
	}

	return text, nil
}

// extractWithLedongthuc extracts text using ledongthuc/pdf library
func (p *PDFParser) extractWithLedongthuc(pdfPath string) (string, error) {
	// Open PDF file
	f, r, err := pdf.Open(pdfPath)
	if err != nil {
		return "", fmt.Errorf("open PDF: %w", err)
	}
	defer f.Close()

	var text strings.Builder
	totalPages := r.NumPage()

	// Extract text from each page
	for pageNum := 1; pageNum <= totalPages; pageNum++ {
		page := r.Page(pageNum)
		if page.V.IsNull() {
			continue
		}

		// Get plain text from page
		pageText, err := page.GetPlainText(nil)
		if err != nil {
			// Log warning but continue with other pages
			continue
		}

		text.WriteString(pageText)
		text.WriteString("\n")
	}

	result := text.String()

	// Validate extracted text
	if len(strings.TrimSpace(result)) < 100 {
		return "", ErrPDFExtractionFailed
	}

	return result, nil
}

// ExtractFromFile extracts text from a local PDF file
// Used for testing and direct file access
func (p *PDFParser) ExtractFromFile(pdfPath string) (string, error) {
	// Try pdftotext first
	text, err := p.extractWithPdftotext(pdfPath)
	if err == nil {
		return text, nil
	}

	// Fallback to ledongthuc/pdf
	text, err = p.extractWithLedongthuc(pdfPath)
	if err == nil {
		return text, nil
	}

	return "", fmt.Errorf("all PDF extraction methods failed")
}

// ExtractFromReader extracts text from a PDF reader
// Used for in-memory PDF processing
func (p *PDFParser) ExtractFromReader(r io.ReaderAt, size int64) (string, error) {
	// ledongthuc/pdf requires a file, so create temp file
	tmpFile, err := os.CreateTemp("", "arxiv-reader-*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	// Copy reader to temp file
	if _, err := io.Copy(tmpFile, io.NewSectionReader(r, 0, size)); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("copy to temp file: %w", err)
	}
	tmpFile.Close()

	return p.ExtractFromFile(tmpFile.Name())
}
