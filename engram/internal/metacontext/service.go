package metacontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// MetacontextService interface defines metacontext analysis operations.
type MetacontextService interface {
	Analyze(ctx context.Context, req *AnalyzeRequest) (*Metacontext, error)
	InvalidateCache(ctx context.Context) error
	GetCacheStats(ctx context.Context) (*CacheStats, error)
}

// Service implements MetacontextService with scanner orchestration and caching.
type Service struct {
	cache               Cache
	scanners            []Scanner
	conversationScanner Scanner
}

// Scanner interface (re-exported from scanners package for convenience).
type Scanner interface {
	Name() string
	Scan(ctx context.Context, req *AnalyzeRequest) ([]Signal, error)
	Priority() int
}

// ScanResult contains scanner execution results.
type ScanResult struct {
	Scanner string
	Signals []Signal
	Error   error
	Timing  time.Duration
}

// NewService creates a new MetacontextService.
func NewService(cache Cache, scanners []Scanner, conversationScanner Scanner) *Service {
	return &Service{
		cache:               cache,
		scanners:            scanners,
		conversationScanner: conversationScanner,
	}
}

// Analyze orchestrates scanner execution and aggregates signals.
// Implements Section 3.1: MetacontextService.Analyze().
func (s *Service) Analyze(ctx context.Context, req *AnalyzeRequest) (*Metacontext, error) {
	// 0. Validate WorkingDir (CRITICAL - Path Traversal Defense M1)
	if err := ValidateWorkingDir(req.WorkingDir); err != nil {
		return nil, err
	}

	// 1. Generate cache key (hash(WorkingDir) only - CRITICAL FIX #2)
	cacheKey := s.generateCacheKey(req)

	// 2. Check cache
	if mc, ok := s.cache.GetMetacontext(ctx, cacheKey); ok {
		// 2a. Run ConversationScanner separately (uncached)
		convSignals, _ := s.conversationScanner.Scan(ctx, req)

		// 2b. Merge cached + conversation signals
		mc = s.mergeSignals(mc, convSignals, req)
		mc.Metadata.CacheHit = true
		return mc, nil
	}

	// 3. Run scanners (excludes ConversationScanner)
	results := s.runScanners(ctx, req)

	// 4. Aggregate signals
	mc := s.aggregateSignals(ctx, results, req)

	// 5. Run ConversationScanner separately
	convSignals, _ := s.conversationScanner.Scan(ctx, req)
	mc = s.mergeSignals(mc, convSignals, req)

	// 6. Store in cache
	s.cache.PutMetacontext(ctx, cacheKey, mc)

	// 7. Validate size
	if err := validateSize(mc); err != nil {
		return nil, err
	}

	mc.Metadata.CacheHit = false
	return mc, nil
}

// generateCacheKey creates cache key from WorkingDir only.
// CRITICAL FIX #2: Conversation NOT included in cache key.
func (s *Service) generateCacheKey(req *AnalyzeRequest) string {
	h := sha256.New()
	h.Write([]byte(req.WorkingDir))
	return hex.EncodeToString(h.Sum(nil))
}

// runScanners executes scanners in parallel (fan-out/fan-in pattern).
// Implements Section 3.2: Scanner Pipeline.
func (s *Service) runScanners(ctx context.Context, req *AnalyzeRequest) <-chan ScanResult {
	results := make(chan ScanResult, len(s.scanners))
	var wg sync.WaitGroup

	for _, scanner := range s.scanners {
		wg.Add(1)
		go func(sc Scanner) {
			defer wg.Done()

			// Panic recovery (SRE CRITICAL #2)
			defer func() {
				if r := recover(); r != nil {
					results <- ScanResult{
						Scanner: sc.Name(),
						Error:   fmt.Errorf("scanner panic: %v", r),
					}
				}
			}()

			start := time.Now()
			signals, err := sc.Scan(ctx, req)
			results <- ScanResult{
				Scanner: sc.Name(),
				Signals: signals,
				Error:   err,
				Timing:  time.Since(start),
			}
		}(scanner)
	}

	// Close results channel when all scanners complete
	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}

// aggregateSignals collects scanner results and builds metacontext.
// Implements Section 3.3: Signal Aggregation.
func (s *Service) aggregateSignals(_ context.Context, results <-chan ScanResult, req *AnalyzeRequest) *Metacontext {
	mc := &Metacontext{
		Languages:   []Signal{},
		Frameworks:  []Signal{},
		Tools:       []Signal{},
		Conventions: []Convention{},
		Personas:    []Persona{},
		Metadata:    MinimalMetadata{Warnings: []string{}},
	}

	allSignals := []Signal{}

	// 1. Collect all signals
	for result := range results {
		if result.Error != nil {
			mc.Metadata.Warnings = append(mc.Metadata.Warnings,
				fmt.Sprintf("%s: %v", result.Scanner, result.Error))
			continue
		}
		allSignals = append(allSignals, result.Signals...)
	}

	// 2. Group by type
	languages, frameworks, tools := groupByType(allSignals)

	// 3. Build context for importance scoring
	context := buildAnalyzeContext(req, allSignals)

	// 4. Deduplicate and truncate (importance-based)
	mc.Languages = deduplicateSignalsWithImportance(languages, context, MaxLanguageSignals)
	mc.Frameworks = deduplicateSignalsWithImportance(frameworks, context, MaxFrameworkSignals)
	mc.Tools = deduplicateSignalsWithImportance(tools, context, MaxToolSignals)

	return mc
}

// mergeSignals merges conversation signals into cached metacontext.
// Implements Section 3.4: ConversationScanner Separation.
func (s *Service) mergeSignals(cached *Metacontext, convSignals []Signal, req *AnalyzeRequest) *Metacontext {
	mc := cached.Clone()

	// Filter conversation signals by type
	_, convFrameworks, convTools := groupByType(convSignals)

	// Append to cached signals
	mc.Frameworks = append(mc.Frameworks, convFrameworks...)
	mc.Tools = append(mc.Tools, convTools...)

	// Deduplicate after merge (conversation signals may boost importance)
	allSignals := append(cached.AllSignals(), convSignals...)
	context := buildAnalyzeContext(req, allSignals)

	mc.Frameworks = deduplicateSignalsWithImportance(mc.Frameworks, context, MaxFrameworkSignals)
	mc.Tools = deduplicateSignalsWithImportance(mc.Tools, context, MaxToolSignals)

	return mc
}

// InvalidateCache purges all cache tiers.
func (s *Service) InvalidateCache(ctx context.Context) error {
	return s.cache.InvalidateAll(ctx)
}

// GetCacheStats returns cache statistics.
func (s *Service) GetCacheStats(ctx context.Context) (*CacheStats, error) {
	return s.cache.GetCacheStats(ctx)
}
