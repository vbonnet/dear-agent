package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/phaseengram"
)

var phaseEngramJSON bool

var phaseEngramCmd = &cobra.Command{
	Use:   "phase-engram <phase>",
	Short: "Resolve phase engram path and hash",
	Long: `Resolve the engram file path and compute its SHA-256 hash for a Wayfinder phase.

This replaces hardcoded hashes in skill templates. Instead of embedding a
stale hash, call this command at runtime to get the current path and hash.

Supported phases: CHARTER, PROBLEM, RESEARCH, DECISION, SPEC, DESIGN, PLAN,
BUILD, RETRO (and their numeric aliases W0, D1-D4, S4-S11).

Examples:
  # Get path and hash for CHARTER phase
  engram phase-engram CHARTER

  # JSON output for scripting
  engram phase-engram CHARTER --json

  # Use in shell to get just the hash
  HASH=$(engram phase-engram CHARTER --json | jq -r .hash)`,
	Args: cobra.ExactArgs(1),
	RunE: runPhaseEngram,
}

func init() {
	phaseEngramCmd.Flags().BoolVar(&phaseEngramJSON, "json", false, "Output as JSON")
	rootCmd.AddCommand(phaseEngramCmd)
}

func runPhaseEngram(cmd *cobra.Command, args []string) error {
	phase := args[0]
	path, hashValue, err := phaseengram.ResolveEngramPathAndHash(phase)
	if err != nil {
		return fmt.Errorf("failed to resolve phase engram: %w", err)
	}

	if phaseEngramJSON {
		out := map[string]string{
			"phase": phase,
			"path":  path,
			"hash":  hashValue,
		}
		data, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		fmt.Printf("path: %s\nhash: %s\n", path, hashValue)
	}

	return nil
}
