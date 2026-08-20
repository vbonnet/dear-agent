package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

var alertsListLimit int
var alertsListStatus string

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Inspect AGM alert routing history",
}

var alertsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent routed alerts",
	Long: `List recent routed alerts.

--status filters before --limit is applied, so a queued alert stays findable
however many dispatched completions were recorded after it.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		var (
			records []ops.AlertRecord
			err     error
		)
		if status := strings.TrimSpace(alertsListStatus); status != "" {
			records, err = ops.ReadAlertRecordsWithStatus(
				ops.DefaultAlertQueuePath(), alertsListLimit, ops.AlertStatus(status))
		} else {
			records, err = ops.ReadAlertRecords(ops.DefaultAlertQueuePath(), alertsListLimit)
		}
		if err != nil {
			return err
		}
		// Encode an empty result as [] rather than null: this is a list
		// endpoint, and a consumer iterating the output should not have to
		// special-case "no alerts" as a different JSON type.
		if records == nil {
			records = []ops.AlertRecord{}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(records); err != nil {
			return fmt.Errorf("encode alerts: %w", err)
		}
		return nil
	},
}

// alertsDrainCmd exists because persistence is not delivery: an alert
// recorded as queued reached nobody, and nothing else in AGM reads the queue.
var alertsDrainCmd = &cobra.Command{
	Use:   "drain",
	Short: "Re-attempt delivery for alerts still recorded as queued",
	Long: `Re-attempt delivery for alerts still recorded as queued.

An alert raised while no supervisor was reachable is recorded rather than
lost, but recording it is not delivering it. This re-routes every alert whose
most recent record is still queued, and reports how many reached a recipient.
The watcher runs the same drain on each scan, so this command is for
operating on a host with no watcher running.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		opCtx, cleanup, err := newOpContextWithStorage()
		if err != nil {
			return handleError(err)
		}
		defer cleanup()
		delivered, err := ops.NewAlertRouter(opCtx).DrainQueued(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"operation": "alerts_drain",
			"delivered": delivered,
		})
	},
}

func init() {
	alertsListCmd.Flags().IntVar(&alertsListLimit, "limit", 50, "maximum alerts to show")
	alertsListCmd.Flags().StringVar(&alertsListStatus, "status", "",
		"filter by status (queued, dispatched, paged_human, quiet, suppressed); applied before --limit")
	alertsCmd.AddCommand(alertsListCmd)
	alertsCmd.AddCommand(alertsDrainCmd)
	rootCmd.AddCommand(alertsCmd)
}
