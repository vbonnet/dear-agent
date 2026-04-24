// AGM Multi-Workspace Test Data Generator
// Creates sample sessions in both OSS and Acme Corp workspaces to demonstrate isolation
// Task 3.4: AGM Multi-Workspace Testing (bead: oss-6xkh)

package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

var logger = slog.Default()

var (
	ossPort  = flag.String("oss-port", "3307", "OSS workspace Dolt port")
	acmePort = flag.String("acme-port", "3308", "Acme workspace Dolt port")
	clean    = flag.Bool("clean", false, "Clean up test data before creating")
)

func main() {
	flag.Parse()

	fmt.Println("========================================")
	fmt.Println("AGM Multi-Workspace Test Data Generator")
	fmt.Println("========================================")
	fmt.Println()

	// Create OSS workspace adapter
	fmt.Println("Connecting to OSS workspace...")
	ossAdapter, err := createAdapter("oss", *ossPort)
	if err != nil {
		logger.Error("Failed to create OSS adapter", "error", err)
		os.Exit(1)
	}
	defer ossAdapter.Close()
	fmt.Println("✓ Connected to OSS workspace")

	// Create Acme Corp workspace adapter
	fmt.Println("Connecting to Acme Corp workspace...")
	acmeAdapter, err := createAdapter("acme", *acmePort)
	if err != nil {
		logger.Warn("Failed to create Acme Corp adapter", "error", err)
		logger.Info("Continuing with OSS workspace only...")
		acmeAdapter = nil
	} else {
		fmt.Println("✓ Connected to Acme Corp workspace")
	}
	if acmeAdapter != nil {
		defer acmeAdapter.Close()
	}
	fmt.Println()

	// Clean up existing test data if requested
	if *clean {
		fmt.Println("Cleaning up existing test data...")
		cleanTestData(ossAdapter, acmeAdapter)
		fmt.Println("✓ Cleanup complete")
		fmt.Println()
	}

	// Create test sessions
	fmt.Println("Creating test sessions...")
	fmt.Println()

	// OSS Sessions
	fmt.Println("OSS Workspace Sessions:")
	fmt.Println("-----------------------")

	ossSessions := []struct {
		name    string
		project string
		purpose string
		tags    []string
	}{
		{
			name:    "Open Source Development",
			project: "~/src/ws/oss/repos/engram",
			purpose: "Working on Engram plugin system",
			tags:    []string{"oss", "engram", "plugins"},
		},
		{
			name:    "AGM Dolt Migration",
			project: "~/src/ws/oss/repos/ai-tools/main/agm",
			purpose: "Migrating AGM to Dolt storage",
			tags:    []string{"oss", "agm", "dolt", "migration"},
		},
		{
			name:    "Beads Issue Tracking",
			project: "~/src/ws/oss/repos/beads",
			purpose: "Implementing git-native issue tracking",
			tags:    []string{"oss", "beads", "git"},
		},
	}

	for i, sessionData := range ossSessions {
		sessionID := fmt.Sprintf("oss-test-%d-%s", i+1, time.Now().Format("20060102"))
		session := createSession(sessionID, sessionData.name, sessionData.project, sessionData.purpose, sessionData.tags)

		if err := ossAdapter.CreateSession(session); err != nil {
			logger.Warn("Failed to create OSS session", "session_id", sessionID, "error", err)
			continue
		}

		// Add sample messages
		messages := createMessages(sessionID, sessionData.purpose)
		if err := ossAdapter.CreateMessages(messages); err != nil {
			logger.Warn("Failed to create messages", "session_id", sessionID, "error", err)
		}

		fmt.Printf("  ✓ Created: %s\n", sessionData.name)
		fmt.Printf("    ID: %s\n", sessionID)
		fmt.Printf("    Project: %s\n", sessionData.project)
		fmt.Printf("    Messages: %d\n", len(messages))
		fmt.Println()
	}

	// Acme Corp Sessions (if available)
	if acmeAdapter != nil {
		fmt.Println("Acme Workspace Sessions:")
		fmt.Println("-------------------------")

		acmeSessions := []struct {
			name    string
			project string
			purpose string
			tags    []string
		}{
			{
				name:    "Healthcare Data Pipeline",
				project: "~/src/ws/acme/projects/data-pipeline",
				purpose: "Building HIPAA-compliant data processing pipeline",
				tags:    []string{"acme", "healthcare", "confidential", "hipaa"},
			},
			{
				name:    "Patient Analytics System",
				project: "~/src/ws/acme/projects/analytics",
				purpose: "Developing patient outcome prediction models",
				tags:    []string{"acme", "ml", "confidential", "phi"},
			},
			{
				name:    "Clinical Trial Dashboard",
				project: "~/src/ws/acme/projects/clinical-dashboard",
				purpose: "Real-time clinical trial monitoring interface",
				tags:    []string{"acme", "clinical", "confidential"},
			},
		}

		for i, sessionData := range acmeSessions {
			sessionID := fmt.Sprintf("acme-test-%d-%s", i+1, time.Now().Format("20060102"))
			session := createSession(sessionID, sessionData.name, sessionData.project, sessionData.purpose, sessionData.tags)

			if err := acmeAdapter.CreateSession(session); err != nil {
				logger.Warn("Failed to create Acme Corp session", "session_id", sessionID, "error", err)
				continue
			}

			// Add sample messages
			messages := createMessages(sessionID, sessionData.purpose)
			if err := acmeAdapter.CreateMessages(messages); err != nil {
				logger.Warn("Failed to create messages", "session_id", sessionID, "error", err)
			}

			fmt.Printf("  ✓ Created: %s\n", sessionData.name)
			fmt.Printf("    ID: %s\n", sessionID)
			fmt.Printf("    Project: %s\n", sessionData.project)
			fmt.Printf("    Messages: %d\n", len(messages))
			fmt.Println()
		}
	}

	// Verify isolation
	fmt.Println("========================================")
	fmt.Println("Verification: Workspace Isolation")
	fmt.Println("========================================")
	fmt.Println()

	// List OSS sessions
	ossList, err := ossAdapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		logger.Error("Failed to list OSS sessions", "error", err)
		os.Exit(1)
	}
	fmt.Printf("OSS Workspace: %d sessions\n", len(ossList))
	for _, s := range ossList {
		fmt.Printf("  - %s (workspace: %s)\n", s.Name, s.Workspace)
	}
	fmt.Println()

	// List Acme Corp sessions
	if acmeAdapter != nil {
		acmeList, err := acmeAdapter.ListSessions(&dolt.SessionFilter{})
		if err != nil {
			logger.Error("Failed to list Acme Corp sessions", "error", err)
			os.Exit(1)
		}
		fmt.Printf("Acme Workspace: %d sessions\n", len(acmeList))
		for _, s := range acmeList {
			fmt.Printf("  - %s (workspace: %s)\n", s.Name, s.Workspace)
		}
		fmt.Println()
	}

	// Verify zero cross-contamination
	fmt.Println("Isolation Check:")
	violations := 0

	for _, s := range ossList {
		if s.Workspace != "oss" {
			fmt.Printf("  ✗ VIOLATION: OSS workspace contains session from '%s'\n", s.Workspace)
			violations++
		}
	}

	if acmeAdapter != nil {
		acmeList, _ := acmeAdapter.ListSessions(&dolt.SessionFilter{})
		for _, s := range acmeList {
			if s.Workspace != "acme" {
				fmt.Printf("  ✗ VIOLATION: Acme Corp workspace contains session from '%s'\n", s.Workspace)
				violations++
			}
		}
	}

	if violations == 0 {
		fmt.Println("  ✓ Zero cross-contamination verified")
		fmt.Println("  ✓ All sessions are properly isolated")
	} else {
		fmt.Printf("  ✗ Found %d workspace violations\n", violations)
		os.Exit(1)
	}
	fmt.Println()

	fmt.Println("========================================")
	fmt.Println("Test Data Creation Complete")
	fmt.Println("========================================")
	fmt.Println()
	fmt.Println("Next Steps:")
	fmt.Println("  1. Run workspace isolation tests:")
	fmt.Println("     ./scripts/test-workspace-isolation.sh")
	fmt.Println()
	fmt.Println("  2. Query test data:")
	fmt.Println("     mysql -h 127.0.0.1 -P 3307 -u root -e 'SELECT * FROM agm_sessions'")
	fmt.Println()
	fmt.Println("  3. Clean up test data:")
	fmt.Println("     go run scripts/create-test-data.go --clean")
}

// createAdapter creates a Dolt adapter for the given workspace
func createAdapter(workspace, port string) (*dolt.Adapter, error) {
	config := &dolt.Config{
		Workspace: workspace,
		Port:      port,
		Host:      "127.0.0.1",
		Database:  "workspace",
		User:      "root",
		Password:  "",
	}

	adapter, err := dolt.New(config)
	if err != nil {
		return nil, err
	}

	// Apply migrations
	if err := adapter.ApplyMigrations(); err != nil {
		adapter.Close()
		return nil, err
	}

	return adapter, nil
}

// createSession creates a test session manifest
func createSession(sessionID, name, project, purpose string, tags []string) *manifest.Manifest {
	return &manifest.Manifest{
		SessionID:     sessionID,
		Name:          name,
		SchemaVersion: "2.0",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Harness:       "claude-code",
		Context: manifest.Context{
			Project: project,
			Purpose: purpose,
			Tags:    tags,
		},
		Claude: manifest.Claude{
			UUID: fmt.Sprintf("uuid-%s", sessionID),
		},
		Tmux: manifest.Tmux{
			SessionName: fmt.Sprintf("tmux-%s", sessionID),
		},
	}
}

// createMessages creates sample messages for a session
func createMessages(sessionID, purpose string) []*dolt.Message {
	return []*dolt.Message{
		{
			SessionID:      sessionID,
			Role:           "user",
			Content:        fmt.Sprintf(`[{"type":"text","text":"Let's work on: %s"}]`, purpose),
			SequenceNumber: 0,
			Harness:        "claude-code",
		},
		{
			SessionID:      sessionID,
			Role:           "assistant",
			Content:        `[{"type":"text","text":"I understand. I'll help you with this task."}]`,
			SequenceNumber: 1,
			Harness:        "claude-code",
		},
		{
			SessionID:      sessionID,
			Role:           "user",
			Content:        `[{"type":"text","text":"What files should we look at first?"}]`,
			SequenceNumber: 2,
			Harness:        "claude-code",
		},
	}
}

// cleanTestData removes all test sessions
func cleanTestData(ossAdapter, acmeAdapter *dolt.Adapter) {
	// Clean OSS workspace
	ossSessions, err := ossAdapter.ListSessions(&dolt.SessionFilter{})
	if err == nil {
		for _, session := range ossSessions {
			if session.SessionID[:3] == "oss" || session.SessionID[:4] == "test" {
				ossAdapter.DeleteSession(session.SessionID)
			}
		}
	}

	// Clean Acme Corp workspace
	if acmeAdapter != nil {
		acmeSessions, err := acmeAdapter.ListSessions(&dolt.SessionFilter{})
		if err == nil {
			for _, session := range acmeSessions {
				if session.SessionID[:6] == "acme" || session.SessionID[:4] == "test" {
					acmeAdapter.DeleteSession(session.SessionID)
				}
			}
		}
	}
}
