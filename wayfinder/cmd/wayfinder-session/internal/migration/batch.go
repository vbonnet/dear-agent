package migration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// BatchMigrationOptions contains options for batch migration
type BatchMigrationOptions struct {
	WorkspaceRoot string
	DryRun        bool
	Parallel      bool
	MaxWorkers    int
}

// BatchMigrationReport contains the summary of a batch migration
type BatchMigrationReport struct {
	TotalProjects   int
	SuccessCount    int
	FailureCount    int
	SkippedCount    int
	SuccessProjects []string
	FailedProjects  map[string]string   // project path -> error message
	SkippedProjects map[string]string   // project path -> reason
	Warnings        map[string][]string // project path -> warnings
}

// MigrateAll migrates all projects in a workspace
func MigrateAll(options BatchMigrationOptions) (*BatchMigrationReport, error) {
	report := &BatchMigrationReport{
		SuccessProjects: []string{},
		FailedProjects:  make(map[string]string),
		SkippedProjects: make(map[string]string),
		Warnings:        make(map[string][]string),
	}

	// Find all V1 projects
	projects, err := findV1Projects(options.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to find V1 projects: %w", err)
	}

	report.TotalProjects = len(projects)

	if report.TotalProjects == 0 {
		return report, nil
	}

	// Migrate projects
	if options.Parallel {
		migrateParallel(projects, options, report)
	} else {
		migrateSequential(projects, options, report)
	}

	return report, nil
}

// findV1Projects finds all projects with V1 WAYFINDER-STATUS.md files
func findV1Projects(workspaceRoot string) ([]string, error) {
	var projects []string

	// Check if workspace root exists
	if _, err := os.Stat(workspaceRoot); err != nil {
		if os.IsNotExist(err) {
			return projects, nil
		}
		return nil, err
	}

	// Walk the workspace root looking for WAYFINDER-STATUS.md files
	err := filepath.Walk(workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip paths with errors
		}

		// Look for WAYFINDER-STATUS.md files
		if !info.IsDir() && info.Name() == status.StatusFilename {
			projectPath := filepath.Dir(path)

			// Detect schema version
			schemaVersion, err := status.DetectSchemaVersion(path)
			if err != nil {
				return nil // Skip invalid files
			}

			// Only include V1 projects
			if schemaVersion != status.SchemaVersionV2 {
				projects = append(projects, projectPath)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return projects, nil
}

// migrateSequential migrates projects one at a time
func migrateSequential(projects []string, options BatchMigrationOptions, report *BatchMigrationReport) {
	for i, projectPath := range projects {
		// Report progress
		fmt.Printf("Migrating project %d/%d: %s\n", i+1, report.TotalProjects, projectPath)

		if options.DryRun {
			processDryRun(projectPath, report)
		} else {
			processMigration(projectPath, report)
		}
	}
}

// migrateParallel migrates projects in parallel
func migrateParallel(projects []string, options BatchMigrationOptions, report *BatchMigrationReport) {
	maxWorkers := options.MaxWorkers
	if maxWorkers <= 0 {
		maxWorkers = 4 // Default to 4 workers
	}

	var wg sync.WaitGroup
	projectChan := make(chan string, len(projects))

	// Use mutex to protect report updates
	var mu sync.Mutex

	// Start workers
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for projectPath := range projectChan {
				if options.DryRun {
					processDryRunParallel(projectPath, report, &mu)
				} else {
					processMigrationParallel(projectPath, report, &mu)
				}
			}
		}()
	}

	// Send projects to workers
	for _, projectPath := range projects {
		projectChan <- projectPath
	}
	close(projectChan)

	// Wait for all workers to finish
	wg.Wait()
}

// processMigration processes a single project migration
func processMigration(projectPath string, report *BatchMigrationReport) {
	result, err := MigrateProject(projectPath)
	if err != nil {
		report.FailureCount++
		report.FailedProjects[projectPath] = err.Error()
		fmt.Printf("  ✗ Failed: %s\n", err.Error())
		return
	}

	if !result.Success {
		if len(result.Warnings) > 0 && result.Warnings[0] == "Already V2 schema, skipping migration" {
			report.SkippedCount++
			report.SkippedProjects[projectPath] = result.Warnings[0]
			fmt.Printf("  ⊘ Skipped: %s\n", result.Warnings[0])
		} else {
			report.FailureCount++
			report.FailedProjects[projectPath] = result.ErrorMessage
			fmt.Printf("  ✗ Failed: %s\n", result.ErrorMessage)
		}
		return
	}

	report.SuccessCount++
	report.SuccessProjects = append(report.SuccessProjects, projectPath)

	if len(result.Warnings) > 0 {
		report.Warnings[projectPath] = result.Warnings
	}

	fmt.Printf("  ✓ Success (backup: %s)\n", result.BackupPath)
}

// processMigrationParallel processes a single project migration with mutex locking
func processMigrationParallel(projectPath string, report *BatchMigrationReport, mu *sync.Mutex) {
	result, err := MigrateProject(projectPath)

	mu.Lock()
	defer mu.Unlock()

	if err != nil {
		report.FailureCount++
		report.FailedProjects[projectPath] = err.Error()
		fmt.Printf("  ✗ Failed (%s): %s\n", projectPath, err.Error())
		return
	}

	if !result.Success {
		if len(result.Warnings) > 0 && result.Warnings[0] == "Already V2 schema, skipping migration" {
			report.SkippedCount++
			report.SkippedProjects[projectPath] = result.Warnings[0]
			fmt.Printf("  ⊘ Skipped (%s): %s\n", projectPath, result.Warnings[0])
		} else {
			report.FailureCount++
			report.FailedProjects[projectPath] = result.ErrorMessage
			fmt.Printf("  ✗ Failed (%s): %s\n", projectPath, result.ErrorMessage)
		}
		return
	}

	report.SuccessCount++
	report.SuccessProjects = append(report.SuccessProjects, projectPath)

	if len(result.Warnings) > 0 {
		report.Warnings[projectPath] = result.Warnings
	}

	fmt.Printf("  ✓ Success (%s): backup at %s\n", projectPath, result.BackupPath)
}

// processDryRun processes a dry-run for a single project
func processDryRun(projectPath string, report *BatchMigrationReport) {
	warnings, err := DryRun(projectPath)
	if err != nil {
		report.FailureCount++
		report.FailedProjects[projectPath] = err.Error()
		fmt.Printf("  ✗ Would fail: %s\n", err.Error())
		return
	}

	// Check if already V2
	if len(warnings) > 0 && strings.Contains(warnings[0], "Already V2 schema") {
		report.SkippedCount++
		report.SkippedProjects[projectPath] = warnings[0]
		fmt.Printf("  ⊘ Would skip: %s\n", warnings[0])
		return
	}

	report.SuccessCount++
	report.SuccessProjects = append(report.SuccessProjects, projectPath)

	if len(warnings) > 0 {
		report.Warnings[projectPath] = warnings
		fmt.Printf("  ✓ Would succeed (warnings: %d)\n", len(warnings))
	} else {
		fmt.Printf("  ✓ Would succeed\n")
	}
}

// processDryRunParallel processes a dry-run for a single project with mutex locking
func processDryRunParallel(projectPath string, report *BatchMigrationReport, mu *sync.Mutex) {
	warnings, err := DryRun(projectPath)

	mu.Lock()
	defer mu.Unlock()

	if err != nil {
		report.FailureCount++
		report.FailedProjects[projectPath] = err.Error()
		fmt.Printf("  ✗ Would fail (%s): %s\n", projectPath, err.Error())
		return
	}

	// Check if already V2
	if len(warnings) > 0 && strings.Contains(warnings[0], "Already V2 schema") {
		report.SkippedCount++
		report.SkippedProjects[projectPath] = warnings[0]
		fmt.Printf("  ⊘ Would skip (%s): %s\n", projectPath, warnings[0])
		return
	}

	report.SuccessCount++
	report.SuccessProjects = append(report.SuccessProjects, projectPath)

	if len(warnings) > 0 {
		report.Warnings[projectPath] = warnings
		fmt.Printf("  ✓ Would succeed (%s): %d warnings\n", projectPath, len(warnings))
	} else {
		fmt.Printf("  ✓ Would succeed (%s)\n", projectPath)
	}
}

// PrintReport prints a summary report
func (r *BatchMigrationReport) PrintReport() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("MIGRATION SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total projects: %d\n", r.TotalProjects)
	fmt.Printf("  ✓ Successful: %d\n", r.SuccessCount)
	fmt.Printf("  ✗ Failed:     %d\n", r.FailureCount)
	fmt.Printf("  ⊘ Skipped:    %d\n", r.SkippedCount)

	if r.FailureCount > 0 {
		fmt.Println("\nFailed Projects:")
		for path, errMsg := range r.FailedProjects {
			fmt.Printf("  - %s\n    Error: %s\n", path, errMsg)
		}
	}

	if len(r.Warnings) > 0 {
		fmt.Println("\nWarnings:")
		for path, warnings := range r.Warnings {
			fmt.Printf("  - %s\n", path)
			for _, warning := range warnings {
				fmt.Printf("    * %s\n", warning)
			}
		}
	}

	fmt.Println(strings.Repeat("=", 60))
}
