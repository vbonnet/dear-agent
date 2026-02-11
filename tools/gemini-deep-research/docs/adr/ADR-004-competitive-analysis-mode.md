# ADR-004: Competitive Analysis Mode

**Status**: Accepted
**Date**: 2025-01-15 (backfilled 2025-02-11)
**Deciders**: Engineering Team, Product Team
**Tags**: feature, templates, discovery, competitive-intelligence

## Context

Product teams need to conduct competitive analysis to understand competitor capabilities and identify product gaps. The manual process involves:

1. Finding competitor product pages (time-consuming search)
2. Reading competitor documentation (hours of manual analysis)
3. Extracting key features (error-prone note-taking)
4. Identifying gaps vs. our product (subjective analysis)
5. Prioritizing improvements (inconsistent frameworks)

This process is:
- **Time-intensive**: 4-8 hours per competitor
- **Inconsistent**: Different frameworks across team members
- **Not scalable**: Cannot track multiple competitors monthly
- **Low reuse**: Analysis results not easily shared

### Requirements

1. **Automated Discovery**: Find competitor URLs from natural language queries
2. **Gap Analysis**: Compare competitor strengths with our weaknesses
3. **Prioritization**: Rank recommendations by impact/effort
4. **Structured Output**: Consistent format for stakeholder review
5. **Metadata Tracking**: Record competitor, source query, analysis date

## Decision

Implement **Competitive Analysis Mode** as a specialized pipeline variant with:

### Mode Detection

Auto-detect competitive intent from query patterns:

```go
func DetectMode(query string, explicitMode string) Mode {
    // Explicit override
    if explicitMode != "" {
        return parseMode(explicitMode)
    }

    // Pattern-based detection
    patterns := []string{"vs", "compare", "competitor", "competitive"}
    for _, pattern := range patterns {
        if strings.Contains(strings.ToLower(query), pattern) {
            return ModeCompetitive
        }
    }

    return ModeGeneral
}
```

**Examples**:
- "GitHub Copilot vs Cursor" → Competitive mode
- "analyze GitHub Copilot" → General mode (no competitive keywords)
- "GitHub Copilot" --mode competitive → Competitive mode (explicit)

### Stage 0: URL Discovery

New pipeline stage for competitive mode only:

```go
// Stage 0: Discover competitor URLs (competitive mode only)
if mode.IsCompetitive() && !flags.NoDiscovery {
    urls, err := discovery.DiscoverCompetitorURLs(ctx, query, config)
    if err != nil {
        // Fallback: use provided URL if valid
        if ValidateURL(query) == nil {
            targetURL = query
        } else {
            return error
        }
    } else {
        targetURL = urls[0] // Use first discovered URL
    }
}
```

**Discovery Strategy**:
1. Extract competitor name from query ("GitHub Copilot" from "GitHub Copilot vs Cursor")
2. Generate search queries:
   - `{competitor} documentation`
   - `{competitor} GitHub`
   - `{competitor} official site`
3. Execute Google Custom Search API
4. Return top N URLs (configurable via `--discovery-limit`)

**Fallback Behavior**:
- If discovery fails (missing credentials, no results), fall back to analyzing provided URL
- No hard failure unless both discovery and provided URL are invalid

### Template System

Mode-specific prompt templates:

```
internal/templates/competitive/
├── analyze.tmpl        # Topic analysis (Step 3)
└── gap-analysis.tmpl   # Deep research (Step 4)
```

**analyze.tmpl** (Competitive Topic Analysis):
```
Analyze competitor documentation:
- Competitor: {{.Competitor}}
- Target (our tool): {{.Target}}
- URL: {{.URL}}

Extract:
- Core features and functionality
- Architecture patterns
- Unique differentiators
- Technology stack
```

**gap-analysis.tmpl** (Gap Analysis Research):
```
Compare capabilities:
- Competitor: {{.Competitor}}
- Target: {{.Target}}
- Topics: {{.Topics}}

Generate:
1. Competitor Strengths (what they do well)
2. Our Gaps (what we lack compared to them)
3. Prioritized Recommendations (impact/effort ranked)
4. Technical Insights (architecture, tech stack)
```

### Output Format

Competitive mode produces additional output:

```
output/20250115-103000-competitive-github-com-features-copilot/
├── competitive-summary.md    # Executive summary (NEW)
├── report.md                 # Gap analysis report
├── metadata.json             # Includes mode, competitor, source_query
├── topics.json               # Identified topics
└── content.txt               # Extracted content
```

**competitive-summary.md**:
```markdown
# Competitive Analysis Summary

**Competitor:** GitHub Copilot
**Analyzed URL:** https://github.com/features/copilot
**Generated:** 2025-01-15 10:30:00

---

## Analysis Focus Areas
1. AI-powered code completion
2. Multi-language support
3. IDE integration

---

## Gap Analysis Report

### Key Sections:
- Competitor Strengths: Features where they excel
- Our Gaps: Areas we lack
- Prioritized Recommendations: Impact/effort ranked
- Strategic Insights: Technical differences

---

⚠️ **Critical Items Identified**
High-priority gaps found. Review full report.
```

**metadata.json** (Competitive Mode):
```json
{
  "url": "https://github.com/features/copilot",
  "content_type": "GenericArticle",
  "topics": ["AI completion", "IDE integration"],
  "timestamp": "2025-01-15T10:30:00Z",
  "mode": "competitive",
  "competitor": "GitHub Copilot",
  "source_query": "GitHub Copilot vs Cursor"
}
```

## Consequences

### Positive

1. **Time Savings**: 4-8 hours → 30-60 minutes per competitor analysis
2. **Consistency**: Structured templates ensure uniform analysis
3. **Scalability**: Analyze multiple competitors in parallel
4. **Automation**: URL discovery eliminates manual search
5. **Executive Summaries**: Quick stakeholder review without reading full reports
6. **Metadata Tracking**: Historical tracking of competitor changes
7. **Reusability**: Cache enables fast re-analysis

### Negative

1. **API Dependency**: Requires Google Custom Search API (not free after quota)
2. **Discovery Quality**: Search results quality affects URL relevance
3. **Competitor Extraction**: Name extraction may fail on ambiguous queries
4. **Content Limitations**: JavaScript-heavy sites may have incomplete extraction
5. **Analysis Subjectivity**: AI-generated recommendations require human review

### Neutral

1. **Template Maintenance**: Templates need updates as product evolves
2. **Learning Curve**: Users must understand competitive vs. general modes
3. **Configuration Overhead**: Discovery requires API key setup

## Implementation

### Adding New Template

To customize competitive analysis:

1. Create template file: `internal/templates/competitive/custom.tmpl`
2. Define template data struct:
   ```go
   type PromptData struct {
       Competitor string
       Target     string
       URL        string
       Topics     []string
   }
   ```
3. Render template:
   ```go
   loader, _ := templates.NewLoader()
   prompt, _ := loader.Render(templates.TemplateCustom, data)
   ```

### Discovery Configuration

**Environment Variables**:
```bash
export GOOGLE_CUSTOM_SEARCH_API_KEY="your-api-key"
export GOOGLE_SEARCH_ENGINE_ID="your-search-engine-id"
```

**Setup Steps**:
1. Create API key: https://developers.google.com/custom-search/v1/overview
2. Create Search Engine: https://programmablesearchengine.google.com/
3. Export environment variables

**Bypassing Discovery**:
```bash
# Skip discovery if credentials unavailable
gemini-deep-research "https://competitor.com" --mode competitive --no-discovery
```

### Testing Strategy

**Unit Tests**:
```go
// Test mode detection
func TestModeDetection(t *testing.T) {
    tests := []struct {
        query string
        mode  Mode
    }{
        {"GitHub Copilot vs Cursor", ModeCompetitive},
        {"analyze GitHub Copilot", ModeGeneral},
        {"compare React and Vue", ModeCompetitive},
    }

    for _, tt := range tests {
        result := modes.DetectMode(tt.query, "")
        assert.Equal(t, tt.mode, result)
    }
}

// Test competitor extraction
func TestCompetitorExtraction(t *testing.T) {
    tests := []struct {
        query      string
        competitor string
    }{
        {"GitHub Copilot vs Cursor", "GitHub Copilot"},
        {"analyze Notion", "Notion"},
        {"competitive analysis of Figma", "Figma"},
    }

    for _, tt := range tests {
        result := discovery.ExtractCompetitorName(tt.query)
        assert.Equal(t, tt.competitor, result)
    }
}
```

**Integration Tests**:
```go
// Test E2E competitive pipeline
func TestCompetitivePipeline(t *testing.T) {
    flags := &types.Flags{
        Mode:           "competitive",
        NoDiscovery:    true, // Skip API calls in tests
    }

    exitCode := Run("https://github.com/features/copilot", flags, config)
    assert.Equal(t, 0, exitCode)

    // Verify competitive-specific files
    assert.FileExists(t, "output/.../competitive-summary.md")

    // Verify metadata includes competitive fields
    metadata := readMetadata(t, "output/.../metadata.json")
    assert.Equal(t, "competitive", metadata.Mode)
    assert.NotEmpty(t, metadata.Competitor)
}
```

## Alternatives Considered

### 1. Manual Competitor Analysis

**Approach**: No automation, users manually research competitors

**Pros**:
- No API dependencies
- Complete control over analysis

**Cons**:
- Time-intensive (4-8 hours per competitor)
- Inconsistent quality
- Not scalable
- No structured output

**Decision**: Rejected due to poor scalability

### 2. Generic Web Scraping (No Discovery)

**Approach**: Users provide competitor URLs, skip discovery

**Pros**:
- No Google Custom Search API dependency
- Simpler implementation
- No API costs

**Cons**:
- Users must manually find competitor URLs
- Still time-consuming
- Misses valuable automation

**Decision**: Rejected, but provided as `--no-discovery` option

### 3. Multi-Competitor Comparison

**Approach**: Analyze multiple competitors in one run

**Pros**:
- Comprehensive competitive landscape
- Side-by-side comparison
- Better prioritization

**Cons**:
- Complex implementation
- Higher API costs
- Longer execution time
- Overwhelms stakeholders

**Decision**: Deferred to future enhancement

### 4. Real-Time Monitoring

**Approach**: Continuously monitor competitor changes

**Pros**:
- Up-to-date insights
- Proactive alerts
- Historical tracking

**Cons**:
- Requires persistent infrastructure
- Higher API costs
- Complex scheduling
- Overkill for CLI tool

**Decision**: Rejected, consider for future SaaS offering

### 5. No Mode Distinction

**Approach**: Use general mode for all analysis types

**Pros**:
- Simpler architecture
- No mode detection logic
- Fewer templates

**Cons**:
- Generic prompts miss competitive nuances
- No gap analysis structure
- No competitor metadata tracking
- Poor stakeholder experience

**Decision**: Rejected due to poor competitive analysis quality

## Related Decisions

- ADR-001: Go Rewrite (enables modular mode system)
- ADR-002: Extractor Factory Pattern (supports discovery pipeline)
- ADR-005: Template-Based Prompts (mode-specific templates)
- ADR-006: Discovery Strategy (URL discovery implementation)

## References

- [Competitive Analysis Documentation](../../docs/competitive-analysis.md)
- [Template System](../../internal/templates/)
- [Google Custom Search API](https://developers.google.com/custom-search/v1/overview)
- [Mode Detection](../../internal/modes/detector.go)
- [Discovery Implementation](../../internal/discovery/web_search.go)

## Notes

Competitive Analysis Mode demonstrates the value of mode-specific pipelines. The template system provides flexibility for domain-specific prompts while maintaining a unified architecture.

The discovery system significantly reduces manual effort (from 4-8 hours to 30-60 minutes), but quality depends on:
1. Search result relevance (Google Custom Search API)
2. Competitor name extraction accuracy
3. Content extraction quality (JavaScript-heavy sites may fail)

Future enhancements should focus on:
- Multi-competitor comparison
- Historical change tracking
- Custom template marketplace
- Automated testing of recommendations
