package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/hippocampus"
)

var dailyLogCmd = &cobra.Command{
	Use:   "daily-log",
	Short: "Manage daily activity logs",
	Long:  "View and manage KAIROS-lite daily log files that track commands, decisions, and artifacts.",
}

var dailyLogShowCmd = &cobra.Command{
	Use:   "show [date]",
	Short: "Show daily log for a date (default: today)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := hippocampus.NewDailyLogger("")

		date := time.Now()
		if len(args) > 0 {
			var err error
			date, err = time.Parse("2006-01-02", args[0])
			if err != nil {
				return fmt.Errorf("invalid date %q (use YYYY-MM-DD): %w", args[0], err)
			}
		}

		content, err := logger.ReadDailyLog(date)
		if err != nil {
			return fmt.Errorf("read daily log: %w", err)
		}
		fmt.Print(content)
		return nil
	},
}

var dailyLogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all daily log files",
	RunE: func(cmd *cobra.Command, args []string) error {
		logger := hippocampus.NewDailyLogger("")

		files, err := logger.ListLogFiles()
		if err != nil {
			return fmt.Errorf("list log files: %w", err)
		}

		if len(files) == 0 {
			fmt.Println("No daily logs found.")
			return nil
		}

		for _, f := range files {
			fmt.Println(f)
		}
		return nil
	},
}

func init() {
	dailyLogCmd.AddCommand(dailyLogShowCmd)
	dailyLogCmd.AddCommand(dailyLogListCmd)
	rootCmd.AddCommand(dailyLogCmd)
}
