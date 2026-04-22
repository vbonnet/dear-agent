package workflow

import (
	"fmt"
	"sync"
	"testing"
)

type mockWorkflow struct {
	name      string
	harnesses []string
}

func (m *mockWorkflow) Name() string                 { return m.name }
func (m *mockWorkflow) Description() string          { return "mock" }
func (m *mockWorkflow) SupportedHarnesses() []string { return m.harnesses }
func (m *mockWorkflow) Execute(ctx WorkflowContext) (WorkflowResult, error) {
	return WorkflowResult{Success: true}, nil
}

// resetWorkflowRegistry replaces the workflow registry for testing.
func resetWorkflowRegistry(workflows map[string]Workflow) {
	registry.Lock()
	defer registry.Unlock()
	registry.workflows = workflows
}

func snapshotWorkflowRegistry() map[string]Workflow {
	registry.RLock()
	defer registry.RUnlock()
	cp := make(map[string]Workflow, len(registry.workflows))
	for k, v := range registry.workflows {
		cp[k] = v
	}
	return cp
}

func TestConcurrentWorkflowRegistration(t *testing.T) {
	orig := snapshotWorkflowRegistry()
	resetWorkflowRegistry(make(map[string]Workflow))
	defer func() { resetWorkflowRegistry(orig) }()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := fmt.Sprintf("wf-%d", id)
			Register(&mockWorkflow{name: name, harnesses: []string{"claude-code"}})
		}(i)
	}
	wg.Wait()

	// Verify all workflows were registered
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("wf-%d", i)
		got, ok := Get(name)
		if !ok {
			t.Errorf("workflow %q not found after concurrent registration", name)
			continue
		}
		if got.Name() != name {
			t.Errorf("Get(%q).Name() = %q, want %q", name, got.Name(), name)
		}
	}
}

func TestConcurrentWorkflowLookup(t *testing.T) {
	orig := snapshotWorkflowRegistry()
	resetWorkflowRegistry(make(map[string]Workflow))
	defer func() { resetWorkflowRegistry(orig) }()

	// Pre-populate
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("wf-%d", i)
		Register(&mockWorkflow{name: name, harnesses: []string{"claude-code"}})
	}

	// Concurrent reads, writes, and list operations
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Read existing
			Get(fmt.Sprintf("wf-%d", id%10))
			// List all
			List()
			// ListForHarness
			ListForHarness("claude-code")
			// Write new
			newName := fmt.Sprintf("new-wf-%d", id)
			Register(&mockWorkflow{name: newName, harnesses: []string{"gemini-cli"}})
			// Read back
			Get(newName)
		}(i)
	}
	wg.Wait()
}
