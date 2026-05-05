// Package salience tiers and budgets agent-drift signals so callers can
// distinguish "the agent skipped a project template" (worth interrupting
// for) from "the agent renamed a local variable" (not worth a peep).
//
// The package layers on top of pkg/aggregator without sharing types: the
// outer aggregator records project-health metrics (git_activity, lint_trend,
// ...) whereas this one classifies discrete drift events. Mixing the two
// concepts in one Signal struct conflated time-series metrics with
// categorical events; keeping them separate is intentional.
package salience

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Tier ranks how loudly a signal demands attention. Higher value = more
// salient. Use the named constants; the int values are an implementation
// detail used for ordering and budget-bypass thresholds.
type Tier int

// Tiers, ordered from least to most salient.
const (
	TierNoise Tier = iota
	TierLow
	TierMedium
	TierHigh
	TierCritical
)

// String renders the tier as the lowercase token used on the wire and CLI.
func (t Tier) String() string {
	switch t {
	case TierNoise:
		return "noise"
	case TierLow:
		return "low"
	case TierMedium:
		return "medium"
	case TierHigh:
		return "high"
	case TierCritical:
		return "critical"
	default:
		return fmt.Sprintf("tier(%d)", int(t))
	}
}

// ParseTier accepts the string form (case-insensitive). Empty string maps
// to TierNoise so callers can use a zero-value config field as "unset".
func ParseTier(s string) (Tier, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "noise":
		return TierNoise, nil
	case "low":
		return TierLow, nil
	case "medium", "med":
		return TierMedium, nil
	case "high":
		return TierHigh, nil
	case "critical", "crit":
		return TierCritical, nil
	}
	return TierNoise, fmt.Errorf("salience: unknown tier %q", s)
}

// MarshalJSON encodes Tier as the lowercase string. Numeric encoding would
// be brittle: re-ordering the iota block would silently rewrite history in
// any persisted JSONL.
func (t Tier) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

// UnmarshalJSON accepts both the string form and a JSON number, the
// latter only as a transitional convenience.
func (t *Tier) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		v, err := ParseTier(s)
		if err != nil {
			return err
		}
		*t = v
		return nil
	}
	var n int
	if err := json.Unmarshal(data, &n); err != nil {
		return err
	}
	*t = Tier(n)
	return nil
}

// Kind enumerates the drift-signal categories the salience layer knows
// about. Values are stable strings so JSONL captured today still parses
// after future kinds are added.
type Kind string

// Drift-signal kinds shipped today. New kinds need a DefaultClassifier
// entry and a Validate case.
const (
	KindTemplateSkip   Kind = "template_skip"
	KindTestFailure    Kind = "test_failure"
	KindBuildFailure   Kind = "build_failure"
	KindLintFailure    Kind = "lint_failure"
	KindDependencyBump Kind = "dependency_bump"
	KindCosmetic       Kind = "cosmetic"
	KindNaming         Kind = "naming"
	KindFormatting     Kind = "formatting"
	KindDocOnly        Kind = "doc_only"
)

// AllKinds returns every shipped Kind in declaration order.
func AllKinds() []Kind {
	return []Kind{
		KindTemplateSkip,
		KindTestFailure,
		KindBuildFailure,
		KindLintFailure,
		KindDependencyBump,
		KindCosmetic,
		KindNaming,
		KindFormatting,
		KindDocOnly,
	}
}

// Validate rejects unknown Kind values. Used at the JSONL boundary so a
// typo in upstream telemetry surfaces immediately instead of silently
// classifying as TierNoise.
func (k Kind) Validate() error {
	switch k {
	case KindTemplateSkip, KindTestFailure, KindBuildFailure,
		KindLintFailure, KindDependencyBump, KindCosmetic,
		KindNaming, KindFormatting, KindDocOnly:
		return nil
	}
	return fmt.Errorf("salience: unknown kind %q", string(k))
}

// Signal is one drift observation. Salience is optional on the wire: when
// zero (TierNoise) and the Kind is known, the Aggregator fills it from the
// Classifier. A caller can override by setting Salience explicitly to any
// non-noise tier.
type Signal struct {
	ID         string    `json:"id,omitempty"`
	Kind       Kind      `json:"kind"`
	Subject    string    `json:"subject,omitempty"`
	Salience   Tier      `json:"salience,omitempty"`
	Source     string    `json:"source,omitempty"`
	Note       string    `json:"note,omitempty"`
	ObservedAt time.Time `json:"observedAt,omitempty"`
}

// Validate checks the invariants every signal must satisfy before
// classification: known Kind. Subject and ObservedAt are optional (the
// aggregator stamps ObservedAt when zero) because not every drift source
// carries a meaningful subject.
func (s Signal) Validate() error {
	return s.Kind.Validate()
}
