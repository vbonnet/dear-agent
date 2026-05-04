package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/gateway"
)

// logger writes to stderr (required for stdio MCP transport)
var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

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
	// Parse flags
	a2aPort := flag.Int("a2a-port", 0, "A2A HTTP port (0=disabled, e.g. 8080 to enable)")
	noGateway := flag.Bool("no-gateway", false, "Bypass MCP Gateway middleware")
	flag.Parse()

	// Print header to stderr (stdio transport requirement: logs go to stderr)
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	fmt.Fprintf(os.Stderr, "agm-mcp-server %s (%s)\n", Version, executable)

	// Load configuration
	cfg, err := loadConfig("~/.config/agm/mcp-server.yaml")
	if err != nil {
		logger.Error("Config load failed", "error", err)
		os.Exit(1)
	}

	// Check if server is enabled
	if !cfg.Enabled {
		logger.Info("MCP server disabled in config")
		return
	}

	logger.Info("Starting AGM MCP Server", "version", "1.0.0")
	logger.Info("Configuration loaded", "sessions_dir", cfg.SessionsDir)

	// Create MCP server (v1.2.0 API)
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "agm",
		Version: "1.0.0",
	}, nil)

	// Register MCP tools (v1.2.0 API)
	addListSessionsTool(server, cfg)
	addSearchSessionsTool(server, cfg)
	addGetSessionMetadataTool(server, cfg)

	// Register mutation tools
	addArchiveSessionTool(server, cfg)
	addKillSessionTool(server, cfg)

	// Register schema introspection tool
	addListOpsTool(server, cfg)

	// Register Wayfinder forwarding tools (Phase 7.1)
	addListWayfinderSessionsTool(server, cfg)
	addGetWayfinderSessionTool(server, cfg)

	logger.Info("Registered MCP tools", "tools", "agm_list_sessions, agm_search_sessions, agm_get_session, agm_archive_session, agm_kill_session, agm_list_ops, engram_list_wayfinder_sessions, engram_get_wayfinder_session")
	logger.Info("Wayfinder forwarding enabled", "engram_mcp_url", cfg.EngramMCPURL)

	// Install MCP Gateway middleware (unless --no-gateway flag is set)
	if !*noGateway && !slices.Contains(os.Args[1:], "--no-gateway") {
		gatewayCfg, err := gateway.LoadConfig("~/.config/agm/gateway.yaml")
		if err != nil {
			logger.Warn("Gateway config load failed, using defaults", "error", err)
			gatewayCfg = gateway.DefaultConfig()
		}
		if gatewayCfg.Enabled {
			gw := gateway.New(gatewayCfg, logger)
			gw.Install(server)
			logger.Info("MCP Gateway installed")
		} else {
			logger.Info("MCP Gateway disabled in config")
		}
	} else {
		logger.Info("MCP Gateway bypassed (--no-gateway flag)")
	}

	// Auto-register with Claude Code (optional)
	if cfg.AutoRegister {
		if err := registerWithClaudeCode(cfg.ClaudeConfigPath); err != nil {
			logger.Warn("Auto-registration failed (non-fatal)", "error", err)
		} else {
			logger.Info("Auto-registered with Claude Code", "config_path", cfg.ClaudeConfigPath)
		}
	}

	// Determine A2A port: flag overrides config
	effectiveA2APort := cfg.A2A.Port
	if *a2aPort != 0 {
		effectiveA2APort = *a2aPort
	}
	// Config can also enable via a2a.enabled without explicit port
	if cfg.A2A.Enabled && effectiveA2APort == 0 {
		effectiveA2APort = 8080
	}

	// Set up signal-based shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Start A2A HTTP server if enabled
	var httpServer *http.Server
	if effectiveA2APort > 0 {
		bind := cfg.A2A.Bind
		if bind == "" {
			bind = "127.0.0.1"
		}
		addr := fmt.Sprintf("%s:%d", bind, effectiveA2APort)

		handler := newA2AHandler(logger)
		httpServer = &http.Server{
			Addr:              addr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}

		ln, err := net.Listen("tcp", addr) //nolint:noctx // TODO(context): plumb ctx through this layer
		if err != nil {
			logger.Error("A2A HTTP listen failed", "addr", addr, "error", err)
			stop() // explicit cleanup before exit (otherwise the deferred stop() at the top of main wouldn't run)
			os.Exit(1) //nolint:gocritic // stop() called explicitly above
		}

		logger.Info("A2A HTTP server listening", "addr", addr)
		go func() {
			if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("A2A HTTP server error", "error", err)
			}
		}()
	}

	// Create stdio transport (v1.2.0 API)
	transport := &mcp.StdioTransport{}

	// Run MCP server (blocks until connection closes or context cancelled)
	logger.Info("Starting MCP server with stdio transport")
	if err := server.Run(ctx, transport); err != nil {
		logger.Error("Server error", "error", err)
	}

	// Graceful shutdown of A2A HTTP server
	if httpServer != nil {
		logger.Info("Shutting down A2A HTTP server")
		if err := httpServer.Close(); err != nil {
			logger.Error("A2A HTTP shutdown error", "error", err)
		}
	}
}
