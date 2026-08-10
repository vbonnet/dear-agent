package router

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// QuotaReader reads provider usage and quota state for routing policy.
// Implementations must not expose credentials or raw provider logs in errors.
type QuotaReader interface {
	ReadQuota(ctx context.Context) (*QuotaSnapshot, error)
}

// QuotaSnapshot is the provider-neutral quota view consumed by router policy.
type QuotaSnapshot struct {
	Source    string
	Generated time.Time
	Providers []ProviderQuota
}

// ProviderQuota summarizes one provider family, such as anthropic, openai, or
// gemini. Windows carries the provider's own reset windows when available.
type ProviderQuota struct {
	Family  string
	Account string
	Windows []QuotaWindow
}

// QuotaWindow is one usage window reported by a provider or local usage source.
type QuotaWindow struct {
	Name             string
	RemainingPercent float64
	UsedPercent      float64
	ResetAt          time.Time
}

// QuotaPolicy controls how quota state influences routing.
type QuotaPolicy struct {
	// AvoidBelowRemainingPercent marks a provider unavailable while another
	// candidate is viable. Zero disables avoidance.
	AvoidBelowRemainingPercent float64

	// DeprioritizeBelowRemainingPercent marks a provider usable but expensive
	// in candidate ordering. Zero disables deprioritization.
	DeprioritizeBelowRemainingPercent float64

	// MaxSnapshotAge rejects stale snapshots. Zero accepts any age.
	MaxSnapshotAge time.Duration
}

// QuotaDecision is the policy result for one provider family.
type QuotaDecision struct {
	Family        string
	Avoid         bool
	Deprioritize  bool
	Reason        string
	MinRemaining  float64
	SnapshotStale bool
}

// EvaluateProviderQuota reduces a quota snapshot to a per-provider routing
// decision. The most constrained active window wins.
func EvaluateProviderQuota(snapshot *QuotaSnapshot, family string, now time.Time, policy QuotaPolicy) QuotaDecision {
	decision := QuotaDecision{Family: family}
	if snapshot == nil {
		decision.Reason = "quota snapshot unavailable"
		return decision
	}
	if policy.MaxSnapshotAge > 0 && !snapshot.Generated.IsZero() && now.Sub(snapshot.Generated) > policy.MaxSnapshotAge {
		decision.SnapshotStale = true
		decision.Reason = "quota snapshot stale"
		return decision
	}

	found := false
	minRemaining := 100.0
	for _, provider := range snapshot.Providers {
		if provider.Family != family {
			continue
		}
		for _, window := range provider.Windows {
			found = true
			if window.RemainingPercent < minRemaining {
				minRemaining = window.RemainingPercent
			}
		}
	}
	if !found {
		decision.Reason = "provider quota unavailable"
		return decision
	}

	decision.MinRemaining = minRemaining
	switch {
	case policy.AvoidBelowRemainingPercent > 0 && minRemaining <= policy.AvoidBelowRemainingPercent:
		decision.Avoid = true
		decision.Reason = fmt.Sprintf("remaining quota %.1f%% at or below avoid threshold %.1f%%", minRemaining, policy.AvoidBelowRemainingPercent)
	case policy.DeprioritizeBelowRemainingPercent > 0 && minRemaining <= policy.DeprioritizeBelowRemainingPercent:
		decision.Deprioritize = true
		decision.Reason = fmt.Sprintf("remaining quota %.1f%% at or below deprioritize threshold %.1f%%", minRemaining, policy.DeprioritizeBelowRemainingPercent)
	default:
		decision.Reason = "quota available"
	}
	return decision
}

// CommandRunner is the execution seam used by CodexBarQuotaReader tests.
type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type execCommandRunner struct{}

func (execCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// CodexBarQuotaReader reads CodexBar's redacted dashboard JSON. It depends only
// on the codexbar CLI and does not read provider credentials directly.
type CodexBarQuotaReader struct {
	Command string
	Runner  CommandRunner
	Timeout time.Duration
}

// ReadQuota shells out to `codexbar dashboard --identity redacted` and parses
// the provider quota windows CodexBar exposes.
func (r CodexBarQuotaReader) ReadQuota(ctx context.Context) (*QuotaSnapshot, error) {
	command := r.Command
	if command == "" {
		command = "codexbar"
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = 2 * time.Second
	}
	runner := r.Runner
	if runner == nil {
		runner = execCommandRunner{}
	}

	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	output, err := runner.Output(callCtx, command, "dashboard", "--identity", "redacted")
	if err != nil {
		return nil, fmt.Errorf("codexbar quota: read dashboard: %w", err)
	}
	snapshot, err := ParseCodexBarDashboard(output, time.Now())
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// ParseCodexBarDashboard parses CodexBar dashboard v1 JSON into the generic
// quota snapshot used by router policy.
func ParseCodexBarDashboard(data []byte, fallbackGenerated time.Time) (*QuotaSnapshot, error) {
	var payload struct {
		GeneratedAt string `json:"generatedAt"`
		Providers   []struct {
			ID      string `json:"id"`
			Family  string `json:"family"`
			Account string `json:"account"`
			Windows []struct {
				Name             string  `json:"name"`
				RemainingPercent float64 `json:"remainingPercent"`
				UsedPercent      float64 `json:"usedPercent"`
				ResetAt          string  `json:"resetAt"`
			} `json:"windows"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("codexbar quota: parse dashboard json: %w", err)
	}

	generated := fallbackGenerated
	if payload.GeneratedAt != "" {
		parsed, err := time.Parse(time.RFC3339, payload.GeneratedAt)
		if err != nil {
			return nil, fmt.Errorf("codexbar quota: parse generatedAt: %w", err)
		}
		generated = parsed
	}

	snapshot := &QuotaSnapshot{
		Source:    "codexbar",
		Generated: generated,
		Providers: make([]ProviderQuota, 0, len(payload.Providers)),
	}
	for _, provider := range payload.Providers {
		family := provider.Family
		if family == "" {
			family = provider.ID
		}
		quota := ProviderQuota{
			Family:  family,
			Account: provider.Account,
			Windows: make([]QuotaWindow, 0, len(provider.Windows)),
		}
		for _, window := range provider.Windows {
			var resetAt time.Time
			if window.ResetAt != "" {
				parsed, err := time.Parse(time.RFC3339, window.ResetAt)
				if err != nil {
					return nil, fmt.Errorf("codexbar quota: parse resetAt for %s/%s: %w", family, window.Name, err)
				}
				resetAt = parsed
			}
			quota.Windows = append(quota.Windows, QuotaWindow{
				Name:             window.Name,
				RemainingPercent: window.RemainingPercent,
				UsedPercent:      window.UsedPercent,
				ResetAt:          resetAt,
			})
		}
		snapshot.Providers = append(snapshot.Providers, quota)
	}
	return snapshot, nil
}
