package agent

import "sync"

// registry stores registered agents by name.
var registry = struct {
	sync.RWMutex
	agents map[string]Agent
}{
	agents: make(map[string]Agent),
}

// Register adds an agent to the registry under the given name.
// If an agent with the same name already exists, it is replaced.
// This function is typically called from adapter package init() functions.
func Register(name string, agent Agent) {
	registry.Lock()
	defer registry.Unlock()
	registry.agents[name] = agent
}

// Get retrieves an agent by name from the registry.
// Returns the agent and true if found, or nil and false if not found.
func Get(name string) (Agent, bool) {
	registry.RLock()
	defer registry.RUnlock()
	agent, ok := registry.agents[name]
	return agent, ok
}

// resetRegistryForTest replaces the registry map contents for testing.
// Not safe for concurrent use — call only from test setup.
func resetRegistryForTest(agents map[string]Agent) {
	registry.Lock()
	defer registry.Unlock()
	registry.agents = agents
}

// snapshotRegistryForTest returns a shallow copy of the registry map for test save/restore.
func snapshotRegistryForTest() map[string]Agent {
	registry.RLock()
	defer registry.RUnlock()
	cp := make(map[string]Agent, len(registry.agents))
	for k, v := range registry.agents {
		cp[k] = v
	}
	return cp
}
