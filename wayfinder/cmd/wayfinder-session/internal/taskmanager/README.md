# Task Manager Package

Task management system for Wayfinder V2 with dependency validation and CLI integration.

## Quick Start

```bash
# Add a task to S8 phase
wayfinder session task add S8 "Implement OAuth2 endpoint" --effort 4 --priority P0

# Update task status
wayfinder session task update S8-1 --status in-progress

# List all tasks in S8
wayfinder session task list --phase S8

# Show detailed task info
wayfinder session task show S8-1

# Delete a task
wayfinder session task delete S8-2
```

## Features

- **Task CRUD Operations**: Create, read, update, delete tasks
- **Dependency Management**: Define task dependencies with cycle detection
- **Atomic Updates**: All operations update WAYFINDER-STATUS.md atomically
- **Rich Metadata**: Track effort, priority, deliverables, acceptance criteria
- **Cross-Phase Dependencies**: Tasks can depend on tasks from other phases
- **Comprehensive Validation**: Prevents invalid states and circular dependencies

## Files

### Core Implementation
- `taskmanager.go` - Task CRUD operations and file I/O
- `dependency_validator.go` - Cycle detection and dependency graph operations
- `types.go` - Type definitions for options and filters

### Tests (88.1% coverage)
- `taskmanager_test.go` - Unit tests for TaskManager
- `dependency_validator_test.go` - Tests for cycle detection
- `cli_integration_test.go` - End-to-end workflow tests

### Documentation
- `task-cli-implementation.md` - Complete implementation guide
- `README.md` - This file

## CLI Commands

### task add
Add a new task to a phase with optional metadata.

**Usage:**
```bash
wayfinder session task add <phase-id> "<title>" [flags]
```

**Flags:**
- `--effort <days>` - Estimated effort in days
- `--priority <P0|P1|P2>` - Priority level
- `--depends-on <ids>` - Comma-separated task IDs
- `--deliverables <files>` - Comma-separated file paths
- `--acceptance-criteria <criteria>` - Comma-separated criteria
- `--description <text>` - Detailed description
- `--assigned-to <name>` - Assignee name
- `--bead-id <id>` - Associated bead ID
- `--notes <text>` - Additional notes

### task update
Update an existing task's status or metadata.

**Usage:**
```bash
wayfinder session task update <task-id> [flags]
```

**Flags:**
- `--status <status>` - pending | in-progress | completed | blocked
- `--tests-status <status>` - passed | failed | pending
- `--priority <P0|P1|P2>` - Update priority
- `--effort <days>` - Update effort estimate
- All flags from `task add` are also supported

### task list
List tasks with optional filtering.

**Usage:**
```bash
wayfinder session task list [flags]
```

**Flags:**
- `--phase <phase-id>` - Filter by phase (W0, D1, D2, D3, D4, S6, S7, S8, S11)
- `--status <status>` - Filter by status (pending, in-progress, completed, blocked)

### task show
Display detailed information about a task.

**Usage:**
```bash
wayfinder session task show <task-id>
```

### task delete
Delete a task from the roadmap.

**Usage:**
```bash
wayfinder session task delete <task-id>
```

**Note:** Fails if other tasks depend on this task.

## API Usage

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/taskmanager"

// Create task manager
tm := taskmanager.New("/path/to/project")

// Add task
task, err := tm.AddTask("S8", "Implement feature", &taskmanager.TaskOptions{
    EffortDays: 4.0,
    Priority:   status.PriorityP0,
    DependsOn:  []string{"S7-1"},
    Deliverables: []string{"src/feature.go", "src/feature_test.go"},
})

// Update task
updated, err := tm.UpdateTask(task.ID, &taskmanager.UpdateOptions{
    Status: status.TaskStatusInProgress,
})

// List tasks
tasks, err := tm.ListTasks(&taskmanager.TaskFilter{
    PhaseID: "S8",
    Status:  status.TaskStatusPending,
})

// Get task details
task, err := tm.GetTask("S8-1")

// Delete task
err = tm.DeleteTask("S8-2")
```

## Dependency Validation

The package uses depth-first search (DFS) to detect cycles in the dependency graph:

```go
validator := taskmanager.NewDependencyValidator(status)

// Validate a task
err := validator.ValidateTask(task)

// Get dependency chain (transitive)
chain, err := validator.GetDependencyChain("S8-4")

// Get direct dependencies
blockedBy := validator.GetBlockedBy("S8-2")

// Get tasks that depend on this one
blocks := validator.GetBlocks("S8-1")
```

## Testing

```bash
# Run all tests
go test ./cmd/wayfinder-session/internal/taskmanager/...

# Run with coverage
go test -cover ./cmd/wayfinder-session/internal/taskmanager/...

# Run specific test
go test -run TestCLIWorkflow ./cmd/wayfinder-session/internal/taskmanager/...

# Verbose output
go test -v ./cmd/wayfinder-session/internal/taskmanager/...
```

## Error Handling

Common errors:

| Error | Solution |
|-------|----------|
| `invalid phase ID: X99` | Use W0, D1, D2, D3, D4, S6, S7, S8, or S11 |
| `dependency task not found: S8-99` | Create dependency first |
| `circular dependency detected` | Remove cycle in dependency graph |
| `task not found: INVALID` | Check task ID with `task list` |
| `it is referenced by [S8-2]` | Delete/update dependent tasks first |

## Performance

- File I/O: ~10ms per operation (SSD)
- Validation: O(V+E) for cycle detection
- Typical overhead: <50ms for <100 tasks

## Integration

The task commands are registered with the wayfinder session command in:
- `cortex/cmd/wayfinder/cmd/session.go`

CLI handlers are in:
- `cortex/cmd/wayfinder-session/commands/task*.go`

## See Also

- [task-cli-implementation.md](./task-cli-implementation.md) - Complete implementation guide
- [StatusV2 Schema](wayfinder-status-v2-schema.yaml)
- [StatusV2 Types](../status/types_v2.go)
