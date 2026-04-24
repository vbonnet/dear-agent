package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

var (
	wikiQuerySaveQuery    string
	wikiQuerySaveAnswer   string
	wikiQuerySaveOutput   string
	wikiQuerySaveCategory string
	wikiQuerySaveNoIngest bool
)

var wikiQuerySaveCmd = &cobra.Command{
	Use:   "query-save",
	Short: "Save a synthesised query answer as a new wiki page (compounding mechanism)",
	Long: `query-save writes a query + its synthesised answer as a new research page
in engram-kb, then runs the ingest workflow (backlink audit + index update).

This is the compounding mechanism: answers that are worth keeping become
first-class wiki pages, making future queries faster.

The page is placed under:
  02-research-index/topic-<slug>.md   (default)
  01-decisions/ADR-NNN-<slug>.md      (if --category decisions)

Examples:
  agm wiki query-save --query "How does ecphory retrieval work?" \
      --answer-file /tmp/answer.txt

  agm wiki query-save --query "..." --answer "..." --output 02-research-index/topic-foo.md

  agm wiki query-save --query "..." --answer "..." --category decisions`,
	RunE: runWikiQuerySave,
}

func init() {
	wikiCmd.AddCommand(wikiQuerySaveCmd)
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveQuery, "query", "",
		"the question that was asked (required)")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveAnswer, "answer", "",
		"the synthesised answer text (use --answer-file for long answers)")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveOutput, "output", "",
		"explicit output path (repo-relative); auto-derived from query if omitted")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveCategory, "category", "research",
		"output category: research | decisions")
	wikiQuerySaveCmd.Flags().BoolVar(&wikiQuerySaveNoIngest, "no-ingest", false,
		"skip backlink audit and index update after saving")
	_ = wikiQuerySaveCmd.MarkFlagRequired("query")
}

func runWikiQuerySave(cmd *cobra.Command, _ []string) error {
	kbPath, err := resolveKBPath(wikiKBPath)
	if err != nil {
		return err
	}

	if wikiQuerySaveAnswer == "" {
		return fmt.Errorf("--answer is required (or pipe answer text via --answer-file)")
	}

	// Derive output path
	outRel := wikiQuerySaveOutput
	if outRel == "" {
		outRel = deriveOutputPath(wikiQuerySaveQuery, wikiQuerySaveCategory)
	}
	absOut := filepath.Join(kbPath, outRel)

	// Check for collision
	if _, statErr := os.Stat(absOut); statErr == nil {
		return fmt.Errorf("page already exists: %s — choose a different output path", outRel)
	}

	content := buildQueryPage(wikiQuerySaveQuery, wikiQuerySaveAnswer, outRel)

	if mkdirErr := os.MkdirAll(filepath.Dir(absOut), 0o755); mkdirErr != nil {
		return fmt.Errorf("cannot create directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(absOut, []byte(content), 0o644); writeErr != nil {
		return fmt.Errorf("failed to write page: %w", writeErr)
	}
	fmt.Printf("✅ Saved: %s\n", outRel)

	now := time.Now()
	if appendErr := appendToLog(kbPath, wikibrain.FormatQuerySaveLogEntry(wikiQuerySaveQuery, outRel, now)); appendErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not append to log.md: %v\n", appendErr)
	}

	if !wikiQuerySaveNoIngest {
		// Re-use ingest logic via flag injection
		wikiIngestPage = outRel
		wikiIngestNoAppend = true // already logged above
		wikiIngestNoIndex = false
		wikiKBPath = kbPath // already resolved
		if ingestErr := runWikiIngest(cmd, nil); ingestErr != nil {
			fmt.Fprintf(os.Stderr, "warning: ingest audit failed: %v\n", ingestErr)
		}
	}

	return nil
}

// deriveOutputPath builds a safe repo-relative path from the query text.
func deriveOutputPath(query, category string) string {
	slug := slugify(query)
	if len(slug) > 50 {
		slug = slug[:50]
	}
	switch category {
	case "decisions":
		return filepath.Join("01-decisions", "topic-"+slug+".md")
	default:
		return filepath.Join("02-research-index", "topic-"+slug+".md")
	}
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var sb strings.Builder
	prev := '-'
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
			prev = r
		} else if prev != '-' {
			sb.WriteRune('-')
			prev = '-'
		}
	}
	return strings.Trim(sb.String(), "-")
}

func buildQueryPage(query, answer, relPath string) string {
	title := strings.TrimSuffix(filepath.Base(relPath), ".md")
	title = strings.TrimPrefix(title, "topic-")
	title = strings.ReplaceAll(title, "-", " ")

	now := time.Now()
	return fmt.Sprintf(`# %s

- **Last updated:** %s
- **Source:** synthesised from wiki query
- **Query:** %s

## Answer

%s

## See Also

<!-- Add related pages here after reviewing backlink suggestions -->
`, toTitleCase(title), now.Format("2006-01-02"), query, answer)
}

func toTitleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
