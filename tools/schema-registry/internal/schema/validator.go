// Package schema provides schema validation.
package schema

import (
	"fmt"
	"strings"

	"github.com/xeipuuv/gojsonschema"
)

// CompatibilityMode defines schema compatibility requirements
type CompatibilityMode string

// Compatibility mode constants define schema evolution rules.
const (
	CompatibilityBackward CompatibilityMode = "backward"
	CompatibilityForward  CompatibilityMode = "forward"
	CompatibilityFull     CompatibilityMode = "full"
	CompatibilityNone     CompatibilityMode = "none"
)

// CompatibilityResult represents the result of compatibility checking
type CompatibilityResult struct {
	Passed     bool     `json:"passed"`
	Mode       string   `json:"mode"`
	Violations []string `json:"violations"`
	Warnings   []string `json:"warnings"`
}

// ValidateSchema validates a schema against JSON Schema Draft 2020-12
//
//nolint:gocyclo // schema validation requires exhaustive field checking
func ValidateSchema(schemaData map[string]interface{}) error {
	// Check required top-level fields
	requiredFields := []string{"$schema", "component", "version", "schemas"}
	for _, field := range requiredFields {
		if _, ok := schemaData[field]; !ok {
			return fmt.Errorf("missing required field: %s", field)
		}
	}

	// Validate component name format (lowercase, kebab-case)
	component, ok := schemaData["component"].(string)
	if !ok {
		return fmt.Errorf("component must be a string")
	}
	if !isValidComponentName(component) {
		return fmt.Errorf("invalid component name: must be lowercase, kebab-case (^[a-z][a-z0-9-]*$)")
	}

	// Validate version format (semver)
	version, ok := schemaData["version"].(string)
	if !ok {
		return fmt.Errorf("version must be a string")
	}
	if !isValidSemver(version) {
		return fmt.Errorf("invalid version: must be semantic version (MAJOR.MINOR.PATCH)")
	}

	// Validate compatibility mode if present
	if compat, ok := schemaData["compatibility"].(string); ok {
		if !isValidCompatibilityMode(compat) {
			return fmt.Errorf("invalid compatibility mode: must be backward, forward, full, or none")
		}
	}

	// Validate schemas object
	schemas, ok := schemaData["schemas"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("schemas must be an object")
	}
	if len(schemas) == 0 {
		return fmt.Errorf("schemas object cannot be empty")
	}

	// Validate each schema is valid JSON Schema
	for name, schemaObj := range schemas {
		schemaMap, ok := schemaObj.(map[string]interface{})
		if !ok {
			return fmt.Errorf("schema '%s' must be an object", name)
		}

		if err := validateJSONSchema(schemaMap); err != nil {
			return fmt.Errorf("schema '%s' is invalid: %w", name, err)
		}
	}

	// Validate examples against schemas if present
	if examples, ok := schemaData["examples"].(map[string]interface{}); ok {
		for name, example := range examples {
			schemaObj, ok := schemas[name]
			if !ok {
				return fmt.Errorf("example '%s' has no corresponding schema", name)
			}

			if err := ValidateData(schemaObj, example); err != nil {
				return fmt.Errorf("example '%s' does not validate against its schema: %w", name, err)
			}
		}
	}

	return nil
}

// ValidateData validates data against a JSON schema
func ValidateData(schema, data interface{}) error {
	schemaLoader := gojsonschema.NewGoLoader(schema)
	dataLoader := gojsonschema.NewGoLoader(data)

	result, err := gojsonschema.Validate(schemaLoader, dataLoader)
	if err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	if !result.Valid() {
		var errors []string
		for _, err := range result.Errors() {
			errors = append(errors, err.String())
		}
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// CheckCompatibility checks compatibility between old and new schemas
func CheckCompatibility(oldSchema, newSchema map[string]interface{}, mode CompatibilityMode) *CompatibilityResult {
	result := &CompatibilityResult{
		Passed:     true,
		Mode:       string(mode),
		Violations: []string{},
		Warnings:   []string{},
	}

	if mode == CompatibilityNone {
		return result
	}

	// Extract schemas maps
	oldSchemas, ok := oldSchema["schemas"].(map[string]interface{})
	if !ok {
		result.Passed = false
		result.Violations = append(result.Violations, "old schema has no 'schemas' field")
		return result
	}

	newSchemas, ok := newSchema["schemas"].(map[string]interface{})
	if !ok {
		result.Passed = false
		result.Violations = append(result.Violations, "new schema has no 'schemas' field")
		return result
	}

	// Check each schema
	for schemaName := range oldSchemas {
		oldS, ok1 := oldSchemas[schemaName].(map[string]interface{})
		newS, ok2 := newSchemas[schemaName]

		if !ok1 {
			continue
		}

		if !ok2 {
			result.Violations = append(result.Violations, fmt.Sprintf("schema '%s' removed", schemaName))
			result.Passed = false
			continue
		}

		newSMap, ok := newS.(map[string]interface{})
		if !ok {
			continue
		}

		// Check backward compatibility
		if mode == CompatibilityBackward || mode == CompatibilityFull {
			violations := checkBackwardCompatibility(oldS, newSMap, schemaName)
			result.Violations = append(result.Violations, violations...)
			if len(violations) > 0 {
				result.Passed = false
			}
		}

		// Check forward compatibility
		if mode == CompatibilityForward || mode == CompatibilityFull {
			violations := checkForwardCompatibility(oldS, newSMap, schemaName)
			result.Violations = append(result.Violations, violations...)
			if len(violations) > 0 {
				result.Passed = false
			}
		}
	}

	return result
}

// checkBackwardCompatibility checks if new schema can read old data
//
//nolint:gocyclo // compatibility checking requires exhaustive field comparison
func checkBackwardCompatibility(oldSchema, newSchema map[string]interface{}, schemaName string) []string {
	var violations []string

	oldProps, _ := oldSchema["properties"].(map[string]interface{})
	newProps, _ := newSchema["properties"].(map[string]interface{})

	oldRequired := getRequiredFields(oldSchema)
	newRequired := getRequiredFields(newSchema)

	// Check removed required fields
	for _, field := range oldRequired {
		if !contains(newRequired, field) {
			violations = append(violations, fmt.Sprintf("%s: removed required field '%s'", schemaName, field))
		}
	}

	// Check added required fields without defaults
	for _, field := range newRequired {
		if !contains(oldRequired, field) {
			// Check if it has a default
			if newProps != nil {
				if fieldSchema, ok := newProps[field].(map[string]interface{}); ok {
					if _, hasDefault := fieldSchema["default"]; !hasDefault {
						violations = append(violations, fmt.Sprintf("%s: added required field '%s' without default", schemaName, field))
					}
				}
			}
		}
	}

	// Check type changes
	if oldProps != nil && newProps != nil {
		for fieldName, oldProp := range oldProps {
			if newProp, ok := newProps[fieldName]; ok {
				oldPropMap, ok1 := oldProp.(map[string]interface{})
				newPropMap, ok2 := newProp.(map[string]interface{})

				if ok1 && ok2 {
					oldType := getTypeString(oldPropMap)
					newType := getTypeString(newPropMap)

					if oldType != "" && newType != "" && oldType != newType {
						violations = append(violations, fmt.Sprintf("%s: changed type for field '%s': %s → %s", schemaName, fieldName, oldType, newType))
					}
				}
			}
		}
	}

	return violations
}

// checkForwardCompatibility checks if old schema can read new data
func checkForwardCompatibility(oldSchema, newSchema map[string]interface{}, schemaName string) []string {
	var violations []string

	oldRequired := getRequiredFields(oldSchema)
	newRequired := getRequiredFields(newSchema)

	// Check added required fields
	for _, field := range newRequired {
		if !contains(oldRequired, field) {
			violations = append(violations, fmt.Sprintf("%s: added required field '%s' (breaks forward compatibility)", schemaName, field))
		}
	}

	return violations
}

// Helper functions

func isValidComponentName(name string) bool {
	if len(name) == 0 {
		return false
	}
	if name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, ch := range name {
		isLower := ch >= 'a' && ch <= 'z'
		isDigit := ch >= '0' && ch <= '9'
		if !isLower && !isDigit && ch != '-' {
			return false
		}
	}
	return true
}

func isValidSemver(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if len(part) == 0 {
			return false
		}
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return false
			}
		}
	}
	return true
}

func isValidCompatibilityMode(mode string) bool {
	return mode == "backward" || mode == "forward" || mode == "full" || mode == "none"
}

func validateJSONSchema(schema map[string]interface{}) error {
	// Basic validation - check it has type or properties
	if _, hasType := schema["type"]; !hasType {
		if _, hasProps := schema["properties"]; !hasProps {
			return fmt.Errorf("schema must have 'type' or 'properties'")
		}
	}
	return nil
}

func getRequiredFields(schema map[string]interface{}) []string {
	if required, ok := schema["required"].([]interface{}); ok {
		var fields []string
		for _, f := range required {
			if str, ok := f.(string); ok {
				fields = append(fields, str)
			}
		}
		return fields
	}
	return []string{}
}

func getTypeString(schema map[string]interface{}) string {
	if typeVal, ok := schema["type"].(string); ok {
		return typeVal
	}
	if typeArr, ok := schema["type"].([]interface{}); ok {
		var types []string
		for _, t := range typeArr {
			if str, ok := t.(string); ok {
				types = append(types, str)
			}
		}
		return strings.Join(types, "|")
	}
	return ""
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
