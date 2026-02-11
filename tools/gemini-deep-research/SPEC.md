# Gemini Deep Research - Product Specification

## Document Information

- **Version**: 2.0.0
- **Status**: Active
- **Last Updated**: 2025-02-11
- **Component**: Gemini Deep Research Tool
- **Stakeholders**: AI Tools Users, Research Teams, Product Managers

## Executive Summary

Gemini Deep Research is a command-line tool that automates end-to-end research workflows by extracting content from web sources (YouTube videos, arXiv papers, HuggingFace papers, and web articles), analyzing them with Gemini to identify research topics, and conducting comprehensive research using the Gemini Deep Research API.

### Key Value Propositions

1. **Automated Research Pipeline**: Transform any URL into comprehensive research reports
2. **Multi-Source Support**: Handle diverse content types with specialized extractors
3. **Competitive Analysis**: Automated competitor analysis with gap analysis reports
4. **Caching & Performance**: Intelligent caching system prevents duplicate research
5. **Template-Driven**: Customizable prompts for domain-specific research

## Problem Statement

### Current Pain Points

1. **Manual Research is Time-Consuming**: Teams spend hours manually researching topics, extracting content, and synthesizing information
2. **Competitive Intelligence Gaps**: Understanding competitor capabilities requires manual analysis of multiple sources
3. **Inconsistent Research Quality**: Ad-hoc research processes produce inconsistent results
4. **Content Extraction Challenges**: Different content types (videos, papers, web pages) require different extraction methods
5. **No Research Reusability**: Repeated research on the same topics wastes API quota and time

### Target Users

- **Product Managers**: Conducting competitive analysis and market research
- **Researchers**: Analyzing academic papers and technical content
- **Engineering Teams**: Researching implementation approaches and technical decisions
- **Content Teams**: Extracting insights from videos and articles

## Product Goals

### Primary Goals

1. Automate content extraction from multiple source types (YouTube, arXiv, HuggingFace, web)
2. Generate comprehensive research reports using Gemini Deep Research API
3. Support competitive analysis workflows with gap analysis
4. Provide intelligent caching to avoid duplicate research
5. Enable customization through template-based prompts

### Success Metrics

- **Research Quality**: 90%+ user satisfaction with report comprehensiveness
- **Time Savings**: 80% reduction in manual research time
- **Cache Hit Rate**: 60%+ of requests served from cache
- **Extraction Success Rate**: 95%+ successful content extraction
- **API Efficiency**: < 5% duplicate API calls due to caching

## Functional Requirements

### FR-1: Content Type Detection

**Priority**: P0 (Critical)

**Description**: Automatically detect content type from URL

**Acceptance Criteria**:
- Detect YouTube videos from youtube.com and youtu.be URLs
- Detect arXiv papers from arxiv.org URLs
- Detect HuggingFace papers from huggingface.co/papers URLs
- Fallback to generic web article for unknown URLs
- Support manual override via `--type` flag
- Log detected type and source hostname

**Test Cases**:
- TC-1.1: YouTube URL → ContentTypeVideo
- TC-1.2: arXiv URL → ContentTypeArxiv
- TC-1.3: HuggingFace URL → ContentTypeHuggingFace
- TC-1.4: Generic URL → ContentTypeArticle
- TC-1.5: `--type video` override → ContentTypeVideo

### FR-2: Content Extraction

**Priority**: P0 (Critical)

**Description**: Extract text content from detected content type

**Acceptance Criteria**:
- Extract video transcripts using yt-dlp for YouTube
- Extract paper content from arXiv (PDF or API)
- Extract paper content from HuggingFace
- Extract readable text from generic web pages using go-readability
- Return unified Content struct with raw text and metadata
- Handle extraction errors gracefully with clear error messages

**Test Cases**:
- TC-2.1: Extract YouTube transcript with subtitles
- TC-2.2: Extract arXiv paper via API
- TC-2.3: Extract arXiv paper from PDF (fallback)
- TC-2.4: Extract HuggingFace paper metadata and abstract
- TC-2.5: Extract web article with readability
- TC-2.6: Handle extraction failure (no subtitles, blocked access)

### FR-3: Topic Analysis

**Priority**: P0 (Critical)

**Description**: Analyze extracted content with Gemini to identify research topics

**Acceptance Criteria**:
- Execute Gemini CLI with content
- Parse JSON response to extract topic list
- Support custom analyze prompts via `--analyze-prompt`
- Support @file syntax for loading prompts from files
- Validate topics are non-empty and reasonable count (1-20)
- Return list of topic strings

**Test Cases**:
- TC-3.1: Default prompt generates 3-5 topics
- TC-3.2: Custom prompt generates domain-specific topics
- TC-3.3: @file syntax loads prompt from file
- TC-3.4: Invalid JSON response returns error
- TC-3.5: Empty topics list returns error

### FR-4: Deep Research

**Priority**: P0 (Critical)

**Description**: Run comprehensive research using Gemini Deep Research API

**Acceptance Criteria**:
- Create research client with API key from environment
- Start research interaction via POST /interactions
- Poll status endpoint with configurable interval
- Handle timeout after configurable duration (default 60 minutes)
- Retry transient errors with exponential backoff
- Extract final report in Markdown format
- Log progress during polling

**Test Cases**:
- TC-4.1: Successful research returns Markdown report
- TC-4.2: Timeout after configured duration
- TC-4.3: Retry on HTTP 429, 500, 502, 503, 504
- TC-4.4: Fail fast on HTTP 401, 404
- TC-4.5: Poll status every N seconds

### FR-5: Competitive Analysis Mode

**Priority**: P1 (High)

**Description**: Support competitive analysis workflows with URL discovery and gap analysis

**Acceptance Criteria**:
- Auto-detect competitive mode from query keywords (vs, compare, competitor)
- Support explicit `--mode competitive` flag
- Discover competitor URLs using Google Custom Search (Stage 0)
- Extract competitor name from query (e.g., "GitHub Copilot" from "GitHub Copilot vs Cursor")
- Use competitive templates for analysis and research prompts
- Generate gap analysis reports with prioritized recommendations
- Create competitive-summary.md with executive overview
- Include mode, competitor, and source_query in metadata.json

**Test Cases**:
- TC-5.1: Query "X vs Y" → competitive mode detected
- TC-5.2: Discovery returns 5 URLs by default
- TC-5.3: `--discovery-limit 10` returns 10 URLs
- TC-5.4: `--no-discovery` skips URL discovery
- TC-5.5: Competitor name extracted correctly
- TC-5.6: Gap analysis template used for research
- TC-5.7: competitive-summary.md created in output

### FR-6: Intelligent Caching

**Priority**: P1 (High)

**Description**: Cache research results to avoid duplicate API calls

**Acceptance Criteria**:
- Generate cache key from normalized URL and content type
- Check cache before starting research (unless `--force`)
- Store research results with content hash for validation
- Return cached results if content unchanged
- Write new results to cache after successful research
- Cache directory configurable via `GEMINI_DR_CACHE_DIR`

**Test Cases**:
- TC-6.1: First request creates cache entry
- TC-6.2: Second request returns cached result
- TC-6.3: `--force` bypasses cache
- TC-6.4: Changed content invalidates cache
- TC-6.5: Cache directory created if missing

### FR-7: Custom Prompts

**Priority**: P1 (High)

**Description**: Support custom prompts for each pipeline stage

**Acceptance Criteria**:
- Support `--extract-prompt`, `--analyze-prompt`, `--research-prompt` flags
- Support inline text prompts
- Support @file syntax to load prompts from files (e.g., `@prompts/security.txt`)
- Support template variables: {url}, {topics}, {content_type}
- Validate template variables and fail fast on unknown variables
- Load prompts in ConfigParser → FileResolver → VariableSubstitutor pipeline

**Test Cases**:
- TC-7.1: Inline prompt used for analysis
- TC-7.2: @file syntax loads prompt from file
- TC-7.3: Template variable {url} substituted correctly
- TC-7.4: Template variable {topics} substituted after analysis
- TC-7.5: Unknown variable {foo} fails with helpful error
- TC-7.6: File not found error for invalid @file path

### FR-8: Output Generation

**Priority**: P0 (Critical)

**Description**: Write research results to structured output directory

**Acceptance Criteria**:
- Create timestamped output directory (YYYYMMDD-HHMMSS-{slug})
- Write metadata.json with URL, topics, timestamp, mode
- Write topics.json with topic list
- Write report.md with research report
- Write content.txt with extracted content
- Write competitive-summary.md for competitive mode
- Return output directory path

**Test Cases**:
- TC-8.1: General mode creates 4 files (metadata, topics, report, content)
- TC-8.2: Competitive mode creates 5 files (adds competitive-summary)
- TC-8.3: Metadata includes mode-specific fields
- TC-8.4: Directory name includes timestamp and slug
- TC-8.5: Files written with correct encoding (UTF-8)

### FR-9: Configuration Management

**Priority**: P0 (Critical)

**Description**: Load configuration from environment variables and CLI flags

**Acceptance Criteria**:
- Load GEMINI_API_KEY from environment (required)
- Load GOOGLE_CLOUD_PROJECT from environment (optional)
- Load GEMINI_DR_OUTPUT_DIR from environment (default: ./output)
- Load GEMINI_DR_TIMEOUT from environment (default: 60 minutes)
- CLI flags override environment variables
- Validate configuration before pipeline execution
- Provide helpful error messages for missing configuration

**Test Cases**:
- TC-9.1: Missing GEMINI_API_KEY returns clear error
- TC-9.2: CLI --timeout overrides environment variable
- TC-9.3: Default values used when not specified
- TC-9.4: Configuration logged at pipeline start
- TC-9.5: Invalid timeout value returns error

### FR-10: Error Handling

**Priority**: P0 (Critical)

**Description**: Provide clear error messages and exit codes

**Acceptance Criteria**:
- Exit code 0 for success
- Exit code 1 for invalid arguments/configuration
- Exit code 2 for content extraction failures
- Exit code 3 for topic analysis failures
- Exit code 4 for deep research failures
- Exit code 5 for output writing failures
- Exit code 6 for unexpected errors
- Error messages include actionable troubleshooting steps
- Log stack traces for debugging (without exposing secrets)

**Test Cases**:
- TC-10.1: Invalid URL → exit code 1 with examples
- TC-10.2: Extraction failure → exit code 2 with cause
- TC-10.3: Gemini CLI error → exit code 3 with command
- TC-10.4: API timeout → exit code 4 with timeout config
- TC-10.5: File write error → exit code 5 with permissions check

## Non-Functional Requirements

### NFR-1: Performance

**Priority**: P0

**Requirements**:
- Content extraction: < 30 seconds for 95% of requests
- Topic analysis: < 60 seconds for 95% of requests
- Deep research: < 60 minutes (configurable)
- Cache lookup: < 1 second
- Total pipeline: < 65 minutes (worst case, no cache)

### NFR-2: Reliability

**Priority**: P0

**Requirements**:
- Extraction success rate: > 95% for accessible URLs
- API retry success rate: > 90% for transient failures
- Cache consistency: 100% (no corrupted cache entries)
- Graceful degradation: Continue on cache failures

### NFR-3: Usability

**Priority**: P1

**Requirements**:
- Help text covers all common use cases
- Error messages include actionable next steps
- Configuration examples provided for all flags
- Progress logging during long operations
- Clear output structure with README-like metadata

### NFR-4: Security

**Priority**: P0

**Requirements**:
- API keys never logged or written to output files
- HTTPS-only for API calls
- URL validation before processing
- Safe file path handling (prevent directory traversal)
- Content size limits via timeouts

### NFR-5: Maintainability

**Priority**: P1

**Requirements**:
- Comprehensive unit tests (> 80% coverage)
- Integration tests for E2E pipeline
- Clear package boundaries and interfaces
- Minimal external dependencies
- Backward-compatible configuration changes

### NFR-6: Extensibility

**Priority**: P2

**Requirements**:
- Plugin system for custom content extractors
- Template system for custom prompts
- Configurable output formats
- Hooks for custom processing steps

## User Stories

### US-1: Basic Research Workflow

**As a** researcher
**I want to** analyze a YouTube video about AI
**So that** I can quickly understand key topics and get a comprehensive research report

**Acceptance Criteria**:
- Run `gemini-deep-research https://youtube.com/watch?v=VIDEO_ID`
- Tool extracts video transcript automatically
- Gemini identifies 3-5 key topics
- Deep Research generates comprehensive report
- Results saved to timestamped directory

### US-2: Competitive Analysis

**As a** product manager
**I want to** analyze a competitor's product page
**So that** I can identify gaps in our product

**Acceptance Criteria**:
- Run `gemini-deep-research "Competitor X vs Our Product" --mode competitive`
- Tool discovers competitor URLs automatically
- Extracts competitor capabilities
- Generates gap analysis with prioritized recommendations
- Creates executive summary for stakeholders

### US-3: Custom Research Focus

**As a** security researcher
**I want to** focus analysis on security vulnerabilities
**So that** I can get security-specific insights

**Acceptance Criteria**:
- Create custom prompt: `@prompts/security.txt`
- Run with `--analyze-prompt @prompts/security.txt`
- Tool uses custom prompt for topic analysis
- Topics focus on security aspects
- Research report emphasizes security topics

### US-4: Batch Competitor Analysis

**As a** product team lead
**I want to** analyze multiple competitors monthly
**So that** I can track competitive landscape changes

**Acceptance Criteria**:
- Script loops through competitor list
- Each competitor analyzed with `--mode competitive`
- Results cached for fast re-analysis
- Use `--force` to refresh monthly
- Output organized by timestamp

### US-5: Research Reuse

**As a** team member
**I want to** reuse existing research results
**So that** I don't waste API quota on duplicate research

**Acceptance Criteria**:
- First run creates cache entry
- Second run returns cached result in < 5 seconds
- Cache respects content changes (content hash validation)
- Use `--force` to refresh stale cache
- Clear messaging when cache is used

## User Interface Specification

### Command-Line Interface

#### Primary Command

```bash
gemini-deep-research [FLAGS] <URL>
```

#### Flags

**Per-Stage Prompts**:
- `--extract-prompt <text|@file>`: Custom extraction prompt
- `--analyze-prompt <text|@file>`: Custom analysis prompt
- `--research-prompt <text|@file>`: Custom research prompt

**Content Type**:
- `--type <video|article|arxiv|huggingface>`: Override content type detection

**Analysis Mode**:
- `--mode <general|competitive>`: Analysis mode (auto-detected by default)
- `--no-discovery`: Skip URL discovery in competitive mode
- `--discovery-limit <n>`: Max URLs to discover (default: 5, max: 20)

**Output Configuration**:
- `--output-dir <path>`: Output directory (default: ./output)
- `--timeout <minutes>`: Research timeout (default: 60)
- `--project <id>`: GCP project ID
- `--force`: Bypass cache

**Utility**:
- `--help, -h`: Show help
- `--version, -v`: Show version

#### Usage Examples

**Basic Usage**:
```bash
# YouTube video
gemini-deep-research https://youtube.com/watch?v=VIDEO_ID

# arXiv paper
gemini-deep-research https://arxiv.org/abs/2601.20802

# Web article
gemini-deep-research https://example.com/article
```

**Competitive Analysis**:
```bash
# Discover competitor URLs
gemini-deep-research "GitHub Copilot vs Cursor" --mode competitive

# Analyze specific URL
gemini-deep-research https://github.com/features/copilot --mode competitive --no-discovery
```

**Custom Prompts**:
```bash
# Inline prompt
gemini-deep-research --analyze-prompt "Focus on security" https://example.com

# Prompt from file
gemini-deep-research --analyze-prompt @prompts/security.txt https://example.com
```

**Configuration**:
```bash
# Custom output directory
gemini-deep-research --output-dir /tmp/research https://example.com

# Extended timeout
gemini-deep-research --timeout 90 https://arxiv.org/abs/2601.20802
```

### Output Format

#### General Mode

```
output/20240203-150405-example-com/
├── metadata.json      # Pipeline metadata
├── topics.json        # Identified topics
├── report.md          # Research report
└── content.txt        # Extracted content
```

#### Competitive Mode

```
output/20240203-150405-competitive-github-com-features-copilot/
├── competitive-summary.md    # Executive summary
├── report.md                 # Gap analysis report
├── metadata.json             # Analysis metadata
├── topics.json               # Identified topics
└── content.txt               # Extracted content
```

### Console Output

**Pipeline Execution**:
```
Configuration:
  URL: https://example.com
  Mode: general
  Output Directory: ./output
  Timeout: 60 minutes

Step 1: Detecting content type...
  Detected: article

Checking cache for existing research...
  No cached research found, proceeding with extraction...

Step 2: Extracting content...
  Extracted 5234 characters

Step 2.5: Loading prompt configuration...
  Prompts loaded and @file syntax resolved

Step 3: Analyzing topics with Gemini...
  Identified 4 topics:
    1. Topic A
    2. Topic B
    3. Topic C
    4. Topic D

Step 4: Running Deep Research...
Research prompt: [prompt text]
  [polling progress...]

Step 5: Writing output files...
  Cached research at: ~/.cache/gemini-deep-research/.../report.md
  Output directory: output/20240203-150405-example-com

Pipeline completed successfully!
```

## Technical Constraints

### Dependencies

**Required**:
- Go 1.21 or higher
- Gemini CLI (for topic analysis)
- GEMINI_API_KEY environment variable

**Optional**:
- yt-dlp (for YouTube videos)
- GOOGLE_CUSTOM_SEARCH_API_KEY and GOOGLE_SEARCH_ENGINE_ID (for competitive mode discovery)

### Platform Support

- Linux (primary)
- macOS (supported)
- Windows (supported with WSL or native Go)

### API Limits

- Gemini Deep Research API: Subject to quota limits
- Google Custom Search API: 100 queries/day (free tier)

### Resource Constraints

- Memory: < 500MB for typical workloads
- Disk: Varies with cache size (typically < 100MB/month)
- Network: Requires stable internet connection

## Data Model

### Package Structure

**Note:** The specification references a `pkg/` directory, but the actual implementation organizes packages at the root level:

- `detector/` - Content type detection
- `config/` - Configuration management
- `gemini/` - Gemini CLI integration and topic analysis
- `research/` - Deep Research API client
- `extractors/` - Content extraction (youtube, arxiv, web)
- `internal/` - Internal utilities (cache, modes, discovery, templates)
- `cmd/` - Command-line interface
- `types/` - Shared type definitions

All documented packages exist and match the functionality described in this specification.

### Configuration

```go
type Config struct {
    APIKey       string    // Gemini API key
    ProjectID    string    // GCP project ID
    Timeout      int       // Deep Research timeout (minutes)
    PollInterval int       // Status polling interval (seconds)
    OutputDir    string    // Output directory path
    CacheDir     string    // Cache directory path
    Stdout       io.Writer // Standard output stream
    Stderr       io.Writer // Standard error stream
}
```

### Content

```go
type Content struct {
    Raw      string                 // Extracted text content
    Metadata map[string]interface{} // Content-specific metadata
}
```

### Flags

```go
type Flags struct {
    ExtractPrompt   string // Custom extraction prompt
    AnalyzePrompt   string // Custom analysis prompt
    ResearchPrompt  string // Custom research prompt
    Type            string // Content type override
    Mode            string // Analysis mode
    OutputDir       string // Output directory
    Timeout         int    // Research timeout
    Project         string // GCP project ID
    Force           bool   // Force cache refresh
    NoDiscovery     bool   // Skip URL discovery
    DiscoveryLimit  int    // Max URLs to discover
}
```

### Metadata Output

**General Mode**:
```json
{
  "url": "https://example.com",
  "content_type": "GenericArticle",
  "topics": ["Topic 1", "Topic 2", "Topic 3"],
  "timestamp": "2024-02-03T15:04:05Z",
  "mode": "general",
  "metadata": {
    "title": "Article Title",
    "authors": ["Author Name"]
  }
}
```

**Competitive Mode**:
```json
{
  "url": "https://github.com/features/copilot",
  "content_type": "GenericArticle",
  "topics": ["AI completion", "IDE integration"],
  "timestamp": "2024-02-03T15:04:05Z",
  "mode": "competitive",
  "competitor": "GitHub Copilot",
  "source_query": "GitHub Copilot vs Cursor",
  "metadata": {}
}
```

## Implementation Phases

### Phase 1: Core Pipeline (Completed)
- Content type detection
- Content extraction (YouTube, arXiv, HuggingFace, web)
- Topic analysis with Gemini
- Deep Research API integration
- Basic output generation

### Phase 2: Competitive Analysis (Completed)
- Mode detection
- URL discovery with Google Custom Search
- Competitive templates
- Gap analysis reports
- Executive summaries

### Phase 3: Caching & Performance (Completed)
- Intelligent caching system
- Content hash validation
- Cache directory management
- Force refresh support

### Phase 4: Customization (Completed)
- Custom prompt support
- @file syntax for prompts
- Template variable system
- Prompt validation

### Phase 5: Future Enhancements (Planned)
- Multi-competitor comparison
- Historical tracking
- Custom templates
- Export formats (PDF, HTML)
- Web UI

## Testing Strategy

### Unit Tests
- Test individual functions in isolation
- Mock external dependencies (HTTP, CLI, file system)
- Focus on business logic and edge cases
- Target: > 80% code coverage

### Integration Tests
- Test full E2E pipeline
- Test cache behavior
- Test competitive mode workflows
- Test error handling paths
- Use real/mocked components as appropriate

### Manual Testing
- User acceptance testing for common workflows
- Performance testing under load
- Error message clarity testing
- Documentation accuracy verification

## Documentation Requirements

### User Documentation
- README.md: Quick start, usage examples, configuration
- ARCHITECTURE.md: Technical architecture, package descriptions
- SPEC.md: This document
- docs/competitive-analysis.md: Competitive mode guide
- MIGRATION.md: Migration from bash version

### Developer Documentation
- ARCHITECTURE.md: Package structure, data flow, extension points
- ADR documents: Architectural decision records
- Code comments: Package-level and function-level documentation
- Test documentation: Test coverage reports, test strategy

### Operational Documentation
- Installation instructions
- Troubleshooting guide
- API setup guides (Gemini, Google Custom Search)
- Environment configuration examples

## Risks and Mitigations

### Risk 1: API Rate Limiting
**Impact**: High
**Probability**: Medium
**Mitigation**: Implement intelligent caching, provide clear quota monitoring, retry with exponential backoff

### Risk 2: Content Extraction Failures
**Impact**: High
**Probability**: Medium
**Mitigation**: Multi-strategy extraction (API + PDF fallback), clear error messages, graceful degradation

### Risk 3: Gemini CLI Dependency
**Impact**: High
**Probability**: Low
**Mitigation**: Document installation clearly, provide version compatibility matrix, consider direct API integration

### Risk 4: Cache Corruption
**Impact**: Medium
**Probability**: Low
**Mitigation**: Content hash validation, atomic file writes, graceful degradation on cache errors

### Risk 5: Discovery API Costs
**Impact**: Low
**Probability**: Medium
**Mitigation**: Provide --no-discovery flag, document free tier limits, cache discovery results

## Compliance and Privacy

### Data Privacy
- No user data collected or transmitted
- API keys stored locally in environment variables
- Research results stored locally (not transmitted to third parties)
- Content extraction respects robots.txt (where applicable)

### API Terms of Service
- Comply with Gemini API terms of service
- Comply with Google Custom Search API terms
- Respect content provider terms (YouTube, arXiv, etc.)
- Rate limiting to prevent abuse

## Success Criteria

### Launch Criteria
- ✅ All P0 functional requirements implemented
- ✅ > 80% unit test coverage
- ✅ Integration tests passing
- ✅ Documentation complete (README, ARCHITECTURE, SPEC)
- ✅ Manual testing completed for common workflows

### Post-Launch Metrics
- User adoption: > 100 unique users in first 3 months
- Satisfaction: > 80% positive feedback
- API efficiency: < 5% duplicate API calls
- Error rate: < 5% of pipeline executions
- Cache hit rate: > 60% after 1 month of usage

## Appendix

### Glossary

- **Content Type**: Category of source content (video, article, arxiv, huggingface)
- **Extractor**: Component that extracts text from specific content type
- **Pipeline**: End-to-end workflow from URL to research report
- **Cache**: Local storage for research results to avoid duplicate API calls
- **Gap Analysis**: Competitive analysis report comparing strengths and weaknesses
- **Template Variable**: Placeholder in prompts replaced with runtime values
- **Discovery**: Stage 0 of competitive mode that finds competitor URLs

### Related Documents

- [ARCHITECTURE.md](ARCHITECTURE.md): Technical architecture documentation
- [README.md](README.md): User-facing documentation
- [docs/competitive-analysis.md](docs/competitive-analysis.md): Competitive mode guide
- [MIGRATION.md](MIGRATION.md): Migration guide from bash version

### Revision History

| Version | Date       | Author | Changes                     |
|---------|------------|--------|-----------------------------|
| 2.0.0   | 2025-02-11 | AI     | Initial specification (backfill) |
