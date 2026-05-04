package detectors

import (
	"context"
	"fmt"
	"sync"

	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// Registry manages registered detectors
type Registry struct {
	mu        sync.RWMutex
	detectors map[string]Detector
}

// NewRegistry creates a new detector registry
func NewRegistry() *Registry {
	return &Registry{
		detectors: make(map[string]Detector),
	}
}

// Register adds a detector to the registry
func (r *Registry) Register(detector Detector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := detector.Name()
	if _, exists := r.detectors[name]; exists {
		return fmt.Errorf("detector %q already registered", name)
	}

	r.detectors[name] = detector
	return nil
}

// Get retrieves detector by name
func (r *Registry) Get(name string) (Detector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	detector, exists := r.detectors[name]
	return detector, exists
}

// DetectorsForInstructionType returns all detectors handling given type
func (r *Registry) DetectorsForInstructionType(instructionType string) []Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Detector
	for _, detector := range r.detectors {
		for _, supportedType := range detector.SupportedInstructionTypes() {
			if supportedType == instructionType {
				result = append(result, detector)
				break
			}
		}
	}
	return result
}

// RunAll executes all registered detectors on input
// Returns aggregated violations even if some detectors fail
func (r *Registry) RunAll(ctx context.Context, input DetectorInput) ([]telemetry.ViolationEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allViolations []telemetry.ViolationEvent
	var errors []error

	for _, detector := range r.detectors {
		violations, err := detector.Detect(ctx, input)
		if err != nil {
			// Log error but continue with other detectors (graceful degradation)
			errors = append(errors, fmt.Errorf("%s: %w", detector.Name(), err))
			continue
		}
		allViolations = append(allViolations, violations...)
	}

	// Return violations even if some detectors failed
	if len(errors) > 0 {
		return allViolations, fmt.Errorf("detector failures: %v", errors)
	}

	return allViolations, nil
}

// globalRegistry is the singleton registry instance
var globalRegistry *Registry
var once sync.Once

// GlobalRegistry returns the global detector registry
func GlobalRegistry() *Registry {
	once.Do(func() {
		globalRegistry = NewRegistry()
		// Register default detectors
		globalRegistry.Register(NewBashCommandPatternDetector()) //nolint:errcheck
	})
	return globalRegistry
}
