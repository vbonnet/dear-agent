package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/surface"
	"github.com/vbonnet/dear-agent/pkg/codegen"
)

const contractAbsent = "<absent>"

type contractSchemaNode struct {
	Path                 string
	Type                 string
	ItemType             string
	Description          string
	Enum                 string
	EnumPresent          bool
	AdditionalProperties string
	Required             bool
}

type contractTool struct {
	Operation   string
	Name        string
	Description string
	Nodes       map[string]contractSchemaNode
}

type compatibilityRecord struct {
	ID            string
	Operation     string
	LiveTool      string
	Dimension     string
	RegistryPath  string
	LivePath      string
	RegistryValue string
	LiveValue     string
}

func TestMCPCompiledSurfaceMatchesFiniteCompatibilityContract(t *testing.T) {
	tools := registeredMCPTools(t, registerMCPTools)
	wantNames := []string{
		"agm_archive_session",
		"agm_create_session",
		"agm_get_session_metadata",
		"agm_kill_session",
		"agm_list_ops",
		"agm_list_sessions",
		"agm_search_sessions",
		"agm_send_message",
		"engram_get_wayfinder_session",
		"engram_list_wayfinder_sessions",
	}
	gotNames := make([]string, 0, len(tools))
	for _, tool := range tools {
		gotNames = append(gotNames, tool.Name)
	}
	sort.Strings(gotNames)
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("compiled tools = %v, want exact ten-tool set %v", gotNames, wantNames)
	}

	logical := logicalRegistryContract(t)
	live := compiledContract(t, tools)
	if err := compareCompatibility(logical, live, currentCompatibilityRecords()); err != nil {
		t.Fatal(err)
	}
	if err := compareExactExternalToolContracts(live); err != nil {
		t.Fatal(err)
	}
}

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
				node.EnumPresent = true
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

func TestSchemaCanonicalizationPreservesAbsentAndEmptyEnum(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "absent", raw: `{"type":"string"}`, want: contractAbsent},
		{name: "explicit empty", raw: `{"type":"string","enum":[]}`, want: "[]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal([]byte(tt.raw), &schema); err != nil {
				t.Fatalf("decode raw schema fixture: %v", err)
			}
			nodes := make(map[string]contractSchemaNode)
			canonicalizeSchema(t, "/", schema, false, nodes)
			if got := enumValue(nodes["/"]); got != tt.want {
				t.Fatalf("canonical enum = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSchemaRequiredMembersMustBeDeclaredAndUnique(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantKey string
	}{
		{
			name:    "undeclared member",
			raw:     `{"type":"object","properties":{"known":{"type":"string"}},"required":["ghost"]}`,
			wantKey: "DAH-002/schema-required-undeclared",
		},
		{
			name:    "duplicate member",
			raw:     `{"type":"object","properties":{"known":{"type":"string"}},"required":["known","known"]}`,
			wantKey: "DAH-002/schema-required-duplicate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal([]byte(tt.raw), &schema); err != nil {
				t.Fatalf("decode raw schema fixture: %v", err)
			}
			_, err := schemaRequiredFields(schema)
			if err == nil || !strings.Contains(err.Error(), tt.wantKey) {
				t.Fatalf("schemaRequiredFields() error = %v, want stable key %q", err, tt.wantKey)
			}
		})
	}
}

func registeredMCPTools(t *testing.T, register func(*mcp.Server, *Config)) []*mcp.Tool {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "surface-contract-test", Version: "test"}, nil)
	register(server, &Config{})
	client := mcp.NewClient(&mcp.Implementation{Name: "surface-contract-client", Version: "test"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	result, err := clientSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	return result.Tools
}

func logicalRegistryContract(t *testing.T) map[string]contractTool {
	t.Helper()
	requestTypes := map[string]reflect.Type{
		"ListSessionsRequest":   reflect.TypeFor[surface.ListSessionsRequest](),
		"GetSessionRequest":     reflect.TypeFor[surface.GetSessionRequest](),
		"SearchSessionsRequest": reflect.TypeFor[surface.SearchSessionsRequest](),
		"GetStatusRequest":      reflect.TypeFor[surface.GetStatusRequest](),
		"ArchiveSessionRequest": reflect.TypeFor[surface.ArchiveSessionRequest](),
		"KillSessionRequest":    reflect.TypeFor[surface.KillSessionRequest](),
		"ListOpsRequest":        reflect.TypeFor[surface.ListOpsRequest](),
	}
	usedRequestTypes := make(map[string]bool, len(requestTypes))
	for _, operation := range surface.Registry {
		if operation.MCP == nil {
			continue
		}
		if _, ok := requestTypes[operation.RequestType]; !ok {
			t.Fatalf("DAH-002/unknown-request-type: operation %q references %q",
				operation.Name, operation.RequestType)
		}
		usedRequestTypes[operation.RequestType] = true
	}
	if len(usedRequestTypes) != len(requestTypes) {
		t.Fatalf("DAH-002/stale-request-type-map: used %v, mapped %v",
			sortedKeys(usedRequestTypes), sortedKeys(requestTypes))
	}

	irs, err := codegen.BuildIRs(surface.Registry, requestTypes, nil)
	if err != nil {
		t.Fatalf("codegen.BuildIRs: %v", err)
	}
	result := make(map[string]contractTool, len(irs))
	for _, ir := range irs {
		if ir.MCP() == nil {
			continue
		}
		tool := contractTool{
			Operation:   ir.OpName(),
			Name:        ir.MCP().ToolName,
			Description: ir.MCPDescription,
			Nodes: map[string]contractSchemaNode{
				"/": {Path: "/", Type: "object", AdditionalProperties: "false"},
			},
		}
		for _, field := range ir.MCPFields() {
			path := "/" + escapeJSONPointer(field.JSONName)
			typeName, itemType := logicalJSONType(field.GoType)
			values := slices.Clone(field.Enum)
			sort.Strings(values)
			tool.Nodes[path] = contractSchemaNode{
				Path: path, Type: typeName, ItemType: itemType, Description: field.Description,
				Enum: strings.Join(values, ","), EnumPresent: field.Enum != nil, Required: field.Required,
				AdditionalProperties: contractAbsent,
			}
		}
		if _, duplicate := result[tool.Operation]; duplicate {
			t.Fatalf("DAH-002/duplicate-registry-operation: %q", tool.Operation)
		}
		result[tool.Operation] = tool
	}
	return result
}

func compiledContract(t *testing.T, tools []*mcp.Tool) map[string]contractTool {
	t.Helper()
	result := make(map[string]contractTool, len(tools))
	for _, tool := range tools {
		if _, duplicate := result[tool.Name]; duplicate {
			t.Fatalf("DAH-002/duplicate-compiled-tool: %q", tool.Name)
		}
		data, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal schema for %s: %v", tool.Name, err)
		}
		var schema map[string]any
		if err := json.Unmarshal(data, &schema); err != nil {
			t.Fatalf("decode schema for %s: %v", tool.Name, err)
		}
		nodes := make(map[string]contractSchemaNode)
		canonicalizeSchema(t, "/", schema, false, nodes)
		result[tool.Name] = contractTool{Name: tool.Name, Description: tool.Description, Nodes: nodes}
	}
	return result
}

func canonicalizeSchema(
	t *testing.T,
	path string,
	schema map[string]any,
	required bool,
	nodes map[string]contractSchemaNode,
) {
	t.Helper()
	enum, enumPresent := schemaEnum(t, schema)
	node := contractSchemaNode{
		Path: path, Type: canonicalType(t, schema["type"]), Description: stringValue(schema["description"]),
		Enum: enum, EnumPresent: enumPresent, Required: required,
		AdditionalProperties: contractAbsent,
	}
	if items, ok := schema["items"].(map[string]any); ok {
		node.ItemType = canonicalType(t, items["type"])
	}
	if value, ok := schema["additionalProperties"]; ok {
		node.AdditionalProperties = fmt.Sprint(value)
	}
	nodes[path] = node
	properties, _ := schema["properties"].(map[string]any)
	requiredFields, err := schemaRequiredFields(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range sortedKeys(properties) {
		child, ok := properties[name].(map[string]any)
		if !ok {
			t.Fatalf("schema property %s/%s = %T, want object", path, name, properties[name])
		}
		childPath := "/" + escapeJSONPointer(name)
		if path != "/" {
			childPath = path + "/" + escapeJSONPointer(name)
		}
		canonicalizeSchema(t, childPath, child, requiredFields[name], nodes)
	}
}

func schemaRequiredFields(schema map[string]any) (map[string]bool, error) {
	requiredFields := make(map[string]bool)
	raw, ok := schema["required"]
	if !ok || raw == nil {
		return requiredFields, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("DAH-002/schema-required-type: required = %T, want array", raw)
	}
	properties, _ := schema["properties"].(map[string]any)
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("DAH-002/schema-required-item-type: required member = %T, want string", item)
		}
		if requiredFields[name] {
			return nil, fmt.Errorf("DAH-002/schema-required-duplicate: %q", name)
		}
		if _, declared := properties[name]; !declared {
			return nil, fmt.Errorf("DAH-002/schema-required-undeclared: %q", name)
		}
		requiredFields[name] = true
	}
	return requiredFields, nil
}

func compareCompatibility(
	logical map[string]contractTool,
	live map[string]contractTool,
	records []compatibilityRecord,
) error {
	if err := validateCompatibilityRecords(records); err != nil {
		return err
	}
	observed := make([]compatibilityRecord, 0, len(records))
	usedLiveTools := make(map[string]bool)

	for _, operation := range sortedKeys(logical) {
		registryTool := logical[operation]
		liveTool, ok := live[registryTool.Name]
		if ok {
			usedLiveTools[registryTool.Name] = true
		} else if rename, found := findRecord(records, operation, "tool_rename", registryTool.Name); found {
			liveTool, ok = live[rename.LiveValue]
			if ok {
				usedLiveTools[liveTool.Name] = true
				observed = append(observed, compatibilityRecord{
					Operation: operation, LiveTool: liveTool.Name, Dimension: "tool_rename",
					RegistryValue: registryTool.Name, LiveValue: liveTool.Name,
				})
			}
		}
		if !ok {
			observed = append(observed, compatibilityRecord{
				Operation: operation, LiveTool: contractAbsent, Dimension: "operation_omission",
				RegistryValue: registryTool.Name, LiveValue: contractAbsent,
			})
			continue
		}
		if registryTool.Description != liveTool.Description {
			observed = append(observed, semanticDifference(
				operation, liveTool.Name, "tool_description", "<tool>", "<tool>",
				registryTool.Description, liveTool.Description,
			))
		}
		observed = append(observed, compareToolSchemas(operation, registryTool, liveTool, records)...)
	}

	logicalNames := compiledLogicalNames()
	for _, name := range sortedKeys(live) {
		if usedLiveTools[name] {
			continue
		}
		operation := logicalNames[name]
		if operation == "" {
			operation = contractAbsent
		}
		observed = append(observed, compatibilityRecord{
			Operation: operation, LiveTool: name, Dimension: "tool_addition",
			RegistryValue: contractAbsent, LiveValue: name,
		})
	}
	return consumeCompatibilityRecords(observed, records)
}

func compareToolSchemas(
	operation string,
	registryTool contractTool,
	liveTool contractTool,
	records []compatibilityRecord,
) []compatibilityRecord {
	var observed []compatibilityRecord
	usedLivePaths := make(map[string]bool)
	for _, registryPath := range sortedKeys(registryTool.Nodes) {
		registryNode := registryTool.Nodes[registryPath]
		livePath := registryPath
		liveNode, ok := liveTool.Nodes[livePath]
		if !ok {
			if move, found := findPathMove(records, operation, registryPath); found {
				livePath = move.LivePath
				liveNode, ok = liveTool.Nodes[livePath]
				if ok {
					observed = append(observed, compatibilityRecord{
						Operation: operation, LiveTool: liveTool.Name, Dimension: "path_move",
						RegistryPath: registryPath, LivePath: livePath,
					})
				}
			}
		}
		if !ok {
			observed = append(observed, compatibilityRecord{
				Operation: operation, LiveTool: liveTool.Name, Dimension: "path_omission",
				RegistryPath: registryPath, LivePath: contractAbsent,
				RegistryValue: pathShape(registryNode), LiveValue: contractAbsent,
			})
			observed = append(observed, semanticDifference(
				operation, liveTool.Name, "requiredness", registryPath, contractAbsent,
				requiredValue(registryNode.Required), contractAbsent,
			))
			continue
		}
		usedLivePaths[livePath] = true
		observed = append(observed,
			compareNodes(operation, liveTool.Name, registryPath, livePath, registryNode, liveNode)...)
	}
	for _, livePath := range sortedKeys(liveTool.Nodes) {
		if usedLivePaths[livePath] {
			continue
		}
		liveNode := liveTool.Nodes[livePath]
		observed = append(observed, compatibilityRecord{
			Operation: operation, LiveTool: liveTool.Name, Dimension: "path_addition",
			RegistryPath: contractAbsent, LivePath: livePath,
			RegistryValue: contractAbsent, LiveValue: pathShape(liveNode),
		})
		if liveNode.Required {
			observed = append(observed, semanticDifference(
				operation, liveTool.Name, "requiredness", livePath, livePath, "optional", "required",
			))
		}
	}
	return observed
}

func compareNodes(
	operation string,
	liveTool string,
	registryPath string,
	livePath string,
	registry contractSchemaNode,
	live contractSchemaNode,
) []compatibilityRecord {
	var result []compatibilityRecord
	differences := []struct {
		dimension string
		registry  string
		live      string
	}{
		{"type", registry.Type, live.Type},
		{"item_type", registry.ItemType, live.ItemType},
		{"requiredness", requiredValue(registry.Required), requiredValue(live.Required)},
		{"enum", enumValue(registry), enumValue(live)},
		{"description", registry.Description, live.Description},
		{"closed_object", registry.AdditionalProperties, live.AdditionalProperties},
	}
	for _, difference := range differences {
		if difference.registry == difference.live {
			continue
		}
		result = append(result, semanticDifference(
			operation, liveTool, difference.dimension, registryPath, livePath,
			difference.registry, difference.live,
		))
	}
	return result
}

func semanticDifference(
	operation string,
	liveTool string,
	dimension string,
	registryPath string,
	livePath string,
	registryValue string,
	liveValue string,
) compatibilityRecord {
	return compatibilityRecord{
		Operation: operation, LiveTool: liveTool, Dimension: dimension,
		RegistryPath: registryPath, LivePath: livePath,
		RegistryValue: registryValue, LiveValue: liveValue,
	}
}

func consumeCompatibilityRecords(observed, expected []compatibilityRecord) error {
	expectedBySignature := make(map[string]compatibilityRecord, len(expected))
	for _, record := range expected {
		signature := compatibilitySignature(record)
		if previous, duplicate := expectedBySignature[signature]; duplicate {
			return fmt.Errorf("%s: duplicates %s", record.ID, previous.ID)
		}
		expectedBySignature[signature] = record
	}
	consumed := make(map[string]bool, len(expected))
	for _, difference := range observed {
		signature := compatibilitySignature(difference)
		record, ok := expectedBySignature[signature]
		if !ok {
			key := "DAH-002/unaccounted-difference"
			switch difference.Dimension {
			case "tool_addition":
				key = "DAH-002/unaccounted-live-tool"
			case "operation_omission":
				key = "DAH-002/unaccounted-operation-omission"
			case "requiredness":
				key = "DAH-002/unaccounted-requiredness"
			case "enum":
				key = "DAH-002/unaccounted-enum"
			}
			return fmt.Errorf("%s: %s", key, describeCompatibility(difference))
		}
		if consumed[record.ID] {
			return fmt.Errorf("%s: compatibility record consumed more than once", record.ID)
		}
		consumed[record.ID] = true
	}
	ids := make([]string, 0, len(expected))
	byID := make(map[string]compatibilityRecord, len(expected))
	for _, record := range expected {
		ids = append(ids, record.ID)
		byID[record.ID] = record
	}
	sort.Strings(ids)
	for _, id := range ids {
		if !consumed[id] {
			return fmt.Errorf("%s: stale compatibility record: %s", id, describeCompatibility(byID[id]))
		}
	}
	return nil
}

func validateCompatibilityRecords(records []compatibilityRecord) error {
	validDimensions := map[string]bool{
		"tool_rename": true, "operation_omission": true, "tool_addition": true,
		"path_move": true, "path_omission": true, "path_addition": true,
		"type": true, "item_type": true, "requiredness": true, "enum": true,
		"description": true, "closed_object": true, "tool_description": true,
	}
	seenIDs := make(map[string]bool, len(records))
	for _, record := range records {
		if !strings.HasPrefix(record.ID, "DAH-002/") || record.Operation == "" ||
			record.Dimension == "" || !validDimensions[record.Dimension] {
			return fmt.Errorf("DAH-002/invalid-compatibility-record: %+v", record)
		}
		if seenIDs[record.ID] {
			return fmt.Errorf("%s: duplicate compatibility identifier", record.ID)
		}
		seenIDs[record.ID] = true
		joined := strings.Join([]string{
			record.Operation, record.LiveTool, record.RegistryPath, record.LivePath,
			record.RegistryValue, record.LiveValue,
		}, "\n")
		if strings.ContainsAny(joined, "*?") {
			return fmt.Errorf("%s: wildcard and prefix-style records are forbidden", record.ID)
		}
		if strings.HasPrefix(record.Dimension, "path_") ||
			(!strings.HasPrefix(record.Dimension, "tool_") && record.Dimension != "operation_omission") {
			if record.RegistryPath == "" || record.LivePath == "" {
				return fmt.Errorf("%s: path-based record has an empty path", record.ID)
			}
		}
	}
	return nil
}

func currentCompatibilityRecords() []compatibilityRecord {
	return []compatibilityRecord{
		toolRule("DAH-002/get-session-tool-rename", "get_session", "agm_get_session_metadata",
			"tool_rename", "agm_get_session", "agm_get_session_metadata"),
		toolRule("DAH-002/get-status-operation-omission", "get_status", contractAbsent,
			"operation_omission", "agm_get_status", contractAbsent),
		toolRule("DAH-002/create-session-extra-tool", "create_session", "agm_create_session",
			"tool_addition", contractAbsent, "agm_create_session"),
		toolRule("DAH-002/send-message-extra-tool", "send_message", "agm_send_message",
			"tool_addition", contractAbsent, "agm_send_message"),
		toolRule("DAH-002/list-wayfinder-extra-tool", "list_wayfinder_sessions",
			"engram_list_wayfinder_sessions", "tool_addition", contractAbsent, "engram_list_wayfinder_sessions"),
		toolRule("DAH-002/get-wayfinder-extra-tool", "get_wayfinder_session",
			"engram_get_wayfinder_session", "tool_addition", contractAbsent, "engram_get_wayfinder_session"),

		pathRule("DAH-002/list-sessions-status-move", "list_sessions", "agm_list_sessions",
			"path_move", "/status", "/filters/status", "", ""),
		pathRule("DAH-002/list-sessions-harness-move", "list_sessions", "agm_list_sessions",
			"path_move", "/harness", "/filters/agent_type", "", ""),
		pathRule("DAH-002/list-sessions-limit-move", "list_sessions", "agm_list_sessions",
			"path_move", "/limit", "/filters/limit", "", ""),
		pathRule("DAH-002/list-sessions-offset-omission", "list_sessions", "agm_list_sessions",
			"path_omission", "/offset", contractAbsent,
			pathShape(contractSchemaNode{Type: "integer", Description: "Pagination offset",
				AdditionalProperties: contractAbsent}), contractAbsent),
		pathRule("DAH-002/list-sessions-offset-requiredness", "list_sessions", "agm_list_sessions",
			"requiredness", "/offset", contractAbsent, "optional", contractAbsent),
		pathRule("DAH-002/list-sessions-filters-addition", "list_sessions", "agm_list_sessions",
			"path_addition", contractAbsent, "/filters", contractAbsent,
			pathShape(contractSchemaNode{Type: "object", AdditionalProperties: "false"})),
		pathRule("DAH-002/list-sessions-filters-required", "list_sessions", "agm_list_sessions",
			"requiredness", "/filters", "/filters", "optional", "required"),
		pathRule("DAH-002/list-sessions-fields-addition", "list_sessions", "agm_list_sessions",
			"path_addition", contractAbsent, "/fields", contractAbsent,
			pathShape(contractSchemaNode{Type: "array|null", ItemType: "string",
				Description:          "Field mask: only return these fields (e.g. [id, name, status]). Omit for all fields.",
				AdditionalProperties: contractAbsent})),
		pathRule("DAH-002/list-sessions-status-enum", "list_sessions", "agm_list_sessions",
			"enum", "/status", "/filters/status", enumList("active", "archived", "all"), contractAbsent),
		pathRule("DAH-002/list-sessions-harness-enum", "list_sessions", "agm_list_sessions",
			"enum", "/harness", "/filters/agent_type",
			enumList("claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli", "gemini-cli", "all"), contractAbsent),
		pathRule("DAH-002/list-sessions-status-description", "list_sessions", "agm_list_sessions",
			"description", "/status", "/filters/status", "Filter by session status",
			"Filter by status: active (default), archived, or all"),
		pathRule("DAH-002/list-sessions-harness-description", "list_sessions", "agm_list_sessions",
			"description", "/harness", "/filters/agent_type", "Filter by harness; gemini-cli is deprecated",
			"Filter by harness: claude-code, codex-cli, agy, opencode-cli, pi-cli, gemini-cli, or all"),
		pathRule("DAH-002/list-sessions-limit-description", "list_sessions", "agm_list_sessions",
			"description", "/limit", "/filters/limit", "Maximum sessions to return (1-1000)",
			"Maximum sessions to return (1-1000, default 100)"),

		pathRule("DAH-002/search-sessions-status-move", "search_sessions", "agm_search_sessions",
			"path_move", "/status", "/filters/status", "", ""),
		pathRule("DAH-002/search-sessions-limit-move", "search_sessions", "agm_search_sessions",
			"path_move", "/limit", "/filters/limit", "", ""),
		pathRule("DAH-002/search-sessions-filters-addition", "search_sessions", "agm_search_sessions",
			"path_addition", contractAbsent, "/filters", contractAbsent,
			pathShape(contractSchemaNode{Type: "object", AdditionalProperties: "false"})),
		pathRule("DAH-002/search-sessions-filters-required", "search_sessions", "agm_search_sessions",
			"requiredness", "/filters", "/filters", "optional", "required"),
		pathRule("DAH-002/search-sessions-status-enum", "search_sessions", "agm_search_sessions",
			"enum", "/status", "/filters/status", enumList("active", "archived", "all"), contractAbsent),
		pathRule("DAH-002/search-sessions-status-description", "search_sessions", "agm_search_sessions",
			"description", "/status", "/filters/status", "Filter by session status",
			"Filter by status: active (default), archived, or all"),
		pathRule("DAH-002/search-sessions-limit-description", "search_sessions", "agm_search_sessions",
			"description", "/limit", "/filters/limit", "Maximum results to return (1-50)",
			"Maximum results (1-50, default 10)"),

		pathRule("DAH-002/archive-dry-run-addition", "archive_session", "agm_archive_session",
			"path_addition", contractAbsent, "/dry_run", contractAbsent,
			pathShape(contractSchemaNode{Type: "boolean",
				Description:          "Preview the archive without executing. Returns what would happen.",
				AdditionalProperties: contractAbsent})),
		pathRule("DAH-002/archive-identifier-description", "archive_session", "agm_archive_session",
			"description", "/identifier", "/identifier", "Session ID, name, or UUID prefix",
			"Session ID, name, or tmux session name to archive"),

		pathRule("DAH-002/kill-dry-run-addition", "kill_session", "agm_kill_session",
			"path_addition", contractAbsent, "/dry_run", contractAbsent,
			pathShape(contractSchemaNode{Type: "boolean",
				Description:          "Preview the kill without executing. Returns what would happen.",
				AdditionalProperties: contractAbsent})),
		pathRule("DAH-002/kill-identifier-description", "kill_session", "agm_kill_session",
			"description", "/identifier", "/identifier", "Session ID, name, or UUID prefix",
			"Session ID, name, or tmux session name to kill"),
		pathRule("DAH-002/kill-force-description", "kill_session", "agm_kill_session",
			"description", "/force", "/force", "Bypass the recent-activity safety check",
			"Bypass the recent-activity safety check."),
		pathRule("DAH-002/kill-confirmed-stuck-description", "kill_session", "agm_kill_session",
			"description", "/confirmed_stuck", "/confirmed_stuck",
			"Confirm that a live harness is stuck and may be killed",
			"Confirm that a live harness is stuck and may be killed. Required for an active session."),
	}
}

func toolRule(id, operation, liveTool, dimension, registryValue, liveValue string) compatibilityRecord {
	return compatibilityRecord{
		ID: id, Operation: operation, LiveTool: liveTool, Dimension: dimension,
		RegistryValue: registryValue, LiveValue: liveValue,
	}
}

func pathRule(
	id string,
	operation string,
	liveTool string,
	dimension string,
	registryPath string,
	livePath string,
	registryValue string,
	liveValue string,
) compatibilityRecord {
	return compatibilityRecord{
		ID: id, Operation: operation, LiveTool: liveTool, Dimension: dimension,
		RegistryPath: registryPath, LivePath: livePath,
		RegistryValue: registryValue, LiveValue: liveValue,
	}
}

func findRecord(
	records []compatibilityRecord,
	operation string,
	dimension string,
	registryValue string,
) (compatibilityRecord, bool) {
	for _, record := range records {
		if record.Operation == operation && record.Dimension == dimension &&
			record.RegistryValue == registryValue {
			return record, true
		}
	}
	return compatibilityRecord{}, false
}

func findPathMove(records []compatibilityRecord, operation, registryPath string) (compatibilityRecord, bool) {
	for _, record := range records {
		if record.Operation == operation && record.Dimension == "path_move" &&
			record.RegistryPath == registryPath {
			return record, true
		}
	}
	return compatibilityRecord{}, false
}

func compiledLogicalNames() map[string]string {
	return map[string]string{
		"agm_list_sessions":              "list_sessions",
		"agm_search_sessions":            "search_sessions",
		"agm_get_session_metadata":       "get_session",
		"agm_archive_session":            "archive_session",
		"agm_kill_session":               "kill_session",
		"agm_create_session":             "create_session",
		"agm_send_message":               "send_message",
		"agm_list_ops":                   "list_ops",
		"engram_list_wayfinder_sessions": "list_wayfinder_sessions",
		"engram_get_wayfinder_session":   "get_wayfinder_session",
	}
}

func cloneContract(input map[string]contractTool) map[string]contractTool {
	result := make(map[string]contractTool, len(input))
	for name, tool := range input {
		cloned := tool
		cloned.Nodes = make(map[string]contractSchemaNode, len(tool.Nodes))
		maps.Copy(cloned.Nodes, tool.Nodes)
		result[name] = cloned
	}
	return result
}

func compatibilitySignature(record compatibilityRecord) string {
	return fmt.Sprintf("%q|%q|%q|%q|%q|%q|%q",
		record.Operation, record.LiveTool, record.Dimension, record.RegistryPath,
		record.LivePath, record.RegistryValue, record.LiveValue)
}

func describeCompatibility(record compatibilityRecord) string {
	return fmt.Sprintf("operation=%q live_tool=%q dimension=%q registry=%q:%q live=%q:%q",
		record.Operation, record.LiveTool, record.Dimension,
		record.RegistryPath, record.RegistryValue, record.LivePath, record.LiveValue)
}

func pathShape(node contractSchemaNode) string {
	return fmt.Sprintf("type=%q,item=%q,description=%q,enum=%q,additionalProperties=%q",
		node.Type, node.ItemType, node.Description, enumValue(node), node.AdditionalProperties)
}

func logicalJSONType(goType string) (string, string) {
	switch goType {
	case "string":
		return "string", ""
	case "int", "int64", "float64":
		return "integer", ""
	case "bool":
		return "boolean", ""
	case "[]string":
		return "array", "string"
	default:
		return goType, ""
	}
}

func canonicalType(t *testing.T, value any) string {
	t.Helper()
	switch value := value.(type) {
	case nil:
		return ""
	case string:
		return value
	case []any:
		values := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				t.Fatalf("schema type member = %T, want string", item)
			}
			values = append(values, text)
		}
		sort.Strings(values)
		return strings.Join(values, "|")
	default:
		t.Fatalf("schema type = %T, want string or string array", value)
		return ""
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func schemaEnum(t *testing.T, schema map[string]any) (string, bool) {
	t.Helper()
	value, present := schema["enum"]
	if !present {
		return "", false
	}
	if value == nil {
		t.Fatal("DAH-002/schema-enum-null: enum must be a string array")
	}
	return strings.Join(stringSlice(t, value), ","), true
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	if value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("schema string array = %T, want []any", value)
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("schema string-array item = %T, want string", item)
		}
		result = append(result, text)
	}
	sort.Strings(result)
	return result
}

func enumList(values ...string) string {
	values = slices.Clone(values)
	sort.Strings(values)
	return "[" + strings.Join(values, ",") + "]"
}

func enumValue(node contractSchemaNode) string {
	if !node.EnumPresent {
		return contractAbsent
	}
	return "[" + node.Enum + "]"
}

func requiredValue(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func sortedKeys[M ~map[string]V, V any](input M) []string {
	result := make([]string, 0, len(input))
	for key := range input {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
