package main

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/costtrack"
)

var (
	budgetWeekly   bool
	budgetSessions bool
)

var budgetCmd = &cobra.Command{
	Use:   "budget",
	Short: "Show current token and cost budget consumption",
	Long: `Show daily and weekly budget consumption with per-session breakdowns.

Budget enforcement is configured in ~/.config/agm/config.yaml:

  budget:
    enabled: true
    weekly_limit_usd: 50.00
    session_token_caps:
      claude-code: 500000    # 500K tokens per session
      codex-cli:   200000
    fallback_chain: [sonnet, haiku]

The daily budget uses carryover smoothing:
  day_budget = (weekly_limit / 7) + carryover_from_prior_days

Examples:
  agm budget              # Today's status
  agm budget --week       # Weekly day-by-day breakdown
  agm budget --sessions   # Per-session token consumption`,
	RunE: runBudget,
}

func init() {
	budgetCmd.Flags().BoolVar(&budgetWeekly, "week", false, "Show full weekly breakdown")
	budgetCmd.Flags().BoolVar(&budgetSessions, "sessions", false, "Show per-session token breakdown")
	rootCmd.AddCommand(budgetCmd)
}

func runBudget(_ *cobra.Command, _ []string) error {
	if cfg == nil || !cfg.Budget.Enabled {
		_, _ = fmt.Fprintln(os.Stderr, "Budget enforcement is disabled.")
		_, _ = fmt.Fprintln(os.Stderr, "Enable it in ~/.config/agm/config.yaml:")
		_, _ = fmt.Fprintln(os.Stderr, "  budget:")
		_, _ = fmt.Fprintln(os.Stderr, "    enabled: true")
		_, _ = fmt.Fprintln(os.Stderr, "    weekly_limit_usd: 50.00")
		return nil
	}

	stateFile := cfg.Budget.StateFile
	if stateFile == "" {
		homeDir, _ := os.UserHomeDir()
		stateFile = filepath.Join(homeDir, ".agm", "budget-state.json")
	}

	caps := costtrack.HarnessTokenCaps(maps.Clone(cfg.Budget.SessionTokenCaps))

	enforcer, err := costtrack.NewBudgetEnforcer(costtrack.EnforcerConfig{
		StateFilePath:    stateFile,
		WeeklyLimitUSD:   cfg.Budget.WeeklyLimitUSD,
		SessionTokenCaps: caps,
	})
	if err != nil {
		return fmt.Errorf("load budget state: %w", err)
	}

	now := time.Now()
	printBudgetReport(enforcer, now)
	return nil
}

func printBudgetReport(e *costtrack.BudgetEnforcer, now time.Time) {
	out := os.Stdout
	snap := e.WeeklySnap()
	daily := e.DailyStatus(now)
	weekly := e.WeeklyStatus()
	dayIdx := snap.DayIndex(now.UTC())

	_, _ = fmt.Fprintf(out, "Budget Status — %s (%s)\n", now.Format("2006-01-02"), now.Weekday())
	_, _ = fmt.Fprintln(out, strings.Repeat("─", 64))

	dailyStatus := budgetLabel(daily.Percent, daily.Allowed)
	_, _ = fmt.Fprintf(out, "Today    %s %5.1f%% (%s)  $%.2f / $%.2f\n",
		progressBar(daily.Percent, 28), daily.Percent, dailyStatus, daily.Spent, daily.Limit)

	weeklyStatus := budgetLabel(weekly.Percent, weekly.Allowed)
	_, _ = fmt.Fprintf(out, "Week     %s %5.1f%% (%s)  $%.2f / $%.2f\n",
		progressBar(weekly.Percent, 28), weekly.Percent, weeklyStatus, weekly.Spent, weekly.Limit)

	if budgetWeekly {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Daily Breakdown:")
		_, _ = fmt.Fprintln(out, strings.Repeat("─", 64))
		for i := range 7 {
			day := snap.WeekStart.AddDate(0, 0, i)
			spent := snap.DailySpend[i]
			prefix := "  "
			if i == dayIdx {
				prefix = "→ "
			}
			_, _ = fmt.Fprintf(out, "%sDay %d  %-12s  $%.2f\n",
				prefix, i+1, day.Format("Mon Jan 02"), spent)
		}
	}

	if budgetSessions {
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, "Session Token Usage:")
		_, _ = fmt.Fprintln(out, strings.Repeat("─", 64))
		sessions := e.AllSessions()
		if len(sessions) == 0 {
			_, _ = fmt.Fprintln(out, "  No session data recorded this week.")
		} else {
			_, _ = fmt.Fprintf(out, "%-36s %-14s %12s %12s\n", "SESSION ID", "HARNESS", "TOKENS", "CAP")
			for _, ss := range sessions {
				capStr := "unlimited"
				if ss.TokenCap > 0 {
					capStr = fmt.Sprintf("%d", ss.TokenCap)
				}
				shortID := ss.SessionID
				if runes := []rune(shortID); len(runes) > 35 {
					shortID = string(runes[:8]) + "…" + string(runes[len(runes)-8:])
				}
				_, _ = fmt.Fprintf(out, "%-36s %-14s %12d %12s\n",
					shortID, ss.Harness, ss.TokensUsed, capStr)
			}
		}
	}
}

func budgetLabel(pct float64, allowed bool) string {
	switch {
	case !allowed:
		return "EXHAUSTED"
	case pct >= 90:
		return "CRITICAL "
	case pct >= 75:
		return "WARNING  "
	case pct >= 50:
		return "ALERT    "
	default:
		return "OK       "
	}
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", width-filled) + "]"
}
