package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/hippocampus"
)

var criticCmd = &cobra.Command{
	Use:   "critic",
	Short: "Run critic analysis on code input",
	Long:  "Analyze code from stdin for security and correctness issues using the built-in critic.",
	RunE: func(cmd *cobra.Command, args []string) error {
		scanner := bufio.NewScanner(os.Stdin)
		var lines []string
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}

		input := strings.Join(lines, "\n")
		decision := hippocampus.CriticCheck(input)

		dyad := hippocampus.NewDyad("")
		if err := dyad.LogDecision(decision); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to log decision: %v\n", err)
		}

		if decision.Approved {
			fmt.Println("✓ No issues found")
			return nil
		}

		fmt.Printf("✗ Found %d issue(s):\n", len(decision.Issues))
		for _, issue := range decision.Issues {
			fmt.Printf("  [%s] %s: %s\n", issue.Severity, issue.Category, issue.Detail)
			if issue.Line != "" {
				fmt.Printf("    → %s\n", issue.Line)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(criticCmd)
}
