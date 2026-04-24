package cmd

import "github.com/spf13/cobra"

var tokensCmd = &cobra.Command{
	Use:   "tokens",
	Short: "Token estimation commands",
	Long: `Commands for estimating token counts in engram files.

Engram uses multiple tokenization methods to estimate token usage:
  - char/4: Simple heuristic (character count divided by 4)
  - tiktoken: OpenAI's cl100k_base encoding (if available)
  - simple: Word-based tokenizer (if available)

Use these commands to optimize engram files and stay within token limits.`,
}

func init() {
	rootCmd.AddCommand(tokensCmd)
}
