package workflow

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// registry stores registered workflows by name.
var registry = struct {
	sync.RWMutex
	workflows map[string]Workflow
}{
	workflows: make(map[string]Workflow),
}

// Register adds a workflow to the registry under the given name.
// If a workflow with the same name already exists, it is replaced.
// This function is typically called from workflow implementation init() functions.
func Register(workflow Workflow) {
	registry.Lock()
	defer registry.Unlock()
	registry.workflows[workflow.Name()] = workflow
}

// Get retrieves a workflow by name from the registry.
// Returns the workflow and true if found, or nil and false if not found.
func Get(name string) (Workflow, bool) {
	registry.RLock()
	defer registry.RUnlock()
	workflow, ok := registry.workflows[name]
	return workflow, ok
}

// List returns all registered workflows.
// Results are sorted alphabetically by workflow name.
func List() []Workflow {
	registry.RLock()
	workflows := make([]Workflow, 0, len(registry.workflows))
	for _, w := range registry.workflows {
		workflows = append(workflows, w)
	}
	registry.RUnlock()

	// Sort by name
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name() < workflows[j].Name()
	})

	return workflows
}

// ListForHarness returns workflows that support the specified harness.
// Results are sorted alphabetically by workflow name.
func ListForHarness(harnessName string) []Workflow {
	registry.RLock()
	var compatible []Workflow
	for _, w := range registry.workflows {
		for _, supported := range w.SupportedHarnesses() {
			if supported == harnessName {
				compatible = append(compatible, w)
				break
			}
		}
	}
	registry.RUnlock()

	// Sort by name
	sort.Slice(compatible, func(i, j int) bool {
		return compatible[i].Name() < compatible[j].Name()
	})

	return compatible
}

// listNames returns sorted names of all registered workflows.
func listNames() []string {
	registry.RLock()
	defer registry.RUnlock()
	available := make([]string, 0, len(registry.workflows))
	for name := range registry.workflows {
		available = append(available, name)
	}
	sort.Strings(available)
	return available
}

// ValidateCompatibility checks if a workflow is compatible with a harness.
// Returns error if the workflow doesn't support the harness.
func ValidateCompatibility(workflowName, harnessName string) error {
	workflow, ok := Get(workflowName)
	if !ok {
		return fmt.Errorf("workflow '%s' not found. Available workflows: %s",
			workflowName, strings.Join(listNames(), ", "))
	}

	// Check if harness is in supported list
	for _, supported := range workflow.SupportedHarnesses() {
		if supported == harnessName {
			return nil
		}
	}

	return fmt.Errorf("workflow '%s' not supported by harness '%s'. Supported harnesses: %s",
		workflowName, harnessName, strings.Join(workflow.SupportedHarnesses(), ", "))
}

// IsWorkflowSupported checks if a workflow exists and supports the given harness.
// Returns true if the workflow exists and supports the harness, false otherwise.
func IsWorkflowSupported(workflowName, harnessName string) bool {
	return ValidateCompatibility(workflowName, harnessName) == nil
}
