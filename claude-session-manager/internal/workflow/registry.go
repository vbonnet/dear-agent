package workflow

import (
	"fmt"
	"sort"
	"strings"
)

// registry stores registered workflows by name.
var registry = make(map[string]Workflow)

// Register adds a workflow to the registry under the given name.
// If a workflow with the same name already exists, it is replaced.
// This function is typically called from workflow implementation init() functions.
func Register(workflow Workflow) {
	registry[workflow.Name()] = workflow
}

// Get retrieves a workflow by name from the registry.
// Returns the workflow and true if found, or nil and false if not found.
func Get(name string) (Workflow, bool) {
	workflow, ok := registry[name]
	return workflow, ok
}

// List returns all registered workflows.
// Results are sorted alphabetically by workflow name.
func List() []Workflow {
	workflows := make([]Workflow, 0, len(registry))
	for _, w := range registry {
		workflows = append(workflows, w)
	}

	// Sort by name
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].Name() < workflows[j].Name()
	})

	return workflows
}

// ListForAgent returns workflows that support the specified agent.
// Results are sorted alphabetically by workflow name.
func ListForAgent(agentName string) []Workflow {
	var compatible []Workflow
	for _, w := range registry {
		for _, supported := range w.SupportedAgents() {
			if supported == agentName {
				compatible = append(compatible, w)
				break
			}
		}
	}

	// Sort by name
	sort.Slice(compatible, func(i, j int) bool {
		return compatible[i].Name() < compatible[j].Name()
	})

	return compatible
}

// ValidateCompatibility checks if a workflow is compatible with an agent.
// Returns error if the workflow doesn't support the agent.
func ValidateCompatibility(workflowName, agentName string) error {
	workflow, ok := Get(workflowName)
	if !ok {
		available := make([]string, 0, len(registry))
		for name := range registry {
			available = append(available, name)
		}
		sort.Strings(available)
		return fmt.Errorf("workflow '%s' not found. Available workflows: %s",
			workflowName, strings.Join(available, ", "))
	}

	// Check if agent is in supported list
	for _, supported := range workflow.SupportedAgents() {
		if supported == agentName {
			return nil
		}
	}

	return fmt.Errorf("workflow '%s' not supported by agent '%s'. Supported agents: %s",
		workflowName, agentName, strings.Join(workflow.SupportedAgents(), ", "))
}

// IsWorkflowSupported checks if a workflow exists and supports the given agent.
// Returns true if the workflow exists and supports the agent, false otherwise.
func IsWorkflowSupported(workflowName, agentName string) bool {
	return ValidateCompatibility(workflowName, agentName) == nil
}
