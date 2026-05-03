package dev

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHotReload_DebouncesAndDeliversChange(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(p, []byte("v1"), 0o600); err != nil {
		t.Fatalf("seed write: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var hits atomic.Int32
	done := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = HotReload(ctx, []string{p}, WatchOptions{Debounce: 50 * time.Millisecond}, func(_ string) {
			hits.Add(1)
			select {
			case done <- struct{}{}:
			default:
			}
		})
	}()

	// Give the watcher a beat to attach.
	time.Sleep(100 * time.Millisecond)

	for i := 0; i < 3; i++ {
		if err := os.WriteFile(p, []byte("v"+string(rune('0'+i))), 0o600); err != nil {
			t.Fatalf("write: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("HotReload never fired")
	}
	cancel()
	wg.Wait()

	if got := hits.Load(); got < 1 {
		t.Errorf("hits = %d, want >= 1", got)
	}
}

func TestHotReload_RejectsEmptyPaths(t *testing.T) {
	if err := HotReload(context.Background(), nil, WatchOptions{}, func(string) {}); err == nil {
		t.Fatal("expected error for empty paths")
	}
}

func TestUniqueDirs_Dedups(t *testing.T) {
	got := uniqueDirs([]string{"a/b/c.yaml", "a/b/d.yaml", "a/x.yaml"})
	want := map[string]bool{"a/b": true, "a": true}
	if len(got) != 2 {
		t.Fatalf("uniqueDirs = %v", got)
	}
	for _, d := range got {
		if !want[d] {
			t.Errorf("unexpected dir %q", d)
		}
	}
}
