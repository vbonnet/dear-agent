package agent

import (
	"fmt"
	"maps"
	"slices"
	"sync"
)

// HarnessConformanceFinding describes a shared adapter-contract violation.
type HarnessConformanceFinding struct {
	Harness     string
	Requirement string
	Message     string
}

func (f HarnessConformanceFinding) Error() string {
	return fmt.Sprintf("%s %s: %s", f.Harness, f.Requirement, f.Message)
}

// ValidateActiveHarnessConformance runs the shared non-I/O adapter contract
// against every active parity harness.
func ValidateActiveHarnessConformance() []HarnessConformanceFinding {
	var findings []HarnessConformanceFinding
	for _, harness := range ActiveHarnesses() {
		adapter, err := newConformanceAdapter(harness)
		if err != nil {
			findings = append(findings, HarnessConformanceFinding{
				Harness:     harness,
				Requirement: "adapter construction",
				Message:     err.Error(),
			})
			continue
		}
		findings = append(findings, ValidateHarnessConformance(harness, adapter)...)
	}
	return findings
}

// ValidateHarnessConformance validates one adapter against the shared
// harness-neutral contract. It intentionally avoids lifecycle methods that
// create tmux sessions or call external harness binaries.
func ValidateHarnessConformance(harness string, adapter Harness) []HarnessConformanceFinding {
	harness = NormalizeHarnessName(harness)
	var findings []HarnessConformanceFinding

	findings = append(findings, validateHarnessIdentity(harness, adapter)...)
	if adapter == nil {
		return findings
	}

	findings = append(findings, validateHarnessCapabilities(harness, adapter.Capabilities())...)
	findings = append(findings, validateHarnessModelCoverage(harness)...)

	return findings
}

func validateHarnessIdentity(harness string, adapter Harness) []HarnessConformanceFinding {
	var findings []HarnessConformanceFinding
	if !slices.Contains(ActiveHarnesses(), harness) {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "active harness",
			Message:     fmt.Sprintf("harness is not in active parity set %v", ActiveHarnesses()),
		})
	}
	if adapter == nil {
		return append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "adapter instance",
			Message:     "adapter is nil",
		})
	}
	if got := adapter.Name(); got != harness {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "canonical name",
			Message:     fmt.Sprintf("Name() = %q, want %q", got, harness),
		})
	}
	if adapter.Version() == "" {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "version",
			Message:     "Version() is empty",
		})
	}
	return findings
}

func validateHarnessCapabilities(harness string, caps Capabilities) []HarnessConformanceFinding {
	var findings []HarnessConformanceFinding
	if caps.ModelName == "" {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "capabilities",
			Message:     "Capabilities().ModelName is empty",
		})
	}
	if caps.MaxContextWindow <= 0 {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "capabilities",
			Message:     fmt.Sprintf("Capabilities().MaxContextWindow = %d, want > 0", caps.MaxContextWindow),
		})
	}
	if !caps.SupportsTools {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "capabilities",
			Message:     "Capabilities().SupportsTools is false",
		})
	}
	if !caps.SupportsStreaming {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "capabilities",
			Message:     "Capabilities().SupportsStreaming is false",
		})
	}
	return findings
}

func validateHarnessModelCoverage(harness string) []HarnessConformanceFinding {
	var findings []HarnessConformanceFinding
	defaultModel, ok := DefaultModelForHarness(harness)
	if !ok || defaultModel == "" {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "default model",
			Message:     "DefaultModelForHarness returned no model",
		})
	} else if err := ValidateModel(harness, defaultModel); err != nil {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "default model",
			Message:     fmt.Sprintf("default model %q failed validation: %v", defaultModel, err),
		})
	}

	testModel, ok := TestModelForHarness(harness)
	if !ok || testModel == "" {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "test model",
			Message:     "TestModelForHarness returned no model",
		})
	} else if err := ValidateModel(harness, testModel); err != nil {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "test model",
			Message:     fmt.Sprintf("test model %q failed validation: %v", testModel, err),
		})
	}

	if aliases := ModelAliases(harness); len(aliases) == 0 {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "model aliases",
			Message:     "ModelAliases returned no aliases",
		})
	}
	if families := ModelFamiliesForHarness(harness); len(families) == 0 {
		findings = append(findings, HarnessConformanceFinding{
			Harness:     harness,
			Requirement: "model families",
			Message:     "ModelFamiliesForHarness returned no families",
		})
	}

	return findings
}

func newConformanceAdapter(harness string) (Harness, error) {
	return newHarnessWithStore(harness, newMemorySessionStore())
}

type memorySessionStore struct {
	mu       sync.RWMutex
	sessions map[SessionID]*SessionMetadata
}

func newMemorySessionStore() *memorySessionStore {
	return &memorySessionStore{sessions: map[SessionID]*SessionMetadata{}}
}

func (s *memorySessionStore) Get(sessionID SessionID) (*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metadata, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrSessionNotFound
	}
	return metadata, nil
}

func (s *memorySessionStore) Set(sessionID SessionID, metadata *SessionMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionID] = metadata
	return nil
}

func (s *memorySessionStore) Delete(sessionID SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sessions, sessionID)
	return nil
}

func (s *memorySessionStore) List() (map[SessionID]*SessionMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return maps.Clone(s.sessions), nil
}
