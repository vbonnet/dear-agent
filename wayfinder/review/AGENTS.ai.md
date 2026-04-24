---
type: guidance
plugin: multi-persona-review
version: 0.1.0
---

# Multi-Persona Review Plugin - AI Agent Guide

**Type:** Guidance Plugin
**Purpose:** Multi-perspective code review with parallel AI personas for comprehensive feedback

---

## Quick Start

**For AI Agents performing code reviews:**

1. **Load relevant personas:**
   ```
   Read tool: ~/.engram/core/plugins/personas/library/{category}/{persona-name}.ai.md
   ```

2. **Apply personas to code:**
   ```
   You are reviewing code using multi-persona analysis.
   Personas loaded: [list of personas]
   Code to review: [diff or file content]
   ```

3. **Synthesize results:**
   ```
   Deduplicate findings, group by severity, format as comprehensive review
   ```

---

## Overview

Multi-persona review runs parallel code reviews using multiple AI personas (security engineer, code quality specialist, performance reviewer, etc.) to provide comprehensive feedback on code changes. Reduces blind spots through cross-validation of findings from multiple expert perspectives.

**Key Capabilities:**
- **Parallel execution** - 3x faster than sequential reviews
- **Smart deduplication** - Groups similar findings across personas (50%+ noise reduction)
- **Multiple output formats** - Text, JSON, GitHub PR comments
- **CI/CD optimized** - Designed for ephemeral GitHub Actions runners
- **Cost tracking** - GCP, AWS, Datadog, file, or stdout logging
- **Token efficiency** - Distilled personas (≤1,500 tokens each) with prompt caching

**Historical Effectiveness:**
- 88% token reduction with distilled personas (vs verbose personas)
- 96.5% total savings with prompt caching combined
- 50%+ noise reduction through deduplication
- Production-ready with 104 passing tests

---

## When to Use Multi-Persona Review

### Always Use For
- ✅ Pull request reviews (GitHub, GitLab, Bitbucket)
- ✅ Pre-commit code quality checks
- ✅ Architecture review of design documents
- ✅ Security-sensitive changes (auth, encryption, PII)
- ✅ API design reviews
- ✅ CI/CD pipeline validation

### Consider For
- ⏳ Refactoring reviews (ensure no regressions)
- ⏳ Documentation review (technical accuracy)
- ⏳ Configuration changes (infra, deployment)

### Skip For
- ❌ Trivial typo fixes
- ❌ Comment-only changes (no code logic)
- ❌ Auto-generated code (migrations, lockfiles)

---

## Persona Usage Examples

### Pattern 1: GitHub PR Review (Claude Code)

**User request:** "Review PR 133 in myorg/backend-api"

**Agent workflow:**
```bash
# 1. Fetch PR diff
gh pr diff 133 --repo myorg/backend-api > /tmp/pr-133.diff

# 2. Read diff
Read /tmp/pr-133.diff

# 3. Determine PR type (backend/API) and load appropriate personas
Read ~/.engram/core/plugins/personas/library/security/security-engineer.ai.md
Read ~/.engram/core/plugins/personas/library/development/api-design-reviewer.ai.md
Read ~/.engram/core/plugins/personas/library/development/error-handling-reviewer.ai.md

# 4. Apply each persona perspective to the diff
# - Analyze changes from security perspective
# - Analyze changes from API design perspective
# - Analyze changes from error handling perspective
# - Record diff positions (line numbers) for inline comments
# - Rate severity (CRITICAL, HIGH, MEDIUM, LOW)

# 5. Deduplicate findings across personas
# - Group similar issues
# - Keep highest severity rating
# - Note which personas agreed

# 6. Format review (markdown for preview)
Write /tmp/review-133.md

# 7. Preview and ask for approval
"I've completed the multi-persona review of PR #133.

Found:
- 0 critical issues
- 2 high-priority issues (API design, error handling)
- 5 medium issues

Full review: /tmp/review-133.md

Would you like me to post this as inline comments (draft mode)?"

# 8. If approved, post as draft PR review
gh-post-review --input /tmp/review-133.json --draft
```

**IMPORTANT:** Only comment on **added lines** (marked with `+` in diff). Issues with existing code go in summary section.

### Pattern 2: Pre-Commit Quality Check (CLI)

**User scenario:** Developer runs review before committing changes

```bash
# Review uncommitted changes (diff mode)
multi-persona-review --scan diff --mode quick --personas security-engineer,code-quality-reviewer

# Review specific files before commit
multi-persona-review src/auth.ts src/api.ts --mode thorough

# Output as JSON for CI/CD
multi-persona-review src/ --format json --scan diff > review.json
```

**Exit codes:**
- `0` - No critical/high issues found (safe to commit)
- `1` - Critical or high-severity issues found (block commit)

### Pattern 3: Wayfinder S6 Design Review

**During S6 (Design) phase with blocking review gate:**

```bash
# AI agent loads design doc and applies multi-persona review
Read design-doc.md

# Load required personas for S6 (architecture review)
Read ~/.engram/core/plugins/personas/library/architecture/tech-lead.ai.md
Read ~/.engram/core/plugins/personas/library/security/security-engineer.ai.md
Read ~/.engram/core/plugins/personas/library/quality/qa-engineer.ai.md

# Load domain expert (detected during S4)
Read ~/.engram/core/plugins/personas/library/data/ml-engineer.ai.md

# Apply each persona to design doc
# - Tech Lead: Check architecture, scalability, maintainability
# - Security Engineer: Threat modeling, secure design patterns
# - QA Engineer: Testability, edge cases, validation strategy
# - ML Engineer: Model evaluation, bias detection, fairness

# Synthesize feedback
# - Aggregate findings by severity
# - Identify blocking issues (require resolution before S7)
# - Format as structured review

# Decision gate
# - ✅ All required personas approved → Proceed to S7
# - ❌ Any persona blocked → Address issues, re-invoke S6
```

---

## Review Patterns by Use Case

### Backend/API Changes

**Recommended personas:**
- `security-engineer` - OWASP Top 10, auth, input validation
- `api-design-reviewer` - REST/GraphQL best practices, versioning
- `error-handling-reviewer` - Error handling, resilience patterns
- `database-specialist` (if DB changes) - Query optimization, schema design

**Focus areas:**
- Authentication/authorization checks
- Input validation and sanitization
- Error handling and logging
- API contract stability
- Performance implications

### Frontend/UI Changes

**Recommended personas:**
- `accessibility-specialist` - WCAG compliance, screen readers, keyboard nav
- `code-quality-reviewer` - Code smells, maintainability
- `error-handling-reviewer` - Error states, fallback UI
- `user-advocate` - User experience, usability

**Focus areas:**
- Accessibility (keyboard nav, ARIA labels, color contrast)
- Error states and loading states
- Performance (bundle size, lazy loading)
- User experience consistency

### Infrastructure/DevOps Changes

**Recommended personas:**
- `security-engineer` - Secrets management, least privilege
- `devops-engineer` - CI/CD, deployment strategy, rollback
- `performance-reviewer` - Resource limits, scaling, monitoring

**Focus areas:**
- Secrets and credentials handling
- Deployment safety (rollback, canary)
- Monitoring and alerting
- Cost implications

### Database Changes

**Recommended personas:**
- `security-engineer` - SQL injection, PII handling
- `database-specialist` - Schema design, query optimization
- `performance-reviewer` - Indexing, query performance
- `data-privacy` (if PII) - GDPR/CCPA compliance

**Focus areas:**
- Migration safety (rollback plan)
- Index design and query performance
- Data privacy and retention policies
- Schema versioning

---

## Token Optimization Guidance

### Persona Token Budgets

**Per persona:**
- Target: 1,000-1,200 tokens
- Maximum: 1,500 tokens
- Format: .ai.md (distilled, for agent consumption)

**Multi-persona reviews:**
- 3 personas: ~3,150 tokens (1.5% of Claude context)
- 5 personas: ~5,250 tokens (2.6% of context)
- 7 personas: ~7,350 tokens (3.7% of context)

**Compared to verbose personas:**
- Before: 3 personas = ~27,000 tokens (13% of context)
- After: 3 personas = ~3,150 tokens (1.5% of context)
- **Savings: 88% reduction** ✅

### Prompt Caching Strategy

**Enable prompt caching for repeated reviews:**

```typescript
// First review: Full token cost (3,150 tokens)
const review1 = await reviewFiles({
  files: ['src/auth.ts'],
  personas: [security, apiDesign, errorHandling],
  mode: 'thorough',
});

// Subsequent reviews: 90% cache discount (~315 tokens)
const review2 = await reviewFiles({
  files: ['src/user.ts'],
  personas: [security, apiDesign, errorHandling], // Same personas = cached
  mode: 'thorough',
});
```

**Total savings with caching: 96.5%** ✅

**When caching applies:**
- Same personas across multiple reviews
- Same LLM session/context window
- Personas loaded in same order

**When caching doesn't apply:**
- New LLM session (restart)
- Different persona order
- Modified persona content

### Token Optimization Tips

**DO:**
- ✅ Use .ai.md distilled personas (NOT .why.md verbose versions)
- ✅ Reuse same persona set across reviews (maximize caching)
- ✅ Load personas in consistent order
- ✅ Use `--scan diff` mode (review only changed lines)
- ✅ Select minimal persona set for task (3-5 personas, not all 23)

**DON'T:**
- ❌ Load .why.md files during reviews (educational only, 5-10x larger)
- ❌ Load all 23 personas for every review (overkill, token waste)
- ❌ Change persona order between reviews (breaks caching)
- ❌ Use `--scan full` mode unnecessarily (reviews entire codebase)

---

## Programmatic API Usage

### Basic Review Workflow

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
} from '@engram/multi-persona-review';

// 1. Load personas from search paths
const personas = await loadPersonas([
  '.engram/personas',           // Project-specific overrides
  '~/.engram/personas',          // User overrides
  '~/.engram/core/plugins/personas/library', // Core personas
]);

// 2. Select personas for review
const selectedPersonas = [
  personas.get('security-engineer')!,
  personas.get('code-quality-reviewer')!,
  personas.get('error-handling-reviewer')!,
];

// 3. Create Anthropic reviewer
const reviewer = createAnthropicReviewer({
  apiKey: process.env.ANTHROPIC_API_KEY!,
  model: 'claude-3-5-sonnet-20241022',
});

// 4. Run review (parallel execution by default)
const result = await reviewFiles(
  {
    files: ['src/index.ts', 'src/utils.ts'],
    personas: selectedPersonas,
    mode: 'thorough', // quick | thorough
    fileScanMode: 'diff', // full | diff | changed
    deduplicate: true, // Enable smart deduplication
    similarityThreshold: 0.8, // 80% similarity = duplicate
  },
  process.cwd(),
  reviewer
);

// 5. Format results
const formatted = formatReviewResult(result, {
  format: 'text', // text | json | github
  colors: true,
  groupByFile: true,
  showCost: true,
  showSummary: true,
});

console.log(formatted);

// 6. Check exit code based on severity
const hasBlockingIssues = result.findings.some(
  (f) => f.severity === 'CRITICAL' || f.severity === 'HIGH'
);
process.exit(hasBlockingIssues ? 1 : 0);
```

### Advanced: Custom Cost Tracking

```typescript
import { reviewFiles, createGCPCostSink } from '@engram/multi-persona-review';

// GCP Cloud Monitoring integration
const costSink = createGCPCostSink({
  projectId: 'my-gcp-project',
  metricType: 'custom.googleapis.com/ai/review_cost',
});

const result = await reviewFiles(
  {
    files: ['src/'],
    personas: selectedPersonas,
    mode: 'quick',
    costSink, // Sends cost metrics to GCP
  },
  process.cwd(),
  reviewer
);

// Cost data automatically logged to GCP Cloud Monitoring
// - Input tokens, output tokens, cache read/write
// - Model, timestamp, git metadata (branch, commit, author)
```

---

## CLI Interface

### Common Commands

```bash
# Initialize project (creates .engram/config.yml)
multi-persona-review init

# List available personas with descriptions
multi-persona-review --list-personas

# Quick review of uncommitted changes (git diff)
multi-persona-review --scan diff --mode quick

# Thorough review of specific files
multi-persona-review src/auth.ts src/api.ts --mode thorough

# Review with custom persona selection
multi-persona-review src/ --personas security-engineer,performance-reviewer

# Preview what would be reviewed (dry-run)
multi-persona-review --dry-run src/

# Verbose output for debugging
multi-persona-review --verbose src/

# Output as JSON for CI/CD pipelines
multi-persona-review src/ --format json > review.json

# Output as GitHub PR comment markdown
multi-persona-review src/ --format github > pr-comment.md

# Track costs to GCP Cloud Monitoring
multi-persona-review src/ --cost-sink gcp --gcp-project my-project-id

# Track costs to file (JSONL format)
multi-persona-review src/ --cost-sink file --cost-file ./costs.jsonl
```

### Claude Code Auto-Detection

**When running in Claude Code with VertexAI authentication:**

The CLI automatically detects Claude Code sessions and uses VertexAI credentials. **No `--provider` or `--vertex-project` flags needed!**

```bash
# Inside Claude Code - auto-detected
multi-persona-review src/

# Explicitly override (use Anthropic API instead)
multi-persona-review src/ --provider anthropic --api-key sk-ant-...
```

**Auto-detected environment variables:**
- `CLAUDE_CODE_USE_VERTEX=1` - Triggers auto-detection
- `ANTHROPIC_VERTEX_PROJECT_ID` - GCP project ID
- `CLOUD_ML_REGION` - VertexAI region

**Provider selection precedence:**
1. Explicit `--provider` flag (highest priority)
2. Claude Code env vars (when `CLAUDE_CODE_USE_VERTEX=1`)
3. Standard env vars (`ANTHROPIC_API_KEY`, `VERTEX_PROJECT_ID`)
4. Default (`anthropic`)

### VertexAI Sub-Agent Patterns

**Pattern: Parallel Persona Execution with Caching**

```typescript
import {
  loadPersonas,
  reviewFiles,
  createVertexAIClaudeReviewer,
} from '@wayfinder/multi-persona-review';

// 1. Create VertexAI Claude reviewer (sub-agent orchestrator)
const reviewer = createVertexAIClaudeReviewer({
  projectId: process.env.VERTEX_PROJECT_ID!,
  location: 'us-east5',
  model: 'claude-sonnet-4-5@20250929',
});

// 2. Load personas
const personas = await loadPersonas([
  '.engram/personas',
  '~/.engram/personas',
]);

const selectedPersonas = [
  personas.get('security-engineer')!,
  personas.get('code-health')!,
  personas.get('error-handling-specialist')!,
];

// 3. Run review with sub-agent orchestration
const result = await reviewFiles(
  {
    files: ['src/auth.ts', 'src/api.ts'],
    personas: selectedPersonas,
    mode: 'thorough',
    options: {
      useSubAgents: true,       // Enable sub-agent orchestration
      promptCaching: true,       // Enable 5-min ephemeral caching
      showCacheMetrics: true,    // Display cache hit rate
    },
  },
  process.cwd(),
  reviewer
);

// 4. Check cache metrics
if (result.cacheMetrics) {
  console.log(`Cache hit rate: ${result.cacheMetrics.hitRate}%`);
  console.log(`Cost savings: $${result.cacheMetrics.costSavings}`);
}
```

**Pattern: Batch Reviews with Aggressive Caching**

```bash
# Environment setup for batch reviews
export MULTI_PERSONA_BATCH_MODE=true
export MULTI_PERSONA_REVIEW_COUNT=10

# First review: Pays cache write cost
multi-persona-review src/file1.ts --show-cache-metrics

# Subsequent reviews: 90% cache discount (within 5 min)
multi-persona-review src/file2.ts --show-cache-metrics
multi-persona-review src/file3.ts --show-cache-metrics
# ... continues with cache hits ...

# Result: 86% total cost savings vs no caching
```

**Pattern: Claude Code Integration**

```typescript
// AI agent code running inside Claude Code
import { reviewFiles, createVertexAIClaudeReviewer } from '@wayfinder/multi-persona-review';

// Auto-detect Claude Code VertexAI credentials
const claudeCodeUseVertex = process.env.CLAUDE_CODE_USE_VERTEX === '1';
const vertexProject = process.env.ANTHROPIC_VERTEX_PROJECT_ID;
const vertexRegion = process.env.CLOUD_ML_REGION || 'us-east5';

let reviewer;
if (claudeCodeUseVertex && vertexProject) {
  // Use VertexAI Claude (automatic in Claude Code)
  reviewer = createVertexAIClaudeReviewer({
    projectId: vertexProject,
    location: vertexRegion,
  });
  console.log(`Using VertexAI Claude (auto-detected from Claude Code)`);
} else {
  // Fallback to Anthropic API
  reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
  });
}

// Run review
const result = await reviewFiles({
  files: ['src/'],
  personas: selectedPersonas,
  mode: 'quick',
}, process.cwd(), reviewer);
```

**Pattern: Cost-Optimized Reviews (Non-Git)**

```bash
# Reviewing code in temporary/non-git directory
cd /tmp/downloaded-library

# Use full-document mode with quick review
multi-persona-review . \
  --full \
  --mode quick \
  --provider vertexai-claude \
  --personas security-engineer,code-health

# Cost comparison:
# - Full mode: $0.20-0.50 per review
# - Quick mode: ~60% reduction → $0.08-0.20
# - With caching (2+ reviews): 86% reduction → $0.01-0.03
```

### CLI Options Reference

| Option | Values | Default | Description |
|--------|--------|---------|-------------|
| `--mode` | `quick`, `thorough` | `quick` | Review depth (quick = fast, thorough = comprehensive) |
| `--scan` | `full`, `diff`, `changed` | `full` | File scan mode (diff = git diff only) |
| `--personas` | Comma-separated list | Config default | Override persona selection |
| `--format` | `text`, `json`, `github` | `text` | Output format |
| `--cost-sink` | `gcp`, `file`, `stdout` | `stdout` | Cost tracking destination |
| `--no-colors` | Boolean | `false` | Disable ANSI colors in text output |
| `--no-cost` | Boolean | `false` | Hide cost information |
| `--no-dedupe` | Boolean | `false` | Disable deduplication |
| `--flat` | Boolean | `false` | Flat output (don't group by file) |
| `--dry-run` | Boolean | `false` | Preview without running review |
| `--verbose` | Boolean | `false` | Detailed logging |

---

## Configuration

### Project Configuration (.engram/config.yml)

```yaml
crossCheck:
  # Default mode for reviews
  defaultMode: quick  # quick | thorough

  # Default persona set
  defaultPersonas:
    - security-engineer
    - code-quality-reviewer
    - error-handling-reviewer

  # Options
  options:
    deduplicate: true           # Enable smart deduplication
    similarityThreshold: 0.8    # 80% similarity = duplicate
    parallel: true              # Parallel execution (3x faster)

  # Cost tracking
  costTracking:
    type: gcp  # gcp | aws | datadog | webhook | file | stdout
    gcpProjectId: my-project-id
    metricType: custom.googleapis.com/ai/review_cost

  # File scanning
  scanning:
    exclude:
      - "**/*.test.ts"
      - "**/dist/**"
      - "**/node_modules/**"
    include:
      - "src/**/*.ts"
      - "src/**/*.js"
```

### User Configuration (~/.engram/config.yml)

Override project defaults with user preferences:

```yaml
crossCheck:
  defaultMode: thorough  # User prefers thorough reviews
  defaultPersonas:
    - security-engineer
    - code-quality-reviewer
    - error-handling-reviewer
    - performance-reviewer  # User adds performance checks
```

### Company Configuration (~/.engram/company/config.yml)

Enforce company-wide standards:

```yaml
crossCheck:
  # Mandatory personas (cannot be removed by users)
  mandatoryPersonas:
    - security-engineer
    - fintech-compliance  # Required for financial services company

  # Cost tracking (enforce GCP logging)
  costTracking:
    type: gcp
    gcpProjectId: company-monitoring-project
    required: true  # Users cannot override
```

**Search order:** User → Project → Company → Core (first match wins)

---

## Deduplication Strategy

### How Deduplication Works

**Problem:** Multiple personas often find the same issue with slightly different wording.

**Example (before deduplication):**
- Security Engineer: "Missing input validation on userId parameter (CRITICAL)"
- Error Handling Reviewer: "userId parameter not validated, could cause errors (HIGH)"
- API Design Reviewer: "userId parameter lacks validation (MEDIUM)"

**After deduplication:**
- **CRITICAL:** Missing input validation on userId parameter
  - **Personas:** Security Engineer, Error Handling Reviewer, API Design Reviewer
  - **Issue:** [merged description]
  - **Fix:** [merged recommendations]

### Deduplication Algorithm

1. **Calculate similarity** between findings (Levenshtein distance + keyword matching)
2. **Group similar findings** (threshold: 0.8 = 80% similar)
3. **Keep highest severity** rating
4. **Merge descriptions** and recommendations
5. **Note which personas agreed**

### Configuration

```yaml
crossCheck:
  options:
    deduplicate: true
    similarityThreshold: 0.8  # 0.0 (strict) - 1.0 (lenient)
```

**Recommended thresholds:**
- `0.9` - Very strict (only exact duplicates)
- `0.8` - Balanced (default, 50%+ noise reduction)
- `0.7` - Lenient (may merge distinct issues)

---

## Cost Tracking

### Supported Cost Sinks

**GCP Cloud Monitoring:**
```yaml
costTracking:
  type: gcp
  gcpProjectId: my-project-id
  metricType: custom.googleapis.com/ai/review_cost
```

**File (JSONL):**
```yaml
costTracking:
  type: file
  filePath: ./costs.jsonl
```

**Stdout:**
```yaml
costTracking:
  type: stdout
```

### Cost Data Schema

```typescript
interface CostEvent {
  timestamp: number;
  model: string;
  inputTokens: number;
  outputTokens: number;
  cacheCreationTokens: number;
  cacheReadTokens: number;
  totalCost: number; // USD
  metadata: {
    gitBranch?: string;
    gitCommit?: string;
    gitAuthor?: string;
    personasUsed: string[];
    filesReviewed: number;
  };
}
```

---

## Failure Modes and Error Handling

### Persona Loading Errors

**Error:** Persona file not found
- **Cause:** Misspelled persona name or file missing
- **Fix:** List available personas: `multi-persona-review --list-personas`
- **Behavior:** Skip missing persona, continue with others, warn user

**Error:** Invalid persona YAML schema
- **Cause:** Malformed frontmatter or missing required fields
- **Fix:** Validate against schema (see personas plugin docs)
- **Behavior:** Skip invalid persona, warn user

### Review Execution Errors

**Error:** API rate limit (429)
- **Cause:** Too many requests to Anthropic API
- **Fix:** Exponential backoff (5s → 10s → 20s)
- **Behavior:** Retry up to 3 times, then fail with error

**Error:** Timeout (120s exceeded)
- **Cause:** Large file or slow API response
- **Fix:** Use `--mode quick` or `--scan diff` to reduce scope
- **Behavior:** Retry once (240s timeout), then skip file

**Error:** Network failure
- **Cause:** No internet connection or API unavailable
- **Fix:** Check network, retry
- **Behavior:** Retry with exponential backoff, then fail

### Degraded Mode Thresholds

- **1 failure:** Continue with warning
- **2-3 failures:** Degraded mode (warn, suggest manual review)
- **4+ failures:** Critical mode (recommend stop, fix config)
- **All personas failed:** Block completely (exit code 1)

---

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Multi-Persona Code Review
on: [pull_request]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install multi-persona-review
        run: npm install -g @engram/multi-persona-review

      - name: Run review on PR diff
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review \
            --scan diff \
            --mode quick \
            --format github \
            --cost-sink file \
            --cost-file ./review-cost.jsonl \
            > pr-review.md

      - name: Post review comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('pr-review.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: review
            });

      - name: Upload cost tracking
        uses: actions/upload-artifact@v4
        with:
          name: review-costs
          path: review-cost.jsonl
```

---

## Boundaries for AI Agents

### ✅ Always

- Load .ai.md distilled personas (NOT .why.md verbose versions)
- Apply personas systematically to code/design docs
- Deduplicate findings across personas
- Rate severity based on impact (CRITICAL, HIGH, MEDIUM, LOW)
- Preview reviews before posting to PRs
- Ask user for approval before posting GitHub comments
- Track costs when configured
- Use parallel execution (3x faster)

### ⚠️ Ask First

- Posting reviews to GitHub PRs (always preview and get approval)
- Overriding default persona selection
- Skipping deduplication (can increase noise)
- Using `--mode thorough` on large codebases (cost implications)

### 🚫 Never

- Post PR reviews without explicit user approval
- Use .why.md files during reviews (token inefficient, educational only)
- Load all 23 personas for every review (overkill)
- Comment on existing code in PR diffs (only added lines marked with +)
- Skip security-engineer persona for security-sensitive changes
- Ignore blocking issues (CRITICAL/HIGH severity)

---

## Integration with Foundation Principles

**FP4: Multi-Perspective Validation**
- Multi-persona review embodies cross-validation through diverse expert perspectives
- 50%+ noise reduction through deduplication
- 88% token efficiency with distilled personas

**FP9: Explicit Trade-offs**
- Quick vs thorough mode (speed vs depth)
- Parallel vs sequential execution (cost vs latency)
- Token budget vs persona comprehensiveness

**FP10: Continuous Improvement**
- Cost tracking enables ROI analysis
- Deduplication patterns inform persona refinement
- Failure modes drive error handling improvements

---

## Examples

### Example 1: Solo Developer Pre-Commit Check

**Scenario:** Developer wants to check code quality before committing

```bash
# Review uncommitted changes
multi-persona-review --scan diff --mode quick

# Output (text format):
# Multi-Persona Code Review
#
# Reviewed by: security-engineer, code-quality-reviewer, error-handling-reviewer
#
# Summary:
# - Critical: 0
# - High: 1
# - Medium: 3
# - Low: 2
#
# --- src/auth.ts ---
#
# [HIGH] Missing input validation on userId parameter
# Personas: security-engineer, error-handling-reviewer
# Issue: User input not validated before database query
# Fix: Add Zod schema validation before query
#
# [MEDIUM] Inconsistent error handling
# Persona: error-handling-reviewer
# Issue: Some code paths throw, others return null
# Fix: Standardize to throw HttpError with status codes

# Exit code: 1 (blocking issue found)
```

### Example 2: GitHub PR Review with Draft Comments

**Scenario:** Review PR 133 and post as draft review

```bash
# AI agent workflow (with user approval)
gh pr diff 133 --repo myorg/backend > /tmp/pr-133.diff

# Load personas based on PR type (backend)
# - security-engineer
# - api-design-reviewer
# - error-handling-reviewer

# Apply multi-persona review to diff
multi-persona-review /tmp/pr-133.diff \
  --format github \
  --personas security-engineer,api-design-reviewer,error-handling-reviewer \
  > /tmp/pr-133.md

# Preview and ask user
"Review complete. Found 2 HIGH issues. Post as draft review? (Y/n)"

# If approved, post as draft
gh-post-review --input /tmp/pr-133.json --draft
```

### Example 3: S6 Design Review (Wayfinder Integration)

**Scenario:** Wayfinder S6 phase with domain expert detection

```bash
# S6 Design phase starts
Read design-doc.md

# Load required personas (from Wayfinder config)
# - tech-lead (architecture, maintainability)
# - security-engineer (threat modeling)
# - qa-engineer (testability, edge cases)

# Load detected domain expert (ML project)
# - ml-engineer (model evaluation, bias detection)

# Apply multi-persona review to design doc
# Tech Lead: ✅ APPROVED (architecture looks good)
# Security Engineer: ❌ BLOCKED (missing threat model for user data)
# QA Engineer: ⚠️ CONDITIONAL (add integration test strategy)
# ML Engineer: ⚠️ CONDITIONAL (add bias metrics to evaluation)

# Decision: ❌ BLOCKED
# Action required: Address security threat model before S7
```

---

## Path Placeholders

- `{personas}` → `~/.engram/core/plugins/personas`
- `{multi-persona-review}` → `~/.engram/core/plugins/multi-persona-review`

Use in configs:
```yaml
personas:
  - {personas}/library/security/security-engineer.ai.md
  - {personas}/library/development/api-design-reviewer.ai.md
```

---

## Support

**Issues:** File in Beads tracker with `multi-persona-review` label
**Questions:** See README.md or DOCUMENTATION.md
**Updates:** Check CHANGELOG.md for version history
**CI/CD Examples:** See DEPLOYMENT.md

---

**Plugin Version:** 0.1.0
**Last Updated:** 2025-12-15
