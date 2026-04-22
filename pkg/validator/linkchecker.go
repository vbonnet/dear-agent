package validator

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/fatih/color"
)

// BrokenLink represents a broken link found in a file
type BrokenLink struct {
	FilePath string
	LinkPath string
	LinkText string
}

// LinkChecker validates internal .ai.md links in documentation files
type LinkChecker struct {
	contentDir  string
	brokenLinks []BrokenLink
	linkPattern *regexp.Regexp
}

// NewLinkChecker creates a new LinkChecker instance
func NewLinkChecker(contentDir string) (*LinkChecker, error) {
	absContentDir, err := filepath.Abs(contentDir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve content directory: %w", err)
	}

	// Pattern: [text](path) where path contains .ai.md
	linkPattern := regexp.MustCompile(`\[([^\]]*)\]\(([^)]*\.ai\.md)\)`)

	return &LinkChecker{
		contentDir:  absContentDir,
		brokenLinks: make([]BrokenLink, 0),
		linkPattern: linkPattern,
	}, nil
}

// CheckFile validates all internal .ai.md links in a single file
func (lc *LinkChecker) CheckFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fileContent := string(content)
	absFilePath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve file path: %w", err)
	}

	fileDir := filepath.Dir(absFilePath)

	// Find all matches
	matches := lc.linkPattern.FindAllStringSubmatchIndex(fileContent, -1)

	for _, match := range matches {
		// match[0], match[1] = full match start/end
		// match[2], match[3] = link text start/end
		// match[4], match[5] = link path start/end
		linkText := fileContent[match[2]:match[3]]
		linkPath := fileContent[match[4]:match[5]]

		// Check if this link should be ignored
		if lc.shouldIgnoreLink(fileContent, match[0]) {
			continue
		}

		// Resolve link path relative to file directory
		targetPath := filepath.Join(fileDir, linkPath)
		targetPath = filepath.Clean(targetPath)

		// Check if target exists
		if !fileExists(targetPath) {
			// Try relative to content root
			altTarget := filepath.Join(lc.contentDir, linkPath)
			altTarget = filepath.Clean(altTarget)

			if !fileExists(altTarget) {
				// Both paths failed, record broken link
				relFilePath, err := filepath.Rel(lc.contentDir, absFilePath)
				if err != nil {
					relFilePath = absFilePath
				}

				lc.brokenLinks = append(lc.brokenLinks, BrokenLink{
					FilePath: relFilePath,
					LinkPath: linkPath,
					LinkText: linkText,
				})
			}
		}
	}

	return nil
}

// shouldIgnoreLink checks if a link has an ignore comment on the previous line or same line
func (lc *LinkChecker) shouldIgnoreLink(content string, matchStart int) bool {
	// Find the last newline before the match start
	// This gives us the start of the previous line
	prevLineStart := strings.LastIndex(content[:matchStart], "\n")

	// Get content from previous line start (or beginning of file) to match start
	var searchContent string
	if prevLineStart == -1 {
		// No previous newline, search from start
		searchContent = content[:matchStart]
	} else {
		// Search from the newline (includes previous line and current line up to match)
		searchContent = content[prevLineStart:matchStart]
	}

	// Check if the ignore comment appears in this range
	return strings.Contains(searchContent, "<!-- link-check-ignore -->")
}

// CheckAll validates all .ai.md files in the content directory or specific files
func (lc *LinkChecker) CheckAll(files []string) (int, error) {
	var filePaths []string

	if len(files) > 0 {
		filePaths = files
	} else {
		// Find all .ai.md files recursively
		var err error
		filePaths, err = lc.findAllAiMdFiles()
		if err != nil {
			return 0, fmt.Errorf("failed to find .ai.md files: %w", err)
		}
	}

	for _, filePath := range filePaths {
		if fileExists(filePath) {
			if err := lc.CheckFile(filePath); err != nil {
				color.Red("Error checking %s: %v", filePath, err)
			}
		}
	}

	return len(lc.brokenLinks), nil
}

// findAllAiMdFiles recursively finds all .ai.md files in the content directory
func (lc *LinkChecker) findAllAiMdFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(lc.contentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(info.Name(), ".ai.md") {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// PrintResults displays the broken links report
func (lc *LinkChecker) PrintResults() {
	if len(lc.brokenLinks) == 0 {
		color.Green("✅ No broken links found")
		return
	}

	color.Yellow("⚠️  Broken Links Found: %d\n", len(lc.brokenLinks))

	// Group by file
	byFile := make(map[string][]BrokenLink)
	for _, link := range lc.brokenLinks {
		byFile[link.FilePath] = append(byFile[link.FilePath], link)
	}

	// Sort file paths for consistent output
	filePaths := make([]string, 0, len(byFile))
	for filePath := range byFile {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	for _, filePath := range filePaths {
		color.Red("❌ %s", filePath)
		for _, link := range byFile[filePath] {
			fmt.Printf("   → %s\n", link.LinkPath)
			if link.LinkText != "" {
				fmt.Printf("      (link text: '%s')\n", link.LinkText)
			}
		}
	}

	color.Blue("\nNote:")
	fmt.Println(" To ignore example links, add comment above link:")
	fmt.Println("  <!-- link-check-ignore -->")
	fmt.Println("  [example](file1.ai.md)")
}

// GetBrokenLinks returns the list of broken links found
func (lc *LinkChecker) GetBrokenLinks() []BrokenLink {
	return lc.brokenLinks
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
