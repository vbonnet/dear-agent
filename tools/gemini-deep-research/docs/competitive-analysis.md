# Competitive Analysis Mode

## Overview

Competitive Analysis Mode is a specialized research workflow for analyzing competitor tools and generating actionable gap analysis reports. It automates the process of discovering competitor URLs, extracting their capabilities, and identifying opportunities for improvement.

## Key Features

1. **Automatic URL Discovery**: Finds relevant competitor URLs using Google Custom Search
2. **Intelligent Competitor Extraction**: Identifies competitor names from queries like "X vs Y"
3. **Gap Analysis Reports**: Generates structured reports comparing competitor strengths with our gaps
4. **Mode-Specific Output**: Creates executive summaries and competitive-focused metadata

## Quick Start

### Basic Usage

```bash
# Analyze a specific competitor URL
gemini-deep-research "https://github.com/features/copilot" --mode competitive

# Let the tool discover competitor URLs from a query
gemini-deep-research "GitHub Copilot vs Cursor" --mode competitive

# Skip URL discovery and analyze the provided URL directly
gemini-deep-research "https://github.com/features/copilot" --mode competitive --no-discovery
```

### Common Patterns

```bash
# Analyze with custom discovery limit
gemini-deep-research "React vs Vue" --mode competitive --discovery-limit 10

# Force refresh (bypass cache)
gemini-deep-research "Notion vs Obsidian" --mode competitive --force

# Analyze without automatic URL discovery
gemini-deep-research "https://www.notion.so/product" --mode competitive --no-discovery
```

## How It Works

### Pipeline Overview

Competitive Analysis Mode follows a 6-stage pipeline:

```
Stage 0: Discovery (optional)
  ↓
Step 1: Content Type Detection
  ↓
Step 2: Content Extraction
  ↓
Step 3: Topic Analysis (Competitive Template)
  ↓
Step 4: Gap Analysis Research
  ↓
Step 5: Output Generation (Competitive Format)
```

### Stage 0: Discovery

**Purpose**: Find relevant competitor URLs from a natural language query

**How it works**:
1. Extracts competitor name from query (e.g., "GitHub Copilot" from "GitHub Copilot vs Cursor")
2. Generates 3 search queries:
   - `{competitor} documentation`
   - `{competitor} GitHub`
   - `{competitor} official site`
3. Executes Google Custom Search API for each query
4. Returns up to N URLs (configurable via `--discovery-limit`)

**Fallback behavior**:
- If discovery fails (missing credentials, no results), falls back to analyzing the provided URL
- Logs warning but continues execution
- No hard failure unless both discovery and provided URL are invalid

**Example**:
```bash
# Input query
gemini-deep-research "GitHub Copilot vs Cursor"

# Discovery extracts: "GitHub Copilot"
# Searches:
#   - "GitHub Copilot documentation"
#   - "GitHub Copilot GitHub"
#   - "GitHub Copilot official site"
# Returns: ["https://github.com/features/copilot", "https://docs.github.com/copilot", ...]
# Analyzes: First URL (github.com/features/copilot)
```

### Step 3: Topic Analysis (Competitive Template)

**Purpose**: Identify key focus areas for gap analysis

**Template**: `templates/competitive-analyze.tmpl`

**Prompt structure**:
- Competitor name
- Target tool name (default: "Our Tool")
- URL being analyzed
- Task: Identify competitor strengths and capabilities

**Output**: List of topics representing competitor's key features

**Example output**:
```json
[
  "AI-powered code completion",
  "Multi-language support",
  "IDE integration (VS Code, JetBrains)",
  "Context-aware suggestions",
  "Enterprise features"
]
```

### Step 4: Gap Analysis Research

**Purpose**: Generate detailed comparison and recommendations

**Template**: `templates/competitive-gap-analysis.tmpl`

**Prompt structure**:
- Competitor name
- Target tool name
- Identified topics (from Step 3)
- Task: Compare capabilities and prioritize recommendations

**Output structure**:
```markdown
# Gap Analysis: {Competitor}

## Competitor Strengths
- Feature X: Description and implementation details
- Feature Y: How they solve problem Z

## Our Gaps
- Missing Feature A: Impact and user value
- Limited Feature B: Current state vs competitor

## Prioritized Recommendations
1. **High Priority**: Feature A
   - Impact: 9/10
   - Effort: 6/10
   - Rationale: Critical user need, moderate complexity

2. **Medium Priority**: Feature B
   - Impact: 7/10
   - Effort: 4/10
   - Rationale: Nice-to-have, quick win

## Technical Insights
- Architecture differences
- Technology stack comparison
- Implementation considerations
```

### Step 5: Output Generation

**Directory structure**:
```
{TIMESTAMP}-competitive-{URL}/
├── competitive-summary.md    # Executive summary
├── report.md                 # Full gap analysis report
├── metadata.json             # Analysis metadata (includes mode, competitor, source query)
├── topics.json               # Identified topics
└── content.txt               # Extracted competitor content
```

**competitive-summary.md format**:
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

### Key Sections to Review:
- **Competitor Strengths**: Features where the competitor excels
- **Our Gaps**: Areas where our tool lacks compared to competitor
- **Prioritized Recommendations**: Actionable improvements ranked by impact/effort
- **Strategic Insights**: Technical differences and architectural considerations

⚠️  **Critical Items Identified**

The analysis has identified high-priority gaps. Review the full report for immediate action items.

---

## Files in This Analysis

- `competitive-summary.md` - This executive summary
- `report.md` - Full detailed gap analysis report
- `metadata.json` - Analysis metadata and configuration
- `topics.json` - Identified focus areas
- `content.txt` - Extracted competitor content
```

## Configuration

### Environment Variables (Discovery)

Required for URL discovery (Stage 0):

```bash
# Google Custom Search API credentials
export GOOGLE_CUSTOM_SEARCH_API_KEY="your-api-key-here"
export GOOGLE_SEARCH_ENGINE_ID="your-search-engine-id"
```

**Setup instructions**:
1. Create API key: https://developers.google.com/custom-search/v1/overview
2. Create Search Engine: https://programmablesearchengine.google.com/
3. Export environment variables

**Fallback**: Use `--no-discovery` flag to skip URL discovery if credentials are not available

### CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode` | string | auto | Analysis mode: `general` or `competitive` |
| `--no-discovery` | bool | false | Skip URL discovery in competitive mode |
| `--discovery-limit` | int | 5 | Max URLs to discover (0-20) |
| `--force` | bool | false | Bypass cache and force fresh research |

## Mode Detection

The tool automatically detects competitive mode when:
1. User explicitly sets `--mode competitive`
2. URL matches competitive patterns:
   - Contains "vs" keyword (e.g., "React vs Vue")
   - Contains "compare" keyword
   - Contains "competitor" keyword

**Manual override**: Use `--mode general` to force general mode even for competitive queries

**Example**:
```bash
# Auto-detected as competitive
gemini-deep-research "GitHub Copilot vs Cursor"

# Forced to general mode
gemini-deep-research "GitHub Copilot vs Cursor" --mode general
```

## Output Metadata

Competitive mode adds mode-specific fields to `metadata.json`:

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

**Field descriptions**:
- `mode`: Analysis mode (always "competitive" in this mode)
- `competitor`: Extracted competitor name
- `source_query`: Original user query (for discovery mode)

## Error Handling

### Discovery Failures

**Scenario 1: Missing API credentials**

```
Error: GOOGLE_CUSTOM_SEARCH_API_KEY environment variable not set

Setup instructions:
1. Create a Google Custom Search API key: https://developers.google.com/custom-search/v1/overview
2. Export the environment variable:
   export GOOGLE_CUSTOM_SEARCH_API_KEY="your-api-key"
3. Alternatively, use --no-discovery flag to skip URL discovery
```

**Resolution**: Either set up credentials or use `--no-discovery`

---

**Scenario 2: No URLs discovered**

```
Warning: No competitor URLs discovered
Falling back to analyzing the provided URL directly...
```

**Resolution**: Tool automatically falls back to analyzing the provided URL

---

**Scenario 3: API quota exceeded**

```
Warning: Discovery failed: search API call failed: quota exceeded
Falling back to analyzing the provided URL directly...
```

**Resolution**: Wait for quota reset or use `--no-discovery`

### Template Errors

**Scenario: Empty competitor name**

```
Error: failed to extract competitor name from query: "analyze tool"

Tip: Try using a more explicit query like 'GitHub Copilot vs Cursor' or 'analyze GitHub Copilot'
```

**Resolution**: Rephrase query to include explicit competitor name

---

**Scenario: No topics identified**

```
Error: no topics provided for gap analysis

Topics should be identified during the analysis stage. This is likely a pipeline error.
```

**Resolution**: Check content extraction quality or URL accessibility

## Best Practices

### 1. Query Formulation

**Good queries** (clear competitor name):
```bash
✅ "GitHub Copilot vs Cursor"
✅ "analyze GitHub Copilot"
✅ "competitive analysis of Notion"
✅ "what can we learn from Figma"
```

**Poor queries** (ambiguous competitor):
```bash
❌ "analyze tool"
❌ "vs competitor"
❌ "compare features"
```

### 2. Discovery Optimization

**Use `--discovery-limit` for speed**:
```bash
# Fast: Only discover top 3 URLs
gemini-deep-research "React vs Vue" --mode competitive --discovery-limit 3

# Comprehensive: Discover up to 10 URLs
gemini-deep-research "React vs Vue" --mode competitive --discovery-limit 10
```

**Skip discovery when you have the URL**:
```bash
# Analyze specific URL directly
gemini-deep-research "https://react.dev" --mode competitive --no-discovery
```

### 3. Cache Management

**Use `--force` sparingly**:
- Cache improves speed significantly (seconds vs minutes)
- Only use `--force` when:
  - Competitor website updated
  - Previous analysis was incomplete
  - Testing prompt changes

**Check cache before forcing**:
```bash
# First run (creates cache)
gemini-deep-research "GitHub Copilot vs Cursor" --mode competitive

# Subsequent runs (uses cache)
gemini-deep-research "GitHub Copilot vs Cursor" --mode competitive

# Force refresh only when needed
gemini-deep-research "GitHub Copilot vs Cursor" --mode competitive --force
```

## Integration Examples

### CI/CD Pipeline

```yaml
# .github/workflows/competitor-analysis.yml
name: Monthly Competitor Analysis

on:
  schedule:
    - cron: '0 0 1 * *'  # First day of month
  workflow_dispatch:

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Install gemini-deep-research
        run: go install github.com/vbonnet/ai-tools/tools/gemini-deep-research@latest

      - name: Analyze competitors
        env:
          GOOGLE_CUSTOM_SEARCH_API_KEY: ${{ secrets.GOOGLE_API_KEY }}
          GOOGLE_SEARCH_ENGINE_ID: ${{ secrets.SEARCH_ENGINE_ID }}
          GEMINI_API_KEY: ${{ secrets.GEMINI_API_KEY }}
        run: |
          gemini-deep-research "GitHub Copilot" --mode competitive --force
          gemini-deep-research "Cursor" --mode competitive --force

      - name: Upload reports
        uses: actions/upload-artifact@v3
        with:
          name: competitor-analysis
          path: output/
```

### Scripted Analysis

```bash
#!/bin/bash
# analyze-competitors.sh

COMPETITORS=(
  "GitHub Copilot"
  "Cursor"
  "Tabnine"
  "Codeium"
)

for competitor in "${COMPETITORS[@]}"; do
  echo "Analyzing: $competitor"
  gemini-deep-research "$competitor" \
    --mode competitive \
    --discovery-limit 5 \
    --force
done

echo "Analysis complete. Reports in output/"
```

## Troubleshooting

### Issue: Discovery always fails

**Symptoms**:
- Always falls back to provided URL
- No URLs discovered

**Checks**:
1. Verify API credentials are set:
   ```bash
   echo $GOOGLE_CUSTOM_SEARCH_API_KEY
   echo $GOOGLE_SEARCH_ENGINE_ID
   ```
2. Test API manually:
   ```bash
   curl "https://www.googleapis.com/customsearch/v1?key=$GOOGLE_CUSTOM_SEARCH_API_KEY&cx=$GOOGLE_SEARCH_ENGINE_ID&q=test"
   ```
3. Check quota limits in Google Cloud Console

### Issue: Empty gap analysis reports

**Symptoms**:
- Report contains minimal content
- Few or no recommendations

**Checks**:
1. Verify URL is accessible:
   ```bash
   curl -I "https://competitor-url"
   ```
2. Check content extraction quality:
   - Review `content.txt` in output directory
   - Should have substantial text (>1KB)
3. Try with a different competitor URL

### Issue: Mode not detected correctly

**Symptoms**:
- Competitive query runs in general mode
- General query runs in competitive mode

**Fix**: Use explicit `--mode` flag:
```bash
# Force competitive mode
gemini-deep-research "GitHub Copilot" --mode competitive

# Force general mode
gemini-deep-research "GitHub Copilot vs Cursor" --mode general
```

## Limitations

1. **URL Discovery**:
   - Requires Google Custom Search API (not free after quota)
   - Quality depends on search result relevance
   - Limited to 100 queries/day (free tier)

2. **Content Extraction**:
   - JavaScript-heavy sites may have incomplete extraction
   - Paywalled content not accessible
   - Rate limiting may affect extraction

3. **Analysis Quality**:
   - Depends on Gemini model capabilities
   - May miss subtle competitive advantages
   - Recommendations are AI-generated (review required)

## Future Enhancements

Planned improvements:
- [ ] Multi-competitor comparison (compare A vs B vs C)
- [ ] Historical tracking (monitor competitor changes over time)
- [ ] Custom templates (user-defined analysis prompts)
- [ ] Automated testing (validate recommendations)
- [ ] Export formats (PDF, HTML, Notion)

## See Also

- [General Mode Documentation](./general-mode.md)
- [Template System](./templates.md)
- [Configuration Reference](./configuration.md)
- [API Documentation](./api.md)
