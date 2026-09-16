package retrolint

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// GenerateBaseline scans retrospectives in retrosDir and produces grandfathered baseline entries.
func GenerateBaseline(ctx context.Context, repoRoot, retrosDir string) ([]BaselineEntry, error) {
	opts := Options{
		RepoRoot:  repoRoot,
		RetrosDir: retrosDir,
	}
	files, err := discoverFiles(opts)
	if err != nil {
		return nil, err
	}

	today := time.Now().UTC().Format("2006-01-02")
	var entries []BaselineEntry

	for _, f := range files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		res, err := EvaluateRetrospectiveFile(ctx, repoRoot, f, nil)
		if err != nil || res.Status != StatusPass {
			relPath := f
			if retrosDir != "" {
				if r, relErr := filepath.Rel(retrosDir, f); relErr == nil && !strings.HasPrefix(r, "..") {
					relPath = r
				}
			}
			entries = append(entries, BaselineEntry{
				Path:   NormalizePath(relPath),
				Reason: "grandfathered prior to machine-enforced retro-lint",
				Added:  today,
				Status: "grandfathered",
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

// WriteBaseline writes baseline entries as canonical single-line JSONL.
func WriteBaseline(w io.Writer, entries []BaselineEntry) error {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return fmt.Errorf("encoding baseline entry: %w", err)
		}
	}
	return nil
}
