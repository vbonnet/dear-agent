package plugin

// EnforcementAdapter adapts the plugin Registry to the enforcement.PluginRegistry interface
// This avoids circular dependencies between plugin and enforcement packages
type EnforcementAdapter struct {
	registry *Registry
}

// NewEnforcementAdapter creates a new adapter
func NewEnforcementAdapter(registry *Registry) *EnforcementAdapter {
	return &EnforcementAdapter{registry: registry}
}

// PluginInfo minimal plugin information for enforcement validation
type PluginInfo struct {
	Name    string
	Version string
}

// ListPlugins returns list of installed plugins with minimal info
func (a *EnforcementAdapter) ListPlugins() []PluginInfo {
	plugins := a.registry.ListPlugins()
	result := make([]PluginInfo, len(plugins))

	for i, p := range plugins {
		result[i] = PluginInfo{
			Name:    p.Manifest.Name,
			Version: p.Manifest.Version,
		}
	}

	return result
}
