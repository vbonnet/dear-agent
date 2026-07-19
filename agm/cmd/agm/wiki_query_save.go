package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

var (
	wikiQuerySaveQuery      string
	wikiQuerySaveQueryFile  string
	wikiQuerySaveAnswer     string
	wikiQuerySaveAnswerFile string
	wikiQuerySaveOutput     string
	wikiQuerySaveCategory   string
	wikiQuerySaveNoIngest   bool
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
  agm wiki query-save --query-file /tmp/question.txt \
      --answer-file /tmp/answer.txt

  agm wiki query-save --query-file /tmp/question.txt \
      --answer-file /tmp/answer.txt --output 02-research-index/topic-foo.md

  agm wiki query-save --query-file /tmp/question.txt \
      --answer-file /tmp/answer.txt --category decisions`,
	RunE: runWikiQuerySave,
}

func init() {
	wikiCmd.AddCommand(wikiQuerySaveCmd)
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveQuery, "query", "",
		"the question that was asked (use --query-file for untrusted or long text)")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveQueryFile, "query-file", "",
		"file containing the question that was asked")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveAnswer, "answer", "",
		"the synthesised answer text (use --answer-file for long answers)")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveAnswerFile, "answer-file", "",
		"file containing the synthesised answer text")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveOutput, "output", "",
		"explicit output path (repo-relative); auto-derived from query if omitted")
	wikiQuerySaveCmd.Flags().StringVar(&wikiQuerySaveCategory, "category", "research",
		"output category: research | decisions")
	wikiQuerySaveCmd.Flags().BoolVar(&wikiQuerySaveNoIngest, "no-ingest", false,
		"skip backlink audit and index update after saving")
	wikiQuerySaveCmd.MarkFlagsMutuallyExclusive("query", "query-file")
	wikiQuerySaveCmd.MarkFlagsOneRequired("query", "query-file")
	wikiQuerySaveCmd.MarkFlagsMutuallyExclusive("answer", "answer-file")
	wikiQuerySaveCmd.MarkFlagsOneRequired("answer", "answer-file")
}

func runWikiQuerySave(cmd *cobra.Command, _ []string) error {
	kbPath, err := resolveKBPath(wikiKBPath)
	if err != nil {
		return err
	}

	query, err := resolveWikiTextInput("query", wikiQuerySaveQuery, wikiQuerySaveQueryFile)
	if err != nil {
		return err
	}
	answer, err := resolveWikiTextInput("answer", wikiQuerySaveAnswer, wikiQuerySaveAnswerFile)
	if err != nil {
		return err
	}

	// Derive output path
	outRel := wikiQuerySaveOutput
	if outRel == "" {
		outRel = deriveOutputPath(query, wikiQuerySaveCategory)
	}
	absOut, err := resolveWikiOutputPath(kbPath, outRel)
	if err != nil {
		return err
	}

	// Check for collision
	if _, statErr := os.Stat(absOut); statErr == nil {
		return fmt.Errorf("page already exists: %s — choose a different output path", outRel)
	}

	content := buildQueryPage(query, answer, outRel)

	if mkdirErr := os.MkdirAll(filepath.Dir(absOut), 0o700); mkdirErr != nil {
		return fmt.Errorf("cannot create directory: %w", mkdirErr)
	}
	if writeErr := os.WriteFile(absOut, []byte(content), 0o600); writeErr != nil {
		return fmt.Errorf("failed to write page: %w", writeErr)
	}
	fmt.Printf("✅ Saved: %s\n", outRel)

	now := time.Now()
	if appendErr := appendToLog(kbPath, wikibrain.FormatQuerySaveLogEntry(query, outRel, now)); appendErr != nil {
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

func resolveWikiOutputPath(kbPath, output string) (string, error) {
	if strings.TrimSpace(output) == "" || filepath.IsAbs(output) {
		return "", fmt.Errorf("--output must be a repository-relative file path")
	}
	absKB, err := filepath.Abs(kbPath)
	if err != nil {
		return "", fmt.Errorf("resolve knowledge base path: %w", err)
	}
	absOut, err := filepath.Abs(filepath.Join(absKB, filepath.FromSlash(output)))
	if err != nil {
		return "", fmt.Errorf("resolve --output: %w", err)
	}
	rel, err := filepath.Rel(absKB, absOut)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("--output must stay within the knowledge base")
	}
	resolvedKB, err := filepath.EvalSymlinks(absKB)
	if err != nil {
		return "", fmt.Errorf("resolve knowledge base symlinks: %w", err)
	}
	resolvedOut, err := resolveWikiOutputSymlinks(absOut)
	if err != nil {
		return "", fmt.Errorf("resolve --output symlinks: %w", err)
	}
	resolvedRel, err := filepath.Rel(resolvedKB, resolvedOut)
	if err != nil || resolvedRel == "." || resolvedRel == ".." || strings.HasPrefix(resolvedRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("--output must stay within the knowledge base")
	}
	return absOut, nil
}

// resolveWikiOutputSymlinks resolves the target when it exists, or the nearest
// existing ancestor when it does not. This prevents an in-tree symlinked
// directory from redirecting a newly created page outside the knowledge base.
func resolveWikiOutputSymlinks(path string) (string, error) {
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	current := filepath.Dir(path)
	missing := []string{filepath.Base(path)}
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			for _, component := range slices.Backward(missing) {
				resolved = filepath.Join(resolved, component)
			}
			return resolved, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

const maxWikiQueryInputBytes = 1024 * 1024

func resolveWikiTextInput(name, inline, filePath string) (string, error) {
	if inline != "" && filePath != "" {
		return "", fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name)
	}
	value := inline
	if filePath != "" {
		file, err := os.Open(filePath)
		if err != nil {
			return "", fmt.Errorf("read --%s-file: %w", name, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxWikiQueryInputBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return "", fmt.Errorf("read --%s-file: %w", name, readErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close --%s-file: %w", name, closeErr)
		}
		if len(data) > maxWikiQueryInputBytes {
			return "", fmt.Errorf("--%s-file exceeds %d bytes", name, maxWikiQueryInputBytes)
		}
		value = string(data)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("--%s or --%s-file is required", name, name)
	}
	return value, nil
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
