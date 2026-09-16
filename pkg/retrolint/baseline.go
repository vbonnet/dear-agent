package retrolint

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// NormalizePath standardizes retrospective paths for baseline indexing.
func NormalizePath(p string) string {
	p = filepath.Clean(p)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return p
}

// Baseline manages grandfathered retrospectives exempt from missing-guard requirements.
type Baseline struct {
	Entries []BaselineEntry
	byPath  map[string]BaselineEntry
}

// LoadBaseline parses a JSONL baseline store.
func LoadBaseline(r io.Reader) (*Baseline, error) {
	b := &Baseline{
		byPath: make(map[string]BaselineEntry),
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var entry BaselineEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return nil, fmt.Errorf("line %d: invalid JSON: %w", lineNum, err)
		}
		if entry.Path == "" {
			return nil, fmt.Errorf("line %d: path is required", lineNum)
		}
		k := NormalizePath(entry.Path)
		baseName := filepath.Base(k)
		b.byPath[k] = entry
		b.byPath[baseName] = entry
		b.Entries = append(b.Entries, entry)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("reading baseline store: %w", err)
	}
	return b, nil
}

// LoadBaselineFile loads a baseline store from a file path.
func LoadBaselineFile(path string) (*Baseline, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadBaseline(f)
}

// Contains reports whether a given path is waived by the grandfathered baseline.
func (b *Baseline) Contains(path string) bool {
	if b == nil {
		return false
	}
	norm := NormalizePath(path)
	if _, ok := b.byPath[norm]; ok {
		return true
	}
	base := filepath.Base(norm)
	if _, ok := b.byPath[base]; ok {
		return true
	}
	return false
}

// CheckRatchet enforces that baseline entries must not reference removed files
// or files that now declare valid guards (RLINT-06).
func CheckRatchet(ctx context.Context, repoRoot, retrosDir string, baseline *Baseline) ([]string, error) {
	if baseline == nil || len(baseline.Entries) == 0 {
		return nil, nil
	}
	var errors []string
	for _, entry := range baseline.Entries {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// Locate file
		fullPath := entry.Path
		if !filepath.IsAbs(fullPath) {
			candidate1 := filepath.Join(retrosDir, entry.Path)
			candidate2 := filepath.Join(repoRoot, entry.Path)
			if _, err := os.Stat(candidate1); err == nil {
				fullPath = candidate1
			} else if _, err := os.Stat(candidate2); err == nil {
				fullPath = candidate2
			} else {
				fullPath = candidate1
			}
		}

		fi, err := os.Stat(fullPath)
		if err != nil || fi.IsDir() {
			errors = append(errors, fmt.Sprintf("baseline entry %q references non-existent or removed file", entry.Path))
			continue
		}

		// Evaluate retrospective without baseline to see if it now passes on its own
		res, err := EvaluateRetrospectiveFile(ctx, repoRoot, fullPath, nil)
		if err == nil && res != nil && res.Status == StatusPass {
			errors = append(errors, fmt.Sprintf("baseline entry %q now declares valid guards; remove from baseline to ratchet down", entry.Path))
		}
	}
	return errors, nil
}
