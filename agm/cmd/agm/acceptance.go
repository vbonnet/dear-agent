package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/acceptance"
)

var acceptanceCmd = &cobra.Command{
	Use:   "acceptance",
	Short: "Inspect acceptance criteria for the current repo",
	Long: `Acceptance criteria are machine-checkable exit conditions for tasks,
declared in .dear-agent.yml under acceptance-criteria:.

These are the DEAR Define phase making "what does done look like?" explicit.
They are NOT a blocking gate: workers can check their own work against the
list, and the Audit phase verifies them after completion.

Examples:
  agm acceptance show              # human-readable list
  agm acceptance show -o json      # machine-readable list`,
	Args: cobra.ArbitraryArgs,
	RunE: groupRunE,
}

var acceptanceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the acceptance criteria for this repo",
	Long: `Locate the nearest .dear-agent.yml (walking up from the current
directory) and print its acceptance-criteria: section. Exits with a
non-zero status only when the file is malformed; a missing file or
empty section is fine and prints "no acceptance criteria declared".`,
	RunE: runAcceptanceShow,
}

var acceptanceOutputFormat string

func init() {
	rootCmd.AddCommand(acceptanceCmd)
	acceptanceCmd.AddCommand(acceptanceShowCmd)
	acceptanceShowCmd.Flags().StringVarP(&acceptanceOutputFormat, "output", "o", "", "output format: text (default), json")
}

func runAcceptanceShow(_ *cobra.Command, _ []string) error {
	root, err := findDearAgentRoot()
	if err != nil {
		// No .dear-agent.yml found anywhere. That's not a hard error —
		// the worker is told there are no criteria and can proceed.
		return printAcceptance(nil, "")
	}

	crits, err := acceptance.Load(root)
	if err != nil {
		return fmt.Errorf("acceptance: %w", err)
	}
	return printAcceptance(crits, root)
}

func printAcceptance(crits []acceptance.Criterion, root string) error {
	format := acceptanceOutputFormat
	if format == "" {
		format = outputFormat
	}
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		// Always emit an array (possibly empty) so consumers don't have
		// to special-case nil.
		if crits == nil {
			crits = []acceptance.Criterion{}
		}
		return enc.Encode(crits)
	}

	if len(crits) == 0 {
		fmt.Println("No acceptance criteria declared.")
		if root != "" {
			fmt.Printf("  source: %s/.dear-agent.yml\n", root)
		}
		return nil
	}

	if root != "" {
		fmt.Printf("Acceptance criteria from %s/.dear-agent.yml:\n\n", root)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TYPE\tCOMMAND\tDESCRIPTION")
	for _, c := range crits {
		cmd := c.Command
		if cmd == "" {
			cmd = "-"
		}
		desc := c.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Type, cmd, desc)
	}
	return w.Flush()
}

// announceAcceptanceCriteria prints a short banner listing the
// acceptance-criteria declared for the repo containing workDir, if
// any. Failures are ignored — surfacing criteria is informational
// and must never block session creation.
//
// Called from `agm new` so a worker sees its DEAR Define-phase exit
// conditions at session start. The same data is available on demand
// via `agm acceptance show`.
func announceAcceptanceCriteria(workDir string) {
	root := findDearAgentRootFrom(workDir)
	if root == "" {
		return
	}
	crits, err := acceptance.Load(root)
	if err != nil || len(crits) == 0 {
		return
	}
	fmt.Printf("\nAcceptance criteria (from %s/.dear-agent.yml):\n", root)
	for _, c := range crits {
		fmt.Printf("  • %s\n", c.String())
	}
	fmt.Println()
}

// findDearAgentRootFrom is the workDir-rooted variant of
// findDearAgentRoot. Returns "" if no .dear-agent.yml is found.
func findDearAgentRootFrom(start string) string {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, ".dear-agent.yml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// findDearAgentRoot walks up from the current working directory looking
// for a .dear-agent.yml. Returns the directory that contains the file,
// or an error if none is found before the filesystem root.
func findDearAgentRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, ".dear-agent.yml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .dear-agent.yml found in %s or any parent", cwd)
		}
		dir = parent
	}
}
