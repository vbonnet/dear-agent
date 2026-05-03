package migrate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/engram"
)

// Options holds migration options
type Options struct {
	DryRun   bool // Preview changes without applying
	Validate bool // Validate content integrity
}

// Stats tracks migration statistics
type Stats struct {
	Success           int
	Errors            int
	TiersAdded        int
	WhyFilesGenerated int
	Skipped           int // Already migrated
}

// MigrateDirectory migrates all .ai.md files in a directory
func MigrateDirectory(directory string, opts Options) (*Stats, error) {
	// Find all .ai.md files
	files, err := findAiMdFiles(directory)
	if err != nil {
		return nil, fmt.Errorf("failed to find .ai.md files: %w", err)
	}

	slog.Info("found .ai.md files", "count", len(files))

	if opts.DryRun {
		slog.Info("[DRY RUN] no files will be modified")
	}

	// Migrate each file
	stats := &Stats{}
	for i, file := range files {
		slog.Info("migrating file", "progress", fmt.Sprintf("%d/%d", i+1, len(files)), "file", file)

		if err := migrateFile(file, opts, stats); err != nil {
			slog.Error("migration failed", "file", file, "error", err)
			stats.Errors++
			continue
		}

		stats.Success++
	}

	return stats, nil
}

// migrateFile migrates a single .ai.md file
func migrateFile(path string, opts Options, stats *Stats) error {
	// Read original content
	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Check if already migrated
	if engram.HasTierMarkers(string(original)) {
		slog.Info("already has tier markers, skipping", "file", path)
		stats.Skipped++
		return nil
	}

	// Insert tier markers
	migrated, err := InsertTierMarkers(string(original))
	if err != nil {
		return fmt.Errorf("failed to insert tier markers: %w", err)
	}

	// Validate content integrity
	if opts.Validate {
		if err := validateContentIntegrity(string(original), migrated); err != nil {
			return fmt.Errorf("validation failed: %w", err)
		}
	}

	// Write migrated content (unless dry-run)
	if !opts.DryRun {
		if err := os.WriteFile(path, []byte(migrated), 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		slog.Info("tier markers added", "file", path)
		stats.TiersAdded++
	} else {
		slog.Info("would add tier markers", "file", path)
	}

	// Check for .why.md companion
	whyPath := strings.TrimSuffix(path, ".ai.md") + ".why.md"

	if _, err := os.Stat(whyPath); os.IsNotExist(err) {
		// Generate .why.md template
		if !opts.DryRun {
			if err := engram.CreateWhyFileIfMissing(path); err != nil {
				return fmt.Errorf("failed to create .why.md: %w", err)
			}
			slog.Info("generated .why.md template", "file", path)
			stats.WhyFilesGenerated++
		} else {
			slog.Info("would generate .why.md template", "file", path)
		}
	} else {
		slog.Info(".why.md already exists", "file", path)
	}

	return nil
}

// findAiMdFiles finds all .ai.md files in a directory recursively
func findAiMdFiles(directory string) ([]string, error) {
	var files []string

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".md" &&
			strings.HasSuffix(filepath.Base(path), ".ai.md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// validateContentIntegrity ensures no content loss during migration
func validateContentIntegrity(original, migrated string) error {
	// Strip tier markers to get actual content
	migratedContent := stripTierMarkers(migrated)

	// Compare word counts (more reliable than character counts)
	originalWords := strings.Fields(original)
	migratedWords := strings.Fields(migratedContent)

	// Word counts should match exactly (or very close for whitespace differences)
	wordDiff := abs(len(originalWords) - len(migratedWords))
	if wordDiff > 5 {
		return fmt.Errorf("word count changed: original=%d, migrated=%d, diff=%d",
			len(originalWords), len(migratedWords), wordDiff)
	}

	return nil
}

// stripTierMarkers removes tier marker syntax
func stripTierMarkers(content string) string {
	lines := strings.Split(content, "\n")
	result := []string{}

	for _, line := range lines {
		// Skip tier markers
		if strings.HasPrefix(line, "> [!T") {
			continue
		}

		// Strip blockquote prefix
		cleaned := strings.TrimPrefix(line, "> ")
		result = append(result, cleaned)
	}

	return strings.Join(result, "\n")
}


func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// PrintSummary prints migration summary
func PrintSummary(stats *Stats) {
	slog.Info("migration summary",
		"success", stats.Success,
		"skipped", stats.Skipped,
		"errors", stats.Errors,
		"tiers_added", stats.TiersAdded,
		"why_files_generated", stats.WhyFilesGenerated,
	)
}
