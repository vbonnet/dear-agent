package main

import (
	"context"
	"log"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Main entry point for AGM MCP server
// Adapted from Engram MCP server pattern
func main() {
	// CRITICAL: Redirect logs to stderr (stdio transport requirement)
	log.SetOutput(os.Stderr)

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

	// Create MCP server
	server := mcp.NewServer("agm", "1.0.0", nil)

	// Register MCP tools
	listTool := createListSessionsTool(cfg)
	searchTool := createSearchSessionsTool(cfg)
	getTool := createGetSessionMetadataTool(cfg)
	server.AddTools(listTool, searchTool, getTool)

	log.Println("Registered 3 tools: agm_list_sessions, agm_search_sessions, agm_get_session_metadata")

	// Auto-register with Claude Code (optional)
	if cfg.AutoRegister {
		if err := registerWithClaudeCode(cfg.ClaudeConfigPath); err != nil {
			log.Printf("Auto-registration failed (non-fatal): %v", err)
		} else {
			log.Printf("Auto-registered with Claude Code: %s", cfg.ClaudeConfigPath)
		}
	}

	// Create stdio transport
	transport := mcp.NewStdioTransport()

	// Run server (blocks until connection closes)
	log.Println("Starting MCP server with stdio transport")
	ctx := context.Background()
	if err := server.Run(ctx, transport); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
