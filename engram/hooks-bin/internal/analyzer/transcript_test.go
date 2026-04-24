package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTranscript_ValidJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	data := `{"parentUuid":null,"sessionId":"sess1","type":"user","uuid":"u1","timestamp":"2026-01-09T00:11:57.427Z","message":{"role":"user","content":"hello"}}
{"parentUuid":"u1","sessionId":"sess1","type":"assistant","uuid":"u2","timestamp":"2026-01-09T00:11:58.427Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"toolu_001","name":"Bash","input":{"command":"git status"}}]}}
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].SessionID != "sess1" {
		t.Errorf("expected sessionId sess1, got %s", entries[0].SessionID)
	}
	if entries[0].Type != "user" {
		t.Errorf("expected type user, got %s", entries[0].Type)
	}
	if entries[1].Type != "assistant" {
		t.Errorf("expected type assistant, got %s", entries[1].Type)
	}
	if entries[1].UUID != "u2" {
		t.Errorf("expected uuid u2, got %s", entries[1].UUID)
	}
}

func TestReadTranscript_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	data := `{"sessionId":"s1","type":"user","uuid":"u1","message":{"role":"user","content":"hi"}}

{"sessionId":"s1","type":"assistant","uuid":"u2","message":{"role":"assistant","content":"bye"}}

`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestReadTranscript_SkipsMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")

	data := `{"sessionId":"s1","type":"user","uuid":"u1","message":{"role":"user","content":"hi"}}
this is not json
{"sessionId":"s1","type":"assistant","uuid":"u2","message":{"role":"assistant","content":"bye"}}
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadTranscript(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (skipping malformed), got %d", len(entries))
	}
}

func TestReadTranscript_MissingFile(t *testing.T) {
	_, err := ReadTranscript("/nonexistent/path/transcript.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestTranscriptCache_HitAndMiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	data := `{"sessionId":"s1","type":"user","uuid":"u1","message":{"role":"user","content":"hi"}}
`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}

	cache := NewTranscriptCache(2)

	// First call: cache miss, loads from disk.
	entries1, err := cache.Get(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries1) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries1))
	}

	// Second call: cache hit, same result.
	entries2, err := cache.Get(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry from cache, got %d", len(entries2))
	}
}

func TestTranscriptCache_Eviction(t *testing.T) {
	dir := t.TempDir()
	cache := NewTranscriptCache(2)

	// Create 3 transcript files.
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, "t"+string(rune('0'+i))+".jsonl")
		data := `{"sessionId":"s","type":"user","uuid":"u","message":{"role":"user","content":"x"}}
`
		if err := os.WriteFile(path, []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}

	p0 := filepath.Join(dir, "t0.jsonl")
	p1 := filepath.Join(dir, "t1.jsonl")
	p2 := filepath.Join(dir, "t2.jsonl")

	// Load first two — fills cache.
	if _, err := cache.Get(p0); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Get(p1); err != nil {
		t.Fatal(err)
	}

	// Cache should have 2 entries.
	if len(cache.cache) != 2 {
		t.Fatalf("expected cache size 2, got %d", len(cache.cache))
	}

	// Load third — should evict p0.
	if _, err := cache.Get(p2); err != nil {
		t.Fatal(err)
	}
	if len(cache.cache) != 2 {
		t.Fatalf("expected cache size 2 after eviction, got %d", len(cache.cache))
	}
	if _, ok := cache.cache[p0]; ok {
		t.Error("expected p0 to be evicted from cache")
	}
	if _, ok := cache.cache[p1]; !ok {
		t.Error("expected p1 to still be in cache")
	}
	if _, ok := cache.cache[p2]; !ok {
		t.Error("expected p2 to be in cache")
	}
}

func TestTranscriptCache_MissingFile(t *testing.T) {
	cache := NewTranscriptCache(2)
	_, err := cache.Get("/nonexistent/file.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	// Error should be cached too — same error on second call.
	_, err2 := cache.Get("/nonexistent/file.jsonl")
	if err2 == nil {
		t.Fatal("expected cached error for missing file")
	}
}
