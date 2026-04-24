# Multi-Persona Review Plugin - Architecture

## System Overview

The Multi-Persona Review plugin is structured as a modular, pipeline-based architecture that orchestrates code review across multiple AI personas, aggregates findings, and tracks costs.

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI / API Entry                         │
│                        (cli.ts, index.ts)                       │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Configuration Layer                          │
│  ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ Config Loader  │  │ Persona Loader  │  │ File Scanner    │  │
│  │ (.yml parsing) │  │ (.yaml/.ai.md)  │  │ (git aware)     │  │
│  └────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Review Engine Core                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Review Orchestration (review-engine.ts)                  │  │
│  │  - Prepare context (scan files)                          │  │
│  │  - Run persona reviews (parallel/sequential)             │  │
│  │  - Aggregate results                                     │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      LLM Client Layer                            │
│  ┌─────────────────────┐  ┌──────────────────┐  ┌────────────┐  │
│  │ Anthropic Client    │  │ VertexAI Gemini  │  │ VertexAI   │  │
│  │ (Claude Direct API) │  │ Client           │  │ Claude     │  │
│  └─────────────────────┘  └──────────────────┘  └────────────┘  │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Post-Processing Layer                          │
│  ┌────────────────┐  ┌─────────────────┐  ┌─────────────────┐  │
│  │ Deduplication  │  │ Cost Tracking   │  │ Formatters      │  │
│  │ (similarity)   │  │ (sinks)         │  │ (text/json/gh)  │  │
│  └────────────────┘  └─────────────────┘  └─────────────────┘  │
└─────────────────────────────────────────────────────────────────┘
```

## Component Architecture

### 1. Configuration Layer

#### Config Loader (`config-loader.ts`)
**Responsibilities:**
- Load and parse Wayfinder YAML configuration files
- Merge user config with defaults
- Validate configuration structure
- Resolve persona search paths with placeholder expansion

**Key Functions:**
```typescript
loadConfig(configPath: string): Promise<WayfinderConfig>
loadCrossCheckConfig(configPath?: string): Promise<CrossCheckConfig>
mergeWithDefaults(userConfig?: CrossCheckConfig): CrossCheckConfig
validateConfig(config: CrossCheckConfig): void
```

**Error Codes:** CONFIG_1xxx

#### Persona Loader (`persona-loader.ts`)
**Responsibilities:**
- Load persona definitions from YAML and .ai.md files
- Validate persona structure (name, version, focusAreas, prompt)
- Support hierarchical persona search paths
- Handle both legacy (.yaml) and new (.ai.md) formats

**Key Functions:**
```typescript
loadPersonaFile(personaPath: string): Promise<Persona>
loadPersonasFromDir(dirPath: string): Promise<Map<string, Persona>>
loadPersonas(searchPaths: string[]): Promise<Map<string, Persona>>
loadSpecificPersonas(names: string[], searchPaths: string[]): Promise<Persona[]>
validatePersona(persona: unknown, filePath: string): asserts persona is Persona
resolvePersonaPaths(paths: string[], cwd: string): string[]
```

**Persona Format Support:**
1. **Legacy YAML** (`security-engineer.yaml`):
   ```yaml
   name: security-engineer
   displayName: Security Engineer
   version: 1.0.0
   description: Security expert
   focusAreas: [sql-injection, xss, auth]
   prompt: "You are a security expert..."
   ```

2. **New .ai.md** (`security-engineer.ai.md`):
   ```markdown
   ---
   name: security-engineer
   displayName: Security Engineer
   version: 1.0.0
   description: Security expert
   focusAreas: [sql-injection, xss, auth]
   ---

   You are a security expert who reviews code for...
   ```

**Error Codes:** PERSONA_2xxx

#### File Scanner (`file-scanner.ts`)
**Responsibilities:**
- Scan files based on glob patterns
- Support full, diff, and changed modes
- Extract git diffs with context lines
- Skip binary files and respect size limits
- Map diff line numbers to original file lines

**Key Functions:**
```typescript
scanFiles(patterns: string[], cwd: string, options: ScanOptions): Promise<FileContent[]>
scanFileFull(filePath: string, cwd: string): Promise<FileContent>
scanFileDiff(filePath: string, cwd: string, options: ScanOptions): Promise<FileContent>
getGitDiff(filePath: string, ref: string, contextLines: number): Promise<string>
parseDiffLineMapping(diff: string): LineMapping
getChangedFiles(cwd: string): Promise<string[]>
isBinaryFile(filePath: string): Promise<boolean>
shouldExcludeFile(filePath: string, patterns: string[]): boolean
```

**Error Codes:** FILE_SCANNER_3xxx

### 2. Review Engine Core

#### Review Engine (`review-engine.ts`)
**Responsibilities:**
- Orchestrate the entire review process
- Prepare context by scanning files
- Run persona reviews in parallel or sequentially
- Aggregate findings from all personas
- Calculate summary statistics and costs
- Handle partial failures gracefully

**Key Functions:**
```typescript
reviewFiles(config: ReviewConfig, cwd: string, reviewer: ReviewerFunction): Promise<ReviewResult>
prepareContext(config: ReviewConfig, cwd: string): Promise<ReviewContext>
runPersonaReview(persona: string, input: PersonaReviewInput, reviewer: ReviewerFunction): Promise<PersonaReviewOutput>
aggregateResults(config: ReviewConfig, context: ReviewContext, results: PersonaReviewOutput[]): ReviewResult
createSummary(filesReviewed: number, findings: Finding[]): ReviewSummary
groupFindingsByFile(findings: Finding[]): Record<string, Finding[]>
validateReviewConfig(config: ReviewConfig): void
```

**Execution Flow:**
```
1. validateReviewConfig()
   ↓
2. prepareContext()
   - Determine scan mode (full/diff/changed)
   - Scan files using file-scanner
   - Generate session ID and timestamp
   ↓
3. Run persona reviews (parallel by default)
   - For each persona: runPersonaReview()
   - Call reviewer function (Anthropic/VertexAI Gemini/VertexAI Claude)
   - Handle individual failures gracefully
   ↓
4. aggregateResults()
   - Collect all findings
   - Apply deduplication (if enabled)
   - Calculate total costs
   - Group findings by file
   - Generate summary statistics
   ↓
5. Record costs (if cost sink configured)
   ↓
6. Return ReviewResult
```

**Error Codes:** REVIEW_4xxx

### 3. LLM Client Layer

#### Anthropic Client (`anthropic-client.ts`)
**Responsibilities:**
- Communicate with Claude API
- Format review prompts with persona system messages
- Parse JSON findings from Claude responses
- Calculate costs based on token usage
- Handle rate limiting and API errors

**Key Functions:**
```typescript
createAnthropicReviewer(config: AnthropicClientConfig): ReviewerFunction
buildReviewPrompt(input: PersonaReviewInput): string
parseClaudeResponse(response: string, personaName: string): Finding[]
calculateCost(model: string, inputTokens: number, outputTokens: number): number
formatFileContent(input: PersonaReviewInput): string
```

**Review Prompt Structure:**
```
SYSTEM MESSAGE: {persona.prompt}

USER MESSAGE:
  Mode instructions (quick/thorough/custom)
  Focus areas: {persona.focusAreas}
  Output format: JSON array of findings
  File contents (markdown code blocks)
```

**Response Parsing:**
- Extract JSON from markdown code blocks
- Map raw findings to Finding type
- Generate unique finding IDs
- Assign persona attribution
- Handle malformed responses gracefully

**Cost Calculation:**
```typescript
const MODEL_PRICING = {
  'claude-3-5-sonnet-20241022': {
    input: $3.0 / 1M tokens,
    output: $15.0 / 1M tokens
  },
  'claude-3-5-haiku-20241022': {
    input: $0.8 / 1M tokens,
    output: $4.0 / 1M tokens
  },
  'claude-3-opus-20240229': {
    input: $15.0 / 1M tokens,
    output: $75.0 / 1M tokens
  }
}
```

**Error Codes:** ANTHROPIC_5xxx

#### Vertex AI Client (`vertex-ai-client.ts`)
**Responsibilities:**
- Communicate with Gemini API via Vertex AI
- Similar functionality to Anthropic client
- Support for Gemini-specific features

**Error Codes:** VERTEXAI_6xxx

#### Vertex AI Claude Client (`vertex-ai-claude-client.ts`)
**Responsibilities:**
- Communicate with Claude API via VertexAI Anthropic publisher endpoint
- Unified GCP billing and authentication
- Support for latest Claude models (Sonnet 4.5, Haiku 4.5, Opus 4.6)
- Region-specific model availability (us-east5)

**Authentication:**
- Uses Google Application Default Credentials (ADC)
- Supports service accounts and gcloud auth
- No separate API keys required (unified with GCP)

**Endpoint Format:**
```
https://{location}-aiplatform.googleapis.com/v1/projects/{projectId}/locations/{location}/publishers/anthropic/models/{model}:streamRawPredict
```

**Supported Models:**
- `claude-sonnet-4-5@20250929` - Best balance ($3/$15 per million tokens)
- `claude-haiku-4-5@20251001` - Most cost-effective ($0.80/$4 per million tokens)
- `claude-opus-4-6@20260205` - Highest quality ($5/$25 per million tokens)

**Auto-Detection:**
- CLI and Wayfinder gate automatically detect Claude models
- If `VERTEX_MODEL` contains "claude" → uses vertexai-claude provider
- Defaults to us-east5 region (where Claude models are available)

**Benefits:**
- Unified GCP billing with other services
- Committed use discounts for high-volume usage
- Better integration with GCP infrastructure (monitoring, logging)
- Same quality as Anthropic direct API

**Configuration:**
```typescript
export interface VertexAIClaudeClientConfig {
  projectId: string;          // GCP project ID
  location?: string;          // Default: 'us-east5'
  model?: string;             // Default: 'claude-sonnet-4-5@20250929'
  temperature?: number;
  maxTokens?: number;
  timeout?: number;
}
```

**Key Functions:**
```typescript
createVertexAIClaudeReviewer(config: VertexAIClaudeClientConfig): PersonaReviewer
```

**Error Codes:** VERTEXAI_6xxx (shared with Gemini client)

**Implementation Details:**

The VertexAI Claude client implements the same `ReviewerFunction` interface as Anthropic client but uses Google Cloud's infrastructure:

```typescript
// Authentication via Google ADC
import { GoogleAuth } from 'google-auth-library';

const auth = new GoogleAuth({
  scopes: ['https://www.googleapis.com/auth/cloud-platform'],
  keyFilename: config.credentialsPath, // Optional service account
});

// Get access token
const client = await auth.getClient();
const accessToken = await client.getAccessToken();

// API call to VertexAI Anthropic endpoint
const response = await fetch(
  `https://${location}-aiplatform.googleapis.com/v1/projects/${projectId}/locations/${location}/publishers/anthropic/models/${model}:streamRawPredict`,
  {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${accessToken.token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      anthropic_version: 'vertex-2023-10-16',
      messages: [...],
      system: personaPrompt,
      max_tokens: 4096,
      temperature: 0.0,
    }),
  }
);
```

**Prompt Caching Support:**

VertexAI Claude supports Anthropic's prompt caching with 5-minute TTL:

```typescript
{
  system: [
    {
      type: 'text',
      text: personaPrompt,  // Cached if ≥1024 tokens
      cache_control: { type: 'ephemeral' }
    }
  ]
}
```

**Cost Calculation:**

```typescript
// VertexAI Claude pricing (us-east5)
const VERTEX_CLAUDE_PRICING = {
  'claude-sonnet-4-5@20250929': {
    input: 3.0 / 1_000_000,           // $3 per 1M tokens
    output: 15.0 / 1_000_000,         // $15 per 1M tokens
    cacheWrite: 3.75 / 1_000_000,     // $3.75 per 1M tokens (25% markup)
    cacheRead: 0.30 / 1_000_000,      // $0.30 per 1M tokens (90% discount)
  },
  'claude-haiku-4-5@20251001': {
    input: 0.80 / 1_000_000,
    output: 4.0 / 1_000_000,
    cacheWrite: 1.0 / 1_000_000,
    cacheRead: 0.08 / 1_000_000,
  },
};
```

**Regional Availability:**

Claude models on VertexAI are available in limited regions:
- **us-east5** - Primary region for Claude models
- Other regions may be added by Google over time

**Error Handling:**

```typescript
// VertexAI-specific error codes
VERTEXAI_6001: 'Authentication failed (invalid ADC or service account)',
VERTEXAI_6002: 'Project ID not found or invalid',
VERTEXAI_6003: 'Region does not support Claude models (try us-east5)',
VERTEXAI_6004: 'Model not found or not enabled in project',
VERTEXAI_6005: 'Quota exceeded (check GCP quotas)',
```

#### Sub-Agent Orchestrator (`sub-agent-orchestrator.ts`)

**Responsibilities:**
- Orchestrate multiple sub-agent instances for parallel persona execution
- Manage prompt caching strategy (ephemeral 5-min TTL)
- Aggregate results from all sub-agents
- Track cache metrics (hit rate, cost savings)
- Handle individual sub-agent failures gracefully

**Why Sub-Agents?**

Traditional approach: Single API call with all personas in one prompt
- ❌ Large prompt size (reduces cache efficiency)
- ❌ Sequential processing (slower)
- ❌ One persona failure blocks entire review

Sub-agent approach: Dedicated API call per persona
- ✅ Smaller prompts per sub-agent (better caching)
- ✅ Parallel execution (3x faster)
- ✅ Isolated failures (one persona fails, others continue)
- ✅ Per-persona cost attribution

**Architecture:**

```typescript
interface SubAgent {
  personaName: string;
  reviewer: ReviewerFunction;
  cacheStrategy: 'ephemeral' | 'none';
}

interface SubAgentResult {
  persona: string;
  findings: Finding[];
  cost: CostInfo;
  cacheMetrics?: CacheMetrics;
  error?: Error;
}

class SubAgentOrchestrator {
  private subAgents: Map<string, SubAgent>;

  async executeReviews(
    personas: Persona[],
    files: FileContent[],
    options: ReviewOptions
  ): Promise<SubAgentResult[]> {
    // Create sub-agents for each persona
    const subAgents = personas.map(p => this.createSubAgent(p, options));

    // Execute in parallel
    const results = await Promise.all(
      subAgents.map(agent => this.executeSubAgent(agent, files))
    );

    return results;
  }
}
```

**Caching Strategy:**

```typescript
// Ephemeral caching (5-min TTL)
// Persona prompts are cached automatically if ≥1024 tokens
const systemPrompt = {
  type: 'text',
  text: persona.prompt,  // Must be ≥1024 tokens for cache eligibility
  cache_control: { type: 'ephemeral' }
};

// Cache hit rate calculation
interface CacheMetrics {
  cacheCreationTokens: number;   // First review: persona written to cache
  cacheReadTokens: number;       // Subsequent reviews: read from cache
  inputTokens: number;           // Non-cached input tokens
  totalTokens: number;
  hitRate: number;               // cacheReadTokens / (cacheReadTokens + cacheCreationTokens)
  costSavings: number;           // Dollar amount saved vs no caching
}
```

**Auto-TTL Selection:**

The orchestrator automatically selects cache TTL based on expected review count:

```typescript
function selectCacheTTL(expectedReviews: number): 'ephemeral' | 'none' {
  // Break-even: 2-3 reviews within TTL window
  // Ephemeral (5 min) beneficial if ≥4 reviews expected
  if (expectedReviews >= 4) {
    return 'ephemeral';  // Worth 25% cache write cost
  }
  return 'none';  // Not worth cache write overhead
}

// Detection sources (in precedence order):
// 1. Explicit --review-count CLI flag
// 2. Environment variable MULTI_PERSONA_REVIEW_COUNT
// 3. Batch mode env var MULTI_PERSONA_BATCH_MODE=true
// 4. Git detection (CI/CD = many reviews, local = few reviews)
// 5. Default: Conservative (no caching)
```

**Error Handling:**

```typescript
async executeSubAgent(agent: SubAgent, files: FileContent[]): Promise<SubAgentResult> {
  try {
    const result = await agent.reviewer({
      persona: agent.persona,
      files,
      mode: this.options.mode,
    });
    return result;
  } catch (error) {
    // Individual sub-agent failure doesn't block review
    console.error(`Sub-agent ${agent.personaName} failed:`, error);
    return {
      persona: agent.personaName,
      findings: [],
      cost: { cost: 0, inputTokens: 0, outputTokens: 0 },
      error,
    };
  }
}
```

**Performance Metrics:**

Benchmarks (3 personas, 5 files, ~10KB code):
- **Sequential**: ~45 seconds total
- **Parallel (sub-agents)**: ~15 seconds total (**3x faster**)
- **Cache hit rate**: 88-95% (after first review)
- **Cost savings**: 86% reduction with caching enabled

### 4. Agency-Agents Layer

#### Vote Aggregation (`review-engine.ts`)
**Responsibilities:**
- Aggregate voting decisions across personas
- Apply tier-weighted voting (Tier 1=3x, Tier 2=1x, Tier 3=0.5x)
- Calculate GO/NO-GO consensus based on threshold
- Merge findings from multiple personas reporting same issue
- Preserve vote metadata for transparency

**Key Functions:**
```typescript
getVoteWeight(tier: 1 | 2 | 3 | undefined): number
aggregateVotes(findings: Finding[], personas: Persona[], threshold: number): Finding[]
```

**Vote Aggregation Algorithm:**
```typescript
function aggregateVotes(findings: Finding[], personas: Persona[], threshold: number): Finding[] {
  // 1. Group findings by unique identifier (file + line + title)
  // 2. For each group:
  //    - Calculate weighted vote sum:
  //      - GO votes: sum of weights for personas voting GO
  //      - NO-GO votes: sum of weights for personas voting NO-GO
  //      - Total weight: sum of all persona weights
  //    - Determine final decision:
  //      - GO if (GO votes / total weight) > threshold
  //      - NO-GO otherwise
  //    - Store vote metadata for transparency
  // 3. Return aggregated findings with final decisions
}
```

**Tier Weighting Rationale:**
- **Tier 1 (3x weight)**: Senior experts, security leads - critical decisions
- **Tier 2 (1x weight)**: Standard reviewers - balanced perspective
- **Tier 3 (0.5x weight)**: Junior reviewers, experimental personas - learning feedback

**Vote Metadata:**
```typescript
metadata: {
  voteAggregation: {
    goVotes: number;      // Weighted GO vote sum
    noGoVotes: number;    // Weighted NO-GO vote sum
    totalWeight: number;  // Sum of all persona weights
    goRatio: number;      // GO votes / total weight
    threshold: number;    // Configured threshold
  }
}
```

#### Confidence Filtering (`review-engine.ts`)
**Responsibilities:**
- Filter findings based on minimum confidence threshold
- Default to 1.0 confidence if not specified by persona
- Enable users to reduce noise by filtering low-confidence findings

**Implementation:**
```typescript
const minConfidence = config.options?.minConfidence;
if (minConfidence !== undefined && minConfidence > 0) {
  allFindings = allFindings.filter(f => {
    const confidence = f.confidence ?? 1.0;
    return confidence >= minConfidence;
  });
}
```

**Use Cases:**
- **High confidence mode (0.9)**: Only show findings persona is very certain about
- **Balanced mode (0.7)**: Filter out low-confidence findings (default for many teams)
- **No filtering (0.0)**: Show all findings regardless of confidence

#### Lateral Thinking Protocol
**Responsibilities:**
- Require personas to propose 3 alternative approaches for each finding
- Encourage creative problem-solving and diverse solutions
- Display alternatives in all formatters

**Prompt Integration:**
```markdown
## Agency-Agents Protocol
For each finding, you must:
2. **Lateral Thinking**: Propose 3 alternative approaches or solutions
   - Think outside the box
   - Consider different perspectives
   - Suggest creative solutions
```

**Output Format:**
```json
{
  "alternatives": [
    "Alternative approach 1",
    "Alternative approach 2",
    "Alternative approach 3"
  ]
}
```

#### Expertise Boundary Detection
**Responsibilities:**
- Personas flag findings outside their expertise area
- Enable routing to appropriate specialized persona
- Improve finding quality and reduce false positives

**Prompt Integration:**
```markdown
## Agency-Agents Protocol
For each finding, you must:
3. **Scope Detection**: Flag findings outside your expertise
   - Set "outOfScope": true if the finding requires expertise you don't have
   - Be honest about your limitations
```

**Example Scenario:**
- Security persona finds performance issue → flags `outOfScope: true`
- System can route to performance persona for expert review
- Reduces false positives from personas reviewing outside their domain

### 5. Post-Processing Layer

#### Deduplication (`deduplication.ts`)
**Responsibilities:**
- Identify similar findings across personas
- Merge similar findings to reduce noise
- Preserve highest severity level
- Combine persona attributions
- Average confidence scores

**Algorithm:**
```typescript
function deduplicateFindings(findings: Finding[], threshold: number): {
  findings: Finding[];
  duplicatesRemoved: number;
} {
  // 1. Group findings by file
  // 2. Within each file, group by line proximity (±5 lines)
  // 3. Calculate similarity scores for titles/descriptions
  // 4. Merge groups with similarity >= threshold
  // 5. For merged groups:
  //    - Take highest severity
  //    - Combine personas array
  //    - Average confidence scores
  //    - Merge metadata
}
```

**Similarity Calculation:**
- Levenshtein distance for titles
- Token overlap for descriptions
- File and line number proximity
- Configurable threshold (default 0.8)

**Key Functions:**
```typescript
deduplicateFindings(findings: Finding[], threshold: number): DeduplicationResult
areSimilarFindings(f1: Finding, f2: Finding, threshold: number): boolean
mergeFindings(findings: Finding[]): Finding
```

#### Cost Tracking (`cost-sink.ts`, `cost-sinks/`)
**Responsibilities:**
- Abstract interface for cost tracking backends
- Record costs with metadata (mode, files, findings)
- Support multiple sink implementations
- Handle sink failures gracefully (log but don't block)

**Cost Sink Interface:**
```typescript
interface CostSink {
  record(cost: CostInfo, metadata: CostMetadata): Promise<void>;
  flush?(): Promise<void>;
}
```

**Implementations:**
1. **StdoutCostSink**: Print to console
2. **FileCostSink**: Append to JSON log file
3. **GCPCostSink**: Export to Google Cloud Monitoring
4. **Future**: AWS CloudWatch, Datadog, Webhook

**GCP Cost Sink Architecture:**
```typescript
// Uses @google-cloud/monitoring to export metrics
// Metric: custom.googleapis.com/multi_persona_review/cost
// Labels: mode, model, persona
// Value: cost in USD
// Metadata: files_reviewed, total_findings, timestamp
```

**Error Codes:** COST_SINK_7xxx

#### Formatters (`formatters/`)
**Responsibilities:**
- Transform ReviewResult into different output formats
- Support human-readable, machine-parseable, and CI/CD formats
- Apply color coding and formatting conventions

**Implementations:**
1. **TextFormatter** (`text-formatter.ts`):
   - Terminal output with chalk colors
   - Severity-based color coding (red=critical, yellow=medium, etc.)
   - File grouping and line number references
   - Summary statistics table

2. **JSONFormatter** (`json-formatter.ts`):
   - Structured JSON output
   - Full ReviewResult serialization
   - Machine-parseable for downstream tools

3. **GitHubFormatter** (`github-formatter.ts`):
   - GitHub Actions annotation format
   - Maps findings to `::warning` and `::error` annotations
   - Clickable file:line references in PR

**Key Functions:**
```typescript
formatReviewResult(result: ReviewResult, options?: TextFormatOptions): string
formatReviewResultJSON(result: ReviewResult): string
formatReviewResultGitHub(result: ReviewResult): string
```

## Data Flow

### End-to-End Review Flow

```
1. User Input
   ├─ CLI: multi-persona-review quick src/
   └─ API: reviewFiles(config, cwd, reviewer)

2. Configuration Resolution
   ├─ Load .wayfinder/config.yml
   ├─ Merge with defaults
   ├─ Resolve persona search paths
   └─ Load specified personas

3. File Scanning
   ├─ Expand glob patterns
   ├─ Filter by mode (full/diff/changed)
   ├─ Skip binaries and large files
   └─ Extract git diffs (if diff mode)

4. Persona Reviews (Parallel)
   ├─ Persona A: security-engineer
   │  ├─ Format prompt with system message + files
   │  ├─ Call Claude API
   │  ├─ Parse findings from response
   │  └─ Calculate cost
   ├─ Persona B: performance-reviewer
   │  └─ ...
   └─ Persona C: code-quality-reviewer
      └─ ...

5. Result Aggregation
   ├─ Collect all findings
   ├─ Deduplicate similar findings (80% threshold)
   ├─ **Agency-Agents: Aggregate votes** (tier-weighted)
   │  ├─ Group findings by file:line:title
   │  ├─ Calculate weighted vote sums
   │  └─ Determine GO/NO-GO consensus
   ├─ **Agency-Agents: Filter by confidence** (if --min-confidence set)
   ├─ Sum costs across personas
   ├─ Group findings by file
   └─ Calculate summary statistics

6. Cost Recording
   ├─ Extract cost metadata
   ├─ Send to configured sink (stdout/file/gcp)
   └─ Log errors if sink fails

7. Output Formatting
   ├─ Apply formatter (text/json/github)
   └─ Return/print formatted result
```

### Persona Review Execution

```typescript
// Parallel execution (default)
const reviewPromises = personas.map(persona => {
  const input: PersonaReviewInput = {
    persona,
    files: context.files,
    mode: config.mode,
    options: config.options
  };
  return runPersonaReview(persona.name, input, reviewer);
});
const results = await Promise.all(reviewPromises);

// Sequential execution (--no-parallel)
for (const persona of personas) {
  const result = await runPersonaReview(persona.name, input, reviewer);
  results.push(result);
}
```

## Extensibility Points

### 1. Custom Personas
- Drop `.yaml` or `.ai.md` files in persona search paths
- Persona loader auto-discovers and validates
- Override built-in personas via user/project paths

### 2. Custom LLM Providers
- Implement `ReviewerFunction` interface:
  ```typescript
  type ReviewerFunction = (input: PersonaReviewInput) => Promise<PersonaReviewOutput>
  ```
- Format prompts for provider's API
- Parse provider-specific responses
- Calculate costs based on provider pricing

### 3. Custom Cost Sinks
- Implement `CostSink` interface
- Register in `createCostSink()` factory
- Configure via YAML: `costTracking: { type: 'custom', config: {...} }`

### 4. Custom Formatters
- Implement formatter function: `(result: ReviewResult) => string`
- Add to formatters export in `index.ts`
- Use via `--format custom` CLI flag

## Error Handling Strategy

### Error Code Ranges
- **1xxx**: Configuration errors (non-recoverable)
- **2xxx**: Persona errors (non-recoverable)
- **3xxx**: File scanner errors (skip file, continue)
- **4xxx**: Review engine errors (fail fast)
- **5xxx**: Anthropic API errors (retry, then fail persona)
- **6xxx**: Vertex AI errors (retry, then fail persona)
- **7xxx**: Cost sink errors (log, continue)

### Graceful Degradation
```typescript
// Individual persona failures don't block review
try {
  const result = await reviewer(input);
  return result;
} catch (error) {
  return {
    persona: persona.name,
    findings: [],
    cost: { persona: persona.name, cost: 0, inputTokens: 0, outputTokens: 0 },
    errors: [{
      code: 'REVIEWER_ERROR',
      message: `Persona ${persona.name} failed: ${error.message}`,
      persona: persona.name,
      details: error
    }]
  };
}

// Review succeeds if at least one persona completes
if (successfulResults.length === 0) {
  throw new ReviewEngineError('ALL_PERSONAS_FAILED', 'All personas failed');
}
```

## Performance Optimizations

### 1. Parallel Persona Execution
- Run personas concurrently by default
- Reduce review time from O(n) to O(1) for n personas
- CLI: `--no-parallel` flag to disable

### 2. File Scanning Optimizations
- Binary file detection (magic number checking)
- File size limits (default 1MB)
- Git diff mode reduces token usage by 80%+
- Exclude patterns (node_modules, dist, etc.)

### 3. Deduplication Optimizations
- Two-pass algorithm: group by file first, then similarity
- Early exit for dissimilar findings
- Configurable threshold for performance tuning

### 4. Cost Tracking Optimizations
- Async cost recording (don't block return)
- Batch writes to file/cloud sinks
- Optional flush on exit

## Testing Strategy

### Unit Tests
- Each module has isolated unit tests
- Mock external dependencies (LLM APIs, file system, git)
- Test error handling paths
- Validate type safety and assertions

### Integration Tests
- End-to-end review flows with mock reviewers
- Configuration loading and merging
- Persona search path resolution
- Deduplication effectiveness

### Scenario Tests (`tests/scenarios.test.ts`)
- Real-world code review scenarios:
  - SQL injection detection
  - N+1 query performance issues
  - Accessibility violations
  - Deduplication effectiveness
  - False positive handling
- Validate detection accuracy
- Measure noise reduction
- Test multi-persona convergence

### Performance Tests
- Review completion time benchmarks
- Token usage and cost validation
- Parallel vs sequential execution comparison

## Security Architecture

### API Key Handling
- Never log API keys
- Support environment variables
- Validate keys before making API calls
- Rotate keys via config updates

### Input Validation
- Validate file paths to prevent traversal
- Sanitize git diff parsing
- Validate persona YAML structure
- Limit file sizes and pattern expansion

### Output Sanitization
- Remove sensitive data from error messages
- Cost metadata doesn't include file contents
- Sanitize LLM responses before parsing

## Deployment Architecture

### NPM Package
```
@wayfinder/multi-persona-review
├─ dist/
│  ├─ index.js (ES modules)
│  ├─ index.d.ts (TypeScript types)
│  └─ cli.js (executable)
├─ package.json
└─ README.md
```

### Installation
```bash
# Global installation (for CLI)
npm install -g @wayfinder/multi-persona-review

# Project-local installation (for API)
npm install --save-dev @wayfinder/multi-persona-review
```

### Runtime Requirements
- Node.js >= 18.0.0
- Git >= 2.0 (for diff mode)
- Environment variables for API keys

## Monitoring and Observability

### Cost Tracking Metrics
- Total cost per review session
- Cost per persona
- Token usage (input/output)
- Files reviewed
- Findings count

### Performance Metrics
- Review duration (total, per-persona)
- File scanning time
- Deduplication time
- API latency

### Quality Metrics
- Finding distribution by severity
- Deduplication effectiveness (% reduction)
- Persona agreement (findings per persona)
- Error rates by code

### Logging
- Structured logs with severity levels
- Persona execution traces
- Error stack traces
- Cost sink operations

## Adversarial Deliberation

The deliberation subsystem adds opt-in multi-round debate between personas, mediated by a lead reviewer. See [ADR-003](docs/adr/003-adversarial-deliberation.md) for the full architecture decision.

### Components

- **`deliberation-types.ts`**: Type definitions — `DeliberationConfig`, `Tension`, `Challenge`, `ChallengeResponse`, `DeliberationMemo`, `DeliberationStats`
- **`deliberation-engine.ts`**: `DeliberationEngine` class — orchestrates rounds of challenge/response between personas
- **`personas/lead-reviewer.ai.md`**: Orchestrator persona (Opus model, tier 1) that identifies tensions and synthesizes memos
- **`personas/contrarian.ai.md`**: Devil's advocate persona that challenges consensus and severity inflation

### Data Flow (Deliberation Mode)

```
Phase 1: Independent Review (existing)
  └→ Each persona reviews in parallel (Promise.all)
     └→ Prompt caching: system prompt cached per persona

Phase 2: Deliberation (opt-in via ReviewOptions.deliberation.enabled)
  └→ Lead Reviewer compiles brief from all persona findings
  └→ Lead Reviewer identifies tensions (contradictions, trade-offs)
  └→ For each round (up to maxRounds):
       └→ Lead Reviewer formulates challenges for unresolved tensions
       └→ Challenges sent to persona agents in parallel
       └→ Personas respond with structured stances (support/oppose/revise/withdraw)
       └→ Tensions updated; resolved when no 'oppose' positions remain
       └→ Constraint check: timeout, token budget, convergence threshold

Phase 3: Synthesis
  └→ Lead Reviewer produces DeliberationMemo
  └→ Memo attached to ReviewResult.deliberationMemo
```

### Prompt Caching During Deliberation

Each deliberation round reuses the persona's existing conversation, appending new messages. Anthropic's prefix caching means:
- Round 1: System prompt (cache READ) + initial review (cache READ) + challenge (full price)
- Round 2+: All prior turns cached. Only new challenge is full price (~10% of Round 1 cost)

### Constraint System

| Constraint | Default | Purpose |
|-----------|---------|---------|
| `maxRounds` | 3 | Prevent circular arguments |
| `maxDeliberationTokens` | 50,000 | Cost control |
| `timeoutMs` | 120,000 ms | User experience |
| `convergenceThreshold` | 0.8 | Stop when 80% of tensions resolved |

### Error Handling

Deliberation uses graceful degradation throughout:
- Lead reviewer tension identification fails → proceed with no tensions (skip deliberation)
- Individual persona challenge fails → default 'support' response
- Memo synthesis fails → return basic memo with raw findings
- Individual persona agent not found → skip that challenge

## Future Architecture Considerations

### Caching Layer
- Cache persona results for unchanged files
- Invalidate on file content changes
- Store in `.wayfinder/cache/` directory
- Reduce API costs by 70%+ for incremental reviews

### Incremental Review
- Track git history to identify new/modified code
- Only review changed sections
- Preserve existing findings for unchanged code

### Distributed Execution
- Queue-based persona distribution
- Support for review workers (horizontal scaling)
- Cloud function deployment (AWS Lambda, GCP Cloud Functions)

### Real-time Review
- File watcher for IDE integration
- WebSocket-based streaming results
- Incremental finding updates
