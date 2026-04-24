package metacontext

import (
	"context"
	"os"
	"testing"
	"time"
)

// ============================================================================
// Performance Benchmarks: Validate S6 performance budgets
// S7 Plan: Week 4 Testing, Benchmark Category
// Target: Cache hit <10.1ms, Cache miss <95ms, Overall <100ms (p95)
// ============================================================================

// BenchmarkCache_Hit benchmarks cache hit path (target: p95 <10.1ms)
func BenchmarkCache_Hit(b *testing.B) {
	cache, err := NewUnifiedCache()
	if err != nil {
		b.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()
	key := "benchmark-key"
	mc := &Metacontext{
		Languages: []Signal{
			{Name: "Go", Confidence: 0.95, Source: "file"},
		},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
	}

	// Pre-populate cache
	cache.PutMetacontext(ctx, key, mc)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := cache.GetMetacontext(ctx, key)
		if !ok {
			b.Fatal("Cache hit should return true")
		}
	}
}

// BenchmarkCache_Miss benchmarks cache miss path (target: p95 <95ms)
func BenchmarkCache_Miss(b *testing.B) {
	cache, err := NewUnifiedCache()
	if err != nil {
		b.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Different key each iteration (always miss)
		key := string(rune('a' + (i % 26)))
		_, ok := cache.GetMetacontext(ctx, key)
		if ok {
			b.Fatal("Cache miss should return false")
		}
	}
}

// BenchmarkService_Analyze_CacheHit benchmarks full Analyze() with cache hit
// Target: p95 <10.1ms (cached path)
func BenchmarkService_Analyze_CacheHit(b *testing.B) {
	// Create temp directory
	tmpdir, err := os.MkdirTemp("", "bench-service-*")
	if err != nil {
		b.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	cache, _ := NewUnifiedCache()
	scanners := []Scanner{
		&MockScanner{
			name:     "file",
			priority: 30,
			signals:  []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
		},
	}
	conversationScanner := &MockScanner{name: "conversation", priority: 10}

	service := NewService(cache, scanners, conversationScanner)
	ctx := context.Background()
	req := &AnalyzeRequest{WorkingDir: tmpdir}

	// Pre-populate cache
	service.Analyze(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mc, err := service.Analyze(ctx, req)
		if err != nil {
			b.Fatalf("Analyze() failed: %v", err)
		}
		if !mc.Metadata.CacheHit {
			b.Fatal("Should be cache hit")
		}
	}
}

// BenchmarkService_Analyze_CacheMiss benchmarks full Analyze() with cache miss
// Target: p95 <95ms (full analysis path)
func BenchmarkService_Analyze_CacheMiss(b *testing.B) {
	cache, _ := NewUnifiedCache()
	scanners := []Scanner{
		&MockScanner{
			name:     "file",
			priority: 30,
			signals:  []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
		},
		&MockScanner{
			name:     "dependency",
			priority: 40,
			signals:  []Signal{{Name: "Gin", Confidence: 0.9, Source: "dependency"}},
		},
	}
	conversationScanner := &MockScanner{
		name:     "conversation",
		priority: 10,
		signals:  []Signal{{Name: "Vue", Confidence: 0.7, Source: "conversation"}},
	}

	service := NewService(cache, scanners, conversationScanner)
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create unique temp dir each iteration (different cache key)
		tmpdir, err := os.MkdirTemp("", "bench-miss-*")
		if err != nil {
			b.Fatalf("Failed to create temp dir: %v", err)
		}

		req := &AnalyzeRequest{WorkingDir: tmpdir}
		mc, err := service.Analyze(ctx, req)
		if err != nil {
			os.RemoveAll(tmpdir)
			b.Fatalf("Analyze() failed: %v", err)
		}
		if mc.Metadata.CacheHit {
			os.RemoveAll(tmpdir)
			b.Fatal("Should be cache miss")
		}

		os.RemoveAll(tmpdir)
	}
}

// BenchmarkSignalDeduplication benchmarks importance-based deduplication
func BenchmarkSignalDeduplication(b *testing.B) {
	// Create 100 signals with duplicates
	signals := make([]Signal, 100)
	for i := 0; i < 100; i++ {
		signals[i] = Signal{
			Name:       string(rune('A' + (i % 26))), // 26 unique names
			Confidence: float64(100-i) / 100.0,
			Source:     "file",
		}
	}

	context := AnalyzeContext{
		FileModTimes:        make(map[string]time.Time),
		ConversationMatches: make(map[string]int),
		PrimarySignals:      make(map[string]bool),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := deduplicateSignalsWithImportance(signals, context, 15)
		if len(result) > 15 {
			b.Fatalf("Expected max 15 signals, got %d", len(result))
		}
	}
}
