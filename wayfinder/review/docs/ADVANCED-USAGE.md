# Advanced Usage Guide

This guide covers advanced features of Multi-Persona Review including Agency-Agents patterns, custom personas, cost optimization, and CI/CD integration.

---

## Table of Contents

- [Agency-Agents Patterns](#agency-agents-patterns)
- [Custom Personas](#custom-personas)
- [Cost Optimization](#cost-optimization)
- [Batch Reviews](#batch-reviews)
- [CI/CD Integration](#cicd-integration)
- [Programmatic API](#programmatic-api)

---

## Agency-Agents Patterns

Agency-Agents patterns enhance multi-persona collaboration through voting, confidence scoring, lateral thinking, and expertise boundary detection.

### Voting Mechanism

Personas vote GO or NO-GO on code quality with tier-weighted aggregation.

**Tier Weighting**:
- **Tier 1** (Senior): 3x weight (security-engineer, tech-lead)
- **Tier 2** (Mid-level): 1x weight (code-health, performance-reviewer)
- **Tier 3** (Junior): 0.5x weight (documentation-reviewer)

**Vote Aggregation Formula**:
```
weighted_vote = (GO_votes × weights) / total_weights
Result: GO if weighted_vote > threshold (default: 0.5)
```

**Usage**:
```bash
# Default threshold (0.5)
multi-persona-review src/ --vote-threshold 0.5

# Strict threshold (0.7) - require strong consensus
multi-persona-review src/ --vote-threshold 0.7

# Lenient threshold (0.3) - allow dissent
multi-persona-review src/ --vote-threshold 0.3
```

**Example Output**:
```
Weighted Vote: 0.85 (GO)
  - security-engineer (Tier 1): GO (weight: 3.0)
  - tech-lead (Tier 1): GO (weight: 3.0)
  - code-health (Tier 2): NO-GO (weight: 1.0)
  Result: 6.0 / 7.0 = 0.857 > 0.7 threshold → GO
```

### Confidence Scoring

Filter findings by AI confidence level (0-1 scale).

**Confidence Levels**:
- **0.9+**: Very certain (algorithmic issues, obvious vulnerabilities)
- **0.7-0.9**: High confidence (pattern-based issues)
- **0.5-0.7**: Moderate confidence (style, best practices)
- **<0.5**: Low confidence (suggestions, edge cases)

**Usage**:
```bash
# High confidence only (0.9+)
multi-persona-review src/ --min-confidence 0.9

# Balanced (0.7+) - recommended for most teams
multi-persona-review src/ --min-confidence 0.7

# Include moderate confidence (0.5+)
multi-persona-review src/ --min-confidence 0.5

# Show all findings (no filter)
multi-persona-review src/  # No --min-confidence flag
```

### Lateral Thinking (Alternative Approaches)

Personas propose 3 alternative solutions before final recommendation.

**Example Output**:
```
🔴 CRITICAL: SQL injection vulnerability (src/db.ts:42)
  Decision: NO-GO | Confidence: 0.95

  Alternatives:
    1. Use parameterized queries with pg library
       - Pros: Immediate fix, minimal refactoring
       - Cons: Still requires manual query building

    2. Migrate to Prisma ORM with type-safe queries
       - Pros: Type safety, automatic escaping
       - Cons: Significant refactoring required

    3. Implement stored procedures with input validation
       - Pros: Database-level security
       - Cons: Complex to maintain, less portable

  Recommended: Option 1 (least disruptive, immediate security fix)
```

### Expertise Boundary Detection

Personas flag findings outside their expertise and route to appropriate personas.

**Example**:
```
🔍 Performance Reviewer:
  ⚠️ OUT OF SCOPE: Cryptography implementation (src/auth.ts:89)
  → Routing to Security Engineer for review

🔍 Security Engineer:
  🔴 CRITICAL: Weak cryptographic algorithm (src/auth.ts:89)
    Using MD5 for password hashing (broken)
    Suggestion: Use bcrypt or Argon2
```

---

## Custom Personas

Create project-specific personas for specialized reviews.

### Creating a Custom Persona

**File**: `.engram/personas/database-reviewer.ai.md`

```yaml
---
name: database-reviewer
displayName: Database Reviewer
version: 1.0.0
description: Database design and SQL optimization specialist
focusAreas:
  - database-design
  - sql-optimization
  - indexing
  - query-performance
tier: 2  # Mid-level reviewer
severityLevels:
  - critical
  - high
  - medium
---

# Database Reviewer

You are a database specialist reviewing code for database-related issues.

## Focus Areas

1. **Schema Design**:
   - Normalization (avoid redundancy)
   - Proper indexing strategies
   - Foreign key constraints
   - Data type selection

2. **Query Optimization**:
   - N+1 query problems
   - Missing indexes
   - Inefficient joins
   - Unnecessary subqueries

3. **Performance**:
   - Query execution plans
   - Index usage
   - Connection pooling
   - Caching strategies

## Review Checklist

- [ ] Are indexes defined for foreign keys?
- [ ] Are queries using proper JOINs instead of multiple queries?
- [ ] Is connection pooling configured?
- [ ] Are transactions used appropriately?
- [ ] Are database migrations version-controlled?

## Output Format

Return findings in JSON format with:
- decision: "GO" or "NO-GO"
- confidence: 0-1 score
- alternatives: array of 3 alternative approaches

## Examples

**Good**:
```sql
SELECT u.*, p.* FROM users u
JOIN profiles p ON p.user_id = u.id
WHERE u.active = true
LIMIT 100;
```

**Bad** (N+1 problem):
```javascript
const users = await db.query('SELECT * FROM users');
for (const user of users) {
  user.profile = await db.query('SELECT * FROM profiles WHERE user_id = ?', [user.id]);
}
```

Expand this content to ≥1,024 tokens for cache eligibility...
```

### Using Custom Personas

```bash
# Use custom persona
multi-persona-review src/ --personas database-reviewer

# Combine with built-in personas
multi-persona-review src/ --personas security-engineer,database-reviewer,tech-lead
```

### Persona Token Requirements

For optimal caching (86% cost savings):
- **Minimum**: ≥1,024 tokens (~4,096 characters)
- **Recommended**: 1,200-1,500 tokens
- **Maximum**: No limit, but diminishing returns after 2,000 tokens

**Check Token Count**:
```bash
# Estimate tokens (4 chars ≈ 1 token)
wc -c < .engram/personas/database-reviewer.ai.md
# Divide by 4: 5120 chars / 4 = 1,280 tokens ✓
```

---

## Cost Optimization

### Batch Reviews for Cache Hits

Reviews within 5-minute window share cached persona prompts.

**Strategy**:
```bash
# Set batch mode for aggressive caching
export MULTI_PERSONA_BATCH_MODE=true

# Review multiple files sequentially
multi-persona-review src/api.ts --show-cache-metrics
# Output: Cache write: $0.03

multi-persona-review src/db.ts --show-cache-metrics
# Output: Cache hit! Savings: $0.02 (66%)

multi-persona-review src/auth.ts --show-cache-metrics
# Output: Cache hit! Savings: $0.02 (66%)

# Total savings: $0.04 vs $0.09 (44% reduction)
```

### Model Selection

Choose the right model for your use case:

| Model | Speed | Quality | Cost (per 1M tokens) | Use Case |
|-------|-------|---------|----------------------|----------|
| **Claude Haiku** | Fastest | Good | $0.25 / $1.25 | Quick reviews, PR checks |
| **Claude Sonnet** | Fast | Best | $3.00 / $15.00 | Thorough reviews (recommended) |
| **Claude Opus** | Slow | Highest | $15.00 / $75.00 | Critical code, security audits |
| **Gemini Flash** | Fast | Good | $0.075 / $0.30 | Cost-sensitive reviews |

**Usage**:
```bash
# Haiku for quick PR reviews
multi-persona-review src/ --model claude-3-5-haiku-20241022

# Sonnet for balanced reviews (default)
multi-persona-review src/ --model claude-3-5-sonnet-20241022

# Opus for critical security reviews
multi-persona-review src/ --model claude-3-opus-20240229

# Gemini for cost-sensitive reviews
multi-persona-review src/ --provider vertexai --model gemini-2.5-flash
```

### Scan Mode Optimization

Reduce review scope for faster, cheaper reviews:

```bash
# Changed files only (fastest)
multi-persona-review src/ --scan changed

# Git diff only (review changes, not entire files)
multi-persona-review src/ --scan diff

# Full files (thorough, but expensive)
multi-persona-review src/ --scan full
```

### Cost Tracking

Monitor costs to optimize spending:

```bash
# Track to GCP Cloud Monitoring
multi-persona-review src/ \
  --cost-sink gcp \
  --gcp-project my-project \
  --show-cache-metrics

# Track to file
multi-persona-review src/ \
  --cost-sink file \
  --cost-file ./costs.jsonl \
  --show-cache-metrics

# Analyze costs
cat costs.jsonl | jq '.totalCost' | awk '{sum+=$1} END {print sum}'
# Output: 2.45 (total spent in USD)
```

---

## Batch Reviews

### Sequential Batch Mode

Review multiple files/directories sequentially to maximize cache hits:

```bash
#!/bin/bash
export MULTI_PERSONA_BATCH_MODE=true

FILES=(
  "src/api/"
  "src/db/"
  "src/auth/"
  "src/utils/"
)

for file in "${FILES[@]}"; do
  multi-persona-review "$file" --show-cache-metrics
done

# Output shows cumulative savings:
# File 1: $0.08 (no cache)
# File 2: $0.02 (75% savings)
# File 3: $0.02 (75% savings)
# File 4: $0.02 (75% savings)
# Total: $0.14 vs $0.32 (56% reduction)
```

### Parallel Batch Mode (Advanced)

Review multiple files in parallel (loses cache benefits but faster):

```bash
#!/bin/bash
FILES=("src/api/" "src/db/" "src/auth/" "src/utils/")

for file in "${FILES[@]}"; do
  multi-persona-review "$file" --format json > "review-${file//\//-}.json" &
done

wait  # Wait for all parallel reviews to complete

# Merge results
jq -s 'add' review-*.json > combined-review.json
```

---

## CI/CD Integration

### GitHub Actions (Recommended)

**Complete Workflow** (`.github/workflows/code-review.yml`):

```yaml
name: Multi-Persona Code Review

on:
  pull_request:
    branches: [main, develop]
    paths:
      - 'src/**'
      - 'lib/**'

jobs:
  review:
    name: AI Code Review
    runs-on: ubuntu-latest
    permissions:
      pull-requests: write
      contents: read

    steps:
      - name: Checkout Code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for git diff

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'
          cache: 'npm'

      - name: Install Multi-Persona Review
        run: npm install -g @wayfinder/multi-persona-review

      - name: Run Review (Changed Files Only)
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
          MULTI_PERSONA_BATCH_MODE: 'true'
        run: |
          multi-persona-review src/ \
            --scan changed \
            --mode quick \
            --format github \
            --vote-threshold 0.7 \
            --min-confidence 0.8 \
            --show-cache-metrics \
            > review.md || true

      - name: Comment on PR
        uses: actions/github-script@v7
        if: always()
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('review.md', 'utf8');

            // Find existing comment
            const { data: comments } = await github.rest.issues.listComments({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number,
            });

            const botComment = comments.find(comment =>
              comment.user.type === 'Bot' &&
              comment.body.includes('Multi-Persona Review')
            );

            // Update or create comment
            if (botComment) {
              await github.rest.issues.updateComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                comment_id: botComment.id,
                body: review
              });
            } else {
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.issue.number,
                body: review
              });
            }

      - name: Check for Critical Issues
        run: |
          CRITICAL_COUNT=$(jq '[.findings[] | select(.severity == "critical")] | length' < review.json)
          if [ "$CRITICAL_COUNT" -gt "0" ]; then
            echo "::error::Found $CRITICAL_COUNT critical issues"
            exit 1
          fi
```

### GitLab CI

```yaml
code-review:
  stage: test
  image: node:20
  script:
    - npm install -g @wayfinder/multi-persona-review
    - |
      multi-persona-review src/ \
        --scan changed \
        --format json \
        --vote-threshold 0.7 \
        > review.json || true
    - cat review.json
  artifacts:
    reports:
      codequality: review.json
  only:
    - merge_requests
```

---

## Programmatic API

### Advanced Review Configuration

```typescript
import {
  loadPersonas,
  reviewFiles,
  createAnthropicReviewer,
  formatReviewResult,
  createCostSink,
} from '@wayfinder/multi-persona-review';

async function advancedReview() {
  // Load personas
  const personas = await loadPersonas([
    '.engram/personas',
    '~/.engram/personas',
  ]);

  // Create reviewer with custom config
  const reviewer = createAnthropicReviewer({
    apiKey: process.env.ANTHROPIC_API_KEY!,
    model: 'claude-3-5-sonnet-20241022',
    temperature: 0.2,  // Lower = more deterministic
    maxTokens: 8192,   // Higher = more detailed findings
  });

  // Create cost sink
  const costSink = createCostSink('gcp', {
    projectId: process.env.GCP_PROJECT_ID!,
  });

  // Run review with Agency-Agents patterns
  const result = await reviewFiles(
    {
      files: ['src/'],
      personas: [
        personas.get('security-engineer')!,
        personas.get('tech-lead')!,
        personas.get('performance-reviewer')!,
      ],
      mode: 'thorough',
      fileScanMode: 'diff',
      options: {
        parallel: true,
        maxConcurrency: 3,
        deduplicate: true,
        similarityThreshold: 0.85,
        voteThreshold: 0.7,      // Agency-Agents voting
        minConfidence: 0.8,      // Agency-Agents confidence
      },
    },
    process.cwd(),
    reviewer
  );

  // Track costs
  await costSink.recordCost(result.cost, {
    branch: process.env.CI_BRANCH,
    commit: process.env.CI_COMMIT_SHA,
    author: process.env.CI_AUTHOR,
  });

  // Calculate weighted vote
  const weightedVote = calculateWeightedVote(
    result.findings,
    [personas.get('security-engineer')!, personas.get('tech-lead')!]
  );

  // Format output
  const formatted = formatReviewResult(result, {
    colors: true,
    showCost: true,
    showSummary: true,
  });

  console.log(formatted);

  // Exit with appropriate code
  if (weightedVote < 0.7) {
    console.error('❌ NO-GO - Critical issues found');
    process.exit(1);
  } else {
    console.log('✅ GO - Code approved');
    process.exit(0);
  }
}

advancedReview().catch(console.error);
```

### Custom Vote Aggregation

```typescript
function calculateWeightedVote(
  findings: Finding[],
  personas: Persona[]
): number {
  const voteMap = new Map<string, 'GO' | 'NO-GO'>();

  // Collect votes from findings
  for (const finding of findings) {
    if (finding.decision) {
      voteMap.set(finding.persona, finding.decision);
    }
  }

  let goVotes = 0;
  let totalWeight = 0;

  for (const persona of personas) {
    const weight = getVoteWeight(persona.tier);
    totalWeight += weight;

    const vote = voteMap.get(persona.name) || 'GO';  // Default to GO if no findings
    if (vote === 'GO') {
      goVotes += weight;
    }
  }

  return goVotes / totalWeight;
}

function getVoteWeight(tier: 1 | 2 | 3 | undefined): number {
  if (tier === 1) return 3.0;
  if (tier === 3) return 0.5;
  return 1.0;  // Tier 2
}
```

---

## Best Practices

### 1. Review Changed Files Only in CI/CD

```yaml
# Good: Fast, cost-effective
multi-persona-review src/ --scan changed --mode quick

# Bad: Expensive, slow
multi-persona-review src/ --scan full --mode thorough
```

### 2. Use Batch Mode for Sequential Reviews

```bash
# Good: Maximizes cache hits
export MULTI_PERSONA_BATCH_MODE=true
multi-persona-review src/file1.ts
multi-persona-review src/file2.ts

# Bad: No cache benefits
multi-persona-review src/file1.ts
# Wait >5 minutes
multi-persona-review src/file2.ts
```

### 3. Appropriate Vote Thresholds

```bash
# Critical code (security, payments): High threshold
multi-persona-review src/auth/ --vote-threshold 0.8

# Regular features: Moderate threshold
multi-persona-review src/features/ --vote-threshold 0.6

# Experimental code: Low threshold
multi-persona-review src/experimental/ --vote-threshold 0.4
```

### 4. Persona Selection

```bash
# Security-critical: Use security personas
multi-persona-review src/auth/ --personas security-engineer,tech-lead

# Performance-critical: Use performance personas
multi-persona-review src/api/ --personas performance-reviewer,tech-lead

# UI code: Use accessibility personas
multi-persona-review src/ui/ --personas accessibility-specialist,code-health
```

---

## See Also

- [Getting Started](GETTING-STARTED.md)
- [API Documentation](API.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Architecture](../ARCHITECTURE.md)
