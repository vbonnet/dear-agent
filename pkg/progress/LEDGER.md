# Progress Ledger - Commit-Anchored Progress Tracking

<!-- Last audited at: 2026-06-24 -->

**Version**: 1.0.0  
**Status**: Implemented  
**Purpose**: Anchor work session progress to git commits for resilience during context compaction and session restarts

---

## Overview

The Progress Ledger enables long-running worker sessions to record milestones in git commits with structured, machine-parseable messages. When a session experiences context compaction, crashes, or restarts, the worker can read `git log` to reconstruct its progress state instead of relying solely on context windows or verbose bead notes.

**Problem**: Long-running Claude Code worker sessions lose granular state on compaction or restart, making it difficult to resume work at the exact phase that was interrupted.

**Solution**: Anchor each significant milestone to a git commit with Progress-* trailers (RFC 2822 style). The commit message becomes the ledger—persistent, queryable, and tamper-resistant.

---

## Design Principles

1. **Commit-Centric**: Each milestone is recorded at commit time. Commits are the source of truth.
2. **Structured & Parseable**: Trailers use a fixed format for reliable machine parsing.
3. **Resume-Safe**: A restarted session can read `git log` and understand exactly where work left off.
4. **Lightweight**: Minimal overhead—one commit per significant phase transition.
5. **Complementary**: Works alongside beads, session manifests, and task ledgers—not a replacement.

---

## Commit Message Format

Each progress-tracked commit uses **Git conventional commit trailers** (RFC 2822 style):

```
<type>(<scope>): <subject>

<optional body>

Progress-Phase: <phase>
Progress-Milestone: <milestone-id>
Progress-Status: <status>
Progress-Timestamp: <ISO8601>
Bead-ID: <bead-id>
```

### Trailers

| Trailer | Format | Example | Purpose |
|---------|--------|---------|---------|
| `Progress-Phase` | string | `design`, `implementation`, `verification` | Current work phase |
| `Progress-Milestone` | string | `ce-ek4f-design-api` | Unique milestone ID for this bead |
| `Progress-Status` | enum | `in-progress`, `blocked`, `paused`, `done` | State at commit time |
| `Progress-Timestamp` | RFC3339 | `2026-06-24T10:30:00Z` | Commit timestamp (machine-parseable) |
| `Bead-ID` | string | `ce-ek4f` | Links to outcome tracking (bead) |

### Example Commit

```
feat(api-design): implement RESTful endpoints

Designed and documented all API endpoints for user authentication
and session management. Review-ready for security team.

Progress-Phase: design
Progress-Milestone: ce-ek4f-api-design
Progress-Status: done
Progress-Timestamp: 2026-06-24T10:30:00Z
Bead-ID: ce-ek4f
```

---

## API Reference

### Creating a Ledger

```go
ledger := progress.NewLedger(repoPath, beadID)
```

- `repoPath`: Path to git repository (default: `.` if empty)
- `beadID`: Bead ID linking this work to outcome tracking (e.g., `ce-ek4f`)

### Recording a Milestone

```go
sha, err := ledger.Commit(phase, milestone, status, body)
```

- `phase`: Current work phase (required)
- `milestone`: Unique milestone ID for this bead (required)
- `status`: State at commit time (required): `in-progress`, `blocked`, `paused`, `done`
- `body`: Optional commit message body for additional context
- Returns: commit SHA, or error if validation fails

### Querying Progress

#### Get Last Recorded Phase

```go
phase, milestone, status, timestamp, sha, err := ledger.LastPhase()
```

Returns the most recent progress milestone. Use this on session resume to understand where work left off.

#### Query All Progress Commits

```go
entries, err := ledger.QueryLog(since)
```

- `since`: Optional git revision spec (e.g., SHA, branch, timestamp range). Empty string queries all commits.
- Returns: `[]ProgressEntry` in reverse chronological order (newest first)

### ProgressEntry Structure

```go
type ProgressEntry struct {
    Phase      string    // e.g., "design", "implementation"
    Milestone  string    // e.g., "ce-ek4f-design-api"
    Status     string    // "in-progress", "blocked", "paused", "done"
    Timestamp  time.Time // ISO8601 parsed timestamp
    BeadID     string    // Links to outcome tracking
    CommitSHA  string    // Git commit SHA
    Subject    string    // First line of commit message
}
```

---

## Usage Examples

### Basic Session Checkpoint

```go
ledger := progress.NewLedger(".", "ce-ek4f")

// After completing exploration phase
sha, err := ledger.Commit("exploration", "ce-ek4f-explore", "done", 
    "Analyzed codebase structure and identified key components")
if err != nil {
    log.Fatalf("Failed to commit milestone: %v", err)
}
fmt.Printf("Exploration phase recorded at %s\n", sha)
```

### Session Resume

```go
ledger := progress.NewLedger(".", "ce-ek4f")

// On session startup, check if there's prior progress
phase, milestone, status, ts, sha, err := ledger.LastPhase()
if err != nil {
    log.Fatalf("Failed to query progress: %v", err)
}

if phase != "" {
    fmt.Printf("Resuming from %s phase (%s) at %s\n", phase, milestone, ts)
    // Resume work from this point
} else {
    fmt.Println("Starting fresh work (no prior progress)")
}
```

### Auditing Work Progress

```go
ledger := progress.NewLedger(".", "ce-ek4f")

// Get all progress entries for this bead
entries, err := ledger.QueryLog("")
if err != nil {
    log.Fatalf("Failed to query log: %v", err)
}

for _, e := range entries {
    fmt.Printf("[%s] %s: %s (status: %s)\n", e.Timestamp.Format(time.RFC3339), 
        e.Milestone, e.Phase, e.Status)
}
```

### Blocked Work Checkpoint

```go
ledger := progress.NewLedger(".", "ce-ek4f")

// Record that work is blocked waiting for external input
sha, err := ledger.Commit("implementation", "ce-ek4f-impl-auth", "blocked",
    "Implementation blocked: waiting for security team approval of auth design")
if err != nil {
    log.Fatalf("Failed to record blocking milestone: %v", err)
}
```

---

## Integration with Session Management

### Storing Last Milestone in Session Manifest

When integrating with AGM's session manifest (future enhancement):

```go
// In session manifest (manifest.go)
type SessionManifest struct {
    // ... existing fields ...
    LastProgressMilestone string    `yaml:"last_progress_milestone"`
    LastProgressTimestamp time.Time `yaml:"last_progress_timestamp"`
}

// On session startup
func (m *SessionManifest) ResumeFromLedger(repoPath string) error {
    ledger := progress.NewLedger(repoPath, m.BeadID)
    phase, milestone, status, ts, sha, err := ledger.LastPhase()
    if err != nil {
        return err
    }
    m.LastProgressMilestone = sha
    m.LastProgressTimestamp = ts
    // Use these to rehydrate session state
    return nil
}
```

---

## Testing

The ledger includes comprehensive unit tests covering:

- **Basic commit recording** — verify commits include all trailers
- **Milestone querying** — retrieve progress entries from git log
- **Timestamp parsing** — validate RFC3339 timestamp handling
- **Validation** — required fields validation (phase, milestone, status)
- **Multiple ledgers** — entries from different beads tagged correctly
- **Empty repos** — handle graceful fallback when no progress exists

Run tests:
```bash
go test ./pkg/progress -v -run TestLedger
```

---

## Edge Cases & Behaviors

### Empty Repository (No Progress Yet)

`LastPhase()` returns empty values for phase, milestone, status, and zero timestamp. This is expected and safe—sessions can detect this and start fresh work.

### Multiple Beads in Same Repo

Each `Ledger` instance has its own `BeadID`. All progress commits are visible in the same git history, but each entry preserves its bead tag for linking to outcome records.

### Git Signature & Committer

Ledger commits are made with `--allow-empty` flag, using the user's configured git email/name. No special signing is required, but if the user has signing configured, `git commit` respects it.

### Concurrent Sessions

If two worker sessions write progress to the same repo concurrently, commits are appended linearly to git history. Each session records its own milestones; the ledger is conflict-free.

---

## Future Enhancements

1. **Session Manifest Integration**: Store `LastProgressMilestone` SHA in AGM session manifest for fast resume.
2. **Bead Auto-Summary**: On bead close, query progress ledger to auto-populate bead summary.
3. **Progress Dashboard**: CLI tool to visualize progress ledger across multiple workers.
4. **Time-Series Analytics**: Analyze phase duration trends from ledger timestamps.

---

## Dependencies

- `golang.org/x/sync` (via exec package)—for git command execution
- Standard library: `time`, `regexp`, `strings`, `fmt`, `os`, `os/exec`

## See Also

- [Progress Package README](README.md)
- [Progress Package Architecture](ARCHITECTURE.md)
- `pkg/progress/ledger.go` — Implementation
- `pkg/progress/ledger_test.go` — Tests
