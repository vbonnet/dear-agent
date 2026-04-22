> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> The bash-blocker migration to Go is complete (v2.3.0).
> Remaining hooks listed here may still need migration — verify before acting on this plan.
> Kept for historical reference.

# Hook Migration Plan - Python to Go Functional Parity

## Status

**Current State**: Go hooks exist but lack critical features from Python versions
**Completion**: ~40% (basic functionality only)
**Remaining Work**: 6-8 hours for full functional parity

---

## Hooks Requiring Enhancement

### 1. posttool-auto-commit-beads

**Current Go Implementation** (`hooks/cmd/posttool-auto-commit-beads/main.go`):
- ✅ Detects bead operations (bd create/update/close/abandon)
- ✅ Auto-commits .beads/issues.jsonl
- ✅ Basic commit messages

**Python Features NOT Yet Implemented**:
- ❌ **Context Extraction** (lines 161-209 in Python)
  - Extract bead ID from command: `"bd create oss-lmtg"` → `"oss-lmtg"`
  - Extract operation type: create | update | import | close | abandon
  - Extract labels from `--label` flags: `["phase-3", "swarm"]`

- ❌ **Git Commit Linking** (lines 212-249 in Python)
  - Get most recent git commit hash (8-char short)
  - Get commit subject line
  - Include in commit message for traceability

- ❌ **Configuration Management** (lines 57-91 in Python)
  - Load config from `~/.claude/hooks/beads-hook-config.yaml`
  - Feature flags: `context_extraction`, `git_commit_context`, `session_statistics`
  - Performance timeouts: `timeout_git_command_ms`, `max_execution_time_ms`

- ❌ **Sensitive Data Sanitization** (lines 46-50 in Python)
  - Redact patterns: `token: [REDACTED]`, `password: [REDACTED]`, etc.
  - Apply before committing to prevent credential leaks

- ❌ **Semantic Commit Messages**
  - Current: Generic "chore: auto-commit beads database changes"
  - Needed: Context-aware messages like:
    ```
    beads: Create oss-lmtg [phase-3, swarm]

    Context: Recent commit abc1234 (feat: Add config loader)
    Operation: create
    Bead: oss-lmtg
    Labels: phase-3, swarm

    🤖 Auto-committed by engram posttool-auto-commit-beads hook
    ```

**Implementation Approach**:
1. Add `extractBeadContext()` function (parse command string)
2. Add `extractGitContext()` function (git log with timeout)
3. Add `loadConfig()` function (YAML parsing)
4. Add `sanitizeSensitiveData()` function (regex replacements)
5. Update `main()` to use extracted context in commit messages

**Estimated Effort**: 4 hours

---

### 2. pretool-worktree-enforcer

**Current Go Implementation** (`hooks/cmd/pretool-worktree-enforcer/main.go`):
- ✅ Detects worktree paths
- ✅ Redirects file operations (Write/Edit/Read) to worktree
- ✅ Basic path resolution

**Missing Features**:
- ❌ **Session State Management**
  - Track current worktree session
  - Remember last worktree path
  - Persist across tool calls (in-memory cache)

- ❌ **LRU Cache for Performance**
  - Cache git command results (rev-parse, show-toplevel)
  - Reduce redundant git invocations (40% performance improvement in Python)
  - Max 100 entries, TTL 5 minutes

- ❌ **Configuration Loading**
  - Load from `~/.claude/hooks/worktree-enforcer-config.yaml`
  - Feature flags: `cache_enabled`, `session_tracking_enabled`
  - Performance tuning: `cache_max_entries`, `cache_ttl_seconds`

**Implementation Approach**:
1. Add `sessionState` struct to track current worktree
2. Add `lruCache` struct with expiry timestamps
3. Add `loadConfig()` for YAML configuration
4. Update `detectWorktree()` to use cache
5. Add `updateSession()` to persist worktree context

**Estimated Effort**: 3 hours

---

### 3. pretool-validate-paired-files

**Current Go Implementation** (`hooks/cmd/pretool-validate-paired-files/main.go`):
- ✅ Basic paired file validation (e.g., `.why.md` for `.ai.md`)
- ✅ Returns validation errors

**Missing Features** (if Python version has them):
- Check Python version at `hooks/pretool-validate-paired-files.py` for comparison
- Likely needs configuration for custom pairing rules

**Estimated Effort**: 1 hour (minimal gaps expected)

---

## Go-Specific Implementation Notes

### YAML Configuration Parsing

Use `gopkg.in/yaml.v3` for config files:

```go
import "gopkg.in/yaml.v3"

type HookConfig struct {
    ContextExtraction struct {
        Enabled           bool `yaml:"enabled"`
        GitCommitContext  bool `yaml:"git_commit_context"`
        BeadLabels        bool `yaml:"bead_labels"`
        SessionStatistics bool `yaml:"session_statistics"`
    } `yaml:"context_extraction"`
    Performance struct {
        TimeoutGitCommandMs   int `yaml:"timeout_git_command_ms"`
        TimeoutSessionStatsMs int `yaml:"timeout_session_stats_ms"`
        MaxExecutionTimeMs    int `yaml:"max_execution_time_ms"`
    } `yaml:"performance"`
}

func loadConfig() (*HookConfig, error) {
    paths := []string{
        filepath.Join(os.Getenv("HOME"), ".claude", "hooks", "beads-hook-config.yaml"),
        filepath.Join(os.Getenv("HOME"), ".config", "engram", "beads-hook-config.yaml"),
    }

    for _, path := range paths {
        data, err := os.ReadFile(path)
        if err != nil {
            continue // Try next path
        }

        var config HookConfig
        if err := yaml.Unmarshal(data, &config); err != nil {
            return nil, err
        }
        return &config, nil
    }

    return defaultConfig(), nil
}
```

### Regex-Based Context Extraction

```go
import "regexp"

var (
    beadOpRegex = regexp.MustCompile(`bd\s+(create|update|import|close|abandon)`)
    beadIDRegex = regexp.MustCompile(`bd\s+(?:create|update)\s+(\S+?)(?:\s|$)`)
    labelRegex  = regexp.MustCompile(`--label\s+(\S+)`)
)

func extractBeadContext(command string) map[string]interface{} {
    opMatch := beadOpRegex.FindStringSubmatch(command)
    if opMatch == nil {
        return map[string]interface{}{}
    }

    operation := opMatch[1]
    beadID := ""
    if operation == "create" || operation == "update" {
        if idMatch := beadIDRegex.FindStringSubmatch(command); idMatch != nil {
            beadID = idMatch[1]
        }
    }

    labels := []string{}
    for _, match := range labelRegex.FindAllStringSubmatch(command, -1) {
        labels = append(labels, match[1])
    }

    return map[string]interface{}{
        "bead_id":   beadID,
        "operation": operation,
        "labels":    labels,
    }
}
```

### Git Context with Timeout

```go
import (
    "context"
    "os/exec"
    "time"
)

func extractGitContext(repoPath string, timeoutMs int) map[string]string {
    ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutMs)*time.Millisecond)
    defer cancel()

    cmd := exec.CommandContext(ctx, "git", "log", "-1", "--format=%h|%s")
    cmd.Dir = repoPath
    output, err := cmd.Output()
    if err != nil {
        return map[string]string{}
    }

    parts := strings.SplitN(strings.TrimSpace(string(output)), "|", 2)
    if len(parts) != 2 {
        return map[string]string{}
    }

    return map[string]string{
        "commit_hash":    parts[0],
        "commit_subject": parts[1],
    }
}
```

### Sensitive Data Sanitization

```go
var sensitivePatterns = []struct {
    pattern     *regexp.Regexp
    replacement string
}{
    {regexp.MustCompile(`token[:\s=]+\S+`), "token: [REDACTED-TOKEN]"},
    {regexp.MustCompile(`password[:\s=]+\S+`), "password: [REDACTED-PASSWORD]"},
    {regexp.MustCompile(`secret[:\s=]+\S+`), "secret: [REDACTED-SECRET]"},
    {regexp.MustCompile(`key[:\s=]+\S+`), "key: [REDACTED-KEY]"},
}

func sanitizeSensitiveData(text string) string {
    for _, sp := range sensitivePatterns {
        text = sp.pattern.ReplaceAllString(text, sp.replacement)
    }
    return text
}
```

### LRU Cache Implementation

```go
import "container/list"

type cacheEntry struct {
    key      string
    value    string
    expiry   time.Time
}

type lruCache struct {
    maxEntries int
    ttl        time.Duration
    cache      map[string]*list.Element
    ll         *list.List
}

func newLRUCache(maxEntries int, ttl time.Duration) *lruCache {
    return &lruCache{
        maxEntries: maxEntries,
        ttl:        ttl,
        cache:      make(map[string]*list.Element),
        ll:         list.New(),
    }
}

func (c *lruCache) Get(key string) (string, bool) {
    if elem, ok := c.cache[key]; ok {
        entry := elem.Value.(*cacheEntry)
        if time.Now().Before(entry.expiry) {
            c.ll.MoveToFront(elem)
            return entry.value, true
        }
        // Expired - remove
        c.ll.Remove(elem)
        delete(c.cache, key)
    }
    return "", false
}

func (c *lruCache) Set(key, value string) {
    if elem, ok := c.cache[key]; ok {
        c.ll.MoveToFront(elem)
        entry := elem.Value.(*cacheEntry)
        entry.value = value
        entry.expiry = time.Now().Add(c.ttl)
        return
    }

    entry := &cacheEntry{
        key:    key,
        value:  value,
        expiry: time.Now().Add(c.ttl),
    }
    elem := c.ll.PushFront(entry)
    c.cache[key] = elem

    if c.ll.Len() > c.maxEntries {
        oldest := c.ll.Back()
        if oldest != nil {
            c.ll.Remove(oldest)
            delete(c.cache, oldest.Value.(*cacheEntry).key)
        }
    }
}
```

---

## Testing Requirements

Each enhanced hook MUST have:

1. **Unit Tests** (table-driven)
   - Context extraction: 10 test cases (valid commands, edge cases, malformed input)
   - Config loading: 5 test cases (valid YAML, missing file, invalid YAML)
   - Sanitization: 8 test cases (all sensitive patterns + combinations)
   - LRU cache: 6 test cases (hit, miss, expiry, eviction)

2. **Integration Tests**
   - End-to-end hook execution with mock stdin/stdout
   - Git command integration (requires test repo fixture)
   - Config file integration (use testdata/)

3. **Benchmark Tests**
   - Measure performance improvement from caching
   - Ensure <100ms total execution time (Python baseline: 50ms)

**Test Coverage Target**: ≥90% (matches Task 3.3 requirement)

---

## Build & Deployment

```bash
# Build hooks
cd ./engram/hooks
make build-linux        # For Linux systems
make build-macos        # For macOS (amd64 + arm64)

# Run tests
make test

# Deploy to ~/.claude/hooks/
./deploy.sh

# Verify deployment
ls -lh ~/.claude/hooks/posttool-auto-commit-beads
```

---

## Acceptance Criteria

Task 3.1 complete when:

1. ✅ All Python features implemented in Go
2. ✅ Config file support (YAML parsing)
3. ✅ Tests written (≥90% coverage)
4. ✅ All tests passing (`make test`)
5. ✅ Benchmarks show <100ms execution time
6. ✅ Hooks deployed to `~/.claude/hooks/`
7. ✅ Manual testing: Create bead → verify semantic commit message
8. ✅ Manual testing: File operation in worktree → verify redirection
9. ✅ Documentation updated (this file)

---

## Why This Work Remains

**Complexity Underestimated**: Initial estimate was "70% already done" but actual parity gap is ~60%.

**Permission Limitations**: Automated agents hit bash/build permission blocks, preventing:
- Running `make build-linux`
- Running `make test`
- Running git commands for integration tests
- Deploying hooks to `~/.claude/hooks/`

**Manual Work Required**: This enhancement requires:
1. Local development environment (Go 1.21+)
2. Git test fixtures for integration tests
3. Iterative build/test cycles
4. Manual verification of hook behavior

**Recommendation**: Complete as standalone task outside swarm timeboxing.

---

**Last Updated**: 2026-02-19
**Bead**: oss-5kaq (Task 3.1)
**Phase**: 3 (Language Consolidation - Implementation)
