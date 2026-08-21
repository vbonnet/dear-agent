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

// quotaMeterCmd reads or writes only its own published state file under
// the user's home directory, and its --refresh path exists precisely so
// the scheduled launchd job (com.dear-agent.quota-refresh.plist) keeps
// working when repository/session initialization is unavailable — the
// same reason quotaCmd in dispatch_surfaces.go opts out via skipRootInit.
// Without this, an unrelated AGM configuration failure would stop every
// refresh until readings go stale and the guardrail fails open on data
// nobody can trust (codex review on #1218).
var quotaMeterCmd = skipRootInit(&cobra.Command{
	Use:   "quota-meter",
	Short: "Per-provider subscription quota, spend pace, and cost-guardrail state",
	Long: `Report how much subscription quota each provider has left, how fast it is
being consumed, and whether the cost guardrail is halting or throttling new
sessions on it.

This is the interface an orchestrator polls. It reads a published state file
by default, so it is a fast local read and safe to call often — refreshing
from the meter itself takes seconds and belongs on a schedule, not on a
dispatch decision.

  agm quota-meter                    # human-readable table from the last reading
  agm quota-meter --json             # the same reading as JSON, for orchestrators
  agm quota-meter --refresh          # read the meter now and republish the file
  agm quota-meter --family openai    # one provider
  agm quota-meter --check            # exit non-zero if any provider is halted

Exit codes follow the standard agm taxonomy:
  0  read succeeded (with --check: no provider is halted)
  1  no usable reading, or the refresh failed
  4  state conflict — with --check, at least one provider's guardrail is open

Unreadable is never reported as exhausted. A provider whose credentials are
missing shows as unreadable with the reason, and the guardrail stays closed
for it — the meter can only stop work on evidence, never on its absence.`,
	Args: cobra.NoArgs,
	RunE: runQuotaMeter,
})

var (
	quotaMeterJSON    bool
	quotaMeterRefresh bool
	quotaMeterFamily  string
	quotaMeterCheck   bool
	quotaMeterPath    string
	quotaMeterMaxAge  time.Duration
	quotaMeterCommand string
	quotaMeterTimeout time.Duration
)

func init() {
	quotaMeterCmd.Flags().BoolVar(&quotaMeterJSON, "json", false, "emit JSON instead of a table")
	quotaMeterCmd.Flags().BoolVar(&quotaMeterRefresh, "refresh", false, "read the meter now and republish the state file")
	quotaMeterCmd.Flags().StringVar(&quotaMeterFamily, "family", "", "report only this provider family (anthropic, openai, gemini)")
	quotaMeterCmd.Flags().BoolVar(&quotaMeterCheck, "check", false, "exit 4 if any reported provider's guardrail is open")
	quotaMeterCmd.Flags().StringVar(&quotaMeterPath, "state-file", "", "published state file (default: $XDG_STATE_HOME or ~/.local/state/dear-agent/quota/latest.json)")
	quotaMeterCmd.Flags().DurationVar(&quotaMeterMaxAge, "max-age", quota.DefaultSpawnGateMaxAge, "warn when the published reading is older than this")
	quotaMeterCmd.Flags().StringVar(&quotaMeterCommand, "command", quota.DefaultCodexBarCommand, "meter executable used by --refresh")
	quotaMeterCmd.Flags().DurationVar(&quotaMeterTimeout, "timeout", quota.DefaultReadTimeout, "maximum time for one --refresh read")
	rootCmd.AddCommand(quotaMeterCmd)
}

func runQuotaMeter(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true

	path, err := resolveQuotaMeterStatePath()
	if err != nil {
		return err
	}

	state, err := loadOrRefreshQuotaMeterState(cmd.Context(), path)
	if err != nil {
		return err
	}

	providers := state.Providers
	if quotaMeterFamily != "" {
		entry, ok := state.Provider(strings.ToLower(quotaMeterFamily))
		if !ok {
			return fmt.Errorf("no quota reading for provider family %q", quotaMeterFamily)
		}
		providers = []quota.ProviderState{entry}
	}

	if quotaMeterJSON {
		if err := emitQuotaMeterJSON(state, providers, path); err != nil {
			return err
		}
	} else {
		writeQuotaMeterTable(state, providers, path)
	}

	if quotaMeterCheck {
		return checkQuotaMeterState(state, providers, quotaMeterMaxAge, time.Now())
	}
	return nil
}

// checkQuotaMeterState is --check's verdict, factored out so it is
// testable without cobra or a real state file.
//
// A reading older than maxAge is unevaluated, not evidence that a
// provider is still halted. SpawnGate already treats a stale published
// state as "no usable reading" and fails open past this same age
// (DefaultSpawnGateMaxAge); --check must agree, or an orchestrator
// polling this command can keep admissions halted indefinitely off a
// BreakerState the refresh job stopped updating long ago, even though
// the live spawn gate has already recovered (codex review on #1218).
func checkQuotaMeterState(state *quota.State, providers []quota.ProviderState, maxAge time.Duration, now time.Time) error {
	if maxAge > 0 && state.Age(now) > maxAge {
		return nil
	}
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
	return nil
}

func resolveQuotaMeterStatePath() (string, error) {
	if quotaMeterPath != "" {
		return quotaMeterPath, nil
	}
	return quota.DefaultStateFilePath()
}

func loadOrRefreshQuotaMeterState(ctx context.Context, path string) (*quota.State, error) {
	if !quotaMeterRefresh {
		state, err := quota.ReadStateFile(path)
		if err == nil {
			return state, nil
		}
		return nil, fmt.Errorf("%w\nRun 'agm quota-meter --refresh' to publish a reading", err)
	}
	return refreshQuotaMeterState(ctx, path)
}

// refreshQuotaMeterState reads the meter and republishes the state file. This
// is the slow path — seconds, because the meter refreshes providers over
// the network — and is why every other consumer reads the file instead.
func refreshQuotaMeterState(ctx context.Context, path string) (*quota.State, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	meter := quota.New(quota.Options{
		Reader:          quota.CodexBarReader{Command: quotaMeterCommand, Timeout: quotaMeterTimeout},
		RefreshInterval: -1,
	})
	snapshot, err := meter.Refresh(ctx)
	if err != nil {
		return nil, fmt.Errorf("refresh quota meter: %w", err)
	}
	if err := requireAuditedCodexBarVersion(snapshot); err != nil {
		// The previous published state — which did pass this floor when
		// it was written — is left in place rather than overwritten, so
		// the guardrail degrades to "stale" instead of to "wrong".
		return nil, err
	}
	breaker := quota.NewBreaker(meter, quota.BreakerPolicy{})
	state := quota.BuildState(snapshot, meter, breaker, time.Now())
	if err := quota.WriteStateFile(path, state); err != nil {
		return nil, err
	}
	return state, nil
}

// requireAuditedCodexBarVersion refuses a snapshot from a CodexBar build
// below ADR-038's audited floor (engram-research #313: earlier builds
// carry a recorded SQLite cost-store defect). cmd/workflow-run refuses to
// route on an unaudited build via the same shared floor
// (quota.MeetsMinCodexBarVersion); this scheduled refresh must refuse to
// publish one too, or the launchd job launders an unaudited reading into
// guardrail verdicts every consumer trusts (codex review on #1218).
//
// An empty SourceVersion is deliberately NOT treated as "no evidence,
// don't gate" the way an absent reading is everywhere else in this
// package: MeetsMinCodexBarVersion already reports "" as below the
// floor, and that is the correct direction here specifically, because
// this check's job is the opposite of the package's usual one. Elsewhere,
// missing evidence must not let quota data block a spawn or demote a
// route — but here, missing evidence (a build too old, or too broken, to
// report its own version) must not be trusted to let quota data start
// gating anything at all. Silently accepting an unversioned snapshot
// would let exactly the unaudited build ADR-038 excludes start producing
// guardrail verdicts (codex review on #1218, second pass). A nil snapshot
// (defensive only; Refresh never returns one on success) has no version
// to check and is let through.
func requireAuditedCodexBarVersion(snapshot *quota.Snapshot) error {
	if snapshot == nil {
		return nil
	}
	if quota.MeetsMinCodexBarVersion(snapshot.SourceVersion) {
		return nil
	}
	version := snapshot.SourceVersion
	if version == "" {
		version = "(none reported)"
	}
	return fmt.Errorf("refuse to publish: codexbar %s is below the audited floor %s (ADR-038)",
		version, quota.MinAuditedCodexBarVersion)
}

func emitQuotaMeterJSON(state *quota.State, providers []quota.ProviderState, path string) error {
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
		Stale:      quotaMeterMaxAge > 0 && state.Age(time.Now()) > quotaMeterMaxAge,
		Providers:  providers,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("encode quota json: %w", err)
	}
	return nil
}

func writeQuotaMeterTable(state *quota.State, providers []quota.ProviderState, path string) {
	age := state.Age(time.Now()).Round(time.Second)
	staleNote := ""
	if quotaMeterMaxAge > 0 && state.Age(time.Now()) > quotaMeterMaxAge {
		staleNote = fmt.Sprintf("  STALE (older than %s — run 'agm quota-meter --refresh')", quotaMeterMaxAge)
	}
	fmt.Printf("source: %s %s   generated: %s   age: %s%s\nstate file: %s\n\n",
		orDashQuotaMeter(state.Source), orDashQuotaMeter(state.SourceVersion),
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
				"", windowLabelQuotaMeter(w), w.RemainingPercent, w.UsedPercent, reset)
		}
	}
}

func windowLabelQuotaMeter(w quota.WindowState) string {
	switch {
	case w.Label != "":
		return w.Label
	case w.ID != "":
		return w.ID
	default:
		return "usage window"
	}
}

func orDashQuotaMeter(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
