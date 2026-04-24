package ops

import (
	"github.com/vbonnet/dear-agent/agm/internal/discovery"
)

// ListWorkspacesRequest defines input for listing workspaces.
type ListWorkspacesRequest struct{}

// WorkspaceInfo is a single workspace entry for JSON output.
type WorkspaceInfo struct {
	Name      string `json:"name"`
	Root      string `json:"root"`
	OutputDir string `json:"output_dir,omitempty"`
	Enabled   bool   `json:"enabled"`
}

// ListWorkspacesResult is the output of ListWorkspaces.
type ListWorkspacesResult struct {
	Operation  string          `json:"operation"`
	Workspaces []WorkspaceInfo `json:"workspaces"`
	Total      int             `json:"total"`
}

// ListWorkspaces returns all configured workspaces.
func ListWorkspaces(ctx *OpContext, _ *ListWorkspacesRequest) (*ListWorkspacesResult, error) {
	wsInfos, err := discovery.ListWorkspacesUsingContract()
	if err != nil {
		// Graceful degradation: workspace CLI may not be available
		return &ListWorkspacesResult{
			Operation:  "list_workspaces",
			Workspaces: []WorkspaceInfo{},
			Total:      0,
		}, nil
	}

	infos := make([]WorkspaceInfo, 0, len(wsInfos))
	for _, ws := range wsInfos {
		infos = append(infos, WorkspaceInfo{
			Name:      ws.Name,
			Root:      ws.Root,
			OutputDir: ws.OutputDir,
			Enabled:   ws.Enabled,
		})
	}

	return &ListWorkspacesResult{
		Operation:  "list_workspaces",
		Workspaces: infos,
		Total:      len(infos),
	}, nil
}
