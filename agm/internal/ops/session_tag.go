package ops

import "fmt"

// TagSessionRequest defines the input for tagging a session.
type TagSessionRequest struct {
	// Identifier is the session name, ID, or tmux name.
	Identifier string `json:"identifier"`

	// Add is the tag to add (namespace:value format, e.g. "role:worker").
	Add string `json:"add,omitempty"`

	// Remove is the tag to remove.
	Remove string `json:"remove,omitempty"`
}

// TagSessionResult is the output of TagSession.
type TagSessionResult struct {
	Operation string   `json:"operation"`
	SessionID string   `json:"session_id"`
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
	Action    string   `json:"action"` // "added" or "removed"
	Tag       string   `json:"tag"`
}

// TagSession adds or removes a tag on an existing session.
func TagSession(ctx *OpContext, req *TagSessionRequest) (*TagSessionResult, error) {
	if req.Add == "" && req.Remove == "" {
		return nil, ErrInvalidInput("tag", "Provide a tag to add or --remove <tag>.")
	}
	if req.Add != "" && req.Remove != "" {
		return nil, ErrInvalidInput("tag", "Cannot add and remove a tag in the same operation.")
	}

	// Resolve session
	getResult, err := GetSession(ctx, &GetSessionRequest{Identifier: req.Identifier})
	if err != nil {
		return nil, err
	}

	m, err := ctx.Storage.GetSession(getResult.Session.ID)
	if err != nil {
		return nil, ErrStorageError("get_session", err)
	}

	var action string

	if req.Add != "" {
		// Check for duplicate
		for _, t := range m.Context.Tags {
			if t == req.Add {
				// Tag already present — return current state without error
				return &TagSessionResult{
					Operation: "tag_session",
					SessionID: m.SessionID,
					Name:      m.Name,
					Tags:      append([]string(nil), m.Context.Tags...),
					Action:    "noop",
					Tag:       req.Add,
				}, nil
			}
		}
		m.Context.Tags = append(m.Context.Tags, req.Add)
		action = "added"
	} else {
		// Remove tag
		found := false
		tags := make([]string, 0, len(m.Context.Tags))
		for _, t := range m.Context.Tags {
			if t == req.Remove {
				found = true
				continue
			}
			tags = append(tags, t)
		}
		if !found {
			return nil, ErrInvalidInput("tag", fmt.Sprintf("Tag %q not found on session %s.", req.Remove, m.Name))
		}
		m.Context.Tags = tags
		action = "removed"
	}

	if err := ctx.Storage.UpdateSession(m); err != nil {
		return nil, ErrStorageError("update_session", err)
	}

	return &TagSessionResult{
		Operation: "tag_session",
		SessionID: m.SessionID,
		Name:      m.Name,
		Tags:      append([]string(nil), m.Context.Tags...),
		Action:    action,
		Tag:       req.Add + req.Remove,
	}, nil
}
