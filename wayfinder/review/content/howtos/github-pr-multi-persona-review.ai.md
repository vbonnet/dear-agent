---
type: workflow
title: Multi-Persona GitHub PR Review
description: Review GitHub PRs using multiple persona perspectives for comprehensive code review
tags: [github, code-review, multi-persona, workflow, multi-persona-review]
agents: [claude-code, cursor]
load_when: "Reviewing GitHub pull requests, conducting code reviews, or using multi-persona analysis"
modified: 2025-12-02
---

# Review GitHub PR with Multi-Persona Analysis

**When:** User asks to review a GitHub PR

**What:** AI agent reads PR diff, applies multiple persona perspectives, generates comprehensive review

**How:** Fetch diff → Load personas → Apply each persona → Format review → Preview → Post (with approval)

---

## Before Reviewing PRs

⚠️ **Prerequisites:**
- GitHub CLI (`gh`) installed and authenticated
- Access to personas plugin (`~/.engram/core/plugins/personas/`)

**Quick rules:**
- Select personas appropriate to PR type (backend/frontend/infra)
- Always preview review before posting
- Ask user for approval before posting comments

---

## Workflow

### Step 1: Fetch PR Diff

```bash
gh pr diff {PR_NUMBER} --repo {OWNER/REPO} > /tmp/pr-{PR_NUMBER}.diff
```

**Why save to file:** Allows re-review if needed, keeps audit trail

**Required:**
- `{PR_NUMBER}` - Pull request number (e.g., 133)
- `{OWNER/REPO}` - Repository (e.g., myorg/myproject)

### Step 2: Read Diff

Use Read tool to load the diff:
```
Read /tmp/pr-{PR_NUMBER}.diff
```

### Step 3: Load Personas

Read persona files from personas plugin:

```
Read ~/.engram/core/plugins/personas/library/core/{PERSONA}.ai.md
```

**Persona selection by PR type:**

| PR Type | Recommended Personas |
|---------|---------------------|
| Backend/API | `security-engineer`, `api-design-reviewer`, `error-handling-reviewer` |
| Frontend/UI | `accessibility-specialist`, `code-quality-reviewer`, `error-handling-reviewer` |
| Database | `security-engineer`, `database-specialist`, `performance-reviewer` |
| Infrastructure | `security-engineer`, `devops-engineer`, `performance-reviewer` |
| Full-stack | `security-engineer`, `code-quality-reviewer`, `performance-reviewer` |
| Not sure | `security-engineer`, `code-quality-reviewer`, `error-handling-reviewer` |

### Step 4: Apply Each Persona

For each persona:
1. Read persona's focus areas and review criteria
2. Analyze diff from that persona's perspective
3. Identify issues matching persona's expertise (on **added lines only**, marked with `+`)
4. Rate severity (CRITICAL, HIGH, MEDIUM, LOW)
5. Record diff position (line number in diff output)
6. Suggest fixes

**IMPORTANT:** Only comment on added lines (marked with `+` in diff). For issues with existing code, add to summary section instead.

**How to find diff position:**
- Count line number in diff output starting from 1
- Include all lines (headers, hunks, context, changes)
- Example: If added line is at position 308 in diff, use position=308

**Output format per persona:**
```markdown
## {Persona Name}

**{SEVERITY}:** {Issue title}
- **File:** {file}, **Position:** {diff_position}
- **Issue:** {description}
- **Fix:** {recommendation}
- **Why it matters:** {impact}
```

### Step 5: Deduplicate Issues

If multiple personas find same issue:
- Keep highest severity rating
- Merge perspectives in description
- Note which personas agreed

### Step 6: Format Review

Combine all persona reviews into single markdown document:

```markdown
# Multi-Persona Code Review: PR #{NUMBER}

- **Reviewed by:** {persona1}, {persona2}, {persona3}
- **Date:** {date}

## Summary

- **Critical issues:** {count}
- **High-priority issues:** {count}
- **Medium issues:** {count}
- **Low issues:** {count}

---

## {Persona 1 Name}

{findings}

---

## {Persona 2 Name}

{findings}

---

## Overall Assessment

{summary paragraph}
```

### Step 7: Preview Review

Write review to `/tmp/review-{PR_NUMBER}.md` using Write tool, then display summary to user:

```
"I've completed the multi-persona review of PR #{NUMBER}.

Found:
- {count} critical issues
- {count} high-priority issues
- {count} medium issues

Full review: /tmp/review-{PR_NUMBER}.md

Would you like me to:
1. Post this review to the PR
2. Show you the full review first
3. Revise with different personas"
```

### Step 8: Format for Inline Posting (Only with Approval)

**If user approves posting with inline comments:**

1. Convert review to JSON format with diff positions:
```json
{
  "pr_number": 133,
  "repo": "owner/repo",
  "summary": "## Multi-Persona Code Review\n\n**Reviewed by:** {personas}\n\n### Summary\n- Critical: {count}\n- High: {count}\n- Medium: {count}\n\n### Issues with Existing Code\n(Not in diff, for discussion only)\n- {issue1}\n- {issue2}",
  "event": "REQUEST_CHANGES",
  "comments": [
    {
      "file": "path/to/file.ts",
      "position": 308,
      "body": "**HIGH:** {Issue title}\n\n**Persona:** {persona}\n\n**Issue:** {description}\n\n**Fix:** {recommendation}"
    }
  ]
}
```

2. Save to `/tmp/review-{PR_NUMBER}.json`

3. Post using github-connector plugin:
```bash
gh-post-review --input /tmp/review-{PR_NUMBER}.json --draft
```

**NEVER post without explicit user approval.**

**Note:** `--draft` creates a pending review. User must view and submit it in GitHub UI.

---

## Example: Frontend PR Review

**User:** "Review PR 133 in myorg/frontend-app"

**Agent:**

1. Fetch diff:
```bash
gh pr diff 133 --repo myorg/frontend-app > /tmp/pr-133.diff
```

2. Check PR type (UI changes based on title/files)

3. Select personas: accessibility-specialist, code-quality-reviewer, error-handling-reviewer

4. Read each persona:
```
Read ~/.engram/core/plugins/personas/library/core/accessibility-specialist.ai.md
Read ~/.engram/core/plugins/personas/library/core/code-quality-reviewer.ai.md
Read ~/.engram/core/plugins/personas/library/core/error-handling-reviewer.ai.md
```

5. Read diff:
```
Read /tmp/pr-133.diff
```

6. Apply each persona perspective, identifying issues on added lines and recording diff positions

7. Write review to `/tmp/review-133.md` (human-readable) and `/tmp/review-133.json` (for posting)

8. Show summary to user:
```
"I've completed the multi-persona review of PR #133.

Found:
- 0 critical issues
- 2 high-priority issues (accessibility, error handling)
- 5 medium issues

Full review: /tmp/review-133.md

Would you like me to post this as inline comments (draft mode)?"
```

9. If user says yes:
```bash
gh-post-review --input /tmp/review-133.json --draft
```

User then reviews and submits via GitHub UI at: https://github.com/myorg/frontend-app/pull/133/files

---

## Errors

**Error:** `gh: command not found`
- **Cause:** GitHub CLI not installed
- **Fix:** Tell user to install: `brew install gh` (macOS) or https://cli.github.com/

**Error:** `Could not resolve to a PullRequest`
- **Cause:** Invalid PR number or repo
- **Fix:** Verify PR number and repo name:
```bash
gh pr list --repo {OWNER/REPO}
```

**Error:** Persona file not found
- **Cause:** Persona doesn't exist or name misspelled
- **Fix:** List available personas:
```bash
ls ~/.engram/core/plugins/personas/library/core/*.ai.md
```

**Error:** No issues found
- **Cause:** This is okay! Code quality is good.
- **Fix:** Tell user: "Multi-persona review found no critical issues. The PR looks good from {persona list} perspectives."

---

## Verification

**How to verify review was posted:**
```bash
gh pr view {PR_NUMBER} --repo {OWNER/REPO} --comments
```

**Expected output:**
```
Comments:
  #1 (your-username) 2 minutes ago
     # Multi-Persona Code Review: PR #133
     ...
```

---

## Best Practices

**DO:**
- Read persona files to understand their focus areas
- Apply persona criteria to actual code changes
- Provide specific file/line references
- Rate severity based on impact
- Deduplicate findings across personas
- Always preview before posting
- Ask user for approval

**DON'T:**
- Post reviews without user approval
- Hallucinate issues not in the diff
- Mix up persona perspectives
- Skip severity ratings
- Forget to save review to file (for user reference)

---

## Related

- Personas: `~/.engram/core/plugins/personas/library/core/`
- Persona docs: `~/.engram/core/plugins/personas/README.md`
- GitHub connector: `~/plugins/github-connector/` (for posting reviews)
- GitHub CLI: https://cli.github.com/
