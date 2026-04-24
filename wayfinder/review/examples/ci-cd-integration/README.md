# CI/CD Integration Example

This example demonstrates how to integrate multi-persona-review into your GitHub Actions CI/CD pipeline.

## Overview

This example shows how to:
- Run automated code reviews on pull requests
- Post review findings as PR comments
- Fail builds on critical/high severity findings
- Track review costs over time
- Use different AI providers in CI/CD

## Files

- `.github/workflows/review.yml` - GitHub Actions workflow configuration
- `.github/workflows/review-on-push.yml` - Alternative: review on push to main
- `README.md` - This file with setup instructions

## Prerequisites

### 1. API Credentials

Add one of the following secrets to your GitHub repository:

**Option A: Anthropic (Claude)**
- Go to Settings > Secrets and variables > Actions
- Add secret: `ANTHROPIC_API_KEY`

**Option B: VertexAI (Gemini or Claude)**
- Go to Settings > Secrets and variables > Actions
- Add secret: `VERTEX_PROJECT_ID` (your GCP project ID)
- Add secret: `VERTEX_SERVICE_ACCOUNT_KEY` (GCP service account JSON key)
- Optionally add: `VERTEX_LOCATION` (default: us-central1 for Gemini, us-east5 for Claude)

### 2. Permissions

Ensure the workflow has permission to:
- Read repository contents
- Write pull request comments
- Read and write checks

In your repository settings:
- Go to Settings > Actions > General
- Under "Workflow permissions", select "Read and write permissions"
- Enable "Allow GitHub Actions to create and approve pull requests"

## Workflow Configuration

### Basic Pull Request Review

The `review.yml` workflow runs on every pull request:

```yaml
name: Code Review

on:
  pull_request:
    types: [opened, synchronize]

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
        run: |
          npm install -g @wayfinder/multi-persona-review

      - name: Run code review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review src/ \
            --mode thorough \
            --format github \
            --scan diff > review-comment.md

      - name: Post PR comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const comment = fs.readFileSync('review-comment.md', 'utf8');
            github.rest.issues.createComment({
              issue_number: context.issue.number,
              owner: context.repo.owner,
              repo: context.repo.repo,
              body: comment
            });
```

### Using VertexAI (Gemini)

```yaml
- name: Setup GCP credentials
  uses: google-github-actions/auth@v2
  with:
    credentials_json: ${{ secrets.VERTEX_SERVICE_ACCOUNT_KEY }}

- name: Run code review with VertexAI
  env:
    VERTEX_PROJECT_ID: ${{ secrets.VERTEX_PROJECT_ID }}
    VERTEX_LOCATION: us-central1
  run: |
    multi-persona-review src/ \
      --provider vertexai \
      --mode thorough \
      --format github \
      --scan diff > review-comment.md
```

### Using VertexAI (Claude)

```yaml
- name: Setup GCP credentials
  uses: google-github-actions/auth@v2
  with:
    credentials_json: ${{ secrets.VERTEX_SERVICE_ACCOUNT_KEY }}

- name: Run code review with VertexAI Claude
  env:
    VERTEX_PROJECT_ID: ${{ secrets.VERTEX_PROJECT_ID }}
    VERTEX_LOCATION: us-east5
    VERTEX_MODEL: claude-sonnet-4-5@20250929
  run: |
    multi-persona-review src/ \
      --provider vertexai-claude \
      --mode thorough \
      --format github \
      --scan diff > review-comment.md
```

## Cost Tracking

### Track costs to GCP Cloud Monitoring

```yaml
- name: Run review with cost tracking
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  run: |
    multi-persona-review src/ \
      --mode thorough \
      --cost-sink gcp \
      --gcp-project ${{ secrets.GCP_PROJECT_ID }}
```

### Track costs to file (stored as artifact)

```yaml
- name: Run review with file cost tracking
  run: |
    multi-persona-review src/ \
      --mode thorough \
      --cost-sink file \
      --cost-file costs.jsonl

- name: Upload cost data
  uses: actions/upload-artifact@v4
  with:
    name: review-costs
    path: costs.jsonl
```

## Fail on Critical Findings

The CLI automatically exits with code 1 if critical or high severity findings are detected:

```yaml
- name: Run review (fail on critical)
  run: |
    multi-persona-review src/ --mode thorough --scan diff
  # This step will fail the build if critical/high findings exist
```

## Scan Modes for CI/CD

Choose the appropriate scan mode:

- `--scan diff` - Review only changed lines (fastest, recommended for PRs)
- `--scan changed` - Review entire changed files (thorough)
- `--scan full` - Review all files (use sparingly, expensive)

## Caching for Cost Optimization

Multi-persona-review automatically detects CI environments and enables aggressive caching:

```yaml
- name: Run review with caching
  env:
    CI: true  # Automatically detected, enables aggressive caching
  run: |
    multi-persona-review src/ \
      --mode thorough \
      --show-cache-metrics
```

The plugin will:
- Automatically select 1-hour cache TTL in CI
- Reuse cached persona prompts across reviews
- Show cache performance metrics
- Reduce costs by 50-86% for repeated reviews

## Example Output

When posted as a PR comment, the review will look like:

```markdown
## 🔍 Multi-Persona Code Review

**Files Reviewed:** 3 changed files

### Critical Issues (1)

**🔴 Hardcoded API Key** - `src/auth.ts:15`
- **Persona:** Security Engineer
- **Issue:** API key stored directly in source code
- **Recommendation:** Use environment variables or AWS Secrets Manager

### High Issues (2)

**🟠 SQL Injection Risk** - `src/database.ts:42`
- **Persona:** Security Engineer
- **Issue:** Unsanitized user input in SQL query
- **Recommendation:** Use parameterized queries or ORM

---

**Summary:** 1 critical, 2 high, 3 medium issues found
**Cost:** $0.45 | **Time:** 8.3s | **Cache Hit Rate:** 75%

_Generated by multi-persona-review v0.1.0_
```

## Best Practices

1. **Use `--scan diff` for PRs** - Only review changed code to minimize costs
2. **Enable caching** - The plugin automatically optimizes for CI environments
3. **Track costs** - Monitor spending with `--cost-sink gcp` or `--cost-sink file`
4. **Customize personas** - Use `--personas` to focus on relevant concerns
5. **Use VertexAI Claude** - Better integration with GCP, same Claude quality

## Troubleshooting

### Issue: "No API credentials configured"

**Solution:** Ensure you've added the appropriate secret to your repository:
- For Anthropic: `ANTHROPIC_API_KEY`
- For VertexAI: `VERTEX_PROJECT_ID` and `VERTEX_SERVICE_ACCOUNT_KEY`

### Issue: "Permission denied to post PR comment"

**Solution:** Update workflow permissions in repository settings:
- Settings > Actions > General > Workflow permissions
- Select "Read and write permissions"

### Issue: "Cost too high"

**Solution:** Optimize your workflow:
- Use `--scan diff` instead of `--scan full`
- Reduce personas with `--personas security-engineer,code-health`
- Use `--mode quick` for faster reviews
- Let caching optimize automatically (enabled in CI by default)

## Next Steps

- See `../custom-personas/` to create specialized review personas
- See `../programmatic-api/` for advanced integration scenarios
- Check the main [DEPLOYMENT.md](../../DEPLOYMENT.md) for more CI/CD examples
