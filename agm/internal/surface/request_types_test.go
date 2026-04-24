package surface

import (
	"encoding/json"
	"testing"
)

func TestListSessionsRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  ListSessionsRequest
	}{
		{
			name: "all fields set",
			req: ListSessionsRequest{
				Status:  "active",
				Harness: "claude-code",
				Limit:   50,
				Offset:  10,
			},
		},
		{
			name: "zero value",
			req:  ListSessionsRequest{},
		},
		{
			name: "only status",
			req:  ListSessionsRequest{Status: "archived"},
		},
		{
			name: "max limit",
			req:  ListSessionsRequest{Limit: 1000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got ListSessionsRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestListSessionsRequest_JSONOmitempty(t *testing.T) {
	req := ListSessionsRequest{}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// All fields have omitempty, so zero-value struct should produce empty JSON object
	// (except int zero values which json still includes unless omitempty)
	// Status and Harness (strings) should be omitted when empty
	if _, ok := m["status"]; ok {
		t.Error("zero-value status should be omitted (omitempty)")
	}
	if _, ok := m["harness"]; ok {
		t.Error("zero-value harness should be omitted (omitempty)")
	}
}

func TestGetSessionRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  GetSessionRequest
	}{
		{
			name: "with identifier",
			req:  GetSessionRequest{Identifier: "my-session"},
		},
		{
			name: "empty identifier",
			req:  GetSessionRequest{},
		},
		{
			name: "uuid prefix",
			req:  GetSessionRequest{Identifier: "abc123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got GetSessionRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestSearchSessionsRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  SearchSessionsRequest
	}{
		{
			name: "all fields",
			req: SearchSessionsRequest{
				Query:  "deploy",
				Status: "all",
				Limit:  25,
			},
		},
		{
			name: "query only",
			req:  SearchSessionsRequest{Query: "test"},
		},
		{
			name: "empty",
			req:  SearchSessionsRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got SearchSessionsRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestGetStatusRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  GetStatusRequest
	}{
		{
			name: "include archived",
			req:  GetStatusRequest{IncludeArchived: true},
		},
		{
			name: "exclude archived",
			req:  GetStatusRequest{IncludeArchived: false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got GetStatusRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestArchiveSessionRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  ArchiveSessionRequest
	}{
		{
			name: "with identifier",
			req:  ArchiveSessionRequest{Identifier: "session-to-archive"},
		},
		{
			name: "empty",
			req:  ArchiveSessionRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got ArchiveSessionRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestKillSessionRequest_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		req  KillSessionRequest
	}{
		{
			name: "with identifier",
			req:  KillSessionRequest{Identifier: "stuck-session"},
		},
		{
			name: "empty",
			req:  KillSessionRequest{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.req)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			var got KillSessionRequest
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			if got != tt.req {
				t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, tt.req)
			}
		})
	}
}

func TestListOpsRequest_JSONRoundTrip(t *testing.T) {
	req := ListOpsRequest{}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var got ListOpsRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got != req {
		t.Errorf("round-trip mismatch:\n  got  %+v\n  want %+v", got, req)
	}
}

func TestSearchSessionsRequest_JSONFromExternal(t *testing.T) {
	// Simulate JSON coming from an external source (MCP client)
	input := `{"query":"deploy-fix","status":"active","limit":5}`

	var got SearchSessionsRequest
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Query != "deploy-fix" {
		t.Errorf("Query = %q, want %q", got.Query, "deploy-fix")
	}
	if got.Status != "active" {
		t.Errorf("Status = %q, want %q", got.Status, "active")
	}
	if got.Limit != 5 {
		t.Errorf("Limit = %d, want %d", got.Limit, 5)
	}
}

func TestListSessionsRequest_JSONFromExternal(t *testing.T) {
	input := `{"status":"all","harness":"claude-code","limit":200,"offset":50}`

	var got ListSessionsRequest
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if got.Status != "all" {
		t.Errorf("Status = %q, want %q", got.Status, "all")
	}
	if got.Harness != "claude-code" {
		t.Errorf("Harness = %q, want %q", got.Harness, "claude-code")
	}
	if got.Limit != 200 {
		t.Errorf("Limit = %d, want %d", got.Limit, 200)
	}
	if got.Offset != 50 {
		t.Errorf("Offset = %d, want %d", got.Offset, 50)
	}
}

func TestGetStatusRequest_OmitemptyBehavior(t *testing.T) {
	req := GetStatusRequest{IncludeArchived: false}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("Unmarshal to map error: %v", err)
	}

	// include_archived has omitempty, so false should be omitted
	if _, ok := m["include_archived"]; ok {
		t.Error("false include_archived should be omitted (omitempty)")
	}
}
