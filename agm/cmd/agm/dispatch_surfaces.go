package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dispatchstate"
)

var quotaJSON bool
var quotaCheck bool
var quotaProvider string

var quotaCmd = &cobra.Command{
	Use:   "quota",
	Short: "Read provider quota status captured by CodexBar",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		status := dispatchstate.ReadQuotaStatus(home, quotaProvider, time.Now())
		if quotaJSON || quotaCheck {
			return json.NewEncoder(os.Stdout).Encode(status)
		}
		if !status.Available {
			fmt.Printf("quota unavailable: %s\n", status.Reason)
			return nil
		}
		fmt.Printf("quota available: %s\n", status.Path)
		if status.Warning {
			fmt.Println("quota warning: true")
		}
		return nil
	},
}

var completionRelayTargetCmd = &cobra.Command{
	Use:   "relay-target",
	Short: "Read or set the live completion relay target",
}

var completionRelayTargetGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Read the live completion relay target",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(dispatchstate.ResolveRelayTarget(home, "", os.Getenv))
	},
}

var completionRelayTargetSetCmd = &cobra.Command{
	Use:   "set <session>",
	Short: "Set the live completion relay target session",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		result, err := dispatchstate.SetRelayTarget(home, args[0])
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	},
}

func init() {
	quotaCmd.Flags().BoolVar(&quotaJSON, "json", false, "Emit quota status as JSON")
	quotaCmd.Flags().BoolVar(&quotaCheck, "check", false, "Emit quota status for automated checks")
	quotaCmd.Flags().StringVar(&quotaProvider, "provider", "codex", "Provider quota to read")
	rootCmd.AddCommand(quotaCmd)

	completionRelayTargetCmd.AddCommand(completionRelayTargetGetCmd)
	completionRelayTargetCmd.AddCommand(completionRelayTargetSetCmd)
}
