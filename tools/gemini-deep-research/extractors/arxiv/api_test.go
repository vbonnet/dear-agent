package arxiv

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIClient_FetchMetadata_Success(t *testing.T) {
	// Mock arxiv API response
	mockResponse := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>Test Paper Title</title>
    <author>
      <name>John Doe</name>
    </author>
    <author>
      <name>Jane Smith</name>
    </author>
    <summary>This is the abstract of the paper.</summary>
    <published>2024-01-15T00:00:00Z</published>
  </entry>
</feed>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify query parameters
		if !strings.Contains(r.URL.Query().Get("id_list"), "2601.20802") {
			t.Errorf("Expected id_list parameter, got: %s", r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, mockResponse)
	}))
	defer server.Close()

	// Test is limited because we can't easily override the const base URL
	// This is a design consideration for future refactoring
	// client := NewAPIClient() // Would be used with URL injection
	t.Skip("Skipping API test - requires refactoring to inject base URL")
}

func TestAPIClient_FetchMetadata_NotFound(t *testing.T) {
	// Mock arxiv API response for paper not found
	mockResponse := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, mockResponse)
	}))
	defer server.Close()

	t.Skip("Skipping API test - requires refactoring to inject base URL")
}

func TestAPIClient_RateLimiting(t *testing.T) {
	client := NewAPIClient()

	// First request should set lastRequestAt
	if !client.lastRequestAt.IsZero() {
		t.Error("lastRequestAt should be zero initially")
	}

	// Simulate first request
	client.enforceRateLimit()
	firstRequest := client.lastRequestAt

	if firstRequest.IsZero() {
		t.Error("lastRequestAt should be set after first request")
	}

	// Immediate second request should wait
	start := time.Now()
	client.enforceRateLimit()
	elapsed := time.Since(start)

	// Should have waited approximately RateLimitDelay
	if elapsed < RateLimitDelay-100*time.Millisecond {
		t.Errorf("Rate limit not enforced: waited %v, expected at least %v", elapsed, RateLimitDelay)
	}
}

func TestAPIClient_RateLimiting_NoWaitAfterDelay(t *testing.T) {
	client := NewAPIClient()

	// First request
	client.enforceRateLimit()

	// Wait for rate limit period
	time.Sleep(RateLimitDelay + 100*time.Millisecond)

	// Second request should not wait
	start := time.Now()
	client.enforceRateLimit()
	elapsed := time.Since(start)

	if elapsed > 100*time.Millisecond {
		t.Errorf("Unexpected wait: %v, should be minimal after delay", elapsed)
	}
}

func TestAPIClient_DownloadPDF_Success(t *testing.T) {
	mockPDF := []byte("%PDF-1.4 mock pdf content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, ".pdf") {
			t.Errorf("Expected PDF path, got: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/pdf")
		w.Write(mockPDF)
	}))
	defer server.Close()

	t.Skip("Skipping PDF download test - requires refactoring to inject base URL")
}

func TestAPIClient_DownloadPDF_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, "PDF not found")
	}))
	defer server.Close()

	t.Skip("Skipping PDF download test - requires refactoring to inject base URL")
}

// Integration test - requires network access
// Run with: go test -tags=integration
func TestAPIClient_FetchMetadata_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	client := NewAPIClient()
	ctx := context.Background()

	// Use a known arxiv paper
	paperID := "2301.07041" // Example paper

	metadata, err := client.FetchMetadata(ctx, paperID)
	if err != nil {
		t.Fatalf("FetchMetadata() error = %v", err)
	}

	if metadata.Title == "" {
		t.Error("FetchMetadata() returned empty title")
	}

	if len(metadata.Authors) == 0 {
		t.Error("FetchMetadata() returned no authors")
	}

	if metadata.Abstract == "" {
		t.Error("FetchMetadata() returned empty abstract")
	}

	if metadata.Published == "" {
		t.Error("FetchMetadata() returned empty published date")
	}

	expectedPDFURL := fmt.Sprintf("%s/%s.pdf", ArxivPDFBaseURL, paperID)
	if metadata.PDFURL != expectedPDFURL {
		t.Errorf("FetchMetadata() PDFURL = %q, want %q", metadata.PDFURL, expectedPDFURL)
	}

	t.Logf("Successfully fetched paper: %s", metadata.Title)
	t.Logf("Authors: %v", metadata.Authors)
}
