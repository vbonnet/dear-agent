package hippocampus

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MemoryFile represents a scanned memory file with parsed frontmatter.
type MemoryFile struct {
	Path        string
	Name        string // filename without directory
	Description string // from frontmatter "description:" field
	Type        string // from frontmatter "type:" field (user, feedback, project, reference)
	ModTime     time.Time
	Preview     string // first 30 lines of content (after frontmatter)
}

// RelevanceResult represents an LLM-scored memory file for a query.
type RelevanceResult struct {
	File  MemoryFile
	Score float64 // 0.0-1.0 relevance score
}

// ScanMemoryFiles reads memory files from dir, parses frontmatter from the first
// 30 lines, sorts by mtime (newest first), and caps at 200 files.
// Skips MEMORY.md and non-.md files.
func ScanMemoryFiles(dir string) ([]MemoryFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read memory dir: %w", err)
	}

	var files []MemoryFile
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || name == "MEMORY.md" {
			continue
		}

		path := filepath.Join(dir, name)

		// Security: validate path, normalize unicode, check symlinks
		validatedPath, err := ValidateMemoryFile(path, dir)
		if err != nil {
			continue // skip files that fail security checks
		}
		path = validatedPath

		mf, err := scanOneFile(path, filepath.Base(path))
		if err != nil {
			continue // skip unreadable files
		}
		files = append(files, mf)
	}

	// Sort newest first by ModTime
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModTime.After(files[j].ModTime)
	})

	// Cap at 200 files
	if len(files) > 200 {
		files = files[:200]
	}

	return files, nil
}

// scanOneFile reads the first 30 lines of a file and extracts frontmatter fields.
func scanOneFile(path, name string) (MemoryFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return MemoryFile{}, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return MemoryFile{}, err
	}

	mf := MemoryFile{
		Path:    path,
		Name:    name,
		ModTime: info.ModTime(),
	}

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	pastFrontmatter := false
	lineNum := 0
	var previewLines []string

	for scanner.Scan() && lineNum < 30 {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if trimmed == "---" {
			if !inFrontmatter && !pastFrontmatter {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				inFrontmatter = false
				pastFrontmatter = true
				continue
			}
		}

		if inFrontmatter {
			if strings.HasPrefix(trimmed, "description:") {
				mf.Description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			} else if strings.HasPrefix(trimmed, "type:") {
				mf.Type = strings.TrimSpace(strings.TrimPrefix(trimmed, "type:"))
			}
			continue
		}

		previewLines = append(previewLines, line)
	}

	mf.Preview = strings.Join(previewLines, "\n")
	return mf, nil
}

// RelevanceQuery is sent to the LLM for relevance scoring.
type RelevanceQuery struct {
	Query string       `json:"query"`
	Files []fileDigest `json:"files"`
}

type fileDigest struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	AgeDays     string `json:"age"`
}

// RelevanceResponse is the expected JSON response from the LLM.
type RelevanceResponse struct {
	Selected []struct {
		Index int     `json:"index"`
		Score float64 `json:"score"`
	} `json:"selected"`
}

// SideQueryFunc is a function that sends a prompt to an LLM and returns the response.
// This matches the "sideQuery" pattern from Claude Code — a lightweight LLM call
// with a small token budget for scoring/classification tasks.
type SideQueryFunc func(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error)

// FindRelevantMemories uses an LLM sideQuery to select the most relevant memory
// files for a given query. Returns up to 5 results, sorted by relevance score.
// Files in excludePaths are skipped (already surfaced).
func FindRelevantMemories(ctx context.Context, query string, files []MemoryFile, sideQuery SideQueryFunc, excludePaths map[string]bool) ([]RelevanceResult, error) {
	if len(files) == 0 || sideQuery == nil {
		return nil, nil
	}

	// Filter out already-surfaced files
	var candidates []MemoryFile
	for _, f := range files {
		if excludePaths != nil && excludePaths[f.Path] {
			continue
		}
		candidates = append(candidates, f)
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// Build file digests for the LLM
	digests := make([]fileDigest, len(candidates))
	for i, f := range candidates {
		age := "today"
		days, err := MemoryAgeDays(f.Path)
		if err == nil {
			switch {
			case days == 0:
				age = "today"
			case days == 1:
				age = "yesterday"
			default:
				age = fmt.Sprintf("%d days ago", days)
			}
		}
		digests[i] = fileDigest{
			Index:       i,
			Name:        f.Name,
			Description: f.Description,
			Type:        f.Type,
			AgeDays:     age,
		}
	}

	rq := RelevanceQuery{
		Query: query,
		Files: digests,
	}
	queryJSON, err := json.Marshal(rq)
	if err != nil {
		return nil, fmt.Errorf("marshal relevance query: %w", err)
	}

	systemPrompt := `You are a memory relevance scorer. Given a user query and a list of memory files with descriptions, select the most relevant files.

Return JSON with a "selected" array containing objects with "index" (file index) and "score" (0.0-1.0 relevance). Select at most 5 files. Only include files with score >= 0.3. Order by score descending.

Example response:
{"selected": [{"index": 2, "score": 0.9}, {"index": 0, "score": 0.6}]}`

	response, err := sideQuery(ctx, systemPrompt, string(queryJSON), 256)
	if err != nil {
		return nil, fmt.Errorf("relevance sideQuery: %w", err)
	}

	// Parse JSON response
	var rr RelevanceResponse
	if err := json.Unmarshal([]byte(response), &rr); err != nil {
		// Try to extract JSON from response (LLM may wrap in markdown)
		if jsonStr := extractJSON(response); jsonStr != "" {
			if err2 := json.Unmarshal([]byte(jsonStr), &rr); err2 != nil {
				return nil, fmt.Errorf("parse relevance response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("parse relevance response: %w", err)
		}
	}

	// Build results with staleness caveats
	var results []RelevanceResult
	for _, sel := range rr.Selected {
		if sel.Index < 0 || sel.Index >= len(candidates) {
			continue
		}
		if sel.Score < 0.3 {
			continue
		}
		results = append(results, RelevanceResult{
			File:  candidates[sel.Index],
			Score: sel.Score,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Cap at 5
	if len(results) > 5 {
		results = results[:5]
	}

	return results, nil
}

// SurfaceRelevantMemories scans a memory directory, finds relevant files for the
// query, and returns their content with staleness caveats injected.
func SurfaceRelevantMemories(ctx context.Context, memoryDir, query string, sideQuery SideQueryFunc, excludePaths map[string]bool) ([]string, error) {
	files, err := ScanMemoryFiles(memoryDir)
	if err != nil {
		return nil, err
	}

	results, err := FindRelevantMemories(ctx, query, files, sideQuery, excludePaths)
	if err != nil {
		return nil, err
	}

	var contents []string
	for _, r := range results {
		content, err := SurfaceMemoryWithFreshness(r.File.Path)
		if err != nil {
			continue
		}
		contents = append(contents, content)
	}

	return contents, nil
}

// extractJSON attempts to find a JSON object in a string that may contain
// markdown code fences or other wrapper text.
func extractJSON(s string) string {
	// Try to find JSON between { and }
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	end := strings.LastIndex(s, "}")
	if end < start {
		return ""
	}
	return s[start : end+1]
}
