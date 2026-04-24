package papersearch_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/papersearch"
)

func TestMultiSearcher_FanOut(t *testing.T) {
	arxivSrv := newArXivServer(t, arxivFixture, http.StatusOK)
	defer arxivSrv.Close()

	s2Srv := newS2Server(t, s2Fixture, http.StatusOK, nil)
	defer s2Srv.Close()

	arxivClient := papersearch.NewArXivClient(arxivSrv.Client(), 0)
	arxivClient.BaseURL = arxivSrv.URL

	s2Client := papersearch.NewS2Client(s2Srv.Client(), 0)
	s2Client.BaseURL = s2Srv.URL

	ms := papersearch.NewMultiSearcher(papersearch.MultiSearcherConfig{
		ArXiv: arxivClient,
		S2:    s2Client,
	})

	papers, err := ms.Search(context.Background(), []string{"deep learning"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// arXiv fixture: 2 papers; S2 fixture: 2 papers → 4 unique (no overlap)
	if len(papers) < 2 {
		t.Errorf("expected >=2 papers, got %d", len(papers))
	}
}

func TestMultiSearcher_DedupByTitle(t *testing.T) {
	// Both backends return a paper with the same title.
	sameTitle := `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <id>http://arxiv.org/abs/2301.01234v1</id>
    <title>Deep Learning</title>
    <summary>Same title as S2.</summary>
    <author><name>Author</name></author>
  </entry>
</feed>`
	// S2 fixture also has "Deep Learning" as the first paper.

	arxivSrv := newArXivServer(t, sameTitle, http.StatusOK)
	defer arxivSrv.Close()

	s2Srv := newS2Server(t, s2Fixture, http.StatusOK, nil)
	defer s2Srv.Close()

	arxivClient := papersearch.NewArXivClient(arxivSrv.Client(), 0)
	arxivClient.BaseURL = arxivSrv.URL

	s2Client := papersearch.NewS2Client(s2Srv.Client(), 0)
	s2Client.BaseURL = s2Srv.URL

	ms := papersearch.NewMultiSearcher(papersearch.MultiSearcherConfig{
		ArXiv: arxivClient,
		S2:    s2Client,
	})

	papers, err := ms.Search(context.Background(), []string{"deep learning"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// "Deep Learning" should appear only once.
	count := 0
	for _, p := range papers {
		if p.Title == "Deep Learning" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 1 occurrence of 'Deep Learning', got %d", count)
	}
}

func TestMultiSearcher_OneBackendFails(t *testing.T) {
	// arXiv returns an HTTP error; S2 returns valid results.
	arxivSrv := newArXivServer(t, "error", http.StatusInternalServerError)
	defer arxivSrv.Close()

	s2Srv := newS2Server(t, s2Fixture, http.StatusOK, nil)
	defer s2Srv.Close()

	arxivClient := papersearch.NewArXivClient(arxivSrv.Client(), 0)
	arxivClient.BaseURL = arxivSrv.URL

	s2Client := papersearch.NewS2Client(s2Srv.Client(), 0)
	s2Client.BaseURL = s2Srv.URL

	ms := papersearch.NewMultiSearcher(papersearch.MultiSearcherConfig{
		ArXiv: arxivClient,
		S2:    s2Client,
	})

	// Should not return an error — graceful degradation.
	papers, err := ms.Search(context.Background(), []string{"deep learning"})
	if err != nil {
		t.Fatalf("unexpected error (should degrade gracefully): %v", err)
	}
	// S2 results should still come through.
	if len(papers) == 0 {
		t.Error("expected S2 results despite arXiv failure")
	}
}

func TestMultiSearcher_NilBackends(t *testing.T) {
	ms := papersearch.NewMultiSearcher(papersearch.MultiSearcherConfig{})
	papers, err := ms.Search(context.Background(), []string{"anything"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if papers != nil {
		t.Errorf("expected nil with no backends, got %v", papers)
	}
}

// ---- error backend for testing graceful degradation ----

// errBackend simulates a backend that always returns an error.
// We test this by pointing arXiv at a server that returns 500.
// The multi_test above covers this via HTTP — this documents the contract.
var _ = errors.New // ensure errors is used (suppress unused import if test is minimal)
