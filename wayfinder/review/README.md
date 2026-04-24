# Multi-Persona Review Plugin

[![CI](https://github.com/wayfinder/multi-persona-review/workflows/CI/badge.svg)](https://github.com/wayfinder/multi-persona-review/actions?query=workflow%3ACI)
[![codecov](https://codecov.io/gh/wayfinder/multi-persona-review/branch/main/graph/badge.svg)](https://codecov.io/gh/wayfinder/multi-persona-review)
[![npm version](https://badge.fury.io/js/%40wayfinder%2Fmulti-persona-review.svg)](https://www.npmjs.com/package/@wayfinder/multi-persona-review)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.0-blue)](https://www.typescriptlang.org/)
[![Node.js](https://img.shields.io/badge/Node.js-18%2B-green)](https://nodejs.org/)

Multi-persona code review system using AI personas with Agency-Agents collaboration patterns for comprehensive, intelligent code analysis.

---

## Table of Contents

- [Overview](#overview)
- [Key Features](#key-features)
- [Quick Start](#quick-start)
- [Agency-Agents Patterns](#agency-agents-patterns)
- [Installation](#installation)
- [Configuration](#configuration)
- [Usage](#usage)
  - [CLI Interface](#cli-interface)
  - [Programmatic API](#programmatic-api)
- [AI Providers](#ai-providers)
- [Personas](#personas)
- [Caching & Performance](#caching--performance)
- [CI/CD Integration](#cicd-integration)
- [Development](#development)
- [Architecture](#architecture)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

Multi-Persona Review is an automated code review system that leverages multiple AI personas (security engineer, performance reviewer, accessibility specialist, etc.) to provide comprehensive, multi-perspective feedback on your code.

### What Makes It Unique?

- **Agency-Agents Collaboration**: Personas vote on findings (GO/NO-GO), propose alternatives, and indicate expertise boundaries
- **300+ Tests**: Comprehensive test coverage with CI/CD automation
- **Smart Deduplication**: Groups similar findings across personas (50%+ noise reduction)
- **Multi-Provider Support**: Anthropic Claude, Google Vertex AI (Gemini & Claude)
- **Production-Ready**: Battle-tested with CI/CD integration, cost tracking, and error handling

**Current Version**: 0.1.0 | **Tests**: 300 passing | **Production Readiness**: 8.5/10

---

## Key Features

### Core Capabilities

- ✅ **Multi-Persona Reviews**: Run 3+ personas in parallel for comprehensive feedback
- ✅ **Agency-Agents Patterns**: GO/NO-GO voting, confidence scoring, lateral thinking alternatives
- ✅ **Smart Deduplication**: Similarity-based finding merging (50%+ noise reduction)
- ✅ **Multiple Output Formats**: Text (ANSI colors), JSON (CI/CD), GitHub PR comments
- ✅ **Cost Tracking**: GCP Cloud Monitoring, file-based, or stdout
- ✅ **Prompt Caching**: 86% cost savings with 99.5% cache hit rate
- ✅ **Adversarial Deliberation**: Opt-in multi-round debate between personas to resolve contradictions ([ADR-003](docs/adr/003-adversarial-deliberation.md))
- ✅ **CI/CD Optimized**: GitHub Actions workflows, exit codes, automated releases

### AI Provider Support

- **Anthropic Claude**: Direct API (claude-sonnet-4.5, claude-haiku-4.5, claude-opus-4.6)
- **Vertex AI Gemini**: Google Gemini models (gemini-2.5-flash, gemini-2.5-flash-lite)
- **Vertex AI Claude**: Claude via Vertex AI (recommended for GCP users)

### Quality & Automation

- **CI/CD**: 4 GitHub Actions workflows (ci, release, security, coverage)
- **Pre-commit Hooks**: Conventional commits, ESLint, Prettier
- **Automated Releases**: semantic-release with CHANGELOG generation
- **Dependency Updates**: Dependabot weekly scans
- **Security**: npm audit + CodeQL daily scans

---

## Quick Start

### Installation

```bash
npm install @wayfinder/multi-persona-review
```

### Basic Usage

```bash
# Review current directory
multi-persona-review

# Review specific files
multi-persona-review src/index.ts src/utils.ts

# Thorough review with custom personas
multi-persona-review src/ --mode thorough --personas security-engineer,performance-reviewer

# Output as JSON for CI/CD
multi-persona-review src/ --format json > review.json

# Agency-Agents: Review with voting
multi-persona-review src/ --vote-threshold 0.7 --min-confidence 0.8
```

### Environment Setup

```bash
# Anthropic (Claude Direct API)
export ANTHROPIC_API_KEY=your_api_key_here
multi-persona-review src/

# OR VertexAI Claude (recommended for GCP users)
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-east5
export VERTEX_MODEL=claude-sonnet-4-5@20250929
multi-persona-review src/ --provider vertexai-claude

# OR VertexAI Gemini
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-central1
export VERTEX_MODEL=gemini-2.5-flash
multi-persona-review src/ --provider vertexai

# Non-git usage (no repository required)
cd /tmp/my-code
multi-persona-review . --full
```

### Running in Claude Code

When running in a Claude Code session with VertexAI authentication, multi-persona-review automatically detects and uses your Claude Code credentials. **No flags needed!**

```bash
# Auto-detects Claude Code VertexAI session
multi-persona-review src/

# Explicitly use Anthropic API instead
multi-persona-review src/ --provider anthropic --api-key sk-ant-...
```

**Environment variables detected:**
- `CLAUDE_CODE_USE_VERTEX=1` - Indicates Claude Code is using VertexAI
- `ANTHROPIC_VERTEX_PROJECT_ID` - Your GCP project ID
- `CLOUD_ML_REGION` - Your VertexAI region (defaults to us-east5)

These are automatically set by Claude Code and inherited by child processes. The CLI will:
1. Detect `CLAUDE_CODE_USE_VERTEX=1` + project ID present
2. Auto-select `vertexai-claude` provider
3. Use Claude Code's VertexAI credentials seamlessly

**Precedence order** (highest to lowest):
1. Explicit CLI flags (`--provider`, `--vertex-project`)
2. Claude Code environment variables
3. Standard environment variables (`VERTEX_PROJECT_ID`, `ANTHROPIC_API_KEY`)
4. Default values

### Full-Document Review Mode

By default, multi-persona-review uses git diff mode when in a git repository. For complete file analysis without git dependency, use the `--full` flag:

```bash
# Review entire files without git diff
multi-persona-review src/ --full

# Same as --scan full
multi-persona-review src/ --scan full

# Works in non-git directories
cd /tmp/my-code
multi-persona-review . --full
```

**Use cases:**
- **Non-git repositories**: Review code not tracked in git
- **Complete analysis**: Review entire file context, not just changes
- **Initial codebase review**: First-time review of legacy code
- **Temporary directories**: Analyze generated or downloaded code

**Note**: Full-document mode uses more tokens than diff mode. Use `--mode quick` to reduce cost.

### First Review Example

```bash
# Initialize configuration
multi-persona-review init

# List available personas
multi-persona-review --list-personas

# Run your first review
multi-persona-review src/ --mode quick --format text

# Output:
# 🔍 Security Engineer found 3 issues:
#   ✗ CRITICAL: SQL injection vulnerability (src/db.ts:42)
#     Decision: NO-GO | Confidence: 0.95
#     Alternatives: [Use parameterized queries, ORM, prepared statements]
#   ...
#
# 📊 Summary: 5 findings (2 critical, 3 high)
# 💰 Cost: $0.12 | ⚡ Cache savings: 86%
```

---

## Agency-Agents Patterns

Multi-Persona Review implements Agency-Agents collaboration patterns for enhanced multi-persona decision-making:

### 1. **Voting Mechanism** (GO/NO-GO)

Personas vote on code quality with tier-weighted aggregation:

```bash
multi-persona-review src/ --vote-threshold 0.7

# Output shows aggregated vote:
#   Weighted Vote: 0.85 (GO) - Tier 1 personas voted GO
#   Decision: APPROVE for merge
```

**Tier Weighting**:
- **Tier 1** (Senior): 3x weight (security-engineer, tech-lead)
- **Tier 2** (Mid-level): 1x weight (code-health, performance-reviewer)
- **Tier 3** (Junior): 0.5x weight (documentation-reviewer)

**Vote Aggregation**:
```
weighted_vote = (GO_votes * weights) / total_weights
Result: GO if weighted_vote > threshold (default: 0.5)
```

### 2. **Confidence Scoring**

Filter findings by AI confidence level (0-1 scale):

```bash
multi-persona-review src/ --min-confidence 0.8

# Only shows findings with ≥80% confidence
```

**Confidence Levels**:
- **0.9+**: Very certain (typically algorithmic issues)
- **0.7-0.9**: High confidence (pattern-based issues)
- **0.5-0.7**: Moderate confidence (style/best practices)
- **<0.5**: Low confidence (suggestions, edge cases)

### 3. **Lateral Thinking** (Alternative Approaches)

Personas propose 3 alternative solutions before final recommendation:

```
Finding: SQL injection vulnerability
Alternatives:
  1. Use parameterized queries with pg library
  2. Migrate to Prisma ORM with type-safe queries
  3. Implement stored procedures with input validation
Recommended: Option 1 (least disruptive, immediate fix)
```

### 4. **Expertise Boundary Detection**

Personas flag findings outside their expertise and route to appropriate persona:

```
🔍 Performance Reviewer:
  ⚠️ OUT OF SCOPE: Cryptography implementation (src/auth.ts:89)
  → Routing to Security Engineer for review
```

**Benefits**:
- Reduces false positives
- Improves finding quality
- Ensures proper expertise allocation

---

## Installation

### As npm Package

```bash
npm install @wayfinder/multi-persona-review

# Verify installation
multi-persona-review --version
```

### For Development

```bash
git clone https://github.com/wayfinder/multi-persona-review.git
cd multi-persona-review
npm install
npm test          # 300 tests should pass
npm run build
```

### System Requirements

- **Node.js**: ≥18.0.0
- **npm**: ≥9.0.0
- **OS**: Linux, macOS, Windows (WSL recommended)

---

## Configuration

### Configuration File

Create `.engram/config.yml`:

```yaml
crossCheck:
  defaultMode: quick  # or: thorough, custom
  defaultPersonas:
    - security-engineer
    - code-health
    - error-handling-specialist
    - performance-reviewer

  options:
    deduplicate: true
    similarityThreshold: 0.8
    voteThreshold: 0.5          # Agency-Agents: Vote threshold
    minConfidence: 0.7          # Agency-Agents: Min confidence

  costTracking:
    type: stdout  # or: gcp, file
    gcpProject: my-project-id   # if type: gcp
    costFile: ./costs.jsonl     # if type: file
```

### Environment Variables

```bash
# Anthropic (Claude)
export ANTHROPIC_API_KEY=your_api_key_here

# Vertex AI (Claude - recommended)
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-east5
export VERTEX_MODEL=claude-sonnet-4-5@20250929

# Vertex AI (Gemini)
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-central1
export VERTEX_MODEL=gemini-2.5-flash

# Cost tracking
export GCP_PROJECT_ID=your_project_id  # for GCP cost sink

# Caching (optional)
export MULTI_PERSONA_REVIEW_COUNT=5   # For batch reviews
export MULTI_PERSONA_BATCH_MODE=true  # Aggressive caching
```

---

## Usage

### CLI Interface

#### Basic Commands

```bash
# Initialize project
multi-persona-review init

# List available personas
multi-persona-review --list-personas

# Review current directory (quick mode)
multi-persona-review

# Review specific files
multi-persona-review src/index.ts src/utils.ts

# Thorough review
multi-persona-review src/ --mode thorough

# Custom personas
multi-persona-review src/ --personas security-engineer,tech-lead

# Preview (dry-run)
multi-persona-review --dry-run src/

# Verbose output
multi-persona-review --verbose src/
```

#### Agency-Agents Commands

```bash
# Vote-based review (default threshold: 0.5)
multi-persona-review src/ --vote-threshold 0.7

# Confidence filtering
multi-persona-review src/ --min-confidence 0.8

# Combined Agency-Agents review
multi-persona-review src/ \
  --vote-threshold 0.7 \
  --min-confidence 0.8 \
  --mode thorough
```

#### Output Formats

```bash
# Text output with colors (default)
multi-persona-review src/ --format text

# JSON output for CI/CD
multi-persona-review src/ --format json > review.json

# GitHub PR comment markdown
multi-persona-review src/ --format github > pr-comment.md

# No colors (for logs)
multi-persona-review src/ --no-colors
```

#### AI Provider Selection

```bash
# Anthropic (default if ANTHROPIC_API_KEY set)
multi-persona-review src/ --api-key YOUR_API_KEY
multi-persona-review src/ --model claude-3-5-sonnet-20241022

# Vertex AI Gemini
multi-persona-review src/ --provider vertexai --vertex-project my-project

# Vertex AI Claude (recommended for GCP)
multi-persona-review src/ --provider vertexai-claude --vertex-project my-project
```

#### Cost Tracking

```bash
# GCP Cloud Monitoring
multi-persona-review src/ --cost-sink gcp --gcp-project my-project

# File-based
multi-persona-review src/ --cost-sink file --cost-file ./costs.jsonl

# Stdout (default)
multi-persona-review src/ --cost-sink stdout
```

#### Caching Options

```bash
# Show cache metrics
multi-persona-review src/ --show-cache-metrics

# Disable caching
multi-persona-review src/ --no-cache

# Batch reviews (auto-caching)
export MULTI_PERSONA_BATCH_MODE=true
multi-persona-review src/file1.ts
multi-persona-review src/file2.ts  # Cache hit!
multi-persona-review src/file3.ts  # Cache hit!
```

### Programmatic API

#### Using Anthropic (Claude)

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
} from '@wayfinder/multi-persona-review';

// Load personas
const personas = await loadPersonas([
  '.engram/personas',
  '~/.engram/personas',
]);

// Create reviewer
const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
});

// Run review with Agency-Agents patterns
const result = await reviewFiles(
  {
    files: ['src/index.ts'],
    personas: [personas.get('security-engineer')!],
    mode: 'thorough',
    options: {
      voteThreshold: 0.7,        // Agency-Agents voting
      minConfidence: 0.8,        // Agency-Agents confidence
    },
  },
  process.cwd(),
  reviewer
);

// Format results
const formatted = formatReviewResult(result, {
  colors: true,
  showCost: true,
  showSummary: true,
});

console.log(formatted);
```

#### Using Vertex AI (Claude)

```typescript
import {
  loadPersonas,
  reviewFiles,
  createVertexAIClaudeReviewer,
} from '@wayfinder/multi-persona-review';

const reviewer = createVertexAIClaudeReviewer({
  projectId: process.env.VERTEX_PROJECT_ID!,
  location: 'us-east5',
  model: 'claude-sonnet-4-5@20250929',
});

const result = await reviewFiles(
  {
    files: ['src/index.ts'],
    personas: [personas.get('security-engineer')!],
    mode: 'thorough',
  },
  process.cwd(),
  reviewer
);
```

---

## VertexAI Sub-Agent Support

Multi-Persona Review supports **sub-agent orchestration** using VertexAI Claude models for running persona reviews. This provides better integration with GCP infrastructure and unified billing.

### What are Sub-Agents?

Sub-agents are individual AI instances that execute each persona review independently. Instead of running all personas in a single API call, the orchestrator:

1. **Creates dedicated sub-agents** for each persona (security-engineer, code-health, etc.)
2. **Executes reviews in parallel** for faster results
3. **Aggregates findings** and deduplicates across personas
4. **Tracks costs** per persona and total review

### Benefits of VertexAI Sub-Agents

- ✅ **GCP Integration**: Unified billing, committed use discounts, IAM controls
- ✅ **Prompt Caching**: 5-minute cache per sub-agent (86% cost savings)
- ✅ **Parallel Execution**: Reviews run simultaneously (3x faster)
- ✅ **Better Error Handling**: Individual persona failures don't block review
- ✅ **Cost Attribution**: Track costs per persona for optimization

### Sub-Agent Provider Comparison

| Feature | Anthropic Direct | VertexAI Claude | VertexAI Gemini |
|---------|------------------|-----------------|-----------------|
| **Sub-Agent Support** | ✅ Yes | ✅ Yes | ✅ Yes |
| **Prompt Caching** | ✅ 5 min | ✅ 5 min | ❌ No |
| **Parallel Execution** | ✅ Yes | ✅ Yes | ✅ Yes |
| **Authentication** | API Key | ADC / Service Account | ADC / Service Account |
| **Billing** | Direct Anthropic | GCP Billing | GCP Billing |
| **Cost** | $3/$15 per 1M tokens | $3/$15 per 1M tokens | $0.15/$0.60 per 1M tokens |
| **Cache Savings** | 90% read discount | 90% read discount | N/A |
| **Claude Code Auto-Detect** | ❌ No | ✅ Yes | ❌ No |
| **Best For** | Direct Anthropic users | GCP users, Claude Code | Cost-sensitive, experimental |

### Quick Start with VertexAI Sub-Agents

```bash
# 1. Authenticate with GCP
gcloud auth application-default login

# 2. Set environment variables
export VERTEX_PROJECT_ID=your-gcp-project
export VERTEX_LOCATION=us-east5

# 3. Run review (auto-detects vertexai-claude provider)
multi-persona-review src/ --provider vertexai-claude

# Or let it auto-detect in Claude Code
multi-persona-review src/  # Automatically uses VertexAI if CLAUDE_CODE_USE_VERTEX=1
```

### Sub-Agent Example Output

```bash
$ multi-persona-review src/auth.ts --provider vertexai-claude --show-cache-metrics

🔍 Starting multi-persona review with 3 personas...

✅ security-engineer    (tokens: 2,145 | cache: 1,850 read | cost: $0.02)
✅ code-health          (tokens: 1,987 | cache: 1,650 read | cost: $0.02)
✅ error-handling       (tokens: 2,034 | cache: 1,700 read | cost: $0.02)

📊 Cache Metrics:
   Hit Rate: 88.4% (5,200/5,876 tokens from cache)
   Cost Savings: $0.45 → $0.06 (86% reduction)

🔍 Security Engineer found 2 issues:
  ✗ CRITICAL: SQL injection vulnerability (src/auth.ts:42)
  ...

💰 Total Cost: $0.06 | ⚡ Cache Savings: 86%
```

## AI Providers

### Anthropic (Claude) - Direct API

**Authentication**:
```bash
export ANTHROPIC_API_KEY=your_api_key_here
```

**Supported Models**:
- `claude-3-5-sonnet-20241022` (default) - Best balance
- `claude-3-5-haiku-20241022` - Fastest, most cost-effective
- `claude-3-opus-20240229` - Highest quality

**Pricing** (per 1M tokens):
- Sonnet: $3 (input) / $15 (output)
- Haiku: $0.25 (input) / $1.25 (output)
- Opus: $15 (input) / $75 (output)

### Vertex AI (Claude) - Recommended for GCP

**Authentication**:
```bash
gcloud auth application-default login
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-east5
export VERTEX_MODEL=claude-sonnet-4-5@20250929
```

**Supported Models**:
- `claude-sonnet-4-5@20250929` (recommended)
- `claude-haiku-4-5@20251001` - Fastest
- `claude-opus-4-6@20260205` - Highest quality

**Benefits**:
- ✅ Unified GCP billing
- ✅ Committed use discounts (up to 30% savings)
- ✅ Better GCP infrastructure integration
- ✅ Same Claude models, same quality

### Vertex AI (Gemini)

**Authentication**:
```bash
export VERTEX_PROJECT_ID=your_gcp_project_id
export VERTEX_LOCATION=us-central1
export VERTEX_MODEL=gemini-2.5-flash
```

**Supported Models**:
- `gemini-2.5-flash` (recommended) - Fast and capable
- `gemini-2.5-flash-lite` - Most cost-effective

---

## Personas

### Built-in Personas

Multi-Persona Review includes 8 production-ready personas:

| Persona | Focus Areas | Tier | Tokens |
|---------|-------------|------|--------|
| **security-engineer** | OWASP Top 10, auth, injection | 1 | 1,523 |
| **tech-lead** | Architecture, patterns, scalability | 1 | 1,421 |
| **code-health** | Maintainability, readability, DRY | 2 | 1,398 |
| **performance-reviewer** | Algorithms, caching, optimization | 2 | 1,312 |
| **error-handling-specialist** | Exceptions, validation, recovery | 2 | 1,287 |
| **accessibility-specialist** | WCAG, ARIA, keyboard nav | 2 | 1,354 |
| **qa-engineer** | Testing, edge cases, validation | 3 | 1,402 |
| **documentation-reviewer** | Comments, README, examples | 3 | 1,289 |

All personas are cache-optimized (≥1,024 tokens) for 86% cost savings.

### Persona File Formats

#### .ai.md Format (Recommended)

```yaml
---
name: security-engineer
displayName: Security Engineer
version: 1.0.0
description: Security specialist focused on OWASP Top 10
focusAreas:
  - authentication
  - authorization
  - input-validation
tier: 1  # Agency-Agents: Vote weight
---

# Security Engineer

Review code for security vulnerabilities following OWASP Top 10...

[Expand to ≥1,024 tokens for cache eligibility]
```

#### Persona Search Paths

1. `~/.wayfinder/personas` - User overrides
2. `.wayfinder/personas` - Project-specific
3. `{company}/personas` - Company-level (if configured)
4. `{personas}` - Personas plugin library
5. `{core}/personas` - Legacy fallback

### Creating Custom Personas

See [Persona Optimization Guide](docs/persona-optimization-guide.md) for best practices.

---

## Caching & Performance

### Prompt Caching (86% Cost Savings)

Multi-Persona Review uses Anthropic's prompt caching for massive cost savings:

**Performance Metrics**:
- **Cache Hit Rate**: 99.5% (across 96 files)
- **Cost Savings**: 86% for large reviews, 55% for small reviews
- **Break-even**: Just 2 cache hits within 5-minute window

**How It Works**:
1. Persona definitions cached (≥1,024 tokens)
2. Cache TTL: 5 minutes (Anthropic's ephemeral cache)
3. Automatic strategy selection based on review count

**CLI Options**:
```bash
# Show cache metrics
multi-persona-review src/ --show-cache-metrics

# Batch reviews (maximize cache hits)
export MULTI_PERSONA_BATCH_MODE=true
multi-persona-review src/file1.ts --show-cache-metrics
multi-persona-review src/file2.ts --show-cache-metrics  # Cache hit!

# Disable caching (higher costs)
multi-persona-review src/ --no-cache
```

**Auto-Strategy Selection**:
- ≥4 reviews expected → Aggressive caching (worth +25% write cost)
- <4 reviews expected → Conservative caching

### Parallel Execution

Reviews run in parallel by default (3x faster):

```bash
# Parallel execution (default)
multi-persona-review src/

# Sequential execution (fallback)
multi-persona-review src/ --sequential
```

---

## CI/CD Integration

### GitHub Actions

Add to `.github/workflows/code-review.yml`:

```yaml
name: Code Review

on:
  pull_request:
    branches: [main]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @wayfinder/multi-persona-review

      - name: Run Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review src/ \
            --format github \
            --vote-threshold 0.7 \
            --min-confidence 0.8 > review.md

      - name: Comment on PR
        uses: actions/github-script@v7
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
```

### Exit Codes

Multi-Persona Review uses exit codes for CI/CD:

- **Exit 0**: No critical/high findings (safe to merge)
- **Exit 1**: Critical or high findings detected (blocks merge)

```bash
multi-persona-review src/ || echo "Review failed, blocking merge"
```

### Cost Tracking in CI/CD

```yaml
- name: Run Review with Cost Tracking
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
    GCP_PROJECT_ID: ${{ secrets.GCP_PROJECT_ID }}
  run: |
    multi-persona-review src/ \
      --cost-sink gcp \
      --gcp-project $GCP_PROJECT_ID \
      --show-cache-metrics
```

---

## Development

### Setup

```bash
git clone https://github.com/wayfinder/multi-persona-review.git
cd multi-persona-review
npm install
npm run build
npm test  # 300 tests should pass
```

### Scripts

```bash
# Development
npm run build          # Build TypeScript
npm run type-check     # TypeScript compilation check
npm test               # Run tests (300 tests)
npm run test:watch     # Watch mode
npm run test:coverage  # Coverage report

# Code Quality
npm run lint           # ESLint
npm run lint:fix       # Auto-fix linting errors
npm run format         # Prettier formatting
npm run format:check   # Check formatting

# Pre-commit hooks automatically run:
# - ESLint --fix on staged files
# - Prettier --write on staged files
# - Conventional commit validation
```

### Running Tests

```bash
# All tests
npm test

# Specific test file
npm test tests/unit/agency-agents.test.ts

# Watch mode
npm run test:watch

# Coverage
npm run test:coverage
```

**Test Suite**:
- **300 tests** (all passing)
- **Unit tests**: 293 tests
- **Integration tests**: 7 tests (11 skipped - require API keys)
- **Test files**: 14 files
- **Coverage**: >80% (target)

### Pre-commit Hooks

Husky automatically runs on commits:

1. **lint-staged**: ESLint + Prettier on staged files
2. **commitlint**: Validates conventional commits format

If commits fail:
```bash
npm run lint:fix    # Fix linting errors
npm run format      # Format code
git add .
git commit -m "feat: your message"  # Conventional format
```

---

## Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         CLI / API Entry                         │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Configuration Layer                          │
│  ┌────────────┐  ┌─────────────┐  ┌────────────┐               │
│  │ Config     │  │ Persona     │  │ File       │               │
│  │ Loader     │  │ Loader      │  │ Scanner    │               │
│  └────────────┘  └─────────────┘  └────────────┘               │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Review Engine Core                         │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Review Orchestration (review-engine.ts)                  │  │
│  │  - Prepare context (scan files)                          │  │
│  │  - Run persona reviews (parallel/sequential)             │  │
│  │  - Agency-Agents: Aggregate votes, filter by confidence │  │
│  │  - Deduplicate findings                                  │  │
│  └──────────────────────────────────────────────────────────┘  │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                      LLM Client Layer                            │
│  ┌─────────────┐  ┌──────────────┐  ┌────────────────┐         │
│  │ Anthropic   │  │ VertexAI     │  │ VertexAI       │         │
│  │ Client      │  │ Gemini       │  │ Claude         │         │
│  └─────────────┘  └──────────────┘  └────────────────┘         │
└─────────────────────────┬───────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Post-Processing Layer                          │
│  ┌────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │ Dedup      │  │ Cost        │  │ Formatters  │              │
│  │ (similarity)│  │ Tracking    │  │ (text/json) │              │
│  └────────────┘  └─────────────┘  └─────────────┘              │
└─────────────────────────────────────────────────────────────────┘
```

### Agency-Agents Layer

Agency-Agents patterns enhance multi-persona collaboration:

1. **Voting Mechanism**: Tier-weighted GO/NO-GO decisions
2. **Confidence Scoring**: Filter findings by AI confidence (0-1 scale)
3. **Lateral Thinking**: 3 alternative approaches before final recommendation
4. **Expertise Boundary Detection**: Route findings to appropriate personas

See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed documentation.

---

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

### Quick Contribution Guide

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Make changes and add tests
4. Run tests: `npm test` (all 300 tests must pass)
5. Run linting: `npm run lint` (0 errors required)
6. Commit with conventional commits: `git commit -m "feat: add feature"`
7. Push and create PR

### Pull Request Requirements

- ✅ All tests pass (300/300)
- ✅ Linting passes (0 errors)
- ✅ Type checking passes
- ✅ Documentation updated
- ✅ Conventional commit messages
- ✅ CI/CD checks pass

### Development Workflow

Pre-commit hooks automatically enforce:
- ESLint + Prettier on staged files
- Conventional commit format

CI/CD runs on every PR:
- Lint, type check, tests, build
- Matrix testing (Node 18, 20, 22)
- Security scanning (npm audit + CodeQL)
- Code coverage reporting

---

## License

Apache-2.0 License. See [LICENSE](LICENSE) for details.

---

## Resources

- [Documentation](DOCUMENTATION.md) - Complete API reference
- [Architecture](ARCHITECTURE.md) - System design and data flow
- [Specification](SPEC.md) - Functional specification
- [Contributing](CONTRIBUTING.md) - Contribution guidelines
- [Security](SECURITY.md) - Vulnerability reporting
- [Changelog](CHANGELOG.md) - Release history

---

## Support

- **Issues**: [GitHub Issues](https://github.com/wayfinder/multi-persona-review/issues)
- **Discussions**: [GitHub Discussions](https://github.com/wayfinder/multi-persona-review/discussions)
- **Security**: See [SECURITY.md](SECURITY.md) for responsible disclosure

---

**Built with ❤️ using Agency-Agents patterns | Production-Ready | 300+ Tests | 8.5/10 Production Score**
