package main

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/engram/internal/beadstore"
)

// --- Beads tools (the ce-ctsi fix) ---

// BeadsCreateInput mirrors the legacy beads_create schema so existing callers
// keep working, but priority is bd's 0-4 range.
type BeadsCreateInput struct {
	Title            string   `json:"title" jsonschema:"Bead title (brief, imperative form)"`
	Description      string   `json:"description" jsonschema:"Detailed description with context and acceptance criteria"`
	Priority         *int     `json:"priority,omitempty" jsonschema:"Priority 0-4 (0=P0/highest). Default 2."`
	Labels           []string `json:"labels,omitempty" jsonschema:"Labels/tags for categorization"`
	EstimatedMinutes int      `json:"estimated_minutes,omitempty" jsonschema:"Estimated time in minutes (default 60)"`
	IssueType        string   `json:"issue_type,omitempty" jsonschema:"bd issue type: bug, feature, task, epic, chore, decision (default task)"`
}

// BeadsCreateOutput reports a verified write.
type BeadsCreateOutput struct {
	BeadID   string          `json:"bead_id"`
	Verified bool            `json:"verified"` // always true on success: row was read back from the store
	DBPath   string          `json:"db_path"`
	Bead     *beadstore.Bead `json:"bead"`
}

func newStore(cfg *Config) *beadstore.Store {
	return &beadstore.Store{BDPath: cfg.BDPath, DBPath: cfg.BeadsDB}
}

func addBeadsCreateTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "beads_create",
		Description: "Create a new bead (issue/task) in the beads database. " +
			"The write is verified read-after-write against the configured store " +
			"and any failure is a hard error — success means the row is durably readable.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BeadsCreateInput) (*mcp.CallToolResult, any, error) {
		priority := 2
		if input.Priority != nil {
			priority = *input.Priority
		}
		estimate := input.EstimatedMinutes
		if estimate == 0 {
			estimate = 60
		}

		bead, err := newStore(cfg).VerifiedCreate(ctx, beadstore.CreateRequest{
			Title:            input.Title,
			Description:      input.Description,
			Priority:         priority,
			Labels:           input.Labels,
			EstimatedMinutes: estimate,
			IssueType:        input.IssueType,
		})
		if err != nil {
			return mcpError(err), nil, nil
		}

		out := &BeadsCreateOutput{
			BeadID:   bead.ID,
			Verified: true,
			DBPath:   cfg.BeadsDB,
			Bead:     bead,
		}
		return mcpSuccess(out), out, nil
	})
}

// BeadsReconcileInput configures a backfill run.
type BeadsReconcileInput struct {
	LegacyJSONLPath string `json:"legacy_jsonl_path,omitempty" jsonschema:"Legacy JSONL store to backfill from (default: ~/.beads/issues.jsonl, the pre-fix beads_create sink)"`
	DryRun          bool   `json:"dry_run,omitempty" jsonschema:"Report what would be backfilled without writing"`
}

func addBeadsReconcileTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name: "beads_reconcile",
		Description: "Backfill beads from a legacy JSONL store that were acknowledged but never " +
			"landed in the beads database (silent write loss). Idempotent: already-backfilled " +
			"beads (backfill-src label) and title matches are skipped.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BeadsReconcileInput) (*mcp.CallToolResult, any, error) {
		legacyPath := input.LegacyJSONLPath
		if legacyPath == "" {
			legacyPath = cfg.LegacyJSONL
		}

		res, err := newStore(cfg).Reconcile(ctx, legacyPath, input.DryRun)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return mcpSuccess(res), res, nil
	})
}

// --- Read tools (ports of the legacy Python server tools) ---

// EngramRetrieveInput mirrors the legacy engram_retrieve schema.
type EngramRetrieveInput struct {
	Query      string `json:"query" jsonschema:"Search query"`
	TypeFilter string `json:"type_filter,omitempty" jsonschema:"Filter by engram type: ai (.ai.md), why (.why.md), or all (default)"`
	TopK       int    `json:"top_k,omitempty" jsonschema:"Number of results to return (default 5, max 20)"`
}

func addEngramRetrieveTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_retrieve",
		Description: "Retrieve relevant engrams using semantic search. Supports .ai.md (actionable instructions) and .why.md (rationale) engrams.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input EngramRetrieveInput) (*mcp.CallToolResult, any, error) {
		res, err := engramRetrieve(ctx, cfg, input)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return mcpSuccess(res), res, nil
	})
}

// EngramPluginsListInput has no parameters.
type EngramPluginsListInput struct{}

func addEngramPluginsListTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_plugins_list",
		Description: "List installed Engram plugins with metadata (name, version, type, description).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ EngramPluginsListInput) (*mcp.CallToolResult, any, error) {
		res := pluginsList(cfg.EngramRoot)
		return mcpSuccess(res), res, nil
	})
}

// WayfinderPhaseStatusInput identifies the project to inspect.
type WayfinderPhaseStatusInput struct {
	ProjectPath string `json:"project_path" jsonschema:"Path to the Wayfinder project directory"`
}

func addWayfinderPhaseStatusTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "wayfinder_phase_status",
		Description: "Get current phase status of a Wayfinder project (phase, progress, status).",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input WayfinderPhaseStatusInput) (*mcp.CallToolResult, any, error) {
		res, err := wayfinderStatus(input.ProjectPath)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return mcpSuccess(res), res, nil
	})
}

// --- Result helpers (same shape as agm-mcp-server) ---

func mcpSuccess(result any) *mcp.CallToolResult {
	data, _ := json.Marshal(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}
}

func mcpError(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
		IsError: true,
	}
}
