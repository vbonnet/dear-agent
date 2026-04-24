package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

var (
	wikiIngestPage     string
	wikiIngestNoIndex  bool
	wikiIngestNoAppend bool
)

var wikiIngestCmd = &cobra.Command{
	Use:   "ingest --page <path>",
	Short: "Audit backlinks after adding a new page to engram-kb",
	Long: `ingest runs a backlink audit for a newly added page. It scans all
existing pages for mentions of the new page's title or filename stem and
prints suggestions for where to add bidirectional [[links]].

After the audit it optionally regenerates index.md and appends to log.md.

Examples:
  agm wiki ingest --page 02-research-index/topic-new-thing.md
  agm wiki ingest --page /abs/path/to/page.md --no-index`,
	RunE: runWikiIngest,
}

func init() {
	wikiCmd.AddCommand(wikiIngestCmd)
	wikiIngestCmd.Flags().StringVar(&wikiIngestPage, "page", "",
		"repo-relative or absolute path to the newly ingested page (required)")
	wikiIngestCmd.Flags().BoolVar(&wikiIngestNoIndex, "no-index", false,
		"skip regenerating index.md after the audit")
	wikiIngestCmd.Flags().BoolVar(&wikiIngestNoAppend, "no-append", false,
		"skip appending to log.md")
	_ = wikiIngestCmd.MarkFlagRequired("page")
}

func runWikiIngest(cmd *cobra.Command, _ []string) error {
	kbPath, err := resolveKBPath(wikiKBPath)
	if err != nil {
		return err
	}

	// Resolve page path to repo-relative form
	pagePath, err := resolvePagePath(kbPath, wikiIngestPage)
	if err != nil {
		return err
	}

	// Scan all pages including the new one
	pages, err := wikibrain.ScanKB(kbPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	// Find the new page
	var newPage *wikibrain.Page
	var others []*wikibrain.Page
	for _, p := range pages {
		if p.RelPath == pagePath {
			newPage = p
		} else {
			others = append(others, p)
		}
	}
	if newPage == nil {
		return fmt.Errorf("page %q not found in KB — make sure it has been written and is not in a skipped directory", pagePath)
	}

	suggestions := wikibrain.AuditBacklinks(newPage, others)
	printBacklinkSuggestions(newPage, suggestions)

	now := time.Now()

	if !wikiIngestNoAppend {
		if appendErr := appendToLog(kbPath, wikibrain.FormatIngestLogEntry(pagePath, len(suggestions), now)); appendErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not append to log.md: %v\n", appendErr)
		}
	}

	if !wikiIngestNoIndex {
		content := wikibrain.GenerateIndex(pages, now)
		indexPath := filepath.Join(kbPath, "index.md")
		if writeErr := os.WriteFile(indexPath, []byte(content), 0o644); writeErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not write index.md: %v\n", writeErr)
		} else {
			fmt.Printf("\n✅ index.md updated (%d pages)\n", len(pages))
		}
	}

	return nil
}

func resolvePagePath(kbPath, raw string) (string, error) {
	// Absolute path: make relative to KB root
	if filepath.IsAbs(raw) {
		rel, err := filepath.Rel(kbPath, raw)
		if err != nil {
			return "", fmt.Errorf("page %q is not under kb root %q", raw, kbPath)
		}
		return rel, nil
	}
	// Already relative — verify existence
	abs := filepath.Join(kbPath, raw)
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("page %q not found: %w", abs, err)
	}
	return raw, nil
}

func printBacklinkSuggestions(newPage *wikibrain.Page, suggestions []wikibrain.BacklinkSuggestion) {
	fmt.Printf("Backlink audit for: %s\n\n", newPage.RelPath)

	if len(suggestions) == 0 {
		fmt.Println("No backlink suggestions — no existing pages mention this topic.")
		return
	}

	fmt.Printf("Found %d page(s) that may benefit from linking to this page:\n\n", len(suggestions))
	for _, s := range suggestions {
		terms := strings.Join(s.MatchedTerms, ", ")
		fmt.Printf("  📄 %s\n     matched: %s\n     suggest adding: [[%s]]\n\n",
			s.SourcePage,
			terms,
			strings.TrimSuffix(filepath.Base(newPage.RelPath), ".md"),
		)
	}

	fmt.Println("Review each suggestion and add [[links]] where appropriate.")
}
