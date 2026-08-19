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
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gateway"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

// verifyWorkspaceDB resolves the Dolt config and opens the store to confirm the
// workspace's database is reachable, then closes it. It is the boot-time gate
// that turns a silent "handless" tool surface into a loud startup failure
// (ce-vj8a). dolt.New pings the connection (and auto-starts Dolt if configured),
// so a non-existent DB (e.g. an empty 'personal' workspace) returns an error.
func verifyWorkspaceDB() error {
	cfg, err := dolt.DefaultConfig()
	if err != nil {
		return err
	}
	adapter, err := dolt.New(cfg)
	if err != nil {
		return err
	}
	return adapter.Close()
}

// logger writes to stderr (required for stdio MCP transport)
var logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

// Main entry point for AGM MCP server
// Adapted from Engram MCP server pattern
func main() {
	pkgversion.PopulateFromBuildInfo()

	// Parse flags
	a2aPort := flag.Int("a2a-port", 0, "A2A HTTP port (0=disabled, e.g. 8080 to enable)")
	noGateway := flag.Bool("no-gateway", false, "Bypass MCP Gateway middleware")
	flag.Parse()

	// Print header to stderr (stdio transport requirement: logs go to stderr)
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	fmt.Fprintf(os.Stderr, "agm-mcp-server %s (%s)\n", pkgversion.Version, executable)

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

	// Resolve the Dolt workspace (WORKSPACE env, else mcp-server.yaml) and FAIL
	// LOUD if none. Claude Desktop launches the server without the shell
	// environment, so WORKSPACE is often unset; without an explicit workspace the
	// Dolt adapter silently falls back to default_workspace ('personal', which
	// has no DB) and the server boots a non-functional tool surface — the
	// recurring "handless MCP" outage (ce-vj8a). Refuse to start instead.
	ws, err := resolveWorkspace(cfg.Workspace, os.Getenv)
	if err != nil {
		logger.Error("refusing to start — no Dolt workspace (ce-vj8a)", "error", err)
		fmt.Fprintf(os.Stderr, "FATAL: agm-mcp-server cannot start: %v\n", err)
		os.Exit(1)
	}
	if err := os.Setenv("WORKSPACE", ws); err != nil {
		logger.Error("failed to set WORKSPACE", "workspace", ws, "error", err)
		os.Exit(1)
	}
	logger.Info("Resolved Dolt workspace", "workspace", ws)

	// Verify the workspace's Dolt DB is actually reachable BEFORE registering any
	// tools. A resolved-but-unreachable workspace would otherwise register a
	// non-functional tool surface (ce-vj8a). Fail loud with an actionable error.
	if err := verifyWorkspaceDB(); err != nil {
		logger.Error("refusing to start — Dolt DB not reachable (ce-vj8a)", "workspace", ws, "error", err)
		fmt.Fprintf(os.Stderr, "FATAL: agm-mcp-server: workspace %q has no reachable Dolt DB: %v\n", ws, err)
		os.Exit(1)
	}

	logger.Info("Starting AGM MCP Server", "version", pkgversion.Version)
	logger.Info("Configuration loaded", "sessions_dir", cfg.SessionsDir)

	// Create MCP server.
	server := newMCPServer()

	registerMCPTools(server, cfg)

	logger.Info("Registered MCP tools", "tools", "agm_list_sessions, agm_search_sessions, agm_get_session_metadata, agm_get_session_output, agm_archive_session, agm_kill_session, agm_create_session, agm_send_message, agm_list_ops, engram_list_wayfinder_sessions, engram_get_wayfinder_session")
	logger.Info("Wayfinder forwarding enabled", "engram_mcp_url", cfg.EngramMCPURL)

	installGateway(server, *noGateway)

	// Auto-register with Claude Code (optional)
	if cfg.AutoRegister {
		if err := registerWithClaudeCode(cfg.ClaudeConfigPath); err != nil {
			logger.Warn("Auto-registration failed (non-fatal)", "error", err)
		} else {
			logger.Info("Auto-registered with Claude Code", "config_path", cfg.ClaudeConfigPath)
		}
	}

	effectiveA2APort := resolveA2APort(cfg, *a2aPort)

	// Set up signal-based shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Belt-and-suspenders: exit if our parent process dies.
	// The go-sdk StdioTransport handles stdin EOF (belt 1); this goroutine handles
	// the OOM-kill scenario where the parent is killed hard and stdin EOF may not
	// arrive before the OS reparents us to PID 1 (belt 2).
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("parent-monitor panicked", "recover", r)
				stop()
			}
		}()
		ppid := os.Getppid()
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if os.Getppid() != ppid {
					logger.Info("Parent process exited (reparented), shutting down")
					stop()
					return
				}
			}
		}
	}()

	httpServer := startA2AServerIfEnabled(cfg, effectiveA2APort, stop)

	// Create the stdio transport.
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

func newMCPServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:    "agm",
		Version: pkgversion.Version,
	}, nil)
}

// registerMCPTools owns the provider-visible AGM MCP registration set. Tests
// call this exact seam so registry compatibility is checked against the tools
// compiled into the production server. The SDK sorts tool identifiers during
// discovery, so call order is deliberately not part of the wire contract.
func registerMCPTools(server *mcp.Server, cfg *Config) {
	addListSessionsTool(server, cfg)
	addSearchSessionsTool(server, cfg)
	addGetSessionMetadataTool(server, cfg)
	addGetSessionOutputTool(server, cfg)
	addArchiveSessionTool(server, cfg)
	addKillSessionTool(server, cfg)
	addCreateSessionTool(server, cfg)
	addSendMessageTool(server, cfg)
	addCompletionRelayTargetTools(server, cfg)
	addQuotaStatusTool(server, cfg)
	addListOpsTool(server, cfg)
	addListWayfinderSessionsTool(server, cfg)
	addGetWayfinderSessionTool(server, cfg)
}

// installGateway installs the MCP gateway middleware unless disabled by flag.
func installGateway(server *mcp.Server, noGateway bool) {
	if noGateway || slices.Contains(os.Args[1:], "--no-gateway") {
		logger.Info("MCP Gateway bypassed (--no-gateway flag)")
		return
	}
	gatewayCfg, err := gateway.LoadConfig("~/.config/agm/gateway.yaml")
	if err != nil {
		logger.Warn("Gateway config load failed, using defaults", "error", err)
		gatewayCfg = gateway.DefaultConfig()
	}
	if !gatewayCfg.Enabled {
		logger.Info("MCP Gateway disabled in config")
		return
	}
	gw := gateway.New(gatewayCfg, logger)
	gw.Install(server)
	logger.Info("MCP Gateway installed")
}

// resolveA2APort determines the effective A2A port: flag overrides config, and
// if a2a.enabled is true with no explicit port, defaults to 8080.
func resolveA2APort(cfg *Config, flagPort int) int {
	port := cfg.A2A.Port
	if flagPort != 0 {
		port = flagPort
	}
	if cfg.A2A.Enabled && port == 0 {
		port = 8080
	}
	return port
}

// startA2AServerIfEnabled launches the A2A HTTP server in a goroutine when
// effectiveA2APort > 0. On listen failure it calls stop() and exits the process.
// Returns nil when A2A is disabled.
func startA2AServerIfEnabled(cfg *Config, effectiveA2APort int, stop func()) *http.Server {
	if effectiveA2APort <= 0 {
		return nil
	}
	bind := cfg.A2A.Bind
	if bind == "" {
		bind = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", bind, effectiveA2APort)

	handler := newA2AHandler(logger)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ln, err := net.Listen("tcp", addr) //nolint:noctx // TODO(context): plumb ctx through this layer
	if err != nil {
		logger.Error("A2A HTTP listen failed", "addr", addr, "error", err)
		stop()     // explicit cleanup before exit (otherwise the deferred stop() at the top of main wouldn't run)
		os.Exit(1) //nolint:gocritic // stop() called explicitly above
	}

	logger.Info("A2A HTTP server listening", "addr", addr)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("A2A HTTP server panicked", "recover", r)
			}
		}()
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("A2A HTTP server error", "error", err)
		}
	}()
	return httpServer
}
