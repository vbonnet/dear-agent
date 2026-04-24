# Architecture Decision Records - Multi-Persona Review Plugin

## ADR-001: Multi-Persona Architecture with Independent Review Executions

**Status:** Accepted

**Context:**
We needed to design a code review system that could leverage multiple expert perspectives (security, performance, accessibility, etc.) without bias or interference between reviewers. Traditional single-reviewer approaches miss domain-specific issues that experts would catch.

**Decision:**
We adopted a multi-persona architecture where each persona performs independent reviews in parallel, then findings are aggregated and deduplicated. Each persona is a complete expert profile with:
- Unique focus areas (security, performance, etc.)
- Custom system prompt for LLM
- Independent execution context
- Separate cost tracking

**Consequences:**
- **Positive:**
  - Comprehensive coverage across multiple domains
  - Parallel execution reduces total review time
  - No persona bias (reviewers don't influence each other)
  - Easy to add/remove personas without affecting others
  - Clear attribution of findings to personas

- **Negative:**
  - Higher token usage and costs (multiple LLM calls)
  - Potential for duplicate findings requiring deduplication
  - Complexity in aggregating and merging results

**Alternatives Considered:**
1. **Single LLM with combined prompt**: Simpler but loses domain expertise depth
2. **Sequential chained reviews**: Lower parallelism, slower execution
3. **Hierarchical review (general → specific)**: More complex orchestration, unclear benefits

---

## ADR-002: Intelligent Deduplication with Similarity Threshold

**Status:** Accepted

**Context:**
Multi-persona reviews generate many duplicate or near-duplicate findings when multiple personas identify the same issue from different angles. Raw findings create noise and reduce actionability for developers.

**Decision:**
We implemented a deduplication system that:
- Groups findings by file and line proximity (±5 lines)
- Calculates similarity scores using Levenshtein distance for titles
- Merges findings above configurable threshold (default 0.8)
- Preserves highest severity and combines persona attributions
- Averages confidence scores

**Consequences:**
- **Positive:**
  - 50%+ noise reduction in typical reviews
  - Developers see consolidated, high-signal findings
  - Multi-persona consensus increases finding confidence
  - Configurable threshold allows tuning for false positive tolerance

- **Negative:**
  - Risk of over-deduplication (merging distinct issues)
  - Computational overhead (O(n²) similarity comparisons)
  - Tuning threshold requires experimentation

**Alternatives Considered:**
1. **No deduplication**: Too much noise for practical use
2. **Exact matching only**: Misses near-duplicates with different wording
3. **ML-based semantic similarity**: Overcomplicated for current needs, future enhancement

---

## ADR-003: Abstracted LLM Client with Provider-Agnostic Interface

**Status:** Accepted

**Context:**
We needed to support multiple LLM providers (Anthropic Claude, Google Gemini, potentially others) without coupling the review engine to specific APIs. Provider-specific code scattered throughout would make it hard to add new providers or switch defaults.

**Decision:**
We defined a `ReviewerFunction` interface that abstracts LLM interactions:
```typescript
type ReviewerFunction = (input: PersonaReviewInput) => Promise<PersonaReviewOutput>
```

Each provider implements this interface:
- `createAnthropicReviewer()` for Claude
- `createVertexAIReviewer()` for Gemini
- Future: `createOllamaReviewer()`, etc.

**Consequences:**
- **Positive:**
  - Review engine is provider-agnostic
  - Easy to add new LLM providers
  - Users can choose providers based on cost/performance
  - Testable with mock reviewers (no API calls)

- **Negative:**
  - Abstraction limits provider-specific features (e.g., Claude caching, Gemini grounding)
  - Each provider requires custom prompt formatting
  - Cost calculation must be provider-specific

**Alternatives Considered:**
1. **Hard-coded Anthropic client**: Simpler but inflexible
2. **Plugin-based provider system**: Overcomplicated for current needs
3. **LangChain/LlamaIndex integration**: Heavy dependency, unnecessary abstraction

---

## ADR-004: YAML Configuration with Hierarchical Defaults

**Status:** Accepted

**Context:**
Users need to configure personas, review modes, cost tracking, and other options. Configuration should support defaults, user overrides, and project-specific settings without duplication.

**Decision:**
We use a hierarchical YAML configuration system:
```
System Defaults (hardcoded)
  ↓ override
User Config (~/.wayfinder/config.yml)
  ↓ override
Project Config (.wayfinder/config.yml)
  ↓ merge
Final Configuration
```

Configuration includes:
- Default personas to use
- Persona search paths with placeholders (`{company}`, `{core}`)
- Review mode (quick/thorough)
- Cost tracking settings
- GitHub integration settings

**Consequences:**
- **Positive:**
  - Sensible defaults work out-of-box
  - Users can customize globally or per-project
  - Placeholder expansion supports plugin ecosystem
  - YAML is human-readable and git-friendly

- **Negative:**
  - YAML parsing adds dependency
  - Hierarchical merging can be confusing
  - No validation until runtime

**Alternatives Considered:**
1. **JSON configuration**: Less human-friendly, no comments
2. **Environment variables only**: Not suitable for complex nested config
3. **TypeScript config files**: Harder to parse, less portable

---

## ADR-005: Git-Aware File Scanning with Multiple Modes

**Status:** Accepted

**Context:**
Code reviews should focus on relevant code, not entire repositories. Reviewing unchanged files wastes tokens and costs. Different scenarios need different scanning strategies (PR reviews vs full audits).

**Decision:**
We implemented three file scanning modes:
1. **Full Mode**: Scan entire file contents (for comprehensive audits)
2. **Diff Mode**: Scan only git diff with context lines (for PR reviews)
3. **Changed Mode**: Scan only files changed in git (hybrid approach)

File scanner:
- Integrates with git to extract diffs
- Maps diff line numbers to original file lines
- Skips binary files automatically
- Respects file size limits (default 1MB)

**Consequences:**
- **Positive:**
  - 80%+ token reduction in diff mode for typical PRs
  - Faster reviews and lower costs
  - Context lines provide sufficient surrounding code
  - Changed mode balances coverage and efficiency

- **Negative:**
  - Git dependency required for diff/changed modes
  - Line mapping complexity for findings
  - Context lines may miss relevant surrounding code

**Alternatives Considered:**
1. **Full scan only**: Too expensive for PR reviews
2. **Diff only**: May miss context-dependent issues
3. **AST-based smart extraction**: Complex, language-specific

---

## ADR-006: Cost Tracking with Pluggable Sink Architecture

**Status:** Accepted

**Context:**
Users need to track LLM costs for budgeting, chargeback, and optimization. Different users have different monitoring systems (stdout, files, GCP, AWS, Datadog, etc.). Hard-coding one approach doesn't meet all needs.

**Decision:**
We implemented a pluggable cost sink architecture:
```typescript
interface CostSink {
  record(cost: CostInfo, metadata: CostMetadata): Promise<void>;
  flush?(): Promise<void>;
}
```

Built-in sinks:
- `StdoutCostSink`: Print to console (default)
- `FileCostSink`: Append to JSON log file
- `GCPCostSink`: Export to Google Cloud Monitoring

Users configure via YAML:
```yaml
costTracking:
  type: gcp
  config:
    projectId: my-project
    metricName: custom.googleapis.com/multi_persona_review/cost
```

**Consequences:**
- **Positive:**
  - Flexible cost tracking for different environments
  - Easy to add new sinks without changing core
  - Sink failures don't block reviews (graceful degradation)
  - Metadata includes mode, files, findings for analysis

- **Negative:**
  - Each sink requires custom implementation
  - Async recording adds complexity
  - No built-in cost aggregation or alerting

**Alternatives Considered:**
1. **Stdout only**: Too limited for production use
2. **Cloud-specific (GCP only)**: Excludes AWS/Azure users
3. **Built-in database storage**: Overcomplicated, hard to maintain

---

## ADR-007: Persona Format Supporting Both YAML and .ai.md

**Status:** Accepted

**Context:**
We needed a persona format that:
- Is human-readable and git-friendly
- Supports versioning and metadata
- Integrates with the Personas plugin (uses .ai.md format)
- Maintains backward compatibility with existing YAML personas

**Decision:**
We support two persona formats:
1. **Legacy YAML** (`.yaml`):
   ```yaml
   name: security-engineer
   displayName: Security Engineer
   version: 1.0.0
   prompt: "You are a security expert..."
   focusAreas: [sql-injection, xss]
   ```

2. **New .ai.md** (`.ai.md`):
   ```markdown
   ---
   name: security-engineer
   displayName: Security Engineer
   version: 1.0.0
   focusAreas: [sql-injection, xss]
   ---

   You are a security expert who reviews code for...
   ```

Persona loader detects format by file extension and parses accordingly.

**Consequences:**
- **Positive:**
  - Backward compatible with existing YAML personas
  - .ai.md format integrates with Personas plugin
  - Markdown prompts are more readable with syntax highlighting
  - Git diffs are cleaner for prompt changes

- **Negative:**
  - Two formats to maintain
  - Parsing logic is more complex
  - Users may be confused about which format to use

**Alternatives Considered:**
1. **YAML only**: Doesn't integrate with Personas plugin
2. **.ai.md only**: Breaks existing personas, requires migration
3. **JSON**: Less human-friendly than YAML/Markdown

---

## ADR-008: Finding Severity Levels with Confidence Scores

**Status:** Accepted

**Context:**
Code review findings need to be prioritized and triaged. Not all issues are equally important, and LLM-generated findings have varying reliability. Developers need signals for what to fix first.

**Decision:**
We use a five-level severity system:
- **Critical**: Security vulnerabilities, data loss risks
- **High**: Performance issues, major bugs
- **Medium**: Code quality, minor bugs
- **Low**: Style issues, suggestions
- **Info**: Informational notices

Each finding also has a confidence score (0-1) indicating LLM certainty. Deduplication preserves highest severity and averages confidence.

**Consequences:**
- **Positive:**
  - Clear prioritization for developers
  - Severity + confidence enables filtering (e.g., "high confidence critical only")
  - Multi-persona findings boost confidence through consensus
  - Aligns with industry standards (CVSS, etc.)

- **Negative:**
  - LLMs may mis-classify severity
  - Confidence scores are subjective (model-dependent)
  - No automatic remediation based on severity

**Alternatives Considered:**
1. **Three levels (high/medium/low)**: Too coarse-grained
2. **Numeric scores (1-10)**: Less intuitive than named levels
3. **Category-based (security/performance/style)**: Orthogonal to severity

---

## ADR-009: Parallel Persona Execution by Default

**Status:** Accepted

**Context:**
Review time is a critical user experience factor. Running 5 personas sequentially takes 5x longer than a single persona. Users want fast feedback, especially in CI/CD pipelines.

**Decision:**
We execute persona reviews in parallel by default:
```typescript
const promises = personas.map(p => runPersonaReview(p, input, reviewer));
const results = await Promise.all(promises);
```

Users can opt into sequential execution with `--no-parallel` flag if needed (e.g., rate limit constraints).

**Consequences:**
- **Positive:**
  - Review time is O(1) instead of O(n) for n personas
  - Quick mode completes in <10 seconds for typical PRs
  - Better resource utilization on multi-core systems
  - Improved developer experience

- **Negative:**
  - Higher peak memory usage (multiple concurrent LLM calls)
  - Potential rate limiting issues with LLM providers
  - Harder to debug (interleaved logs)

**Alternatives Considered:**
1. **Sequential only**: Too slow for practical use
2. **Parallel with rate limiting queue**: More complex, deferred to future
3. **Batching (groups of N)**: Adds complexity without clear benefit

---

## ADR-010: TypeScript with Strict Mode and Runtime Validation

**Status:** Accepted

**Context:**
Code review tools must be reliable and maintainable. Type errors can lead to incorrect findings, crashes, or security issues. We needed strong type safety but also runtime validation for external inputs (config, persona files, LLM responses).

**Decision:**
We use TypeScript with strict mode enabled:
```json
{
  "compilerOptions": {
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noImplicitAny": true,
    "strictNullChecks": true
  }
}
```

Plus runtime validation:
- Zod/type assertions for config parsing
- Custom validators for persona structure
- JSON schema validation for LLM responses
- Error codes for all failure modes

**Consequences:**
- **Positive:**
  - Compile-time safety catches bugs early
  - Self-documenting code with type annotations
  - Better IDE support and autocomplete
  - Runtime validation prevents crashes from bad inputs

- **Negative:**
  - More verbose code (explicit types required)
  - Learning curve for contributors
  - Build step required (not pure JavaScript)

**Alternatives Considered:**
1. **JavaScript only**: Less type safety, more runtime errors
2. **TypeScript without strict mode**: Weaker guarantees
3. **Runtime validation only (Zod, etc.)**: No compile-time checks

---

## ADR-011: Structured Error Handling with Error Codes

**Status:** Accepted

**Context:**
Errors can occur at many levels (config, persona loading, file scanning, LLM API calls, cost tracking). Users need clear error messages to debug issues. We need to differentiate recoverable vs non-recoverable errors.

**Decision:**
We use a structured error system with error codes:
- CONFIG_1xxx: Configuration errors
- PERSONA_2xxx: Persona loading errors
- FILE_SCANNER_3xxx: File scanning errors
- REVIEW_4xxx: Review engine errors
- ANTHROPIC_5xxx: Anthropic API errors
- VERTEXAI_6xxx: Vertex AI errors
- COST_SINK_7xxx: Cost tracking errors

Each error includes:
```typescript
class ConfigError extends Error {
  constructor(
    public code: string,
    message: string,
    public details?: unknown
  ) {
    super(message);
    this.name = 'ConfigError';
  }
}
```

**Consequences:**
- **Positive:**
  - Clear error categorization for users and logs
  - Programmatic error handling (switch on code)
  - Detailed error messages with context
  - Easier to document and troubleshoot

- **Negative:**
  - More boilerplate for error creation
  - Need to maintain error code registry
  - Risk of code collisions if not coordinated

**Alternatives Considered:**
1. **Generic Error only**: Less information for debugging
2. **HTTP-style codes (404, 500)**: Confusing for non-HTTP context
3. **Enum-based error types**: Less flexible than string codes

---

## ADR-012: No Auto-Fix in Initial Release (Manual Review Required)

**Status:** Accepted

**Context:**
LLMs can suggest code fixes, but automatically applying them is risky:
- Fixes may be incorrect or context-inappropriate
- Breaking changes could be introduced
- Users expect to review changes before committing

**Decision:**
We include `suggestedFix` in findings but do not auto-apply them. Users must:
1. Review suggested fixes manually
2. Apply fixes themselves
3. Test changes before committing

Future enhancement: Interactive mode with user confirmation before applying.

**Consequences:**
- **Positive:**
  - No risk of breaking code automatically
  - Users maintain control over changes
  - Avoids trust issues with LLM-generated code

- **Negative:**
  - Slower workflow (manual copy-paste)
  - Suggested fixes may go unused
  - Less automation value

**Alternatives Considered:**
1. **Auto-fix with confirmation**: Deferred to future release
2. **Auto-fix for low-risk changes only**: Hard to define "low-risk"
3. **No suggested fixes**: Misses opportunity to help users

---

## ADR-013: JSON Output Format for LLM Findings Parsing

**Status:** Accepted

**Context:**
LLM responses need to be parsed into structured Finding objects. LLMs can generate various formats (plain text, Markdown, JSON, XML). We need reliable, unambiguous parsing.

**Decision:**
We require LLMs to return findings as JSON arrays:
```json
[
  {
    "severity": "high",
    "file": "example.ts",
    "line": 42,
    "title": "SQL injection vulnerability",
    "description": "User input not sanitized...",
    "confidence": 0.9
  }
]
```

Prompt explicitly requests JSON format with example. Parser extracts JSON from markdown code blocks if present.

**Consequences:**
- **Positive:**
  - Reliable parsing (JSON is well-defined)
  - LLMs are good at generating JSON
  - Easy to validate with JSON schema
  - Extensible (add new fields without breaking parser)

- **Negative:**
  - LLMs may wrap JSON in markdown (requires extraction)
  - Malformed JSON causes parsing failures
  - No support for complex nested structures

**Alternatives Considered:**
1. **Plain text with regex parsing**: Fragile, hard to maintain
2. **XML format**: More verbose, less LLM-friendly
3. **Markdown with structured sections**: Harder to parse reliably

---

## ADR-014: Persona Search Paths with Placeholder Expansion

**Status:** Accepted

**Context:**
Personas can come from multiple sources:
- User-level (~/.wayfinder/personas)
- Project-level (.wayfinder/personas)
- Company plugins
- Core Wayfinder personas
- Shared Personas plugin library

We needed a flexible way to configure search paths without hard-coding locations.

**Decision:**
We use placeholder-based search paths:
```yaml
personaPaths:
  - ~/.wayfinder/personas       # User overrides
  - .wayfinder/personas          # Project-specific
  - '{company}/personas'         # Company plugin
  - '{personas}'                 # Shared Personas plugin
  - '{core}/personas'            # Core Wayfinder
```

Placeholders are resolved at runtime:
- `~` → User home directory
- `{company}` → Company plugin path (if available)
- `{personas}` → Personas plugin path (if installed)
- `{core}` → Core Wayfinder path

Later paths override earlier ones (user overrides core).

**Consequences:**
- **Positive:**
  - Flexible persona organization
  - Easy to override built-in personas
  - Integrates with plugin ecosystem
  - Declarative configuration

- **Negative:**
  - Placeholder expansion adds complexity
  - Error-prone if paths don't exist (silent skip)
  - Hard to debug missing personas

**Alternatives Considered:**
1. **Fixed paths only**: Inflexible for plugin ecosystem
2. **Environment variables**: Less declarative, harder to manage
3. **Plugin registry**: Overcomplicated for current needs

---

## ADR-015: Accepting 30-50% False Positive Rate for Low Confidence Findings

**Status:** Accepted

**Context:**
LLM-based code review is probabilistic, not deterministic. Perfect accuracy is impossible. We needed to set expectations for false positive rates and decide how to handle them.

**Decision:**
We accept a 30-50% false positive rate for low confidence findings (<0.7 confidence). This is mitigated by:
- Severity classification (false positives are usually LOW severity)
- Confidence scores (users can filter low confidence)
- Deduplication (false positives less likely to be multi-persona)
- Clear finding descriptions (users can quickly dismiss)

For high confidence findings (>0.85), we target <10% false positive rate.

**Consequences:**
- **Positive:**
  - Realistic expectations set with users
  - More findings caught (high recall)
  - Users can filter based on confidence threshold
  - Acceptable trade-off for automated reviews

- **Negative:**
  - Users may lose trust if too many false positives
  - Review fatigue from dismissing false positives
  - Need to tune confidence thresholds over time

**Alternatives Considered:**
1. **Zero false positives**: Impossible with LLMs, would miss real issues
2. **Higher threshold (>70%)**: Misses too many real issues
3. **No confidence scores**: Users can't filter, all or nothing

---

## Summary of Key Architectural Principles

1. **Modularity**: Clear separation of concerns (config, persona loading, review engine, LLM clients, formatters)
2. **Extensibility**: Plugin architecture for personas, LLM providers, cost sinks, formatters
3. **Reliability**: Structured errors, graceful degradation, comprehensive testing
4. **Performance**: Parallel execution, git-aware scanning, deduplication
5. **Usability**: Sensible defaults, hierarchical config, clear output formatting
6. **Cost-Awareness**: Token tracking, cost sinks, configurable review modes
7. **Type Safety**: TypeScript strict mode, runtime validation
8. **Flexibility**: Multiple LLM providers, multiple output formats, configurable options

These architectural decisions collectively enable the Multi-Persona Review plugin to be a reliable, performant, and extensible code review system suitable for production use in diverse development environments.
