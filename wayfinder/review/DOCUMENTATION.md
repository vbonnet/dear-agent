# Multi-Persona Review Documentation

**Version**: 0.1.0-alpha
**Status**: Production Ready
**Last Updated**: 2025-11-23

---

## Table of Contents

1. [Overview](#overview)
2. [Installation](#installation)
3. [Quick Start](#quick-start)
4. [Configuration](#configuration)
5. [CLI Reference](#cli-reference)
6. [Programmatic API](#programmatic-api)
7. [Personas](#personas)
8. [Sub-Agent Architecture](#sub-agent-architecture)
9. [Prompt Caching](#prompt-caching)
10. [Cache Metrics](#cache-metrics)
11. [Cost Tracking](#cost-tracking)
12. [Output Formats](#output-formats)
13. [GitHub Actions Integration](#github-actions-integration)
14. [Troubleshooting](#troubleshooting)
15. [Best Practices](#best-practices)
16. [FAQ](#faq)

---

## Overview

Multi-Persona Review is a multi-persona AI-powered code review plugin for Engram. It runs parallel code reviews using multiple AI personas (security engineer, performance engineer, code health specialist, etc.) to provide comprehensive feedback on your code.

### Key Features

- **Multi-Persona Reviews**: Run 3-8 personas simultaneously for different perspectives
- **Parallel Execution**: 3x faster than sequential reviews
- **Smart Deduplication**: Groups similar findings across personas (50%+ noise reduction)
- **Multiple Output Formats**: Text (ANSI colors), JSON, GitHub markdown
- **Cost Tracking**: Track review costs to GCP Cloud Monitoring, file, or stdout
- **CLI & API**: Use as command-line tool or programmatic API
- **CI/CD Optimized**: Designed for GitHub Actions with cost-efficient operation

### Architecture

- **TypeScript 5.0+** with strict mode
- **Vitest** for testing (104+ tests passing)
- **Claude AI** (Anthropic API) for reviews
- **YAML-based** persona definitions
- **Git integration** for diff-based reviews

---

## Installation

### NPM Package (Local)

```bash
npm install --prefix plugins/multi-persona-review
npm run build --prefix plugins/multi-persona-review
npm link --prefix plugins/multi-persona-review  # Optional: make multi-persona-review available globally
```

### From Source

```bash
git clone https://github.com/vbonnet/engram.git
npm install --prefix engram/plugins/multi-persona-review
npm test --prefix engram/plugins/multi-persona-review  # Optional: run tests
npm run build --prefix engram/plugins/multi-persona-review
```

### Requirements

- Node.js >= 18.0.0
- Anthropic API key
- Git (for diff-based reviews)
- Optional: GCP account (for cost tracking)

---

## Quick Start

### 1. Initialize Configuration

```bash
multi-persona-review init
```

This creates `.engram/config.yml` with defaults.

### 2. Set API Key

```bash
export ANTHROPIC_API_KEY="your-api-key-here"
```

Or pass via CLI flag: `--api-key your-api-key`

#### Claude Code Authentication (Auto-Detected)

When running inside a Claude Code session with VertexAI authentication, multi-persona-review **automatically detects and uses Claude Code credentials**. No configuration needed!

**How it works:**

1. Claude Code sets these environment variables:
   - `CLAUDE_CODE_USE_VERTEX=1` - Signals VertexAI is active
   - `ANTHROPIC_VERTEX_PROJECT_ID` - GCP project ID
   - `CLOUD_ML_REGION` - VertexAI region (e.g., us-east5)

2. Multi-persona-review detects these variables and:
   - Auto-selects `vertexai-claude` provider
   - Uses Claude Code's VertexAI credentials
   - No `--provider` or `--vertex-project` flags required

**Example:**

```bash
# Inside Claude Code session - auto-detected
multi-persona-review src/

# Explicitly use Anthropic API instead (override)
multi-persona-review src/ --provider anthropic --api-key sk-ant-...
```

**Environment Variable Precedence:**

The CLI follows this precedence order (highest to lowest):

1. **Explicit CLI flags** - `--provider`, `--vertex-project`, `--api-key`
2. **Claude Code variables** - `ANTHROPIC_VERTEX_PROJECT_ID`, `CLOUD_ML_REGION` (when `CLAUDE_CODE_USE_VERTEX=1`)
3. **Standard variables** - `VERTEX_PROJECT_ID`, `VERTEX_LOCATION`, `ANTHROPIC_API_KEY`
4. **Default values** - `provider=anthropic`, `location=us-east5`

This ensures explicit flags always override auto-detection, while maintaining backward compatibility with standard VertexAI environment variables.

### 3. List Available Personas

```bash
multi-persona-review --list-personas
```

### 4. Run Your First Review

```bash
# Quick review of current directory
multi-persona-review

# Review specific files
multi-persona-review src/index.ts src/utils.ts

# Thorough review with custom personas
multi-persona-review src/ --mode thorough --personas security-engineer,performance-engineer
```

---

## Configuration

### Configuration File

Create `.engram/config.yml`:

```yaml
crossCheck:
  # Default review mode (quick|thorough|custom)
  defaultMode: quick

  # Default personas to use
  defaultPersonas:
    - security-engineer
    - code-health
    - error-handling-specialist

  # Persona search paths (checked in order)
  personaPaths:
    - .engram/personas        # Project-specific
    - ~/.engram/personas      # User-level
    - /path/to/company/personas  # Company-level

  # Review options
  options:
    # Enable finding deduplication
    deduplicate: true

    # Similarity threshold for deduplication (0-1)
    similarityThreshold: 0.8

    # Maximum number of files to review
    maxFiles: 100

  # Cost tracking configuration
  costTracking:
    type: gcp  # Options: stdout, file, gcp
    config:
      # For GCP sink:
      projectId: my-gcp-project
      keyFilePath: /path/to/service-account-key.json

      # For file sink:
      # filePath: ./multi-persona-review-costs.jsonl

  # GitHub integration (for CI/CD)
  github:
    enabled: true
    changedFilesOnly: true
    skipDrafts: true
    concurrency: 3
    exclude:
      - "**/node_modules/**"
      - "**/*.min.js"
      - "dist/**"
```

### Review Modes

#### Quick Mode (Default)
- **Personas**: 3 (security, error-handling, code-health)
- **Scan Mode**: changed (only modified files)
- **Cost**: Low (~$0.01-0.05 per review)
- **Duration**: Fast (30-60s)

#### Thorough Mode
- **Personas**: 5+ (security, performance, code-health, error-handling, testing)
- **Scan Mode**: diff (all changes with context)
- **Cost**: Medium (~$0.10-0.30 per review)
- **Duration**: Moderate (2-5 minutes)

#### Custom Mode
- **Personas**: User-specified via `--personas` flag
- **Scan Mode**: User-specified via `--scan` flag
- **Cost**: Variable
- **Duration**: Variable

---

## Authentication & Providers

Multi-Persona Review supports three AI provider backends: Anthropic Direct API, VertexAI Claude, and VertexAI Gemini. This section covers authentication for each provider.

### Provider Comparison

| Feature | Anthropic Direct | VertexAI Claude | VertexAI Gemini |
|---------|------------------|-----------------|-----------------|
| **Models** | Claude 3.5 Sonnet/Haiku/Opus | Claude Sonnet 4.5, Haiku 4.5, Opus 4.6 | Gemini 2.5 Flash |
| **Authentication** | API Key | Google ADC | Google ADC |
| **Billing** | Direct Anthropic | GCP Unified Billing | GCP Unified Billing |
| **Prompt Caching** | ✅ 5 min TTL | ✅ 5 min TTL | ❌ Not supported |
| **Sub-Agent Support** | ✅ Yes | ✅ Yes (recommended) | ✅ Yes |
| **Claude Code Auto-Detect** | ❌ No | ✅ Yes | ❌ No |
| **Cost (input/output)** | $3/$15 per 1M | $3/$15 per 1M | $0.15/$0.60 per 1M |
| **Best For** | Direct Anthropic users | GCP users, Claude Code | Cost-sensitive workloads |

### Anthropic Direct API

**Setup:**

1. Get API key from https://console.anthropic.com/settings/keys
2. Set environment variable or pass via CLI:

```bash
# Option 1: Environment variable
export ANTHROPIC_API_KEY=sk-ant-api03-...

# Option 2: CLI flag
multi-persona-review src/ --api-key sk-ant-api03-...
```

**Supported Models:**
- `claude-3-5-sonnet-20241022` (default) - Best balance of speed/quality
- `claude-3-5-haiku-20241022` - Fastest, most cost-effective
- `claude-3-opus-20240229` - Highest quality

**Example:**

```bash
multi-persona-review src/ \
  --provider anthropic \
  --model claude-3-5-sonnet-20241022
```

### VertexAI Claude (Recommended for GCP Users)

**Setup:**

1. **Authenticate with GCP:**

```bash
# Option 1: Application Default Credentials (recommended)
gcloud auth application-default login

# Option 2: Service Account
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

2. **Set VertexAI environment variables:**

```bash
export VERTEX_PROJECT_ID=your-gcp-project-id
export VERTEX_LOCATION=us-east5  # Claude models available in us-east5
export VERTEX_MODEL=claude-sonnet-4-5@20250929
```

3. **Run review:**

```bash
# Auto-detects vertexai-claude if VERTEX_MODEL contains "claude"
multi-persona-review src/

# Or explicitly specify provider
multi-persona-review src/ --provider vertexai-claude
```

**Supported Models (us-east5 region):**
- `claude-sonnet-4-5@20250929` (recommended) - $3/$15 per 1M tokens
- `claude-haiku-4-5@20251001` - $0.80/$4 per 1M tokens
- `claude-opus-4-6@20260205` - $5/$25 per 1M tokens

**Benefits:**
- ✅ **Unified GCP Billing** - Single bill for all GCP services
- ✅ **Committed Use Discounts** - Up to 30% savings for high-volume usage
- ✅ **IAM Integration** - Use GCP roles and service accounts
- ✅ **Cloud Monitoring** - Native integration with GCP observability
- ✅ **Prompt Caching** - 90% cost reduction for cached tokens (5 min TTL)

**Claude Code Auto-Detection:**

When running inside Claude Code with VertexAI authentication, the plugin automatically detects and uses Claude Code credentials:

```bash
# Inside Claude Code - automatically detected
multi-persona-review src/

# Override to use Anthropic API instead
multi-persona-review src/ --provider anthropic --api-key sk-ant-...
```

**Environment variables auto-detected:**
- `CLAUDE_CODE_USE_VERTEX=1` - Triggers auto-detection
- `ANTHROPIC_VERTEX_PROJECT_ID` - GCP project ID
- `CLOUD_ML_REGION` - VertexAI region

### VertexAI Gemini

**Setup:**

1. **Authenticate with GCP** (same as VertexAI Claude above)

2. **Set VertexAI environment variables:**

```bash
export VERTEX_PROJECT_ID=your-gcp-project-id
export VERTEX_LOCATION=us-central1  # Gemini available in us-central1
export VERTEX_MODEL=gemini-2.5-flash
```

3. **Run review:**

```bash
multi-persona-review src/ --provider vertexai
```

**Supported Models:**
- `gemini-2.5-flash` (recommended) - Fast and cost-effective
- `gemini-2.5-flash-lite` - Most economical option

**Note:** Gemini does not support prompt caching, so cost savings are lower than Claude models.

### Authentication Troubleshooting

#### Issue: "API key not found"

**Anthropic:**
```bash
# Check if API key is set
echo $ANTHROPIC_API_KEY

# Verify key format (should start with "sk-ant-")
# If missing, get from https://console.anthropic.com/settings/keys
export ANTHROPIC_API_KEY=sk-ant-api03-...
```

#### Issue: "Google Cloud authentication failed"

**VertexAI:**
```bash
# Check if authenticated
gcloud auth application-default print-access-token

# If error, re-authenticate
gcloud auth application-default login

# Or use service account
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
```

#### Issue: "Project ID not found"

```bash
# Check environment variables
echo $VERTEX_PROJECT_ID
echo $ANTHROPIC_VERTEX_PROJECT_ID  # For Claude Code

# Set project ID
export VERTEX_PROJECT_ID=your-project-id

# Or pass via CLI
multi-persona-review src/ --vertex-project your-project-id
```

#### Issue: "Model not available in region"

Claude models are only available in **us-east5** region. Gemini models are in **us-central1**.

```bash
# For Claude
export VERTEX_LOCATION=us-east5

# For Gemini
export VERTEX_LOCATION=us-central1
```

### Provider Selection Precedence

The CLI selects providers in this order (highest to lowest priority):

1. **Explicit `--provider` flag** - Always overrides auto-detection
2. **Claude Code environment variables** - When `CLAUDE_CODE_USE_VERTEX=1`
3. **Standard environment variables** - `ANTHROPIC_API_KEY`, `VERTEX_PROJECT_ID`
4. **Default** - Falls back to `anthropic` provider

**Examples:**

```bash
# Scenario 1: Explicit provider (highest priority)
export ANTHROPIC_API_KEY=sk-ant-...
multi-persona-review src/ --provider vertexai-claude --vertex-project my-project
# Result: Uses VertexAI Claude (explicit flag wins)

# Scenario 2: Claude Code auto-detection
export CLAUDE_CODE_USE_VERTEX=1
export ANTHROPIC_VERTEX_PROJECT_ID=my-project
multi-persona-review src/
# Result: Auto-detects and uses VertexAI Claude

# Scenario 3: Standard env vars
export VERTEX_PROJECT_ID=my-project
export VERTEX_MODEL=claude-sonnet-4-5@20250929
multi-persona-review src/
# Result: Auto-detects vertexai-claude from model name

# Scenario 4: Anthropic default
export ANTHROPIC_API_KEY=sk-ant-...
multi-persona-review src/
# Result: Uses Anthropic Direct API
```

---

## Full-Document Review Mode

By default, multi-persona-review operates in git-aware mode, reviewing only changed files. For non-git repositories or complete file analysis, use `--full` flag.

### When to Use Full-Document Mode

**Use `--full` when:**
- ✅ Code is not in a git repository
- ✅ You need complete file context (not just diffs)
- ✅ Reviewing generated or downloaded code
- ✅ First-time analysis of legacy codebase
- ✅ Working in temporary directories

**Use default (git-aware) when:**
- ⏺ Code is in git repository
- ⏺ Reviewing PR changes or recent commits
- ⏺ Cost optimization is important (diffs use 80% fewer tokens)

### Examples

```bash
# Full-document mode (no git required)
multi-persona-review . --full

# Same as --scan full
multi-persona-review src/ --scan full

# Non-git directory example
cd /tmp/downloaded-code
multi-persona-review . --full --mode quick

# Review entire files with custom personas
multi-persona-review lib/ --full --personas security-engineer,code-health
```

### Performance Considerations

| Mode | Token Usage | Cost | Best For |
|------|-------------|------|----------|
| **diff** (default) | Low (~1,000-5,000 tokens) | $0.01-0.05 | Git repos, PR reviews |
| **changed** | Medium (~5,000-15,000 tokens) | $0.05-0.15 | Recent git changes |
| **full** | High (~20,000-50,000 tokens) | $0.20-0.50 | Non-git, legacy code |

**Tip:** Use `--mode quick` with `--full` to reduce cost while maintaining coverage.

---

## Non-Git Usage

Multi-Persona Review works perfectly in non-git environments. No repository required!

### Setup

```bash
# No git initialization needed
cd /tmp/my-code

# Review with full-document mode
multi-persona-review . --full
```

### Use Cases

**1. Generated Code Review:**
```bash
# Review code generated by AI or templates
mkdir -p /tmp/generated-api
cd /tmp/generated-api
# ... generate code ...
multi-persona-review . --full --personas security-engineer,api-design-reviewer
```

**2. Downloaded Code Analysis:**
```bash
# Analyze third-party code before integration
wget https://example.com/library.zip
unzip library.zip
cd library/
multi-persona-review src/ --full --mode thorough
```

**3. Temporary Directory Review:**
```bash
# Review code in temporary workspace
mktemp -d /tmp/review-XXXXXX
cd /tmp/review-*
# ... copy files ...
multi-persona-review . --full --personas security-engineer
```

**4. Legacy Code Migration:**
```bash
# Review legacy codebase not yet in git
cd /path/to/legacy-app
multi-persona-review . --full --mode thorough --format json > review.json
```

### Non-Git Limitations

When running without git:
- ❌ No diff mode available (use `--full` or `--scan full`)
- ❌ No git metadata (branch, commit, author) in cost tracking
- ❌ No changed files detection (reviews all matching files)

**Workaround:** Use explicit file patterns to limit scope:

```bash
# Review only specific files/directories
multi-persona-review src/auth.ts src/api/ --full

# Use glob patterns
multi-persona-review "src/**/*.ts" --full
```

---

## CLI Reference

### Commands

#### `multi-persona-review [files...]`

Run code review on specified files or directories.

**Arguments:**
- `[files...]` - Files or directories to review (default: current directory)

**Options:**
```
-m, --mode <mode>           Review mode (quick|thorough|custom) [default: quick]
-p, --personas <personas>   Comma-separated persona names
-s, --scan <mode>           Scan mode (full|diff|changed) [default: changed]
-f, --format <format>       Output format (text|json|github) [default: text]
--no-colors                 Disable colored output
--no-cost                   Hide cost information
--no-dedupe                 Disable deduplication
--flat                      Flat output (don't group by file)
--api-key <key>             Anthropic API key
--model <model>             Claude model [default: claude-3-5-sonnet-20241022]
--cost-sink <type>          Cost tracking sink (stdout|file|gcp)
--cost-file <path>          File path for file cost sink
--gcp-project <id>          GCP project ID for GCP cost sink
--gcp-key-file <path>       GCP service account key file
--show-cache-metrics        Display cache hit rate and cost savings after review
--verbose                   Enable verbose logging
--dry-run                   Show preview without running
--list-personas             List available personas and exit
-V, --version               Output version number
-h, --help                  Display help
```

**Examples:**
```bash
# Quick review with defaults
multi-persona-review

# Review specific files
multi-persona-review src/index.ts src/auth.ts

# Thorough review of entire src directory
multi-persona-review src/ --mode thorough

# Custom review with specific personas
multi-persona-review src/ --personas security-engineer,performance-engineer,database-specialist

# Output as JSON for CI/CD
multi-persona-review src/ --format json > review.json

# Output as GitHub PR comment
multi-persona-review src/ --format github > pr-comment.md

# Dry run to see what would be reviewed
multi-persona-review src/ --dry-run

# Verbose mode for debugging
multi-persona-review src/ --verbose

# Track costs to GCP
multi-persona-review src/ --cost-sink gcp --gcp-project my-project-123
```

#### `multi-persona-review init`

Initialize multi-persona-review configuration file.

**Options:**
```
--force    Overwrite existing configuration
```

**Example:**
```bash
multi-persona-review init
multi-persona-review init --force  # Overwrite existing
```

### Exit Codes

- `0` - Success (no critical or high findings)
- `1` - Failed (critical or high findings, or error occurred)

---

## Programmatic API

### Basic Usage

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
} from '@engram/multi-persona-review';

// 1. Load personas
const personas = await loadPersonas([
  '.engram/personas',
  '~/.engram/personas',
]);
const securityPersona = personas.get('security-engineer')!;

// 2. Create reviewer
const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
});

// 3. Run review
const result = await reviewFiles(
  {
    files: ['src/'],
    personas: [securityPersona],
    mode: 'thorough',
    fileScanMode: 'diff',
    options: {
      deduplicate: true,
      similarityThreshold: 0.8,
    },
  },
  process.cwd(),
  reviewer
);

// 4. Format and display results
const formatted = formatReviewResult(result, {
  colors: true,
  groupByFile: true,
  showCost: true,
  showSummary: true,
});

console.log(formatted);

// Check if critical findings
if (result.summary.findingsBySeverity.critical > 0) {
  process.exit(1);
}
```

### API Reference

#### Core Functions

##### `loadPersonas(paths: string[]): Promise<Map<string, Persona>>`

Load personas from search paths.

**Parameters:**
- `paths` - Array of directories to search for persona YAML files

**Returns:** Map of persona name to Persona object

**Example:**
```typescript
const personas = await loadPersonas([
  '.engram/personas',
  '~/my-personas',
]);

console.log(`Loaded ${personas.size} personas`);
for (const [name, persona] of personas) {
  console.log(`- ${name}: ${persona.description}`);
}
```

##### `reviewFiles(config, cwd, reviewer, options?): Promise<ReviewResult>`

Run code review on files.

**Parameters:**
- `config: ReviewConfig` - Review configuration
- `cwd: string` - Current working directory
- `reviewer: ReviewerFunction` - Reviewer function (from createAnthropicReviewer)
- `options?: { parallel?: boolean, costSink?: CostSink, costMetadata?: CostMetadata }`

**Returns:** ReviewResult with findings, cost, and summary

**Example:**
```typescript
const result = await reviewFiles(
  {
    files: ['src/'],
    personas: [persona1, persona2],
    mode: 'thorough',
    fileScanMode: 'diff',
  },
  process.cwd(),
  reviewer,
  {
    parallel: true,  // Run personas in parallel
    costSink,        // Optional cost tracking
    costMetadata: {
      repository: 'my-repo',
      branch: 'main',
    },
  }
);
```

##### `createAnthropicReviewer(config): ReviewerFunction`

Create a reviewer function using Anthropic Claude API.

**Parameters:**
- `config: { apiKey: string, model?: string }`

**Returns:** ReviewerFunction

**Example:**
```typescript
const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
});
```

#### Formatting Functions

##### `formatReviewResult(result, options): string`

Format review result as colored text.

**Parameters:**
- `result: ReviewResult`
- `options: { colors?, groupByFile?, showCost?, showSummary? }`

**Returns:** Formatted string

##### `formatReviewResultJSON(result): string`

Format review result as JSON.

**Parameters:**
- `result: ReviewResult`

**Returns:** JSON string

##### `formatReviewResultGitHub(result): string`

Format review result as GitHub markdown.

**Parameters:**
- `result: ReviewResult`

**Returns:** Markdown string

#### Cost Tracking

##### `createCostSink(config): Promise<CostSink>`

Create a cost sink for tracking review costs.

**Parameters:**
- `config: CostSinkConfig`

**Returns:** CostSink instance

**Example:**
```typescript
// GCP sink
const gcpSink = await createCostSink({
  type: 'gcp',
  config: {
    projectId: 'my-project-123',
    keyFilePath: './gcp-key.json',
    labels: {
      team: 'backend',
      environment: 'production',
    },
  },
});

// File sink
const fileSink = await createCostSink({
  type: 'file',
  config: {
    filePath: './costs.jsonl',
  },
});

// Use with reviewFiles
await reviewFiles(config, cwd, reviewer, {
  costSink: gcpSink,
  costMetadata: {
    repository: 'my-repo',
    pullRequest: 123,
  },
});
```

---

## Personas

### Built-in Personas

Multi-Persona Review includes 8 core personas:

1. **security-engineer** - Security vulnerabilities, authentication, encryption
2. **performance-engineer** - Performance bottlenecks, scalability, resource usage
3. **code-health** - Code smells, technical debt, maintainability
4. **error-handling-specialist** - Error handling, logging, recovery
5. **testing-advocate** - Test coverage, test quality, testability
6. **accessibility-specialist** - WCAG compliance, screen reader support
7. **database-specialist** - SQL injection, query performance, data integrity
8. **documentation-reviewer** - Code comments, README, API docs

### Creating Custom Personas

Create a YAML file in your personas directory:

```yaml
name: performance-engineer
displayName: Performance Engineer
version: "1.0.0"
description: Specializes in performance optimization and efficiency
focusAreas:
  - Performance
  - Scalability
  - Resource Usage
  - Caching
  - Database Queries
prompt: |
  You are a performance engineering expert reviewing code.

  Focus on:
  - Performance bottlenecks
  - Inefficient algorithms (O(n²) loops, etc.)
  - Memory leaks
  - Database query optimization
  - Caching opportunities
  - Resource usage (CPU, memory, network)

  For each issue found:
  1. Identify the specific line(s) of code
  2. Explain the performance impact
  3. Suggest concrete improvements
  4. Estimate severity (critical/high/medium/low)
```

### Persona File Format

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier (kebab-case) |
| `displayName` | string | Yes | Human-readable name |
| `version` | string | Yes | Semver version (x.y.z) |
| `description` | string | Yes | Brief description |
| `focusAreas` | string[] | Yes | List of expertise areas |
| `prompt` | string | Yes | System prompt for Claude |

---

## Sub-Agent Architecture

### Overview

Multi-Persona Review uses Claude sub-agents to provide **isolated execution contexts** for each persona. This prevents context contamination between personas and enables powerful prompt caching optimizations.

### How It Works

**Traditional Approach (Legacy)**:
```
┌─────────────────────────────────────┐
│  Single Claude Instance             │
│  ┌────────┐ ┌────────┐ ┌────────┐  │
│  │Persona1│ │Persona2│ │Persona3│  │
│  └────────┘ └────────┘ └────────┘  │
│  All share same conversation memory │
└─────────────────────────────────────┘
```

**Sub-Agent Approach (New)**:
```
┌──────────┐    ┌──────────┐    ┌──────────┐
│SubAgent 1│    │SubAgent 2│    │SubAgent 3│
│Persona 1 │    │Persona 2 │    │Persona 3 │
│Isolated  │    │Isolated  │    │Isolated  │
│Context   │    │Context   │    │Context   │
└──────────┘    └──────────┘    └──────────┘
```

### Key Benefits

**1. Context Isolation**
- Each persona has its own isolated conversation context
- No cross-contamination between personas
- More accurate, focused reviews

**2. Prompt Caching**
- Persona definitions are cached with `cache_control: ephemeral`
- Subsequent reviews reuse cached persona prompts
- **86%+ cost savings** for large codebase reviews

**3. Parallel Execution**
- Sub-agents can run in parallel (default)
- Same speed as legacy approach
- Better resource utilization

### Sub-Agent Lifecycle

```typescript
// 1. Create sub-agent pool
const pool = new SubAgentPool();

// 2. Get or create sub-agent for persona
const agent = pool.get(persona, config);

// 3. Run review (uses cached prompt if available)
const result = await agent.review(input);

// 4. Sub-agent stats show cache performance
const stats = agent.getStats();
console.log(`Cache hits: ${stats.cacheHits}`);

// 5. Cleanup (automatic)
await pool.clear();
```

### Configuration

Sub-agents are **enabled by default**. To disable:

```yaml
# .engram/config.yml
options:
  useSubAgents: false  # Fall back to legacy execution
```

Or via CLI:
```bash
multi-persona-review --no-sub-agents src/
```

### Fallback Behavior

If sub-agent creation fails (e.g., authentication error), the system automatically falls back to legacy execution:

```
[WARN] Sub-agent failed for security-engineer, falling back to legacy reviewer
```

This ensures reviews complete even if sub-agent infrastructure has issues.

### Pool Management

The sub-agent pool manages persona instances efficiently:

- **LRU Eviction**: Removes least-recently-used agents when pool is full
- **Cache Reuse**: Returns existing agent for same persona (maximizes cache hits)
- **Stats Tracking**: Per-agent statistics (reviews, cache hits, tokens)

Default pool size: **10 agents**

---

## Prompt Caching

### Overview

Prompt caching reduces API costs by **caching stable portions** of prompts (persona definitions) and reusing them across multiple reviews.

### How It Works

**Without Caching**:
```
Review 1: Send full persona prompt (1,500 tokens) + document → $0.015
Review 2: Send full persona prompt (1,500 tokens) + document → $0.015
Review 3: Send full persona prompt (1,500 tokens) + document → $0.015
Total: $0.045
```

**With Caching**:
```
Review 1: Send persona (1,500 tokens) + cache + document → $0.015
Review 2: Read cached persona + document → $0.002
Review 3: Read cached persona + document → $0.002
Total: $0.019 (58% savings)
```

### Cache Structure

```typescript
{
  model: "claude-3-5-sonnet-20241022",
  system: [
    {
      type: "text",
      text: "You are a security engineer...", // Persona prompt
      cache_control: { type: "ephemeral" }    // Cache this!
    }
  ],
  messages: [
    { role: "user", content: "Review this code..." } // Varies per review
  ]
}
```

### Cost Savings

Real data from Wayfinder project reviews:

| Scenario | Files | Without Cache | With Cache | Savings |
|----------|-------|---------------|------------|---------|
| Small review | 7 | $0.067 | $0.030 | 55.2% |
| Large review | 96 | $12.69 | $1.76 | **86.1%** |
| Typical review | 30-50 | $4-6 | $0.60-0.90 | 83-85% |

**Break-even**: Just **2 cache hits** within 5-minute window

### Requirements

For optimal caching, personas must exceed **1,024 tokens** (Sonnet threshold).

The following personas are cache-optimized:
- ✅ `tech-lead` (1,600 tokens)
- ✅ `security-engineer` (1,450 tokens)
- ✅ `qa-engineer` (1,650 tokens)
- ✅ `product-manager` (1,550 tokens)
- ✅ `devops-engineer` (1,750 tokens)

### Cache TTL

- **Default**: 5 minutes
- **Maximum**: 5 minutes (Anthropic limitation)
- **Recommendation**: Run sequential reviews within 5 minutes to maximize cache hits

### Configuration

Caching is **enabled by default** when using sub-agents.

To disable caching:
```bash
multi-persona-review --no-cache src/
```

Or in config:
```yaml
options:
  useSubAgents: false  # Disables caching (uses legacy mode)
```

### Best Practices

**1. Batch Reviews**
Run multiple reviews within 5-minute window:
```bash
# Review multiple files sequentially (cache reuse)
multi-persona-review src/auth.ts
multi-persona-review src/api.ts     # Cache hit!
multi-persona-review src/db.ts      # Cache hit!
```

**2. Use Consistent Personas**
Stick to same personas across review sessions for maximum cache hits.

**3. Monitor Cache Performance**
Use `--show-cache-metrics` to see cache hit rates:
```bash
multi-persona-review --show-cache-metrics src/

Cache Performance:
  security-engineer: 3/3 cache hits (100%)
  tech-lead: 2/3 cache hits (67%)
  Total savings: $0.045 (86%)
```

### Limitations

- Cache expires after 5 minutes (Anthropic limitation)
- Only works with sub-agents (legacy mode doesn't cache)
- Persona prompt must be ≥1,024 tokens for Sonnet

---

## Cache Metrics

### Overview

Cache metrics help you understand caching performance and optimize costs.

### Viewing Metrics

**CLI Flag**:
```bash
multi-persona-review --show-cache-metrics src/

Review Complete!

Findings: 12 total (3 critical, 5 high, 4 medium)

Cache Performance:
┌────────────────────┬───────┬──────┬─────────┐
│ Persona            │ Hits  │ Miss │ Hit Rate│
├────────────────────┼───────┼──────┼─────────┤
│ security-engineer  │   3   │  1   │  75.0%  │
│ tech-lead          │   3   │  0   │ 100.0%  │
│ qa-engineer        │   2   │  1   │  66.7%  │
└────────────────────┴───────┴──────┴─────────┘

Total Cost: $0.42 (saved $2.58 vs no cache, 86% reduction)
```

**Programmatic API**:
```typescript
const pool = new SubAgentPool();
const agent = pool.get(persona, config);

// After reviews...
const stats = agent.getStats();

console.log({
  cacheHits: stats.cacheHits,           // 5
  cacheMisses: stats.cacheMisses,       // 1
  cacheHitRate: stats.cacheHits /
    (stats.cacheHits + stats.cacheMisses), // 83.3%
  totalCacheReads: stats.totalCacheReads,   // 7,500 tokens
  totalCacheWrites: stats.totalCacheWrites, // 1,500 tokens
});
```

### Key Metrics

| Metric | Description | Good Target |
|--------|-------------|-------------|
| **Cache Hits** | Number of times cache was used | ≥80% of reviews |
| **Cache Misses** | Cache creation (first use) | First review only |
| **Cache Hit Rate** | Hits / (Hits + Misses) | ≥80% |
| **Cache Reads** | Tokens read from cache | High = good |
| **Cache Writes** | Tokens written to cache | Low = good |

### Interpreting Results

**Good Cache Performance**:
```
Cache Hit Rate: 85%+
Total Savings: 80%+ cost reduction
Pattern: First review = miss, subsequent = hits
```

**Poor Cache Performance**:
```
Cache Hit Rate: <50%
Total Savings: <40% cost reduction
Causes: Infrequent reviews, >5min gaps, persona changes
```

### Troubleshooting Low Hit Rates

**1. Time Gaps >5 Minutes**
```
Review 1: 10:00 AM → Cache created
Review 2: 10:06 AM → Cache expired (miss)
```
**Fix**: Run reviews closer together (within 5 minutes)

**2. Persona Modifications**
```
Review 1: persona v1.0 → Cache created
Review 2: persona v1.1 → Cache miss (different persona)
```
**Fix**: Avoid modifying persona definitions between reviews

**3. Small Personas**
```
Persona size: 800 tokens (below 1,024 threshold)
Result: No caching benefit
```
**Fix**: Expand persona prompt to ≥1,024 tokens (add examples, rubrics)

### Cost Sink Integration

Cache metrics are included in cost sink outputs:

**GCP Cloud Monitoring**:
```yaml
costSink:
  type: gcp
  projectId: my-project
  metricType: custom.googleapis.com/ai/review_cache_performance
```

Metrics written:
- `cache_hit_rate` (gauge, 0-100%)
- `cache_reads_tokens` (counter)
- `cache_writes_tokens` (counter)
- `cost_savings_usd` (counter)

---

## Cost Tracking

### Overview

Multi-Persona Review can track review costs to help you monitor and optimize usage.

### Cost Sinks

#### Stdout (Default)

Logs cost information to stderr as JSON.

```yaml
costTracking:
  type: stdout
```

#### File

Appends cost records to JSONL file.

```yaml
costTracking:
  type: file
  config:
    filePath: ./multi-persona-review-costs.jsonl
```

#### GCP Cloud Monitoring

Sends metrics to Google Cloud Monitoring.

```yaml
costTracking:
  type: gcp
  config:
    projectId: my-gcp-project-123
    keyFilePath: /path/to/service-account-key.json
    metricPrefix: custom.googleapis.com/engram/multi-persona-review
    labels:
      team: backend
      environment: production
```

### GCP Setup

1. Install package:
   ```bash
   npm install @google-cloud/monitoring
   ```

2. Create service account:
   ```bash
   gcloud iam service-accounts create multi-persona-review-metrics
   ```

3. Grant permissions:
   ```bash
   gcloud projects add-iam-policy-binding PROJECT_ID \
     --member="serviceAccount:multi-persona-review-metrics@PROJECT_ID.iam.gserviceaccount.com" \
     --role="roles/monitoring.metricWriter"
   ```

4. Create key:
   ```bash
   gcloud iam service-accounts keys create ~/multi-persona-review-key.json \
     --iam-account=multi-persona-review-metrics@PROJECT_ID.iam.gserviceaccount.com
   ```

### Metrics Tracked

- `cost/total` - Total USD cost per review
- `tokens/total` - Total tokens used
- `cost/by_persona` - Cost per persona
- `tokens/input_by_persona` - Input tokens per persona
- `tokens/output_by_persona` - Output tokens per persona
- `findings/total` - Total findings count
- `files/reviewed` - Files reviewed count
- `cache/hit_rate` - Overall cache hit rate (percentage)
- `cache/hit_rate_by_persona` - Cache hit rate per persona
- `cache/tokens_saved` - Tokens saved through caching
- `cache/savings_dollars` - Cost savings from caching
- `cache/alert` - Cache performance degradation alerts

### Cache Hit Rate Alerts

Multi-Persona Review automatically monitors cache performance per persona and generates alerts when cache hit rates drop below acceptable thresholds.

**Alert Thresholds:**
- **Target**: ≥80% cache hit rate per persona (optimal performance)
- **Alert**: <50% cache hit rate for 3 consecutive reviews (degraded performance)
- **Severity**:
  - `warning`: Hit rate between 30-50% (investigate)
  - `critical`: Hit rate <30% (immediate action needed)

**Common Causes:**
- Persona prompt has been modified (breaks cache invalidation)
- Persona version incremented (intentional cache invalidation)
- Unstable persona definition (dynamic content in prompt)
- Cache control configuration issues

**Viewing Alerts:**

Use the `--show-cache-metrics` flag to display cache alerts after a review:

```bash
multi-persona-review src/ --show-cache-metrics
```

Example alert output:

```
⚠ Cache Performance Alerts
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[WARNING] security-engineer

  Cache Hit Rate:     35.2% (target: 80%)
  Consecutive Issues: 3 reviews

  Persona "security-engineer" has cache hit rate of 35.2% (target: 80%,
  threshold: 50%). This has occurred for 3 consecutive reviews. Poor cache
  performance increases API costs and latency.

  Suggestions:
    • Cache performance is below target - review recent changes
    • Ensure persona prompts are stable across reviews
    • Check if persona version has been incremented (invalidates cache)
    • Review persona focusAreas and prompt for unintended modifications
```

**GCP Cloud Monitoring Integration:**

Alerts are automatically sent to GCP Cloud Monitoring if configured:

```yaml
costTracking:
  type: gcp
  config:
    projectId: my-gcp-project-123
    keyFilePath: /path/to/service-account-key.json
```

GCP metrics:
- `cache/alert` - Alert events with severity and consecutive failure count
- `cache/hit_rate_by_persona` - Per-persona hit rate tracking

**Resolving Alerts:**

1. **Review Recent Changes**: Check if persona definitions were modified
2. **Stabilize Prompts**: Ensure persona prompts don't contain dynamic content
3. **Version Carefully**: Only increment persona versions when necessary
4. **Monitor Metrics**: Track cache performance over time in GCP

### Cost Estimation

| Mode | Personas | Avg Tokens | Avg Cost |
|------|----------|------------|----------|
| Quick | 3 | 5,000 | $0.01-0.05 |
| Thorough | 5 | 15,000 | $0.10-0.30 |
| Full (10 files) | 5 | 50,000 | $0.50-1.50 |

*Costs based on Claude 3.5 Sonnet pricing ($3/MTok input, $15/MTok output)*

---

## Output Formats

### Text (Default)

ANSI-colored terminal output with file grouping.

```bash
multi-persona-review src/ --format text
```

Features:
- Colored severity indicators (🔴 critical, 🟠 high, 🟡 medium, 🔵 low, ℹ️ info)
- Grouped by file
- Cost breakdown
- Summary statistics

### JSON

Machine-readable JSON for CI/CD integration.

```bash
multi-persona-review src/ --format json > review.json
```

Structure:
```json
{
  "sessionId": "uuid",
  "startTime": "2025-11-23T10:00:00Z",
  "endTime": "2025-11-23T10:01:30Z",
  "findings": [
    {
      "id": "finding-1",
      "file": "src/auth.ts",
      "line": 42,
      "severity": "critical",
      "title": "SQL Injection Vulnerability",
      "description": "...",
      "personas": ["security-engineer"]
    }
  ],
  "summary": {
    "filesReviewed": 10,
    "totalFindings": 25,
    "findingsBySeverity": {
      "critical": 2,
      "high": 5,
      "medium": 10,
      "low": 8,
      "info": 0
    }
  },
  "cost": {
    "totalCost": 0.15,
    "totalTokens": 12500
  }
}
```

### GitHub

Markdown formatted for GitHub PR comments.

```bash
multi-persona-review src/ --format github > pr-comment.md
gh pr comment 123 --body-file pr-comment.md
```

Features:
- Collapsible details per finding
- Severity grouping
- Summary with emojis
- Cost tracking

---

## GitHub Actions Integration

### Basic Workflow

`.github/workflows/multi-persona-review.yml`:

```yaml
name: Multi-Persona Review Code Review

on:
  pull_request:
    types: [opened, synchronize]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
        with:
          fetch-depth: 0  # Full history for diff mode

      - uses: actions/setup-node@v3
        with:
          node-version: '18'

      - name: Install Multi-Persona Review
        run: |
          npm install --prefix plugins/multi-persona-review
          npm run build --prefix plugins/multi-persona-review
          npm link --prefix plugins/multi-persona-review

      - name: Run Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review src/ \
            --mode thorough \
            --format github \
            --cost-sink file \
            --cost-file costs.jsonl \
            > review.md

      - name: Comment PR
        uses: actions/github-script@v6
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('review.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: review
            });

      - name: Upload Costs
        uses: actions/upload-artifact@v3
        with:
          name: review-costs
          path: costs.jsonl
```

### Advanced: Fail on Critical Findings

```yaml
      - name: Run Review
        id: review
        continue-on-error: true
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review src/ --mode thorough --format json > review.json
          cat review.json

      - name: Check Results
        run: |
          CRITICAL=$(jq '.summary.findingsBySeverity.critical' review.json)
          HIGH=$(jq '.summary.findingsBySeverity.high' review.json)
          if [ "$CRITICAL" -gt 0 ] || [ "$HIGH" -gt 5 ]; then
            echo "❌ Found $CRITICAL critical and $HIGH high severity issues"
            exit 1
          fi
```

---

## Troubleshooting

### Provider-Specific Error Messages

Multi-persona-review provides detailed, provider-specific error messages to help you quickly diagnose and fix credential issues.

#### Missing Credentials Error

**Error Message:**
```
Missing provider credentials. Please configure one of:

Anthropic API:
  Environment: ANTHROPIC_API_KEY=sk-ant-...
  CLI flag: --api-key sk-ant-...

VertexAI (Claude Code auto-detected):
  Environment: ANTHROPIC_VERTEX_PROJECT_ID=your-project
               CLOUD_ML_REGION=us-east5
               GOOGLE_APPLICATION_CREDENTIALS=/path/to/key.json
  CLI flags: --provider vertexai-claude --vertex-project your-project

See DOCUMENTATION.md for detailed setup instructions.
```

**Solution**: Configure credentials for your chosen provider (see below).

#### Authentication Failed (Anthropic)

**Error Message:**
```
Authentication failed for anthropic provider: Invalid API key

Troubleshooting:
  1. Verify ANTHROPIC_API_KEY is set correctly
  2. Check API key starts with "sk-ant-"
  3. Ensure API key has not expired
  4. Visit https://console.anthropic.com/settings/keys to manage keys
```

**Solution:**
```bash
# Check current API key
echo $ANTHROPIC_API_KEY

# Set new API key
export ANTHROPIC_API_KEY="sk-ant-api03-..."

# Verify it works
multi-persona-review --verbose src/
```

#### Authentication Failed (VertexAI)

**Error Message:**
```
Authentication failed for vertexai-claude provider: Permission denied

Troubleshooting:
  1. Verify ANTHROPIC_VERTEX_PROJECT_ID is set
  2. Check CLOUD_ML_REGION is valid (e.g., us-east5)
  3. Ensure GOOGLE_APPLICATION_CREDENTIALS points to valid service account key
  4. Verify service account has Vertex AI User role
  5. Check project has Vertex AI API enabled
```

**Solution:**
```bash
# Check environment variables
echo $ANTHROPIC_VERTEX_PROJECT_ID
echo $CLOUD_ML_REGION
echo $GOOGLE_APPLICATION_CREDENTIALS

# Set VertexAI credentials
export ANTHROPIC_VERTEX_PROJECT_ID="my-gcp-project"
export CLOUD_ML_REGION="us-east5"
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

# Verify service account permissions
gcloud projects get-iam-policy $ANTHROPIC_VERTEX_PROJECT_ID \
  --flatten="bindings[].members" \
  --filter="bindings.role:roles/aiplatform.user"

# Enable Vertex AI API if needed
gcloud services enable aiplatform.googleapis.com --project=$ANTHROPIC_VERTEX_PROJECT_ID

# Test with verbose mode
multi-persona-review --verbose src/
```

#### Rate Limit Exceeded

**Error Message:**
```
anthropic API error: Rate limit exceeded

Rate limit exceeded. Try:
  1. Reduce number of parallel personas
  2. Add delays between reviews
  3. Check your API tier limits
```

**Solution:**
```bash
# Run with fewer personas
multi-persona-review src/ --personas security-engineer,code-health

# Run sequentially instead of parallel (slower but respects rate limits)
# Note: This requires programmatic API usage
```

### Common Issues

#### "Anthropic API key required"

**Solution**: Set environment variable or use `--api-key` flag
```bash
export ANTHROPIC_API_KEY="your-key-here"
# or
multi-persona-review --api-key your-key-here src/
```

#### "Persona 'X' not found"

**Solution**: Check persona name and paths
```bash
multi-persona-review --list-personas  # See available personas
multi-persona-review --verbose src/   # See persona search paths
```

#### "EISDIR: illegal operation on a directory"

**Solution**: Pass file path, not directory, to `loadCrossCheckConfig()`
```typescript
// Wrong
const config = await loadCrossCheckConfig('/path/to/dir');

// Right
const config = await loadCrossCheckConfig('/path/to/dir/.engram/config.yml');
// or
const config = await loadCrossCheckConfig(); // Uses default
```

#### "All personas failed to complete review"

**Causes:**
- API key invalid
- Network issues
- File scanning errors

**Solution:** Use `--verbose` to see detailed errors
```bash
multi-persona-review --verbose src/
```

#### Low Cache Hit Rate (<50%)

**Causes:**
- Reviews spaced >5 minutes apart (cache expired)
- Persona definitions modified between reviews
- Persona size <1,024 tokens

**Solution:** Check cache metrics and timing
```bash
multi-persona-review --show-cache-metrics --verbose src/

# If time gaps are the issue:
# Run reviews closer together (within 5 minutes)

# If persona size is the issue:
# Expand persona prompts to ≥1,024 tokens
```

#### "Sub-agent failed, falling back to legacy reviewer"

**Causes:**
- Authentication error (401/403)
- Network connectivity issues
- API rate limiting

**Solution:** This is expected fallback behavior. The review continues using legacy mode.
```bash
# Check API key
echo $ANTHROPIC_API_KEY

# Verify authentication
curl -H "x-api-key: $ANTHROPIC_API_KEY" \
  https://api.anthropic.com/v1/messages \
  -X POST -d '{"model":"claude-3-5-sonnet-20241022","max_tokens":10,"messages":[{"role":"user","content":"test"}]}'
```

### Provider Setup Examples

#### Setting up Anthropic API

1. **Get API Key:**
   - Visit https://console.anthropic.com/settings/keys
   - Create new API key
   - Copy key (starts with "sk-ant-")

2. **Configure Environment:**
   ```bash
   # Add to ~/.bashrc or ~/.zshrc
   export ANTHROPIC_API_KEY="sk-ant-api03-..."

   # Or use project-specific .env file
   echo 'ANTHROPIC_API_KEY=sk-ant-api03-...' > .env
   source .env
   ```

3. **Verify Setup:**
   ```bash
   multi-persona-review --verbose --dry-run src/
   # Should see: [REVIEW-ENGINE] Using sub-agents with anthropic provider
   ```

#### Setting up VertexAI (GCP)

1. **Enable VertexAI API:**
   ```bash
   gcloud services enable aiplatform.googleapis.com
   ```

2. **Create Service Account:**
   ```bash
   # Create service account
   gcloud iam service-accounts create claude-reviewer \
     --display-name="Claude Code Reviewer"

   # Grant Vertex AI User role
   gcloud projects add-iam-policy-binding PROJECT_ID \
     --member="serviceAccount:claude-reviewer@PROJECT_ID.iam.gserviceaccount.com" \
     --role="roles/aiplatform.user"

   # Create and download key
   gcloud iam service-accounts keys create ~/claude-reviewer-key.json \
     --iam-account=claude-reviewer@PROJECT_ID.iam.gserviceaccount.com
   ```

3. **Configure Environment:**
   ```bash
   # Add to ~/.bashrc or ~/.zshrc
   export ANTHROPIC_VERTEX_PROJECT_ID="your-gcp-project"
   export CLOUD_ML_REGION="us-east5"
   export GOOGLE_APPLICATION_CREDENTIALS="$HOME/claude-reviewer-key.json"
   ```

4. **Verify Setup:**
   ```bash
   multi-persona-review --verbose --dry-run src/
   # Should see: [REVIEW-ENGINE] Using sub-agents with vertexai-claude provider
   ```

#### Claude Code Auto-Detection

When running inside Claude Code with VertexAI enabled, credentials are auto-detected:

```bash
# Claude Code automatically sets these:
# CLAUDE_CODE_USE_VERTEX=1
# ANTHROPIC_VERTEX_PROJECT_ID=your-project
# CLOUD_ML_REGION=us-east5

# No configuration needed - just run:
multi-persona-review src/

# To verify auto-detection:
multi-persona-review --verbose src/
# Look for: [SUB-AGENT] Provider selected: vertexai-claude
```

### Verbose Mode for Debugging

Enable verbose mode to see detailed provider selection and configuration:

```bash
multi-persona-review --verbose --dry-run src/
```

**Example Output:**
```
[REVIEW-ENGINE] Using sub-agents with anthropic provider
[REVIEW-ENGINE] Model: claude-3-5-sonnet-20241022
[REVIEW-ENGINE] Parallel execution: true
[REVIEW-ENGINE] Personas: security-engineer, code-health, error-handling-specialist
[SUB-AGENT] Provider selected: anthropic
[SUB-AGENT] Using Anthropic API with key: sk-ant-api...
[SUB-AGENT] Auto-TTL: 4 reviews expected → 1h recommended
```

This shows:
- Which provider is selected (anthropic vs vertexai-claude)
- API key prefix (for verification)
- Model being used
- Cache TTL selection
- Persona configuration

#### Cache Not Working

**Symptoms:**
- `--show-cache-metrics` shows 0% hit rate
- Costs same as without caching

**Causes:**
- Sub-agents disabled (`--no-sub-agents`)
- Persona size <1,024 tokens
- Different personas each review

**Solution:** Verify sub-agent and caching are enabled
```bash
# Check if sub-agents are enabled (should NOT see this flag)
multi-persona-review --help | grep no-sub-agents

# Verify persona sizes
multi-persona-review --list-personas --verbose
# Look for "Cache-eligible: Yes" in output

# Use --show-cache-metrics to diagnose
multi-persona-review --show-cache-metrics src/
```

### Debug Mode

Enable verbose logging to see what's happening:

```bash
multi-persona-review --verbose --dry-run src/
```

This shows:
- Working directory
- Persona search paths
- Loaded personas
- Selected personas
- Configuration
- Git metadata
- Cost sink setup

---

## Best Practices

### 1. Use Quick Mode for Daily Development

```bash
multi-persona-review --mode quick
```

- Fast feedback (30-60s)
- Low cost ($0.01-0.05)
- Catches most issues

### 2. Use Thorough Mode for PR Reviews

```bash
multi-persona-review --mode thorough src/ --format github > pr-comment.md
```

- Comprehensive review
- Multiple personas
- Worth the extra cost for quality

### 3. Enable Deduplication

```yaml
options:
  deduplicate: true
  similarityThreshold: 0.8
```

- Reduces noise by 50%+
- Merges similar findings
- Cleaner output

### 4. Track Costs

```yaml
costTracking:
  type: file
  config:
    filePath: ./costs.jsonl
```

- Monitor usage over time
- Identify expensive reviews
- Optimize persona selection

### 5. Create Project-Specific Personas

```yaml
personaPaths:
  - .engram/personas  # Project-specific
  - ~/.engram/personas # User-level
```

- Customize for your stack
- Domain-specific expertise
- Better findings

### 6. Use Parallel Execution

Enabled by default, but can be controlled:

```typescript
await reviewFiles(config, cwd, reviewer, {
  parallel: true,  // 3x faster
});
```

### 7. Exclude Generated Files

```yaml
github:
  exclude:
    - "**/node_modules/**"
    - "**/*.min.js"
    - "dist/**"
    - "build/**"
```

---

## FAQ

### How much does Multi-Persona Review cost?

Costs depend on usage:
- **Quick mode**: $0.01-0.05 per review
- **Thorough mode**: $0.10-0.30 per review
- **Typical team** (20 PRs/day): $5-20/month

### Can I use my own LLM instead of Claude?

Yes! Implement the `ReviewerFunction` interface:

```typescript
const customReviewer: ReviewerFunction = async (input) => {
  // Your LLM integration here
  return {
    persona: input.persona.name,
    findings: [...],
    cost: {...},
  };
};

await reviewFiles(config, cwd, customReviewer);
```

### How do I create a custom persona?

1. Create YAML file in personas directory
2. Define name, description, focusAreas, and prompt
3. Use with `--personas your-persona-name`

See [Personas](#personas) section for details.

### Can Multi-Persona Review auto-fix issues?

Not yet. Auto-fix framework is planned for Session 12 (deferred).

### Does Multi-Persona Review work with other languages?

Yes! It works with any language that Claude can understand:
- TypeScript/JavaScript
- Python
- Go
- Rust
- Java
- C++
- And more...

### How accurate are the findings?

Claude-powered reviews are highly accurate (90%+ precision) but should be reviewed by humans. Use severity levels to prioritize:
- **Critical/High**: Review immediately
- **Medium/Low**: Review during refactoring
- **Info**: Nice-to-have improvements

### Can I run Multi-Persona Review locally?

Yes! Multi-Persona Review is designed for both local development and CI/CD:

```bash
# Local development
multi-persona-review src/ --mode quick

# CI/CD
multi-persona-review src/ --mode thorough --format github
```

---

## Support

- **Issues**: https://github.com/vbonnet/engram/issues
- **Documentation**: This file
- **Examples**: See `/test` directory for code examples

---

**Last Updated**: 2025-11-23
**Version**: 0.1.0-alpha
