# Wayfinder V2 Schema Implementation

This document describes the V2 schema implementation for WAYFINDER-STATUS.md files.

## Overview

Wayfinder V2 consolidates the 13-phase V1 workflow into 9 streamlined phases while adding native roadmap and task tracking capabilities directly in the status file.

## Architecture

### Core Components

1. **types_v2.go** - V2 data structures
   - `StatusV2` - Main status file structure
   - `PhaseHistory` - Phase tracking with build metrics
   - `Roadmap` - Native task tracking
   - `Task` - Individual implementation tasks
   - `QualityMetrics` - Quality tracking data

2. **parser_v2.go** - YAML parser
   - `ParseV2()` - Read V2 files
   - `WriteV2()` - Write V2 files
   - `DetectSchemaVersion()` - Detect V1 vs V2
   - `NewStatusV2()` - Create new V2 status

3. **validator_v2.go** - Schema validation
   - `ValidateV2()` - Complete validation
   - `validatePhaseHistory()` - Phase consistency
   - `validateRoadmap()` - Task dependencies
   - `detectCyclicDependencies()` - Cycle detection
   - `validateQualityMetrics()` - Metrics ranges

### File Format

V2 files are pure YAML between `---` delimiters:

```yaml
---
schema_version: "2.0"
project_name: "Example Project"
project_type: "feature"
risk_level: "M"
current_phase: "S8"
status: "in-progress"
created_at: "2026-02-20T10:00:00Z"
updated_at: "2026-02-20T14:00:00Z"
phase_history:
  - name: "W0"
    status: "completed"
    started_at: "2026-02-20T10:00:00Z"
    completed_at: "2026-02-20T11:00:00Z"
roadmap:
  phases:
    - id: "S7"
      name: "Planning"
      status: "completed"
      tasks:
        - id: "task-1"
          title: "Implement feature X"
          effort_days: 1.0
          status: "completed"
quality_metrics:
  coverage_percent: 85.0
  coverage_target: 80.0
---
```

## 9-Phase Consolidation

### Phase Mapping (V1 → V2)

| V1 Phase | V2 Phase | Notes |
|----------|----------|-------|
| W0 | W0 | Intake & Waypoint |
| D1 | D1 | Discovery & Context |
| D2 | D2 | Investigation & Options |
| D3 | D3 | Architecture & Design Spec |
| D4 | D4 | Solution Requirements |
| S4 | *merged into D4* | Stakeholder approval now in D4 |
| S5 | *merged into S6* | Research now in S6 |
| S6 | S6 | Design (includes S5 research) |
| S7 | S7 | Planning & Task Breakdown |
| S8 | S8 | BUILD Loop |
| S9 | *merged into S8* | Validation now in S8 |
| S10 | *merged into S8* | Deployment now in S8 |
| S11 | S11 | Closure & Retrospective |

### Merged Phase Details

#### D4 (includes S4 Stakeholder Alignment)
- Added `stakeholder_approved: bool` field
- Added `stakeholder_notes: string` field
- Eliminates separate S4 phase

#### S6 (includes S5 Research)
- Added `research_notes: string` field
- Added `tests_feature_created: bool` field
- Implementation-specific research integrated

#### S8 (includes S9 Validation, S10 Deployment)
- Added `validation_status: string` field
- Added `deployment_status: string` field
- Added `build_iterations: int` field
- Tight BUILD → TEST → VALIDATE → DEPLOY loop

## Native Roadmap Structure

### Task Tracking

Tasks are tracked directly in the roadmap:

```yaml
roadmap:
  phases:
    - id: "S7"
      name: "Planning & Task Breakdown"
      status: "completed"
      tasks:
        - id: "task-7.1"
          title: "Break down implementation tasks"
          effort_days: 0.25
          status: "completed"
    - id: "S8"
      name: "BUILD Loop"
      status: "in-progress"
      tasks:
        - id: "task-8.1"
          title: "Implement OAuth2 authorization endpoint"
          effort_days: 0.5
          status: "completed"
          depends_on: []
          blocks: ["task-8.2"]
        - id: "task-8.2"
          title: "Implement OAuth2 token endpoint"
          effort_days: 0.625
          status: "in-progress"
          depends_on: ["task-8.1"]
```

### Dependency Validation

The validator enforces:
1. All `depends_on` references must exist
2. All `blocks` references must exist
3. No cyclic dependencies (using DFS cycle detection)
4. Unique task IDs across all phases

## Validation Rules

### Required Fields
- `schema_version` (must be "2.0")
- `project_name`
- `project_type` (feature | research | infrastructure | refactor | bugfix)
- `risk_level` (XS | S | M | L | XL)
- `current_phase` (W0 | D1 | D2 | D3 | D4 | S6 | S7 | S8 | S11)
- `status` (planning | in-progress | blocked | completed | abandoned)
- `created_at` (ISO 8601 timestamp)
- `updated_at` (ISO 8601 timestamp)

### Enum Validation
All enum fields are validated against allowed values.

### Phase History Rules
- Phase names must be valid V2 phases (W0, D1, D2, D3, D4, S6, S7, S8, S11)
- Cannot use legacy phases (S4, S5, S9, S10)
- Completed phases must have `completed_at` timestamp
- Phase-specific metadata validated (e.g., D4 needs `stakeholder_approved`)

### Roadmap Rules
- Task IDs must be unique
- `depends_on` must reference existing tasks
- No cyclic dependencies
- Task status must be valid (pending | in-progress | completed | blocked)

### Quality Metrics Rules
- Coverage percentages: 0-100
- Scores: 0-100
- Issue counts: non-negative
- Effort hours: non-negative

### Conditional Requirements
- If `status = "completed"`, `completion_date` required
- If `status = "blocked"`, `blocked_reason` recommended

## Usage Examples

### Create New V2 Status

```go
status := NewStatusV2("My Project", ProjectTypeFeature, RiskLevelM)
status.Description = "Implement new feature"
status.Tags = []string{"feature", "backend"}

err := WriteV2ToDir(status, "/path/to/project")
```

### Parse Existing V2 File

```go
status, err := ParseV2FromDir("/path/to/project")
if err != nil {
    log.Fatal(err)
}
```

### Validate V2 Status

```go
err := ValidateV2(status)
if err != nil {
    log.Printf("Validation failed: %v", err)
}
```

### Detect Schema Version

```go
version, err := DetectSchemaVersion("/path/to/WAYFINDER-STATUS.md")
if err != nil {
    log.Fatal(err)
}

if version == "2.0" {
    status, _ := ParseV2(path)
} else {
    status, _ := ReadFrom(path) // V1 parser
}
```

### Add Task to Roadmap

```go
task := Task{
    ID:         "task-8.3",
    Title:      "Implement validation",
    EffortDays: 1.5,
    Status:     TaskStatusPending,
    DependsOn:  []string{"task-8.1", "task-8.2"},
    Priority:   PriorityP0,
}

// Find phase
for i := range status.Roadmap.Phases {
    if status.Roadmap.Phases[i].ID == PhaseV2S8 {
        status.Roadmap.Phases[i].Tasks = append(
            status.Roadmap.Phases[i].Tasks,
            task,
        )
        break
    }
}

// Validate before saving
if err := ValidateV2(status); err != nil {
    log.Fatal(err)
}

err := WriteV2ToDir(status, "/path/to/project")
```

## Testing

### Run All V2 Tests

```bash
cd cortex
go test ./cmd/wayfinder-session/internal/status -v -run "V2"
```

### Test Coverage

```bash
go test ./cmd/wayfinder-session/internal/status -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Files
- `parser_v2_test.go` - Parser tests (parse, write, round-trip)
- `validator_v2_test.go` - Validation tests (enums, dependencies, cycles)
- `testdata/valid-v2.yaml` - Valid example file

## Migration from V1 to V2

### Manual Migration Steps

1. Update `schema_version` to "2.0"
2. Map phase names (S4 → D4, S5 → S6, S9/S10 → S8)
3. Add phase-specific metadata:
   - D4: `stakeholder_approved`
   - S6: `tests_feature_created`, `research_notes`
   - S8: `validation_status`, `deployment_status`
4. Create roadmap structure (if tasks exist elsewhere)
5. Validate with `ValidateV2()`

### Automated Migration (Future)

A migration tool will be implemented to automate V1 → V2 conversion:

```bash
wayfinder-session migrate-to-v2 /path/to/project
```

## Design References

- Schema specification: `wayfinder-status-v2-schema.yaml`
- Phase definitions: `phase-definitions-v2.md`

## Implementation Status

✅ Complete:
- V2 data structures (`types_v2.go`)
- YAML parser (`parser_v2.go`)
- Schema validator (`validator_v2.go`)
- Cyclic dependency detection
- Unit tests (parser, validator)
- Test fixtures

🚧 Future Work:
- CLI integration (`wayfinder-session` commands)
- V1 → V2 migration tool
- Roadmap visualization
- Task progress tracking
- Integration with bead tracking

## Success Criteria

✅ All Go files compile without errors
✅ All tests pass
✅ Can parse example V2 WAYFINDER-STATUS.md
✅ Can write valid V2 files
✅ Validation catches all error cases
✅ Cyclic dependency detection works
✅ Round-trip preserves data

## Notes

- Backward compatibility: V1 and V2 schemas coexist
- Schema detection: `DetectSchemaVersion()` auto-detects format
- Validation: Strict for required fields, warnings for recommendations
- Task dependencies: DFS-based cycle detection algorithm
- Quality metrics: All percentages 0-100, all counts non-negative
