package ops

// GetStatusRequest defines input for getting status of all sessions.
type GetStatusRequest struct {
	// IncludeArchived includes archived sessions in the status report.
	IncludeArchived bool `json:"include_archived,omitempty"`
}

// SessionStatusEntry is a single session's status.
type SessionStatusEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Harness string `json:"harness"`
	Project string `json:"project"`
}

// GetStatusResult is the output of GetStatus.
type GetStatusResult struct {
	Operation string               `json:"operation"`
	Sessions  []SessionStatusEntry `json:"sessions"`
	Summary   StatusSummary        `json:"summary"`
}

// StatusSummary provides aggregate counts.
type StatusSummary struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Stopped  int `json:"stopped"`
	Archived int `json:"archived"`
}

// GetStatus returns the live status of all sessions.
func GetStatus(ctx *OpContext, req *GetStatusRequest) (*GetStatusResult, error) {
	if req == nil {
		req = &GetStatusRequest{}
	}

	listReq := &ListSessionsRequest{
		Limit: 1000,
	}
	if req.IncludeArchived {
		listReq.Status = "all"
	} else {
		listReq.Status = "active"
	}

	listResult, err := ListSessions(ctx, listReq)
	if err != nil {
		return nil, err
	}

	entries := make([]SessionStatusEntry, 0, len(listResult.Sessions))
	summary := StatusSummary{Total: len(listResult.Sessions)}

	for _, s := range listResult.Sessions {
		entries = append(entries, SessionStatusEntry{
			ID:      s.ID,
			Name:    s.Name,
			Status:  s.Status,
			Harness: s.Harness,
			Project: s.Project,
		})

		switch s.Status {
		case "active":
			summary.Active++
		case "stopped":
			summary.Stopped++
		case "archived":
			summary.Archived++
		}
	}

	return &GetStatusResult{
		Operation: "get_status",
		Sessions:  entries,
		Summary:   summary,
	}, nil
}
