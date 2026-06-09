package deepresearch

import (
	"fmt"
	"sort"
	"testing"
	"time"
)

// withTimeout runs fn and fails the test if it does not return within d. This
// catches the regression directly: the old code deadlocked when a worker
// panicked, so a hang here means the bug is back.
func withTimeout(t *testing.T, d time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("runResearchWorkers did not return within %s — likely deadlocked", d)
	}
}

func TestRunResearchWorkers_AllSucceed(t *testing.T) {
	urls := []string{"a", "b", "c"}
	var results []researchResult
	withTimeout(t, 5*time.Second, func() {
		results = runResearchWorkers(urls, func(_ int, url string) (string, error) {
			return "report-" + url, nil
		})
	})

	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d", len(results), len(urls))
	}
	sort.Slice(results, func(i, j int) bool { return results[i].url < results[j].url })
	for i, url := range urls {
		if results[i].url != url {
			t.Errorf("result[%d].url = %q, want %q", i, results[i].url, url)
		}
		if results[i].err != nil {
			t.Errorf("result for %q: unexpected err %v", url, results[i].err)
		}
		if results[i].reportPath != "report-"+url {
			t.Errorf("result for %q: reportPath = %q", url, results[i].reportPath)
		}
	}
}

// TestRunResearchWorkers_PanicDoesNotDeadlock is the regression test for the
// gemini.go deadlock: a worker that panics must not strand the collector. It
// must still return exactly one result per URL, with the panicking URL's result
// carrying a non-nil error.
func TestRunResearchWorkers_PanicDoesNotDeadlock(t *testing.T) {
	urls := []string{"ok-1", "boom", "ok-2"}
	var results []researchResult
	withTimeout(t, 5*time.Second, func() {
		results = runResearchWorkers(urls, func(_ int, url string) (string, error) {
			if url == "boom" {
				panic("simulated research crash")
			}
			return "report-" + url, nil
		})
	})

	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d (a missing send means the panic was not recovered)", len(results), len(urls))
	}

	byURL := map[string]researchResult{}
	for _, r := range results {
		byURL[r.url] = r
	}
	if got := byURL["boom"]; got.err == nil {
		t.Error("expected non-nil err for the panicking URL, got nil")
	}
	for _, url := range []string{"ok-1", "ok-2"} {
		if got := byURL[url]; got.err != nil {
			t.Errorf("expected nil err for %q, got %v", url, got.err)
		}
	}
}

// TestRunResearchWorkers_AllPanic ensures even an all-panic batch terminates
// (no surviving sender means no deadlock) and reports every URL as failed.
func TestRunResearchWorkers_AllPanic(t *testing.T) {
	urls := []string{"x", "y"}
	var results []researchResult
	withTimeout(t, 5*time.Second, func() {
		results = runResearchWorkers(urls, func(_ int, url string) (string, error) {
			panic(fmt.Sprintf("crash-%s", url))
		})
	})
	if len(results) != len(urls) {
		t.Fatalf("got %d results, want %d", len(results), len(urls))
	}
	for _, r := range results {
		if r.err == nil {
			t.Errorf("expected err for %q, got nil", r.url)
		}
	}
}

func TestRunResearchWorkers_Empty(t *testing.T) {
	var results []researchResult
	withTimeout(t, 5*time.Second, func() {
		results = runResearchWorkers(nil, func(_ int, _ string) (string, error) {
			t.Error("fn should not be called for an empty URL list")
			return "", nil
		})
	})
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
}
