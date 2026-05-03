package metacontext

import (
	"context"
	"testing"
)

// ============================================================================
// Integration Tests: service.go (Analyze() orchestration, cache integration)
// S7 Plan: Week 4 Testing, Integration Test Category
// ============================================================================

// MockScanner implements Scanner interface for testing
type MockScanner struct {
	name     string
	priority int
	signals  []Signal
	err      error
}

func (m *MockScanner) Name() string  { return m.name }
func (m *MockScanner) Priority() int { return m.priority }
func (m *MockScanner) Scan(ctx context.Context, req *AnalyzeRequest) ([]Signal, error) {
	return m.signals, m.err
}

// PanicScanner implements Scanner that panics for testing panic recovery
type PanicScanner struct {
	name     string
	priority int
}

func (p *PanicScanner) Name() string  { return p.name }
func (p *PanicScanner) Priority() int { return p.priority }
func (p *PanicScanner) Scan(ctx context.Context, req *AnalyzeRequest) ([]Signal, error) {
	panic("scanner panic")
}

// TestService_Analyze_CacheMiss tests full analysis flow (cache miss)
func TestService_Analyze_CacheMiss(t *testing.T) {
	// Create temp directory
	tmpdir := t.TempDir()

	// Create cache
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	// Create mock scanners
	scanners := []Scanner{
		&MockScanner{
			name:     "file",
			priority: 30,
			signals: []Signal{
				{Name: "Go", Confidence: 0.95, Source: "file"},
			},
		},
		&MockScanner{
			name:     "dependency",
			priority: 40,
			signals: []Signal{
				{Name: "Gin", Confidence: 0.9, Source: "dependency"},
			},
		},
	}

	conversationScanner := &MockScanner{
		name:     "conversation",
		priority: 10,
		signals: []Signal{
			{Name: "Vue", Confidence: 0.7, Source: "conversation"},
		},
	}

	service := NewService(cache, scanners, conversationScanner)

	// Run analysis
	ctx := context.Background()
	req := &AnalyzeRequest{WorkingDir: tmpdir}

	mc, err := service.Analyze(ctx, req)
	if err != nil {
		t.Fatalf("Analyze() failed: %v", err)
	}

	// Verify metacontext
	if mc == nil {
		t.Fatal("Metacontext should not be nil")
	}
	if mc.Metadata.CacheHit {
		t.Error("First analysis should be cache miss")
	}

	// Verify signals aggregated
	allSignals := mc.AllSignals()
	if len(allSignals) < 3 {
		t.Errorf("Expected at least 3 signals, got %d", len(allSignals))
	}

	// Verify Go, Gin, Vue all present
	hasGo, hasGin, hasVue := false, false, false
	for _, sig := range allSignals {
		if sig.Name == "Go" {
			hasGo = true
		}
		if sig.Name == "Gin" {
			hasGin = true
		}
		if sig.Name == "Vue" {
			hasVue = true
		}
	}
	if !hasGo || !hasGin || !hasVue {
		t.Error("Analyze() should aggregate signals from all scanners")
	}
}

// TestService_Analyze_CacheHit tests cache hit path
func TestService_Analyze_CacheHit(t *testing.T) {
	tmpdir := t.TempDir()

	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	scanners := []Scanner{
		&MockScanner{
			name:     "file",
			priority: 30,
			signals:  []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
		},
	}

	conversationScanner := &MockScanner{
		name:     "conversation",
		priority: 10,
		signals:  []Signal{{Name: "React", Confidence: 0.8, Source: "conversation"}},
	}

	service := NewService(cache, scanners, conversationScanner)
	ctx := context.Background()
	req := &AnalyzeRequest{WorkingDir: tmpdir}

	// First analysis (cache miss)
	mc1, err := service.Analyze(ctx, req)
	if err != nil {
		t.Fatalf("First Analyze() failed: %v", err)
	}
	if mc1.Metadata.CacheHit {
		t.Error("First analysis should be cache miss")
	}

	// Second analysis with different conversation (cache hit)
	req2 := &AnalyzeRequest{
		WorkingDir:   tmpdir,
		Conversation: []string{"How do I use Vue?"}, // Different conversation
	}

	conversationScanner2 := &MockScanner{
		name:     "conversation",
		priority: 10,
		signals:  []Signal{{Name: "Vue", Confidence: 0.7, Source: "conversation"}}, // Different signal
	}

	service2 := NewService(cache, scanners, conversationScanner2)
	mc2, err := service2.Analyze(ctx, req2)
	if err != nil {
		t.Fatalf("Second Analyze() failed: %v", err)
	}

	// CRITICAL FIX #2: Cache key = WorkingDir only, so should be cache hit
	if !mc2.Metadata.CacheHit {
		t.Error("Second analysis should be cache hit (WorkingDir-only cache key)")
	}

	// Conversation signals should be merged (Vue, not React)
	hasVue := false
	for _, sig := range mc2.AllSignals() {
		if sig.Name == "Vue" {
			hasVue = true
		}
	}
	if !hasVue {
		t.Error("Cache hit should merge conversation signals")
	}
}

// TestService_Analyze_InvalidWorkingDir tests path validation
func TestService_Analyze_InvalidWorkingDir(t *testing.T) {
	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	service := NewService(cache, []Scanner{}, &MockScanner{})
	ctx := context.Background()

	// Invalid path (path traversal)
	req := &AnalyzeRequest{WorkingDir: "/tmp/../etc/passwd"}

	_, err = service.Analyze(ctx, req)
	if err == nil {
		t.Error("Analyze() should reject invalid working directory")
	}
}

// TestService_Analyze_ScannerPanicRecovery tests panic recovery
func TestService_Analyze_ScannerPanicRecovery(t *testing.T) {
	tmpdir := t.TempDir()

	cache, err := NewUnifiedCache()
	if err != nil {
		t.Fatalf("NewUnifiedCache() failed: %v", err)
	}

	// Create panic scanner
	panicScanner := &PanicScanner{name: "panic-scanner", priority: 50}

	// Create normal scanner
	normalScanner := &MockScanner{
		name:     "normal",
		priority: 30,
		signals:  []Signal{{Name: "Go", Confidence: 0.95, Source: "file"}},
	}

	scanners := []Scanner{panicScanner, normalScanner}
	conversationScanner := &MockScanner{name: "conversation", priority: 10}

	service := NewService(cache, scanners, conversationScanner)
	ctx := context.Background()
	req := &AnalyzeRequest{WorkingDir: tmpdir}

	// Should not panic (panic recovered)
	mc, err := service.Analyze(ctx, req)
	if err != nil {
		t.Fatalf("Analyze() should recover from scanner panic: %v", err)
	}

	// Normal scanner should still execute
	hasGo := false
	for _, sig := range mc.AllSignals() {
		if sig.Name == "Go" {
			hasGo = true
		}
	}
	if !hasGo {
		t.Error("Non-panicking scanners should still execute")
	}
}

// TestService_GenerateCacheKey tests CRITICAL FIX #2
func TestService_GenerateCacheKey(t *testing.T) {
	cache, _ := NewUnifiedCache()
	service := NewService(cache, []Scanner{}, &MockScanner{})

	req1 := &AnalyzeRequest{
		WorkingDir:   "/tmp/project",
		Conversation: []string{"How do I use React?"},
	}

	req2 := &AnalyzeRequest{
		WorkingDir:   "/tmp/project",                // Same directory
		Conversation: []string{"How do I use Vue?"}, // Different conversation
	}

	key1 := service.generateCacheKey(req1)
	key2 := service.generateCacheKey(req2)

	// CRITICAL FIX #2: Cache key based on WorkingDir only
	if key1 != key2 {
		t.Error("Cache key should be same for same WorkingDir (conversation not in key)")
	}

	req3 := &AnalyzeRequest{
		WorkingDir:   "/tmp/different", // Different directory
		Conversation: []string{"How do I use React?"},
	}

	key3 := service.generateCacheKey(req3)

	if key1 == key3 {
		t.Error("Cache key should differ for different WorkingDir")
	}
}

// TestService_InvalidateCache tests cache invalidation
func TestService_InvalidateCache(t *testing.T) {
	tmpdir := t.TempDir()

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

	// Run analysis (cache miss)
	mc1, _ := service.Analyze(ctx, req)
	if mc1.Metadata.CacheHit {
		t.Error("First analysis should be cache miss")
	}

	// Invalidate cache
	err := service.InvalidateCache(ctx)
	if err != nil {
		t.Errorf("InvalidateCache() failed: %v", err)
	}

	// Run analysis again (should be cache miss after invalidation)
	mc2, _ := service.Analyze(ctx, req)
	if mc2.Metadata.CacheHit {
		t.Error("Analysis after invalidation should be cache miss")
	}
}

// TestService_GetCacheStats tests cache statistics
func TestService_GetCacheStats(t *testing.T) {
	tmpdir := t.TempDir()

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

	// Trigger cache miss
	req := &AnalyzeRequest{WorkingDir: tmpdir}
	service.Analyze(ctx, req)

	// Trigger cache hit
	service.Analyze(ctx, req)

	stats, err := service.GetCacheStats(ctx)
	if err != nil {
		t.Errorf("GetCacheStats() failed: %v", err)
	}

	if stats.Hits < 1 {
		t.Errorf("Expected at least 1 hit, got %d", stats.Hits)
	}
	if stats.Misses < 1 {
		t.Errorf("Expected at least 1 miss, got %d", stats.Misses)
	}
}
