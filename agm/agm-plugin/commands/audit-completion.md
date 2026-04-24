---
model: haiku
effort: low
description: Audit worker session quality before archival
argument-hint: "{session-name}"
allowed-tools: Bash(agm session get *), Bash(git -C *), Read, Grep, Glob
---

# Audit Completion

Audit a worker session's code quality and completeness before archival.

**Step 1: Parse arguments**

- Parse $ARGUMENTS to extract session name
- If $ARGUMENTS is empty or whitespace only:
  - Show: "Session name is required. Usage: /agm:audit-completion {session-name}"
  - Exit gracefully

**Step 2: Look up session metadata**

- Run: `agm session get "{SESSION_NAME}" --output json`
- If exit code is not 0:
  - If output contains "not found": show "Session not found: {SESSION_NAME}" and suggest "/agm:agm-list"
  - Otherwise show the error and suggest "agm admin doctor"
  - Exit gracefully
- Parse the JSON output and extract:
  - WORKTREE_PATH from the `project` field (this is the working directory / worktree path)
  - PURPOSE from the `purpose` field (the acceptance criteria / session goal)
  - SESSION_STATE from the `state` field
- If WORKTREE_PATH is empty:
  - Show: "Session has no project directory set. Cannot audit."
  - Exit gracefully

**Step 3: Detect git branch**

- Run: `git -C "{WORKTREE_PATH}" rev-parse --abbrev-ref HEAD`
- If exit code is not 0:
  - Show: "Could not detect git branch in {WORKTREE_PATH}. Is this a git repository?"
  - Exit gracefully
- Store output as BRANCH

**Step 4: Gather commit history**

- Run: `git -C "{WORKTREE_PATH}" log --oneline main..{BRANCH}`
- If exit code is not 0 (e.g., no "main" branch):
  - Try: `git -C "{WORKTREE_PATH}" log --oneline master..{BRANCH}`
  - If that also fails: try `git -C "{WORKTREE_PATH}" log --oneline --max-count=50`
  - Note which base branch was used (or "none" if fallback)
- Store the commit list as COMMITS
- Count the number of commits as COMMIT_COUNT
- If COMMIT_COUNT is 0:
  - Record finding: "NO_COMMITS: No commits found on branch {BRANCH}"

**Step 5: Gather changed files**

- Run: `git -C "{WORKTREE_PATH}" diff --name-only main..{BRANCH}`
  (use same base branch as Step 4)
- If fallback was needed in Step 4, use: `git -C "{WORKTREE_PATH}" diff --name-only HEAD~{COMMIT_COUNT}..HEAD`
- Store the file list as CHANGED_FILES

**Step 6: Check — docs-only changes**

- Examine CHANGED_FILES for code files (anything NOT matching: *.md, README*, LICENSE*, CHANGELOG*, *.txt, .gitignore)
- If ALL changed files are documentation-only:
  - Record finding: "DOCS_ONLY: All changes are documentation files. No code changes detected."
  - Mark this check as WARN
- If code files are present:
  - Mark this check as PASS

**Step 7: Check — missing tests for changed packages**

- From CHANGED_FILES, identify changed source files (*.go excluding *_test.go)
- For each unique package directory with changed .go files:
  - Use Glob to check if *_test.go files exist in that directory within the worktree
  - If no test files exist at all: record finding "MISSING_TESTS: No test files in {package_dir}" — mark as FAIL
  - If test files exist but were NOT modified in this branch (not present in CHANGED_FILES): record finding "STALE_TESTS: Tests in {package_dir} were not updated alongside code changes" — mark as WARN
- For non-Go files, apply similar logic:
  - Python: *_test.py or test_*.py
  - TypeScript/JavaScript: *.test.ts, *.test.js, *.spec.ts, *.spec.js
- If no source files were changed (docs-only): skip this check

**Step 8: Check — deferred work markers in commits**

- Run: `git -C "{WORKTREE_PATH}" log --format="%s%n%b" main..{BRANCH}`
  (use same base branch strategy as Step 4)
- Search the output for patterns (case-insensitive):
  "TODO", "FIXME", "HACK", "XXX", "deferred", "follow-up", "follow up", "punt", "skip for now", "will address later", "out of scope"
- For each match found: record finding "DEFERRED: Commit message contains deferred marker: '{matched_text}' in commit '{subject_line}'" — mark as FAIL

**Step 9: Check — acceptance criteria coverage**

- If PURPOSE is non-empty:
  - Break PURPOSE into individual requirements/criteria (split on newlines, bullet points, or numbered items)
  - For each criterion, search CHANGED_FILES and commit messages for evidence that it was addressed
  - Use Grep to search for relevant keywords from each criterion in the changed files within the worktree
  - For criteria with no evidence of implementation: record finding "UNMET_CRITERIA: No evidence for: '{criterion}'" — mark as FAIL
- If PURPOSE is empty:
  - Record finding: "NO_PURPOSE: Session has no stated purpose/acceptance criteria. Cannot verify completeness."
  - Mark as WARN (not FAIL)

**Step 10: Compile verdict**

Evaluate all findings from Steps 6-9:

- **PASS**: No FAIL-level findings (WARN findings are acceptable)
- **FAIL**: One or more FAIL-level findings exist

Classification:
- FAIL-level: MISSING_TESTS, DEFERRED, UNMET_CRITERIA
- WARN-level: DOCS_ONLY, STALE_TESTS, NO_PURPOSE, NO_COMMITS

Display the audit report:

```
=== Audit Report: {SESSION_NAME} ===
Branch:  {BRANCH}
Path:    {WORKTREE_PATH}
Commits: {COMMIT_COUNT}
Purpose: {PURPOSE or "(none)"}

--- Checks ---
[PASS|WARN|FAIL] Code changes:        {summary}
[PASS|WARN|FAIL] Test coverage:       {summary}
[PASS|WARN|FAIL] Deferred markers:    {summary}
[PASS|WARN|FAIL] Acceptance criteria: {summary}

--- Verdict: PASS or FAIL ---
```

If any FAIL findings exist, list each one with details.

**Step 11: Generate worker feedback (FAIL only)**

If verdict is FAIL, generate a message template:

```
The following gaps were found in session "{SESSION_NAME}" and must be addressed before archival:

{numbered list of FAIL-level findings with actionable descriptions}

Please address these items and re-run /agm:audit-completion {SESSION_NAME} when ready.
```

Suggest: "Send this to the worker with: /agm:agm-send {SESSION_NAME} --prompt \"<paste message>\""

**Error Handling**:
- If agm not found: "Install agm from github.com/vbonnet/dear-agent"
- If git not available in worktree: "Worktree path does not exist or is not a git repository"
- If session not found: suggest `/agm:agm-list` to see available sessions
