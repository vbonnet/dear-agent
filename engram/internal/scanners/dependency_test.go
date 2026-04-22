package scanners

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
	"github.com/vbonnet/dear-agent/internal/testutil"
)

// TestDependencyScanner_PackageJSON_React tests React detection from package.json
func TestDependencyScanner_PackageJSON_React(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	testutil.CreatePackageJSON(t, tmpdir, map[string]string{
		"react":     "^18.0.0",
		"react-dom": "^18.0.0",
	})

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect React
	hasReact := false
	for _, sig := range signals {
		if sig.Name == "React" && sig.Source == "dependency" {
			hasReact = true
			if sig.Confidence != 0.95 {
				t.Errorf("Expected confidence 0.95, got %f", sig.Confidence)
			}
		}
	}

	if !hasReact {
		t.Error("DependencyScanner should detect React from package.json")
	}
}

// TestDependencyScanner_PackageJSON_Multiple tests multiple framework detection
func TestDependencyScanner_PackageJSON_Multiple(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	testutil.CreatePackageJSON(t, tmpdir, map[string]string{
		"react":   "^18.0.0",
		"next":    "^13.0.0",
		"express": "^4.18.0",
	})

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect all three frameworks
	frameworks := map[string]bool{"React": false, "Next.js": false, "Express": false}
	for _, sig := range signals {
		if _, ok := frameworks[sig.Name]; ok {
			frameworks[sig.Name] = true
		}
	}

	for framework, detected := range frameworks {
		if !detected {
			t.Errorf("DependencyScanner should detect %s from package.json", framework)
		}
	}
}

// TestDependencyScanner_GoMod tests Go framework detection from go.mod
func TestDependencyScanner_GoMod(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	testutil.CreateGoMod(t, tmpdir, []string{
		"github.com/gin-gonic/gin",
		"google.golang.org/grpc",
	})

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect Gin and gRPC
	frameworks := map[string]bool{"Gin": false, "gRPC": false}
	for _, sig := range signals {
		if _, ok := frameworks[sig.Name]; ok {
			frameworks[sig.Name] = true
		}
	}

	for framework, detected := range frameworks {
		if !detected {
			t.Errorf("DependencyScanner should detect %s from go.mod", framework)
		}
	}
}

// TestDependencyScanner_RequirementsTxt tests Python framework detection
func TestDependencyScanner_RequirementsTxt(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	testutil.CreateRequirementsTxt(t, tmpdir, []string{
		"django",
		"flask",
		"fastapi",
	})

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should detect all three frameworks
	frameworks := map[string]bool{"Django": false, "Flask": false, "FastAPI": false}
	for _, sig := range signals {
		if _, ok := frameworks[sig.Name]; ok {
			frameworks[sig.Name] = true
		}
	}

	for framework, detected := range frameworks {
		if !detected {
			t.Errorf("DependencyScanner should detect %s from requirements.txt", framework)
		}
	}
}

// TestDependencyScanner_NoDependencyFiles tests handling when no dependency files exist
func TestDependencyScanner_NoDependencyFiles(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() failed: %v", err)
	}

	// Should return empty signals (no dependency files to parse)
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals, got %d", len(signals))
	}
}

// TestDependencyScanner_InvalidPackageJSON tests error handling for invalid JSON
func TestDependencyScanner_InvalidPackageJSON(t *testing.T) {
	tmpdir := testutil.SetupTempDir(t)

	// Create invalid package.json
	packageJSONPath := filepath.Join(tmpdir, "package.json")
	if err := os.WriteFile(packageJSONPath, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write invalid package.json: %v", err)
	}

	scanner := NewDependencyScanner()
	ctx := context.Background()
	req := &metacontext.AnalyzeRequest{WorkingDir: tmpdir}

	// Should handle gracefully (not return error)
	signals, err := scanner.Scan(ctx, req)
	if err != nil {
		t.Fatalf("Scan() should handle invalid package.json gracefully, got error: %v", err)
	}

	// Should return empty signals
	if len(signals) != 0 {
		t.Errorf("Expected 0 signals for invalid package.json, got %d", len(signals))
	}
}

// TestDependencyScanner_Name tests Name() method
func TestDependencyScanner_Name(t *testing.T) {
	scanner := NewDependencyScanner()
	if scanner.Name() != "dependency" {
		t.Errorf("Expected name 'dependency', got '%s'", scanner.Name())
	}
}

// TestDependencyScanner_Priority tests Priority() method
func TestDependencyScanner_Priority(t *testing.T) {
	scanner := NewDependencyScanner()
	if scanner.Priority() != 40 {
		t.Errorf("Expected priority 40, got %d", scanner.Priority())
	}
}
