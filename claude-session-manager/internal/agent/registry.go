package agent

// registry stores registered agents by name.
var registry = make(map[string]Agent)

// Register adds an agent to the registry under the given name.
// If an agent with the same name already exists, it is replaced.
// This function is typically called from adapter package init() functions.
func Register(name string, agent Agent) {
	registry[name] = agent
}

// Get retrieves an agent by name from the registry.
// Returns the agent and true if found, or nil and false if not found.
func Get(name string) (Agent, bool) {
	agent, ok := registry[name]
	return agent, ok
}
