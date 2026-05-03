// Package guidance provides guidance-related functionality.
package guidance

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SearchService handles guidance file searching.
type SearchService struct {
	engramPath string
}

// NewSearchService creates a new search service.
func NewSearchService(engramPath string) *SearchService {
	return &SearchService{engramPath: engramPath}
}

// SearchOptions defines search parameters.
type SearchOptions struct {
	Query  string
	Domain string
	Type   string
	Tag    string
	Limit  int
}

// SearchResult represents a matched guidance file.
type SearchResult struct {
	Path        string   `json:"path"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Score       int      `json:"score"`
	Domain      string   `json:"domain,omitempty"`
	Type        string   `json:"type,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Frontmatter represents YAML frontmatter from .ai.md file.
type Frontmatter struct {
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Domain      string   `yaml:"domain"`
	Type        string   `yaml:"type"`
	Tags        []string `yaml:"tags"`
}

// Search performs a guidance file search.
func (s *SearchService) Search(opts SearchOptions) ([]SearchResult, error) {
	// Collect guidance files
	files, err := s.collectGuidanceFiles()
	if err != nil {
		return nil, err
	}

	// Search and filter files
	results := s.searchFiles(files, opts)

	// Sort and limit results
	return s.sortAndLimitResults(results, opts.Limit), nil
}

// collectGuidanceFiles finds all .ai.md files in engram path
func (s *SearchService) collectGuidanceFiles() ([]string, error) {
	pattern := filepath.Join(s.engramPath, "**/*.ai.md")
	files, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to glob files: %w", err)
	}

	// Fallback to walk if glob doesn't support **
	if len(files) == 0 {
		files, err = s.walkDirectoryForFiles()
		if err != nil {
			return nil, err
		}
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no guidance files (*.ai.md) found in %s", s.engramPath)
	}

	return files, nil
}

// walkDirectoryForFiles walks directory to find .ai.md files
func (s *SearchService) walkDirectoryForFiles() ([]string, error) {
	var files []string
	err := filepath.Walk(s.engramPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".ai.md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}
	return files, nil
}

// searchFiles searches through files and returns matching results
func (s *SearchService) searchFiles(files []string, opts SearchOptions) []SearchResult {
	var results []SearchResult
	query := strings.ToLower(opts.Query)

	for _, filePath := range files {
		result, ok := s.processFile(filePath, query, opts)
		if ok {
			results = append(results, result)
		}
	}

	return results
}

// processFile parses and filters a single file
func (s *SearchService) processFile(filePath, query string, opts SearchOptions) (SearchResult, bool) {
	frontmatter, err := parseFrontmatter(filePath)
	if err != nil {
		log.Printf("WARNING: Failed to parse frontmatter from %s: %v", filePath, err)
		return SearchResult{}, false
	}

	// Apply filters
	if !s.matchesFilters(frontmatter, opts) {
		return SearchResult{}, false
	}

	// Calculate match score
	score := calculateScore(frontmatter, query)
	if score == 0 {
		return SearchResult{}, false
	}

	// Build result
	relPath, err := filepath.Rel(s.engramPath, filePath)
	if err != nil {
		relPath = filePath
	}

	return SearchResult{
		Path:        relPath,
		Title:       frontmatter.Title,
		Description: frontmatter.Description,
		Score:       score,
		Domain:      frontmatter.Domain,
		Type:        frontmatter.Type,
		Tags:        frontmatter.Tags,
	}, true
}

// matchesFilters checks if frontmatter matches filter criteria
func (s *SearchService) matchesFilters(fm *Frontmatter, opts SearchOptions) bool {
	if opts.Domain != "" && fm.Domain != opts.Domain {
		return false
	}
	if opts.Type != "" && fm.Type != opts.Type {
		return false
	}
	if opts.Tag != "" && !contains(fm.Tags, opts.Tag) {
		return false
	}
	return true
}

// sortAndLimitResults sorts by score and applies limit
func (s *SearchService) sortAndLimitResults(results []SearchResult, limit int) []SearchResult {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if limit > 0 && len(results) > limit {
		return results[:limit]
	}

	return results
}

// parseFrontmatter extracts YAML frontmatter from .ai.md file.
func parseFrontmatter(filePath string) (*Frontmatter, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Extract frontmatter (between --- delimiters)
	lines := strings.Split(string(content), "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return nil, fmt.Errorf("no frontmatter found (missing opening ---)")
	}

	// Find closing ---
	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx == -1 {
		return nil, fmt.Errorf("unclosed frontmatter (missing closing ---)")
	}

	// Parse YAML
	yamlContent := strings.Join(lines[1:endIdx], "\n")
	var frontmatter Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &frontmatter); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}

	return &frontmatter, nil
}

// calculateScore scores a frontmatter match against query.
func calculateScore(fm *Frontmatter, query string) int {
	score := 0
	queryLower := strings.ToLower(query)
	titleLower := strings.ToLower(fm.Title)
	descLower := strings.ToLower(fm.Description)

	// Title match: 3 points
	if strings.Contains(titleLower, queryLower) {
		score += 3
	}

	// Description match: 2 points
	if strings.Contains(descLower, queryLower) {
		score += 2
	}

	// Tag match: 1 point
	for _, tag := range fm.Tags {
		if strings.Contains(strings.ToLower(tag), queryLower) {
			score += 1
			break // Only count tags once
		}
	}

	return score
}

// contains checks if slice contains string (case-insensitive).
func contains(slice []string, item string) bool {
	itemLower := strings.ToLower(item)
	for _, s := range slice {
		if strings.ToLower(s) == itemLower {
			return true
		}
	}
	return false
}
