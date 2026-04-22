package analyzer

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ReadTranscript reads a JSONL transcript file and returns entries in order.
// Blank lines and malformed entries are skipped with a warning printed to stderr.
func ReadTranscript(path string) ([]TranscriptEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open transcript %s: %w", path, err)
	}
	defer f.Close()

	var entries []TranscriptEntry
	scanner := bufio.NewScanner(f)
	// Allow large lines (up to 10MB) for transcripts with big content blocks.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry TranscriptEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			fmt.Fprintf(os.Stderr, "warning: transcript %s line %d: %v\n", path, lineNum, err)
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, fmt.Errorf("reading transcript %s: %w", path, err)
	}
	return entries, nil
}

// TranscriptCache provides lazy-loading and caching of transcript files
// with a bounded cache size. When the cache is full, the oldest entry is evicted.
type TranscriptCache struct {
	mu      sync.Mutex
	cache   map[string]cacheEntry
	order   []string // insertion order for eviction
	maxSize int
}

type cacheEntry struct {
	entries []TranscriptEntry
	err     error
}

// NewTranscriptCache creates a new cache that holds at most maxSize transcripts.
func NewTranscriptCache(maxSize int) *TranscriptCache {
	if maxSize <= 0 {
		maxSize = 16
	}
	return &TranscriptCache{
		cache:   make(map[string]cacheEntry, maxSize),
		order:   make([]string, 0, maxSize),
		maxSize: maxSize,
	}
}

// Get returns the transcript entries for the given path, loading from disk
// on the first access and caching the result. Errors are also cached.
func (c *TranscriptCache) Get(path string) ([]TranscriptEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ce, ok := c.cache[path]; ok {
		return ce.entries, ce.err
	}

	// Evict oldest if at capacity.
	if len(c.cache) >= c.maxSize {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.cache, oldest)
	}

	entries, err := ReadTranscript(path)
	c.cache[path] = cacheEntry{entries: entries, err: err}
	c.order = append(c.order, path)
	return entries, err
}
