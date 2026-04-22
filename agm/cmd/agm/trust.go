package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var (
	trustEventType   string
	trustEventDetail string
)

var trustCmd = &cobra.Command{
	Use:   "trust",
	Short: "Trust protocol — track agent session reliability",
	Args:  cobra.ArbitraryArgs,
	RunE:  groupRunE,
}

var trustScoreCmd = &cobra.Command{
	Use:   "score <session-name>",
	Short: "Show trust score for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrustScore,
}

var trustHistoryCmd = &cobra.Command{
	Use:   "history <session-name>",
	Short: "Show trust events for a session",
	Args:  cobra.ExactArgs(1),
	RunE:  runTrustHistory,
}

var trustRecordCmd = &cobra.Command{
	Use:   "record <session-name>",
	Short: "Record a trust event for a session",
	Long: `Record a trust event for a session. Event types:
  success            Task completed correctly (+5)
  false_completion   Claimed done but wasn't (-15)
  stall              Session stalled (-5)
  error_loop         Stuck in error loop (-3)
  permission_churn   Excessive permission prompts (-1)`,
	Args: cobra.ExactArgs(1),
	RunE: runTrustRecord,
}

var trustLeaderboardCmd = &cobra.Command{
	Use:   "leaderboard",
	Short: "Show all sessions ranked by trust score",
	Args:  cobra.NoArgs,
	RunE:  runTrustLeaderboard,
}

func init() {
	trustRecordCmd.Flags().StringVar(&trustEventType, "event", "", "Event type (required): success, false_completion, stall, error_loop, permission_churn")
	trustRecordCmd.Flags().StringVar(&trustEventDetail, "detail", "", "Optional detail message")
	_ = trustRecordCmd.MarkFlagRequired("event")

	trustCmd.AddCommand(trustScoreCmd)
	trustCmd.AddCommand(trustHistoryCmd)
	trustCmd.AddCommand(trustRecordCmd)
	trustCmd.AddCommand(trustLeaderboardCmd)
	rootCmd.AddCommand(trustCmd)
}

func runTrustScore(_ *cobra.Command, args []string) error {
	opCtx := newOpContext()

	result, err := ops.TrustScore(opCtx, &ops.TrustScoreRequest{
		SessionName: args[0],
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printTrustScoreText(result)
	})
}

func runTrustHistory(_ *cobra.Command, args []string) error {
	opCtx := newOpContext()

	result, err := ops.TrustHistory(opCtx, &ops.TrustHistoryRequest{
		SessionName: args[0],
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printTrustHistoryText(result)
	})
}

func runTrustRecord(_ *cobra.Command, args []string) error {
	opCtx := newOpContext()

	result, err := ops.TrustRecord(opCtx, &ops.TrustRecordRequest{
		SessionName: args[0],
		EventType:   trustEventType,
		Detail:      trustEventDetail,
	})
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		fmt.Printf("Recorded %s event for %s\n", result.Event.EventType, result.Event.SessionName)
		if result.Event.Detail != "" {
			fmt.Printf("  Detail: %s\n", result.Event.Detail)
		}
	})
}

func runTrustLeaderboard(_ *cobra.Command, _ []string) error {
	opCtx := newOpContext()

	result, err := ops.TrustLeaderboard(opCtx)
	if err != nil {
		return handleError(err)
	}

	return printResult(result, func() {
		printTrustLeaderboardText(result)
	})
}

func printTrustScoreText(r *ops.TrustScoreResult) {
	fmt.Printf("Trust Score: %s — %d/100\n\n", r.SessionName, r.Score)

	if len(r.Breakdown) == 0 {
		fmt.Println("  No events recorded yet.")
		return
	}

	fmt.Println("  Breakdown:")
	for _, b := range r.Breakdown {
		sign := "+"
		if b.Delta < 0 {
			sign = ""
		}
		fmt.Printf("    %-20s  %d event(s)  %s%d\n", b.EventType, b.Count, sign, b.Delta)
	}
	fmt.Printf("\n  Total events: %d\n", r.TotalEvents)
}

func printTrustHistoryText(r *ops.TrustHistoryResult) {
	fmt.Printf("Trust History: %s (%d events)\n\n", r.SessionName, r.Total)

	if len(r.Events) == 0 {
		fmt.Println("  No events recorded yet.")
		return
	}

	for _, e := range r.Events {
		ts := e.Timestamp.Format("2006-01-02 15:04:05")
		detail := ""
		if e.Detail != "" {
			detail = " — " + e.Detail
		}
		fmt.Printf("  [%s] %s%s\n", ts, e.EventType, detail)
	}
}

func printTrustLeaderboardText(r *ops.TrustLeaderboardResult) {
	if len(r.Entries) == 0 {
		fmt.Println("No sessions with trust data.")
		return
	}

	fmt.Printf("%-30s  %5s  %6s\n", "SESSION", "SCORE", "EVENTS")
	fmt.Println(strings.Repeat("-", 45))
	for _, e := range r.Entries {
		fmt.Printf("%-30s  %5d  %6d\n", e.SessionName, e.Score, e.TotalEvents)
	}
}
