# Getting Started with Multi-Persona Review

**Estimated Time**: 5-10 minutes

This tutorial will guide you through installing and running your first multi-persona code review.

---

## Prerequisites

- **Node.js** ≥18.0.0
- **npm** ≥9.0.0
- **API Key** from Anthropic or Google Cloud project ID for Vertex AI

---

## Step 1: Installation (2 minutes)

### Install via npm

```bash
npm install -g @wayfinder/multi-persona-review
```

### Verify Installation

```bash
multi-persona-review --version
# Expected output: 0.1.0
```

---

## Step 2: API Setup (2 minutes)

Choose your AI provider:

### Option A: Anthropic (Claude) - Recommended for Getting Started

```bash
# Get API key from: https://console.anthropic.com/
export ANTHROPIC_API_KEY=sk-ant-api03-...
```

### Option B: Vertex AI (Claude) - Recommended for GCP Users

```bash
# Authenticate with Google Cloud
gcloud auth application-default login

# Set environment variables
export VERTEX_PROJECT_ID=your-gcp-project-id
export VERTEX_LOCATION=us-east5
export VERTEX_MODEL=claude-sonnet-4-5@20250929
```

---

## Step 3: Run Your First Review (1 minute)

### Basic Review

```bash
cd your-project
multi-persona-review src/
```

**Expected Output**:
```
🔍 Running review with 3 personas...
  ✓ security-engineer (2.3s)
  ✓ code-health (1.8s)
  ✓ error-handling-specialist (2.1s)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📁 src/api.ts

  🔴 CRITICAL: SQL injection vulnerability (line 42)
    └─ security-engineer
    Unsanitized user input passed directly to SQL query
    Suggestion: Use parameterized queries or ORM

  🟠 HIGH: Missing error handling (line 58)
    └─ error-handling-specialist
    Database query has no try-catch block
    Suggestion: Add try-catch and log errors

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Summary: 5 findings (2 critical, 3 high)
💰 Cost: $0.08 (300 input tokens, 150 output tokens)
⏱️ Duration: 3.2s
```

---

## Step 4: Explore Features (3 minutes)

### List Available Personas

```bash
multi-persona-review --list-personas
```

**Output**:
```
Available personas (8):

Tier 1 (Senior):
  security-engineer       Security specialist (OWASP Top 10)
  tech-lead              Architecture and best practices

Tier 2 (Mid-level):
  code-health            Maintainability and readability
  performance-reviewer   Performance optimization
  error-handling-specialist  Exception handling
  accessibility-specialist  WCAG compliance

Tier 3 (Junior):
  qa-engineer            Testing and validation
  documentation-reviewer  Comments and README
```

### Thorough Review

```bash
multi-persona-review src/ --mode thorough
```

**What's Different**:
- Reviews all severity levels (not just critical/high)
- Scans entire files (not just changed lines)
- Uses 5+ personas for comprehensive coverage

### Custom Personas

```bash
multi-persona-review src/ --personas security-engineer,tech-lead
```

### Agency-Agents Review (Advanced)

```bash
multi-persona-review src/ \
  --vote-threshold 0.7 \
  --min-confidence 0.8
```

**New Output Fields**:
```
  🔴 CRITICAL: SQL injection (line 42)
    Decision: NO-GO | Confidence: 0.95
    Alternatives:
      1. Use parameterized queries (pg library)
      2. Migrate to Prisma ORM
      3. Implement stored procedures
    Recommended: Option 1 (least disruptive)
```

---

## Step 5: Configure for Your Project (2 minutes)

### Initialize Configuration

```bash
cd your-project
multi-persona-review init
```

**Creates**: `.engram/config.yml`

```yaml
crossCheck:
  defaultMode: quick  # Change to 'thorough' for comprehensive reviews
  defaultPersonas:
    - security-engineer
    - code-health
    - error-handling-specialist

  options:
    deduplicate: true
    similarityThreshold: 0.8
```

### Customize Configuration

Edit `.engram/config.yml`:

```yaml
crossCheck:
  defaultMode: thorough
  defaultPersonas:
    - security-engineer
    - tech-lead
    - performance-reviewer
    - accessibility-specialist  # Added for web projects

  options:
    deduplicate: true
    similarityThreshold: 0.85    # Stricter deduplication
    voteThreshold: 0.6           # Agency-Agents: Lower threshold
    minConfidence: 0.75          # Agency-Agents: Filter findings

  costTracking:
    type: file                   # Track costs to file
    costFile: ./review-costs.jsonl
```

### Run with Configuration

```bash
multi-persona-review  # Uses .engram/config.yml
```

---

## Step 6: CI/CD Integration (Optional, 3 minutes)

### Add to GitHub Actions

Create `.github/workflows/code-review.yml`:

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
          multi-persona-review src/ --format github > review.md || true

      - name: Comment on PR
        uses: actions/github-script@v7
        if: always()
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

### Add Secrets

1. Go to: GitHub Repo → Settings → Secrets → Actions
2. Add `ANTHROPIC_API_KEY` with your API key
3. Create a PR to test the workflow

---

## Common Commands Cheat Sheet

```bash
# Basic review
multi-persona-review

# Specific files
multi-persona-review src/api.ts src/auth.ts

# Thorough review
multi-persona-review src/ --mode thorough

# Custom personas
multi-persona-review src/ --personas security-engineer,tech-lead

# Agency-Agents review
multi-persona-review src/ --vote-threshold 0.7 --min-confidence 0.8

# Output formats
multi-persona-review src/ --format json > review.json
multi-persona-review src/ --format github > pr-comment.md

# Cost tracking
multi-persona-review src/ --cost-sink file --cost-file costs.jsonl

# Cache performance
multi-persona-review src/ --show-cache-metrics

# Dry run (preview)
multi-persona-review --dry-run src/

# Verbose logging
multi-persona-review --verbose src/

# List personas
multi-persona-review --list-personas

# Initialize configuration
multi-persona-review init

# Help
multi-persona-review --help
```

---

## Troubleshooting

### "Missing API key" Error

**Problem**:
```
Error: ANTHROPIC_API_KEY environment variable not set
```

**Solution**:
```bash
export ANTHROPIC_API_KEY=sk-ant-api03-...
```

### "No personas found" Warning

**Problem**:
```
Warning: No personas found in search paths
```

**Solution**:
1. Verify persona files exist in search paths
2. Check file permissions
3. Use `--list-personas` to see what's available

### High Costs

**Problem**: Review costs are too high

**Solutions**:
1. **Use caching**: Batch reviews within 5-minute window
   ```bash
   export MULTI_PERSONA_BATCH_MODE=true
   ```

2. **Use cheaper models**:
   ```bash
   multi-persona-review src/ --model claude-3-5-haiku-20241022
   ```

3. **Review changed files only**:
   ```bash
   multi-persona-review src/ --scan changed --mode quick
   ```

### Slow Reviews

**Problem**: Reviews taking too long

**Solutions**:
1. **Enable parallel execution** (default):
   ```bash
   multi-persona-review src/  # Already parallel
   ```

2. **Reduce persona count**:
   ```bash
   multi-persona-review src/ --personas security-engineer
   ```

3. **Use quick mode**:
   ```bash
   multi-persona-review src/ --mode quick
   ```

---

## Next Steps

Now that you've completed the getting started tutorial:

1. **Read**: [Advanced Usage Guide](ADVANCED-USAGE.md)
2. **Explore**: [API Documentation](API.md)
3. **Customize**: [Creating Custom Personas](../docs/persona-optimization-guide.md)
4. **Deep Dive**: [Architecture](../ARCHITECTURE.md)

---

## Getting Help

- **Documentation**: [Full Documentation](../DOCUMENTATION.md)
- **Issues**: [GitHub Issues](https://github.com/wayfinder/multi-persona-review/issues)
- **Discussions**: [GitHub Discussions](https://github.com/wayfinder/multi-persona-review/discussions)

---

**Congratulations!** 🎉 You've successfully set up and run your first multi-persona code review.
