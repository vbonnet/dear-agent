package papersearch_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/papersearch"
)

// arXiv Atom XML fixture with two papers.
const arxivFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2301.01234v1</id>
    <title>Attention Is All You Need</title>
    <summary>  We propose a new simple network architecture, the Transformer.  </summary>
    <author><name>Vaswani et al.</name></author>
  </entry>
  <entry>
    <id>http://arxiv.org/abs/2302.05678v2</id>
    <title>BERT: Pre-training of Deep Bidirectional Transformers</title>
    <summary>We introduce BERT.</summary>
    <author><name>Devlin et al.</name></author>
  </entry>
</feed>`

const arxivEmptyFixture = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
</feed>`

func newArXivServer(t *testing.T, body string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

func TestArXivClient_ParseFixture(t *testing.T) {
	srv := newArXivServer(t, arxivFixture, http.StatusOK)
	defer srv.Close()

	client := papersearch.NewArXivClient(srv.Client(), 0)
	client.BaseURL = srv.URL

	papers, err := client.Search(context.Background(), []string{"transformer", "attention"}, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("expected 2 papers, got %d", len(papers))
	}

	p := papers[0]
	if p.Source != "arxiv" {
		t.Errorf("expected source=arxiv, got %q", p.Source)
	}
	if p.PaperID != "2301.01234v1" {
		t.Errorf("expected PaperID=2301.01234v1, got %q", p.PaperID)
	}
	if p.Title != "Attention Is All You Need" {
		t.Errorf("unexpected title: %q", p.Title)
	}
	if strings.HasPrefix(p.Abstract, " ") || strings.HasSuffix(p.Abstract, " ") {
		t.Errorf("abstract should be trimmed, got %q", p.Abstract)
	}
}

func TestArXivClient_EmptyResults(t *testing.T) {
	srv := newArXivServer(t, arxivEmptyFixture, http.StatusOK)
	defer srv.Close()

	client := papersearch.NewArXivClient(srv.Client(), 0)
	client.BaseURL = srv.URL

	papers, err := client.Search(context.Background(), []string{"noquery"}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(papers) != 0 {
		t.Errorf("expected 0 papers, got %d", len(papers))
	}
}

func TestArXivClient_HTTPError(t *testing.T) {
	srv := newArXivServer(t, "service unavailable", http.StatusServiceUnavailable)
	defer srv.Close()

	client := papersearch.NewArXivClient(srv.Client(), 0)
	client.BaseURL = srv.URL

	_, err := client.Search(context.Background(), []string{"test"}, 5)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestArXivClient_NoKeywords(t *testing.T) {
	client := papersearch.NewArXivClient(nil, 0)
	papers, err := client.Search(context.Background(), nil, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if papers != nil {
		t.Errorf("expected nil for empty keywords, got %v", papers)
	}
}
