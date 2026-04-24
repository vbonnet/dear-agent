package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/consolidation"
)

var (
	storeNamespace    string
	storeMemoryID     string
	storeType         string
	storeContent      string
	storeContentStdin bool
	storeImportance   float64
	storeMetadata     string
	storeFormat       string
)

var memoryStoreCmd = &cobra.Command{
	Use:   "store",
	Short: "Store a new memory entry",
	Long: `Store a new memory entry in long-term storage.

Memories are organized by namespace (hierarchical scoping) and type (cognitive function).
The memory provider (default: simple) determines the underlying storage mechanism.

REQUIRED FLAGS
  --namespace     Namespace path (comma-separated, e.g., "user,alice,project")
  --type          Memory type (episodic|semantic|procedural|working)
  --content       Memory content (string or JSON) [mutually exclusive with --content-stdin]
  --content-stdin Read memory content from stdin [mutually exclusive with --content]

OPTIONAL FLAGS
  --memory-id     Unique memory ID (default: auto-generated UUID)
  --importance    Importance score 0-1 (default: 0)
  --metadata      Metadata as JSON object (e.g., '{"source":"wayfinder","phase":"D1"}')
  --format        Output format (json|text) (default: text)

EXAMPLES
  # Store episodic memory
  $ engram memory store \
      --namespace user,alice \
      --type episodic \
      --content "Completed D1 validation phase at 2025-12-15T10:30:00Z"

  # Store with importance and metadata
  $ engram memory store \
      --namespace user,alice,project,myapp \
      --type semantic \
      --content "MemoryConsolidation API uses tiered architecture" \
      --importance 0.9 \
      --metadata '{"source":"research","topic":"architecture"}'

  # Store with custom ID
  $ engram memory store \
      --namespace user,alice \
      --type procedural \
      --content "Run multi-persona review before merging" \
      --memory-id mem-custom-123

  # Store content from stdin (agent-friendly)
  $ echo "Completed architecture review" | engram memory store \
      --namespace user,alice \
      --type episodic \
      --content-stdin`,
	RunE: runMemoryStore,
}

func init() {
	memoryCmd.AddCommand(memoryStoreCmd)

	// Required flags
	memoryStoreCmd.Flags().StringVar(&storeNamespace, "namespace", "", "Namespace path (comma-separated)")
	memoryStoreCmd.Flags().StringVar(&storeType, "type", "", "Memory type (episodic|semantic|procedural|working)")
	memoryStoreCmd.Flags().StringVar(&storeContent, "content", "", "Memory content")
	memoryStoreCmd.Flags().BoolVar(&storeContentStdin, "content-stdin", false, "Read memory content from stdin")
	memoryStoreCmd.MarkFlagRequired("namespace")
	memoryStoreCmd.MarkFlagRequired("type")
	// Note: content OR content-stdin is required, validated in runMemoryStore

	// Mutually exclusive: --content and --content-stdin
	memoryStoreCmd.MarkFlagsMutuallyExclusive("content", "content-stdin")

	// Optional flags
	memoryStoreCmd.Flags().StringVar(&storeMemoryID, "memory-id", "", "Unique memory ID (default: auto-generated)")
	memoryStoreCmd.Flags().Float64Var(&storeImportance, "importance", 0, "Importance score 0-1")
	memoryStoreCmd.Flags().StringVar(&storeMetadata, "metadata", "", "Metadata as JSON object")
	memoryStoreCmd.Flags().StringVarP(&storeFormat, "format", "f", "text", "Output format (json|text)")
}

// runMemoryStore executes the memory store command.
//
// It validates inputs, loads the provider, creates a memory entry,
// and stores it in long-term storage. Memory ID is auto-generated
// if not provided. Supports JSON and text output formats.
func runMemoryStore(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// 1. Parse and validate inputs
	if err := cli.ValidateNamespace(storeNamespace); err != nil {
		return err
	}
	namespace := parseNamespace(storeNamespace)

	memoryType := consolidation.MemoryType(storeType)
	if err := validateMemoryType(memoryType); err != nil {
		return err
	}

	// Get content from --content or --content-stdin
	var content string
	if storeContentStdin {
		// Read from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read from stdin: %w", err)
		}
		content = string(data)
	} else if storeContent != "" {
		content = storeContent
	} else {
		return fmt.Errorf("either --content or --content-stdin is required")
	}

	if err := cli.ValidateNonEmpty("content", content); err != nil {
		return err
	}

	// Validate content length to prevent DoS attacks
	if err := cli.ValidateMaxLength("content", content, cli.MaxContentLength); err != nil {
		return err
	}

	// Generate memory ID if not provided
	memoryID := storeMemoryID
	if memoryID == "" {
		memoryID = "mem-" + uuid.New().String()
	}

	// Parse metadata if provided
	var metadata map[string]interface{}
	if storeMetadata != "" {
		if err := json.Unmarshal([]byte(storeMetadata), &metadata); err != nil {
			return fmt.Errorf("invalid metadata JSON: %w", err)
		}
	}

	// Validate importance range
	if err := cli.ValidateRange("importance", storeImportance, 0, 1); err != nil {
		return err
	}

	// Validate output format
	if err := cli.ValidateOutputFormat(storeFormat, cli.FormatJSON, cli.FormatText); err != nil {
		return err
	}

	// 2. Load provider
	provider, err := loadMemoryProvider(ctx)
	if err != nil {
		return err
	}
	defer provider.Close(ctx)

	// 3. Create memory
	memory := consolidation.Memory{
		SchemaVersion: "1.0",
		ID:            memoryID,
		Type:          memoryType,
		Namespace:     namespace,
		Content:       content,
		Metadata:      metadata,
		Timestamp:     time.Now(),
		Importance:    storeImportance,
	}

	// 4. Store memory
	if err := provider.StoreMemory(ctx, namespace, memory); err != nil {
		return fmt.Errorf("failed to store memory: %w", err)
	}

	// 5. Format output
	return formatStoreOutput(memory, storeFormat)
}

// parseNamespace splits comma-separated namespace string into a slice.
//
// Trims whitespace from each part and filters out empty parts.
// Example: "user, alice , project" → ["user", "alice", "project"]
func parseNamespace(ns string) []string {
	if ns == "" {
		return nil
	}
	parts := strings.Split(ns, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// validateMemoryType validates that the memory type is one of the supported types.
//
// Returns a descriptive error message listing valid types if validation fails.
// Supported types: episodic, semantic, procedural, working.
func validateMemoryType(t consolidation.MemoryType) error {
	switch t {
	case consolidation.Episodic, consolidation.Semantic, consolidation.Procedural, consolidation.Working:
		return nil
	default:
		return fmt.Errorf("invalid memory type %q\nValid types:\n  episodic    - Specific experiences (what happened)\n  semantic    - Extracted knowledge (facts learned)\n  procedural  - Skills and procedures (how to do things)\n  working     - Active context (immediate focus)", t)
	}
}

// loadMemoryProvider loads and initializes the memory provider based on configuration.
//
// Uses configuration hierarchy: --provider flag > ENGRAM_MEMORY_PROVIDER env > default ("simple").
// In v0.1.0, only the "simple" provider is supported. The --config flag specifies the storage
// directory path, not a YAML config file (full config file support coming in v0.2.0).
//
// Returns initialized provider ready for operations, or error if provider type is unsupported
// or initialization fails.
func loadMemoryProvider(ctx context.Context) (consolidation.Provider, error) {
	providerType := getMemoryProvider()
	configPath, err := getMemoryConfig()
	if err != nil {
		return nil, err
	}

	// For v0.1.0, we only support "simple" provider
	// In future versions, we'll load config from configPath and use consolidation.Load()
	if providerType != "simple" {
		return nil, fmt.Errorf("unsupported provider type: %s (only 'simple' is supported in v0.1.0)", providerType)
	}

	// Load simple provider with default config
	config := consolidation.Config{
		ProviderType: "simple",
		Options: map[string]interface{}{
			"storage_path": configPath, // Use config path as storage directory
		},
	}

	provider, err := consolidation.Load(config)
	if err != nil {
		return nil, fmt.Errorf("failed to load provider: %w", err)
	}

	// Initialize provider
	if err := provider.Initialize(ctx, config); err != nil {
		return nil, fmt.Errorf("failed to initialize provider: %w", err)
	}

	return provider, nil
}

// formatStoreOutput formats and prints the store command result.
//
// Supports two output formats:
//   - "json": Full memory object as indented JSON
//   - "text": Human-readable summary with ID, type, namespace, timestamp, importance, and metadata
//
// Returns error if format is invalid.
func formatStoreOutput(memory consolidation.Memory, format string) error {
	switch format {
	case "json":
		data, err := json.MarshalIndent(memory, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	case "text":
		fmt.Printf("✓ Memory stored successfully\n")
		fmt.Printf("  ID:        %s\n", memory.ID)
		fmt.Printf("  Type:      %s\n", memory.Type)
		fmt.Printf("  Namespace: %s\n", strings.Join(memory.Namespace, " > "))
		fmt.Printf("  Timestamp: %s\n", memory.Timestamp.Format(time.RFC3339))
		if memory.Importance > 0 {
			fmt.Printf("  Importance: %.2f\n", memory.Importance)
		}
		if len(memory.Metadata) > 0 {
			metaJSON, _ := json.Marshal(memory.Metadata)
			fmt.Printf("  Metadata:  %s\n", string(metaJSON))
		}
	default:
		return fmt.Errorf("invalid format: %s (use json|text)", format)
	}
	return nil
}
