package plugin

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// Registry manages loaded plugins and their EventBus subscriptions
type Registry struct {
	plugins  []*Plugin
	eventBus *eventbus.LocalBus
	executor *Executor
	logger   *Logger
}

// NewRegistry creates a new plugin registry
func NewRegistry(plugins []*Plugin, bus *eventbus.LocalBus) *Registry {
	logger := NewDefaultLogger()
	return &Registry{
		plugins:  plugins,
		eventBus: bus,
		executor: NewExecutorWithLogger(logger),
		logger:   logger,
	}
}

// NewRegistryWithLogger creates a new plugin registry with a custom logger
func NewRegistryWithLogger(plugins []*Plugin, bus *eventbus.LocalBus, logger *Logger) *Registry {
	return &Registry{
		plugins:  plugins,
		eventBus: bus,
		executor: NewExecutorWithLogger(logger),
		logger:   logger,
	}
}

// RegisterEventBusHandlers registers plugins that subscribe to EventBus events
func (r *Registry) RegisterEventBusHandlers(ctx context.Context) error {
	r.logger.Info(ctx, "Registering EventBus handlers for plugins",
		WithOperation("register_eventbus_handlers").WithExtra("plugin_count", len(r.plugins)))

	registeredCount := 0
	for _, plugin := range r.plugins {
		if len(plugin.Manifest.EventBus.Subscribe) == 0 {
			continue
		}

		r.logger.Debug(ctx, "Registering EventBus subscriptions for plugin",
			WithPlugin(plugin.Manifest.Name).WithExtra("events", plugin.Manifest.EventBus.Subscribe))

		// Register handler for each subscribed event type
		for _, eventType := range plugin.Manifest.EventBus.Subscribe {
			// P1-8: Create local copy to avoid closure bug
			// Without this, all handlers would reference the last plugin in the loop
			currentPlugin := plugin
			pluginName := currentPlugin.Manifest.Name

			r.eventBus.Subscribe(eventType, pluginName, func(ctx context.Context, event *eventbus.Event) (*eventbus.Response, error) {
				errCtx := WithPlugin(pluginName).WithOperation("eventbus_handler").WithExtra("event_type", eventType)
				r.logger.Info(ctx, "Executing EventBus handler", errCtx.WithExtra("event_id", event.ID))

				// Execute handler (placeholder - would need to pass event as JSON)
				_, err := r.executor.Execute(ctx, currentPlugin, "handler", []string{})
				if err != nil {
					r.logger.Error(ctx, "EventBus handler execution failed", errCtx, err)
					return nil, fmt.Errorf("handler failed: %w", err)
				}

				r.logger.Info(ctx, "EventBus handler completed successfully", errCtx)

				// Return success response
				return eventbus.NewResponse(event.ID, pluginName, map[string]interface{}{
					"status": "handled",
				}), nil
			})

			registeredCount++
			r.logger.Debug(ctx, "Registered EventBus subscription",
				WithPlugin(pluginName).WithExtra("event_type", eventType))
		}
	}

	r.logger.Info(ctx, "EventBus handler registration complete",
		WithOperation("register_eventbus_handlers").WithExtra("total_subscriptions", registeredCount))

	return nil
}

// GetPlugin returns a plugin by name
func (r *Registry) GetPlugin(name string) (*Plugin, error) {
	ctx := context.Background()
	r.logger.Debug(ctx, "Looking up plugin", WithPlugin(name).WithOperation("get_plugin"))

	for _, p := range r.plugins {
		if p.Manifest.Name == name {
			r.logger.Debug(ctx, "Plugin found", WithPlugin(name))
			return p, nil
		}
	}

	err := fmt.Errorf("plugin not found: %s", name)
	r.logger.Warn(ctx, "Plugin not found in registry",
		WithPlugin(name).WithOperation("get_plugin").WithExtra("available_plugins", r.getPluginNames()), err)
	return nil, err
}

// getPluginNames returns all plugin names in the registry
func (r *Registry) getPluginNames() []string {
	names := make([]string, len(r.plugins))
	for i, p := range r.plugins {
		names[i] = p.Manifest.Name
	}
	return names
}

// ListPlugins returns all loaded plugins
func (r *Registry) ListPlugins() []*Plugin {
	return r.plugins
}

// ExecuteCommand executes a plugin command
func (r *Registry) ExecuteCommand(ctx context.Context, pluginName string, commandName string, args []string) ([]byte, error) {
	errCtx := WithCommand(pluginName, commandName).WithOperation("execute_command")
	r.logger.Info(ctx, "Executing plugin command", errCtx.WithExtra("args", args))

	plugin, err := r.GetPlugin(pluginName)
	if err != nil {
		r.logger.Error(ctx, "Failed to get plugin for command execution", errCtx, err)
		return nil, err
	}

	output, err := r.executor.Execute(ctx, plugin, commandName, args)
	if err != nil {
		r.logger.Error(ctx, "Command execution failed", errCtx, err)
		return output, err
	}

	r.logger.Info(ctx, "Command execution succeeded", errCtx)
	return output, nil
}

// Close cleans up plugin registry resources
func (r *Registry) Close() error {
	// Cleanup executor resources
	if r.executor != nil {
		return r.executor.Close()
	}
	return nil
}
