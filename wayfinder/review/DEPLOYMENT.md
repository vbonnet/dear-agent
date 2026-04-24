# Multi-Persona Review Deployment Guide

This guide covers deploying and integrating Multi-Persona Review into your CI/CD pipelines, with a focus on GitHub Actions.

## Table of Contents

1. [Overview](#overview)
2. [GitHub Actions Integration](#github-actions-integration)
3. [Basic Workflow Examples](#basic-workflow-examples)
4. [Advanced Configurations](#advanced-configurations)
5. [Environment Variables and Secrets](#environment-variables-and-secrets)
6. [Cost Tracking in CI/CD](#cost-tracking-in-cicd)
7. [Pull Request Comments](#pull-request-comments)
8. [Badge Setup](#badge-setup)
9. [Troubleshooting](#troubleshooting)

## Overview

Multi-Persona Review can be integrated into various CI/CD platforms. This guide focuses on GitHub Actions, but the concepts apply to other platforms like GitLab CI, CircleCI, Jenkins, etc.

### Deployment Models

1. **PR Review** - Run reviews on pull requests and comment with findings
2. **Commit Review** - Review each commit on main/master branch
3. **Scheduled Review** - Periodic full codebase reviews
4. **On-Demand** - Manual workflow dispatch for reviews

### Key Considerations

- **API Costs**: Each review consumes Anthropic API credits
- **Runtime**: Reviews can take 1-5 minutes depending on size
- **Parallelization**: Multiple personas run in parallel for efficiency
- **Caching**: Consider caching node_modules for faster builds

## GitHub Actions Integration

### Prerequisites

1. **Anthropic API Key**: Required for all reviews
   - Get from: https://console.anthropic.com/
   - Store as GitHub secret: `ANTHROPIC_API_KEY`

2. **Repository Access**: Workflows need read access to code
   - Automatically available in GitHub Actions

3. **PR Write Access** (optional): For posting comments
   - Use `GITHUB_TOKEN` (automatically provided)

### Installation in Workflow

Multi-Persona Review can be installed via npm in your workflow:

```yaml
- name: Install Multi-Persona Review
  run: |
    npm install -g @engram/multi-persona-review
    # or from local path during development
    # npm install --prefix plugins/multi-persona-review
    # npm run build --prefix plugins/multi-persona-review
    # npm link --prefix plugins/multi-persona-review
```

## Basic Workflow Examples

### Example 1: Basic PR Review

Run a quick security review on every pull request.

```yaml
name: Multi-Persona Review PR Review

on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0  # Full history for diff analysis

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @engram/multi-persona-review

      - name: Run Security Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review review \
            --persona security-engineer \
            --mode quick \
            --output-format github \
            src/

      - name: Upload Review Results
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: multi-persona-review-results
          path: multi-persona-review-*.json
```

### Example 2: Multi-Persona Review with Failure on Critical Issues

Run multiple personas and fail the build if critical issues are found.

```yaml
name: Multi-Persona Review Multi-Persona Review

on:
  pull_request:
    branches: [main, develop]

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @engram/multi-persona-review

      - name: Run Multi-Persona Review
        id: review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review review \
            --persona security-engineer \
            --persona code-health \
            --persona performance-engineer \
            --mode thorough \
            --output-format json \
            --output-file results.json \
            src/ > review-output.txt || true

          # Check for critical findings
          CRITICAL_COUNT=$(jq '.summary.findingsBySeverity.critical // 0' results.json)
          echo "critical_count=$CRITICAL_COUNT" >> $GITHUB_OUTPUT

          if [ "$CRITICAL_COUNT" -gt 0 ]; then
            echo "::error::Found $CRITICAL_COUNT critical issues!"
            exit 1
          fi

      - name: Upload Results
        uses: actions/upload-artifact@v4
        if: always()
        with:
          name: review-results
          path: |
            results.json
            review-output.txt
```

### Example 3: PR Comment with Findings

Post review findings as a PR comment.

```yaml
name: Multi-Persona Review with PR Comments

on:
  pull_request:

jobs:
  review:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @engram/multi-persona-review

      - name: Run Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review review \
            --persona security-engineer \
            --mode quick \
            --output-format github \
            src/ > review.md

      - name: Post PR Comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const review = fs.readFileSync('review.md', 'utf8');

            // Find existing comment
            const comments = await github.rest.issues.listComments({
              owner: context.repo.owner,
              repo: context.repo.repo,
              issue_number: context.issue.number
            });

            const botComment = comments.data.find(comment =>
              comment.user.type === 'Bot' &&
              comment.body.includes('Multi-Persona Review Code Review')
            );

            const body = `## 🔍 Multi-Persona Review Code Review

            ${review}

            <sub>Powered by Engram Multi-Persona Review | [View Documentation](https://github.com/yourusername/engram)</sub>`;

            if (botComment) {
              // Update existing comment
              await github.rest.issues.updateComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                comment_id: botComment.id,
                body
              });
            } else {
              // Create new comment
              await github.rest.issues.createComment({
                owner: context.repo.owner,
                repo: context.repo.repo,
                issue_number: context.issue.number,
                body
              });
            }
```

### Example 4: Scheduled Full Codebase Review

Run a thorough review of the entire codebase weekly.

```yaml
name: Weekly Full Codebase Review

on:
  schedule:
    - cron: '0 9 * * 1'  # Every Monday at 9 AM UTC
  workflow_dispatch:  # Allow manual trigger

jobs:
  review:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @engram/multi-persona-review

      - name: Run Full Review
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review review \
            --persona security-engineer \
            --persona code-health \
            --persona performance-engineer \
            --persona accessibility-specialist \
            --mode thorough \
            --file-scan-mode full \
            --output-format json \
            --output-file weekly-review.json \
            --cost-tracking file \
            --cost-file costs.jsonl \
            src/

      - name: Upload Results
        uses: actions/upload-artifact@v4
        with:
          name: weekly-review-${{ github.run_number }}
          path: |
            weekly-review.json
            costs.jsonl
          retention-days: 90

      - name: Create Issue for High Severity Findings
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const results = JSON.parse(fs.readFileSync('weekly-review.json', 'utf8'));

            const highSeverity = results.findings.filter(f =>
              f.severity === 'critical' || f.severity === 'high'
            );

            if (highSeverity.length > 0) {
              const body = `## 🚨 Weekly Code Review - High Severity Findings

              Found ${highSeverity.length} high severity issues in weekly review.

              ${highSeverity.map(f => `- **${f.title}** (${f.severity}) - ${f.file}:${f.line || 'N/A'}`).join('\n')}

              [View full results](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }})`;

              await github.rest.issues.create({
                owner: context.repo.owner,
                repo: context.repo.repo,
                title: `Weekly Review: ${highSeverity.length} High Severity Issues Found`,
                body,
                labels: ['code-review', 'automated']
              });
            }
```

## Advanced Configurations

### Custom Personas in CI

Create a repository-specific persona for CI reviews:

```yaml
# .engram/personas/ci-reviewer.yaml
name: ci-reviewer
displayName: CI Security Reviewer
version: "1.0.0"
description: Fast security checks for CI/CD
focusAreas:
  - SQL Injection
  - XSS Vulnerabilities
  - Authentication Issues
  - Secrets in Code
prompt: |
  You are a security reviewer focused on critical vulnerabilities in CI/CD.

  Focus on finding:
  1. SQL injection vulnerabilities
  2. XSS vulnerabilities
  3. Authentication/authorization issues
  4. Hardcoded secrets or credentials
  5. Command injection risks

  Be concise and focus on high-confidence findings only.
  For each finding, provide:
  - Clear title
  - Severity (critical/high only)
  - Specific line number
  - Brief explanation
  - Suggested fix
```

Use in workflow:

```yaml
- name: Run CI Review
  run: |
    multi-persona-review review \
      --persona ci-reviewer \
      --persona-path .engram/personas \
      --mode quick \
      src/
```

### Matrix Strategy for Multiple Directories

Review different parts of your codebase in parallel:

```yaml
jobs:
  review:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        target:
          - { dir: 'src/api', persona: 'security-engineer' }
          - { dir: 'src/frontend', persona: 'accessibility-specialist' }
          - { dir: 'src/database', persona: 'security-engineer' }
          - { dir: 'src/workers', persona: 'performance-engineer' }
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Multi-Persona Review
        run: npm install -g @engram/multi-persona-review

      - name: Review ${{ matrix.target.dir }}
        env:
          ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
        run: |
          multi-persona-review review \
            --persona ${{ matrix.target.persona }} \
            --mode quick \
            --output-format json \
            --output-file review-${{ matrix.target.dir }}.json \
            ${{ matrix.target.dir }}
```

### Conditional Review Based on Changed Files

Only run specific reviews when certain files change:

```yaml
jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      api-changed: ${{ steps.filter.outputs.api }}
      frontend-changed: ${{ steps.filter.outputs.frontend }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            api:
              - 'src/api/**'
            frontend:
              - 'src/frontend/**'

  review-api:
    needs: detect-changes
    if: needs.detect-changes.outputs.api-changed == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Review API
        run: multi-persona-review review --persona security-engineer src/api/

  review-frontend:
    needs: detect-changes
    if: needs.detect-changes.outputs.frontend-changed == 'true'
    runs-on: ubuntu-latest
    steps:
      - name: Review Frontend
        run: multi-persona-review review --persona accessibility-specialist src/frontend/
```

## Environment Variables and Secrets

### Required Secrets

Set these in GitHub repository settings (Settings → Secrets and variables → Actions):

```bash
ANTHROPIC_API_KEY=sk-ant-api03-...
```

### Optional Secrets

For advanced integrations:

```bash
# For GCP cost tracking
GCP_PROJECT_ID=my-project-123
GCP_SERVICE_ACCOUNT_KEY={"type":"service_account",...}

# For Slack notifications
SLACK_WEBHOOK_URL=https://hooks.slack.com/services/...

# For custom integrations
WEBHOOK_URL=https://your-api.com/webhooks/multi-persona-review
```

### Using Environment Variables in Workflow

```yaml
env:
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  NODE_ENV: production
  CROSS_CHECK_MODE: quick
  CROSS_CHECK_PERSONAS: security-engineer,code-health

steps:
  - name: Run Review
    run: |
      multi-persona-review review \
        --mode $CROSS_CHECK_MODE \
        --persona $(echo $CROSS_CHECK_PERSONAS | tr ',' ' ' | xargs -n1 echo --persona | tr '\n' ' ') \
        src/
```

## Cost Tracking in CI/CD

### Track Costs to File Artifact

```yaml
- name: Run Review with Cost Tracking
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  run: |
    multi-persona-review review \
      --persona security-engineer \
      --mode quick \
      --cost-tracking file \
      --cost-file costs.jsonl \
      src/

- name: Upload Cost Data
  uses: actions/upload-artifact@v4
  with:
    name: cost-tracking-${{ github.run_number }}
    path: costs.jsonl
    retention-days: 90
```

### Track Costs to GCP

```yaml
- name: Authenticate to GCP
  uses: google-github-actions/auth@v2
  with:
    credentials_json: ${{ secrets.GCP_SERVICE_ACCOUNT_KEY }}

- name: Run Review with GCP Cost Tracking
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  run: |
    multi-persona-review review \
      --persona security-engineer \
      --mode thorough \
      --cost-tracking gcp \
      --gcp-project ${{ secrets.GCP_PROJECT_ID }} \
      src/
```

### Analyze Costs Over Time

```yaml
- name: Analyze Monthly Costs
  run: |
    # Download all cost artifacts from last 30 days
    # Aggregate and report
    cat costs-*.jsonl | jq -s 'map(.cost.total) | add' > total-cost.txt
    TOTAL=$(cat total-cost.txt)
    echo "Total API costs this month: \$$TOTAL"
```

## Pull Request Comments

### Simple PR Comment

```yaml
- name: Post Simple Comment
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const results = JSON.parse(fs.readFileSync('results.json', 'utf8'));

      const body = `## Multi-Persona Review Review Results

      - **Files Reviewed**: ${results.summary.filesReviewed}
      - **Total Findings**: ${results.summary.totalFindings}
      - **Critical**: ${results.summary.findingsBySeverity.critical}
      - **High**: ${results.summary.findingsBySeverity.high}
      - **Medium**: ${results.summary.findingsBySeverity.medium}
      - **Low**: ${results.summary.findingsBySeverity.low}

      [View full results](https://github.com/${{ github.repository }}/actions/runs/${{ github.run_id }})`;

      await github.rest.issues.createComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: context.issue.number,
        body
      });
```

### Detailed PR Comment with Inline Code

```yaml
- name: Post Detailed Comment
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const results = JSON.parse(fs.readFileSync('results.json', 'utf8'));

      // Group findings by severity
      const critical = results.findings.filter(f => f.severity === 'critical');
      const high = results.findings.filter(f => f.severity === 'high');

      let body = `## 🔍 Multi-Persona Review Code Review\n\n`;

      if (critical.length > 0) {
        body += `### 🚨 Critical Issues (${critical.length})\n\n`;
        critical.forEach(f => {
          body += `#### ${f.title}\n`;
          body += `**File**: \`${f.file}\`${f.line ? `:${f.line}` : ''}\n`;
          body += `**Personas**: ${f.personas.join(', ')}\n\n`;
          body += `${f.description}\n\n`;
          if (f.suggestion) {
            body += `**Suggested Fix**:\n\`\`\`\n${f.suggestion}\n\`\`\`\n\n`;
          }
          body += `---\n\n`;
        });
      }

      if (high.length > 0) {
        body += `### ⚠️ High Priority Issues (${high.length})\n\n`;
        high.forEach(f => {
          body += `- **${f.title}** - \`${f.file}\`${f.line ? `:${f.line}` : ''}\n`;
        });
      }

      body += `\n**Summary**: ${results.summary.totalFindings} total findings across ${results.summary.filesReviewed} files\n`;

      await github.rest.issues.createComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: context.issue.number,
        body
      });
```

### Review Request Changes

Use GitHub's review API to request changes if critical issues found:

```yaml
- name: Request Changes if Critical
  uses: actions/github-script@v7
  with:
    script: |
      const fs = require('fs');
      const results = JSON.parse(fs.readFileSync('results.json', 'utf8'));

      const critical = results.findings.filter(f => f.severity === 'critical');

      if (critical.length > 0) {
        await github.rest.pulls.createReview({
          owner: context.repo.owner,
          repo: context.repo.repo,
          pull_number: context.issue.number,
          event: 'REQUEST_CHANGES',
          body: `Multi-Persona Review found ${critical.length} critical security issues. Please address before merging.`
        });
      } else {
        await github.rest.pulls.createReview({
          owner: context.repo.owner,
          repo: context.repo.repo,
          pull_number: context.issue.number,
          event: 'APPROVE',
          body: 'Multi-Persona Review review passed - no critical issues found.'
        });
      }
```

## Badge Setup

### Create Status Badge

Add to your README.md:

```markdown
![Multi-Persona Review](https://github.com/yourusername/yourrepo/actions/workflows/multi-persona-review.yml/badge.svg)
```

### Dynamic Badge with Results

Use shields.io for dynamic badges:

```yaml
- name: Create Badge
  run: |
    CRITICAL=$(jq '.summary.findingsBySeverity.critical' results.json)
    COLOR="green"
    if [ "$CRITICAL" -gt 0 ]; then
      COLOR="red"
    fi

    curl "https://img.shields.io/badge/security-$CRITICAL%20critical-$COLOR" > badge.svg

- name: Upload Badge
  uses: actions/upload-artifact@v4
  with:
    name: security-badge
    path: badge.svg
```

## Troubleshooting

### Issue: Workflow Fails with "ANTHROPIC_API_KEY not set"

**Solution**: Ensure secret is set in repository settings and referenced in workflow:

```yaml
env:
  ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

### Issue: "No files found after filtering"

**Cause**: Binary files or excluded patterns
**Solution**: Check file patterns and exclusions:

```yaml
- name: Debug File Discovery
  run: |
    find src/ -type f -name "*.ts" -o -name "*.js"
    multi-persona-review review --file-scan-mode full src/
```

### Issue: Review Takes Too Long

**Solutions**:
1. Use `--mode quick` instead of `--mode thorough`
2. Limit to changed files only
3. Use fewer personas
4. Reduce file scope

```yaml
- name: Fast Review
  run: |
    multi-persona-review review \
      --mode quick \
      --file-scan-mode changed \
      --persona security-engineer \
      src/
```

### Issue: API Rate Limits

**Solution**: Add delays between reviews or use caching:

```yaml
- name: Review with Rate Limit Handling
  run: |
    for dir in src/api src/frontend src/backend; do
      multi-persona-review review --mode quick $dir
      sleep 5  # Delay between reviews
    done
```

### Issue: Cost Concerns

**Solutions**:
1. Use `quick` mode for PRs, `thorough` for scheduled reviews
2. Limit personas to most critical ones
3. Track costs with `--cost-tracking`
4. Set up budget alerts in Anthropic console

```yaml
- name: Budget-Conscious Review
  run: |
    multi-persona-review review \
      --mode quick \
      --persona security-engineer \
      --cost-tracking stdout \
      src/
```

### Issue: False Positives

**Solution**: Create custom personas with specific focus areas:

```yaml
# .engram/personas/strict-security.yaml
name: strict-security
displayName: Strict Security Reviewer
version: "1.0.0"
description: Only high-confidence security findings
focusAreas:
  - SQL Injection
  - XSS
  - Authentication
prompt: |
  Only report findings you are highly confident about.
  Ignore potential issues that may be false positives.
  Focus on clear, exploitable vulnerabilities.
```

## Best Practices

1. **Start Small**: Begin with security persona on PRs only
2. **Iterate**: Add more personas and coverage gradually
3. **Monitor Costs**: Track API usage to avoid surprises
4. **Custom Personas**: Create repository-specific personas
5. **Fail Fast**: Fail builds on critical issues only
6. **Cache Dependencies**: Cache node_modules for faster runs
7. **Parallel Reviews**: Use matrix strategy for large codebases
8. **Regular Audits**: Run full reviews weekly/monthly
9. **Team Integration**: Post results to Slack/Teams
10. **Documentation**: Keep personas and configs in version control

## Next Steps

1. Set up basic PR review workflow
2. Add ANTHROPIC_API_KEY secret
3. Test on a small PR
4. Expand to more personas
5. Add cost tracking
6. Implement PR comments
7. Set up scheduled full reviews

For more information, see [DOCUMENTATION.md](./DOCUMENTATION.md).
