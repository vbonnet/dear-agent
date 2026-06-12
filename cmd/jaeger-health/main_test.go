package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRun_BadFlags(t *testing.T) {
	if got := run([]string{"--no-such-flag"}); got != 3 {
		t.Fatalf("run(bad flag) = %d, want 3", got)
	}
}

func TestFetchServices_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []string{"svc-a", "svc-b"},
		})
	}))
	defer srv.Close()

	svcs, err := fetchServices(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("fetchServices: %v", err)
	}
	if len(svcs) != 2 {
		t.Fatalf("got %d services, want 2", len(svcs))
	}
}

func TestFetchServices_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := fetchServices(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("fetchServices with 500: want error, got nil")
	}
}

func TestCountTraces_Empty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer srv.Close()

	n, err := countTraces(context.Background(), srv.Client(), srv.URL, "svc-a", "1h")
	if err != nil {
		t.Fatalf("countTraces: %v", err)
	}
	if n != 0 {
		t.Fatalf("got %d traces, want 0", n)
	}
}

func TestCountTraces_NonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []any{json.RawMessage(`{}`), json.RawMessage(`{}`)},
		})
	}))
	defer srv.Close()

	n, err := countTraces(context.Background(), srv.Client(), srv.URL, "svc-a", "1h")
	if err != nil {
		t.Fatalf("countTraces: %v", err)
	}
	if n != 2 {
		t.Fatalf("got %d traces, want 2", n)
	}
}
