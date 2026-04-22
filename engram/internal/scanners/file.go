package scanners

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// FileScanner detects languages and frameworks by analyzing file patterns.
// Priority: 30 (medium, runs after dependency scanners).
type FileScanner struct {
	name     string
	priority int
}

// NewFileScanner creates a new FileScanner.
func NewFileScanner() *FileScanner {
	return &FileScanner{
		name:     "file",
		priority: 30,
	}
}

func (s *FileScanner) Name() string {
	return s.name
}

func (s *FileScanner) Priority() int {
	return s.priority
}

// Scan analyzes file patterns to detect languages and frameworks.
func (s *FileScanner) Scan(ctx context.Context, req *metacontext.AnalyzeRequest) ([]metacontext.Signal, error) {
	langCounts, frameworksDetected, err := s.walkAndCollect(ctx, req.WorkingDir)
	if err != nil {
		return nil, fmt.Errorf("file walk failed: %w", err)
	}

	signals := s.buildLanguageSignals(langCounts)
	signals = append(signals, s.buildFrameworkSignals(frameworksDetected)...)

	return signals, nil
}

// walkAndCollect walks directory tree and collects language counts and frameworks.
func (s *FileScanner) walkAndCollect(ctx context.Context, workingDir string) (map[string]int, map[string]bool, error) {
	langPatterns := s.getLanguagePatterns()
	frameworkFiles := s.getFrameworkFiles()
	langCounts := make(map[string]int)
	frameworksDetected := make(map[string]bool)

	walker := s.buildFileWalker(ctx, langPatterns, frameworkFiles, langCounts, frameworksDetected)
	err := filepath.WalkDir(workingDir, walker)

	return langCounts, frameworksDetected, err
}

// buildFileWalker creates the WalkDir callback function.
func (s *FileScanner) buildFileWalker(
	ctx context.Context,
	langPatterns map[string][]string,
	frameworkFiles map[string]string,
	langCounts map[string]int,
	frameworksDetected map[string]bool,
) fs.WalkDirFunc {
	return func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip inaccessible paths
		}

		if s.shouldSkipEntry(d, path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fileName := d.Name()
		s.detectFramework(fileName, frameworkFiles, frameworksDetected)
		s.matchLanguagePatterns(fileName, langPatterns, langCounts)

		return nil
	}
}

// shouldSkipEntry checks if directory entry should be skipped.
func (s *FileScanner) shouldSkipEntry(d fs.DirEntry, path string) bool {
	// Skip hidden directories
	if d.IsDir() && strings.HasPrefix(d.Name(), ".") && d.Name() != "." {
		return true
	}

	// Skip sensitive files
	if s.shouldSkip(path) {
		return true
	}

	// Skip large files (>10MB)
	if !d.IsDir() {
		if info, err := d.Info(); err == nil && info.Size() > 10*1024*1024 {
			return true
		}
	}

	return false
}

// detectFramework checks if file indicates a framework and updates map.
func (s *FileScanner) detectFramework(fileName string, frameworkFiles map[string]string, detected map[string]bool) {
	if framework, ok := frameworkFiles[fileName]; ok {
		detected[framework] = true
	}
}

// matchLanguagePatterns checks file against language patterns and updates counts.
func (s *FileScanner) matchLanguagePatterns(fileName string, langPatterns map[string][]string, langCounts map[string]int) {
	for lang, patterns := range langPatterns {
		for _, pattern := range patterns {
			if matched, _ := filepath.Match(pattern, fileName); matched {
				langCounts[lang]++
				break
			}
		}
	}
}

// buildLanguageSignals converts language counts to signals.
func (s *FileScanner) buildLanguageSignals(langCounts map[string]int) []metacontext.Signal {
	signals := []metacontext.Signal{}
	totalFiles := 0

	for _, count := range langCounts {
		totalFiles += count
	}

	if totalFiles == 0 {
		return signals
	}

	for lang, count := range langCounts {
		confidence := float64(count) / float64(totalFiles)
		if confidence > 0.05 {
			signals = append(signals, metacontext.Signal{
				Name:       lang,
				Confidence: confidence,
				Source:     "file",
				Metadata: map[string]string{
					"file_count": fmt.Sprintf("%d", count),
				},
			})
		}
	}

	return signals
}

// buildFrameworkSignals converts detected frameworks to signals.
func (s *FileScanner) buildFrameworkSignals(frameworksDetected map[string]bool) []metacontext.Signal {
	signals := []metacontext.Signal{}

	for framework := range frameworksDetected {
		signals = append(signals, metacontext.Signal{
			Name:       framework,
			Confidence: 1.0,
			Source:     "file",
		})
	}

	return signals
}

// getLanguagePatterns returns language detection patterns.
func (s *FileScanner) getLanguagePatterns() map[string][]string {
	return map[string][]string{
		"Go":         {"*.go"},
		"TypeScript": {"*.ts", "*.tsx"},
		"JavaScript": {"*.js", "*.jsx"},
		"Python":     {"*.py"},
		"Java":       {"*.java"},
		"C++":        {"*.cpp", "*.cc", "*.cxx"},
		"C":          {"*.c", "*.h"},
		"Rust":       {"*.rs"},
		"Ruby":       {"*.rb"},
		"PHP":        {"*.php"},
	}
}

// getFrameworkFiles returns framework detection patterns.
func (s *FileScanner) getFrameworkFiles() map[string]string {
	return map[string]string{
		"package.json":     "Node.js",
		"go.mod":           "Go",
		"requirements.txt": "Python",
		"Cargo.toml":       "Rust",
		"pom.xml":          "Java/Maven",
		"build.gradle":     "Java/Gradle",
	}
}

// SensitivePatterns lists file patterns to exclude for security.
// Implements Security Mitigation M3 (Sensitive File Exclusion).
var SensitivePatterns = []string{
	"**/.env",
	"**/.env.*",
	"**/credentials.json",
	"**/secrets.yaml",
	"**/*.key",
	"**/id_rsa",
	"**/id_dsa",
	"**/id_ecdsa",
	"**/id_ed25519",
	"**/.aws/credentials",
	"**/.ssh/config",
}

// shouldSkip checks if a file should be excluded for security reasons.
func (s *FileScanner) shouldSkip(path string) bool {
	fileName := filepath.Base(path)

	// Exact matches
	sensitiveNames := []string{
		".env",
		"credentials.json",
		"secrets.yaml",
		"id_rsa",
		"id_dsa",
		"id_ecdsa",
		"id_ed25519",
	}

	for _, name := range sensitiveNames {
		if fileName == name {
			return true
		}
	}

	// Pattern matches
	if strings.HasPrefix(fileName, ".env.") {
		return true
	}
	if strings.HasSuffix(fileName, ".key") {
		return true
	}

	return false
}
