# ADR-002: Extractor Factory Pattern

**Status**: Accepted
**Date**: 2024-12-20 (backfilled 2025-02-11)
**Deciders**: Engineering Team
**Tags**: architecture, design-pattern, extensibility

## Context

The tool needs to extract content from multiple source types:
- YouTube videos (transcript via yt-dlp)
- arXiv papers (PDF or API)
- HuggingFace papers (web scraping)
- Generic web articles (readability)

Each content type requires different extraction logic, dependencies, and error handling. As the tool evolves, new content types may be added (podcasts, GitHub repos, etc.).

### Requirements

1. **Unified Interface**: All extractors must return consistent `Content` structure
2. **Type-Specific Logic**: Each content type has unique extraction requirements
3. **Extensibility**: Easy to add new content types without modifying core pipeline
4. **Error Isolation**: Extraction failures should not affect detector or other extractors
5. **Testability**: Each extractor must be independently testable with mocks

## Decision

Implement the **Factory Pattern** for content extractors with the following design:

### Core Interface

```go
// Extractor is the common interface for all content extractors
type Extractor interface {
    Extract(ctx context.Context, url string) (*Content, error)
    Name() string
    ContentType() detector.ContentType
}

// Content is the unified output structure
type Content struct {
    Raw      string                 // Extracted text content
    Metadata map[string]interface{} // Content-specific metadata
}
```

### Factory Implementation

```go
// ExtractorFactory creates and manages extractors
type ExtractorFactory struct {
    youtube     *youtubeExtractorWrapper
    arxiv       *arxivExtractorWrapper
    huggingface *huggingfaceExtractorWrapper
    generic     *genericExtractorWrapper
    httpClient  *http.Client
}

// GetExtractor returns appropriate extractor for content type
func (f *ExtractorFactory) GetExtractor(contentType detector.ContentType) (Extractor, error) {
    switch contentType {
    case detector.ContentTypeVideo:
        return f.youtube, nil
    case detector.ContentTypeArxiv:
        return f.arxiv, nil
    case detector.ContentTypeHuggingFace:
        return f.huggingface, nil
    case detector.ContentTypeArticle:
        return f.generic, nil
    default:
        return nil, fmt.Errorf("unsupported content type: %s", contentType)
    }
}
```

### Wrapper Pattern

Each concrete extractor is wrapped to implement the unified interface:

```go
// youtubeExtractorWrapper wraps youtube.YouTubeExtractor
type youtubeExtractorWrapper struct {
    inner *youtube.YouTubeExtractor
}

func (w *youtubeExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
    content, err := w.inner.Extract(ctx, url)
    if err != nil {
        return nil, err
    }

    return &Content{
        Raw: content.Transcript,
        Metadata: map[string]interface{}{
            "url":      content.URL,
            "video_id": content.VideoID,
            "length":   content.Length,
        },
    }, nil
}
```

### Package Structure

```
extractors/
├── extractor.go         # Interface definition
├── factory.go           # Factory implementation
├── orchestrator.go      # Multi-extractor orchestration
├── youtube/
│   ├── extractor.go     # YouTube-specific logic
│   ├── vtt.go           # VTT subtitle parser
│   └── errors.go        # YouTube-specific errors
├── arxiv/
│   ├── extractor.go     # arXiv-specific logic
│   ├── api.go           # arXiv API client
│   ├── pdf.go           # PDF extraction (fallback)
│   └── errors.go        # arXiv-specific errors
└── web/
    ├── generic.go       # Generic web extractor
    ├── huggingface.go   # HuggingFace-specific logic
    ├── readability.go   # Readability library wrapper
    └── errors.go        # Web extraction errors
```

## Consequences

### Positive

1. **Extensibility**: New extractors added without modifying factory (Open/Closed Principle)
2. **Testability**: Each extractor independently testable with mocks
3. **Separation of Concerns**: Content-type logic isolated in subpackages
4. **Unified Interface**: Pipeline code agnostic to extractor implementation
5. **Error Isolation**: Extraction failures don't affect other extractors
6. **Reusability**: Extractors can be used independently outside pipeline
7. **Type Safety**: Compile-time validation of extractor interface compliance

### Negative

1. **Boilerplate**: Wrapper types add code overhead
2. **Indirection**: Extra layer between pipeline and concrete extractors
3. **Complexity**: Factory pattern adds conceptual overhead for simple use cases

### Neutral

1. **Learning Curve**: Factory pattern is well-known, but requires understanding
2. **Maintenance**: Each extractor maintains its own error types and logic

## Implementation

### Adding New Extractor

To add a new content type (e.g., podcast):

1. **Create package**: `extractors/podcast/`
2. **Implement extractor**:
   ```go
   type PodcastExtractor struct {
       httpClient *http.Client
   }

   func (e *PodcastExtractor) Extract(ctx context.Context, url string) (*PodcastContent, error) {
       // Podcast-specific extraction logic
   }
   ```

3. **Create wrapper**:
   ```go
   type podcastExtractorWrapper struct {
       inner *podcast.PodcastExtractor
   }

   func (w *podcastExtractorWrapper) Extract(ctx context.Context, url string) (*Content, error) {
       content, err := w.inner.Extract(ctx, url)
       if err != nil {
           return nil, err
       }

       return &Content{
           Raw: content.Transcript,
           Metadata: map[string]interface{}{
               "title": content.Title,
               "duration": content.Duration,
           },
       }, nil
   }
   ```

4. **Register in factory**:
   ```go
   func (f *ExtractorFactory) GetExtractor(contentType detector.ContentType) (Extractor, error) {
       switch contentType {
       // ... existing cases ...
       case detector.ContentTypePodcast:
           return f.podcast, nil
       // ...
       }
   }
   ```

5. **Add detector pattern**:
   ```go
   // detector/patterns.go
   patterns := []pattern{
       // ... existing patterns ...
       {hostname: "podcasts.google.com", contentType: ContentTypePodcast},
   }
   ```

### Testing Strategy

**Unit Tests**:
```go
// Test factory returns correct extractor
func TestFactoryGetExtractor(t *testing.T) {
    factory, _ := NewExtractorFactory()

    extractor, err := factory.GetExtractor(detector.ContentTypeVideo)
    require.NoError(t, err)
    assert.Equal(t, "YouTube", extractor.Name())
}

// Test extractor implements interface
func TestYouTubeExtractorInterface(t *testing.T) {
    var _ Extractor = &youtubeExtractorWrapper{}
}

// Test wrapper transforms content correctly
func TestWrapperTransform(t *testing.T) {
    mockExtractor := &mockYouTubeExtractor{
        content: &youtube.Content{
            Transcript: "test transcript",
            VideoID: "abc123",
        },
    }

    wrapper := &youtubeExtractorWrapper{inner: mockExtractor}
    content, err := wrapper.Extract(context.Background(), "https://youtube.com/watch?v=abc123")

    require.NoError(t, err)
    assert.Equal(t, "test transcript", content.Raw)
    assert.Equal(t, "abc123", content.Metadata["video_id"])
}
```

**Integration Tests**:
```go
// Test E2E extraction for each content type
func TestExtractorE2E(t *testing.T) {
    tests := []struct {
        name string
        url  string
        contentType detector.ContentType
    }{
        {"YouTube", "https://youtube.com/watch?v=TEST", detector.ContentTypeVideo},
        {"arXiv", "https://arxiv.org/abs/2501.12345", detector.ContentTypeArxiv},
        // ...
    }

    factory, _ := NewExtractorFactory()

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            extractor, err := factory.GetExtractor(tt.contentType)
            require.NoError(t, err)

            content, err := extractor.Extract(context.Background(), tt.url)
            require.NoError(t, err)
            assert.NotEmpty(t, content.Raw)
        })
    }
}
```

## Alternatives Considered

### 1. Strategy Pattern (No Factory)

**Approach**: Direct instantiation of extractors in pipeline

```go
var extractor Extractor
switch contentType {
case detector.ContentTypeVideo:
    extractor = NewYouTubeExtractor()
case detector.ContentTypeArxiv:
    extractor = NewArxivExtractor()
}
```

**Pros**:
- Simpler, less indirection
- No factory overhead

**Cons**:
- Pipeline tightly coupled to extractor implementations
- Harder to test (no dependency injection)
- Extractor initialization logic in pipeline

**Decision**: Rejected due to tight coupling

### 2. Plugin System (Dynamic Loading)

**Approach**: Load extractors dynamically via Go plugins

```go
plugin, _ := plugin.Open("extractors/youtube.so")
extractor, _ := plugin.Lookup("NewExtractor")
```

**Pros**:
- Extremely extensible
- No recompilation for new extractors

**Cons**:
- Complex build process
- Platform-dependent (.so vs .dll)
- No compile-time type safety
- Difficult to debug
- Overkill for current requirements

**Decision**: Rejected due to complexity

### 3. Interface Registry Pattern

**Approach**: Extractors self-register at init time

```go
// youtube/extractor.go
func init() {
    extractors.Register(detector.ContentTypeVideo, NewYouTubeExtractor())
}

// extractors/registry.go
var registry = make(map[detector.ContentType]Extractor)

func Register(ct detector.ContentType, e Extractor) {
    registry[ct] = e
}
```

**Pros**:
- Decentralized registration
- Easy to add new extractors

**Cons**:
- Global state (mutable registry)
- Difficult to test (shared state)
- Init order dependencies
- No factory lifecycle control

**Decision**: Rejected due to global state concerns

### 4. Functional Options Pattern

**Approach**: Factory with functional options

```go
factory := NewExtractorFactory(
    WithYouTube(ytConfig),
    WithArxiv(arxivConfig),
)
```

**Pros**:
- Flexible configuration
- Optional extractors

**Cons**:
- More complex API
- Unnecessary for current requirements

**Decision**: Rejected as premature optimization

## Related Decisions

- ADR-001: Go Rewrite (establishes modular architecture)
- ADR-003: Caching Strategy (extractors must support content hashing)
- ADR-004: Competitive Analysis Mode (extractors support discovery pipeline)
- ADR-006: Error Handling Strategy (extractors must return structured errors)

## References

- [Factory Pattern](https://refactoring.guru/design-patterns/factory-method)
- [Wrapper Pattern](https://refactoring.guru/design-patterns/decorator)
- [Effective Go - Interfaces](https://golang.org/doc/effective_go#interfaces)
- [Go Proverbs - Accept interfaces, return structs](https://go-proverbs.github.io/)

## Notes

The factory pattern provides the right balance of extensibility and simplicity for this use case. While more complex than direct instantiation, the benefits of decoupling and testability outweigh the overhead.

The wrapper pattern bridges the gap between content-specific extractors and the unified interface, allowing each extractor to maintain its domain-specific API while conforming to the common interface.

Future enhancement: Consider interface-based configuration for extractors if customization requirements grow beyond current needs.
