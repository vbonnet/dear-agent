package metacontext

import (
	"fmt"
	"time"
)

// Metacontext represents the analyzed project context sent to the LLM.
// Token budget: MaxMetacontextTokens = 5000 tokens (~2.5% of 200K context window).
type Metacontext struct {
	Languages   []Signal        `json:"languages"`
	Frameworks  []Signal        `json:"frameworks"`
	Tools       []Signal        `json:"tools"`
	Conventions []Convention    `json:"conventions"`
	Personas    []Persona       `json:"personas"`
	Metadata    MinimalMetadata `json:"metadata"`
}

// Signal represents a detected technology, framework, or tool.
type Signal struct {
	Name       string            `json:"name"`
	Confidence float64           `json:"confidence"` // 0.0-1.0
	Source     string            `json:"source"`     // "file", "dependency", "git", "conversation"
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// Convention represents a coding convention or pattern detected in the project.
type Convention struct {
	Type        string  `json:"type"` // "naming", "formatting", "architecture"
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"` // 0.0-1.0
}

// Persona represents a recommended AI persona based on detected signals.
type Persona struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Score       float64  `json:"score"`   // 0.0-1.0
	Signals     []string `json:"signals"` // Matching signal names
}

// MinimalMetadata contains only LLM-relevant metadata.
// InternalMetadata (timing, version, etc.) is logged but not sent to LLM.
type MinimalMetadata struct {
	CacheHit bool     `json:"cache_hit"`
	Warnings []string `json:"warnings,omitempty"`
}

// InternalMetadata contains observability data (not sent to LLM).
type InternalMetadata struct {
	Timestamp     time.Time                `json:"timestamp"`
	Duration      time.Duration            `json:"duration_ms"`
	ScannerTiming map[string]time.Duration `json:"scanner_timing"`
	Version       string                   `json:"version"`
	CacheHit      bool                     `json:"cache_hit"`
	Warnings      []string                 `json:"warnings,omitempty"`
}

// AnalyzeRequest contains parameters for metacontext analysis.
type AnalyzeRequest struct {
	WorkingDir   string         `json:"working_dir"`
	Conversation []string       `json:"conversation"` // Recent N conversation turns
	Timeout      *time.Duration `json:"timeout,omitempty"`
	SkipScanners []string       `json:"skip_scanners,omitempty"`
}

// Token budget constants (enforced in deduplication).
const (
	MaxMetacontextTokens = 5000 // Total budget
	MaxLanguageSignals   = 10   // Top 10 by importance
	MaxFrameworkSignals  = 15   // Top 15 by importance
	MaxToolSignals       = 20   // Top 20 by importance
	MaxConventions       = 10   // Top 10
	MaxPersonas          = 5    // Top 5
)

// Errors
var (
	ErrInvalidWorkingDir   = fmt.Errorf("invalid working directory")
	ErrMetacontextTooLarge = fmt.Errorf("metacontext exceeds token budget")
	ErrCacheCorruption     = fmt.Errorf("cache corruption detected")
	ErrConfigInvalid       = fmt.Errorf("invalid configuration")
	ErrDiskSpaceExhausted  = fmt.Errorf("disk space exhausted")
)

// estimateTokens estimates token count using rough heuristic (4 chars = 1 token).
func estimateTokens(mc *Metacontext) int {
	// Rough estimate: count all string fields
	tokens := 0

	for _, sig := range mc.Languages {
		tokens += len(sig.Name)/4 + len(sig.Source)/4
		for k, v := range sig.Metadata {
			tokens += len(k)/4 + len(v)/4
		}
	}

	for _, sig := range mc.Frameworks {
		tokens += len(sig.Name)/4 + len(sig.Source)/4
		for k, v := range sig.Metadata {
			tokens += len(k)/4 + len(v)/4
		}
	}

	for _, sig := range mc.Tools {
		tokens += len(sig.Name)/4 + len(sig.Source)/4
		for k, v := range sig.Metadata {
			tokens += len(k)/4 + len(v)/4
		}
	}

	for _, conv := range mc.Conventions {
		tokens += len(conv.Type)/4 + len(conv.Description)/4
	}

	for _, persona := range mc.Personas {
		tokens += len(persona.Name)/4 + len(persona.Description)/4
		for _, sig := range persona.Signals {
			tokens += len(sig) / 4
		}
	}

	for _, warning := range mc.Metadata.Warnings {
		tokens += len(warning) / 4
	}

	return tokens
}

// validateSize checks if metacontext exceeds token budget.
func validateSize(mc *Metacontext) error {
	tokens := estimateTokens(mc)
	if tokens > MaxMetacontextTokens {
		return fmt.Errorf("%w: %d tokens (max %d)", ErrMetacontextTooLarge, tokens, MaxMetacontextTokens)
	}
	return nil
}

// Clone creates a deep copy of Metacontext.
func (mc *Metacontext) Clone() *Metacontext {
	clone := &Metacontext{
		Languages:   make([]Signal, len(mc.Languages)),
		Frameworks:  make([]Signal, len(mc.Frameworks)),
		Tools:       make([]Signal, len(mc.Tools)),
		Conventions: make([]Convention, len(mc.Conventions)),
		Personas:    make([]Persona, len(mc.Personas)),
		Metadata: MinimalMetadata{
			CacheHit: mc.Metadata.CacheHit,
			Warnings: make([]string, len(mc.Metadata.Warnings)),
		},
	}

	copy(clone.Languages, mc.Languages)
	copy(clone.Frameworks, mc.Frameworks)
	copy(clone.Tools, mc.Tools)
	copy(clone.Conventions, mc.Conventions)
	copy(clone.Personas, mc.Personas)
	copy(clone.Metadata.Warnings, mc.Metadata.Warnings)

	return clone
}

// AllSignals returns all signals (languages + frameworks + tools).
func (mc *Metacontext) AllSignals() []Signal {
	all := make([]Signal, 0, len(mc.Languages)+len(mc.Frameworks)+len(mc.Tools))
	all = append(all, mc.Languages...)
	all = append(all, mc.Frameworks...)
	all = append(all, mc.Tools...)
	return all
}
