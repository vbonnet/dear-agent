package context

import (
	"fmt"
	"time"
)

// SessionType represents the type of agent session for threshold selection.
type SessionType string

const (
	SessionOrchestrator     SessionType = "orchestrator"
	SessionWorker           SessionType = "worker"
	SessionMetaOrchestrator SessionType = "meta-orchestrator"
)

// Strategy controls compaction aggressiveness.
type Strategy string

const (
	StrategyConservative Strategy = "conservative"
	StrategyAggressive   Strategy = "aggressive"
)

// CompactConfig holds configuration for a compaction operation.
type CompactConfig struct {
	// Focus is the context preservation instruction (required).
	Focus string

	// SessionType determines which thresholds to use.
	SessionType SessionType

	// Strategy controls compaction aggressiveness.
	Strategy Strategy

	// Usage is the current context window usage.
	Usage *Usage

	// PhaseState is the current phase state.
	PhaseState PhaseState
}

// CompactResult is the JSON-serializable output of a compact operation.
type CompactResult struct {
	Success        bool            `json:"success"`
	ReductionPct   float64         `json:"reduction_pct"`
	NewZone        Zone            `json:"new_zone"`
	CompactedItems []CompactedItem `json:"compacted_items"`
	Layer          string          `json:"layer"`
	Error          string          `json:"error,omitempty"`
	Cooldown       *CooldownStatus `json:"cooldown,omitempty"`
}

// CompactedItem describes a single item that was compacted or flagged.
type CompactedItem struct {
	Type        string `json:"type"`
	Description string `json:"description"`
	Action      string `json:"action"`
}

// CooldownStatus tracks anti-loop safety state.
type CooldownStatus struct {
	Active            bool      `json:"active"`
	LastCompaction    time.Time `json:"last_compaction,omitempty"`
	CompactionCount   int       `json:"compaction_count"`
	MaxCompactions    int       `json:"max_compactions"`
	CooldownRemaining string    `json:"cooldown_remaining,omitempty"`
}

// SessionThresholds holds the three-layer thresholds for a session type.
type SessionThresholds struct {
	Prevention float64 // Layer 1: prevention threshold (%)
	Compaction float64 // Layer 2: compaction threshold (%)
	Rotation   float64 // Layer 3: rotation threshold (%)
}

// Anti-loop safety constants.
const (
	CooldownDuration = 2 * time.Hour
	MaxCompactions   = 3
)

// sessionThresholdMap maps session types to their three-layer thresholds.
var sessionThresholdMap = map[SessionType]SessionThresholds{
	SessionOrchestrator:     {Prevention: 55, Compaction: 65, Rotation: 80},
	SessionWorker:           {Prevention: 70, Compaction: 80, Rotation: 90},
	SessionMetaOrchestrator: {Prevention: 50, Compaction: 60, Rotation: 75},
}

// GetSessionThresholds returns the three-layer thresholds for a session type.
func GetSessionThresholds(st SessionType) SessionThresholds {
	if t, ok := sessionThresholdMap[st]; ok {
		return t
	}
	// Default to orchestrator thresholds
	return sessionThresholdMap[SessionOrchestrator]
}

// CompactEngine performs context compaction with three-layer defense and anti-loop safety.
type CompactEngine struct {
	registry   *Registry
	calculator *Calculator
	history    []time.Time // timestamps of recent compactions
}

// NewCompactEngine creates a new compaction engine.
func NewCompactEngine(registry *Registry) *CompactEngine {
	return &CompactEngine{
		registry:   registry,
		calculator: NewCalculator(registry),
		history:    make([]time.Time, 0),
	}
}

// Compact performs the three-layer compaction analysis and returns a result.
func (e *CompactEngine) Compact(cfg *CompactConfig) *CompactResult {
	if cfg.Focus == "" {
		return &CompactResult{
			Success: false,
			Error:   "--focus flag is required: provide context preservation instructions",
		}
	}

	// Anti-loop safety check
	cooldown := e.checkCooldown()
	if cooldown.Active {
		return &CompactResult{
			Success:  false,
			Error:    fmt.Sprintf("compaction blocked: cooldown active (%s remaining), max %d compactions per session", cooldown.CooldownRemaining, MaxCompactions),
			Cooldown: cooldown,
		}
	}

	thresholds := GetSessionThresholds(cfg.SessionType)
	pct := cfg.Usage.PercentageUsed
	items := make([]CompactedItem, 0)

	// Determine which layer we're in
	var layer string
	var newZone Zone
	var reductionPct float64

	switch {
	case pct >= thresholds.Rotation:
		// Layer 3: Rotation — session should be rotated
		layer = "rotation"
		items = append(items, CompactedItem{
			Type:        "rotation",
			Description: fmt.Sprintf("Context at %.1f%% exceeds rotation threshold (%.0f%%) for %s", pct, thresholds.Rotation, cfg.SessionType),
			Action:      "rotate_session",
		})
		items = append(items, CompactedItem{
			Type:        "focus_preservation",
			Description: cfg.Focus,
			Action:      "carry_forward",
		})
		reductionPct = e.estimateReduction(cfg.Strategy, layer)
		newZone = e.estimateNewZone(pct, reductionPct, cfg.Usage.ModelID)

	case pct >= thresholds.Compaction:
		// Layer 2: Compaction — active compaction needed
		layer = "compaction"
		items = e.buildCompactionItems(cfg, thresholds)
		reductionPct = e.estimateReduction(cfg.Strategy, layer)
		newZone = e.estimateNewZone(pct, reductionPct, cfg.Usage.ModelID)

	case pct >= thresholds.Prevention:
		// Layer 1: Prevention — proactive measures
		layer = "prevention"
		items = append(items, CompactedItem{
			Type:        "prevention",
			Description: fmt.Sprintf("Context at %.1f%% approaching compaction threshold (%.0f%%) for %s", pct, thresholds.Compaction, cfg.SessionType),
			Action:      "reduce_verbosity",
		})
		items = append(items, CompactedItem{
			Type:        "focus_preservation",
			Description: cfg.Focus,
			Action:      "prioritize",
		})
		reductionPct = e.estimateReduction(cfg.Strategy, layer)
		newZone = e.estimateNewZone(pct, reductionPct, cfg.Usage.ModelID)

	default:
		// Below all thresholds — no action needed
		zone, _ := e.calculator.CalculateZone(pct, cfg.Usage.ModelID)
		return &CompactResult{
			Success:        true,
			ReductionPct:   0,
			NewZone:        zone,
			CompactedItems: items,
			Layer:          "none",
			Cooldown:       cooldown,
		}
	}

	// Record compaction for anti-loop tracking
	e.history = append(e.history, time.Now())

	return &CompactResult{
		Success:        true,
		ReductionPct:   reductionPct,
		NewZone:        newZone,
		CompactedItems: items,
		Layer:          layer,
		Cooldown:       e.checkCooldown(),
	}
}

// buildCompactionItems generates the items list for layer 2 compaction.
func (e *CompactEngine) buildCompactionItems(cfg *CompactConfig, thresholds SessionThresholds) []CompactedItem {
	items := make([]CompactedItem, 0, 4)

	items = append(items, CompactedItem{
		Type:        "compaction",
		Description: fmt.Sprintf("Context at %.1f%% exceeds compaction threshold (%.0f%%) for %s", cfg.Usage.PercentageUsed, thresholds.Compaction, cfg.SessionType),
		Action:      "compact_context",
	})

	if cfg.Strategy == StrategyAggressive {
		items = append(items, CompactedItem{
			Type:        "aggressive",
			Description: "Aggressive strategy: drop tool outputs, collapse intermediate reasoning",
			Action:      "deep_compact",
		})
	} else {
		items = append(items, CompactedItem{
			Type:        "conservative",
			Description: "Conservative strategy: summarize old messages, preserve recent context",
			Action:      "shallow_compact",
		})
	}

	items = append(items, CompactedItem{
		Type:        "focus_preservation",
		Description: cfg.Focus,
		Action:      "preserve",
	})

	return items
}

// estimateReduction returns expected reduction percentage based on strategy and layer.
func (e *CompactEngine) estimateReduction(strategy Strategy, layer string) float64 {
	switch layer {
	case "rotation":
		return 80.0
	case "compaction":
		if strategy == StrategyAggressive {
			return 35.0
		}
		return 20.0
	case "prevention":
		return 10.0
	default:
		return 0.0
	}
}

// estimateNewZone calculates the expected zone after reduction.
func (e *CompactEngine) estimateNewZone(currentPct, reductionPct float64, modelID string) Zone {
	newPct := currentPct * (1.0 - reductionPct/100.0)
	zone, err := e.calculator.CalculateZone(newPct, modelID)
	if err != nil {
		return ZoneSafe
	}
	return zone
}

// checkCooldown returns the current cooldown status.
func (e *CompactEngine) checkCooldown() *CooldownStatus {
	now := time.Now()

	// Prune old entries outside cooldown window
	active := make([]time.Time, 0, len(e.history))
	for _, t := range e.history {
		if now.Sub(t) < CooldownDuration {
			active = append(active, t)
		}
	}
	e.history = active

	status := &CooldownStatus{
		CompactionCount: len(active),
		MaxCompactions:  MaxCompactions,
	}

	if len(active) >= MaxCompactions {
		status.Active = true
		oldest := active[0]
		remaining := CooldownDuration - now.Sub(oldest)
		status.CooldownRemaining = fmt.Sprintf("%dm", int(remaining.Minutes()))
	}

	if len(active) > 0 {
		status.LastCompaction = active[len(active)-1]
	}

	return status
}

// ParseSessionType converts a string to SessionType.
func ParseSessionType(s string) (SessionType, error) {
	switch s {
	case "orchestrator":
		return SessionOrchestrator, nil
	case "worker":
		return SessionWorker, nil
	case "meta-orchestrator":
		return SessionMetaOrchestrator, nil
	default:
		return "", fmt.Errorf("unknown session type: %s (expected: orchestrator, worker, meta-orchestrator)", s)
	}
}

// ParseStrategy converts a string to Strategy.
func ParseStrategy(s string) (Strategy, error) {
	switch s {
	case "conservative":
		return StrategyConservative, nil
	case "aggressive":
		return StrategyAggressive, nil
	default:
		return "", fmt.Errorf("unknown strategy: %s (expected: conservative, aggressive)", s)
	}
}
