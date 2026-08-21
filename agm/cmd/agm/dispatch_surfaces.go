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

// skipRootInit is the no-op pair that keeps a command from inheriting the
// root's PersistentPreRunE/PostRunE (DB open, health check, workspace
// detection, audit logging).
//
// Every command in this file reads or writes one plain file under the
// user's home directory and touches nothing else. They exist so Dispatch
// can inspect quota and retarget completion relay precisely when the
// configured workspace or Dolt store is unavailable, so making them fail
// on configuration load would remove the surface exactly when it is
// needed. The shell-completion subtree opts out the same way.
func skipRootInit(cmd *cobra.Command) *cobra.Command {
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error { return nil }
	cmd.PersistentPostRunE = func(*cobra.Command, []string) error { return nil }
	return cmd
}

// quotaCmd predates ADR-038's guardrail (docs/adr/ADR-038-codexbar-quota-routing.md)
// and reads the same published state file `agm quota-meter` does, but
// with its own staleness threshold, default provider, and JSON schema —
// a second, diverging way to ask the same question (codex review on
// #1218). It stays because agm_get_quota_status (agm/cmd/agm-mcp-server)
// is a stable MCP surface built on it and its callers were not audited as
// part of this change. Prefer `agm quota-meter` for anything new; this
// command is not the canonical interface.
var quotaCmd = skipRootInit(&cobra.Command{
	Use:   "quota",
	Short: "Read provider quota status captured by CodexBar (legacy — prefer 'agm quota-meter')",
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
})

var completionRelayTargetCmd = skipRootInit(&cobra.Command{
	Use:   "relay-target",
	Short: "Read or set the live completion relay target",
})

var completionRelayTargetGetCmd = skipRootInit(&cobra.Command{
	Use:   "get",
	Short: "Read the effective completion relay target",
	Long: `Read the effective completion relay target.

This reports the override a running watcher would prefer right now,
resolved through the same precedence the watcher uses: the live
relay-target state file, then AGM_COMPLETION_RELAY_TARGET.

A "fallback" source with an empty target does NOT mean completions go
nowhere. It means no override is set, so routing discovers a live
Dispatch/orchestrator/supervisor session at delivery time and, if none is
reachable, records the alert durably for retry. That recipient cannot be
named here, because it depends on which sessions are live when the
completion happens; read it with 'agm alerts list' after the fact.

A watcher started with an explicit --orchestrator prefers that session
instead of discovering one, and the live relay target still outranks it.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		result := dispatchstate.ResolveRelayTarget(home, "", os.Getenv)
		return json.NewEncoder(os.Stdout).Encode(result)
	},
})

var completionRelayTargetSetCmd = skipRootInit(&cobra.Command{
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
})

func init() {
	quotaCmd.Flags().BoolVar(&quotaJSON, "json", false, "Emit quota status as JSON")
	quotaCmd.Flags().BoolVar(&quotaCheck, "check", false, "Emit quota status for automated checks")
	quotaCmd.Flags().StringVar(&quotaProvider, "provider", "codex", "Provider quota to read")
	rootCmd.AddCommand(quotaCmd)

	completionRelayTargetCmd.AddCommand(completionRelayTargetGetCmd)
	completionRelayTargetCmd.AddCommand(completionRelayTargetSetCmd)
}
