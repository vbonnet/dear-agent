// Package mcp provides MCP server implementation.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/vbonnet/dear-agent/tools/schema-registry/internal/query"
	"github.com/vbonnet/dear-agent/tools/schema-registry/internal/registry"
	"github.com/vbonnet/dear-agent/tools/schema-registry/internal/schema"
)

// Server implements the MCP protocol
type Server struct {
	workspace string
	verbose   bool
	registry  *registry.Registry
}

// NewServer creates a new MCP server
func NewServer(workspace string, verbose bool) *Server {
	return &Server{
		workspace: workspace,
		verbose:   verbose,
	}
}

// HandleRequest handles a JSON-RPC request
func (s *Server) HandleRequest(request map[string]interface{}) map[string]interface{} {
	// Extract method and id
	method, ok := request["method"].(string)
	if !ok {
		return s.errorResponse(request["id"], -32600, "Invalid Request", "method is required")
	}

	// Handle different methods
	switch method {
	case "initialize":
		return s.handleInitialize(request)
	case "tools/list":
		return s.handleToolsList(request)
	case "tools/call":
		return s.handleToolsCall(request)
	default:
		return s.errorResponse(request["id"], -32601, "Method not found", nil)
	}
}

// handleInitialize handles the initialize handshake
func (s *Server) handleInitialize(request map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"result": map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{
					"listChanged": false,
				},
			},
			"serverInfo": map[string]interface{}{
				"name":    "corpus-callosum-mcp-server",
				"version": "1.0.0",
			},
		},
	}
}

// handleToolsList handles tools/list request
func (s *Server) handleToolsList(request map[string]interface{}) map[string]interface{} {
	tools := []map[string]interface{}{
		{
			"name":        "cc__discoverComponents",
			"description": "Discover all registered components in the Corpus Callosum registry. Returns component list with metadata, schemas, and versions.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Filter by specific component name",
					},
				},
			},
		},
		{
			"name":        "cc__getComponentSchema",
			"description": "Retrieve schema definition for a specific component. Returns JSON Schema with type definitions and validation rules.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component": map[string]interface{}{
						"type":        "string",
						"description": "Component identifier (e.g., 'agm', 'wayfinder')",
					},
					"version": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific version (defaults to latest)",
					},
					"schemaName": map[string]interface{}{
						"type":        "string",
						"description": "Optional: Specific schema within component (e.g., 'session')",
					},
				},
				"required": []string{"component"},
			},
		},
		{
			"name":        "cc__queryData",
			"description": "Query data from component storage. Supports filtering, sorting, and pagination. Note: Requires component integration.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component": map[string]interface{}{
						"type":        "string",
						"description": "Component identifier",
					},
					"schema": map[string]interface{}{
						"type":        "string",
						"description": "Schema name (e.g., 'session', 'task')",
					},
					"filter": map[string]interface{}{
						"type":        "object",
						"description": "Filter criteria (JSON object)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum results (default: 100)",
						"minimum":     1,
						"maximum":     1000,
					},
				},
				"required": []string{"component", "schema"},
			},
		},
		{
			"name":        "cc__registerSchema",
			"description": "Register or update component schema. Performs compatibility checking before registration.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component": map[string]interface{}{
						"type":        "string",
						"description": "Component identifier",
					},
					"schema": map[string]interface{}{
						"type":        "object",
						"description": "Complete schema definition (JSON Schema format)",
					},
					"force": map[string]interface{}{
						"type":        "boolean",
						"description": "Skip compatibility check (use with caution)",
						"default":     false,
					},
				},
				"required": []string{"component", "schema"},
			},
		},
		{
			"name":        "cc__validateData",
			"description": "Validate data against component schema. Returns validation result with errors if invalid.",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"component": map[string]interface{}{
						"type":        "string",
						"description": "Component identifier",
					},
					"schema": map[string]interface{}{
						"type":        "string",
						"description": "Schema name",
					},
					"data": map[string]interface{}{
						"type":        "object",
						"description": "Data to validate",
					},
				},
				"required": []string{"component", "schema", "data"},
			},
		},
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      request["id"],
		"result": map[string]interface{}{
			"tools": tools,
		},
	}
}

// extractTraceContext extracts W3C trace context from MCP _meta field.
func extractTraceContext(ctx context.Context, meta map[string]interface{}) context.Context {
	if tp, ok := meta["traceparent"].(string); ok {
		carrier := propagation.MapCarrier{"traceparent": tp}
		return otel.GetTextMapPropagator().Extract(ctx, carrier)
	}
	return ctx
}

// handleToolsCall handles tools/call request
func (s *Server) handleToolsCall(request map[string]interface{}) map[string]interface{} {
	params, ok := request["params"].(map[string]interface{})
	if !ok {
		return s.errorResponse(request["id"], -32602, "Invalid params", nil)
	}

	toolName, ok := params["name"].(string)
	if !ok {
		return s.errorResponse(request["id"], -32602, "Tool name is required", nil)
	}

	// Extract trace context from _meta if present
	ctx := context.Background()
	if meta, ok := params["_meta"].(map[string]interface{}); ok {
		ctx = extractTraceContext(ctx, meta)
	}

	tracer := otel.Tracer("corpus-callosum")
	ctx, span := tracer.Start(ctx, "mcp.tools/call "+toolName)
	defer span.End()
	_ = ctx // ctx available for downstream propagation

	args, _ := params["arguments"].(map[string]interface{})

	// Initialize registry
	reg, err := registry.New(s.workspace)
	if err != nil {
		return s.errorResponse(request["id"], -32000, fmt.Sprintf("Failed to open registry: %v", err), nil)
	}
	defer reg.Close()
	s.registry = reg

	// Route to tool handlers
	switch toolName {
	case "cc__discoverComponents":
		return s.toolDiscoverComponents(request["id"], args)
	case "cc__getComponentSchema":
		return s.toolGetComponentSchema(request["id"], args)
	case "cc__queryData":
		return s.toolQueryData(request["id"], args)
	case "cc__registerSchema":
		return s.toolRegisterSchema(request["id"], args)
	case "cc__validateData":
		return s.toolValidateData(request["id"], args)
	default:
		return s.errorResponse(request["id"], -32601, "Unknown tool", nil)
	}
}

// Tool implementations

func (s *Server) toolDiscoverComponents(id interface{}, args map[string]interface{}) map[string]interface{} {
	componentFilter, _ := args["component"].(string)

	if componentFilter != "" {
		// Get specific component
		comp, err := s.registry.GetComponent(componentFilter)
		if err != nil {
			return s.errorResponse(id, -32000, "Component not found", map[string]interface{}{
				"error_code": "component_not_found",
			})
		}

		versions, err := s.registry.ListVersions(componentFilter)
		if err != nil {
			versions = nil
		}
		schemas := s.getSchemaNames(componentFilter)

		text := fmt.Sprintf("Component: %s\nDescription: %s\nVersion: %s\nSchemas: %v",
			comp.Component, comp.Description, comp.LatestVersion, schemas)

		versionList := make([]map[string]interface{}, len(versions))
		for i, v := range versions {
			versionList[i] = map[string]interface{}{
				"version":       v.Version,
				"created_at":    v.CreatedAt,
				"compatibility": v.Compatibility,
			}
		}

		data := map[string]interface{}{
			"component":      comp.Component,
			"description":    comp.Description,
			"latest_version": comp.LatestVersion,
			"installed_at":   comp.InstalledAt,
			"schemas":        schemas,
			"versions":       versionList,
		}

		return s.successResponse(id, text, "corpus-callosum://components/"+componentFilter, data)
	}

	// List all components
	components, err := s.registry.ListComponents()
	if err != nil {
		return s.errorResponse(id, -32000, "Failed to list components", nil)
	}

	text := fmt.Sprintf("Found %d registered components", len(components))
	if len(components) > 0 {
		text += ":"
		for _, comp := range components {
			text += fmt.Sprintf("\n- %s (v%s): %s", comp.Component, comp.LatestVersion, comp.Description)
		}
	}

	compList := make([]map[string]interface{}, len(components))
	for i, comp := range components {
		schemas := s.getSchemaNames(comp.Component)
		compList[i] = map[string]interface{}{
			"component":      comp.Component,
			"description":    comp.Description,
			"latest_version": comp.LatestVersion,
			"installed_at":   comp.InstalledAt,
			"schemas":        schemas,
		}
	}

	return s.successResponse(id, text, "corpus-callosum://components", map[string]interface{}{
		"components": compList,
	})
}

func (s *Server) toolGetComponentSchema(id interface{}, args map[string]interface{}) map[string]interface{} {
	component, ok := args["component"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "component is required", nil)
	}

	version, _ := args["version"].(string)
	schemaName, _ := args["schemaName"].(string)

	schema, err := s.registry.GetSchema(component, version)
	if err != nil {
		return s.errorResponse(id, -32000, "Schema not found", map[string]interface{}{
			"error_code": "schema_not_found",
		})
	}

	var schemaJSON map[string]interface{}
	if err := json.Unmarshal([]byte(schema.SchemaJSON), &schemaJSON); err != nil {
		return s.errorResponse(id, -32000, "Failed to parse schema", nil)
	}

	if schemaName != "" {
		schemas, ok := schemaJSON["schemas"].(map[string]interface{})
		if !ok {
			return s.errorResponse(id, -32000, "Schemas field not found", nil)
		}

		specificSchema, ok := schemas[schemaName]
		if !ok {
			return s.errorResponse(id, -32000, fmt.Sprintf("Schema '%s' not found", schemaName), nil)
		}

		text := fmt.Sprintf("Schema for %s.%s (v%s)", component, schemaName, schema.Version)

		return s.successResponse(id, text, fmt.Sprintf("corpus-callosum://schemas/%s/%s", component, schemaName),
			map[string]interface{}{
				"component":   component,
				"version":     schema.Version,
				"schema_name": schemaName,
				"schema":      specificSchema,
			})
	}

	text := fmt.Sprintf("Full schema for %s (v%s)", component, schema.Version)

	return s.successResponse(id, text, fmt.Sprintf("corpus-callosum://schemas/%s", component),
		map[string]interface{}{
			"component": component,
			"version":   schema.Version,
			"schema":    schemaJSON,
		})
}

func (s *Server) toolQueryData(id interface{}, args map[string]interface{}) map[string]interface{} {
	component, ok := args["component"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "component is required", nil)
	}

	schemaName, ok := args["schema"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "schema is required", nil)
	}

	// Get filter, limit, and sort parameters
	filter, _ := args["filter"].(map[string]interface{})
	limit := 100 // default
	if limitArg, ok := args["limit"].(float64); ok {
		limit = int(limitArg)
	}

	var sortConfig *query.SortConfig
	if sortArg, ok := args["sort"].(map[string]interface{}); ok {
		field, _ := sortArg["field"].(string)
		order, _ := sortArg["order"].(string)
		if field != "" {
			sortConfig = &query.SortConfig{
				Field: field,
				Order: order,
			}
		}
	}

	// Get the schema definition to extract discovery patterns
	schemaRec, err := s.registry.GetSchema(component, "")
	if err != nil {
		return s.errorResponse(id, -32000, "Schema not found", map[string]interface{}{
			"error_code": "schema_not_found",
			"details":    err.Error(),
		})
	}

	var schemaJSON map[string]interface{}
	if err := json.Unmarshal([]byte(schemaRec.SchemaJSON), &schemaJSON); err != nil {
		return s.errorResponse(id, -32000, "Failed to parse schema", map[string]interface{}{
			"error_code": "schema_parse_error",
			"details":    err.Error(),
		})
	}

	// Create query engine
	queryEngine, err := query.NewQueryEngine()
	if err != nil {
		return s.errorResponse(id, -32000, "Failed to create query engine", map[string]interface{}{
			"error_code": "query_engine_error",
			"details":    err.Error(),
		})
	}

	// Execute query
	result, err := queryEngine.Query(query.QueryParams{
		Component: component,
		Schema:    schemaName,
		Filter:    filter,
		Limit:     limit,
		Sort:      sortConfig,
	}, schemaJSON)

	if err != nil {
		return s.errorResponse(id, -32000, fmt.Sprintf("Query failed: %v", err), map[string]interface{}{
			"error_code": "query_failed",
			"details":    err.Error(),
		})
	}

	// Format response text
	text := fmt.Sprintf("Found %d %s.%s record(s)", result.Count, component, schemaName)
	if len(filter) > 0 {
		text += " matching filter"
	}

	return s.successResponse(id, text, fmt.Sprintf("corpus-callosum://data/%s/%s", component, schemaName), result)
}

func (s *Server) toolRegisterSchema(id interface{}, args map[string]interface{}) map[string]interface{} {
	component, ok := args["component"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "component is required", nil)
	}

	schemaData, ok := args["schema"].(map[string]interface{})
	if !ok {
		return s.errorResponse(id, -32602, "schema is required", nil)
	}

	force, _ := args["force"].(bool)

	// Validate schema
	if err := schema.ValidateSchema(schemaData); err != nil {
		return s.errorResponse(id, -32000, fmt.Sprintf("Schema validation failed: %v", err), map[string]interface{}{
			"error_code": "schema_invalid",
		})
	}

	version, _ := schemaData["version"].(string)
	compatibility := "backward"
	if c, ok := schemaData["compatibility"].(string); ok {
		compatibility = c
	}

	// Check compatibility
	var previousVersion string
	var compatResult *schema.CompatibilityResult
	if !force {
		oldSchema, err := s.registry.GetSchema(component, "")
		if err == nil {
			previousVersion = oldSchema.Version

			var oldSchemaJSON map[string]interface{}
			if err := json.Unmarshal([]byte(oldSchema.SchemaJSON), &oldSchemaJSON); err == nil {
				compatResult = schema.CheckCompatibility(oldSchemaJSON, schemaData, schema.CompatibilityMode(compatibility))
				if !compatResult.Passed {
					return s.errorResponse(id, -32000, "Compatibility check failed", map[string]interface{}{
						"error_code": "compatibility_violation",
						"violations": compatResult.Violations,
					})
				}
			}
		}
	}

	// Register
	if err := s.registry.RegisterSchema(component, version, compatibility, schemaData); err != nil {
		return s.errorResponse(id, -32000, fmt.Sprintf("Failed to register: %v", err), nil)
	}

	text := fmt.Sprintf("Successfully registered %s v%s", component, version)
	if compatResult != nil && compatResult.Passed {
		text += fmt.Sprintf(". Compatibility check passed (%s mode)", compatResult.Mode)
	}

	result := map[string]interface{}{
		"status":    "registered",
		"component": component,
		"version":   version,
	}
	if previousVersion != "" {
		result["previous_version"] = previousVersion
	}
	if compatResult != nil {
		result["compatibility_check"] = compatResult
	}

	return s.successResponse(id, text, fmt.Sprintf("corpus-callosum://registration/%s", component), result)
}

func (s *Server) toolValidateData(id interface{}, args map[string]interface{}) map[string]interface{} {
	component, ok := args["component"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "component is required", nil)
	}

	schemaName, ok := args["schema"].(string)
	if !ok {
		return s.errorResponse(id, -32602, "schema is required", nil)
	}

	data, ok := args["data"]
	if !ok {
		return s.errorResponse(id, -32602, "data is required", nil)
	}

	// Get schema
	schemaRec, err := s.registry.GetSchema(component, "")
	if err != nil {
		return s.errorResponse(id, -32000, "Schema not found", map[string]interface{}{
			"error_code": "schema_not_found",
		})
	}

	var schemaJSON map[string]interface{}
	if err := json.Unmarshal([]byte(schemaRec.SchemaJSON), &schemaJSON); err != nil {
		return s.errorResponse(id, -32000, "Failed to parse schema", nil)
	}

	schemas, ok := schemaJSON["schemas"].(map[string]interface{})
	if !ok {
		return s.errorResponse(id, -32000, "Schemas field not found", nil)
	}

	specificSchema, ok := schemas[schemaName]
	if !ok {
		return s.errorResponse(id, -32000, fmt.Sprintf("Schema '%s' not found", schemaName), nil)
	}

	// Validate
	if err := schema.ValidateData(specificSchema, data); err != nil {
		text := fmt.Sprintf("Validation failed for %s.%s: %v", component, schemaName, err)
		return s.successResponse(id, text, "corpus-callosum://validation", map[string]interface{}{
			"status":    "invalid",
			"component": component,
			"schema":    schemaName,
			"errors":    []string{err.Error()},
		})
	}

	text := fmt.Sprintf("Data is valid according to %s.%s schema", component, schemaName)

	return s.successResponse(id, text, "corpus-callosum://validation", map[string]interface{}{
		"status":    "valid",
		"component": component,
		"schema":    schemaName,
	})
}

// Helper methods

func (s *Server) successResponse(id interface{}, text string, uri string, data interface{}) map[string]interface{} {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		dataJSON = []byte("{}")
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result": map[string]interface{}{
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": text,
				},
				map[string]interface{}{
					"type": "resource",
					"resource": map[string]interface{}{
						"uri":      uri,
						"mimeType": "application/json",
						"text":     string(dataJSON),
					},
				},
			},
		},
	}
}

func (s *Server) errorResponse(id interface{}, code int, message string, data interface{}) map[string]interface{} {
	errObj := map[string]interface{}{
		"code":    code,
		"message": message,
	}
	if data != nil {
		errObj["data"] = data
	}

	return map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   errObj,
	}
}

func (s *Server) getSchemaNames(component string) []string {
	schema, err := s.registry.GetSchema(component, "")
	if err != nil {
		return []string{}
	}

	var schemaJSON map[string]interface{}
	if err := json.Unmarshal([]byte(schema.SchemaJSON), &schemaJSON); err != nil {
		return []string{}
	}

	schemas, ok := schemaJSON["schemas"].(map[string]interface{})
	if !ok {
		return []string{}
	}

	names := []string{}
	for name := range schemas {
		names = append(names, name)
	}
	return names
}
