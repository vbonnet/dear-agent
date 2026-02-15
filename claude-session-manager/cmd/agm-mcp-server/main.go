package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version information - set via ldflags at build time
var (
	Version   = "1.0.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuiltBy   = "unknown"
)

// Main entry point for AGM MCP server
// Adapted from Engram MCP server pattern
func main() {
	// CRITICAL: Redirect logs to stderr (stdio transport requirement)
	log.SetOutput(os.Stderr)

	// Print header to stderr
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	fmt.Fprintf(os.Stderr, "agm-mcp-server %s (%s)\n", Version, executable)

	// Load configuration
	cfg, err := loadConfig("~/.config/agm/mcp-server.yaml")
	if err != nil {
		log.Fatalf("Config load failed: %v", err)
	}

	// Check if server is enabled
	if !cfg.Enabled {
		log.Println("MCP server disabled in config")
		return
	}

	log.Printf("Starting AGM MCP Server v1.0.0")
	log.Printf("Sessions directory: %s", cfg.SessionsDir)

	// Create MCP server (v1.2.0 API)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "agm",
		Version: "1.0.0",
	}, nil)

	// Register MCP tools (v1.2.0 API)
	addListSessionsTool(server, cfg)
	addSearchSessionsTool(server, cfg)
	addGetSessionMetadataTool(server, cfg)

	// Register Wayfinder forwarding tools (Phase 7.1)
	addListWayfinderSessionsTool(server, cfg)
	addGetWayfinderSessionTool(server, cfg)

	log.Println("Registered 5 tools: agm_list_sessions, agm_search_sessions, agm_get_session_metadata, engram_list_wayfinder_sessions, engram_get_wayfinder_session")
	log.Printf("Wayfinder forwarding enabled (Engram MCP URL: %s)", cfg.EngramMCPURL)

	// Auto-register with Claude Code (optional)
	if cfg.AutoRegister {
		if err := registerWithClaudeCode(cfg.ClaudeConfigPath); err != nil {
			log.Printf("Auto-registration failed (non-fatal): %v", err)
		} else {
			log.Printf("Auto-registered with Claude Code: %s", cfg.ClaudeConfigPath)
		}
	}

	// Create stdio transport (v1.2.0 API)
	transport := &mcp.StdioTransport{}

	// Run server (blocks until connection closes)
	log.Println("Starting MCP server with stdio transport")
	ctx := context.Background()
	if err := server.Run(ctx, transport); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
