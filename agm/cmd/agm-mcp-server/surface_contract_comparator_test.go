package main

import (
	"strings"
	"testing"
)

func TestMCPCompatibilityComparatorRejectsDriftAndStaleExceptions(t *testing.T) {
	logical := logicalRegistryContract(t)
	live := compiledContract(t, registeredMCPTools(t, registerMCPTools))
	tests := []struct {
		name    string
		wantKey string
		mutate  func(map[string]contractTool)
	}{
		{
			name:    "new compiled tool",
			wantKey: "DAH-002/unaccounted-live-tool",
			mutate: func(got map[string]contractTool) {
				got["agm_surprise"] = contractTool{Name: "agm_surprise", Nodes: map[string]contractSchemaNode{}}
			},
		},
		{
			name:    "removed legacy fields extension",
			wantKey: "DAH-002/list-sessions-fields-addition",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_list_sessions"]
				delete(tool.Nodes, "/fields")
				got[tool.Name] = tool
			},
		},
		{
			name:    "filters wrapper no longer required",
			wantKey: "DAH-002/list-sessions-filters-required",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_list_sessions"]
				node := tool.Nodes["/filters"]
				node.Required = false
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "description exception resolved",
			wantKey: "DAH-002/kill-force-description",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_kill_session"]
				node := tool.Nodes["/force"]
				node.Description = "Bypass the recent-activity safety check"
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "explicit empty enum added",
			wantKey: "DAH-002/unaccounted-enum",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_get_session_metadata"]
				node := tool.Nodes["/identifier"]
				node.Enum = []string{}
				node.EnumPresent = true
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "nested item constraint changes path compatibility record",
			wantKey: "DAH-002/unaccounted-difference",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_list_sessions"]
				node := tool.Nodes["/fields"]
				node.ItemSchema = `{"enum":[],"type":"string"}`
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "property constraint changes path compatibility record",
			wantKey: "DAH-002/unaccounted-difference",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_list_sessions"]
				node := tool.Nodes["/fields"]
				node.Constraints = `{"minItems":1}`
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "item schema added to shared path",
			wantKey: "DAH-002/unaccounted-item-schema",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_get_session_metadata"]
				node := tool.Nodes["/identifier"]
				node.ItemSchema = `{"type":"string"}`
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
		{
			name:    "constraint added to shared path",
			wantKey: "DAH-002/unaccounted-constraints",
			mutate: func(got map[string]contractTool) {
				tool := got["agm_get_session_metadata"]
				node := tool.Nodes["/identifier"]
				node.Constraints = `{"pattern":"^session-"}`
				tool.Nodes[node.Path] = node
				got[tool.Name] = tool
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := cloneContract(live)
			tt.mutate(fixture)
			err := compareCompatibility(logical, fixture, currentCompatibilityRecords())
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("compareCompatibility() error = %v, want stable key %q", err, tt.wantKey)
			}
		})
	}
}

func TestMCPCompatibilityComparatorRejectsOmittedFieldRequirednessDrift(t *testing.T) {
	logical := logicalRegistryContract(t)
	live := compiledContract(t, registeredMCPTools(t, registerMCPTools))
	listSessions := logical["list_sessions"]
	offset := listSessions.Nodes["/offset"]
	offset.Required = true
	listSessions.Nodes[offset.Path] = offset
	logical[listSessions.Operation] = listSessions

	err := compareCompatibility(logical, live, currentCompatibilityRecords())
	if err == nil || !strings.Contains(err.Error(), "DAH-002/unaccounted-requiredness") ||
		!strings.Contains(err.Error(), `registry="/offset":"required"`) {
		t.Fatalf("compareCompatibility() error = %v, want exact /offset requiredness drift", err)
	}
}

func TestCompatibilityRecordsTreatSchemaMetacharactersAsLiteralData(t *testing.T) {
	record := compatibilityRecord{
		ID: "DAH-002/literal-schema-metacharacters", Operation: "get_session",
		LiveTool: "agm_get_session_metadata", Dimension: "constraints",
		RegistryPath: "/identifier", LivePath: "/identifier",
		RegistryValue: contractAbsent,
		LiveValue:     `{"$ref":"schema.json?mode=strict","pattern":"^x.*$"}`,
	}
	if err := validateCompatibilityRecords([]compatibilityRecord{record}); err != nil {
		t.Fatalf("exact schema metacharacters rejected: %v", err)
	}
	observed := record
	observed.ID = ""
	if err := consumeCompatibilityRecords([]compatibilityRecord{observed}, []compatibilityRecord{record}); err != nil {
		t.Fatalf("exact schema metacharacters not consumed literally: %v", err)
	}

	selector := record
	selector.ID = "DAH-002/non-matching-selector"
	selector.Operation = "*"
	if err := validateCompatibilityRecords([]compatibilityRecord{selector}); err == nil {
		t.Fatal("wildcard operation selector passed compatibility-record validation")
	}
	if err := consumeCompatibilityRecords([]compatibilityRecord{observed}, []compatibilityRecord{selector}); err == nil {
		t.Fatal("literal wildcard selector unexpectedly matched an exact operation")
	}
}
