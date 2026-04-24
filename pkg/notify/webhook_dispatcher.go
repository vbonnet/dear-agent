package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WebhookDispatcher sends notifications as HTTP POST JSON payloads.
type WebhookDispatcher struct {
	url        string
	client     *http.Client
	maxRetries int
	retryDelay time.Duration
}

// WebhookOption configures a WebhookDispatcher.
type WebhookOption func(*WebhookDispatcher)

// WithMaxRetries sets the maximum number of retry attempts for failed requests.
func WithMaxRetries(n int) WebhookOption {
	return func(d *WebhookDispatcher) { d.maxRetries = n }
}

// WithRetryDelay sets the base delay between retries.
func WithRetryDelay(delay time.Duration) WebhookOption {
	return func(d *WebhookDispatcher) { d.retryDelay = delay }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) WebhookOption {
	return func(d *WebhookDispatcher) { d.client = c }
}

// NewWebhookDispatcher creates a dispatcher that POSTs notifications to url.
func NewWebhookDispatcher(url string, opts ...WebhookOption) *WebhookDispatcher {
	d := &WebhookDispatcher{
		url:        url,
		client:     &http.Client{Timeout: 10 * time.Second},
		maxRetries: 3,
		retryDelay: 1 * time.Second,
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

func (d *WebhookDispatcher) Name() string { return "webhook" }

func (d *WebhookDispatcher) Dispatch(ctx context.Context, n *Notification) error {
	body, err := json.Marshal(n)
	if err != nil {
		return fmt.Errorf("marshal notification: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= d.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(d.retryDelay * time.Duration(attempt)):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)

		// Don't retry client errors (4xx).
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return lastErr
		}
	}
	return fmt.Errorf("webhook failed after %d attempts: %w", d.maxRetries+1, lastErr)
}

func (d *WebhookDispatcher) Close() error { return nil }
