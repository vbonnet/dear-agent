# Autonomous Swarm

**Autonomous bead execution harness** for distributed task orchestration with AGM (Agent Gateway Manager) integration.

## Overview

Autonomous Swarm is a Go-based execution system that manages and executes "beads" (autonomous tasks) using a priority queue, AGM session orchestration, and built-in telemetry. It supports iteration limits, escalation detection, and automatic roadmap generation.

**Key Features**:
- Priority-based task queue (Tiers 1-4)
- AGM session integration for autonomous execution
- Iteration limiting (max 3 per bead)
- Escalation signal detection
- JSON Lines event logging
- Automatic roadmap generation

## Architecture

The system is organized into 6 core packages:

1. **pkg/taskqueue**: Task queue management (CRUD, state transitions, YAML persistence)
2. **pkg/csm**: AGM session orchestration (tmux-based execution, session lifecycle)
3. **pkg/executor**: Execution harness (iteration tracking, escalation detection, error classification)
4. **pkg/validation**: S8 file validation, S9 test execution
5. **pkg/telemetry**: Event logging (JSON Lines), roadmap generation
6. **cmd/swarm-executor**: CLI entry point (flag parsing, orchestration)

## Installation

### Prerequisites
- Go 1.25.1+
- Claude Code CLI with AGM support
- tmux (for session management)

### Build
```bash
git clone <repository-url>
cd autonomous-swarm
go build -o swarm-executor ./cmd/swarm-executor
```

## Usage

### Basic Execution
```bash
# Execute a single bead
./swarm-executor \
  --queue ./TASK-QUEUE.yaml \
  --bead-id bead-example-1 \
  --session my-session
```

### Flags
- `--queue <path>`: Path to TASK-QUEUE.yaml file (required)
- `--bead-id <id>`: Bead ID to execute (required)
- `--session <name>`: AGM session name (required)
- `--version`: Show version and exit
- `--help`: Show help and exit

### Exit Codes
- `0`: Success - bead executed successfully
- `1`: Error - execution failed (see stderr for details)
- `2`: Escalation - bead requires human intervention

## Task Queue Format

TASK-QUEUE.yaml example:
```yaml
schema_version: "1.0.0"
last_updated: 2024-01-01T00:00:00Z
ready:
  - id: bead-1
    title: First bead
    tier: 1
    prompts:
      start: "Do task 1"
in_progress: []
blocked: []
completed: []
```

**Tier Priority**:
- Tier 1: Critical (highest priority)
- Tier 2: Important
- Tier 3: Normal
- Tier 4: Nice-to-have (lowest priority)

## Output Files

### EXECUTION-LOG.jsonl
Append-only event log in JSON Lines format. Each line is a JSON object:

```json
{"timestamp":"2024-01-01T12:00:00Z","bead_id":"bead-1","event":"execute","details":{"session":"my-session","action":"start"}}
{"timestamp":"2024-01-01T12:05:00Z","bead_id":"bead-1","event":"complete","details":{"session":"my-session","status":"success"}}
```

**Event Types**:
- `execute`: Bead execution started
- `complete`: Bead completed successfully
- `error`: Execution error occurred
- `escalate`: Escalation signal detected

### ROADMAP.md
Human-readable roadmap generated after each execution (max 1500 tokens):

```markdown
# Task Roadmap

**Last Updated**: 2024-01-01 12:05:00

## Ready
**Count**: 5 beads ready for execution

## In Progress
- **bead-2**: Second bead (session: my-session, iteration: 1)

## Blocked
*No blocked beads*

## Completed
**Count**: 1 beads completed

---
**Progress**: 1/6 (17%) completed
```

## Iteration and Escalation

### Iteration Limits
- Each bead can be retried up to **3 times**
- Retries occur on recoverable errors (AGM timeout, parse errors)
- After 3 iterations, bead is escalated

### Escalation Signals
Beads can explicitly request escalation by including in their output:
```
ESCALATE: <reason>
```

Example:
```
ESCALATE: Requires human decision on API version choice
```

When escalated:
- Bead moves to `blocked` section in queue
- Exit code 2 returned
- Escalation reason logged

## Error Handling

### Error Types
1. **Recoverable** (retry): AGM timeout, parse errors
2. **Escalation** (requires human): Max iterations, explicit ESCALATE signal
3. **Fatal** (stop): File not found, invalid configuration

### Error Flow
```
Execute Bead
  ↓
Recoverable error?
  ├─ Yes → Increment iteration → Retry (max 3)
  └─ No → Escalation/Fatal → Exit with code 1 or 2
```

## Troubleshooting

### "Error: Failed to load task queue"
- Verify `--queue` path is correct
- Check TASK-QUEUE.yaml syntax (use `yq` or YAML validator)
- Ensure file exists and is readable

### "Error: Execution failed: bead not found in ready section"
- Verify `--bead-id` matches a bead in `ready:` section
- Check bead hasn't already been claimed (moved to `in_progress:`)

### Exit code 2 (Escalation)
- Check EXECUTION-LOG.jsonl for escalation reason
- Review ROADMAP.md for bead status
- Beads in `blocked:` section require manual intervention

### AGM session errors
- Ensure Claude Code CLI is installed
- Verify tmux is running
- Check session name doesn't conflict with existing sessions

## Development

### Running Tests
```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# With race detector
go test -race ./...
```

### Linting
```bash
# Format code
gofmt -w .

# Run linter
golangci-lint run ./...

# Static analysis
go vet ./...
```

### Package Coverage
- pkg/taskqueue: 91.4%
- pkg/csm: 67.5%
- pkg/executor: 53.8%
- pkg/validation: 91.7%
- pkg/telemetry: 90.6%
- cmd/swarm-executor: 1.9% (thin orchestration layer)

## Development Process

This library was built using the Wayfinder methodology with comprehensive planning,
multi-persona reviews, and validation phases. See the project retrospective for
detailed development insights.

## License

Apache 2.0

## Version

v0.1.0
