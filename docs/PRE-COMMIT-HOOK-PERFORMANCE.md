# Pre-Commit Hook Performance

## Overview

The pre-commit hook validates staged files for path portability and documentation quality. This document defines performance expectations and testing procedures to prevent "death by a thousand cuts" where the hook gradually slows down commits.

## Performance Thresholds

| Threshold | Time Range | Status | Description |
|-----------|------------|--------|-------------|
| ✅ Excellent | <100ms | Acceptable | Typical commits (1-50 files) |
| ⚠️ Warning | 100-500ms | Review | Large commits, consider optimization |
| ❌ Critical | >500ms | Fix Required | Blocks developer flow, must optimize |

## Current Performance (as of 2026-01-08)

Benchmark results using `scripts/benchmark-pre-commit-hook.sh`:

```
Test 1: Empty staging area
✅ No files                         0 files:   15ms

Test 2: Shell scripts with validation
✅ 1 shell file                     1 files:   14ms

Test 3: Scaling with file count
✅ Shell files                      5 files:   18ms
✅ Shell files                     10 files:   14ms
✅ Shell files                     20 files:   15ms
✅ Shell files                     50 files:   15ms

Test 4: Mixed file types (shell + Python + Markdown)
✅ Mixed types                      3 files:   16ms

Test 5: Slash command documentation validation
✅ Command file                     1 files:   14ms
```

**Summary**: Hook executes in **14-18ms** across all scenarios, well under the 100ms threshold.

## Performance Characteristics

### Startup Overhead
- Base hook initialization: ~10-15ms
- Includes shell script parsing, git operations, and environment setup

### Scaling Behavior
- **Near-constant time**: Hook performance does not degrade with file count
- 1 file: 14ms → 50 files: 15ms (minimal scaling)
- Efficient early-exit logic when no relevant files are staged

### Validation Cost
Each validator runs only on matching files:
- **Path portability** (shell/Python/Makefile): ~1-2ms per 10 files
- **Documentation quality** (commands/\*.md): ~1-2ms per file

## Timeout Protection

The hook includes a 5-second timeout for validation functions to prevent infinite hangs:

```bash
# In pre-commit hook
timeout 5 "$VALIDATION_FUNCTION" || handle_timeout
```

If validation exceeds 5 seconds, the commit is aborted with a clear error message.

## Testing Performance

### Running the Benchmark

```bash
./scripts/benchmark-pre-commit-hook.sh
```

### Manual Testing

Test hook execution time on current staged files:

```bash
time timeout 5 .git/hooks/pre-commit
```

### When to Re-Benchmark

- After adding new validation rules
- After modifying grep/sed patterns
- Quarterly (to catch gradual degradation)
- Before major releases

## Optimization Guidelines

If the hook exceeds 100ms threshold:

### 1. Profile the Bottleneck
```bash
# Add timing to each validator
time validate_shell_scripts "$STAGED_FILES"
time validate_makefiles "$STAGED_FILES"
time validate_python_files "$STAGED_FILES"
time validate_docs "$STAGED_FILES"
```

### 2. Common Bottlenecks

**Grep/sed operations on large files**:
- ❌ `grep -nE 'pattern' "$file"`
- ✅ `grep -nE 'pattern' "$file" | head -100` (limit output)

**Unbounded loops**:
- ❌ Processing all violations immediately
- ✅ Collect violations, report in batch

**Expensive git operations**:
- ❌ Multiple `git diff` calls per file
- ✅ Single `git diff --cached --name-only` upfront

### 3. Optimization Checklist

- [ ] Use `grep -q` for existence checks (don't capture output)
- [ ] Limit output with `| head -N` when only first N results matter
- [ ] Cache `git rev-parse` results (don't repeat per-file)
- [ ] Skip validation if no relevant files are staged
- [ ] Use early-exit (`return 0`) when possible

## Historical Context

### 2026-01-07: Hook Hang Bug

**Issue**: Hook hung indefinitely on commits with 65+ files
**Root Cause**: Hook validation ran twice due to chaining to `pre-commit.old`
**Fix**: Disabled `pre-commit.old` to prevent double execution
**Lesson**: Test hook with large commits to catch scaling issues

See the git history for details on the original investigation.

## Monitoring

### CI Integration (Future)

Add performance check to CI:

```bash
# In CI pipeline
MAX_TIME_MS=200  # Allow more headroom in CI
ACTUAL_TIME=$(./scripts/benchmark-pre-commit-hook.sh | grep "50 files" | awk '{print $4}')

if [ "$ACTUAL_TIME" -gt "$MAX_TIME_MS" ]; then
    echo "ERROR: Hook performance degraded to ${ACTUAL_TIME}ms (threshold: ${MAX_TIME_MS}ms)"
    exit 1
fi
```

### Git Hook Metrics (Future)

Track hook execution time across all commits:

```bash
# In pre-commit hook
START=$(date +%s.%N)
# ... validation ...
END=$(date +%s.%N)
DURATION_MS=$(echo "($END - $START) * 1000" | bc)
echo "$(date -u +%Y-%m-%dT%H:%M:%SZ) | hook_duration_ms | $DURATION_MS" >> ~/.engram/logs/git-hook-perf.log
```

## References

- Benchmark script: `scripts/benchmark-pre-commit-hook.sh`
- Hook implementation: `.bare/hooks/pre-commit`
- Issue tracker: Document performance regressions in `/docs/issues/`
