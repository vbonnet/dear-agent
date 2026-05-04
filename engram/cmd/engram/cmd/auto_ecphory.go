package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/ecphory"
	"github.com/vbonnet/dear-agent/engram/hippocampus"
	"github.com/vbonnet/dear-agent/pkg/engram"
)

// autoEcphoryInput represents the JSON input from stdin (UserPromptSubmit event).
type autoEcphoryInput struct {
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	HookEventName string `json:"hook_event_name"`
	Prompt        string `json:"prompt"`
}

// maxOutputChars is the approximate character budget (~2000 tokens).
const maxOutputChars = 8000

// autoEcphoryTimeout is the hard deadline for the entire command.
const autoEcphoryTimeout = 3 * time.Second

// maxAutoEcphoryResults caps the number of engrams returned.
const maxAutoEcphoryResults = 5

// minAutoEcphoryScore is the minimum relevance score to include an engram.
const minAutoEcphoryScore = 4

var autoEcphoryCmd = &cobra.Command{
	Use:   "auto-ecphory",
	Short: "Automatically retrieve relevant engrams for an agent prompt",
	Long: `Reads a UserPromptSubmit JSON event from stdin, finds relevant engrams
via ecphory tier-1 filtering, and outputs matched engrams as markdown
sections to stdout.

This command is designed to be called as a hook by AI coding agents.
It always exits 0, even on errors, to avoid blocking the agent.

INPUT (stdin JSON):
  {"session_id": "abc", "cwd": "~/project",
   "hook_event_name": "UserPromptSubmit", "prompt": "fix the auth bug"}

OUTPUT (stdout):
  <engram-context>
  ## Title
  Content...
  </engram-context>`,
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runAutoEcphory,
}

func init() {
	rootCmd.AddCommand(autoEcphoryCmd)
}

func runAutoEcphory(cmd *cobra.Command, args []string) error {
	// Never return an error — write diagnostics to stderr only.
	runAutoEcphoryInner(cmd)
	return nil
}

func runAutoEcphoryInner(cmd *cobra.Command) {
	ctx, cancel := context.WithTimeout(context.Background(), autoEcphoryTimeout)
	defer cancel()

	// 1. Read and parse stdin JSON
	input, err := parseAutoEcphoryStdin(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram auto-ecphory: stdin parse: %v\n", err)
		return
	}

	if input.Prompt == "" {
		fmt.Fprintf(os.Stderr, "engram auto-ecphory: empty prompt, skipping\n")
		return
	}

	// 2. Resolve engram base path
	engramPath, err := resolveEngramPath(input.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram auto-ecphory: resolve path: %v\n", err)
		return
	}

	// 3. Check that the path exists before doing expensive work
	if _, err := os.Stat(engramPath); os.IsNotExist(err) {
		// No engrams directory — silently exit
		return
	}

	// 4. Build ecphory and query
	results, err := queryEngrams(ctx, engramPath, input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engram auto-ecphory: query: %v\n", err)
		return
	}

	// 4b. Surface relevant hippocampus memories
	memoryDir := resolveMemoryDir(input.Cwd)
	var memoryContents []string
	if memoryDir != "" {
		surfaced, err := hippocampus.SurfaceRelevantMemories(ctx, memoryDir, input.Prompt, nil, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "engram auto-ecphory: surface memories: %v\n", err)
		} else {
			memoryContents = surfaced
		}
	}

	if len(results) == 0 && len(memoryContents) == 0 {
		return
	}

	// 5. Format and output
	output := formatEngramOutput(results)
	if len(memoryContents) > 0 {
		output += formatMemoryOutput(memoryContents)
	}
	if output != "" {
		fmt.Fprint(cmd.OutOrStdout(), output)
	}
}

// parseAutoEcphoryStdin reads JSON from stdin with context awareness.
func parseAutoEcphoryStdin(ctx context.Context) (*autoEcphoryInput, error) {
	// Use a channel so we can respect context timeout while reading stdin
	type readResult struct {
		data []byte
		err  error
	}
	ch := make(chan readResult, 1)

	go func() {
		data, err := io.ReadAll(os.Stdin)
		ch <- readResult{data, err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("stdin read timed out")
	case res := <-ch:
		if res.err != nil {
			return nil, fmt.Errorf("reading stdin: %w", res.err)
		}
		if len(res.data) == 0 {
			return nil, fmt.Errorf("empty stdin")
		}
		var input autoEcphoryInput
		if err := json.Unmarshal(res.data, &input); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
		return &input, nil
	}
}

// resolveEngramPath determines where engrams live, given the cwd from the event.
func resolveEngramPath(_ string) (string, error) {
	// Try the existing workspace detection first
	basePath, err := getEngramBasePath()
	if err == nil && basePath != "" {
		return basePath, nil
	}

	// Fallback: ENGRAM_HOME or ~/.engram
	if envPath := os.Getenv("ENGRAM_HOME"); envPath != "" {
		return envPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return home + "/.engram", nil
}

// queryEngrams builds the ecphory index and runs a tier-1 query.
// It intentionally skips the API ranking tier (tier 2) so it can complete
// within the 3-second budget.
func queryEngrams(ctx context.Context, engramPath string, input *autoEcphoryInput) ([]*engram.Engram, error) {
	// Build index directly (faster than full NewEcphory which also creates a ranker)
	idx := ecphory.NewIndex()
	if err := idx.Build(engramPath); err != nil {
		return nil, fmt.Errorf("building index: %w", err)
	}

	// Check context after potentially slow index build
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("timed out after index build: %w", err)
	}

	// Tier 1: Get all candidates (no tag/agent filter for auto-ecphory)
	candidates := idx.All()
	if len(candidates) == 0 {
		return nil, nil
	}

	// Load and score candidates using simple keyword matching
	parser := engram.NewParser()
	type scored struct {
		engram *engram.Engram
		score  int
	}
	var matches []scored
	seenTitles := make(map[string]bool) // Deduplicate by title

	promptLower := strings.ToLower(input.Prompt)
	words := strings.Fields(promptLower)

	for _, path := range candidates {
		if ctx.Err() != nil {
			break
		}

		eg, err := parser.Parse(path)
		if err != nil {
			continue
		}

		// Deduplicate: skip engrams with identical titles (common with symlinks)
		titleKey := strings.ToLower(eg.Frontmatter.Title)
		if seenTitles[titleKey] {
			continue
		}
		seenTitles[titleKey] = true

		score := scoreEngram(eg, words, promptLower)
		if score >= minAutoEcphoryScore {
			matches = append(matches, scored{engram: eg, score: score})
		}
	}

	if len(matches) == 0 {
		return nil, nil
	}

	// Sort by score descending
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})

	// Cap results and collect within token budget
	var results []*engram.Engram
	charsUsed := 0
	for _, m := range matches {
		if len(results) >= maxAutoEcphoryResults {
			break
		}

		// Estimate output size: title line + content + wrapper
		entryChars := len(m.engram.Frontmatter.Title) + len(m.engram.Content) + 30
		if charsUsed+entryChars > maxOutputChars {
			if len(results) > 0 {
				break
			}
			// If the first engram exceeds the budget, allow it (will be truncated in format)
		}
		results = append(results, m.engram)
		charsUsed += entryChars
	}

	return results, nil
}

// scoreEngram computes a simple relevance score using keyword overlap
// between the prompt and the engram's metadata and content.
func scoreEngram(eg *engram.Engram, promptWords []string, _ string) int {
	score := 0

	titleLower := strings.ToLower(eg.Frontmatter.Title)
	descLower := strings.ToLower(eg.Frontmatter.Description)
	loadWhenLower := strings.ToLower(eg.Frontmatter.LoadWhen)
	contentLower := strings.ToLower(eg.Content)

	for _, w := range promptWords {
		if len(w) < 3 {
			continue // Skip short words (the, a, is, ...)
		}
		if strings.Contains(titleLower, w) {
			score += 3 // Title matches are highly weighted
		}
		if strings.Contains(descLower, w) {
			score += 2
		}
		if strings.Contains(loadWhenLower, w) {
			score += 3 // load_when is specifically for trigger matching
		}
		for _, tag := range eg.Frontmatter.Tags {
			if strings.Contains(strings.ToLower(tag), w) {
				score += 2
				break
			}
		}
		if strings.Contains(contentLower, w) {
			score++
		}
	}

	return score
}

// formatEngramOutput formats matched engrams as markdown wrapped in
// <engram-context> tags, respecting the character budget.
func formatEngramOutput(engrams []*engram.Engram) string {
	if len(engrams) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<engram-context>\n")

	charsRemaining := maxOutputChars - len("<engram-context>\n</engram-context>\n")

	for _, eg := range engrams {
		header := fmt.Sprintf("## %s\n", eg.Frontmatter.Title)
		content := eg.Content

		// Add freshness caveat if engram file is stale (>1 day old)
		if eg.Path != "" {
			days, err := hippocampus.MemoryAgeDays(eg.Path)
			if err == nil {
				if caveat := hippocampus.MemoryFreshnessText(days); caveat != "" {
					header = fmt.Sprintf("## %s\n\n> %s\n\n", eg.Frontmatter.Title, caveat)
				}
			}
		}

		entryLen := len(header) + len(content) + 1 // +1 for trailing newline
		if entryLen > charsRemaining {
			// Truncate content to fit
			available := charsRemaining - len(header) - len("\n[truncated]\n") - 1
			if available > 0 {
				content = content[:available] + "\n[truncated]"
			} else {
				break
			}
		}

		sb.WriteString(header)
		sb.WriteString(content)
		sb.WriteString("\n")

		charsRemaining -= len(header) + len(content) + 1
		if charsRemaining <= 0 {
			break
		}
	}

	sb.WriteString("</engram-context>\n")
	return sb.String()
}

// resolveMemoryDir finds the Claude Code memory directory for the given cwd.
func resolveMemoryDir(cwd string) string {
	adapter := hippocampus.NewClaudeCodeAdapter("")
	memDir, err := adapter.GetMemoryDir(cwd)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(memDir); os.IsNotExist(err) {
		return ""
	}
	return memDir
}

// formatMemoryOutput formats surfaced memory contents as an engram-context block.
func formatMemoryOutput(contents []string) string {
	if len(contents) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("<engram-context>\n")
	charsRemaining := maxOutputChars

	for _, content := range contents {
		if len(content) > charsRemaining {
			if charsRemaining > 100 {
				sb.WriteString(content[:charsRemaining-20])
				sb.WriteString("\n[truncated]\n")
			}
			break
		}
		sb.WriteString(content)
		sb.WriteString("\n")
		charsRemaining -= len(content) + 1
	}

	sb.WriteString("</engram-context>\n")
	return sb.String()
}
