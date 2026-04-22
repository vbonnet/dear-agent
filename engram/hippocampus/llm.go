package hippocampus

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// LLMProvider extracts signals and detects contradictions using an LLM.
// Optional: when nil or NoopLLM, the pipeline uses V1 pattern-only mode.
type LLMProvider interface {
	// ExtractSignals analyzes transcript text and returns structured signals.
	// Input is concatenated user+assistant text from one or more sessions.
	ExtractSignals(ctx context.Context, transcript string) ([]Signal, error)

	// DetectContradictions compares memory entries for conflicts.
	// Returns contradictions found between entries.
	DetectContradictions(ctx context.Context, existing []string, incoming []string) ([]Contradiction, error)
}

// Contradiction represents a conflict between two memory entries.
type Contradiction struct {
	Existing   string // the existing memory entry
	New        string // the incoming entry that conflicts
	Resolution string // explanation of how to resolve
	Winner     string // "existing" or "new"
}

// NoopLLM is a no-op LLM provider for V1 pattern-only mode.
// All methods return empty results.
type NoopLLM struct{}

// ExtractSignals returns nil (no LLM extraction in V1 mode).
func (n *NoopLLM) ExtractSignals(_ context.Context, _ string) ([]Signal, error) {
	return nil, nil
}

// DetectContradictions returns nil (no LLM contradiction detection in V1 mode).
func (n *NoopLLM) DetectContradictions(_ context.Context, _ []string, _ []string) ([]Contradiction, error) {
	return nil, nil
}

// SonnetLLM implements LLMProvider using an LLM via SideQueryFunc.
// This is the V2 implementation that replaces regex-based extraction with
// LLM-powered signal extraction and contradiction detection.
type SonnetLLM struct {
	SideQuery SideQueryFunc
}

// NewSonnetLLM creates a new LLM provider backed by a SideQueryFunc.
func NewSonnetLLM(sideQuery SideQueryFunc) *SonnetLLM {
	return &SonnetLLM{SideQuery: sideQuery}
}

const extractSignalsSystemPrompt = `You are a memory signal extractor. Analyze the conversation transcript and extract memorable signals.

Signal types:
- "correction": User correcting the assistant ("no actually", "that's wrong", "don't do that")
- "preference": User expressing a preference ("always use", "I prefer", "from now on")
- "decision": A decision being made ("let's go with", "we decided", "the plan is")
- "learning": Something learned ("discovered", "turns out", "TIL", "found that")
- "fact": Factual information worth remembering

For each signal, provide:
- type: one of the signal types above
- content: concise summary of the signal (1-2 sentences)
- confidence: 0.0-1.0 how confident this is a real signal (not noise)

Return JSON: {"signals": [{"type": "...", "content": "...", "confidence": 0.8}]}
Only include signals with confidence >= 0.5. Return {"signals": []} if none found.`

// llmSignalResponse is the expected JSON structure from signal extraction.
type llmSignalResponse struct {
	Signals []struct {
		Type       string  `json:"type"`
		Content    string  `json:"content"`
		Confidence float64 `json:"confidence"`
	} `json:"signals"`
}

// ExtractSignals uses an LLM to analyze transcript text and extract structured signals.
// Falls back gracefully: returns nil signals on LLM error (caller uses V1 regex).
func (s *SonnetLLM) ExtractSignals(ctx context.Context, transcript string) ([]Signal, error) {
	if s.SideQuery == nil {
		return nil, nil
	}

	// Truncate very long transcripts to avoid overwhelming the LLM
	if len(transcript) > 20000 {
		transcript = transcript[:20000] + "\n... (truncated)"
	}

	response, err := s.SideQuery(ctx, extractSignalsSystemPrompt, transcript, 1024)
	if err != nil {
		return nil, fmt.Errorf("extract signals LLM call: %w", err)
	}

	var parsed llmSignalResponse
	if err := unmarshalLLMJSON(response, &parsed); err != nil {
		return nil, fmt.Errorf("parse signal response: %w", err)
	}

	now := time.Now()
	var signals []Signal
	for _, s := range parsed.Signals {
		if s.Confidence < 0.5 {
			continue
		}
		st := parseSignalType(s.Type)
		signals = append(signals, Signal{
			Type:       st,
			Content:    s.Content,
			Timestamp:  now,
			Confidence: s.Confidence,
		})
	}

	return signals, nil
}

const detectContradictionsSystemPrompt = `You are a memory contradiction detector. Compare existing memory entries against incoming entries and find conflicts.

A contradiction exists when two entries make incompatible claims about the same topic.
Example: "Always use tabs" vs "Always use spaces" — these contradict.
Non-example: "Use Go for backend" vs "Use Python for scripts" — these are about different things.

For each contradiction found:
- existing: the existing entry text
- new: the incoming entry that conflicts
- resolution: brief explanation of how to resolve
- winner: "new" (newer information wins) or "existing" (if the new claim seems less reliable)

Return JSON: {"contradictions": [{"existing": "...", "new": "...", "resolution": "...", "winner": "new"}]}
Return {"contradictions": []} if no contradictions found.`

// llmContradictionResponse is the expected JSON structure from contradiction detection.
type llmContradictionResponse struct {
	Contradictions []struct {
		Existing   string `json:"existing"`
		New        string `json:"new"`
		Resolution string `json:"resolution"`
		Winner     string `json:"winner"`
	} `json:"contradictions"`
}

// DetectContradictions uses an LLM to compare memory entries for conflicts.
func (s *SonnetLLM) DetectContradictions(ctx context.Context, existing []string, incoming []string) ([]Contradiction, error) {
	if s.SideQuery == nil {
		return nil, nil
	}

	if len(existing) == 0 || len(incoming) == 0 {
		return nil, nil
	}

	userPrompt := fmt.Sprintf("Existing entries:\n%s\n\nIncoming entries:\n%s",
		strings.Join(existing, "\n"),
		strings.Join(incoming, "\n"),
	)

	response, err := s.SideQuery(ctx, detectContradictionsSystemPrompt, userPrompt, 1024)
	if err != nil {
		return nil, fmt.Errorf("detect contradictions LLM call: %w", err)
	}

	var parsed llmContradictionResponse
	if err := unmarshalLLMJSON(response, &parsed); err != nil {
		return nil, fmt.Errorf("parse contradiction response: %w", err)
	}

	var contradictions []Contradiction
	for _, c := range parsed.Contradictions {
		winner := c.Winner
		if winner != "existing" && winner != "new" {
			winner = "new" // default to newer wins
		}
		contradictions = append(contradictions, Contradiction{
			Existing:   c.Existing,
			New:        c.New,
			Resolution: c.Resolution,
			Winner:     winner,
		})
	}

	return contradictions, nil
}

// parseSignalType converts a string to SignalType, defaulting to SignalFact.
func parseSignalType(s string) SignalType {
	switch SignalType(s) {
	case SignalCorrection:
		return SignalCorrection
	case SignalPreference:
		return SignalPreference
	case SignalDecision:
		return SignalDecision
	case SignalLearning:
		return SignalLearning
	case SignalFact:
		return SignalFact
	default:
		return SignalFact
	}
}

// unmarshalLLMJSON attempts to parse JSON from an LLM response, handling
// markdown code fences and other wrapper text.
func unmarshalLLMJSON(response string, v any) error {
	// Try direct parse first
	if err := json.Unmarshal([]byte(response), v); err == nil {
		return nil
	}

	// Try extracting JSON from markdown/wrapper text
	start := strings.Index(response, "{")
	if start < 0 {
		return fmt.Errorf("no JSON object found in response")
	}
	end := strings.LastIndex(response, "}")
	if end < start {
		return fmt.Errorf("no closing brace found in response")
	}

	return json.Unmarshal([]byte(response[start:end+1]), v)
}
