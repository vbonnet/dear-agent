# Architecture Documentation

## Overview

Gemini Deep Research is a Go-based tool that orchestrates an end-to-end research pipeline. It extracts content from various web sources, analyzes them with Gemini to identify topics, and runs deep research using the Gemini Deep Research API.

## High-Level Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        CLI Entry Point                       │
│                         (main.go)                            │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│                     Command Handler                          │
│                        (cmd/)                                │
│  • Flag validation                                           │
│  • Configuration loading                                     │
│  • Pipeline orchestration                                    │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
        ┌───────────────────┴───────────────────┐
        │         Pipeline Execution            │
        │      (cmd/run.go:executePipeline)     │
        └───────────────────┬───────────────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
   ┌────────┐         ┌──────────┐       ┌──────────┐
   │Detector│         │Extractor │       │  Gemini  │
   │        │         │ Factory  │       │ Analyzer │
   └────────┘         └──────────┘       └──────────┘
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
        ▼                   ▼                   ▼
   ┌────────┐         ┌──────────┐       ┌──────────┐
   │YouTube │         │  ArXiv   │       │   Web    │
   │Extract │         │ Extract  │       │ Extract  │
   └────────┘         └──────────┘       └──────────┘
        │                   │                   │
        └───────────────────┴───────────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │    Research   │
                    │     Client    │
                    │ (Deep Research│
                    │      API)     │
                    └───────────────┘
                            │
                            ▼
                    ┌───────────────┐
                    │    Output     │
                    │    Writer     │
                    └───────────────┘
```

## Pipeline Flow

### Step 1: Content Type Detection

**Package**: `detector`

**Files**:
- `detector/detector.go` - Main detection logic
- `detector/types.go` - Content type definitions
- `detector/patterns.go` - URL pattern matching

**Flow**:
1. Parse URL
2. Check for manual override (`--type` flag)
3. Match URL against known patterns (youtube.com, arxiv.org, etc.)
4. Return detected content type or fallback to "article"

**Output**: `detector.ContentType` enum

### Step 2: Content Extraction

**Package**: `extractors`

**Files**:
- `extractors/extractor.go` - Common extractor interface
- `extractors/factory.go` - Extractor factory
- `extractors/youtube/` - YouTube video transcript extraction
- `extractors/arxiv/` - arXiv paper extraction (PDF + API)
- `extractors/web/` - Web content extraction (readability)

**Flow**:
1. Factory creates appropriate extractor based on content type
2. Extractor fetches content from URL
3. Extractor returns unified `Content` struct with raw text + metadata

**Output**: `extractors.Content` struct

### Step 3: Topic Analysis

**Package**: `gemini`

**Files**:
- `gemini/analyzer.go` - Topic analysis logic
- `gemini/json.go` - JSON response parsing
- `gemini/errors.go` - Error types

**Flow**:
1. Build prompt (default or custom)
2. Execute Gemini CLI with content
3. Parse JSON response to extract topics
4. Validate topics (non-empty, reasonable count)

**Output**: `[]string` (list of topics)

### Step 4: Deep Research

**Package**: `research`

**Files**:
- `research/client.go` - HTTP client for Deep Research API
- `research/poll.go` - Status polling logic
- `research/retry.go` - Retry mechanism with exponential backoff
- `research/api_key.go` - API key management
- `research/errors.go` - Error types

**Flow**:
1. Create research client with API key
2. Start research interaction (POST /interactions)
3. Poll status endpoint until completion or timeout
4. Extract final report from response

**Output**: `string` (research report in Markdown)

### Step 5: Output Writing

**Package**: `cmd`

**Files**:
- `cmd/output.go` - Output file writer

**Flow**:
1. Create timestamped directory
2. Write metadata.json (URL, topics, timestamp)
3. Write topics.json (topic list)
4. Write report.md (research report)
5. Write content.txt (extracted content)

**Output**: Directory path

## Package Descriptions

### `main`

**Purpose**: CLI entry point

**Responsibilities**:
- Parse command-line flags
- Display help/version information
- Delegate to `cmd` package

**Key files**:
- `main.go` - Entry point and flag parsing

### `cmd`

**Purpose**: Command execution and pipeline orchestration

**Responsibilities**:
- Validate flags and configuration
- Orchestrate pipeline steps
- Handle errors and exit codes
- Write output files

**Key files**:
- `cmd/run.go` - Main pipeline execution
- `cmd/flags.go` - Flag validation
- `cmd/output.go` - Output file writing

### `config`

**Purpose**: Configuration management

**Responsibilities**:
- Load configuration from environment and flags
- Provide default values
- Validate configuration

**Key files**:
- `config/config.go` - Config struct and validation
- `config/env.go` - Environment variable parsing
- `config/defaults.go` - Default values

### `detector`

**Purpose**: Content type detection

**Responsibilities**:
- Detect content type from URL
- Support manual overrides
- Provide fallback detection

**Key files**:
- `detector/detector.go` - Detection logic
- `detector/types.go` - Content type enum
- `detector/patterns.go` - URL pattern matching

### `extractors`

**Purpose**: Content extraction from various sources

**Responsibilities**:
- Extract text content from URLs
- Handle different content types (video, paper, web)
- Provide unified content interface

**Subpackages**:
- `extractors/youtube` - YouTube video transcript extraction
- `extractors/arxiv` - arXiv paper extraction
- `extractors/web` - Web content extraction (HuggingFace, generic)

**Key files**:
- `extractors/extractor.go` - Common interface
- `extractors/factory.go` - Extractor factory
- `extractors/orchestrator.go` - Multi-extractor orchestration

### `gemini`

**Purpose**: Gemini API integration for topic analysis

**Responsibilities**:
- Call Gemini CLI for topic analysis
- Parse JSON responses
- Handle API errors

**Key files**:
- `gemini/analyzer.go` - Topic analysis
- `gemini/json.go` - JSON parsing
- `gemini/errors.go` - Error handling

### `research`

**Purpose**: Deep Research API client

**Responsibilities**:
- Start research interactions
- Poll for completion
- Handle retries and timeouts
- Extract final reports

**Key files**:
- `research/client.go` - HTTP client
- `research/poll.go` - Status polling
- `research/retry.go` - Retry logic
- `research/api_key.go` - API key management

### `types`

**Purpose**: Shared type definitions

**Responsibilities**:
- Define common types used across packages
- Provide type validation

**Key files**:
- `types/flags.go` - CLI flag types

## Data Flow

### Input Data

```
URL (string)
  ↓
Flags (types.Flags)
  ↓
Config (config.Config)
```

### Processing Data

```
URL → ContentType → Content → Topics → Report
```

### Output Data

```
OutputDirectory/
  ├── metadata.json     # Full pipeline metadata
  ├── topics.json       # Topic list
  ├── report.md         # Final research report
  └── content.txt       # Extracted content
```

## Error Handling

### Error Types

1. **Configuration Errors** (exit code 1)
   - Invalid flags
   - Missing API key
   - Invalid URL

2. **Extraction Errors** (exit code 2)
   - Network failures
   - Content not available
   - Parsing errors

3. **Analysis Errors** (exit code 3)
   - Gemini CLI errors
   - Invalid JSON responses
   - No topics found

4. **Research Errors** (exit code 4)
   - API authentication failures
   - Rate limiting
   - Timeout
   - Research failures

5. **Output Errors** (exit code 5)
   - File system errors
   - Permission errors

6. **Unexpected Errors** (exit code 6)
   - Panic recovery
   - Unknown errors

### Retry Strategy

The research client implements exponential backoff for transient errors:

```
Attempt 1: Immediate
Attempt 2: Wait 1s
Attempt 3: Wait 2s
Attempt 4: Wait 4s
Attempt 5: Wait 8s
```

Retryable errors:
- HTTP 429 (rate limit)
- HTTP 500, 502, 503, 504 (server errors)
- Network timeouts

Non-retryable errors:
- HTTP 401 (authentication)
- HTTP 404 (not found)
- Invalid request format

## Configuration Precedence

Configuration values are resolved in this order (highest to lowest priority):

1. Command-line flags
2. Environment variables
3. Default values

Example:
```
Timeout value = --timeout flag
             OR GEMINI_DR_TIMEOUT env var
             OR 60 (default)
```

## Testing Strategy

### Unit Tests

- Test individual functions in isolation
- Mock external dependencies
- Focus on business logic

**Files**: `*_test.go` in each package

### Integration Tests

- Test full pipeline with real/mocked components
- Test error paths
- Test output generation

**File**: `integration_test.go`

**Run with**: `go test -v ./...`

**Skip with**: `go test -short ./...`

## Dependencies

### External Tools

- **gemini CLI**: Required for topic analysis
- **yt-dlp**: Optional, required for YouTube videos
- **pdftotext**: Optional, for arXiv PDF extraction

### Go Libraries

- `github.com/google/uuid`: UUID generation
- `github.com/go-shiori/go-readability`: Web content extraction
- Standard library: `net/http`, `encoding/json`, `context`, etc.

## Performance Considerations

### Bottlenecks

1. **Content Extraction**: Network-bound (fetching from URLs)
2. **Topic Analysis**: Gemini CLI execution time
3. **Deep Research**: API processing time (can be 30-60 minutes)

### Optimizations

- Streaming output for long-running operations
- Context-based cancellation
- Retry with exponential backoff
- Efficient JSON parsing

### Scalability

Current implementation is single-threaded and processes one URL at a time. For batch processing, consider:

- Worker pool pattern
- Concurrent extraction
- Rate limiting for API calls

## Security Considerations

### API Keys

- Stored in environment variables (not in code)
- Never logged or written to output files
- Transmitted over HTTPS only

### Content Validation

- URL validation before processing
- Content size limits (implicit via timeouts)
- Safe file path handling (sanitization)

### Error Messages

- Avoid exposing sensitive information
- Sanitize user input in error messages

## Future Enhancements

### Planned Features

1. **Batch Processing**: Process multiple URLs in one run
2. **Resume Support**: Resume interrupted research sessions
3. **Custom Extractors**: Plugin system for custom content types
4. **Caching**: Cache extracted content to avoid re-fetching
5. **Web UI**: Optional web interface for easier use

### Potential Improvements

1. **Parallel Extraction**: Extract from multiple sources concurrently
2. **Advanced Filtering**: Filter topics by relevance
3. **Report Formatting**: Multiple output formats (PDF, HTML, etc.)
4. **Analytics**: Track success rates, timing metrics
5. **Incremental Updates**: Update existing research reports

## Maintenance

### Adding New Content Type

1. Create new extractor in `extractors/`
2. Add pattern matching in `detector/patterns.go`
3. Register in `extractors/factory.go`
4. Add tests
5. Update documentation

### Updating Dependencies

```bash
# Update all dependencies
go get -u ./...

# Update specific dependency
go get -u github.com/google/uuid@latest

# Tidy modules
go mod tidy
```

### Code Quality

```bash
# Format code
go fmt ./...

# Lint code
golangci-lint run

# Run tests with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Debugging

### Enable Verbose Logging

The tool logs to stdout/stderr. For debugging:

1. Check detector logs for content type detection
2. Check extractor logs for content extraction
3. Check research client logs for API calls

### Common Issues

1. **Gemini CLI not found**: Check PATH
2. **API key errors**: Verify GEMINI_API_KEY is set
3. **Extraction failures**: Check network connectivity
4. **Timeout**: Increase with `--timeout` flag

### Tracing Execution

Add logging at key pipeline steps:

```go
fmt.Fprintf(cfg.Stdout, "Step X: Action...\n")
```

This provides visibility into pipeline execution without verbose debugging.
