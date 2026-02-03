package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Research represents research data to be cached
type Research struct {
	URL         string
	ContentHash string
	Topics      []string
	Content     string
	ContentType string // youtube|arxiv|article|pdf
}

// Frontmatter represents the YAML frontmatter structure
type Frontmatter struct {
	URL          string   `yaml:"url"`
	ContentHash  string   `yaml:"content_hash"`
	ResearchedAt string   `yaml:"researched_at"`
	Topics       []string `yaml:"topics"`
	Version      int      `yaml:"version"`
}

// Check searches for cached research by URL
// Returns path to existing research if found
func Check(url string, cacheDir string, contentType string) (resultPath string, exists bool, err error) {
	normalizedURL, err := Normalize(url, contentType)
	if err != nil {
		return "", false, fmt.Errorf("normalize URL: %w", err)
	}

	// Walk cache directory
	err = filepath.WalkDir(cacheDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// Skip permission errors gracefully
			return nil
		}

		// Only check report.md files
		if d.IsDir() || !strings.HasSuffix(d.Name(), "report.md") {
			return nil
		}

		// Read frontmatter
		fm, err := readFrontmatter(path)
		if err != nil {
			// Skip invalid frontmatter files
			return nil
		}

		// Compare URLs
		if fm.URL == normalizedURL {
			exists = true
			resultPath = filepath.Dir(path) // Return directory, not file
			return filepath.SkipAll          // Found match, stop walking
		}

		return nil
	})

	if err != nil {
		return "", false, fmt.Errorf("walk cache dir: %w", err)
	}

	return resultPath, exists, nil
}

// Write saves research to cache with frontmatter
func Write(research *Research, cacheDir string, force bool) (string, error) {
	// Generate slug
	slug := GenerateSlug(research.URL)

	// Build output path
	outputPath := filepath.Join(cacheDir, research.ContentType, slug)

	// Check for existing directory
	version := 1
	if _, err := os.Stat(outputPath); err == nil {
		if force {
			// Find next version number
			version = findNextVersion(outputPath)
			timestamp := time.Now().Format("2006-01-02")
			outputPath = filepath.Join(cacheDir, research.ContentType, fmt.Sprintf("%s-%s", slug, timestamp))
		} else {
			// Collision without force flag - append hash
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(research.URL)))[:8]
			outputPath = filepath.Join(cacheDir, research.ContentType, fmt.Sprintf("%s-%s", slug, hash))
		}
	}

	// Create directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Build frontmatter
	fm := Frontmatter{
		URL:          research.URL,
		ContentHash:  research.ContentHash,
		ResearchedAt: time.Now().UTC().Format(time.RFC3339),
		Topics:       research.Topics,
		Version:      version,
	}

	// Marshal frontmatter
	fmBytes, err := yaml.Marshal(fm)
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}

	// Write report.md with frontmatter
	reportPath := filepath.Join(outputPath, "report.md")
	content := fmt.Sprintf("---\n%s---\n\n%s", string(fmBytes), research.Content)
	if err := os.WriteFile(reportPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write report file: %w", err)
	}

	return outputPath, nil
}

// readFrontmatter extracts and parses YAML frontmatter from file
func readFrontmatter(path string) (*Frontmatter, error) {
	// Read first 2KB (frontmatter is typically <1KB)
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, 2048)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return nil, err
	}

	content := string(buf[:n])

	// Find frontmatter delimiters
	if !strings.HasPrefix(content, "---\n") {
		return nil, fmt.Errorf("no frontmatter found")
	}

	endIdx := strings.Index(content[4:], "\n---")
	if endIdx == -1 {
		return nil, fmt.Errorf("unclosed frontmatter")
	}

	fmContent := content[4 : 4+endIdx]

	// Parse YAML
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	return &fm, nil
}

// findNextVersion finds next version number for existing slug
func findNextVersion(baseDir string) int {
	// Read existing report.md
	reportPath := filepath.Join(baseDir, "report.md")
	fm, err := readFrontmatter(reportPath)
	if err != nil {
		return 1
	}

	return fm.Version + 1
}
