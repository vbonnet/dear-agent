# Hook Enforcement Guide

**Last Updated**: 2026-03-24
**Version**: 2.2.0
**Audience**: Claude Code Users & Developers

---

## Overview

This guide documents the bash command enforcement system (`pretool-bash-blocker` hook) that validates all bash commands before execution in Claude Code sessions.

**Purpose**: Prevent unsafe bash patterns and encourage use of Claude Code's dedicated tools (Read, Write, Edit, Glob, Grep) for better code review and safety.

**Enforcement Mode**: Hard blocking - forbidden commands are denied with exit code 2.

---

## Quick Reference

### ✅ Allowed Commands

```bash
# Git operations
git -C /path status
git commit -m "message"
git log --oneline

# Build & test
go test ./...
npm install
pytest
make build

# Directory operations
mkdir -p config/dir
mkdir build

# Pipes (relaxed in v2.2.0)
git log --oneline | head -5
docker ps | head

# Stderr redirection (relaxed in v2.2.0)
command 2>/dev/null
command 2>&1

# Input redirection (relaxed in v2.2.0)
cmd < input.txt
cmd <<<text

# Standard development tools
docker run ...
bd create ...
wayfinder-next-phase
```

### ❌ Blocked Patterns (Use Tools Instead)

| Blocked Pattern | Use Instead | Tool |
|----------------|-------------|------|
| `cd /path && git status` | `git -C /path status` | Use `-C` flag |
| `cat file.txt` | Read tool | `Read(file.txt)` |
| `grep "pattern" file` | Grep tool | `Grep("pattern")` |
| `find . -name "*.js"` | Glob tool | `Glob("**/*.js")` |
| `echo "text"` | Output directly | Response text |
| `ls -la` | Glob tool | `Glob("*")` |
| `cmd1 && cmd2` | Separate calls | Two Bash calls |
| `cmd > file.txt` | Write tool | `Write(file.txt)` |

---

## Pattern Categories (27 Total)

### 1. Directory Navigation

**Pattern**: `\bcd\s+`

**Rationale**: `cd` commands don't persist between Bash calls. Use absolute paths or `-C` flags instead.

**Examples**:
```bash
# ❌ Blocked
cd ~/project && go test ./...

# ✅ Allowed
go test -C ~/project ./...
```

**Remediation**: Use absolute paths with `-C` flags: `go -C /path test` not `cd /path && go test`

---

### 2. Command Chaining

**Pattern**: `&&`

**Rationale**: Prevents complex command chains that are hard to review and may hide dangerous operations.

**Examples**:
```bash
# ❌ Blocked
npm install && npm test
git add . && git commit -m "msg"

# ✅ Allowed (two separate calls)
Bash: npm install
Bash: npm test
```

**Remediation**: Run commands sequentially with separate Bash calls, not `&&`

---

### 3. Command Separator

**Pattern**: `;`

**Rationale**: Similar to `&&`, prevents command chains.

**Examples**:
```bash
# ❌ Blocked
make clean; make build

# ✅ Allowed
Bash: make clean
Bash: make build
```

**Remediation**: Run as separate Bash calls instead of chaining with `;`

---

### 4. Error Suppression

**Pattern**: `\|\|`

**Rationale**: Hides errors and makes debugging difficult.

**Examples**:
```bash
# ❌ Blocked
git pull || echo "Pull failed"

# ✅ Allowed
Bash: git pull
# Review output, then handle errors explicitly
```

**Remediation**: Run commands sequentially with separate Bash calls, not `||`

---

### 5. Pipe Operator — RELAXED (v2.2.0)

**Status**: No longer blocked as of v2.2.0.

**Rationale for relaxation**: Pipes are fundamental shell usage. No other AI CLI tool blocks them. We already block the commands typically piped to (grep, sed, awk, etc.), so those are caught individually. Pipes to non-blocked commands (e.g., `git log | head`) should be allowed.

**Now allowed**:
```bash
git log --oneline | head -5
docker ps | head
cmd1 | cmd2
```

**Still blocked by other patterns**: `cat file | grep text` (blocked by `cat` and `grep` patterns)

---

### 6. File Operations

**Pattern**: `\b(cat|cp|mv|rm|touch)\b`

**Rationale**: Claude Code has dedicated tools that provide better safety and review.

**Examples**:
```bash
# ❌ Blocked
cat README.md
cp file1.txt file2.txt
rm temp.log

# ✅ Allowed
Read("README.md")
Write("file2.txt", content)
# Ask user before deletions
```

**Remediation**: Use Read/Write/Edit tools instead

**Note**: `mkdir` was removed from this pattern in v2.1.0 (no Claude Code alternative exists)

---

### 7. Output Redirection

**Pattern**: `>`

**Rationale**: Use Write tool for better content review.

**Examples**:
```bash
# ❌ Blocked
echo "test" > file.txt
ls > files.txt

# ✅ Allowed
Write("file.txt", "test")
```

**Remediation**: Use the Write tool instead of redirection operators

---

### 8. Heredocs (Input Redirection Relaxed in v2.2.0)

**Pattern**: `<<` (heredoc only)

**Rationale**: Heredocs are buggy in Claude Code's bash tool. Use Write tool instead.

**Relaxed (v2.2.0)**: `<` (input redirection) and `<<<` (here-string) are no longer blocked — they are not write operations.

**Examples**:
```bash
# ❌ Blocked (heredoc)
cat <<EOF > file.txt
content
EOF

# ✅ Allowed (input redirection — relaxed)
cmd < input.txt
cmd <<<text

# ✅ Allowed (use Write tool for file creation)
Write("file.txt", "content")
```

**Remediation**: Use Write tool instead of heredocs

---

### 9. Control Flow

**Patterns**: `\bfor\b`, `\bwhile\b`, `\bif\b`, `\bthen\b`, `\bfi\b`, `\belif\b`, `\belse\b`

**Rationale**: Shell loops/conditionals are hard to review and error-prone. Use proper scripts or dedicated tools.

**Examples**:
```bash
# ❌ Blocked
for file in *.txt; do cat "$file"; done
if [ -f file.txt ]; then rm file.txt; fi

# ✅ Allowed
Glob("*.txt")  # Returns list, process in response
# Make decisions explicit in conversation
```

**Remediation**: Use dedicated tools or separate Bash calls instead of loops/conditionals

---

### 10. Text Processing

**Patterns**: `\bgrep\b`, `\bsed\b`, `\bawk\b`, `\bhead\b`, `\btail\b`, `\bwc\b`, `\bcut\b`, `\bsort\b`, `\buniq\b`

**Rationale**: Use Grep tool instead for better integration.

**Examples**:
```bash
# ❌ Blocked
grep "TODO" src/*.js
sed 's/old/new/g' file.txt
head -20 log.txt

# ✅ Allowed
Grep("TODO", glob="*.js", path="src")
Edit(file.txt, old="old", new="new")
Read(file.txt, limit=20)
```

**Remediation**: Use Grep/Edit/Read tools instead

---

### 11. File Search

**Pattern**: `\bfind\b`

**Rationale**: Use Glob tool for better performance and integration.

**Examples**:
```bash
# ❌ Blocked
find . -name "*.py"
find /path -type f -mtime -7

# ✅ Allowed
Glob("**/*.py")
# For mtime filtering, list then filter in response
```

**Remediation**: Use the Glob tool instead of find

---

### 12. Directory Listing

**Pattern**: `\bls\b`

**Rationale**: Use Glob tool to list files.

**Examples**:
```bash
# ❌ Blocked
ls -la
ls src/

# ✅ Allowed
Glob("*")
Glob("*", path="src")
```

**Remediation**: Use the Glob tool instead of ls

---

### 13. Echo/Printf

**Pattern**: `\b(echo|printf)\b`

**Rationale**: Output text directly in your response instead of echoing.

**Examples**:
```bash
# ❌ Blocked
echo "Build complete"
printf "Status: %s\n" "$status"

# ✅ Allowed
# Just output text in response:
"Build complete"
```

**Remediation**: Output text directly in your response instead of using echo/printf

---

### 14. Command Substitution

**Patterns**: `\$\(`, `` ` ``

**Rationale**: Makes command review difficult and can hide dangerous operations.

**Examples**:
```bash
# ❌ Blocked
ls $(pwd)
echo `whoami`
mkdir $(date +%Y%m%d)

# ✅ Allowed
Bash: pwd
# Then use output explicitly
Bash: ls /path/from/pwd
```

**Remediation**: Run commands separately and use outputs explicitly

---

### 15. Conditional Tests

**Patterns**: `\btest\b`, `\[`, `\[\[`

**Rationale**: Part of control flow prevention.

**Examples**:
```bash
# ❌ Blocked
test -f file.txt
[ -d /path ]
[[ "$var" == "value" ]]

# ✅ Allowed
# Check conditions explicitly in separate commands
Bash: test -f file.txt && echo "exists" || echo "missing"
# Or ask user to verify
```

**Remediation**: Make conditional checks explicit in conversation

---

### 16. Combined Redirection (Stderr Relaxed in v2.2.0)

**Patterns**: `&>`, `&>>` (combined stdout+stderr — still blocked)

**Relaxed (v2.2.0)**: `2>` and `2>>` (stderr-only redirection) are no longer blocked. These are not write operations and are commonly needed for `2>/dev/null` and `2>&1`.

**Examples**:
```bash
# ❌ Blocked (combined redirection includes stdout)
build.sh &>build.log

# ✅ Allowed (stderr-only — relaxed)
command 2>/dev/null
command 2>&1
command 2>>error.log
```

**Remediation**: For combined redirection (`&>`), capture output using separate commands

---

### 17. Python One-liners & Heredocs

**Patterns**: `python3?\s+-c`, `python3?\s*<<`

**Rationale**: Python scripts should be in files for review, not embedded.

**Examples**:
```bash
# ❌ Blocked
python3 -c "print('hello')"
python3 <<EOF
print('test')
EOF

# ✅ Allowed
Write("script.py", "print('hello')")
Bash: python3 script.py
```

**Remediation**: Create .py files and run them

---

### 18. Environment Variable Prefixes

**Patterns**: `^(GIT_DIR|GIT_WORK_TREE|GOPRIVATE|CGO_ENABLED)=\S+\s+\w`

**Rationale**: Prevents environment manipulation that could affect git or build behavior.

**Examples**:
```bash
# ❌ Blocked
GIT_DIR=/tmp/.git git status
GOPRIVATE=github.com/private go build

# ✅ Allowed
# Configure environment properly, don't use inline vars
```

**Remediation**: Don't use inline environment variables before commands

---

### 19. Git Switch/Stash

**Patterns**: `git\s+switch`, `git\s+stash`

**Rationale**: Branch switching and stashing contaminate shared state in multi-session environments.

**Examples**:
```bash
# ❌ Blocked
git switch feature-branch
git stash push
git stash pop

# ✅ Allowed
git checkout -b feature-branch
# Or use worktrees for isolation
```

**Remediation**:
- git switch: Use `git checkout -b` for new branches, or ask user to switch
- git stash: Use worktrees instead to avoid contaminating shared state

---

### 20. Git Add (Broad Staging)

**Pattern**: `git\s+add\s+(-A|\.|--all)`

**Rationale**: Prevents accidental staging of sensitive files (.env, credentials).

**Examples**:
```bash
# ❌ Blocked
git add .
git add -A
git add --all

# ✅ Allowed
git add specific-file.txt
git add src/module.py
```

**Remediation**: Stage specific files by name instead of bulk staging

**Exception**: `git worktree add`, `git remote add`, `git submodule add` are allowed (different git operations)

---

### 27. git --no-verify flag (hook bypass) [v2.3.0]

**Pattern**: `\bgit\s+[^|&;]*?(--no-verify\b|-n(\s+[^0-9]|\s*$))`

**What it blocks**: Any git command using `--no-verify` or `-n` to skip pre-commit, commit-msg, pre-push, or merge hooks.

```bash
# ❌ Blocked
git commit --no-verify
git commit -m "msg" --no-verify
git push origin main --no-verify
git merge feature-branch --no-verify
git commit -n
git -C /path/to/repo commit --no-verify
git rebase main --no-verify
git am patch.diff --no-verify

# ✅ Allowed
git commit -m "fix: update"
git push origin main
git log -n 5        # -n here means count, not --no-verify
git log -n5          # no space variant
git status
git -C /path commit -m "docs: explain no-verify prevention"
```

**Remediation**: Do not skip verification. Follow three-tier enforcement:

1. **If verification found real issues** — fix the underlying problems in your code
2. **If verification has a bug** — fix the verification hook and redeploy it
3. **If verification blocks legitimate work** — use `AskUserQuestion` to surface the issue to the user, proposing a fix for the verification (not using `--no-verify`)

**Astrocyte enforcement**: Agents must never bypass git hooks. When a pre-commit or pre-push hook fails, the agent should diagnose the failure and either fix the code or fix the hook — never skip the check. If the agent cannot resolve the issue, it must escalate to the user via `AskUserQuestion` with a proposed resolution.

---

## Testing Procedures

### Manual Testing

Test each pattern category to verify blocking:

```bash
# 1. Test cd blocking
Bash: cd /tmp && ls
# Expected: ❌ Denied with "cd command" remediation

# 2. Test command chaining
Bash: echo "test1" && echo "test2"
# Expected: ❌ Denied with "command chaining" remediation

# 3. Test file operations
Bash: cat README.md
# Expected: ❌ Denied with "file operations" remediation

# 4. Test mkdir allowance
Bash: mkdir -p build/artifacts
# Expected: ✅ Allowed

# 5. Test normal commands
Bash: git status
# Expected: ✅ Allowed
```

### Automated Testing

**Unit Tests**: `hooks/internal/validator/validator_test.go`
- 110 test cases covering all patterns
- Tests both blocked and allowed commands

**BDD Tests**: `hooks/internal/validator/validator_test.go` (Ginkgo)
- 40 behavior specs
- Readable given-when-then format

**Integration Tests**: `hooks/cmd/pretool-bash-blocker/main_test.go`
- 88 end-to-end tests (38 unit + 50 BDD)
- Tests hook executable directly

**Run All Tests**:
```bash
cd ./engram/hooks
go test ./...
```

**Expected Results**: 100% pass rate (249/249 tests)

---

## Pattern Update History

### v2.1.0 (2026-03-11) - mkdir Over-Restriction Fix
**Changed**: Removed `mkdir` from file operations pattern
**Rationale**:
- No Claude Code alternative exists for directory creation
- Glob tool can search directories but cannot create them
- All dangerous mkdir patterns already blocked:
  - `mkdir $(whoami)` → Blocked by command substitution
  - `mkdir dir && rm -rf /` → Blocked by command chaining
  - `echo dirs | xargs mkdir` → Blocked by pipe pattern
- 30+ legitimate mkdir use cases identified (config dirs, build artifacts, test setup)

**Impact**: `mkdir` and `mkdir -p` now allowed

### v2.0.0 (2026-02-24) - JSON Protocol & Fail-Open
**Changed**: Updated to Claude Code v2 JSON protocol, added fail-open for invalid JSON
**Patterns Added**: 8 new patterns (35 total before v2.1)

### v1.0.0 (2024) - Initial Release
**Created**: 27 core patterns for bash command validation

---

## Rationale Summary

### Why Block These Patterns?

1. **Safety**: Prevent destructive operations (`rm -rf`, hidden in chains)
2. **Reviewability**: Dedicated tools show exact file content/changes
3. **Consistency**: Enforce Claude Code best practices
4. **Security**: Prevent command injection and environment manipulation
5. **Multi-session**: Avoid state contamination (git stash, cd)

### Why Use Dedicated Tools?

| Tool | Benefits |
|------|----------|
| **Read** | Shows exact content in context, supports line ranges |
| **Write** | Shows full file content for review before writing |
| **Edit** | Shows exact diff (old_string → new_string) |
| **Grep** | Better regex support, context lines, file filtering |
| **Glob** | Fast file search, supports ** patterns, sorted by mtime |

---

## Troubleshooting

### Common Issues

**Q: My legitimate command is blocked. What do I do?**

A: Check if there's a dedicated tool alternative in the Quick Reference table. If not, consider:
1. Breaking the command into parts (remove chaining/piping)
2. Using flags like `-C` instead of `cd`
3. Creating a script file and running it directly

**Q: Why can't I use `cat` to read files?**

A: The Read tool provides better integration:
- Shows content in conversation context
- Supports line ranges (offset/limit)
- Better for large files (pagination)
- Reviewable in tool call history

**Q: Why is `mkdir` allowed but `cat` is blocked?**

A: No Claude Code alternative exists for `mkdir`. The Glob tool can search directories but cannot create them. All dangerous mkdir patterns are still blocked by other rules (command substitution, chaining, piping).

**Q: How do I run multiple commands in sequence?**

A: Use separate Bash tool calls instead of chaining:
```python
# ❌ Blocked
Bash("npm install && npm test")

# ✅ Allowed
Bash("npm install")
Bash("npm test")
```

**Q: Tests are failing. How do I verify the hook works?**

A: Run the full test suite:
```bash
cd ./engram/hooks
go test ./... -v
```

Expected: 249/249 tests passing (159 unit + 90 BDD)

---

## For Developers

### Adding New Patterns

1. **Update patterns.go**: Add new pattern to appropriate category
2. **Update SPEC.md**: Document pattern with rationale
3. **Add unit tests**: Add test cases to validator_test.go
4. **Add BDD tests**: Add specs to validator_test.go (Ginkgo)
5. **Update this guide**: Add pattern to relevant section
6. **Test**: Run `go test ./...` and verify 100% pass

### Pattern Evaluation Order

Patterns are evaluated in array order (index 0 → 26). First match wins.

**Precedence**:
1. ENV= prefix (index 0)
2. cd command (subshell + general)
3. Command chaining (&&, ||)
4. Control flow (if/for/while)
5. Python execution (-c, heredoc)
6. Command separator (;)
7. Command substitution ($(), backticks)
8. Test commands (test/[/[[)
9. Redirections (stdout + heredoc only)
10. File/text processing commands
11. Git safety (switch, stash, add)
(see patterns.go for full order)

### Hook Deployment

```bash
# Build hooks
make -C ./engram/hooks build-macos

# Deploy to ~/.claude/hooks/
engram/hooks/deploy.sh

# Verify deployment
ls -la ~/.claude/hooks/pretool-bash-blocker
```

### Hook Logging

Logs are written to: `~/.claude/hooks/logs/pretool-bash-blocker.log`

**Log format**:
```
[timestamp] DENY Pattern #N matched: <pattern_name> | Command: <command>
[timestamp] DENY Remediation: <suggestion>
[timestamp] ALLOW Command approved: <command>
```

---

## References

- **SPEC.md**: Full technical specification (`hooks/cmd/pretool-bash-blocker/SPEC.md`)
- **Source Code**: `hooks/internal/validator/patterns.go`
- **Tests**: `hooks/internal/validator/validator_test.go`
- **Integration Tests**: `hooks/cmd/pretool-bash-blocker/main_test.go`
- **Coverage Report**: Run `go test -cover ./...`

---

## Version

**Guide Version**: 2.2.0
**Hook Version**: 2.2.0
**Last Updated**: 2026-03-24
**Pattern Count**: 27

---

**Questions or Issues?**

File issues at: `engram/hooks/` (add to backlog)
