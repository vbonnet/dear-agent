// Command engram-mcp is the Engram MCP server.
//
// It supersedes the legacy Python server (ai-tools engram/mcp/
// engram_mcp_server.py) whose beads_create tool appended to a private JSONL
// file and reported success while the bd/dolt beads database never saw the
// write (ce-ctsi). This server's beads tools go through
// engram/internal/beadstore: explicit --db, hard errors, read-after-write
// verification, and a reconcile tool to backfill silently-dropped beads.
//
// Configuration (environment):
//
//	BEADS_DB     Path to the bd database (e.g. ~/beads/context-engine/.beads).
//	             Required for the beads tools; there is NO fallback store —
//	             unconfigured writes fail hard instead of landing nowhere.
//	BD_PATH      bd binary override (default: "bd" from PATH).
//	ENGRAM_ROOT  Engram root for plugin discovery (default: ~/.engram).
//	ENGRAM_CLI   engram binary override (default: "engram" from PATH).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	pkgversion "github.com/vbonnet/dear-agent/pkg/version"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("Server error", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg := configFromEnv()
	logger.Info("Starting Engram MCP Server",
		"version", pkgversion.Version,
		"beads_db", cfg.BeadsDB,
		"engram_root", cfg.EngramRoot)
	if cfg.BeadsDB == "" {
		// Do not exit: the read tools still work. The beads tools return a
		// hard error per call instead of silently writing to a fallback.
		logger.Warn("BEADS_DB not set: beads_create/beads_reconcile will hard-error until configured")
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "engram",
		Version: pkgversion.Version,
	}, nil)

	addBeadsCreateTool(server, cfg)
	addBeadsReconcileTool(server, cfg)
	addEngramRetrieveTool(server, cfg)
	addEngramPluginsListTool(server, cfg)
	addWayfinderPhaseStatusTool(server, cfg)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Exit if the parent MCP host dies without closing stdin (same
	// belt-and-suspenders as agm-mcp-server).
	go watchParent(ctx, stop, logger)

	return server.Run(ctx, &mcp.StdioTransport{})
}

// Config holds the server configuration.
type Config struct {
	BeadsDB    string // bd database path; required for beads tools
	BDPath     string // bd binary; empty = PATH lookup
	EngramRoot string // plugin discovery root
	EngramCLI  string // engram binary; empty = PATH lookup
	// LegacyJSONL is the default legacy store consulted by beads_reconcile
	// when the caller does not pass one.
	LegacyJSONL string
}

func configFromEnv() *Config {
	home, _ := os.UserHomeDir()
	engramRoot := os.Getenv("ENGRAM_ROOT")
	if engramRoot == "" {
		engramRoot = filepath.Join(home, ".engram")
	}
	return &Config{
		BeadsDB:     os.Getenv("BEADS_DB"),
		BDPath:      os.Getenv("BD_PATH"),
		EngramRoot:  engramRoot,
		EngramCLI:   os.Getenv("ENGRAM_CLI"),
		LegacyJSONL: filepath.Join(home, ".beads", "issues.jsonl"),
	}
}

func watchParent(ctx context.Context, stop func(), logger *slog.Logger) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("parent-monitor panicked", "recover", fmt.Sprint(r))
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
				logger.Info("Parent process exited, shutting down")
				stop()
				return
			}
		}
	}
}
