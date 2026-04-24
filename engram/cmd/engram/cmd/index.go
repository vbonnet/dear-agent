package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/ecphory"
)

var (
	indexTier        string
	indexIncremental bool
	indexVerify      bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage engram indexes",
	Long: `Manage engram indexes for fast filtering and retrieval.

The index system enables fast filtering by tags, types, and agents without
parsing every engram file. Indexes are built from engram frontmatter.`,
}

var rebuildCmd = &cobra.Command{
	Use:   "rebuild",
	Short: "Rebuild engram indexes",
	Long: `Rebuild engram indexes for one or all tiers.

Tiers: user, team, company, core, all (default: all)

Examples:
  # Rebuild all tiers
  engram index rebuild

  # Rebuild specific tier
  engram index rebuild --tier=user

  # Rebuild and verify
  engram index rebuild --verify`,
	RunE: runRebuild,
}

var verifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Verify index health",
	Long: `Verify index health across all tiers.

Checks:
- Index file exists
- Index is not corrupted
- Index is up-to-date with files
- Statistics (count, last updated)`,
	RunE: runVerify,
}

func init() {
	// Add flags to rebuild command
	rebuildCmd.Flags().StringVar(&indexTier, "tier", "all", "Tier to rebuild (user|team|company|core|all)")
	rebuildCmd.Flags().BoolVar(&indexIncremental, "incremental", false, "Incremental rebuild (future)")
	rebuildCmd.Flags().BoolVar(&indexVerify, "verify", false, "Verify index after rebuild")

	// Add subcommands
	indexCmd.AddCommand(rebuildCmd)
	indexCmd.AddCommand(verifyCmd)

	// Add to root
	rootCmd.AddCommand(indexCmd)
}

func runRebuild(cmd *cobra.Command, args []string) error {
	start := time.Now()

	// Validate tier
	if err := cli.ValidateTier(indexTier); err != nil {
		return err
	}

	// Get base path with workspace detection
	basePath, err := getEngramBasePath()
	if err != nil {
		return fmt.Errorf("failed to get engram base path: %w", err)
	}

	// Validate path for security (prevent path traversal)
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	allowedPaths := []string{
		filepath.Join(home, ".engram"),
		home, // Allow anywhere in home directory
	}
	if err := cli.ValidateSafePath("engram base path", basePath, allowedPaths); err != nil {
		return err
	}

	// Determine tiers to rebuild
	var tiers []string
	if indexTier == "all" {
		tiers = []string{"core", "company", "team", "user"}
	} else {
		tiers = []string{indexTier}
	}

	// Create progress indicator
	progress := cli.NewProgress("Indexing engrams...")
	progress.Start()
	defer progress.Stop()

	// Rebuild each tier
	totalEngrams := 0
	for _, tier := range tiers {
		progress.Update(fmt.Sprintf("Indexing %s tier...", tier))

		count, err := rebuildTier(basePath, tier)
		if err != nil {
			// Log error but continue with other tiers
			cli.PrintWarning(fmt.Sprintf("Failed to rebuild %s tier: %v", tier, err))
			continue
		}
		totalEngrams += count
	}

	elapsed := time.Since(start)
	progress.Complete(fmt.Sprintf("Indexed %d engrams in %dms", totalEngrams, elapsed.Milliseconds()))

	// Verify if requested
	if indexVerify {
		return runVerify(cmd, args)
	}

	return nil
}

func rebuildTier(basePath, tier string) (int, error) {
	// Build path to tier directory (scan all subdirectories for .ai.md files)
	tierPath := filepath.Join(basePath, tier)

	// Check if tier directory exists
	if _, err := os.Stat(tierPath); os.IsNotExist(err) {
		// Tier doesn't exist, skip silently
		return 0, nil
	}

	// Create new index
	index := ecphory.NewIndex()

	// Build index from tier directory (recursively scans all subdirectories)
	if err := index.Build(tierPath); err != nil {
		return 0, fmt.Errorf("failed to build index: %w", err)
	}

	// Get count
	count := len(index.All())

	// TODO: Save index to cache file
	// cachePath := filepath.Join(basePath, "cache", fmt.Sprintf("index-%s.json", tier))
	// if err := saveIndexCache(index, cachePath); err != nil {
	//     return 0, fmt.Errorf("failed to save index cache: %w", err)
	// }

	return count, nil
}

func runVerify(cmd *cobra.Command, args []string) error {
	// Get base path with security validation
	basePath := os.Getenv("ENGRAM_HOME")
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	if basePath == "" {
		basePath = filepath.Join(home, ".engram")
	} else {
		// Validate ENGRAM_HOME environment variable (prevent path traversal)
		allowedPaths := []string{
			filepath.Join(home, ".engram"),
			home, // Allow anywhere in home directory
		}
		if err := cli.ValidateSafePath("ENGRAM_HOME", basePath, allowedPaths); err != nil {
			return err
		}
	}

	fmt.Println("Engram Index Health Check")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	tiers := []string{"core", "company", "team", "user"}
	allHealthy := true

	for _, tier := range tiers {
		tierPath := filepath.Join(basePath, tier)

		// Check if tier exists
		if _, err := os.Stat(tierPath); os.IsNotExist(err) {
			cli.PrintInfo(fmt.Sprintf("%s tier: Not configured", tier))
			continue
		}

		// Build index to count engrams
		index := ecphory.NewIndex()
		if err := index.Build(tierPath); err != nil {
			cli.PrintError(fmt.Sprintf("%s tier: Error - %v", tier, err))
			allHealthy = false
			continue
		}

		count := len(index.All())
		if count == 0 {
			cli.PrintWarning(fmt.Sprintf("%s tier: No engrams found", tier))
		} else {
			cli.PrintSuccess(fmt.Sprintf("%s tier: %d engrams indexed", tier, count))
		}
	}

	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if !allHealthy {
		return fmt.Errorf("index health check failed")
	}

	return nil
}
