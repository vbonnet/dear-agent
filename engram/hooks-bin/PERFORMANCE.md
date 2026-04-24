> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> Performance figures here reflect the old Python hooks.
> Go hooks now handle all bash validation with <1ms overhead.
> See `cmd/pretool-bash-blocker/SPEC.md` for current performance data.
> Kept for historical reference.

# Hook Performance Analysis

## Measurements

### Startup Overhead
- **Python**: 44ms (with json, sys, os, re imports)
- **Bash**: 6ms (7x faster)
- **Go binary**: <1ms (40x faster, but requires compilation)

### Current Hook Execution Times
- **pretool-bash-blocker.py**: ~296ms per execution (325 lines, complex regex)
- **pretool-beads-protection.py**: ~50-60ms per execution (57 lines, simple)
- **pretool-worktree-enforcer.py**: ~100-200ms per execution (744 lines, git operations)

## Impact Analysis

### Tool Call Frequencies (Typical Session)
- **Read**: 50-100 calls
- **Write**: 20-50 calls
- **Edit**: 10-30 calls
- **Bash**: 10-50 calls

### Cumulative Overhead (Per Session)

**Current State (All Python):**
```
Read/Write/Edit hooks (beads-protection + worktree-enforcer):
  100 calls × (50ms + 150ms) = 20 seconds

Bash hooks (bash-blocker):
  30 calls × 296ms = 8.9 seconds

Total overhead: ~29 seconds per session
```

**Optimized State (Bash for simple hooks):**
```
Read/Write/Edit hooks (bash versions):
  100 calls × (10ms + 10ms) = 2 seconds

Bash hooks (compiled Go):
  30 calls × 5ms = 0.15 seconds

Total overhead: ~2.2 seconds per session

Savings: 26.8 seconds (92% reduction)
```

## Optimization Recommendations

### Priority 1: Simple Hooks → Bash (High Impact, Low Effort)

**pretool-beads-protection.py** (57 lines):
- Current: 50ms per execution
- Target: 10ms per execution (bash)
- Frequency: Very high (every Read/Write/Edit)
- **Impact: 4-5 seconds saved per session**

Logic:
```bash
#!/bin/bash
# Read JSON, check if path contains "/.beads/"
jq -r '.parameters.file_path // ""' | grep -q '/.beads/' && {
  echo '❌ Direct access to .beads/ blocked' >&2
  exit 1
}
exit 0
```

**pretool-validate-paired-files.py** (76 lines):
- Current: 50-60ms per execution
- Target: 10ms per execution (bash)
- Frequency: High (every Edit/Write)
- **Impact: 2-3 seconds saved per session**

Logic:
```bash
#!/bin/bash
# Read JSON, check if path ends with .ai.md, warn if no .why.md
path=$(jq -r '.parameters.file_path // ""')
[[ "$path" == *.ai.md ]] && {
  echo "⚠️  Warning: Editing .ai.md, ensure .why.md is paired" >&2
}
exit 0
```

### Priority 2: Complex Regex → Compiled Go (High Impact, Medium Effort)

**pretool-bash-blocker.py** (325 lines):
- Current: 296ms per execution
- Target: 5-10ms per execution (compiled Go)
- Frequency: Medium (every Bash call)
- **Impact: 8-9 seconds saved per session**

Why Go:
- Fast regex compilation (happens at build time)
- Single binary, no runtime overhead
- Easy JSON parsing (encoding/json)
- Still maintainable (easier than bash for complex logic)

Prototype structure:
```go
package main

import (
    "encoding/json"
    "os"
    "regexp"
)

type ToolCall struct {
    Name       string                 `json:"name"`
    Parameters map[string]interface{} `json:"parameters"`
}

var forbiddenPatterns = []*regexp.Regexp{
    regexp.MustCompile(`^\s*cd\s+`),
    regexp.MustCompile(`\s+&&\s+`),
    // ... compile all patterns at startup
}

func main() {
    var call ToolCall
    json.NewDecoder(os.Stdin).Decode(&call)

    command := call.Parameters["command"].(string)

    for _, pattern := range forbiddenPatterns {
        if pattern.MatchString(command) {
            // Output error, exit 1
        }
    }
    os.Exit(0)
}
```

Build once: `go build -o pretool-bash-blocker hooks/pretool-bash-blocker.go`

### Priority 3: Complex Logic → Keep Python or Go

**pretool-worktree-enforcer.py** (744 lines):
- Current: 100-200ms per execution
- Target: Keep Python (complexity justifies overhead) OR rewrite in Go
- Frequency: High (every Write/Edit/MultiEdit)
- **Impact: 10-15 seconds saved if rewritten in Go**

Decision factors:
- Very complex logic (git detection, caching, provisioning)
- Python version is maintainable, well-tested
- Go version would be faster but harder to maintain
- **Recommendation: Keep Python unless profiling shows specific bottlenecks**

## Implementation Plan

### Phase 1: Quick Wins (1-2 hours)
1. Rewrite pretool-beads-protection.py → pretool-beads-protection.sh
2. Rewrite pretool-validate-paired-files.py → pretool-validate-paired-files.sh
3. Test thoroughly
4. Update settings.json

**Expected savings: 6-8 seconds per session**

### Phase 2: High Impact (4-6 hours)
1. Rewrite pretool-bash-blocker.py → Go version
2. Add build step to install script
3. Test all violation patterns
4. Update settings.json

**Expected savings: 8-9 seconds per session**

### Phase 3: Measure & Decide (2-3 hours)
1. Add timing instrumentation to worktree-enforcer
2. Profile actual execution
3. Decide: keep Python or rewrite in Go
4. If Go: implement and test

**Potential savings: 10-15 seconds per session**

## Maintenance Tradeoffs

### Bash Pros:
- ✅ 7x faster startup than Python
- ✅ No dependencies
- ✅ Simple for simple logic
- ❌ Complex regex/logic gets unmaintainable
- ❌ JSON parsing requires jq

### Python Pros:
- ✅ Readable, maintainable
- ✅ Great for complex logic
- ✅ Rich standard library
- ❌ 44ms startup overhead
- ❌ Slower than compiled languages

### Go Pros:
- ✅ 40x faster startup than Python
- ✅ Compiled binary (no runtime)
- ✅ Good for complex logic
- ✅ Fast regex (compiled at build time)
- ❌ Requires build step
- ❌ More verbose than Python

## Decision Matrix

| Hook | Lines | Complexity | Frequency | Language | Reasoning |
|------|-------|------------|-----------|----------|-----------|
| beads-protection | 57 | Simple | Very High | **Bash** | String matching only |
| paired-files | 76 | Simple | High | **Bash** | String matching only |
| bash-blocker | 325 | Medium | Medium | **Go** | Complex regex, high impact |
| worktree-enforcer | 744 | Complex | High | **Python** | Maintainability > speed |

## Recommended Action

**Start with Phase 1** (bash rewrites of simple hooks):
- Low effort (1-2 hours)
- Immediate impact (6-8 seconds saved)
- Low risk (simple logic, easy to test)

Then **measure actual user experience** before committing to Phase 2/3.

## Testing Strategy

For each rewrite:
1. Test all original test cases
2. Add timing instrumentation
3. Measure before/after
4. Ensure no regressions
5. Update documentation

## Monitoring

Add timing to all hooks:
```bash
start=$(date +%s%N)
# ... hook logic ...
end=$(date +%s%N)
echo "Hook execution: $((($end - $start) / 1000000))ms" >> ~/.claude-hook-timing.log
```

Review timing logs weekly to identify optimization opportunities.
