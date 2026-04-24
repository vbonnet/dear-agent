package consolidation

import "fmt"

// ProviderFactory creates a Provider from configuration.
type ProviderFactory func(config Config) (Provider, error)

var (
	// providers maps provider type name to factory function.
	providers = make(map[string]ProviderFactory)
)

// Register adds a provider factory to the registry.
//
// Called by provider implementations during init() to make themselves available.
// Provider type names must be unique.
//
// Example - Register a custom provider:
//
//	func init() {
//	    consolidation.Register("myprovider", NewMyProvider)
//	}
//
//	func NewMyProvider(config consolidation.Config) (consolidation.Provider, error) {
//	    return &MyProvider{}, nil
//	}
func Register(name string, factory ProviderFactory) {
	providers[name] = factory
}

// Load creates a provider instance from configuration.
//
// Returns ErrProviderNotFound if the provider type is not registered.
// Returns ErrInvalidConfig if the provider configuration is invalid.
//
// Example:
//
//	config := Config{
//	    ProviderType: "simple",
//	    Options: map[string]interface{}{
//	        "storage_path": "/path/to/storage",
//	    },
//	}
//	provider, err := Load(config)
func Load(config Config) (Provider, error) {
	factory, ok := providers[config.ProviderType]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, config.ProviderType)
	}

	return factory(config)
}

// ListProviders returns all registered provider type names.
//
// Useful for debugging and displaying available providers to users.
func ListProviders() []string {
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
