package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

var sessionGetCmd = &cobra.Command{
	Use:   "get <identifier>",
	Short: "Get detailed metadata for a session",
	Long: `Get detailed metadata for a single session by ID, name, or UUID prefix.

Returns comprehensive session information including status, harness, model,
context usage, and all metadata fields.

Output formats:
  • text (default) - Human-readable summary
  • json (-o json)  - Machine-readable JSON with optional --fields mask

Examples:
  agm session get my-session           # Get session by name
  agm session get session-abc123       # Get session by ID
  agm session get abc123               # Get session by UUID prefix
  agm session get my-session -o json   # JSON output
  agm session get my-session -o json --fields id,name,status  # Field mask`,
	Args: cobra.ExactArgs(1),
	RunE: runSessionGet,
}

func init() {
	sessionCmd.AddCommand(sessionGetCmd)
}

func runSessionGet(cmd *cobra.Command, args []string) error {
	identifier := args[0]

	// Construct OpContext with storage
	opCtx, cleanup, err := newOpContextWithStorage()
	if err != nil {
		return fmt.Errorf("failed to connect to Dolt storage: %w", err)
	}
	defer cleanup()

	// Call ops layer
	result, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
		Identifier: identifier,
	})
	if opErr != nil {
		return handleError(opErr)
	}

	// Output based on format
	return printResult(result, func() {
		s := result.Session
		fmt.Printf("Session: %s\n", ui.Bold(s.Name))
		fmt.Printf("  ID:        %s\n", s.ID)
		fmt.Printf("  Status:    %s\n", s.Status)
		fmt.Printf("  State:     %s\n", s.State)
		fmt.Printf("  Harness:   %s\n", s.Harness)
		if s.Model != "" {
			fmt.Printf("  Model:     %s\n", s.Model)
		}
		fmt.Printf("  Project:   %s\n", s.Project)
		if s.Purpose != "" {
			fmt.Printf("  Purpose:   %s\n", s.Purpose)
		}
		if len(s.Tags) > 0 {
			fmt.Printf("  Tags:      %v\n", s.Tags)
		}
		fmt.Printf("  Tmux:      %s\n", s.TmuxSession)
		if s.ClaudeUUID != "" {
			fmt.Printf("  UUID:      %s\n", s.ClaudeUUID)
		}
		if s.ParentSessionID != "" {
			fmt.Printf("  Parent:    %s\n", s.ParentSessionID)
		}
		fmt.Printf("  Workspace: %s\n", s.Workspace)
		fmt.Printf("  Lifecycle: %s\n", s.Lifecycle)
		fmt.Printf("  Created:   %s\n", s.CreatedAt)
		fmt.Printf("  Updated:   %s\n", s.UpdatedAt)
		if s.ContextUsage != nil {
			fmt.Printf("  Context:   %d/%d tokens (%.1f%%)\n",
				s.ContextUsage.UsedTokens, s.ContextUsage.TotalTokens, s.ContextUsage.PercentageUsed)
		}
		if s.PermissionMode != "" {
			fmt.Printf("  Permission: %s\n", s.PermissionMode)
		}
	})
}
