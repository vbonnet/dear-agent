package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors"
)

// OutputMetadata represents the metadata.json structure
type OutputMetadata struct {
	URL         string                 `json:"url"`
	ContentType string                 `json:"content_type"`
	Topics      []string               `json:"topics"`
	Timestamp   string                 `json:"timestamp"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// WriteOutput writes all output files to a timestamped directory
// Returns the output directory path
func WriteOutput(
	outputBaseDir string,
	url string,
	contentType detector.ContentType,
	content *extractors.Content,
	topics []string,
	report string,
) (string, error) {
	// Create timestamped directory name
	timestamp := time.Now().Format("20060102-150405")
	sanitizedURL := sanitizeURL(url)
	dirName := fmt.Sprintf("%s-%s", timestamp, sanitizedURL)
	outputDir := filepath.Join(outputBaseDir, dirName)

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	// Write metadata.json
	metadata := OutputMetadata{
		URL:         url,
		ContentType: contentType.String(),
		Topics:      topics,
		Timestamp:   time.Now().Format(time.RFC3339),
		Metadata:    content.Metadata,
	}

	metadataPath := filepath.Join(outputDir, "metadata.json")
	if err := writeJSON(metadataPath, metadata); err != nil {
		return "", fmt.Errorf("write metadata.json: %w", err)
	}

	// Write topics.json
	topicsData := map[string][]string{"topics": topics}
	topicsPath := filepath.Join(outputDir, "topics.json")
	if err := writeJSON(topicsPath, topicsData); err != nil {
		return "", fmt.Errorf("write topics.json: %w", err)
	}

	// Write report.md
	reportPath := filepath.Join(outputDir, "report.md")
	if err := os.WriteFile(reportPath, []byte(report), 0644); err != nil {
		return "", fmt.Errorf("write report.md: %w", err)
	}

	// Write content.txt (optional, for reference)
	contentPath := filepath.Join(outputDir, "content.txt")
	if err := os.WriteFile(contentPath, []byte(content.Raw), 0644); err != nil {
		return "", fmt.Errorf("write content.txt: %w", err)
	}

	return outputDir, nil
}

// sanitizeURL converts a URL into a safe directory name
func sanitizeURL(url string) string {
	// Remove protocol
	s := strings.TrimPrefix(url, "https://")
	s = strings.TrimPrefix(s, "http://")

	// Remove www.
	s = strings.TrimPrefix(s, "www.")

	// Replace slashes and special chars with hyphens
	s = regexp.MustCompile(`[:/\?&=]+`).ReplaceAllString(s, "-")

	// Remove trailing hyphens
	s = strings.Trim(s, "-")

	// Truncate to reasonable length
	if len(s) > 50 {
		s = s[:50]
	}

	// Remove trailing hyphens again after truncation
	s = strings.TrimRight(s, "-")

	return s
}

// writeJSON writes data as formatted JSON to a file
func writeJSON(path string, data interface{}) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	if err := os.WriteFile(path, jsonData, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
