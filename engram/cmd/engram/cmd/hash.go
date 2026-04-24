package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/hash"
)

var hashCmd = &cobra.Command{
	Use:   "hash <file>",
	Short: "Calculate SHA-256 hash of a file",
	Long: `Calculate SHA-256 hash of a file and output in format: sha256:{hex_hash}

The hash command:
- Supports tilde (~) expansion for home directory paths
- Outputs hash to stdout (no newline) for easy piping
- Returns error if file cannot be read or does not exist

This command is used by Wayfinder to calculate phase engram hashes
for methodology freshness validation.

Exit codes:
  0 - Success (hash calculated)
  1 - Error (file not found, cannot read, etc.)

Examples:
  # Calculate hash of a file
  engram hash myfile.txt

  # Calculate hash with tilde expansion
  engram hash ~/Documents/file.md

  # Use in shell scripts
  HASH=$(engram hash myfile.txt)
  echo "Hash: $HASH"

  # Pipe to other commands
  engram hash file.txt | cut -d: -f2`,
	Args: cobra.ExactArgs(1),
	RunE: runHash,
}

func init() {
	rootCmd.AddCommand(hashCmd)
}

func runHash(cmd *cobra.Command, args []string) error {
	// Calculate hash
	hashValue, err := hash.CalculateFileHash(args[0])
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Output hash to stdout (no newline for easy piping)
	fmt.Print(hashValue)
	return nil
}
