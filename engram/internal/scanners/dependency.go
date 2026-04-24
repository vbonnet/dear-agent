package scanners

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// DependencyScanner detects frameworks and tools from dependency files.
// Priority: 40 (high, runs before file scanner for more accurate framework detection).
type DependencyScanner struct {
	name     string
	priority int
}

// NewDependencyScanner creates a new DependencyScanner.
func NewDependencyScanner() *DependencyScanner {
	return &DependencyScanner{
		name:     "dependency",
		priority: 40,
	}
}

func (s *DependencyScanner) Name() string {
	return s.name
}

func (s *DependencyScanner) Priority() int {
	return s.priority
}

// Scan analyzes dependency files (package.json, go.mod, requirements.txt).
func (s *DependencyScanner) Scan(ctx context.Context, req *metacontext.AnalyzeRequest) ([]metacontext.Signal, error) {
	signals := []metacontext.Signal{}

	// Parse package.json (Node.js)
	packageJSONPath := filepath.Join(req.WorkingDir, "package.json")
	if _, err := os.Stat(packageJSONPath); err == nil {
		pkgSignals, err := s.parsePackageJSON(packageJSONPath)
		if err != nil {
			// Graceful degradation: log warning, continue
			// Would emit telemetry in production
		} else {
			signals = append(signals, pkgSignals...)
		}
	}

	// Parse go.mod (Go)
	goModPath := filepath.Join(req.WorkingDir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		goSignals, err := s.parseGoMod(goModPath)
		if err != nil {
			// Graceful degradation
		} else {
			signals = append(signals, goSignals...)
		}
	}

	// Parse requirements.txt (Python)
	reqPath := filepath.Join(req.WorkingDir, "requirements.txt")
	if _, err := os.Stat(reqPath); err == nil {
		pySignals, err := s.parseRequirementsTxt(reqPath)
		if err != nil {
			// Graceful degradation
		} else {
			signals = append(signals, pySignals...)
		}
	}

	return signals, nil
}

// PackageJSON represents relevant fields from package.json.
type PackageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// parsePackageJSON extracts framework signals from package.json.
func (s *DependencyScanner) parsePackageJSON(path string) ([]metacontext.Signal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	signals := []metacontext.Signal{}

	// Framework detection patterns
	frameworks := map[string]string{
		"react":         "React",
		"vue":           "Vue",
		"@angular/core": "Angular",
		"next":          "Next.js",
		"express":       "Express",
		"fastify":       "Fastify",
		"@nestjs/core":  "NestJS",
		"svelte":        "Svelte",
	}

	// Check dependencies and devDependencies
	allDeps := make(map[string]string)
	for k, v := range pkg.Dependencies {
		allDeps[k] = v
	}
	for k, v := range pkg.DevDependencies {
		allDeps[k] = v
	}

	for dep := range allDeps {
		if framework, ok := frameworks[dep]; ok {
			signals = append(signals, metacontext.Signal{
				Name:       framework,
				Confidence: 0.95, // High confidence (explicit dependency)
				Source:     "dependency",
				Metadata: map[string]string{
					"package": dep,
				},
			})
		}
	}

	return signals, nil
}

// parseGoMod extracts framework signals from go.mod.
func (s *DependencyScanner) parseGoMod(path string) ([]metacontext.Signal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	signals := []metacontext.Signal{}

	// Framework detection patterns
	frameworks := map[string]string{
		"github.com/gin-gonic/gin": "Gin",
		"github.com/gofiber/fiber": "Fiber",
		"github.com/labstack/echo": "Echo",
		"google.golang.org/grpc":   "gRPC",
		"github.com/spf13/cobra":   "Cobra",
		"github.com/urfave/cli":    "cli",
	}

	for pattern, framework := range frameworks {
		if strings.Contains(content, pattern) {
			signals = append(signals, metacontext.Signal{
				Name:       framework,
				Confidence: 0.95,
				Source:     "dependency",
				Metadata: map[string]string{
					"module": pattern,
				},
			})
		}
	}

	return signals, nil
}

// parseRequirementsTxt extracts framework signals from requirements.txt.
func (s *DependencyScanner) parseRequirementsTxt(path string) ([]metacontext.Signal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	content := string(data)
	lines := strings.Split(content, "\n")
	signals := []metacontext.Signal{}

	// Framework detection patterns
	frameworks := map[string]string{
		"django":  "Django",
		"flask":   "Flask",
		"fastapi": "FastAPI",
		"tornado": "Tornado",
		"pyramid": "Pyramid",
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Extract package name (before ==, >=, etc.)
		pkg := strings.FieldsFunc(line, func(r rune) bool {
			return r == '=' || r == '<' || r == '>' || r == '!'
		})[0]

		pkg = strings.ToLower(strings.TrimSpace(pkg))

		if framework, ok := frameworks[pkg]; ok {
			signals = append(signals, metacontext.Signal{
				Name:       framework,
				Confidence: 0.95,
				Source:     "dependency",
				Metadata: map[string]string{
					"package": pkg,
				},
			})
		}
	}

	return signals, nil
}
