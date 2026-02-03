package arxiv

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/research"
)

const (
	// ArxivAPIBaseURL is the base URL for the arxiv API
	ArxivAPIBaseURL = "http://export.arxiv.org/api/query"

	// ArxivPDFBaseURL is the base URL for arxiv PDF downloads
	ArxivPDFBaseURL = "https://arxiv.org/pdf"

	// RateLimitDelay is the delay between arxiv API requests (3 seconds per guidelines)
	RateLimitDelay = 3 * time.Second
)

// APIClient handles arxiv API requests
type APIClient struct {
	httpClient    *http.Client
	lastRequestAt time.Time
}

// NewAPIClient creates a new arxiv API client
func NewAPIClient() *APIClient {
	return &APIClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PaperMetadata represents arxiv paper metadata from API
type PaperMetadata struct {
	Title     string   `xml:"entry>title"`
	Authors   []string `xml:"entry>author>name"`
	Abstract  string   `xml:"entry>summary"`
	Published string   `xml:"entry>published"`
	PDFURL    string   // Constructed from paper ID
}

// arxivAPIResponse represents the XML response from arxiv API
type arxivAPIResponse struct {
	XMLName xml.Name `xml:"feed"`
	Entry   struct {
		Title   string `xml:"title"`
		Authors []struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Summary   string `xml:"summary"`
		Published string `xml:"published"`
	} `xml:"entry"`
}

// FetchMetadata fetches paper metadata from arxiv API
func (c *APIClient) FetchMetadata(ctx context.Context, paperID string) (*PaperMetadata, error) {
	// Rate limiting: ensure 3 seconds between requests
	c.enforceRateLimit()

	// Build API URL
	url := fmt.Sprintf("%s?id_list=%s", ArxivAPIBaseURL, paperID)

	// Fetch with retry logic
	var respBody []byte
	err := research.RetryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &research.HTTPError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("arxiv API returned status %d", resp.StatusCode),
			}
		}

		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read response: %w", err)
		}

		return nil
	}, research.DefaultRetryConfig())

	if err != nil {
		return nil, fmt.Errorf("fetch arxiv API: %w", err)
	}

	// Parse XML response
	var apiResp arxivAPIResponse
	if err := xml.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("parse arxiv XML response: %w", err)
	}

	// Check if entry exists (paper found)
	if apiResp.Entry.Title == "" {
		return nil, ErrPaperNotFound
	}

	// Extract author names
	authors := make([]string, 0, len(apiResp.Entry.Authors))
	for _, author := range apiResp.Entry.Authors {
		authors = append(authors, author.Name)
	}

	// Clean up fields (arxiv API includes extra whitespace)
	metadata := &PaperMetadata{
		Title:     strings.TrimSpace(apiResp.Entry.Title),
		Authors:   authors,
		Abstract:  strings.TrimSpace(apiResp.Entry.Summary),
		Published: strings.TrimSpace(apiResp.Entry.Published),
		PDFURL:    fmt.Sprintf("%s/%s.pdf", ArxivPDFBaseURL, paperID),
	}

	return metadata, nil
}

// enforceRateLimit ensures rate limiting per arxiv guidelines
func (c *APIClient) enforceRateLimit() {
	if c.lastRequestAt.IsZero() {
		c.lastRequestAt = time.Now()
		return
	}

	elapsed := time.Since(c.lastRequestAt)
	if elapsed < RateLimitDelay {
		time.Sleep(RateLimitDelay - elapsed)
	}

	c.lastRequestAt = time.Now()
}

// DownloadPDF downloads the PDF file for a paper
func (c *APIClient) DownloadPDF(ctx context.Context, paperID string) ([]byte, error) {
	url := fmt.Sprintf("%s/%s.pdf", ArxivPDFBaseURL, paperID)

	var pdfData []byte
	err := research.RetryWithBackoff(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return &research.HTTPError{
				StatusCode: resp.StatusCode,
				Message:    fmt.Sprintf("PDF download failed with status %d", resp.StatusCode),
			}
		}

		pdfData, err = io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read PDF data: %w", err)
		}

		return nil
	}, research.DefaultRetryConfig())

	if err != nil {
		return nil, fmt.Errorf("download PDF: %w", err)
	}

	return pdfData, nil
}
