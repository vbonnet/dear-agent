// Package papersearch provides clients for academic paper search APIs.
// It currently supports arXiv and Semantic Scholar, with a MultiSearcher
// that fans out to both and deduplicates results.
package papersearch

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// ArXivClient searches the arXiv Atom-XML API.
// It enforces a minimum inter-request delay to comply with arXiv's
// 3-second-delay convention (https://arxiv.org/help/api/user-manual).
type ArXivClient struct {
	// HTTPClient is used for all requests. Defaults to http.DefaultClient.
	HTTPClient *http.Client
	// BaseURL is the arXiv API endpoint. Defaults to the canonical URL.
	// Override in tests with an httptest.Server URL.
	BaseURL string
	// MaxResults caps the number of papers returned per query.
	MaxResults int
	// limiter enforces at most 1 request per 3 seconds.
	limiter *rate.Limiter
}

// NewArXivClient creates an ArXivClient with sensible defaults.
// delaySeconds is the inter-request delay; pass 0 to use the default (3s).
func NewArXivClient(httpClient *http.Client, delaySeconds int) *ArXivClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if delaySeconds <= 0 {
		delaySeconds = 3
	}
	// rate.NewLimiter(r, b): r = 1 token per delaySeconds, burst = 1.
	r := rate.Every(time.Duration(delaySeconds) * time.Second)
	return &ArXivClient{
		HTTPClient: httpClient,
		BaseURL:    "http://export.arxiv.org/api/query",
		MaxResults: 10,
		limiter:    rate.NewLimiter(r, 1),
	}
}

// Paper is a result from a paper search API.
type Paper struct {
	PaperID  string
	Title    string
	Abstract string
	Source   string
}

// Search queries arXiv for papers matching the given keywords.
// keywords are joined with OR logic (arXiv's +OR+ syntax).
// The rate limiter blocks until the inter-request delay has elapsed.
func (c *ArXivClient) Search(ctx context.Context, keywords []string, maxResults int) ([]Paper, error) {
	if maxResults <= 0 {
		maxResults = c.MaxResults
	}
	if len(keywords) == 0 {
		return nil, nil
	}

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("arxiv: rate limiter: %w", err)
	}

	// Build the query: "all:kw1+OR+all:kw2"
	parts := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		parts = append(parts, "all:"+strings.ReplaceAll(kw, " ", "+"))
	}
	queryStr := strings.Join(parts, "+OR+")

	reqURL := c.BaseURL + "?search_query=" + url.QueryEscape(queryStr) +
		"&max_results=" + strconv.Itoa(maxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("arxiv: build request: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("arxiv: http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("arxiv: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("arxiv: read body: %w", err)
	}

	return parseArXivAtom(body)
}

// -- Atom XML parsing --------------------------------------------------------

// atomFeed is the minimal Atom structure for arXiv responses.
type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID      string       `xml:"id"`
	Title   string       `xml:"title"`
	Summary string       `xml:"summary"`
	Authors []atomAuthor `xml:"author"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func parseArXivAtom(data []byte) ([]Paper, error) {
	var feed atomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, fmt.Errorf("arxiv: parse atom: %w", err)
	}
	papers := make([]Paper, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		// arXiv IDs look like "http://arxiv.org/abs/2301.01234v1"
		arxivID := e.ID
		if idx := strings.LastIndex(arxivID, "/abs/"); idx >= 0 {
			arxivID = arxivID[idx+5:]
		}
		papers = append(papers, Paper{
			PaperID:  arxivID,
			Title:    strings.TrimSpace(e.Title),
			Abstract: strings.TrimSpace(e.Summary),
			Source:   "arxiv",
		})
	}
	return papers, nil
}
