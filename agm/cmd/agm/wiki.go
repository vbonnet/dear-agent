package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const defaultKBPath = "~/src/engram-kb"

var (
	wikiKBPath string // --kb flag shared across wiki subcommands
)

var wikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "Knowledge-base operations for engram-kb",
	Long: `wiki provides lint, index, and backlink operations for the engram-kb
markdown knowledge base.

Subcommands:
  lint    — scan for broken links, orphans, stale pages, and coverage gaps
  index   — regenerate index.md catalog
  ingest  — audit backlinks after adding a new page

By default all subcommands operate on ` + defaultKBPath + `.
Override with --kb.`,
}

func init() {
	rootCmd.AddCommand(wikiCmd)
	wikiCmd.PersistentFlags().StringVar(&wikiKBPath, "kb", defaultKBPath,
		"path to the engram-kb root directory")
}

// resolveKBPath expands ~ and returns an absolute path.
func resolveKBPath(raw string) (string, error) {
	if raw == "" {
		raw = defaultKBPath
	}
	if len(raw) >= 2 && raw[:2] == "~/" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot resolve home directory: %w", err)
		}
		raw = filepath.Join(home, raw[2:])
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("kb path %q does not exist: %w", abs, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("kb path %q is not a directory", abs)
	}
	return abs, nil
}
