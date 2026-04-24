// Package query provides data query engine.
package query

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// QueryEngine executes queries against component data
type QueryEngine struct { //nolint:revive // renaming would break API
	homeDir string
}

// NewQueryEngine creates a new query engine
func NewQueryEngine() (*QueryEngine, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	return &QueryEngine{
		homeDir: homeDir,
	}, nil
}

// QueryResult represents query results
type QueryResult struct { //nolint:revive // renaming would break API
	Component string                   `json:"component"`
	Schema    string                   `json:"schema"`
	Count     int                      `json:"count"`
	Data      []map[string]interface{} `json:"data"`
}

// QueryParams represents query parameters
type QueryParams struct { //nolint:revive // renaming would break API
	Component string
	Schema    string
	Filter    map[string]interface{}
	Limit     int
	Sort      *SortConfig
}

// SortConfig represents sort configuration
type SortConfig struct {
	Field string `json:"field"`
	Order string `json:"order"` // "asc" or "desc"
}

// Query executes a query against component data
func (e *QueryEngine) Query(params QueryParams, schemaData map[string]interface{}) (*QueryResult, error) {
	// Extract schemas map from schema data
	schemas, ok := schemaData["schemas"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("schemas field not found in schema definition")
	}

	// Get specific schema
	schemaSpec, ok := schemas[params.Schema].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("schema '%s' not found", params.Schema)
	}

	// Get discovery patterns
	discoveryPatterns, ok := schemaSpec["discovery_patterns"].([]interface{})
	if !ok || len(discoveryPatterns) == 0 {
		return nil, fmt.Errorf("no discovery_patterns defined for %s.%s", params.Component, params.Schema)
	}

	// Collect data from all discovery patterns
	var allData []map[string]interface{}

	for _, pattern := range discoveryPatterns {
		patternStr, ok := pattern.(string)
		if !ok {
			continue
		}

		// Expand pattern with component path
		expandedPattern := e.expandPattern(params.Component, patternStr)

		// Find matching files
		matches, err := filepath.Glob(expandedPattern)
		if err != nil {
			continue
		}

		// Read and parse each file
		for _, filePath := range matches {
			data, err := e.readFile(filePath)
			if err != nil {
				continue
			}

			// Apply filter
			if params.Filter == nil || e.matchesFilter(data, params.Filter) {
				allData = append(allData, data)
			}
		}
	}

	// Apply sorting
	if params.Sort != nil {
		e.sortData(allData, params.Sort)
	}

	// Apply limit
	if params.Limit > 0 && len(allData) > params.Limit {
		allData = allData[:params.Limit]
	}

	return &QueryResult{
		Component: params.Component,
		Schema:    params.Schema,
		Count:     len(allData),
		Data:      allData,
	}, nil
}

// expandPattern expands a discovery pattern with component path
func (e *QueryEngine) expandPattern(component, pattern string) string {
	// Replace common variables
	pattern = strings.ReplaceAll(pattern, "~", e.homeDir)
	pattern = strings.ReplaceAll(pattern, ".{component}", "."+component)

	// If pattern doesn't start with /, assume it's relative to component dir
	if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "~") {
		componentDir := filepath.Join(e.homeDir, "."+component)
		pattern = filepath.Join(componentDir, pattern)
	}

	return pattern
}

// readFile reads and parses a JSON or YAML file
func (e *QueryEngine) readFile(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}

	// Try JSON first
	if err := json.Unmarshal(data, &result); err == nil {
		return result, nil
	}

	// Try YAML
	if err := yaml.Unmarshal(data, &result); err == nil {
		return result, nil
	}

	return nil, fmt.Errorf("failed to parse file as JSON or YAML")
}

// matchesFilter checks if data matches filter criteria
//
//nolint:gocyclo // filter matching inherently requires multiple type assertions
func (e *QueryEngine) matchesFilter(data map[string]interface{}, filter map[string]interface{}) bool {
	for key, filterValue := range filter {
		// Handle comparison operators
		if strings.HasSuffix(key, "_gt") {
			fieldName := strings.TrimSuffix(key, "_gt")
			dataValue, ok := data[fieldName]
			if !ok {
				return false
			}
			if !e.compareGreaterThan(dataValue, filterValue) {
				return false
			}
			continue
		}

		if strings.HasSuffix(key, "_lt") {
			fieldName := strings.TrimSuffix(key, "_lt")
			dataValue, ok := data[fieldName]
			if !ok {
				return false
			}
			if !e.compareLessThan(dataValue, filterValue) {
				return false
			}
			continue
		}

		if strings.HasSuffix(key, "_gte") {
			fieldName := strings.TrimSuffix(key, "_gte")
			dataValue, ok := data[fieldName]
			if !ok {
				return false
			}
			if e.compareLessThan(dataValue, filterValue) {
				return false
			}
			continue
		}

		if strings.HasSuffix(key, "_lte") {
			fieldName := strings.TrimSuffix(key, "_lte")
			dataValue, ok := data[fieldName]
			if !ok {
				return false
			}
			if e.compareGreaterThan(dataValue, filterValue) {
				return false
			}
			continue
		}

		// Exact match
		dataValue, ok := data[key]
		if !ok || dataValue != filterValue {
			return false
		}
	}

	return true
}

// compareGreaterThan compares two values
func (e *QueryEngine) compareGreaterThan(a, b interface{}) bool {
	// Handle numeric comparisons
	aFloat, aOk := toFloat64(a)
	bFloat, bOk := toFloat64(b)
	if aOk && bOk {
		return aFloat > bFloat
	}

	// Handle string comparisons
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return aStr > bStr
	}

	return false
}

// compareLessThan compares two values
func (e *QueryEngine) compareLessThan(a, b interface{}) bool {
	// Handle numeric comparisons
	aFloat, aOk := toFloat64(a)
	bFloat, bOk := toFloat64(b)
	if aOk && bOk {
		return aFloat < bFloat
	}

	// Handle string comparisons
	aStr, aOk := a.(string)
	bStr, bOk := b.(string)
	if aOk && bOk {
		return aStr < bStr
	}

	return false
}

// toFloat64 converts various numeric types to float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int32:
		return float64(val), true
	case int64:
		return float64(val), true
	case uint:
		return float64(val), true
	case uint32:
		return float64(val), true
	case uint64:
		return float64(val), true
	default:
		return 0, false
	}
}

// sortData sorts data by field
func (e *QueryEngine) sortData(data []map[string]interface{}, sortConfig *SortConfig) {
	sort.Slice(data, func(i, j int) bool {
		valI, okI := data[i][sortConfig.Field]
		valJ, okJ := data[j][sortConfig.Field]

		if !okI || !okJ {
			return okI // Put items with the field first
		}

		// Try numeric comparison
		iFloat, iOk := toFloat64(valI)
		jFloat, jOk := toFloat64(valJ)
		if iOk && jOk {
			if sortConfig.Order == "desc" {
				return iFloat > jFloat
			}
			return iFloat < jFloat
		}

		// Try string comparison
		iStr, iOk := valI.(string)
		jStr, jOk := valJ.(string)
		if iOk && jOk {
			if sortConfig.Order == "desc" {
				return iStr > jStr
			}
			return iStr < jStr
		}

		return false
	})
}
