# Task Management CLI Implementation

This document describes the Task Management CLI implementation for Wayfinder V2.

## Overview

The Task Management CLI provides comprehensive task tracking capabilities directly within WAYFINDER-STATUS.md files. It supports:

- Creating tasks with metadata, dependencies, and acceptance criteria
- Updating task status and properties
- Listing and filtering tasks
- Dependency validation with cycle detection
- Atomic file updates

## Architecture

### Package Structure

```
cmd/wayfinder-session/internal/taskmanager/
├── taskmanager.go              # Core task management logic
├── dependency_validator.go     # Cycle detection & validation
├── types.go                    # Type definitions
├── taskmanager_test.go         # Unit tests
├── dependency_validator_test.go # Validator tests
└── cli_integration_test.go     # Integration tests

cmd/wayfinder-session/commands/
├── task.go          # Task command group
├── task_add.go      # Add task command
├── task_update.go   # Update task command
├── task_list.go     # List tasks command
├── task_show.go     # Show task details command
└── task_delete.go   # Delete task command
```

### Core Components

#### TaskManager

The `TaskManager` handles all task operations on V2 status files:

- **AddTask**: Creates new tasks with validation
- **UpdateTask**: Updates existing tasks atomically
- **GetTask**: Retrieves task by ID
- **ListTasks**: Lists tasks with optional filtering
- **DeleteTask**: Removes tasks (with dependency checks)

#### DependencyValidator

Validates task dependencies using graph algorithms:

- **ValidateTask**: Validates a single task's dependencies
- **DetectCycles**: Uses DFS to find circular dependencies
- **GetDependencyChain**: Returns all transitive dependencies
- **GetBlockedBy**: Returns direct dependencies
- **GetBlocks**: Returns tasks that depend on a given task

## CLI Commands

### wayfinder session task add

Add a new task to a phase.

```bash
wayfinder session task add <phase-id> "<title>" [flags]
```

**Flags:**
- `--effort <days>`: Estimated effort in days
- `--description <text>`: Detailed task description
- `--priority <P0|P1|P2>`: Priority level
- `--depends-on <task-id>,...`: Task IDs this task depends on
- `--deliverables <file>,...`: Files/artifacts to be created
- `--acceptance-criteria <criterion>,...`: Completion criteria
- `--assigned-to <name>`: Agent or developer name
- `--bead-id <id>`: Associated bead ID
- `--notes <text>`: Task-specific notes

**Examples:**

```bash
# Basic task
wayfinder session task add S8 "Implement OAuth2 authorization endpoint"

# Task with effort and priority
wayfinder session task add S8 "Create token endpoint" --effort 5 --priority P0

# Task with dependencies
wayfinder session task add S8 "Integration tests" --depends-on S8-1,S8-2 --effort 4

# Full task with all metadata
wayfinder session task add S8 "Implement token validation" \
  --effort 3 \
  --priority P0 \
  --depends-on S8-1 \
  --deliverables "src/middleware/auth.go,src/middleware/auth_test.go" \
  --acceptance-criteria "Valid JWT tokens allow access,Expired tokens return 401" \
  --assigned-to "claude" \
  --notes "Use RS256 algorithm for JWT validation"
```

### wayfinder session task update

Update an existing task's status or metadata.

```bash
wayfinder session task update <task-id> [flags]
```

**Flags:**
- `--status <status>`: pending | in-progress | completed | blocked
- `--title <text>`: Update task title
- `--description <text>`: Update description
- `--effort <days>`: Update estimated effort
- `--priority <P0|P1|P2>`: Update priority
- `--tests-status <status>`: passed | failed | pending
- `--assigned-to <name>`: Update assignee
- `--notes <text>`: Update notes
- `--bead-id <id>`: Update bead ID
- `--deliverables <file>,...`: Update deliverables
- `--acceptance-criteria <criterion>,...`: Update criteria
- `--depends-on <task-id>,...`: Update dependencies

**Examples:**

```bash
# Mark task as in-progress
wayfinder session task update S8-1 --status in-progress

# Complete task with test status
wayfinder session task update S8-1 --status completed --tests-status passed

# Update priority and effort
wayfinder session task update S8-2 --priority P0 --effort 6

# Update dependencies (validates no cycles)
wayfinder session task update S8-3 --depends-on S8-1,S8-2
```

### wayfinder session task list

List tasks with optional filtering.

```bash
wayfinder session task list [flags]
```

**Flags:**
- `--phase <phase-id>`: Filter by phase (W0, D1, D2, D3, D4, S6, S7, S8, S11)
- `--status <status>`: Filter by status (pending, in-progress, completed, blocked)

**Examples:**

```bash
# List all tasks
wayfinder session task list

# List tasks in S8 phase
wayfinder session task list --phase S8

# List pending tasks
wayfinder session task list --status pending

# List in-progress tasks in S8
wayfinder session task list --phase S8 --status in-progress
```

**Output:**

```
Tasks:
====================================================================================================
ID         Phase    Status       Priority Title
----------------------------------------------------------------------------------------------------
S8-1       S8       completed    P0       Implement OAuth2 authorization endpoint
           └─ Depends on: []
S8-2       S8       in-progress  P0       Implement OAuth2 token endpoint
           └─ Depends on: [S8-1]
S8-3       S8       pending      P0       Implement token validation middleware
           └─ Depends on: [S8-1]
====================================================================================================
Total: 3 tasks
```

### wayfinder session task show

Show detailed information about a task.

```bash
wayfinder session task show <task-id>
```

**Examples:**

```bash
wayfinder session task show S8-1
```

**Output:**

```
Task: S8-1
================================================================================

Title:       Implement OAuth2 authorization endpoint
Status:      completed

Description:
Create /oauth/authorize endpoint with PKCE support

Metadata:
  Effort:         4.0 days
  Priority:       P0
  Assigned to:    claude
  Bead ID:        oss-abc123
  Tests status:   passed

Dependencies:
  (none)

Deliverables:
  - src/oauth/authorize.go
  - src/oauth/authorize_test.go

Acceptance Criteria:
  - Endpoint returns authorization code on success
  - PKCE challenge validated correctly
  - Invalid client_id returns 401

Started at:  2026-02-20 11:00:00
Completed at: 2026-02-20 13:00:00
```

### wayfinder session task delete

Delete a task from the roadmap.

```bash
wayfinder session task delete <task-id>
```

**Examples:**

```bash
wayfinder session task delete S8-3
```

**Notes:**
- Deletion fails if other tasks depend on the target task
- Use `task update` to remove dependencies first if needed

## Dependency Management

### Validation Rules

1. **Existence**: All referenced task IDs must exist
2. **Acyclic**: Dependency graph must be acyclic (no circular dependencies)
3. **Cross-phase**: Dependencies can span phases (e.g., S8 task can depend on S7 task)

### Cycle Detection

The dependency validator uses depth-first search (DFS) to detect cycles:

```go
// Example: Cycle detection prevents this
wayfinder session task add S8 "Task 1" --depends-on S8-2
wayfinder session task add S8 "Task 2" --depends-on S8-1
// Error: circular dependency detected: [S8-1 S8-2 S8-1]
```

### Dependency Graph Operations

```go
validator := NewDependencyValidator(status)

// Get all tasks a task depends on (transitive)
chain, err := validator.GetDependencyChain("S8-4")
// Returns: [S8-1, S8-2, S8-3]

// Get direct dependencies (tasks that block this one)
blockedBy := validator.GetBlockedBy("S8-2")
// Returns: [S8-1]

// Get tasks that depend on this one
blocks := validator.GetBlocks("S8-1")
// Returns: [S8-2, S8-3]
```

## File Operations

### Atomic Updates

All task operations update WAYFINDER-STATUS.md atomically:

1. Read current status file
2. Perform operation (add/update/delete)
3. Validate changes
4. Update `updated_at` timestamp
5. Write complete file atomically

This ensures consistency even with concurrent operations.

### YAML Structure

Tasks are stored in the `roadmap.phases[].tasks[]` structure:

```yaml
roadmap:
  phases:
    - id: S8
      name: BUILD Loop
      status: in-progress
      tasks:
        - id: S8-1
          title: Implement OAuth2 authorization endpoint
          status: completed
          effort_days: 4.0
          priority: P0
          depends_on: []
          deliverables:
            - src/oauth/authorize.go
            - src/oauth/authorize_test.go
          acceptance_criteria:
            - Endpoint returns authorization code on success
            - PKCE challenge validated correctly
          started_at: 2026-02-20T11:00:00Z
          completed_at: 2026-02-20T13:00:00Z
```

## Testing

### Test Coverage

- **taskmanager_test.go**: 17 unit tests
  - Basic operations (add, update, get, list, delete)
  - Task ID generation
  - Dependency validation
  - Error handling
  - Atomic file updates

- **dependency_validator_test.go**: 8 test suites
  - Cycle detection (self, two-task, three-task, complex)
  - Dependency chain traversal
  - Blocked-by relationships
  - Blocks relationships
  - Validation of all tasks

- **cli_integration_test.go**: 5 integration tests
  - Complete CLI workflow
  - Cross-phase dependencies
  - Atomic file updates
  - Error handling
  - Multi-phase operations

### Running Tests

```bash
# Run all taskmanager tests
go test ./cmd/wayfinder-session/internal/taskmanager/...

# Run with verbose output
go test -v ./cmd/wayfinder-session/internal/taskmanager/...

# Run specific test
go test -run TestCLIWorkflow ./cmd/wayfinder-session/internal/taskmanager/...
```

## API Usage

### Programmatic Access

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/taskmanager"

// Create task manager
tm := taskmanager.New("/path/to/project")

// Add task
task, err := tm.AddTask("S8", "Implement feature", &taskmanager.TaskOptions{
    EffortDays: 4.0,
    Priority:   status.PriorityP0,
    DependsOn:  []string{"S7-1"},
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

// Delete task
err = tm.DeleteTask(task.ID)
```

## Error Handling

Common errors and solutions:

| Error | Cause | Solution |
|-------|-------|----------|
| `invalid phase ID: X99` | Invalid phase identifier | Use W0, D1, D2, D3, D4, S6, S7, S8, or S11 |
| `invalid priority: PX` | Invalid priority level | Use P0, P1, or P2 |
| `dependency task not found: S8-99` | Referenced task doesn't exist | Create dependency first or fix task ID |
| `circular dependency detected: [S8-1 S8-2 S8-1]` | Cycle in dependency graph | Remove circular dependency |
| `task not found: INVALID` | Task ID doesn't exist | Check task ID with `task list` |
| `it is referenced by [S8-2]` | Cannot delete task with dependents | Update/delete dependent tasks first |
| `invalid status: bad-status` | Invalid status value | Use pending, in-progress, completed, or blocked |

## Best Practices

### Task Creation

1. **Start with high-level tasks** in S7 (Planning phase)
2. **Break down into detailed tasks** in S8 (BUILD phase)
3. **Use dependencies** to enforce ordering
4. **Set realistic effort** estimates in days
5. **Define acceptance criteria** upfront

### Dependency Management

1. **Keep dependencies simple**: Avoid complex webs
2. **Use cross-phase dependencies** sparingly
3. **Complete blocking tasks first**: Follow dependency order
4. **Validate early**: Add dependencies when creating tasks

### Status Updates

1. **Update status regularly**: Keep roadmap current
2. **Set tests_status**: Document test results
3. **Add notes**: Capture important context
4. **Track timestamps**: Status changes set started_at/completed_at

### Workflow Integration

1. **Link to beads**: Use `--bead-id` to connect with issue tracking
2. **Document deliverables**: List actual files created
3. **Track acceptance criteria**: Verify completion
4. **Use priorities**: Focus on P0 tasks first

## Performance

- **File I/O**: ~10ms per operation (SSD)
- **Validation**: O(V+E) for cycle detection (V=tasks, E=dependencies)
- **Typical overhead**: <50ms for operations on files with <100 tasks

## Future Enhancements

Potential improvements:

1. **Task templates**: Predefined task structures for common patterns
2. **Bulk operations**: Add/update multiple tasks at once
3. **Task scheduling**: Gantt chart generation from dependencies
4. **Progress tracking**: Burndown charts, velocity metrics
5. **Export formats**: JSON, CSV, Markdown table output
6. **Dependency visualization**: Graphviz DOT output
7. **Task assignment**: Multi-agent collaboration tracking
8. **Time tracking**: Actual vs estimated effort analysis

## References

- Schema: `wayfinder-status-v2-schema.yaml`
- StatusV2 Types: `cmd/wayfinder-session/internal/status/types_v2.go`
- Parser: `cmd/wayfinder-session/internal/status/parser_v2.go`
- Validator: `cmd/wayfinder-session/internal/status/validator_v2.go`

## Support

For issues or questions:
- Check error messages for specific guidance
- Review test files for usage examples
- Consult schema documentation for field definitions
