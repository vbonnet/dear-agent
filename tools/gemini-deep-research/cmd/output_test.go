package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/detector"
	"github.com/vbonnet/ai-tools/tools/gemini-deep-research/extractors"
)

// TestWriteOutput_DirectoryNaming tests that output directories are named correctly for each mode
func TestWriteOutput_DirectoryNaming(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		outputConfig    *OutputConfig
		expectDirPrefix string // What should appear in directory name
	}{
		{
			name: "General mode - no competitive prefix",
			url:  "https://www.youtube.com/watch?v=VIDEO_ID",
			outputConfig: &OutputConfig{
				Mode: "general",
			},
			expectDirPrefix: "-youtube.com-watch", // Should NOT contain "competitive"
		},
		{
			name: "Competitive mode - includes competitive prefix",
			url:  "https://github.com/features/copilot",
			outputConfig: &OutputConfig{
				Mode:        "competitive",
				Competitor:  "GitHub Copilot",
				SourceQuery: "GitHub Copilot vs Cursor",
			},
			expectDirPrefix: "-competitive-github.com-features-copilot",
		},
		{
			name: "Competitive mode with complex URL",
			url:  "https://www.example.com/blog/post?id=123",
			outputConfig: &OutputConfig{
				Mode:        "competitive",
				Competitor:  "Example Blog",
				SourceQuery: "competitor analysis",
			},
			expectDirPrefix: "-competitive-example.com-blog-post",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp output directory
			tempDir := t.TempDir()

			// Create test content
			content := &extractors.Content{
				Raw:      "test content",
				Metadata: map[string]interface{}{},
			}
			topics := []string{"topic1", "topic2"}
			report := "test report"

			// Write output
			outputPath, err := WriteOutput(
				tempDir,
				tt.url,
				detector.ContentTypeVideo,
				content,
				topics,
				report,
				tt.outputConfig,
			)
			if err != nil {
				t.Fatalf("WriteOutput failed: %v", err)
			}

			// Check directory name contains expected prefix
			dirName := filepath.Base(outputPath)
			if !strings.Contains(dirName, tt.expectDirPrefix) {
				t.Errorf("Directory name %q does not contain expected prefix %q", dirName, tt.expectDirPrefix)
			}

			// For general mode, ensure "competitive" is NOT in the name
			if tt.outputConfig.Mode == "general" && strings.Contains(dirName, "competitive") {
				t.Errorf("General mode directory %q should not contain 'competitive'", dirName)
			}

			// For competitive mode, ensure "competitive" IS in the name
			if tt.outputConfig.Mode == "competitive" && !strings.Contains(dirName, "competitive") {
				t.Errorf("Competitive mode directory %q should contain 'competitive'", dirName)
			}
		})
	}
}

// TestWriteOutput_MetadataStructure tests that metadata.json contains correct mode-specific fields
func TestWriteOutput_MetadataStructure(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		outputConfig *OutputConfig
		checkMeta    func(t *testing.T, meta OutputMetadata)
	}{
		{
			name: "General mode - no competitor fields",
			url:  "https://www.youtube.com/watch?v=VIDEO_ID",
			outputConfig: &OutputConfig{
				Mode: "general",
			},
			checkMeta: func(t *testing.T, meta OutputMetadata) {
				if meta.Mode != "general" {
					t.Errorf("Expected mode 'general', got %q", meta.Mode)
				}
				if meta.Competitor != "" {
					t.Errorf("General mode should have empty Competitor, got %q", meta.Competitor)
				}
				if meta.SourceQuery != "" {
					t.Errorf("General mode should have empty SourceQuery, got %q", meta.SourceQuery)
				}
			},
		},
		{
			name: "Competitive mode - includes competitor fields",
			url:  "https://github.com/features/copilot",
			outputConfig: &OutputConfig{
				Mode:        "competitive",
				Competitor:  "GitHub Copilot",
				SourceQuery: "GitHub Copilot vs Cursor",
			},
			checkMeta: func(t *testing.T, meta OutputMetadata) {
				if meta.Mode != "competitive" {
					t.Errorf("Expected mode 'competitive', got %q", meta.Mode)
				}
				if meta.Competitor != "GitHub Copilot" {
					t.Errorf("Expected competitor 'GitHub Copilot', got %q", meta.Competitor)
				}
				if meta.SourceQuery != "GitHub Copilot vs Cursor" {
					t.Errorf("Expected source query 'GitHub Copilot vs Cursor', got %q", meta.SourceQuery)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp output directory
			tempDir := t.TempDir()

			// Create test content
			content := &extractors.Content{
				Raw:      "test content",
				Metadata: map[string]interface{}{"key": "value"},
			}
			topics := []string{"topic1", "topic2"}
			report := "test report"

			// Write output
			outputPath, err := WriteOutput(
				tempDir,
				tt.url,
				detector.ContentTypeVideo,
				content,
				topics,
				report,
				tt.outputConfig,
			)
			if err != nil {
				t.Fatalf("WriteOutput failed: %v", err)
			}

			// Read metadata.json
			metadataPath := filepath.Join(outputPath, "metadata.json")
			metadataBytes, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatalf("Failed to read metadata.json: %v", err)
			}

			// Parse metadata
			var meta OutputMetadata
			if err := json.Unmarshal(metadataBytes, &meta); err != nil {
				t.Fatalf("Failed to parse metadata.json: %v", err)
			}

			// Check common fields
			if meta.URL != tt.url {
				t.Errorf("Expected URL %q, got %q", tt.url, meta.URL)
			}
			if meta.ContentType != "Video" {
				t.Errorf("Expected content type 'Video', got %q", meta.ContentType)
			}
			if len(meta.Topics) != 2 {
				t.Errorf("Expected 2 topics, got %d", len(meta.Topics))
			}

			// Run mode-specific checks
			tt.checkMeta(t, meta)
		})
	}
}

// TestWriteOutput_CompetitiveSummaryGeneration tests that competitive-summary.md is generated correctly
func TestWriteOutput_CompetitiveSummaryGeneration(t *testing.T) {
	tests := []struct {
		name              string
		outputConfig      *OutputConfig
		expectSummaryFile bool
	}{
		{
			name: "General mode - no summary file",
			outputConfig: &OutputConfig{
				Mode: "general",
			},
			expectSummaryFile: false,
		},
		{
			name: "Competitive mode - summary file created",
			outputConfig: &OutputConfig{
				Mode:        "competitive",
				Competitor:  "GitHub Copilot",
				SourceQuery: "GitHub Copilot vs Cursor",
			},
			expectSummaryFile: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp output directory
			tempDir := t.TempDir()

			// Create test content
			content := &extractors.Content{
				Raw:      "test content",
				Metadata: map[string]interface{}{},
			}
			topics := []string{"topic1", "topic2"}
			report := "test report"

			// Write output
			outputPath, err := WriteOutput(
				tempDir,
				"https://example.com",
				detector.ContentTypeArticle,
				content,
				topics,
				report,
				tt.outputConfig,
			)
			if err != nil {
				t.Fatalf("WriteOutput failed: %v", err)
			}

			// Check if competitive-summary.md exists
			summaryPath := filepath.Join(outputPath, "competitive-summary.md")
			_, err = os.Stat(summaryPath)
			summaryExists := !os.IsNotExist(err)

			if tt.expectSummaryFile && !summaryExists {
				t.Error("Expected competitive-summary.md to be created, but it was not")
			}
			if !tt.expectSummaryFile && summaryExists {
				t.Error("Did not expect competitive-summary.md to be created, but it was")
			}
		})
	}
}

// TestGenerateCompetitiveSummary tests the summary generation function
func TestGenerateCompetitiveSummary(t *testing.T) {
	tests := []struct {
		name       string
		competitor string
		url        string
		topics     []string
		report     string
		checkFn    func(t *testing.T, summary string)
	}{
		{
			name:       "Basic summary with topics",
			competitor: "GitHub Copilot",
			url:        "https://github.com/features/copilot",
			topics:     []string{"Code completion", "AI assistance", "IDE integration"},
			report:     "test report content",
			checkFn: func(t *testing.T, summary string) {
				// Check header
				if !strings.Contains(summary, "# Competitive Analysis Summary") {
					t.Error("Expected summary to contain main header")
				}
				if !strings.Contains(summary, "GitHub Copilot") {
					t.Error("Expected summary to contain competitor name")
				}
				if !strings.Contains(summary, "https://github.com/features/copilot") {
					t.Error("Expected summary to contain analyzed URL")
				}

				// Check topics section
				if !strings.Contains(summary, "## Analysis Focus Areas") {
					t.Error("Expected summary to contain focus areas section")
				}
				if !strings.Contains(summary, "Code completion") {
					t.Error("Expected summary to contain first topic")
				}
				if !strings.Contains(summary, "AI assistance") {
					t.Error("Expected summary to contain second topic")
				}

				// Check key sections
				if !strings.Contains(summary, "## Gap Analysis Report") {
					t.Error("Expected summary to contain gap analysis section")
				}
				if !strings.Contains(summary, "Competitor Strengths") {
					t.Error("Expected summary to mention competitor strengths")
				}
				if !strings.Contains(summary, "Our Gaps") {
					t.Error("Expected summary to mention gaps")
				}

				// Check file list
				if !strings.Contains(summary, "competitive-summary.md") {
					t.Error("Expected summary to list itself")
				}
				if !strings.Contains(summary, "report.md") {
					t.Error("Expected summary to list report.md")
				}
			},
		},
		{
			name:       "Empty topics list",
			competitor: "Competitor X",
			url:        "https://example.com",
			topics:     []string{},
			report:     "test report",
			checkFn: func(t *testing.T, summary string) {
				if !strings.Contains(summary, "No specific focus areas identified") {
					t.Error("Expected summary to handle empty topics list")
				}
			},
		},
		{
			name:       "Report with critical items",
			competitor: "Competitor Y",
			url:        "https://example.com",
			topics:     []string{"topic1"},
			report:     "This is a CRITICAL issue that needs attention. High priority item found.",
			checkFn: func(t *testing.T, summary string) {
				if !strings.Contains(summary, "Critical Items Identified") {
					t.Error("Expected summary to detect critical items")
				}
				if !strings.Contains(summary, "high-priority") {
					t.Error("Expected summary to mention high-priority items")
				}
			},
		},
		{
			name:       "Report without critical items",
			competitor: "Competitor Z",
			url:        "https://example.com",
			topics:     []string{"topic1"},
			report:     "This is a normal report with no urgent items.",
			checkFn: func(t *testing.T, summary string) {
				if strings.Contains(summary, "Critical Items Identified") {
					t.Error("Did not expect critical items section in non-critical report")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := generateCompetitiveSummary(tt.competitor, tt.url, tt.topics, tt.report)
			tt.checkFn(t, summary)
		})
	}
}

// TestWriteOutput_AllFilesCreated tests that all expected files are created
func TestWriteOutput_AllFilesCreated(t *testing.T) {
	tests := []struct {
		name          string
		outputConfig  *OutputConfig
		expectedFiles []string
	}{
		{
			name: "General mode files",
			outputConfig: &OutputConfig{
				Mode: "general",
			},
			expectedFiles: []string{
				"metadata.json",
				"topics.json",
				"report.md",
				"content.txt",
			},
		},
		{
			name: "Competitive mode files",
			outputConfig: &OutputConfig{
				Mode:        "competitive",
				Competitor:  "Test Competitor",
				SourceQuery: "test query",
			},
			expectedFiles: []string{
				"metadata.json",
				"topics.json",
				"report.md",
				"content.txt",
				"competitive-summary.md",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp output directory
			tempDir := t.TempDir()

			// Create test content
			content := &extractors.Content{
				Raw:      "test content",
				Metadata: map[string]interface{}{},
			}
			topics := []string{"topic1"}
			report := "test report"

			// Write output
			outputPath, err := WriteOutput(
				tempDir,
				"https://example.com",
				detector.ContentTypeArticle,
				content,
				topics,
				report,
				tt.outputConfig,
			)
			if err != nil {
				t.Fatalf("WriteOutput failed: %v", err)
			}

			// Check all expected files exist
			for _, filename := range tt.expectedFiles {
				filePath := filepath.Join(outputPath, filename)
				if _, err := os.Stat(filePath); os.IsNotExist(err) {
					t.Errorf("Expected file %q was not created", filename)
				}
			}
		})
	}
}
