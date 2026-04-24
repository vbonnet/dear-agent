# Wayfinder V2 Quick Start Guide

This guide shows you how to use the V2 schema implementation.

## Import

```go
import "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/status"
```

## Create a New V2 Status

```go
// Create with minimal required fields
st := status.NewStatusV2(
    "My Awesome Project",
    status.ProjectTypeFeature,
    status.RiskLevelM,
)

// Add optional fields
st.Description = "Building an awesome feature"
st.Repository = "https://github.com/user/repo"
st.Branch = "feature/awesome"
st.Tags = []string{"backend", "api"}
```

## Add Phase History

```go
now := time.Now()
completed := now.Add(-1 * time.Hour)

st.PhaseHistory = append(st.PhaseHistory, status.PhaseHistory{
    Name:        status.PhaseV2W0,
    Status:      status.PhaseStatusV2Completed,
    StartedAt:   now.Add(-2 * time.Hour),
    CompletedAt: &completed,
    Deliverables: []string{"W0-intake.md"},
})
```

## Add Roadmap with Tasks

```go
st.Roadmap = &status.Roadmap{
    Phases: []status.RoadmapPhase{
        {
            ID:     status.PhaseV2S8,
            Name:   "BUILD Loop",
            Status: status.PhaseStatusV2InProgress,
            Tasks: []status.Task{
                {
                    ID:         "task-8.1",
                    Title:      "Implement core feature",
                    EffortDays: 2.0,
                    Status:     status.TaskStatusCompleted,
                    Priority:   status.PriorityP0,
                    Deliverables: []string{"src/core.go"},
                },
                {
                    ID:         "task-8.2",
                    Title:      "Add validation",
                    EffortDays: 1.5,
                    Status:     status.TaskStatusInProgress,
                    Priority:   status.PriorityP0,
                    DependsOn:  []string{"task-8.1"},
                },
            },
        },
    },
}
```

## Add Quality Metrics

```go
st.QualityMetrics = &status.QualityMetrics{
    CoveragePercent:  85.5,
    CoverageTarget:   80.0,
    AssertionDensity: 3.5,
    P0Issues:         0,
    P1Issues:         2,
}
```

## Validate

```go
if err := status.ValidateV2(st); err != nil {
    log.Fatalf("Validation failed: %v", err)
}
```

## Write to File

```go
// Write to specific file
err := status.WriteV2(st, "/path/to/WAYFINDER-STATUS.md")

// Or write to directory (uses WAYFINDER-STATUS.md filename)
err := status.WriteV2ToDir(st, "/path/to/project")
```

## Read from File

```go
// Read from specific file
st, err := status.ParseV2("/path/to/WAYFINDER-STATUS.md")

// Or read from directory
st, err := status.ParseV2FromDir("/path/to/project")
```

## Detect Schema Version

```go
version, err := status.DetectSchemaVersion("/path/to/WAYFINDER-STATUS.md")
if err != nil {
    log.Fatal(err)
}

if version == "2.0" {
    st, _ := status.ParseV2(path)
    // Use V2 API
} else {
    st, _ := status.ReadFrom(path)
    // Use V1 API
}
```

## Complete Example

```go
package main

import (
    "log"
    "time"

    "github.com/vbonnet/engram/core/cortex/cmd/wayfinder-session/internal/status"
)

func main() {
    // Create new V2 status
    st := status.NewStatusV2(
        "OAuth2 Authentication",
        status.ProjectTypeFeature,
        status.RiskLevelL,
    )
    st.Description = "Implement OAuth2 for API endpoints"

    // Add completed phase
    now := time.Now()
    completed := now.Add(-24 * time.Hour)
    st.PhaseHistory = []status.PhaseHistory{
        {
            Name:        status.PhaseV2W0,
            Status:      status.PhaseStatusV2Completed,
            StartedAt:   now.Add(-48 * time.Hour),
            CompletedAt: &completed,
        },
    }

    // Add roadmap
    st.Roadmap = &status.Roadmap{
        Phases: []status.RoadmapPhase{
            {
                ID:     status.PhaseV2S8,
                Name:   "BUILD Loop",
                Status: status.PhaseStatusV2InProgress,
                Tasks: []status.Task{
                    {
                        ID:         "task-8.1",
                        Title:      "OAuth2 authorization endpoint",
                        EffortDays: 0.5,
                        Status:     status.TaskStatusCompleted,
                    },
                    {
                        ID:         "task-8.2",
                        Title:      "OAuth2 token endpoint",
                        EffortDays: 0.625,
                        Status:     status.TaskStatusInProgress,
                        DependsOn:  []string{"task-8.1"},
                    },
                },
            },
        },
    }

    // Validate
    if err := status.ValidateV2(st); err != nil {
        log.Fatalf("Validation failed: %v", err)
    }

    // Write to file
    if err := status.WriteV2ToDir(st, "."); err != nil {
        log.Fatalf("Failed to write: %v", err)
    }

    log.Println("Successfully created WAYFINDER-STATUS.md")
}
```

## Common Patterns

### Add a Task to Existing Roadmap

```go
// Find the phase
for i := range st.Roadmap.Phases {
    if st.Roadmap.Phases[i].ID == status.PhaseV2S8 {
        // Add new task
        newTask := status.Task{
            ID:         "task-8.3",
            Title:      "Integration tests",
            EffortDays: 1.0,
            Status:     status.TaskStatusPending,
            DependsOn:  []string{"task-8.2"},
        }
        st.Roadmap.Phases[i].Tasks = append(
            st.Roadmap.Phases[i].Tasks,
            newTask,
        )
        break
    }
}

// Validate dependencies
if err := status.ValidateV2(st); err != nil {
    log.Printf("Invalid: %v", err)
}
```

### Mark Task as Completed

```go
// Find and update task
for i := range st.Roadmap.Phases {
    for j := range st.Roadmap.Phases[i].Tasks {
        if st.Roadmap.Phases[i].Tasks[j].ID == "task-8.2" {
            now := time.Now()
            st.Roadmap.Phases[i].Tasks[j].Status = status.TaskStatusCompleted
            st.Roadmap.Phases[i].Tasks[j].CompletedAt = &now
            break
        }
    }
}

// Update timestamp
st.UpdatedAt = time.Now()
```

### Add Phase-Specific Metadata

```go
// D4 phase with stakeholder approval
approved := true
st.PhaseHistory = append(st.PhaseHistory, status.PhaseHistory{
    Name:                status.PhaseV2D4,
    Status:              status.PhaseStatusV2Completed,
    StartedAt:           time.Now().Add(-4 * time.Hour),
    CompletedAt:         &completed,
    StakeholderApproved: &approved,
    StakeholderNotes:    "Approved by security team",
})

// S6 phase with research notes
testsCreated := true
st.PhaseHistory = append(st.PhaseHistory, status.PhaseHistory{
    Name:                status.PhaseV2S6,
    Status:              status.PhaseStatusV2Completed,
    StartedAt:           time.Now().Add(-2 * time.Hour),
    CompletedAt:         &completed,
    TestsFeatureCreated: &testsCreated,
    ResearchNotes:       "Selected Auth0 library",
})

// S8 phase with validation/deployment status
st.PhaseHistory = append(st.PhaseHistory, status.PhaseHistory{
    Name:             status.PhaseV2S8,
    Status:           status.PhaseStatusV2InProgress,
    StartedAt:        time.Now().Add(-1 * time.Hour),
    ValidationStatus: status.ValidationStatusInProgress,
    DeploymentStatus: status.DeploymentStatusPending,
    BuildIterations:  3,
})
```

## Error Handling

### Validation Errors

```go
err := status.ValidateV2(st)
if err != nil {
    // Validation returns multi-line error with all issues
    log.Printf("Validation failed:\n%v", err)

    // Example output:
    // validation failed:
    //   - invalid project_type 'invalid', must be one of: feature, research, ...
    //   - task 'task-2': depends_on references non-existent task 'task-1'
    //   - cyclic dependency detected: task-a -> task-b -> task-a
}
```

### Parse Errors

```go
st, err := status.ParseV2(path)
if err != nil {
    // Check error type
    if strings.Contains(err.Error(), "failed to read file") {
        log.Printf("File not found: %v", err)
    } else if strings.Contains(err.Error(), "failed to parse YAML") {
        log.Printf("Invalid YAML: %v", err)
    } else if strings.Contains(err.Error(), "invalid frontmatter") {
        log.Printf("Missing --- delimiters: %v", err)
    }
}
```

## Testing

### Unit Test Example

```go
func TestMyFeature(t *testing.T) {
    // Create test status
    st := status.NewStatusV2("Test", status.ProjectTypeFeature, status.RiskLevelS)

    // Add test data
    st.Roadmap = &status.Roadmap{
        Phases: []status.RoadmapPhase{
            {
                ID:     status.PhaseV2S7,
                Name:   "Planning",
                Status: status.PhaseStatusV2Completed,
                Tasks: []status.Task{
                    {ID: "task-1", Title: "Test", Status: status.TaskStatusCompleted},
                },
            },
        },
    }

    // Validate
    if err := status.ValidateV2(st); err != nil {
        t.Fatalf("Validation failed: %v", err)
    }

    // Test your feature...
}
```

## More Information

- Full implementation docs: `V2_IMPLEMENTATION.md`
- Implementation summary: `IMPLEMENTATION_SUMMARY.md`
- Test fixtures: `testdata/`
- Design spec: `wayfinder-status-v2-schema.yaml`
