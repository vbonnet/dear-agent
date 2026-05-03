// Package platform provides the core Engram platform runtime that wires together
// all system components into a cohesive whole.
//
// The Platform type represents a fully initialized Engram instance with all subsystems
// configured and ready for use. It coordinates:
//
//   - Configuration: 4-tier hierarchy (core, company, team, user)
//   - Agent Detection: Automatic detection of AI coding agents
//   - Telemetry: Event collection and analytics
//   - EventBus: Inter-plugin communication
//   - Plugins: Plugin loading, registration, and execution
//   - Ecphory: Memory retrieval and pattern matching
//
// Initialization follows a specific order to ensure dependencies are met:
//  1. Load configuration from all tiers
//  2. Detect AI agent platform
//  3. Initialize telemetry collection
//  4. Create EventBus for inter-plugin communication
//  5. Load and register plugins
//  6. Initialize ecphory retrieval system
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
//	defer cancel()
//
//	platform, err := platform.New(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer platform.Close()
//
//	// Access platform components
//	cfg := platform.Config()
//	plugins := platform.Plugins()
//	ecphory := platform.Ecphory()
//
// The Platform type is the main entry point for CLI commands and server initialization.
package platform

import (
	"context"
	"fmt"

	"github.com/vbonnet/dear-agent/engram/ecphory"
	"github.com/vbonnet/dear-agent/engram/internal/agent"
	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/plugin"
	"github.com/vbonnet/dear-agent/internal/telemetry"
	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// Platform represents the core Engram platform runtime
type Platform struct {
	config    *config.Config
	agent     agent.Agent
	telemetry *telemetry.Collector
	eventBus  *eventbus.LocalBus
	plugins   *plugin.Registry
	ecphory   *ecphory.Ecphory
}

// New creates a new Platform instance with context support for cancellation and timeout
func New(ctx context.Context) (*Platform, error) {
	// Load configuration from all tiers
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("config load failed: %w", err)
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("platform initialization cancelled: %w", ctx.Err())
	default:
	}

	// Detect agent platform
	detector := agent.NewDetector()
	agentType := detector.Detect()

	// Override config agent if detected
	if agentType != agent.AgentUnknown {
		cfg.Platform.Agent = string(agentType)
	}

	// Initialize telemetry
	tel, err := telemetry.NewCollector(cfg.Telemetry.Enabled, cfg.Telemetry.Path)
	if err != nil {
		return nil, fmt.Errorf("telemetry initialization failed: %w", err)
	}

	// Register telemetry listeners for analytics (FR1, FR2, FR3)
	if cfg.Telemetry.Enabled {
		// Determine log directory (default: ~/.engram/logs)
		logDir := cfg.Telemetry.Path
		if logDir == "" {
			// Default to ~/.engram/logs if not configured
			logDir = cfg.Platform.EngramPath + "/logs"
		}

		// FR1: Ecphory correctness validation
		ecphoryLogger := telemetry.NewEcphoryAuditLogger(logDir)
		tel.AddListener(ecphoryLogger)

		// FR2: Persona effectiveness tracking
		personaLogger := telemetry.NewPersonaEffectivenessLogger(logDir)
		tel.AddListener(personaLogger)

		// FR3: Wayfinder ROI tracking
		wayfinderLogger := telemetry.NewWayfinderROILogger(logDir)
		tel.AddListener(wayfinderLogger)
	}

	// Record platform initialization
	_ = tel.Record(telemetry.EventConfigLoaded, cfg.Platform.Agent, telemetry.LevelInfo, map[string]interface{}{
		"agent": cfg.Platform.Agent,
	})

	// Check context cancellation
	select {
	case <-ctx.Done():
		_ = tel.Close()
		return nil, fmt.Errorf("platform initialization cancelled after telemetry: %w", ctx.Err())
	default:
	}

	// Initialize EventBus
	bus := eventbus.NewBus(tel)

	// Load plugins
	pluginLoader := plugin.NewLoader(cfg.Plugins.Paths, cfg.Plugins.Disabled)
	loadedPlugins, err := pluginLoader.Load()
	if err != nil {
		_ = bus.Close() // Close EventBus first
		_ = tel.Close() // Then close telemetry
		return nil, fmt.Errorf("plugin load failed: %w", err)
	}

	// Create plugin registry
	registry := plugin.NewRegistry(loadedPlugins, bus)

	// Register EventBus handlers with context
	if err := registry.RegisterEventBusHandlers(ctx); err != nil {
		_ = bus.Close() // Close EventBus first
		_ = tel.Close() // Then close telemetry
		return nil, fmt.Errorf("plugin registration failed: %w", err)
	}

	// Check context cancellation
	select {
	case <-ctx.Done():
		_ = bus.Close() // Close EventBus first
		_ = tel.Close() // Then close telemetry
		return nil, fmt.Errorf("platform initialization cancelled after plugins: %w", ctx.Err())
	default:
	}

	// Initialize ecphory (if engram path configured)
	var eph *ecphory.Ecphory
	if cfg.Platform.EngramPath != "" {
		eph, err = ecphory.NewEcphory(cfg.Platform.EngramPath, cfg.Platform.TokenBudget)
		if err != nil {
			// Non-fatal - ecphory is optional
			eph = nil
		}
	}

	return &Platform{
		config:    cfg,
		agent:     agentType,
		telemetry: tel,
		eventBus:  bus,
		plugins:   registry,
		ecphory:   eph,
	}, nil
}

// Config returns the platform configuration
func (p *Platform) Config() *config.Config {
	return p.config
}

// Agent returns the detected agent
func (p *Platform) Agent() agent.Agent {
	return p.agent
}

// Telemetry returns the telemetry collector
func (p *Platform) Telemetry() *telemetry.Collector {
	return p.telemetry
}

// EventBus returns the EventBus
func (p *Platform) EventBus() *eventbus.LocalBus {
	return p.eventBus
}

// Plugins returns the plugin registry
func (p *Platform) Plugins() *plugin.Registry {
	return p.plugins
}

// Ecphory returns the ecphory retrieval system
func (p *Platform) Ecphory() *ecphory.Ecphory {
	return p.ecphory
}

// Close cleans up platform resources in reverse initialization order
func (p *Platform) Close() error {
	// Handle nil platform
	if p == nil {
		return nil
	}

	var errs []error

	// Add panic recovery to prevent crashes during cleanup
	defer func() {
		if r := recover(); r != nil {
			errs = append(errs, fmt.Errorf("panic during close: %v", r))
		}
	}()

	// Close resources in reverse initialization order with panic protection
	// 6. Close ecphory (if initialized)
	if p.ecphory != nil {
		if err := safeClose("ecphory", p.ecphory.Close); err != nil {
			errs = append(errs, err)
		}
	}

	// 5. Close plugins
	if p.plugins != nil {
		if err := safeClose("plugins", p.plugins.Close); err != nil {
			errs = append(errs, err)
		}
	}

	// 4. Close EventBus
	if p.eventBus != nil {
		if err := safeClose("eventbus", p.eventBus.Close); err != nil {
			errs = append(errs, err)
		}
	}

	// 3. Close telemetry
	if p.telemetry != nil {
		if err := safeClose("telemetry", p.telemetry.Close); err != nil {
			errs = append(errs, err)
		}
	}

	// Combine all errors
	if len(errs) > 0 {
		return fmt.Errorf("platform close errors: %v", errs)
	}

	return nil
}

// safeClose wraps Close() calls to prevent panics from propagating
func safeClose(name string, closeFn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s close panicked: %v", name, r)
		}
	}()

	if err := closeFn(); err != nil {
		return fmt.Errorf("%s close: %w", name, err)
	}
	return nil
}
