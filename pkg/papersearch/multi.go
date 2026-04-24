package papersearch

import (
	"context"
	"log/slog"
	"strings"
	"sync"
)

// Backend is a generic paper search backend used by MultiSearcher.
type Backend interface {
	Search(ctx context.Context, keywords []string, maxResults int) ([]Paper, error)
}

// s2Backend wraps S2Client to implement the Backend interface.
// S2 takes a single query string, so we join keywords with spaces.
type s2Backend struct {
	client     *S2Client
	maxResults int
}

func (b *s2Backend) Search(ctx context.Context, keywords []string, maxResults int) ([]Paper, error) {
	if maxResults <= 0 {
		maxResults = b.maxResults
	}
	query := strings.Join(keywords, " ")
	return b.client.Search(ctx, query, maxResults)
}

// arxivBackend wraps ArXivClient to implement the Backend interface.
type arxivBackend struct {
	client     *ArXivClient
	maxResults int
}

func (b *arxivBackend) Search(ctx context.Context, keywords []string, maxResults int) ([]Paper, error) {
	if maxResults <= 0 {
		maxResults = b.maxResults
	}
	return b.client.Search(ctx, keywords, maxResults)
}

// MultiSearcher fans out to N backends and merges their results,
// deduplicating by normalized title. Results are ordered arXiv-first,
// then Semantic Scholar (matching the Python source_search.py ordering).
type MultiSearcher struct {
	backends   []Backend
	maxResults int
	logger     *slog.Logger
}

// MultiSearcherConfig holds configuration for one search backend.
type MultiSearcherConfig struct {
	// ArXiv backend. nil means disabled.
	ArXiv *ArXivClient
	// ArXivMaxResults caps arXiv results. 0 = use client default.
	ArXivMaxResults int
	// S2 backend. nil means disabled.
	S2 *S2Client
	// S2MaxResults caps S2 results. 0 = use client default.
	S2MaxResults int
	// MaxResults total per MultiSearcher.Search call (sum across backends).
	MaxResults int
	// Logger for degradation warnings. nil = slog.Default().
	Logger *slog.Logger
}

// NewMultiSearcher builds a MultiSearcher from the given config.
// Backends are ordered arXiv first, then S2, mirroring the Python ordering.
func NewMultiSearcher(cfg MultiSearcherConfig) *MultiSearcher {
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxResults := cfg.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}
	ms := &MultiSearcher{
		maxResults: maxResults,
		logger:     logger,
	}
	if cfg.ArXiv != nil {
		ms.backends = append(ms.backends, &arxivBackend{
			client:     cfg.ArXiv,
			maxResults: cfg.ArXivMaxResults,
		})
	}
	if cfg.S2 != nil {
		ms.backends = append(ms.backends, &s2Backend{
			client:     cfg.S2,
			maxResults: cfg.S2MaxResults,
		})
	}
	return ms
}

// Search fans out to all configured backends concurrently, then merges
// and deduplicates results by normalized title.
// If a backend returns an error, the error is logged and that backend's
// results are skipped (graceful degradation).
func (m *MultiSearcher) Search(ctx context.Context, keywords []string) ([]Paper, error) {
	if len(m.backends) == 0 {
		return nil, nil
	}

	type result struct {
		papers []Paper
		err    error
	}

	results := make([]result, len(m.backends))
	var wg sync.WaitGroup
	wg.Add(len(m.backends))

	for i, b := range m.backends {
		i, b := i, b
		go func() {
			defer wg.Done()
			papers, err := b.Search(ctx, keywords, m.maxResults)
			results[i] = result{papers: papers, err: err}
		}()
	}
	wg.Wait()

	// Merge in backend order (arXiv first, S2 second), deduplicate by title.
	seen := make(map[string]struct{})
	var merged []Paper
	for i, r := range results {
		if r.err != nil {
			m.logger.Warn("papersearch: backend error; skipping",
				"backend_index", i, "err", r.err)
			continue
		}
		for _, p := range r.papers {
			key := normalizeTitle(p.Title)
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			merged = append(merged, p)
		}
	}
	return merged, nil
}

// normalizeTitle lowercases and strips leading/trailing whitespace so that
// "Attention Is All You Need" and "attention is all you need" compare equal.
func normalizeTitle(title string) string {
	return strings.ToLower(strings.TrimSpace(title))
}
