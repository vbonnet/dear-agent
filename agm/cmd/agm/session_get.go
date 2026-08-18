package main

import (
	"encoding/json"
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

	if isJSONOutput() {
		return printSessionGetJSON(result)
	}

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

var getTopLevelFields = map[string]bool{
	"operation": true,
	"session":   true,
}

var getSessionFields = map[string]bool{
	"id":                true,
	"name":              true,
	"status":            true,
	"state":             true,
	"harness":           true,
	"model":             true,
	"project":           true,
	"purpose":           true,
	"tags":              true,
	"tmux_session":      true,
	"claude_uuid":       true,
	"parent_session_id": true,
	"workspace":         true,
	"working_directory": true,
	"lifecycle":         true,
	"created_at":        true,
	"updated_at":        true,
	"final_output_at":   true,
	"context_usage":     true,
	"permission_mode":   true,
	"harness_history":   true,
}

// printSessionGetJSON keeps `agm session get -o json --fields id,name,status`
// useful for agents. The global field-mask helper filters only top-level result
// objects; get accepts per-session detail fields too, so those masks are applied
// inside the stable `session` envelope instead of collapsing the result to `{}`.
func printSessionGetJSON(result *ops.GetSessionResult) error {
	if len(fieldsFlag) == 0 {
		return printJSON(result)
	}
	if response, ok := buildSessionGetFieldMaskResponse(result, fieldsFlag); ok {
		return printJSONNoFieldMask(response)
	}
	return printJSON(result)
}

func filterSessionDetailFields(s ops.SessionDetail, fields []string) map[string]any {
	source := map[string]any{}
	data, err := json.Marshal(s)
	if err == nil {
		_ = json.Unmarshal(data, &source)
	}

	row := map[string]any{}
	for _, f := range fields {
		if v, ok := source[f]; ok {
			row[f] = v
		}
	}
	return row
}

func buildSessionGetFieldMaskResponse(result *ops.GetSessionResult, fields []string) (map[string]any, bool) {
	response := map[string]any{}
	var sessionFields []string
	includeFullSession := false

	for _, f := range fields {
		switch {
		case f == "operation":
			response["operation"] = result.Operation
		case f == "session":
			includeFullSession = true
		case getSessionFields[f]:
			sessionFields = append(sessionFields, f)
		case getTopLevelFields[f]:
			// Known top-level field handled above.
		}
	}

	if len(sessionFields) > 0 {
		response["session"] = filterSessionDetailFields(result.Session, sessionFields)
	} else if includeFullSession {
		response["session"] = result.Session
	}

	if len(response) == 0 {
		return nil, false
	}
	return response, true
}
