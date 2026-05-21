package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readServers loads the config file and returns the map that should hold the
// "agm" entry, accounting for both the flat and nested ("mcpServers") shapes.
func readServers(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	root := map[string]any{}
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if nested, ok := root["mcpServers"].(map[string]any); ok {
		return nested
	}
	return root
}

func TestRegisterWithClaudeCode_CreatesFlatConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "mcp_servers.json")

	if err := registerWithClaudeCode(path); err != nil {
		t.Fatalf("register: %v", err)
	}

	servers := readServers(t, path)
	entry, ok := servers["agm"].(map[string]any)
	if !ok {
		t.Fatalf("agm entry missing or wrong type: %#v", servers["agm"])
	}
	cmd, _ := entry["command"].(string)
	if cmd == "" {
		t.Fatalf("agm command not recorded: %#v", entry)
	}
}

func TestRegisterWithClaudeCode_PreservesNestedShapeAndOtherServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	seed := map[string]any{
		"mcpServers": map[string]any{
			"other": map[string]any{"command": "/usr/bin/other"},
		},
		"unrelated": "keep-me",
	}
	data, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := registerWithClaudeCode(path); err != nil {
		t.Fatalf("register: %v", err)
	}

	// Top-level unrelated key and the nested shape must survive.
	full := map[string]any{}
	raw, _ := os.ReadFile(path)
	if err := json.Unmarshal(raw, &full); err != nil {
		t.Fatal(err)
	}
	if full["unrelated"] != "keep-me" {
		t.Errorf("unrelated top-level key lost: %#v", full["unrelated"])
	}
	servers := readServers(t, path)
	if _, ok := servers["other"]; !ok {
		t.Errorf("pre-existing 'other' server dropped: %#v", servers)
	}
	if _, ok := servers["agm"].(map[string]any); !ok {
		t.Errorf("agm not registered in nested shape: %#v", servers)
	}
}

func TestRegisterWithClaudeCode_PreservesUserFieldsOnCommandUpdate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp_servers.json")
	// Pre-existing agm entry with a stale command plus user-defined fields.
	seed := map[string]any{
		"agm": map[string]any{
			"command": "/old/path/agm-mcp-server",
			"args":    []any{"--no-gateway"},
			"env":     map[string]any{"AGM_SESSIONS_DIR": "/custom"},
		},
	}
	data, _ := json.MarshalIndent(seed, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := registerWithClaudeCode(path); err != nil {
		t.Fatalf("register: %v", err)
	}

	entry := readServers(t, path)["agm"].(map[string]any)
	if cmd, _ := entry["command"].(string); cmd == "/old/path/agm-mcp-server" {
		t.Errorf("command was not updated: %v", entry["command"])
	}
	if _, ok := entry["args"]; !ok {
		t.Errorf("user-defined args dropped: %#v", entry)
	}
	if _, ok := entry["env"]; !ok {
		t.Errorf("user-defined env dropped: %#v", entry)
	}
}

func TestRegisterWithClaudeCode_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp_servers.json")

	if err := registerWithClaudeCode(path); err != nil {
		t.Fatalf("first register: %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := registerWithClaudeCode(path); err != nil {
		t.Fatalf("second register: %v", err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("re-registration mutated config:\nfirst=%s\nsecond=%s", first, second)
	}
}

func TestRegisterWithClaudeCode_RejectsMalformedMcpServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".claude.json")
	// mcpServers present but the wrong type — must not clobber.
	if err := os.WriteFile(path, []byte(`{"mcpServers":"oops"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := registerWithClaudeCode(path); err == nil {
		t.Fatalf("expected error on malformed mcpServers, got nil")
	}
}
