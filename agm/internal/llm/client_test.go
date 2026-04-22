package llm

import (
	"strings"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ClientConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config with all fields",
			cfg: ClientConfig{
				ProjectID: "my-project",
				Location:  "us-east1",
				ModelID:   "claude-3-opus",
				RateLimit: 20,
			},
			wantErr: false,
		},
		{
			name: "valid config with defaults",
			cfg: ClientConfig{
				ProjectID: "my-project",
			},
			wantErr: false,
		},
		{
			name:    "missing project ID",
			cfg:     ClientConfig{},
			wantErr: true,
			errMsg:  "project ID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tt.errMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if client == nil {
				t.Fatal("client should not be nil")
			}
		})
	}
}

func TestNewClient_Defaults(t *testing.T) {
	client, err := NewClient(ClientConfig{ProjectID: "test-proj"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.location != "us-central1" {
		t.Errorf("default location = %q, want %q", client.location, "us-central1")
	}
	if client.modelID != "claude-3-5-haiku@20241022" {
		t.Errorf("default modelID = %q, want %q", client.modelID, "claude-3-5-haiku@20241022")
	}
	if client.limiter == nil {
		t.Error("limiter should not be nil")
	}
}

func TestNewClient_CustomValues(t *testing.T) {
	client, err := NewClient(ClientConfig{
		ProjectID: "proj",
		Location:  "europe-west1",
		ModelID:   "claude-3-opus",
		RateLimit: 5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if client.location != "europe-west1" {
		t.Errorf("location = %q, want %q", client.location, "europe-west1")
	}
	if client.modelID != "claude-3-opus" {
		t.Errorf("modelID = %q, want %q", client.modelID, "claude-3-opus")
	}
	if client.projectID != "proj" {
		t.Errorf("projectID = %q, want %q", client.projectID, "proj")
	}
}

func TestBuildSearchPrompt(t *testing.T) {
	client := &Client{
		projectID: "test",
		location:  "us-central1",
		modelID:   "test-model",
	}

	tests := []struct {
		name      string
		req       SearchRequest
		wantParts []string
	}{
		{
			name: "basic query",
			req: SearchRequest{
				Query: "debugging authentication",
				Sessions: []SessionMetadata{
					{SessionID: "s1", Name: "auth-fix"},
				},
			},
			wantParts: []string{
				"debugging authentication",
				"Session ID: s1",
				"Name: auth-fix",
				"JSON array",
			},
		},
		{
			name: "sessions with tags and project",
			req: SearchRequest{
				Query: "test query",
				Sessions: []SessionMetadata{
					{
						SessionID: "s1",
						Name:      "feature-x",
						Tags:      []string{"go", "testing"},
						Project:   "ai-tools",
					},
				},
			},
			wantParts: []string{
				"Tags: [go, testing]",
				"Project: ai-tools",
			},
		},
		{
			name: "session without tags or project",
			req: SearchRequest{
				Query: "q",
				Sessions: []SessionMetadata{
					{SessionID: "s1", Name: "plain"},
				},
			},
			wantParts: []string{
				"Session ID: s1",
				"Name: plain",
			},
		},
		{
			name: "multiple sessions",
			req: SearchRequest{
				Query: "search",
				Sessions: []SessionMetadata{
					{SessionID: "s1", Name: "first"},
					{SessionID: "s2", Name: "second"},
					{SessionID: "s3", Name: "third"},
				},
			},
			wantParts: []string{
				"1. Session ID: s1",
				"2. Session ID: s2",
				"3. Session ID: s3",
			},
		},
		{
			name: "empty sessions",
			req: SearchRequest{
				Query:    "anything",
				Sessions: []SessionMetadata{},
			},
			wantParts: []string{
				"anything",
				"Available sessions:",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := client.buildSearchPrompt(tt.req)

			for _, part := range tt.wantParts {
				if !strings.Contains(got, part) {
					t.Errorf("buildSearchPrompt() missing %q in:\n%s", part, got)
				}
			}
		})
	}
}

func TestSearchResult_Fields(t *testing.T) {
	r := SearchResult{
		SessionID: "abc-123",
		Relevance: 0.85,
		Reason:    "Ranked #1 by LLM",
	}

	if r.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q", r.SessionID, "abc-123")
	}
	if r.Relevance != 0.85 {
		t.Errorf("Relevance = %f, want 0.85", r.Relevance)
	}
	if r.Reason != "Ranked #1 by LLM" {
		t.Errorf("Reason = %q, want %q", r.Reason, "Ranked #1 by LLM")
	}
}
