package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	deleteNamespace string
	deleteMemoryID  string
	deleteConfirm   bool
	deleteFormat    string
)

var memoryDeleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Delete a memory entry",
	Long: `Delete a memory entry from long-term storage.

This operation is irreversible. By default, you will be prompted to confirm
the deletion. Use --confirm to skip the confirmation prompt.

REQUIRED FLAGS
  --namespace     Namespace path (comma-separated, e.g., "user,alice,project")
  --memory-id     Memory ID to delete

OPTIONAL FLAGS
  --confirm       Skip confirmation prompt (dangerous!)
  --format        Output format (json|text) (default: text)

EXAMPLES
  # Delete with confirmation prompt
  $ engram memory delete \
      --namespace user,alice \
      --memory-id mem-123

  # Delete without confirmation (use with caution!)
  $ engram memory delete \
      --namespace user,alice \
      --memory-id mem-123 \
      --confirm

  # Delete and output JSON
  $ engram memory delete \
      --namespace user,alice \
      --memory-id mem-123 \
      --confirm \
      --format json`,
	RunE: runMemoryDelete,
}

func init() {
	memoryCmd.AddCommand(memoryDeleteCmd)

	// Required flags
	memoryDeleteCmd.Flags().StringVar(&deleteNamespace, "namespace", "", "Namespace path (comma-separated)")
	memoryDeleteCmd.Flags().StringVar(&deleteMemoryID, "memory-id", "", "Memory ID to delete")
	memoryDeleteCmd.MarkFlagRequired("namespace")
	memoryDeleteCmd.MarkFlagRequired("memory-id")

	// Optional flags
	memoryDeleteCmd.Flags().BoolVar(&deleteConfirm, "confirm", false, "Skip confirmation prompt")
	memoryDeleteCmd.Flags().StringVarP(&deleteFormat, "format", "f", "text", "Output format (json|text)")
}

// runMemoryDelete executes the memory delete command.
//
// Deletes a memory entry from long-term storage after confirming the operation
// (unless --confirm flag is used to bypass confirmation prompt). The deletion
// is permanent and cannot be undone.
func runMemoryDelete(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Parse and validate inputs
	namespace := parseNamespace(deleteNamespace)
	if len(namespace) == 0 {
		return fmt.Errorf("namespace cannot be empty")
	}

	if deleteMemoryID == "" {
		return fmt.Errorf("memory-id cannot be empty")
	}

	// 2. Confirmation prompt (unless --confirm flag is set)
	if !deleteConfirm {
		fmt.Printf("⚠️  WARNING: This will permanently delete memory '%s'\n", deleteMemoryID)
		fmt.Printf("   Namespace: %s\n", strings.Join(namespace, " > "))
		fmt.Print("\nAre you sure you want to continue? (yes/no): ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" && response != "y" {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	// 3. Load provider
	provider, err := loadMemoryProvider(ctx)
	if err != nil {
		return err
	}
	defer provider.Close(ctx)

	// 4. Delete memory
	if err := provider.DeleteMemory(ctx, namespace, deleteMemoryID); err != nil {
		return fmt.Errorf("failed to delete memory: %w", err)
	}

	// 5. Format output
	return formatDeleteOutput(deleteMemoryID, namespace, deleteFormat)
}

// formatDeleteOutput formats and prints the delete command result.
//
// Supports two output formats:
//   - "json": Object with memory_id, namespace, and deleted fields
//   - "text": Human-readable success message with ID and namespace
//
// Returns error if format is invalid.
func formatDeleteOutput(memoryID string, namespace []string, format string) error {
	switch format {
	case "json":
		output := map[string]interface{}{
			"memory_id": memoryID,
			"namespace": namespace,
			"deleted":   true,
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))

	case "text":
		fmt.Printf("✓ Memory deleted successfully\n")
		fmt.Printf("  ID:        %s\n", memoryID)
		fmt.Printf("  Namespace: %s\n", strings.Join(namespace, " > "))

	default:
		return fmt.Errorf("invalid format: %s (use json|text)", format)
	}

	return nil
}
