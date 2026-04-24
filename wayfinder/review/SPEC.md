# Multi-Persona Review Plugin - Specification

## Overview

The Multi-Persona Review plugin is an automated code review system that leverages multiple AI personas to perform comprehensive code analysis from different expert perspectives (security, performance, accessibility, etc.). It integrates with Wayfinder workflows and supports multiple LLM providers.

## Purpose

Enable automated, multi-perspective code reviews that:
- Detect issues across multiple domains (security, performance, accessibility, error handling, etc.)
- Reduce false positives through intelligent deduplication
- Provide actionable feedback with suggested fixes
- Track review costs across different LLM providers
- Integrate seamlessly into CI/CD pipelines and development workflows

## Key Features

### 1. Multi-Persona Review System
- Load and execute multiple review personas in parallel or sequentially
- Each persona focuses on specific expertise areas (security, performance, accessibility, etc.)
- Personas are configurable via YAML files or .ai.md frontmatter format
- Support for user, project, company, and core persona libraries

### 2. Finding Detection and Deduplication
- Collect findings from all personas with severity levels (critical, high, medium, low, info)
- Intelligent deduplication merges similar findings from multiple personas
- Configurable similarity threshold (default 0.8)
- Preserves highest severity and combines persona attributions
- Reduces review noise by 50%+ while maintaining detection accuracy

### 3. File Scanning Modes
- **Full Mode**: Review entire file contents
- **Diff Mode**: Review only git diff with context lines
- **Changed Mode**: Review only files changed in git

### 4. Review Modes
- **Quick**: Focus on critical/high severity issues only, scan changed files
- **Thorough**: Comprehensive review across all severity levels, full file scan
- **Custom**: User-configurable mode with specific personas and options

### 5. LLM Provider Support
- **Anthropic Claude (Direct API)**: Primary provider with claude-sonnet-4.5, claude-haiku-4.5, claude-opus-4.5 (latest as of Feb 2026)
- **Google Vertex AI (Gemini)**: Secondary provider for Gemini models (gemini-2.5-flash, gemini-2.5-flash-lite)
- **Google Vertex AI (Claude)**: Recommended provider for Claude via VertexAI Anthropic publisher
  - Models: claude-sonnet-4-5@20250929, claude-haiku-4-5@20251001, claude-opus-4-6@20260205
  - Benefits: Unified GCP billing, committed use discounts, better GCP integration
  - Region: us-east5 (where Claude models are available)
- Token usage tracking and cost calculation
- Configurable model selection, temperature, and max tokens
- Auto-detection: If VERTEX_MODEL contains "claude", automatically uses vertexai-claude provider

### 6. Cost Tracking
- Real-time cost calculation based on token usage
- Multiple cost sink backends:
  - stdout (console output)
  - file (JSON log files)
  - GCP Cloud Monitoring (metrics export)
  - AWS, Datadog, Webhook (extensible architecture)
- Per-persona cost breakdown
- Session metadata (mode, files reviewed, findings count)

### 7. Agency-Agents Collaboration Patterns
- **Voting Mechanism**: GO/NO-GO decisions with tier-weighted aggregation
  - Tier 1 personas (security leads): 3x vote weight
  - Tier 2 personas (standard reviewers): 1x vote weight
  - Tier 3 personas (junior reviewers): 0.5x vote weight
  - GO decision if weighted_vote_sum / total_weight > threshold (default: 0.5)
  - CLI: `--vote-threshold <0-1>` to customize threshold
- **Confidence Scoring**: Filter findings by confidence level
  - Confidence scores from 0-1 (default: 0.8 if not specified)
  - CLI: `--min-confidence <0-1>` to filter low-confidence findings
  - Helps reduce false positives and noise
- **Lateral Thinking**: Personas propose 3 alternative approaches per finding
  - Encourages creative problem-solving
  - Provides multiple solution paths
  - Displayed in all formatters (text, JSON, GitHub)
- **Expertise Boundary Detection**: Personas flag out-of-scope findings
  - `outOfScope: true` when finding requires different expertise
  - Enables routing to appropriate persona
  - Improves finding quality and reduces false positives

### 8. Output Formatters
- **Text Formatter**: Human-readable terminal output with color coding
  - Displays Agency-Agents fields: decision, alternatives, out-of-scope warnings
- **JSON Formatter**: Machine-parseable structured output
  - Full serialization including Agency-Agents metadata
- **GitHub Formatter**: GitHub Actions annotation format for PR comments
  - Markdown formatting for alternatives and decision indicators

### 9. Configuration System
- YAML-based configuration at `.wayfinder/config.yml`
- Hierarchical defaults: user → project → company → core
- Configurable persona search paths with placeholder expansion
- GitHub integration settings (PR triggers, file filters, concurrency)

## Core Types

### Persona
```typescript
interface Persona {
  name: string;              // Unique identifier (kebab-case)
  displayName: string;       // Human-readable name
  version: string;           // Semver version
  description: string;       // Persona expertise description
  focusAreas: string[];      // Areas of expertise
  prompt: string;            // System prompt for LLM
  severityLevels?: Severity[];
  gitHistoryAccess?: boolean;
  tier?: 1 | 2 | 3;          // Agency-Agents: Vote weight (1=3x, 2=1x, 3=0.5x)
  config?: Record<string, unknown>;
}
```

### Finding
```typescript
interface Finding {
  id: string;                // Unique finding identifier
  file: string;              // File path (relative to repo root)
  line?: number;             // Line number (1-indexed)
  lineEnd?: number;          // End line for multi-line findings
  severity: Severity;        // critical | high | medium | low | info
  personas: string[];        // Personas that reported this finding
  title: string;             // Short summary
  description: string;       // Detailed explanation
  categories?: string[];     // Tags (e.g., "performance", "security")
  suggestedFix?: SuggestedFix;
  confidence?: number;       // 0-1 confidence score

  // Agency-Agents fields
  decision?: 'GO' | 'NO-GO'; // Voting decision (GO = approve, NO-GO = block)
  alternatives?: string[];   // 3 alternative approaches (lateral thinking)
  outOfScope?: boolean;      // Finding outside persona's expertise

  metadata?: Record<string, unknown>;
}
```

### ReviewResult
```typescript
interface ReviewResult {
  sessionId: string;
  startTime: Date;
  endTime: Date;
  config: ReviewConfig;
  findings: Finding[];
  findingsByFile: Record<string, Finding[]>;
  summary: ReviewSummary;
  cost: CostInfo;
  errors?: ReviewError[];
}
```

## Usage Patterns

### Command Line Interface
```bash
# Quick review of changed files
multi-persona-review quick src/

# Thorough review with specific personas
multi-persona-review thorough --personas security-engineer,performance-reviewer src/

# Review with JSON output
multi-persona-review quick --format json src/ > review.json

# Review with GCP cost tracking
multi-persona-review thorough --cost-sink gcp --gcp-project my-project src/

# Agency-Agents: Review with voting threshold and confidence filtering
multi-persona-review thorough --vote-threshold 0.7 --min-confidence 0.8 src/

# Agency-Agents: Strict review (high vote threshold, high confidence)
multi-persona-review thorough --vote-threshold 0.8 --min-confidence 0.9 src/
```

### Programmatic API
```typescript
import { reviewFiles, createAnthropicReviewer, loadSpecificPersonas } from '@wayfinder/multi-persona-review';

// Load personas
const personas = await loadSpecificPersonas(
  ['security-engineer', 'performance-reviewer'],
  ['~/.wayfinder/personas', '.wayfinder/personas']
);

// Create reviewer
const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022'
});

// Execute review
const result = await reviewFiles(
  {
    files: ['src/**/*.ts'],
    personas,
    mode: 'thorough',
    options: {
      deduplicate: true,
      similarityThreshold: 0.8
    }
  },
  process.cwd(),
  reviewer
);

console.log(`Found ${result.summary.totalFindings} issues`);
```

### GitHub Actions Integration
```yaml
name: Multi-Persona Review
on: pull_request

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Run Multi-Persona Review
        run: |
          npx @wayfinder/multi-persona-review quick \
            --format github \
            --personas security-engineer,code-quality-reviewer \
            $(git diff --name-only origin/main...HEAD)
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Configuration

### Default Configuration
```yaml
crossCheck:
  defaultPersonas:
    - security-engineer
    - code-health
    - error-handling-specialist
  defaultMode: quick
  personaPaths:
    - ~/.wayfinder/personas        # User overrides
    - .wayfinder/personas           # Project-specific
    - '{company}/personas'          # Company-level
    - '{personas}'                  # Shared Personas plugin
    - '{core}/personas'             # Legacy fallback
  costTracking:
    type: stdout
  options:
    contextLines: 3
    autoFix: false
    deduplicate: true
    similarityThreshold: 0.8
    model: claude-3-5-sonnet-20241022
  github:
    enabled: false
    changedFilesOnly: true
    skipDrafts: true
    concurrency: 3
    include: ['**/*']
    exclude: ['**/node_modules/**', '**/dist/**', '**/build/**', '**/*.min.js']
```

## Performance Characteristics

### Benchmarks
- **Quick Mode (3 personas)**: < 10 seconds for typical PR (5-10 files)
- **Thorough Mode (5+ personas)**: < 30 seconds for comprehensive review
- **Deduplication Overhead**: < 1 second (negligible)

### Accuracy Metrics
- **Detection Rate**: 95%+ for critical security/performance issues
- **False Positive Rate**: 30-50% for low confidence findings (acceptable)
- **Noise Reduction**: 50%+ through deduplication

### Cost Estimates (as of February 2026)
- **Quick Review (Claude Sonnet 4.5)**: $0.05-0.15 per PR
- **Thorough Review (Claude Sonnet 4.5)**: $0.20-0.50 per PR
- **Quick Review (Claude Haiku 4.5)**: $0.01-0.03 per PR (cost-optimized)

**Current Anthropic Pricing (2026)**:
- Claude Haiku 4.5: $1 input / $5 output per million tokens
- Claude Sonnet 4.5: $3 input / $15 output per million tokens
- Claude Opus 4.5: $5 input / $25 output per million tokens
- Batch processing: 50% discount for non-urgent workloads

Note: Verify current rates at https://platform.claude.com/docs/en/about-claude/pricing

## Error Handling

### Error Codes
- **CONFIG_1xxx**: Configuration loading errors
- **PERSONA_2xxx**: Persona loading/validation errors
- **FILE_SCANNER_3xxx**: File scanning errors
- **REVIEW_4xxx**: Review engine errors
- **ANTHROPIC_5xxx**: Anthropic API errors
- **VERTEXAI_6xxx**: Vertex AI errors
- **COST_SINK_7xxx**: Cost tracking errors

### Graceful Degradation
- Individual persona failures don't block review (logged as errors)
- Review succeeds if at least one persona completes successfully
- Cost tracking failures logged but don't block review
- File scanning errors skip problematic files with warning

## Security Considerations

### API Key Management
- Support for environment variables (ANTHROPIC_API_KEY, GOOGLE_APPLICATION_CREDENTIALS)
- Never log or expose API keys in output
- Cost tracking sinks use secure credential mechanisms

### File Access
- Respect .gitignore patterns
- Skip binary files automatically
- File size limits (default 1MB) to prevent OOM
- Path traversal validation for user inputs

### Data Privacy
- Code content sent to external LLM APIs (Claude, Gemini)
- Cost metadata can include file counts but not file contents
- Local-only mode not currently supported (future enhancement)

## Extension Points

### Custom Personas
- Users can add personas via `.wayfinder/personas/*.yaml` or `.ai.md` files
- Company-level persona libraries via plugin system
- Persona versioning enables gradual rollout of improvements

### Custom Cost Sinks
- Implement `CostSink` interface for new backends
- Register via configuration: `costTracking: { type: 'custom', config: {...} }`
- Examples: AWS CloudWatch, Datadog, webhooks

### Custom Formatters
- Implement output formatters for new platforms (GitLab, Bitbucket, etc.)
- Extend `formatReviewResult` with new format types

## Future Enhancements

### Planned Features
- Auto-fix mode: Apply suggested fixes automatically with user confirmation
- Git history analysis: Personas can request git blame/log for context
- Multi-file context: Cross-file analysis for architectural issues
- Local LLM support: Ollama, LlamaCpp for offline/private reviews
- Incremental reviews: Cache persona results to avoid re-reviewing unchanged code
- Custom severity rules: Project-specific severity mappings

### Under Consideration
- IDE integrations (VS Code, IntelliJ)
- Real-time review on file save
- Learning from user feedback (tune deduplication, severity classification)
- Team-level personas (organization-specific expertise)

## Success Metrics

### Adoption Metrics
- Number of reviews per day/week
- Number of active personas in use
- Number of custom personas created by users

### Quality Metrics
- Finding acceptance rate (% of findings addressed by developers)
- Time to resolution for different severity levels
- False positive rate trending over time

### Efficiency Metrics
- Average review time per PR
- Cost per review by mode/persona
- Developer time saved (vs manual review)

## Dependencies

### Runtime Dependencies
- `@anthropic-ai/sdk`: Anthropic Claude API client
- `@google-cloud/vertexai`: Google Vertex AI client
- `yaml`: YAML parsing for configuration and personas
- `chalk`: Terminal color output
- `commander`: CLI argument parsing

### Development Dependencies
- `typescript`: TypeScript compiler
- `vitest`: Testing framework
- `esbuild`: Fast bundler

### Optional Dependencies
- `@google-cloud/monitoring`: GCP Cloud Monitoring cost sink

## Versioning and Compatibility

### Version: 0.1.0
- Initial release with core functionality
- Anthropic Claude and Vertex AI support
- Multi-persona deduplication
- Cost tracking with multiple sinks

### Compatibility
- Node.js: >= 18.0.0
- Wayfinder: Any version (standalone plugin)
- Git: >= 2.0 (for diff mode)

### Breaking Changes Policy
- Semver versioning
- Persona format changes trigger major version bump
- Configuration schema changes documented in migration guides
- API changes follow deprecation cycle (warn → remove)

## License

Apache-2.0
