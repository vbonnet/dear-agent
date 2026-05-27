package notify

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebhookDispatcher_NameAndCloseAreNoops(t *testing.T) {
	d := NewWebhookDispatcher("http://example.invalid")
	if d.Name() != "webhook" {
		t.Fatalf("Name = %q, want %q", d.Name(), "webhook")
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestWebhookDispatcher_PostsJSON(t *testing.T) {
	var got struct {
		body        Notification
		contentType string
		method      string
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.contentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(data, &got.body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL)
	n := &Notification{
		ID: "evt-1", Title: "t", Body: "b", Level: slog.LevelInfo, Source: "s",
	}
	if err := d.Dispatch(context.Background(), n); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got.contentType)
	}
	if got.body.ID != "evt-1" || got.body.Title != "t" {
		t.Errorf("server received wrong body: %+v", got.body)
	}
}

func TestWebhookDispatcher_DoesNotRetry4xx(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL,
		WithMaxRetries(5),
		WithRetryDelay(0), // no sleep between retries
	)
	err := d.Dispatch(context.Background(), &Notification{ID: "x"})
	if err == nil {
		t.Fatal("expected error from 400 response")
		return
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should mention 400, got: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("4xx must short-circuit retries; got %d calls", got)
	}
}

func TestWebhookDispatcher_Retries5xxThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL,
		WithMaxRetries(5),
		WithRetryDelay(time.Millisecond),
	)
	if err := d.Dispatch(context.Background(), &Notification{ID: "x"}); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 calls (2 failures + 1 success), got %d", got)
	}
}

func TestWebhookDispatcher_Retries5xxExhausts(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL,
		WithMaxRetries(2),
		WithRetryDelay(time.Millisecond),
	)
	err := d.Dispatch(context.Background(), &Notification{ID: "x"})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
		return
	}
	// maxRetries=2 → initial attempt + 2 retries = 3 calls.
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 calls (1 + 2 retries), got %d", got)
	}
	if !strings.Contains(err.Error(), "3 attempts") {
		t.Errorf("error should report attempt count, got: %v", err)
	}
}

func TestWebhookDispatcher_RespectsContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL,
		WithMaxRetries(10),
		WithRetryDelay(50*time.Millisecond),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := d.Dispatch(ctx, &Notification{ID: "x"})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestWebhookDispatcher_CustomHTTPClient(t *testing.T) {
	// Use WithHTTPClient to plug in a 0-timeout client and confirm the option
	// is wired through.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewWebhookDispatcher(srv.URL,
		WithHTTPClient(&http.Client{Timeout: 5 * time.Millisecond}),
		WithMaxRetries(0),
		WithRetryDelay(0),
	)
	err := d.Dispatch(context.Background(), &Notification{ID: "x"})
	if err == nil {
		t.Fatal("expected client timeout error")
	}
}
