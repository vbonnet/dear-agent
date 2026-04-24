package taskmanager

import (
	"fmt"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// DependencyValidator validates task dependencies and detects cycles
type DependencyValidator struct {
	status *status.StatusV2
	graph  map[string][]string // taskID -> list of dependencies
}

// NewDependencyValidator creates a new dependency validator
func NewDependencyValidator(st *status.StatusV2) *DependencyValidator {
	v := &DependencyValidator{
		status: st,
		graph:  make(map[string][]string),
	}
	v.buildGraph()
	return v
}

// buildGraph constructs the dependency graph from all tasks
func (v *DependencyValidator) buildGraph() {
	if v.status.Roadmap == nil {
		return
	}

	for _, phase := range v.status.Roadmap.Phases {
		for _, task := range phase.Tasks {
			v.graph[task.ID] = task.DependsOn
		}
	}
}

// ValidateTask validates a task's dependencies and checks for cycles
func (v *DependencyValidator) ValidateTask(task *status.Task) error {
	// Validate all dependencies exist
	for _, dep := range task.DependsOn {
		if _, exists := v.graph[dep]; !exists {
			// Check if this is a new task not yet in graph
			if task.ID == "" || dep != task.ID {
				return fmt.Errorf("dependency task not found: %s", dep)
			}
		}
	}

	// Add/update task in graph for cycle detection
	oldDeps := v.graph[task.ID]
	v.graph[task.ID] = task.DependsOn

	// Check for cycles
	if err := v.detectCycles(); err != nil {
		// Restore old dependencies if validation fails
		if len(oldDeps) > 0 || task.ID != "" {
			v.graph[task.ID] = oldDeps
		} else {
			delete(v.graph, task.ID)
		}
		return err
	}

	return nil
}

// detectCycles uses DFS to detect cycles in the dependency graph
func (v *DependencyValidator) detectCycles() error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for taskID := range v.graph {
		if !visited[taskID] {
			if path := v.dfsCycleDetect(taskID, visited, recStack, []string{}); path != nil {
				return fmt.Errorf("circular dependency detected: %v", path)
			}
		}
	}

	return nil
}

// dfsCycleDetect performs depth-first search to detect cycles
// Returns the cycle path if found, nil otherwise
func (v *DependencyValidator) dfsCycleDetect(taskID string, visited, recStack map[string]bool, path []string) []string {
	visited[taskID] = true
	recStack[taskID] = true
	path = append(path, taskID)

	// Visit all dependencies
	for _, dep := range v.graph[taskID] {
		if !visited[dep] {
			if cyclePath := v.dfsCycleDetect(dep, visited, recStack, path); cyclePath != nil {
				return cyclePath
			}
		} else if recStack[dep] {
			// Found a cycle - return the path from dep to current task
			cycleStart := -1
			for i, id := range path {
				if id == dep {
					cycleStart = i
					break
				}
			}
			if cycleStart >= 0 {
				return append(path[cycleStart:], dep)
			}
			return append(path, dep)
		}
	}

	recStack[taskID] = false
	return nil
}

// ValidateAll validates all tasks in the status file
func (v *DependencyValidator) ValidateAll() error {
	// First, validate all dependencies exist
	for taskID, deps := range v.graph {
		for _, dep := range deps {
			if _, exists := v.graph[dep]; !exists {
				return fmt.Errorf("task %s has invalid dependency: %s (task not found)", taskID, dep)
			}
		}
	}

	// Then check for cycles
	return v.detectCycles()
}

// GetDependencyChain returns all tasks that the given task depends on (transitively)
func (v *DependencyValidator) GetDependencyChain(taskID string) ([]string, error) {
	visited := make(map[string]bool)
	var chain []string

	if err := v.collectDependencies(taskID, visited, &chain); err != nil {
		return nil, err
	}

	return chain, nil
}

// collectDependencies recursively collects all dependencies
func (v *DependencyValidator) collectDependencies(taskID string, visited map[string]bool, chain *[]string) error {
	if visited[taskID] {
		return nil
	}

	visited[taskID] = true

	deps, exists := v.graph[taskID]
	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	for _, dep := range deps {
		*chain = append(*chain, dep)
		if err := v.collectDependencies(dep, visited, chain); err != nil {
			return err
		}
	}

	return nil
}

// GetBlockedBy returns all tasks that block the given task (direct dependencies)
func (v *DependencyValidator) GetBlockedBy(taskID string) []string {
	if deps, exists := v.graph[taskID]; exists {
		result := make([]string, len(deps))
		copy(result, deps)
		return result
	}
	return []string{}
}

// GetBlocks returns all tasks that the given task blocks (tasks that depend on it)
func (v *DependencyValidator) GetBlocks(taskID string) []string {
	var blocks []string
	for id, deps := range v.graph {
		for _, dep := range deps {
			if dep == taskID {
				blocks = append(blocks, id)
				break
			}
		}
	}
	return blocks
}
