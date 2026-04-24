package papersearch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/papersearch"
)

const s2Fixture = `{
  "data": [
    {
      "paperId": "abc123",
      "title": "Deep Learning",
      "abstract": "A survey of deep learning methods.",
      "authors": [{"name": "LeCun"}, {"name": "Bengio"}],
      "year": 2015,
      "citationCount": 5000
    },
    {
      "paperId": "def456",
      "title": "No Abstract Paper",
      "abstract": null,
      "authors": [],
      "year": 2020,
      "citationCount": 1
    }
  ]
}`

const s2EmptyFixture = `{"data": []}`

func newS2Server(t *testing.T, body string, statusCode int, headerCheck func(*testing.T, *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if headerCheck != nil {
			headerCheck(t, r)
		}
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestS2Client_ParseFixture(t *testing.T) {
	srv := newS2Server(t, s2Fixture, http.StatusOK, nil)
	defer srv.Close()

	client := papersearch.NewS2Client(srv.Client(), 0)
	client.BaseURL = srv.URL

	papers, err := client.Search(context.Background(), "deep learning", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p0 := papers[0]
	if p0.PaperID != "abc123" {
		t.Errorf("unexpected PaperID: %q", p0.PaperID)
	}
	if p0.Abstract != "A survey of deep learning methods." {
		t.Errorf("unexpected abstract: %q", p0.Abstract)
	}
	if p0.Source != "semantic_scholar" {
		t.Errorf("unexpected source: %q", p0.Source)
	}

	// Paper with null abstract should fall back to title.
	p1 := papers[1]
	if p1.Abstract != p1.Title {
		t.Errorf("expected abstract to fall back to title, got %q", p1.Abstract)
	}
}

func TestS2Client_WithAPIKey(t *testing.T) {
	var gotKey string
	srv := newS2Server(t, s2EmptyFixture, http.StatusOK, func(t *testing.T, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
	})
	defer srv.Close()

	client := papersearch.NewS2Client(srv.Client(), 0)
	client.BaseURL = srv.URL
	client.APIKey = "test-key-123"

	_, err := client.Search(context.Background(), "transformers", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "test-key-123" {
		t.Errorf("expected x-api-key=test-key-123, got %q", gotKey)
	}
}

func TestS2Client_WithoutAPIKey(t *testing.T) {
	var gotKey string
	srv := newS2Server(t, s2EmptyFixture, http.StatusOK, func(t *testing.T, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
	})
	defer srv.Close()

	client := papersearch.NewS2Client(srv.Client(), 0)
	client.BaseURL = srv.URL
	client.APIKey = "" // explicitly empty

	_, err := client.Search(context.Background(), "transformers", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "" {
		t.Errorf("expected no x-api-key header, got %q", gotKey)
	}
}

func TestS2Client_HTTPError(t *testing.T) {
	srv := newS2Server(t, "bad gateway", http.StatusBadGateway, nil)
	defer srv.Close()

	client := papersearch.NewS2Client(srv.Client(), 0)
	client.BaseURL = srv.URL

	_, err := client.Search(context.Background(), "test", 5)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestS2Client_EmptyQuery(t *testing.T) {
	client := papersearch.NewS2Client(nil, 0)
	papers, err := client.Search(context.Background(), "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if papers != nil {
		t.Errorf("expected nil for empty query, got %v", papers)
	}
}

func TestS2Client_RateLimiting(t *testing.T) {
	// Verify that multiple calls do not panic and that the limiter is invoked.
	// We use a very high rate (1000 req/5min) so the test doesn't block.
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(s2EmptyFixture))
	}))
	defer srv.Close()

	client := papersearch.NewS2Client(srv.Client(), 1000)
	client.BaseURL = srv.URL

	for i := 0; i < 3; i++ {
		_, err := client.Search(context.Background(), "test", 5)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
	}
	if callCount != 3 {
		t.Errorf("expected 3 HTTP calls, got %d", callCount)
	}
}
