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
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		records, err := ops.ReadAlertRecords(ops.DefaultAlertQueuePath(), alertsListLimit)
		if err != nil {
			return err
		}
		status := strings.TrimSpace(alertsListStatus)
		if status != "" {
			filtered := records[:0]
			for _, rec := range records {
				if string(rec.Status) == status {
					filtered = append(filtered, rec)
				}
			}
			records = filtered
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(records); err != nil {
			return fmt.Errorf("encode alerts: %w", err)
		}
		return nil
	},
}

func init() {
	alertsListCmd.Flags().IntVar(&alertsListLimit, "limit", 50, "maximum alerts to show")
	alertsListCmd.Flags().StringVar(&alertsListStatus, "status", "", "filter by status")
	alertsCmd.AddCommand(alertsListCmd)
	rootCmd.AddCommand(alertsCmd)
}
