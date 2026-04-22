package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/internal/pricing"
)

var costCmd = &cobra.Command{
	Use:   "cost",
	Short: "Show token usage and cost report for sessions",
	Long: `Show aggregated token usage and cost data for all sessions.

Displays per-session:
  - Model used
  - Token counts (input/output)
  - Duration
  - Estimated cost (priced per-model from internal/pricing)

Examples:
  agm cost              # Show cost report for all active sessions
  agm cost --all        # Include archived sessions
  agm cost --budget 50  # Alert if total spend exceeds $50`,
	RunE: runCostReport,
}

var (
	costAll    bool
	costBudget float64
)

func init() {
	costCmd.Flags().BoolVar(&costAll, "all", false, "include archived sessions")
	costCmd.Flags().Float64Var(&costBudget, "budget", 0, "budget threshold in USD; alert if total spend exceeds this amount")
	rootCmd.AddCommand(costCmd)
}

func runCostReport(cmd *cobra.Command, _ []string) error {
	adapter, err := getStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to storage: %w", err)
	}
	defer adapter.Close()

	filter := &dolt.SessionFilter{
		ExcludeArchived: !costAll,
	}

	manifests, err := adapter.ListSessions(filter)
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(manifests) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No sessions found.")
		return nil
	}

	totalCost := printCostReport(cmd, manifests)

	if costBudget > 0 && totalCost > costBudget {
		fmt.Fprintf(cmd.ErrOrStderr(), "\n⚠ BUDGET ALERT: Total spend $%.2f exceeds budget $%.2f (%.0f%% over)\n",
			totalCost, costBudget, (totalCost-costBudget)/costBudget*100)
	}

	return nil
}

// sessionCostRow holds computed cost data for display.
type sessionCostRow struct {
	Name          string
	Model         string
	TokensIn      int64
	TokensOut     int64
	Duration      time.Duration
	EstimatedCost float64
	CostIsKnown   bool
	Commits       int
	CostPerCommit float64
}

// sessionModel picks the best available model identifier from a manifest.
// LastKnownModel is what the statusline actually observed; Model is the
// configured default. An empty result is surfaced as "?" in the report
// rather than silently defaulting to Opus pricing.
func sessionModel(m *manifest.Manifest) string {
	if m.LastKnownModel != "" {
		return m.LastKnownModel
	}
	return m.Model
}

func buildCostRow(m *manifest.Manifest) sessionCostRow {
	row := sessionCostRow{
		Name:  m.Name,
		Model: sessionModel(m),
	}

	if m.CostTracking != nil {
		row.TokensIn = m.CostTracking.TokensIn
		row.TokensOut = m.CostTracking.TokensOut
		if !m.CostTracking.StartTime.IsZero() && !m.CostTracking.EndTime.IsZero() {
			row.Duration = m.CostTracking.EndTime.Sub(m.CostTracking.StartTime)
		}
	}

	// Cost resolution:
	//   1. If tokens are present, price them per-model via the shared table.
	//   2. Otherwise fall back to LastKnownCost (statusline-reported).
	// Previously every row was priced at Opus rates. Now a Sonnet session
	// gets Sonnet pricing, a Haiku session gets Haiku pricing, etc.
	switch {
	case row.TokensIn > 0 || row.TokensOut > 0:
		row.EstimatedCost = estimateCost(row.Model, row.TokensIn, row.TokensOut)
		row.CostIsKnown = pricing.IsKnown(row.Model)
	case m.LastKnownCost > 0:
		row.EstimatedCost = m.LastKnownCost
		row.CostIsKnown = true
	}

	if row.Duration == 0 && !m.CreatedAt.IsZero() && !m.UpdatedAt.IsZero() {
		row.Duration = m.UpdatedAt.Sub(m.CreatedAt)
	}

	return row
}

// estimateCost prices a token pair using the shared pricing table.
// Unknown models return 0 (surfaced as "?" in the report).
func estimateCost(model string, tokensIn, tokensOut int64) float64 {
	return pricing.Estimate(model, tokensIn, tokensOut)
}

func costFormatDuration(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func costFormatTokens(n int64) string {
	if n == 0 {
		return "—"
	}
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1_000 {
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	}
	return fmt.Sprintf("%d", n)
}

func formatCost(c float64) string {
	if c == 0 {
		return "—"
	}
	return fmt.Sprintf("$%.2f", c)
}

// formatModelCell formats the model for display. Returns "?" for empty
// (unknown model), which signals to the reader that the cost column may be
// inaccurate rather than pretending we priced it correctly.
func formatModelCell(model string) string {
	if model == "" {
		return "?"
	}
	short := strings.ToLower(model)
	for _, alias := range []string{"opus", "sonnet", "haiku"} {
		if strings.Contains(short, alias) {
			return alias
		}
	}
	if len(model) > 10 {
		return model[:10]
	}
	return model
}

func printCostReport(cmd *cobra.Command, manifests []*manifest.Manifest) float64 {
	out := cmd.OutOrStdout()

	rows := make([]sessionCostRow, 0, len(manifests))
	var totalIn, totalOut int64
	var totalCost float64
	var unpricedSessions int

	for _, m := range manifests {
		row := buildCostRow(m)
		rows = append(rows, row)
		totalIn += row.TokensIn
		totalOut += row.TokensOut
		totalCost += row.EstimatedCost
		if (row.TokensIn > 0 || row.TokensOut > 0) && !row.CostIsKnown {
			unpricedSessions++
		}
	}

	// MODEL column lets readers verify which price tier was applied.
	fmt.Fprintf(out, "%-28s %-8s %10s %10s %8s %10s %8s %12s\n",
		"SESSION", "MODEL", "TOKENS IN", "TOKENS OUT", "DURATION", "EST. COST", "COMMITS", "COST/COMMIT")
	fmt.Fprintln(out, strings.Repeat("─", 108))

	for _, r := range rows {
		name := r.Name
		if len(name) > 27 {
			name = name[:24] + "..."
		}
		cpc := "—"
		if r.Commits > 0 && r.EstimatedCost > 0 {
			cpc = fmt.Sprintf("$%.2f", r.CostPerCommit)
		}

		fmt.Fprintf(out, "%-28s %-8s %10s %10s %8s %10s %8s %12s\n",
			name,
			formatModelCell(r.Model),
			costFormatTokens(r.TokensIn),
			costFormatTokens(r.TokensOut),
			costFormatDuration(r.Duration),
			formatCost(r.EstimatedCost),
			"—",
			cpc,
		)
	}

	fmt.Fprintln(out, strings.Repeat("─", 108))
	fmt.Fprintf(out, "%-28s %-8s %10s %10s %8s %10s\n",
		fmt.Sprintf("TOTAL (%d sessions)", len(rows)),
		"",
		costFormatTokens(totalIn),
		costFormatTokens(totalOut),
		"",
		formatCost(totalCost),
	)

	// Per-model price reference — only list models that actually appeared.
	shownPrices := map[string]bool{}
	fmt.Fprintln(out)
	for _, r := range rows {
		key := formatModelCell(r.Model)
		if shownPrices[key] || key == "?" {
			continue
		}
		p := pricing.Lookup(r.Model)
		if p == pricing.UnknownModel {
			continue
		}
		fmt.Fprintf(out, "Pricing: %-8s $%.2f/M input, $%.2f/M output\n",
			key, p.InputPerMillion, p.OutputPerMillion)
		shownPrices[key] = true
	}
	if unpricedSessions > 0 {
		fmt.Fprintf(out, "\nNote: %d session(s) had tokens but no known model — costs shown as $0.00\n",
			unpricedSessions)
	}

	return totalCost
}
