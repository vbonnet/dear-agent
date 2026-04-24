package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

var (
	updateNamespace     string
	updateMemoryID      string
	updateSetContent    string
	updateAppendContent string
	updateSetMetadata   string
	updateSetImportance float64
	updateSetType       string
	updateFormat        string
	updateHasImportance bool
)

var memoryUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update an existing memory entry",
	Long: `Update an existing memory entry with partial modifications.

Only the fields specified are updated. This enables selective updates without
having to read and replace the entire memory.

REQUIRED FLAGS
  --namespace     Namespace path (comma-separated, e.g., "user,alice,project")
  --memory-id     Memory ID to update

OPTIONAL FLAGS (at least one required)
  --set-content      Replace entire content
  --append-content   Append to existing content (string content only)
  --set-metadata     Update metadata (JSON object, merges with existing)
  --set-importance   Update importance score (0-1)
  --set-type         Change memory type (episodic|semantic|procedural|working)
  --format           Output format (json|text) (default: text)

EXAMPLES
  # Append to content
  $ engram memory update \
      --namespace user,alice \
      --memory-id mem-123 \
      --append-content " - Review complete"

  # Update metadata and importance
  $ engram memory update \
      --namespace user,alice \
      --memory-id mem-123 \
      --set-metadata '{"reviewed":true,"status":"done"}' \
      --set-importance 0.95

  # Replace content
  $ engram memory update \
      --namespace user,alice \
      --memory-id mem-123 \
      --set-content "New content replacing old content"

  # Change memory type
  $ engram memory update \
      --namespace user,alice \
      --memory-id mem-123 \
      --set-type semantic`,
	RunE: runMemoryUpdate,
}

func init() {
	memoryCmd.AddCommand(memoryUpdateCmd)

	// Required flags
	memoryUpdateCmd.Flags().StringVar(&updateNamespace, "namespace", "", "Namespace path (comma-separated)")
	memoryUpdateCmd.Flags().StringVar(&updateMemoryID, "memory-id", "", "Memory ID to update")
	memoryUpdateCmd.MarkFlagRequired("namespace")
	memoryUpdateCmd.MarkFlagRequired("memory-id")

	// Optional update flags
	memoryUpdateCmd.Flags().StringVar(&updateSetContent, "set-content", "", "Replace entire content")
	memoryUpdateCmd.Flags().StringVar(&updateAppendContent, "append-content", "", "Append to existing content")
	memoryUpdateCmd.Flags().StringVar(&updateSetMetadata, "set-metadata", "", "Update metadata (JSON object)")
	memoryUpdateCmd.Flags().Float64Var(&updateSetImportance, "set-importance", 0, "Update importance score (0-1)")
	memoryUpdateCmd.Flags().StringVar(&updateSetType, "set-type", "", "Change memory type")
	memoryUpdateCmd.Flags().StringVarP(&updateFormat, "format", "f", "text", "Output format (json|text)")

	// Track whether importance flag was set (to distinguish 0 from not-set)
	memoryUpdateCmd.Flags().Lookup("set-importance").Changed = false
}

// runMemoryUpdate executes the memory update command.
//
// Performs partial updates to an existing memory without replacing the entire entry.
// Supports updating content (replace or append), metadata (merge), importance, and type.
// At least one update field must be specified.
func runMemoryUpdate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Validate inputs
	namespace, err := validateUpdateInputs(cmd)
	if err != nil {
		return err
	}

	// 2. Build MemoryUpdate
	updates, err := buildMemoryUpdate(cmd)
	if err != nil {
		return err
	}

	// 3. Load provider and execute update
	provider, err := loadMemoryProvider(ctx)
	if err != nil {
		return err
	}
	defer provider.Close(ctx)

	if err := provider.UpdateMemory(ctx, namespace, updateMemoryID, updates); err != nil {
		return fmt.Errorf("failed to update memory: %w", err)
	}

	// 4. Format output
	return formatUpdateOutput(updateMemoryID, namespace, updates, updateFormat)
}

// validateUpdateInputs validates all input flags for memory update
func validateUpdateInputs(cmd *cobra.Command) ([]string, error) {
	if err := cli.ValidateNamespace(updateNamespace); err != nil {
		return nil, err
	}
	namespace := parseNamespace(updateNamespace)

	if err := cli.ValidateNonEmpty("memory-id", updateMemoryID); err != nil {
		return nil, err
	}

	if err := cli.ValidateOutputFormat(updateFormat, cli.FormatJSON, cli.FormatText); err != nil {
		return nil, err
	}

	// Validate at least one update field is provided
	updateHasImportance = cmd.Flags().Changed("set-importance")
	updateFields := map[string]string{
		"set-content":    updateSetContent,
		"append-content": updateAppendContent,
		"set-metadata":   updateSetMetadata,
		"set-type":       updateSetType,
	}
	if updateHasImportance {
		updateFields["set-importance"] = "set"
	}

	if err := cli.ValidateAtLeastOne(updateFields, "update field"); err != nil {
		return nil, err
	}

	return namespace, nil
}

// buildMemoryUpdate constructs MemoryUpdate from command flags
func buildMemoryUpdate(cmd *cobra.Command) (consolidation.MemoryUpdate, error) {
	updates := consolidation.MemoryUpdate{}

	// Set content replacement
	if updateSetContent != "" {
		var content interface{} = updateSetContent
		updates.SetContent = &content
	}

	// Set content append
	if updateAppendContent != "" {
		updates.AppendContent = &updateAppendContent
	}

	// Set metadata
	if updateSetMetadata != "" {
		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(updateSetMetadata), &metadata); err != nil {
			return updates, fmt.Errorf("invalid metadata JSON: %w", err)
		}
		updates.SetMetadata = metadata
	}

	// Set importance
	if cmd.Flags().Changed("set-importance") {
		if err := cli.ValidateRange("set-importance", updateSetImportance, 0, 1); err != nil {
			return updates, err
		}
		updates.SetImportance = &updateSetImportance
	}

	// Set type
	if updateSetType != "" {
		t := consolidation.MemoryType(updateSetType)
		if err := validateMemoryType(t); err != nil {
			return updates, err
		}
		updates.SetType = &t
	}

	return updates, nil
}

// formatUpdateOutput formats and prints the update command result.
//
// Supports two output formats:
//   - "json": Object with memory_id, namespace, and updates fields
//   - "text": Human-readable summary listing which fields were updated
//
// Returns error if format is invalid.
func formatUpdateOutput(memoryID string, namespace []string, updates consolidation.MemoryUpdate, format string) error {
	switch format {
	case "json":
		output := map[string]interface{}{
			"memory_id": memoryID,
			"namespace": namespace,
			"updates":   updates,
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))

	case "text":
		fmt.Printf("✓ Memory updated successfully\n")
		fmt.Printf("  ID:        %s\n", memoryID)
		fmt.Printf("  Namespace: %s\n", strings.Join(namespace, " > "))
		fmt.Printf("  Updates:\n")

		if updates.SetContent != nil {
			fmt.Printf("    - Content replaced\n")
		}
		if updates.AppendContent != nil {
			fmt.Printf("    - Content appended: %q\n", *updates.AppendContent)
		}
		if updates.SetMetadata != nil {
			metaJSON, _ := json.Marshal(updates.SetMetadata)
			fmt.Printf("    - Metadata updated: %s\n", string(metaJSON))
		}
		if updates.SetImportance != nil {
			fmt.Printf("    - Importance set to: %.2f\n", *updates.SetImportance)
		}
		if updates.SetType != nil {
			fmt.Printf("    - Type changed to: %s\n", *updates.SetType)
		}

	default:
		return fmt.Errorf("invalid format: %s (use json|text)", format)
	}

	return nil
}
