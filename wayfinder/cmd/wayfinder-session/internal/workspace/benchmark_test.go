package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// BenchmarkWorkspaceQueries measures query performance for workspace-isolated operations
// Target: <10ms per operation (acceptable overhead vs monolithic ~1ms)
func BenchmarkWorkspaceQueries(b *testing.B) {
	// Setup test workspace
	testRoot := b.TempDir()
	workspaceRoot := filepath.Join(testRoot, "benchmark", "wf")

	if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
		b.Fatalf("Failed to create workspace root: %v", err)
	}

	// Create test project
	projectPath := filepath.Join(workspaceRoot, "benchmark-project")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		b.Fatalf("Failed to create project directory: %v", err)
	}

	testStatus := &status.Status{
		SchemaVersion: status.SchemaVersion,
		Version:       status.WayfinderV2,
		SessionID:     "benchmark-session-" + time.Now().Format("20060102-150405"),
		ProjectPath:   projectPath,
		StartedAt:     time.Now(),
		Status:        status.StatusInProgress,
		CurrentPhase:  "build.implement",
		Phases: []status.Phase{
			{Name: "discovery.problem", Status: status.PhaseStatusCompleted, StartedAt: timePtr(time.Now()), CompletedAt: timePtr(time.Now())},
			{Name: "build.implement", Status: status.PhaseStatusInProgress, StartedAt: timePtr(time.Now())},
		},
	}

	if err := status.Save(projectPath, testStatus); err != nil {
		b.Fatalf("Failed to save test status: %v", err)
	}

	// Benchmark: Load project status
	b.Run("LoadProjectStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := status.Load(projectPath)
			if err != nil {
				b.Fatalf("LoadProjectStatus failed: %v", err)
			}
		}
	})

	// Benchmark: Save project status
	b.Run("SaveProjectStatus", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			testStatus.CurrentPhase = fmt.Sprintf("phase-%d", i)
			if err := status.Save(projectPath, testStatus); err != nil {
				b.Fatalf("SaveProjectStatus failed: %v", err)
			}
		}
	})

	// Benchmark: Detect workspace from path
	b.Run("DetectWorkspace", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			workspace := DetectWorkspace(projectPath)
			if workspace == "" {
				b.Fatal("DetectWorkspace failed")
			}
		}
	})

	// Benchmark: List projects in workspace
	b.Run("ListProjects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			projects, err := ListProjects(workspaceRoot)
			if err != nil {
				b.Fatalf("ListProjects failed: %v", err)
			}
			if len(projects) != 1 {
				b.Fatalf("Expected 1 project, got %d", len(projects))
			}
		}
	})

	// Benchmark: Validate workspace isolation
	b.Run("ValidateWorkspaceIsolation", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			isValid := ValidateWorkspaceIsolation(projectPath, "benchmark")
			if !isValid {
				b.Fatal("ValidateWorkspaceIsolation failed")
			}
		}
	})
}

// BenchmarkWorkspaceQueriesWithData measures performance with realistic data volume
func BenchmarkWorkspaceQueriesWithData(b *testing.B) {
	// Setup test workspace with multiple projects
	testRoot := b.TempDir()

	config := TestDataConfig{
		RootDir:           testRoot,
		OSSProjects:       10,
		AcmeProjects:      10,
		IncludePhaseFiles: true,
	}

	if err := GenerateTestData(config); err != nil {
		b.Fatalf("Failed to generate test data: %v", err)
	}

	ossRoot := filepath.Join(testRoot, "oss", "wf")
	acmeRoot := filepath.Join(testRoot, "acme", "wf")

	// Benchmark: List projects in OSS workspace
	b.Run("ListProjects_OSS_10projects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			projects, err := ListProjects(ossRoot)
			if err != nil {
				b.Fatalf("ListProjects failed: %v", err)
			}
			if len(projects) != config.OSSProjects {
				b.Fatalf("Expected %d projects, got %d", config.OSSProjects, len(projects))
			}
		}
	})

	// Benchmark: List projects in Acme workspace
	b.Run("ListProjects_Acme_10projects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			projects, err := ListProjects(acmeRoot)
			if err != nil {
				b.Fatalf("ListProjects failed: %v", err)
			}
			if len(projects) != config.AcmeProjects {
				b.Fatalf("Expected %d projects, got %d", config.AcmeProjects, len(projects))
			}
		}
	})

	// Benchmark: Validate test data (comprehensive check)
	b.Run("ValidateTestData_20projects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			result, err := ValidateTestData(config)
			if err != nil {
				b.Fatalf("ValidateTestData failed: %v", err)
			}
			if !result.IsValid {
				b.Fatalf("Validation failed with %d violations", len(result.Violations))
			}
		}
	})
}

// BenchmarkMonolithicVsMultiWorkspace compares performance overhead
func BenchmarkMonolithicVsMultiWorkspace(b *testing.B) {
	testRoot := b.TempDir()

	// Monolithic setup: all projects in single directory
	monolithicRoot := filepath.Join(testRoot, "monolithic")
	if err := os.MkdirAll(monolithicRoot, 0755); err != nil {
		b.Fatalf("Failed to create monolithic root: %v", err)
	}

	// Multi-workspace setup: projects separated by workspace
	multiWorkspaceRoot := testRoot

	// Create 20 projects in monolithic
	for i := 0; i < 20; i++ {
		projectPath := filepath.Join(monolithicRoot, fmt.Sprintf("project-%d", i))
		if err := os.MkdirAll(projectPath, 0755); err != nil {
			b.Fatalf("Failed to create monolithic project: %v", err)
		}

		st := &status.Status{
			SchemaVersion: status.SchemaVersion,
			Version:       status.WayfinderV2,
			SessionID:     fmt.Sprintf("session-%d", i),
			ProjectPath:   projectPath,
			StartedAt:     time.Now(),
			Status:        status.StatusInProgress,
			CurrentPhase:  "build.implement",
		}

		if err := status.Save(projectPath, st); err != nil {
			b.Fatalf("Failed to save monolithic project: %v", err)
		}
	}

	// Create 20 projects in multi-workspace (10 OSS, 10 Acme)
	config := TestDataConfig{
		RootDir:           multiWorkspaceRoot,
		OSSProjects:       10,
		AcmeProjects:      10,
		IncludePhaseFiles: false,
	}

	if err := GenerateTestData(config); err != nil {
		b.Fatalf("Failed to generate multi-workspace test data: %v", err)
	}

	ossRoot := filepath.Join(multiWorkspaceRoot, "oss", "wf")

	// Benchmark: List all projects (monolithic)
	b.Run("Monolithic_ListAll_20projects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			projects, err := ListProjects(monolithicRoot)
			if err != nil {
				b.Fatalf("ListProjects failed: %v", err)
			}
			if len(projects) != 20 {
				b.Fatalf("Expected 20 projects, got %d", len(projects))
			}
		}
	})

	// Benchmark: List workspace projects (multi-workspace)
	b.Run("MultiWorkspace_ListOSS_10projects", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			projects, err := ListProjects(ossRoot)
			if err != nil {
				b.Fatalf("ListProjects failed: %v", err)
			}
			if len(projects) != 10 {
				b.Fatalf("Expected 10 projects, got %d", len(projects))
			}
		}
	})
}

// BenchmarkWorkspaceScalability tests performance with increasing project counts
func BenchmarkWorkspaceScalability(b *testing.B) {
	projectCounts := []int{10, 50, 100, 200}

	for _, count := range projectCounts {
		b.Run(fmt.Sprintf("ListProjects_%dprojects", count), func(b *testing.B) {
			testRoot := b.TempDir()
			workspaceRoot := filepath.Join(testRoot, "scalability", "wf")

			if err := os.MkdirAll(workspaceRoot, 0755); err != nil {
				b.Fatalf("Failed to create workspace root: %v", err)
			}

			// Create projects
			for i := 0; i < count; i++ {
				projectPath := filepath.Join(workspaceRoot, fmt.Sprintf("project-%d", i))
				if err := os.MkdirAll(projectPath, 0755); err != nil {
					b.Fatalf("Failed to create project: %v", err)
				}

				st := &status.Status{
					SchemaVersion: status.SchemaVersion,
					Version:       status.WayfinderV2,
					SessionID:     fmt.Sprintf("session-%d", i),
					ProjectPath:   projectPath,
					StartedAt:     time.Now(),
					Status:        status.StatusInProgress,
				}

				if err := status.Save(projectPath, st); err != nil {
					b.Fatalf("Failed to save project: %v", err)
				}
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				projects, err := ListProjects(workspaceRoot)
				if err != nil {
					b.Fatalf("ListProjects failed: %v", err)
				}
				if len(projects) != count {
					b.Fatalf("Expected %d projects, got %d", count, len(projects))
				}
			}
		})
	}
}
