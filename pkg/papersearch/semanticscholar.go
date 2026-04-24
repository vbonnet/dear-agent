package papersearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"golang.org/x/time/rate"
)

// S2Client searches the Semantic Scholar Graph API.
// It reads an optional API key from the SEMANTIC_SCHOLAR_API_KEY environment
// variable; authenticated requests have higher rate limits.
//
// Default rate: 80 requests per 5 minutes (leaving buffer under the 100/5min
// unauthenticated limit, and well under the authenticated 1 req/s limit).
type S2Client struct {
	// HTTPClient is used for all requests. Defaults to http.DefaultClient.
	HTTPClient *http.Client
	// BaseURL is the Semantic Scholar API endpoint. Defaults to the canonical URL.
	BaseURL string
	// MaxResults caps the number of papers returned per query.
	MaxResults int
	// APIKey, if non-empty, is sent as the x-api-key header.
	APIKey string
	// limiter enforces the configured request rate.
	limiter *rate.Limiter
}

// NewS2Client creates a S2Client with sensible defaults.
// requestsPer5Min is the maximum requests in a 5-minute window;
// pass 0 to use the default (80).
func NewS2Client(httpClient *http.Client, requestsPer5Min int) *S2Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if requestsPer5Min <= 0 {
		requestsPer5Min = 80
	}
	// Convert requests/5min → rate.Every per-request interval.
	interval := time.Duration(5*60*int(time.Second)) / time.Duration(requestsPer5Min)
	return &S2Client{
		HTTPClient: httpClient,
		BaseURL:    "https://api.semanticscholar.org/graph/v1/paper/search",
		MaxResults: 10,
		APIKey:     os.Getenv("SEMANTIC_SCHOLAR_API_KEY"),
		limiter:    rate.NewLimiter(rate.Every(interval), 1),
	}
}

// Search queries Semantic Scholar for papers matching query.
func (c *S2Client) Search(ctx context.Context, query string, maxResults int) ([]Paper, error) {
	if maxResults <= 0 {
		maxResults = c.MaxResults
	}
	if query == "" {
		return nil, nil
	}

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("s2: rate limiter: %w", err)
	}

	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", strconv.Itoa(maxResults))
	params.Set("fields", "paperId,title,abstract,authors,year,citationCount")

	reqURL := c.BaseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("s2: build request: %w", err)
	}
	if c.APIKey != "" {
		req.Header.Set("x-api-key", c.APIKey)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("s2: http: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // best-effort close

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("s2: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("s2: read body: %w", err)
	}

	return parseS2Response(body)
}

// -- JSON parsing ------------------------------------------------------------

type s2Response struct {
	Data []s2Paper `json:"data"`
}

type s2Paper struct {
	PaperID      string     `json:"paperId"`
	Title        string     `json:"title"`
	Abstract     *string    `json:"abstract"`
	Authors      []s2Author `json:"authors"`
	Year         *int       `json:"year"`
	CitationCount int       `json:"citationCount"`
}

type s2Author struct {
	Name string `json:"name"`
}

func parseS2Response(data []byte) ([]Paper, error) {
	var resp s2Response
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("s2: parse json: %w", err)
	}
	papers := make([]Paper, 0, len(resp.Data))
	for _, item := range resp.Data {
		abstract := item.Title // fallback: use title if no abstract
		if item.Abstract != nil && *item.Abstract != "" {
			abstract = *item.Abstract
		}
		papers = append(papers, Paper{
			PaperID:  item.PaperID,
			Title:    item.Title,
			Abstract: abstract,
			Source:   "semantic_scholar",
		})
	}
	return papers, nil
}
