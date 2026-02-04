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
	Mode        string                 `json:"mode"`                   // Analysis mode (general|competitive)
	Competitor  string                 `json:"competitor,omitempty"`   // Competitor name (competitive mode only)
	SourceQuery string                 `json:"source_query,omitempty"` // Original query (competitive mode only)
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// OutputConfig holds configuration for output generation
type OutputConfig struct {
	Mode        string // Analysis mode (general|competitive)
	Competitor  string // Competitor name (competitive mode only)
	SourceQuery string // Original query (competitive mode only)
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
	outputConfig *OutputConfig,
) (string, error) {
	// Default to general mode if not specified
	if outputConfig == nil {
		outputConfig = &OutputConfig{Mode: "general"}
	}

	// Create timestamped directory name
	timestamp := time.Now().Format("20060102-150405")
	sanitizedURL := sanitizeURL(url)

	// Add mode prefix for competitive analysis
	var dirName string
	if outputConfig.Mode == "competitive" {
		dirName = fmt.Sprintf("%s-competitive-%s", timestamp, sanitizedURL)
	} else {
		dirName = fmt.Sprintf("%s-%s", timestamp, sanitizedURL)
	}
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
		Mode:        outputConfig.Mode,
		Competitor:  outputConfig.Competitor,
		SourceQuery: outputConfig.SourceQuery,
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

	// Write competitive analysis summary (competitive mode only)
	if outputConfig.Mode == "competitive" {
		summaryPath := filepath.Join(outputDir, "competitive-summary.md")
		summary := generateCompetitiveSummary(outputConfig.Competitor, url, topics, report)
		if err := os.WriteFile(summaryPath, []byte(summary), 0644); err != nil {
			return "", fmt.Errorf("write competitive-summary.md: %w", err)
		}
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

// generateCompetitiveSummary creates a formatted summary for competitive analysis
func generateCompetitiveSummary(competitor string, url string, topics []string, report string) string {
	var sb strings.Builder

	// Header
	sb.WriteString("# Competitive Analysis Summary\n\n")
	sb.WriteString(fmt.Sprintf("**Competitor:** %s\n\n", competitor))
	sb.WriteString(fmt.Sprintf("**Analyzed URL:** %s\n\n", url))
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", time.Now().Format("2006-01-02 15:04:05")))
	sb.WriteString("---\n\n")

	// Analysis Focus Areas
	sb.WriteString("## Analysis Focus Areas\n\n")
	if len(topics) > 0 {
		for i, topic := range topics {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, topic))
		}
	} else {
		sb.WriteString("_No specific focus areas identified_\n")
	}
	sb.WriteString("\n---\n\n")

	// Extract key sections from report
	sb.WriteString("## Gap Analysis Report\n\n")
	sb.WriteString("The full gap analysis report has been generated based on the competitor's capabilities.\n\n")
	sb.WriteString("### Key Sections to Review:\n\n")
	sb.WriteString("- **Competitor Strengths:** Features and capabilities where the competitor excels\n")
	sb.WriteString("- **Our Gaps:** Areas where our tool lacks compared to the competitor\n")
	sb.WriteString("- **Prioritized Recommendations:** Actionable improvements ranked by impact and effort\n")
	sb.WriteString("- **Strategic Insights:** Technical differences and architectural considerations\n\n")
	sb.WriteString("📄 **See `report.md` for the complete detailed analysis**\n\n")

	// Extract urgency/priority if present in report
	if strings.Contains(strings.ToLower(report), "critical") ||
	   strings.Contains(strings.ToLower(report), "high priority") {
		sb.WriteString("---\n\n")
		sb.WriteString("⚠️  **Critical Items Identified**\n\n")
		sb.WriteString("The analysis has identified high-priority or critical gaps. ")
		sb.WriteString("Review the full report for immediate action items.\n\n")
	}

	// Footer
	sb.WriteString("---\n\n")
	sb.WriteString("## Files in This Analysis\n\n")
	sb.WriteString("- `competitive-summary.md` - This executive summary (current file)\n")
	sb.WriteString("- `report.md` - Full detailed gap analysis report\n")
	sb.WriteString("- `metadata.json` - Analysis metadata and configuration\n")
	sb.WriteString("- `topics.json` - Identified focus areas\n")
	sb.WriteString("- `content.txt` - Extracted competitor content\n\n")
	sb.WriteString("_Generated by gemini-deep-research competitive analysis mode_\n")

	return sb.String()
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
