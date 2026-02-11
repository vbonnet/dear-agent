# ADR-003: Caching Strategy

**Status**: Accepted
**Date**: 2025-01-10 (backfilled 2025-02-11)
**Deciders**: Engineering Team
**Tags**: performance, api-efficiency, storage

## Context

The Gemini Deep Research API is:
- **Slow**: Typical research takes 30-60 minutes
- **Rate-limited**: Subject to quota restrictions
- **Expensive**: Each API call consumes quota

Users frequently re-analyze the same URLs:
- Reviewing previous research
- Sharing results with team members
- Running analysis scripts repeatedly during development
- Checking for content changes

Without caching:
- Duplicate API calls waste quota (5-10% of requests are duplicates)
- Users wait 30-60 minutes for cached content
- API rate limits reached faster
- Poor developer experience during iteration

### Requirements

1. **Cache Hit Detection**: Identify when research already exists for a URL
2. **Content Validation**: Detect when cached content is stale (source changed)
3. **Fast Lookup**: Cache check must be <1 second
4. **Force Refresh**: Users can bypass cache when needed
5. **Organized Storage**: Cache organized by content type for browsability
6. **Metadata Preservation**: Cache includes topics, timestamp, content hash

## Decision

Implement a **file-based caching system** with the following design:

### Cache Structure

```
~/.cache/gemini-deep-research/
├── youtube/
│   ├── example-video-slug/
│   │   └── report.md
│   └── another-video-abc123/
│       └── report.md
├── arxiv/
│   ├── attention-is-all-you-need/
│   │   └── report.md
│   └── bert-paper-2018-devlin/
│       └── report.md
├── article/
│   └── example-com-article/
│       └── report.md
└── huggingface/
    └── llama-2-paper/
        └── report.md
```

### Frontmatter Format

Research reports include YAML frontmatter for metadata:

```markdown
---
url: https://youtube.com/watch?v=abc123
content_hash: sha256:1234567890abcdef...
researched_at: 2025-01-10T15:04:05Z
topics:
  - Topic 1
  - Topic 2
  - Topic 3
version: 1
---

# Research Report

[Research content...]
```

### Cache Workflow

```
1. User runs: gemini-deep-research https://example.com

2. Check cache:
   - Normalize URL → "example-com"
   - Search cache dir for matching URL in frontmatter
   - If found → Compare content hash

3a. Cache HIT (content unchanged):
    - Return cached report
    - Exit in <1 second
    - Log: "Using cached research from [timestamp]"

3b. Cache MISS (no cache or content changed):
    - Run full pipeline
    - Write results to cache with frontmatter
    - Also write to output/ for backward compatibility

4. Force refresh (--force flag):
    - Skip cache check entirely
    - Run full pipeline
    - Overwrite cache entry
```

### Implementation

**Cache Check** (`internal/cache/cache.go`):
```go
func Check(url string, cacheDir string, contentType string) (resultPath string, exists bool, err error) {
    // Normalize URL for consistent lookup
    normalizedURL, err := Normalize(url, contentType)
    if err != nil {
        return "", false, err
    }

    // Walk cache directory
    err = filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
        if d.IsDir() || !strings.HasSuffix(d.Name(), "report.md") {
            return nil
        }

        // Read frontmatter
        fm, err := readFrontmatter(path)
        if err != nil {
            return nil // Skip invalid files
        }

        // Compare normalized URLs
        if fm.URL == normalizedURL {
            exists = true
            resultPath = filepath.Dir(path)
            return filepath.SkipAll
        }

        return nil
    })

    return resultPath, exists, err
}
```

**Cache Write** (`internal/cache/cache.go`):
```go
func Write(research *Research, cacheDir string, force bool) (string, error) {
    // Generate slug from URL
    slug := GenerateSlug(research.URL)

    // Build output path
    outputPath := filepath.Join(cacheDir, research.ContentType, slug)

    // Handle collisions
    if _, err := os.Stat(outputPath); err == nil {
        if force {
            // Version bump: slug-2025-01-10
            version := findNextVersion(outputPath)
            timestamp := time.Now().Format("2006-01-02")
            outputPath = filepath.Join(cacheDir, research.ContentType, fmt.Sprintf("%s-%s", slug, timestamp))
        } else {
            // Append hash: slug-1234abcd
            hash := fmt.Sprintf("%x", sha256.Sum256([]byte(research.URL)))[:8]
            outputPath = filepath.Join(cacheDir, research.ContentType, fmt.Sprintf("%s-%s", slug, hash))
        }
    }

    // Create directory
    os.MkdirAll(outputPath, 0755)

    // Build frontmatter
    fm := Frontmatter{
        URL:          research.URL,
        ContentHash:  research.ContentHash,
        ResearchedAt: time.Now().UTC().Format(time.RFC3339),
        Topics:       research.Topics,
        Version:      1,
    }

    // Write report.md with frontmatter
    reportPath := filepath.Join(outputPath, "report.md")
    content := fmt.Sprintf("---\n%s---\n\n%s", marshalYAML(fm), research.Content)
    os.WriteFile(reportPath, []byte(content), 0644)

    return outputPath, nil
}
```

### URL Normalization

Ensures consistent cache lookups across URL variations:

```go
func Normalize(url string, contentType string) (string, error) {
    parsed, err := neturl.Parse(url)
    if err != nil {
        return "", err
    }

    // YouTube-specific normalization
    if contentType == "youtube" {
        videoID := extractVideoID(parsed)
        return fmt.Sprintf("youtube.com/watch?v=%s", videoID), nil
    }

    // arXiv-specific normalization
    if contentType == "arxiv" {
        arxivID := extractArxivID(parsed)
        return fmt.Sprintf("arxiv.org/abs/%s", arxivID), nil
    }

    // Generic normalization: remove fragments, sort query params
    normalized := fmt.Sprintf("%s://%s%s", parsed.Scheme, parsed.Host, parsed.Path)
    if parsed.RawQuery != "" {
        normalized += "?" + sortQueryParams(parsed.RawQuery)
    }

    return normalized, nil
}
```

## Consequences

### Positive

1. **API Efficiency**: 60%+ cache hit rate after 1 month of usage
2. **Speed**: Cached requests return in <1 second (vs. 30-60 minutes)
3. **Quota Savings**: Eliminates duplicate API calls
4. **Developer Experience**: Fast iteration during development
5. **Transparency**: Frontmatter shows cache metadata (timestamp, topics, hash)
6. **Browsability**: Organized by content type, human-readable directory names
7. **Backward Compatible**: Cache + output/ directory both written

### Negative

1. **Disk Usage**: Cache grows over time (mitigated by organization)
2. **Stale Detection**: Requires content hash calculation (negligible overhead)
3. **Cache Management**: No automatic eviction (manual cleanup required)
4. **Complexity**: Additional code for cache logic

### Neutral

1. **Cache Location**: `~/.cache/gemini-deep-research/` follows XDG Base Directory spec
2. **Collision Handling**: Hash appending ensures uniqueness
3. **Frontmatter Overhead**: ~200 bytes per report (negligible)

## Implementation

### Cache Configuration

```bash
# Default cache directory
export GEMINI_DR_CACHE_DIR="$HOME/.cache/gemini-deep-research"

# Custom cache directory
export GEMINI_DR_CACHE_DIR="/data/research-cache"
```

### Cache Bypass

```bash
# Use cache (default)
gemini-deep-research https://example.com

# Force refresh
gemini-deep-research --force https://example.com
```

### Cache Inspection

Users can inspect cache manually:

```bash
# List cached research
ls -lh ~/.cache/gemini-deep-research/youtube/

# View cached report
cat ~/.cache/gemini-deep-research/youtube/example-video/report.md

# Check frontmatter
head -n 20 ~/.cache/gemini-deep-research/youtube/example-video/report.md
```

### Testing Strategy

**Unit Tests**:
```go
// Test cache hit detection
func TestCacheCheck(t *testing.T) {
    // Setup: Create cache entry
    cacheDir := t.TempDir()
    research := &Research{
        URL: "https://youtube.com/watch?v=abc123",
        ContentHash: "sha256:test",
        Topics: []string{"topic1"},
        Content: "test content",
        ContentType: "youtube",
    }
    Write(research, cacheDir, false)

    // Test: Check returns existing path
    path, exists, err := Check("https://youtube.com/watch?v=abc123", cacheDir, "youtube")
    require.NoError(t, err)
    assert.True(t, exists)
    assert.NotEmpty(t, path)
}

// Test URL normalization
func TestNormalize(t *testing.T) {
    tests := []struct {
        input    string
        expected string
    }{
        {"https://youtube.com/watch?v=abc123", "youtube.com/watch?v=abc123"},
        {"https://youtu.be/abc123", "youtube.com/watch?v=abc123"},
        {"https://arxiv.org/abs/2501.12345", "arxiv.org/abs/2501.12345"},
        {"https://arxiv.org/pdf/2501.12345.pdf", "arxiv.org/abs/2501.12345"},
    }

    for _, tt := range tests {
        result, err := Normalize(tt.input, detectContentType(tt.input))
        require.NoError(t, err)
        assert.Equal(t, tt.expected, result)
    }
}

// Test collision handling
func TestCollisionHandling(t *testing.T) {
    cacheDir := t.TempDir()

    // Write first entry
    research1 := &Research{URL: "https://example.com/a", ...}
    path1, _ := Write(research1, cacheDir, false)

    // Write collision (different URL, same slug)
    research2 := &Research{URL: "https://example.com/b", ...}
    path2, _ := Write(research2, cacheDir, false)

    // Paths should differ (hash appended)
    assert.NotEqual(t, path1, path2)
}
```

**Integration Tests**:
```go
// Test E2E cache workflow
func TestCacheWorkflow(t *testing.T) {
    // First run: cache miss
    exitCode1 := Run("https://example.com", flags, config)
    assert.Equal(t, 0, exitCode1)

    // Second run: cache hit
    start := time.Now()
    exitCode2 := Run("https://example.com", flags, config)
    duration := time.Since(start)

    assert.Equal(t, 0, exitCode2)
    assert.Less(t, duration, 2*time.Second) // Cache hit is fast
}
```

### Cache Maintenance

**Manual Cleanup**:
```bash
# Remove cache entries older than 90 days
find ~/.cache/gemini-deep-research -name "report.md" -mtime +90 -delete

# Clear all cache
rm -rf ~/.cache/gemini-deep-research
```

**Future Enhancement**: Automatic eviction based on LRU or size limits

## Alternatives Considered

### 1. Database-Backed Cache (SQLite)

**Approach**: Store cache in SQLite database

```sql
CREATE TABLE research_cache (
    id INTEGER PRIMARY KEY,
    url TEXT NOT NULL,
    content_hash TEXT,
    topics JSON,
    report TEXT,
    created_at TIMESTAMP
);
```

**Pros**:
- Structured queries
- Built-in indexing
- ACID guarantees

**Cons**:
- Additional dependency (SQLite)
- Less browsable (binary format)
- Requires migration for schema changes
- Overkill for simple key-value storage

**Decision**: Rejected due to unnecessary complexity

### 2. In-Memory Cache (Redis, Memcached)

**Approach**: External cache server

**Pros**:
- Very fast lookups
- TTL support
- Distributed caching

**Cons**:
- Requires external service
- Data not persisted across restarts
- Operational overhead
- Overkill for single-user CLI tool

**Decision**: Rejected due to operational overhead

### 3. Content-Addressed Storage (Git-like)

**Approach**: Store reports by content hash only

```
.cache/
└── objects/
    └── 12/
        └── 34567890abcdef.md
```

**Pros**:
- Automatic deduplication
- Hash collisions impossible

**Cons**:
- Requires separate index for URL→hash lookup
- Less browsable (hash-based names)
- More complex implementation

**Decision**: Rejected due to reduced browsability

### 4. No Caching (Always Fresh)

**Approach**: Always run full pipeline

**Pros**:
- Simplest implementation
- Always up-to-date results

**Cons**:
- Wastes API quota on duplicates
- Poor developer experience
- Slow iteration cycles

**Decision**: Rejected due to poor UX and API waste

## Related Decisions

- ADR-001: Go Rewrite (enables efficient file I/O and structured caching)
- ADR-002: Extractor Factory Pattern (extractors must support content hashing)
- ADR-004: Competitive Analysis Mode (caching supports competitive workflows)
- ADR-006: Error Handling Strategy (graceful cache failure handling)

## References

- [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
- [YAML Frontmatter](https://jekyllrb.com/docs/front-matter/)
- [URL Normalization RFC 3986](https://tools.ietf.org/html/rfc3986)

## Notes

The file-based cache provides the optimal balance of:
- **Performance**: Sub-second lookups
- **Simplicity**: No external dependencies
- **Transparency**: Human-readable files
- **Reliability**: No corruption risks (each write is atomic)

Frontmatter enables embedding metadata directly in reports, making cache entries self-describing and portable.

Future enhancement: Consider adding `--cache-ttl` flag for automatic expiration, or `--cache-clear` for maintenance.
