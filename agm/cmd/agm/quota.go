package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/llm/quota"
)

var quotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Per-provider subscription quota, spend pace, and cost-guardrail state",
	Long: `Report how much subscription quota each provider has left, how fast it is
being consumed, and whether the cost guardrail is halting or throttling new
sessions on it.

This is the interface an orchestrator polls. It reads a published state file
by default, so it is a fast local read and safe to call often — refreshing
from the meter itself takes seconds and belongs on a schedule, not on a
dispatch decision.

  agm quota                    # human-readable table from the last reading
  agm quota --json             # the same reading as JSON, for orchestrators
  agm quota --refresh          # read the meter now and republish the file
  agm quota --family openai    # one provider
  agm quota --check            # exit non-zero if any provider is halted

Exit codes follow the standard agm taxonomy:
  0  read succeeded (with --check: no provider is halted)
  1  no usable reading, or the refresh failed
  4  state conflict — with --check, at least one provider's guardrail is open

Unreadable is never reported as exhausted. A provider whose credentials are
missing shows as unreadable with the reason, and the guardrail stays closed
for it — the meter can only stop work on evidence, never on its absence.`,
	Args: cobra.NoArgs,
	RunE: runQuota,
}

var (
	quotaJSON    bool
	quotaRefresh bool
	quotaFamily  string
	quotaCheck   bool
	quotaPath    string
	quotaMaxAge  time.Duration
	quotaCommand string
	quotaTimeout time.Duration
)

func init() {
	quotaCmd.Flags().BoolVar(&quotaJSON, "json", false, "emit JSON instead of a table")
	quotaCmd.Flags().BoolVar(&quotaRefresh, "refresh", false, "read the meter now and republish the state file")
	quotaCmd.Flags().StringVar(&quotaFamily, "family", "", "report only this provider family (anthropic, openai, gemini)")
	quotaCmd.Flags().BoolVar(&quotaCheck, "check", false, "exit 3 if any reported provider's guardrail is open")
	quotaCmd.Flags().StringVar(&quotaPath, "state-file", "", "published state file (default: $XDG_STATE_HOME or ~/.local/state/dear-agent/quota/latest.json)")
	quotaCmd.Flags().DurationVar(&quotaMaxAge, "max-age", quota.DefaultSpawnGateMaxAge, "warn when the published reading is older than this")
	quotaCmd.Flags().StringVar(&quotaCommand, "command", quota.DefaultCodexBarCommand, "meter executable used by --refresh")
	quotaCmd.Flags().DurationVar(&quotaTimeout, "timeout", quota.DefaultReadTimeout, "maximum time for one --refresh read")
	rootCmd.AddCommand(quotaCmd)
}

func runQuota(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	path, err := resolveQuotaStatePath()
	if err != nil {
		return err
	}

	state, err := loadOrRefreshQuotaState(cmd.Context(), path)
	if err != nil {
		return err
	}

	providers := state.Providers
	if quotaFamily != "" {
		entry, ok := state.Provider(strings.ToLower(quotaFamily))
		if !ok {
			return fmt.Errorf("no quota reading for provider family %q", quotaFamily)
		}
		providers = []quota.ProviderState{entry}
	}

	if quotaJSON {
		if err := emitQuotaJSON(state, providers, path); err != nil {
			return err
		}
	} else {
		writeQuotaTable(state, providers, path)
	}

	if quotaCheck {
		for _, p := range providers {
			if quota.BreakerState(p.BreakerState) == quota.BreakerOpen {
				// A halted provider is a state conflict, not a command
				// failure: the request was well-formed and the tool
				// worked, the provider is simply out of room. Reusing
				// the taxonomy lets a caller branch on $? == 4 the same
				// way it already does everywhere else in agm.
				return &exitError{
					code: ExitStateConflict,
					msg:  fmt.Sprintf("quota guardrail is open for %s: %s", p.Family, p.Reason),
				}
			}
		}
	}
	return nil
}

func resolveQuotaStatePath() (string, error) {
	if quotaPath != "" {
		return quotaPath, nil
	}
	return quota.DefaultStateFilePath()
}

func loadOrRefreshQuotaState(ctx context.Context, path string) (*quota.State, error) {
	if !quotaRefresh {
		state, err := quota.ReadStateFile(path)
		if err == nil {
			return state, nil
		}
		return nil, fmt.Errorf("%w\nRun 'agm quota --refresh' to publish a reading", err)
	}
	return refreshQuotaState(ctx, path)
}

// refreshQuotaState reads the meter and republishes the state file. This
// is the slow path — seconds, because the meter refreshes providers over
// the network — and is why every other consumer reads the file instead.
func refreshQuotaState(ctx context.Context, path string) (*quota.State, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	meter := quota.New(quota.Options{
		Reader:          quota.CodexBarReader{Command: quotaCommand, Timeout: quotaTimeout},
		RefreshInterval: -1,
	})
	snapshot, err := meter.Refresh(ctx)
	if err != nil {
		return nil, fmt.Errorf("refresh quota meter: %w", err)
	}
	breaker := quota.NewBreaker(meter, quota.BreakerPolicy{})
	state := quota.BuildState(snapshot, meter, breaker, time.Now())
	if err := quota.WriteStateFile(path, state); err != nil {
		return nil, err
	}
	return state, nil
}

func emitQuotaJSON(state *quota.State, providers []quota.ProviderState, path string) error {
	payload := struct {
		*quota.State
		StateFile  string                `json:"stateFile"`
		AgeSeconds float64               `json:"ageSeconds"`
		Stale      bool                  `json:"stale"`
		Providers  []quota.ProviderState `json:"providers"`
	}{
		State:      state,
		StateFile:  path,
		AgeSeconds: state.Age(time.Now()).Seconds(),
		Stale:      quotaMaxAge > 0 && state.Age(time.Now()) > quotaMaxAge,
		Providers:  providers,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode quota json: %w", err)
	}
	return nil
}

func writeQuotaTable(state *quota.State, providers []quota.ProviderState, path string) {
	age := state.Age(time.Now()).Round(time.Second)
	staleNote := ""
	if quotaMaxAge > 0 && state.Age(time.Now()) > quotaMaxAge {
		staleNote = fmt.Sprintf("  STALE (older than %s — run 'agm quota --refresh')", quotaMaxAge)
	}
	fmt.Printf("source: %s %s   generated: %s   age: %s%s\nstate file: %s\n\n",
		orDashQuota(state.Source), orDashQuota(state.SourceVersion),
		state.GeneratedAt.Format(time.RFC3339), age, staleNote, path)

	fmt.Printf("%-10s %-12s %-9s %-12s %s\n", "FAMILY", "SOURCE", "LEFT", "GUARDRAIL", "DETAIL")
	for _, p := range providers {
		left := "    —"
		if p.Readable {
			left = fmt.Sprintf("%5.1f%%", p.RemainingPercent)
		}
		fmt.Printf("%-10s %-12s %-9s %-12s %s\n", p.Family, p.SourceID, left, p.BreakerState, p.Reason)
		if p.Overspending && p.PaceSummary != "" {
			fmt.Printf("%-10s   burn: %s\n", "", p.PaceSummary)
		}
		for _, w := range p.Windows {
			reset := ""
			if w.ResetAt != nil {
				reset = "  resets " + w.ResetAt.Format(time.RFC3339)
			}
			fmt.Printf("%-10s   · %-28s %5.1f%% left (%.1f%% used)%s\n",
				"", windowLabelQuota(w), w.RemainingPercent, w.UsedPercent, reset)
		}
	}
}

func windowLabelQuota(w quota.WindowState) string {
	switch {
	case w.Label != "":
		return w.Label
	case w.ID != "":
		return w.ID
	default:
		return "usage window"
	}
}

func orDashQuota(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
