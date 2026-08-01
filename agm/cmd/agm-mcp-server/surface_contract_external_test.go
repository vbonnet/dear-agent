package main

import (
	"fmt"
	"strings"
	"testing"
)

func compareExactExternalToolContracts(live map[string]contractTool) error {
	expected := exactExternalToolContracts()
	for _, name := range sortedKeys(expected) {
		want := expected[name]
		got, ok := live[name]
		if !ok {
			return fmt.Errorf("DAH-002/external-tool-missing: %q", name)
		}
		if got.Name != want.Name {
			return fmt.Errorf("DAH-002/external-tool-name: key=%q got=%q want=%q", name, got.Name, want.Name)
		}
		if got.Description != want.Description {
			return fmt.Errorf("DAH-002/external-tool-description: tool=%q got=%q want=%q",
				name, got.Description, want.Description)
		}
		if differences := compareToolSchemas(want.Operation, want, got, nil); len(differences) > 0 {
			difference := differences[0]
			return fmt.Errorf("%s: %s", externalSchemaFailureKey(difference.Dimension),
				describeCompatibility(difference))
		}
	}
	return nil
}

func externalSchemaFailureKey(dimension string) string {
	return "DAH-002/external-tool-schema-" + strings.ReplaceAll(dimension, "_", "-")
}

func exactExternalToolContracts() map[string]contractTool {
	return map[string]contractTool{
		"agm_create_session": {
			Operation: "create_session",
			Name:      "agm_create_session",
			Description: "Create a new AGM session (tmux + harness + manifest). " +
				"Use when you need to spawn a new agent harness session programmatically.",
			Nodes: map[string]contractSchemaNode{
				"/":        exactExternalRoot(),
				"/cwd":     exactExternalField("/cwd", "string", "Absolute path to the working directory for the new session (required)", true),
				"/harness": exactExternalField("/harness", "string", "Agent harness: claude-code, codex-cli, agy, opencode-cli, pi-cli, or deprecated gemini-cli. Defaults to claude-code.", false),
				"/model":   exactExternalField("/model", "string", "Model to use (e.g. sonnet, 5.5, 5.6, 3.5-flash, z-ai/glm-5.2). Defaults to the selected harness default.", false),
				"/prompt":  exactExternalField("/prompt", "string", "Initial prompt to send to the session after startup (required)", true),
				"/title":   exactExternalField("/title", "string", "Session name. If omitted, derived from cwd directory name.", false),
			},
		},
		"agm_send_message": {
			Operation:   "send_message",
			Name:        "agm_send_message",
			Description: "Send a message to a running AGM session. Use when you need to deliver a prompt or instruction to an existing session.",
			Nodes: map[string]contractSchemaNode{
				"/":           exactExternalRoot(),
				"/message":    exactExternalField("/message", "string", "Message text to send to the session (required)", true),
				"/session_id": exactExternalField("/session_id", "string", "Session ID, name, or UUID prefix of the target session (required)", true),
			},
		},
		"engram_get_wayfinder_session": {
			Operation:   "get_wayfinder_session",
			Name:        "engram_get_wayfinder_session",
			Description: "Get detailed Wayfinder session info by ID. Use when you need phase status for a specific project.",
			Nodes: map[string]contractSchemaNode{
				"/":           exactExternalRoot(),
				"/session_id": exactExternalField("/session_id", "string", "Session UUID", true),
			},
		},
		"engram_list_wayfinder_sessions": {
			Operation:   "list_wayfinder_sessions",
			Name:        "engram_list_wayfinder_sessions",
			Description: "List Wayfinder sessions from Engram. Use when checking status of SDLC projects.",
			Nodes: map[string]contractSchemaNode{
				"/":              exactExternalRoot(),
				"/limit":         exactExternalField("/limit", "integer", "Maximum sessions to return (max 1000, default 100)", false),
				"/status_filter": exactExternalField("/status_filter", "string", "Filter by status: active, completed, failed, abandoned", false),
			},
		},
	}
}

func exactExternalRoot() contractSchemaNode {
	return contractSchemaNode{
		Path: "/", Type: "object", ItemType: "", Description: "", Enum: "",
		AdditionalProperties: "false", Required: false,
	}
}

func exactExternalField(path, typeName, description string, required bool) contractSchemaNode {
	return contractSchemaNode{
		Path: path, Type: typeName, ItemType: "", Description: description, Enum: "",
		AdditionalProperties: contractAbsent, Required: required,
	}
}

func TestExactExternalMCPToolContractsRejectDrift(t *testing.T) {
	live := compiledContract(t, registeredMCPTools(t, registerMCPTools))
	tests := []struct {
		name    string
		wantKey string
		mutate  func(map[string]contractTool)
	}{
		{
			name:    "missing tool",
			wantKey: "DAH-002/external-tool-missing",
			mutate:  func(got map[string]contractTool) { delete(got, "agm_create_session") },
		},
		{
			name:    "tool name",
			wantKey: "DAH-002/external-tool-name",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_create_session"]
				tool.Name = "agm_create_session_drift"
				got["agm_create_session"] = tool
			},
		},
		{
			name:    "tool description",
			wantKey: "DAH-002/external-tool-description",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_create_session"]
				tool.Description += " drift"
				got[tool.Name] = tool
			},
		},
		{
			name:    "path omission",
			wantKey: "DAH-002/external-tool-schema-path-omission",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_create_session"]
				delete(tool.Nodes, "/title")
				got[tool.Name] = tool
			},
		},
		{
			name:    "path addition",
			wantKey: "DAH-002/external-tool-schema-path-addition",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_create_session"]
				tool.Nodes["/surprise"] = exactExternalField("/surprise", "string", "unexpected", false)
				got[tool.Name] = tool
			},
		},
		{
			name:    "type",
			wantKey: "DAH-002/external-tool-schema-type",
			mutate: externalNodeMutation("agm_create_session", "/model", func(node *contractSchemaNode) {
				node.Type = "integer"
			}),
		},
		{
			name:    "item type",
			wantKey: "DAH-002/external-tool-schema-item-type",
			mutate: externalNodeMutation("agm_create_session", "/model", func(node *contractSchemaNode) {
				node.ItemType = "string"
			}),
		},
		{
			name:    "requiredness",
			wantKey: "DAH-002/external-tool-schema-requiredness",
			mutate: externalNodeMutation("agm_create_session", "/title", func(node *contractSchemaNode) {
				node.Required = true
			}),
		},
		{
			name:    "enum",
			wantKey: "DAH-002/external-tool-schema-enum",
			mutate: externalNodeMutation("agm_create_session", "/harness", func(node *contractSchemaNode) {
				node.Enum = "codex-cli"
				node.EnumPresent = true
			}),
		},
		{
			name:    "field description",
			wantKey: "DAH-002/external-tool-schema-description",
			mutate: externalNodeMutation("agm_create_session", "/title", func(node *contractSchemaNode) {
				node.Description += " drift"
			}),
		},
		{
			name:    "closed object",
			wantKey: "DAH-002/external-tool-schema-closed-object",
			mutate: externalNodeMutation("agm_create_session", "/", func(node *contractSchemaNode) {
				node.AdditionalProperties = "true"
			}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := cloneContract(live)
			tt.mutate(fixture)
			err := compareExactExternalToolContracts(fixture)
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("compareExactExternalToolContracts() error = %v, want stable key %q", err, tt.wantKey)
			}
		})
	}
}

func externalNodeMutation(
	toolName string,
	path string,
	mutate func(*contractSchemaNode),
) func(map[string]contractTool) {
	return func(got map[string]contractTool) {
		tool := got[toolName]
		node := tool.Nodes[path]
		mutate(&node)
		tool.Nodes[path] = node
		got[toolName] = tool
	}
}
