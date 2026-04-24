package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

var (
	retrieveNamespace     string
	retrieveType          string
	retrieveMinImportance float64
	retrieveLimit         int
	retrieveText          string
	retrieveFormat        string
)

var memoryRetrieveCmd = &cobra.Command{
	Use:   "retrieve",
	Short: "Retrieve memories matching a query",
	Long: `Retrieve memories from long-term storage matching the specified query.

Supports filtering by type, importance, and text search. Results are sorted
by timestamp descending (newest first).

REQUIRED FLAGS
  --namespace     Namespace path (comma-separated, e.g., "user,alice,project")

OPTIONAL FLAGS
  --type          Filter by memory type (episodic|semantic|procedural|working)
  --min-importance Filter by minimum importance score (0-1)
  --limit         Maximum number of results (default: 10, 0 = all)
  --text          Text search in content (simple substring match in v0.1.0)
  --format        Output format (json|text|table) (default: table)

EXAMPLES
  # Retrieve last 10 memories from namespace
  $ engram memory retrieve --namespace user,alice

  # Retrieve episodic memories only
  $ engram memory retrieve --namespace user,alice --type episodic --limit 20

  # Retrieve high-importance memories
  $ engram memory retrieve --namespace user,alice --min-importance 0.8

  # Text search
  $ engram memory retrieve --namespace user,alice --text "validation"

  # Export to JSON
  $ engram memory retrieve --namespace user,alice --format json > memories.json`,
	RunE: runMemoryRetrieve,
}

func init() {
	memoryCmd.AddCommand(memoryRetrieveCmd)

	// Required flags
	memoryRetrieveCmd.Flags().StringVar(&retrieveNamespace, "namespace", "", "Namespace path (comma-separated)")
	memoryRetrieveCmd.MarkFlagRequired("namespace")

	// Optional flags
	memoryRetrieveCmd.Flags().StringVar(&retrieveType, "type", "", "Filter by memory type")
	memoryRetrieveCmd.Flags().Float64Var(&retrieveMinImportance, "min-importance", 0, "Minimum importance score")
	memoryRetrieveCmd.Flags().IntVarP(&retrieveLimit, "limit", "n", 10, "Maximum number of results (0 = all)")
	memoryRetrieveCmd.Flags().StringVar(&retrieveText, "text", "", "Text search in content")
	memoryRetrieveCmd.Flags().StringVarP(&retrieveFormat, "format", "f", "table", "Output format (json|text|table)")
}

// runMemoryRetrieve executes the memory retrieve command.
//
// Queries long-term storage for memories matching specified criteria (type,
// importance, text search). Results are sorted by timestamp descending (newest first)
// and limited by the --limit flag (default: 10). Supports JSON, text, and table output formats.
func runMemoryRetrieve(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Parse and validate inputs
	if err := cli.ValidateNamespace(retrieveNamespace); err != nil {
		return err
	}
	namespace := parseNamespace(retrieveNamespace)

	// Validate memory type if provided
	var memoryType *consolidation.MemoryType
	if retrieveType != "" {
		t := consolidation.MemoryType(retrieveType)
		if err := validateMemoryType(t); err != nil {
			return err
		}
		memoryType = &t
	}

	// Validate importance range
	if err := cli.ValidateRange("min-importance", retrieveMinImportance, 0, 1); err != nil {
		return err
	}

	// Validate limit
	if err := cli.ValidatePositive("limit", retrieveLimit); err != nil {
		return err
	}

	// Validate output format
	if err := cli.ValidateOutputFormat(retrieveFormat, cli.FormatJSON, cli.FormatText, cli.FormatTable); err != nil {
		return err
	}

	// 2. Load provider
	provider, err := loadMemoryProvider(ctx)
	if err != nil {
		return err
	}
	defer provider.Close(ctx)

	// 3. Build query
	query := consolidation.Query{
		Limit: retrieveLimit,
	}
	if memoryType != nil {
		query.Type = *memoryType
	}
	if retrieveMinImportance > 0 {
		query.MinImportance = retrieveMinImportance
	}
	if retrieveText != "" {
		query.Text = retrieveText
	}

	// 4. Retrieve memories
	memories, err := provider.RetrieveMemory(ctx, namespace, query)
	if err != nil {
		return fmt.Errorf("failed to retrieve memories: %w", err)
	}

	if len(memories) == 0 {
		fmt.Println("No memories found matching the query.")
		return nil
	}

	// 5. Format output
	return formatRetrieveOutput(memories, retrieveFormat)
}

// formatRetrieveOutput formats and prints the retrieve command results.
//
// Supports three output formats:
//   - "json": Array of memory objects as indented JSON
//   - "text": Detailed view of each memory with all fields
//   - "table": Compact tabular view with ID, Type, Timestamp, and truncated content
//
// Returns error if format is invalid.
func formatRetrieveOutput(memories []consolidation.Memory, format string) error {
	switch format {
	case "json":
		data, err := json.MarshalIndent(memories, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))

	case "text":
		for i, m := range memories {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("Memory %d/%d\n", i+1, len(memories))
			fmt.Printf("  ID:        %s\n", m.ID)
			fmt.Printf("  Type:      %s\n", m.Type)
			fmt.Printf("  Namespace: %s\n", strings.Join(m.Namespace, " > "))
			fmt.Printf("  Timestamp: %s\n", m.Timestamp.Format(time.RFC3339))
			if m.Importance > 0 {
				fmt.Printf("  Importance: %.2f\n", m.Importance)
			}
			// Truncate content if too long
			content := fmt.Sprintf("%v", m.Content)
			if len(content) > 200 {
				content = content[:200] + "..."
			}
			fmt.Printf("  Content:   %s\n", content)
			if len(m.Metadata) > 0 {
				metaJSON, _ := json.Marshal(m.Metadata)
				fmt.Printf("  Metadata:  %s\n", string(metaJSON))
			}
		}

	case "table":
		// Print table header
		fmt.Printf("%-36s  %-12s  %-20s  %-10s  %s\n",
			"ID", "Type", "Timestamp", "Importance", "Content")
		fmt.Println(strings.Repeat("-", 120))

		// Print rows
		for _, m := range memories {
			content := fmt.Sprintf("%v", m.Content)
			if len(content) > 50 {
				content = content[:47] + "..."
			}
			timestamp := m.Timestamp.Format("2006-01-02 15:04:05")
			importance := "-"
			if m.Importance > 0 {
				importance = fmt.Sprintf("%.2f", m.Importance)
			}
			fmt.Printf("%-36s  %-12s  %-20s  %-10s  %s\n",
				m.ID, m.Type, timestamp, importance, content)
		}

		fmt.Printf("\nTotal: %d memories\n", len(memories))

	default:
		return fmt.Errorf("invalid format: %s (use json|text|table)", format)
	}

	return nil
}
