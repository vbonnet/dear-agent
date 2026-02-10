# The Oligo: Automated Git Integration

The Oligo is a specialized agent loop for high-friction integration tasks, specifically automated git merge conflict resolution.

## Brain/Body Architecture

### The Body (Go - This Module)

**Responsibilities:**
- Git plumbing operations (checkout, merge, conflict detection)
- Process orchestration (checkout → merge → check cycle)
- File system operations
- Infrastructure reliability

**What it DOES:**
- `git checkout <branch>`
- `git merge <source>`
- `git diff --name-only --diff-filter=U` (detect conflicts)
- `git merge --abort` (rollback)

**What it DOES NOT do:**
- Semantic conflict resolution
- Code analysis
- Intent inference
- AI-based decision making

### The Brain (Python engram)

**Responsibilities:**
- Semantic conflict resolution
- Code understanding
- Intent inference from both versions
- AI-generated merge solutions

**Invocation:**
```bash
engram resolve-conflict --prompt "<conflict description>" --repo <repo-path>
```

**Expected behavior:**
1. Read conflict markers from files
2. Analyze both versions (HEAD vs incoming)
3. Understand semantic intent of each change
4. Generate merged version preserving both intents
5. Remove conflict markers (<<<<<<<, =======, >>>>>>>)
6. Stage resolved files with `git add`

## Usage

```go
import "github.com/vbonnet/ai-tools/main/autonomous-swarm/pkg/oligo"

// Create Oligo loop
loop, err := oligo.NewOligoLoop("/path/to/repo", "engram")
if err != nil {
    log.Fatal(err)
}

// Execute integration (checkout -> merge -> resolve)
result, err := loop.ExecuteIntegration("feature-branch", "main")
if err != nil {
    log.Fatal(err)
}

if result.Success {
    fmt.Println("Integration successful!")
} else if result.HasConflict {
    fmt.Printf("Conflicts in: %v\n", result.ConflictFiles)
} else {
    fmt.Printf("Error: %v\n", result.Error)
}
```

## Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ Oligo Loop (Go - The Body)                                  │
└─────────────────────────────────────────────────────────────┘
         │
         ▼
    ┌─────────┐
    │ Checkout│  git checkout <target-branch>
    │ Target  │
    └────┬────┘
         │
         ▼
    ┌─────────┐
    │  Merge  │  git merge <source-branch>
    │ Source  │
    └────┬────┘
         │
         ▼
    ┌─────────────┐
    │ Check       │  git diff --name-only --diff-filter=U
    │ Conflicts?  │
    └──────┬──────┘
           │
      ┌────┴────┐
      │         │
   NO │         │ YES
      │         │
      ▼         ▼
  ┌───────┐  ┌──────────────────────────────┐
  │Success│  │ Invoke Engram (Python Brain) │
  └───────┘  └──────────────────────────────┘
                      │
                      ▼
              ┌────────────────┐
              │ engram         │
              │ resolve        │
              │ -conflict      │
              └───────┬────────┘
                      │
                      ▼
          ┌──────────────────────┐
          │ AI analyzes conflicts│
          │ Generates resolution │
          │ Stages files         │
          └──────────┬───────────┘
                     │
                     ▼
             ┌──────────────┐
             │ Re-check     │
             │ conflicts    │
             └──────┬───────┘
                    │
                ┌───┴────┐
                │        │
             NONE│        │REMAIN
                │        │
                ▼        ▼
            ┌───────┐  ┌──────┐
            │Success│  │Failure│
            └───────┘  └──────┘
```

## Why This Split?

### Go Handles Git Plumbing (Body Work)

**Strengths:**
- Fast, compiled binary
- Reliable system command execution
- Error handling and retries
- No Python runtime dependency
- Deterministic behavior

**Example:**
```go
cmd := exec.Command("git", "-C", repoPath, "merge", sourceBranch)
if err := cmd.Run(); err != nil {
    // Handle error, detect conflicts, retry logic
}
```

### Python Handles Semantic Resolution (Brain Work)

**Strengths:**
- AI/ML integration (Claude Code, GPT, etc.)
- Natural language understanding
- Code analysis libraries
- Flexible prompt engineering
- Rapid iteration on resolution strategies

**Example:**
```python
def resolve_conflict(file_path):
    conflict_content = read_file(file_path)
    head_version = extract_head(conflict_content)
    incoming_version = extract_incoming(conflict_content)

    # AI analyzes both versions
    prompt = f"Merge these two versions:\nHEAD: {head_version}\nIncoming: {incoming_version}"
    resolution = claude_code.ask(prompt)

    write_file(file_path, resolution)
    subprocess.run(['git', 'add', file_path])
```

## Testing

Run tests:
```bash
go test ./pkg/oligo/
```

Test coverage:
- Repository validation
- Checkout operations
- Clean merges (no conflicts)
- Conflicted merges
- Conflict detection
- Merge abort
- Branch queries
- Integration flow (end-to-end)

## Error Handling

### Go (Oligo)

- `GitOperation.Success`: Overall success flag
- `GitOperation.HasConflict`: Conflict detection
- `GitOperation.ConflictFiles`: List of conflicting files
- `GitOperation.Error`: Detailed error information

### Python (engram)

Expected exit codes:
- `0`: Conflicts resolved successfully
- `1`: Resolution failed (syntax errors, unresolvable semantic conflicts)
- `2`: User intervention required (escalation)

## Future Enhancements

1. **Parallel resolution**: Resolve multiple files concurrently
2. **Confidence scoring**: Python returns confidence level for each resolution
3. **Interactive mode**: Present options to user for low-confidence resolutions
4. **Learning**: Track successful resolutions for pattern recognition
5. **Rollback**: Automatic rollback on failed test runs post-resolution

## Related Components

- **autonomous-swarm**: Bead execution framework
- **engram**: Python AI coordination system
- **AGM**: Session monitoring and state management
