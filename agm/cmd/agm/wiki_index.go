package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/pkg/wikibrain"
)

var wikiIndexDryRun bool

var wikiIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Regenerate index.md — flat catalog of every page in engram-kb",
	Long: `index scans all pages and writes a grouped, link-annotated catalog to
index.md at the KB root. Run this after ingesting new content.

Examples:
  agm wiki index
  agm wiki index --kb ~/src/engram-kb
  agm wiki index --dry-run     # print to stdout, do not write`,
	RunE: runWikiIndex,
}

func init() {
	wikiCmd.AddCommand(wikiIndexCmd)
	wikiIndexCmd.Flags().BoolVar(&wikiIndexDryRun, "dry-run", false,
		"print generated index.md to stdout instead of writing it")
}

func runWikiIndex(cmd *cobra.Command, _ []string) error {
	kbPath, err := resolveKBPath(wikiKBPath)
	if err != nil {
		return err
	}

	pages, err := wikibrain.ScanKB(kbPath)
	if err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	now := time.Now()
	content := wikibrain.GenerateIndex(pages, now)

	if wikiIndexDryRun {
		fmt.Print(content)
		return nil
	}

	indexPath := filepath.Join(kbPath, "index.md")
	if err := os.WriteFile(indexPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write index.md: %w", err)
	}
	fmt.Printf("✅ Wrote %s (%d pages)\n", indexPath, len(pages))

	if appendErr := appendToLog(kbPath, wikibrain.FormatIndexLogEntry(len(pages), now)); appendErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not append to log.md: %v\n", appendErr)
	}

	return nil
}
