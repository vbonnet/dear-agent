package main

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func logicalItemSchema(t *testing.T, itemType string) string {
	t.Helper()
	if itemType == "" {
		return contractAbsent
	}
	return canonicalSchemaJSON(t, map[string]any{"type": itemType})
}

func canonicalSchemaJSON(t *testing.T, schema any) string {
	t.Helper()
	data, err := json.Marshal(normalizeSubschema(schema))
	if err != nil {
		t.Fatalf("canonicalize item schema: %v", err)
	}
	return string(data)
}

func normalizeSchemaObject(schema map[string]any) map[string]any {
	result := make(map[string]any, len(schema))
	for keyword, value := range schema {
		switch keyword {
		case "enum", "required", "type":
			result[keyword] = normalizeSetArray(value)
		case "dependentRequired":
			result[keyword] = normalizeDependentRequired(value)
		case "dependencies":
			result[keyword] = normalizeDependencies(value)
		case "allOf", "anyOf", "oneOf":
			result[keyword] = normalizeSchemaArray(value, true)
		case "prefixItems":
			result[keyword] = normalizeSchemaArray(value, false)
		case "$defs", "definitions", "dependentSchemas", "patternProperties", "properties":
			result[keyword] = normalizeSchemaMap(value)
		case "additionalItems", "additionalProperties", "contains", "contentSchema", "else", "if",
			"items", "not", "propertyNames", "then", "unevaluatedItems", "unevaluatedProperties":
			result[keyword] = normalizeSubschema(value)
		default:
			result[keyword] = value
		}
	}
	return result
}

func normalizeSetArray(value any) any {
	values, ok := value.([]any)
	if !ok {
		return value
	}
	result := make([]any, len(values))
	copy(result, values)
	sortCanonicalJSON(result)
	return result
}

func normalizeDependentRequired(value any) any {
	properties, ok := value.(map[string]any)
	if !ok {
		return value
	}
	result := make(map[string]any, len(properties))
	for property, dependencies := range properties {
		result[property] = normalizeSetArray(dependencies)
	}
	return result
}

func normalizeDependencies(value any) any {
	dependencies, ok := value.(map[string]any)
	if !ok {
		return value
	}
	result := make(map[string]any, len(dependencies))
	for property, dependency := range dependencies {
		if _, propertyDependency := dependency.([]any); propertyDependency {
			result[property] = normalizeSetArray(dependency)
			continue
		}
		result[property] = normalizeSubschema(dependency)
	}
	return result
}

func normalizeSchemaArray(value any, setLike bool) any {
	values, ok := value.([]any)
	if !ok {
		return value
	}
	result := make([]any, len(values))
	for index, schema := range values {
		result[index] = normalizeSubschema(schema)
	}
	if setLike {
		sortCanonicalJSON(result)
	}
	return result
}

func normalizeSchemaMap(value any) any {
	properties, ok := value.(map[string]any)
	if !ok {
		return value
	}
	result := make(map[string]any, len(properties))
	for property, schema := range properties {
		result[property] = normalizeSubschema(schema)
	}
	return result
}

func normalizeSubschema(value any) any {
	switch schema := value.(type) {
	case map[string]any:
		return normalizeSchemaObject(schema)
	case []any:
		result := make([]any, len(schema))
		for index, item := range schema {
			result[index] = normalizeSubschema(item)
		}
		return result
	default:
		return value
	}
}

func sortCanonicalJSON(values []any) {
	sort.Slice(values, func(i, j int) bool {
		return canonicalJSONSortKey(values[i]) < canonicalJSONSortKey(values[j])
	})
}

func canonicalJSONSortKey(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func itemSchemaValue(value string) string {
	if value == "" {
		return contractAbsent
	}
	return value
}

func schemaConstraints(t *testing.T, schema map[string]any) string {
	t.Helper()
	constraints := make(map[string]any)
	for keyword, value := range schema {
		switch keyword {
		case "additionalProperties", "description", "enum", "items", "properties", "required", "type":
			continue
		default:
			constraints[keyword] = value
		}
	}
	if len(constraints) == 0 {
		return contractAbsent
	}
	return canonicalSchemaJSON(t, constraints)
}

func constraintValue(value string) string {
	if value == "" {
		return contractAbsent
	}
	return value
}

func decodeSchemaJSON(t *testing.T, raw string) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil {
		t.Fatalf("decode raw schema fixture: %v", err)
	}
	return schema
}

func TestSchemaCanonicalizationPreservesAbsentAndEmptyEnum(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "absent", raw: `{"type":"string"}`, want: contractAbsent},
		{name: "explicit empty", raw: `{"type":"string","enum":[]}`, want: "[]"},
		{name: "empty member", raw: `{"type":"string","enum":[""]}`, want: `[""]`},
		{name: "comma-bearing member", raw: `{"type":"string","enum":["a,b","c"]}`, want: `["a,b","c"]`},
		{name: "split comma-bearing member", raw: `{"type":"string","enum":["a","b,c"]}`, want: `["a","b,c"]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := decodeSchemaJSON(t, tt.raw)
			nodes := make(map[string]contractSchemaNode)
			canonicalizeSchema(t, "/", schema, false, nodes)
			if got := enumValue(nodes["/"]); got != tt.want {
				t.Fatalf("canonical enum = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSchemaCanonicalizationPreservesNestedItemConstraints(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "type only",
			raw:  `{"type":"array","items":{"type":"string"}}`,
			want: `{"type":"string"}`,
		},
		{
			name: "explicit empty enum",
			raw:  `{"type":"array","items":{"type":"string","enum":[]}}`,
			want: `{"enum":[],"type":"string"}`,
		},
		{
			name: "arbitrary constraints with set normalization",
			raw:  `{"type":"array","items":{"pattern":"^x+$","enum":["b","a"],"minLength":1,"type":"string"}}`,
			want: `{"enum":["a","b"],"minLength":1,"pattern":"^x+$","type":"string"}`,
		},
		{
			name: "boolean schema",
			raw:  `{"type":"array","items":false}`,
			want: `false`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := decodeSchemaJSON(t, tt.raw)
			nodes := make(map[string]contractSchemaNode)
			canonicalizeSchema(t, "/", schema, false, nodes)
			if got := itemSchemaValue(nodes["/"].ItemSchema); got != tt.want {
				t.Fatalf("canonical item schema = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSchemaCanonicalizationPreservesPropertyConstraints(t *testing.T) {
	raw := `{"type":"object","properties":{"value":{"type":"string","pattern":"^x+$","minLength":1}}}`
	schema := decodeSchemaJSON(t, raw)
	nodes := make(map[string]contractSchemaNode)
	canonicalizeSchema(t, "/", schema, false, nodes)
	if got, want := constraintValue(nodes["/value"].Constraints), `{"minLength":1,"pattern":"^x+$"}`; got != want {
		t.Fatalf("canonical property constraints = %q, want %q", got, want)
	}
	if got := constraintValue(nodes["/"].Constraints); got != contractAbsent {
		t.Fatalf("canonical root constraints = %q, want %q", got, contractAbsent)
	}
}

func TestSchemaCanonicalizationPreservesBooleanPropertySchemas(t *testing.T) {
	raw := `{"type":"object","properties":{"allowed":true,"blocked":false},"required":["blocked"]}`
	schema := decodeSchemaJSON(t, raw)
	nodes := make(map[string]contractSchemaNode)
	canonicalizeSchema(t, "/", schema, false, nodes)
	if got := constraintValue(nodes["/allowed"].Constraints); got != "true" {
		t.Fatalf("allowed property schema = %q, want true", got)
	}
	blocked := nodes["/blocked"]
	if got := constraintValue(blocked.Constraints); got != "false" {
		t.Fatalf("blocked property schema = %q, want false", got)
	}
	if !blocked.Required {
		t.Fatal("blocked boolean property lost requiredness")
	}
}

func TestLogicalJSONTypeDistinguishesIntegerAndNumber(t *testing.T) {
	tests := []struct {
		goType string
		want   string
	}{
		{goType: "int", want: "integer"},
		{goType: "int64", want: "integer"},
		{goType: "float64", want: "number"},
	}
	for _, tt := range tests {
		t.Run(tt.goType, func(t *testing.T) {
			got, itemType := logicalJSONType(tt.goType)
			if got != tt.want || itemType != "" {
				t.Fatalf("logicalJSONType(%q) = (%q, %q), want (%q, empty)", tt.goType, got, itemType, tt.want)
			}
		})
	}
}

func TestSchemaCanonicalizationUsesSchemaContext(t *testing.T) {
	tests := []struct {
		name      string
		left      string
		right     string
		wantEqual bool
	}{
		{
			name:      "dependent-required-order-is-semantic-set",
			left:      `{"dependentRequired":{"card":["billing","security"]}}`,
			right:     `{"dependentRequired":{"card":["security","billing"]}}`,
			wantEqual: true,
		},
		{
			name:      "draft-seven-property-dependency-order-is-semantic-set",
			left:      `{"dependencies":{"card":["billing","security"]}}`,
			right:     `{"dependencies":{"card":["security","billing"]}}`,
			wantEqual: true,
		},
		{
			name:      "draft-seven-schema-dependency-is-normalized-as-schema",
			left:      `{"dependencies":{"card":{"required":["billing","security"]}}}`,
			right:     `{"dependencies":{"card":{"required":["security","billing"]}}}`,
			wantEqual: true,
		},
		{
			name:      "enum-order-is-semantic-set",
			left:      `{"enum":[{"kind":"a"},{"kind":"b"}]}`,
			right:     `{"enum":[{"kind":"b"},{"kind":"a"}]}`,
			wantEqual: true,
		},
		{
			name:      "required-and-type-order-are-semantic-sets",
			left:      `{"required":["a","b"],"type":["null","object"]}`,
			right:     `{"required":["b","a"],"type":["object","null"]}`,
			wantEqual: true,
		},
		{
			name:      "schema-combinator-order-is-semantic-set",
			left:      `{"allOf":[{"required":["a","b"]},{"type":"object"}]}`,
			right:     `{"allOf":[{"type":"object"},{"required":["b","a"]}]}`,
			wantEqual: true,
		},
		{
			name:      "any-of-order-is-semantic-set",
			left:      `{"anyOf":[{"type":"string"},{"type":"null"}]}`,
			right:     `{"anyOf":[{"type":"null"},{"type":"string"}]}`,
			wantEqual: true,
		},
		{
			name:      "one-of-order-is-semantic-set",
			left:      `{"oneOf":[{"const":"a"},{"const":"b"}]}`,
			right:     `{"oneOf":[{"const":"b"},{"const":"a"}]}`,
			wantEqual: true,
		},
		{
			name:      "const-array-order-is-instance-data",
			left:      `{"const":{"type":["a","b"]}}`,
			right:     `{"const":{"type":["b","a"]}}`,
			wantEqual: false,
		},
		{
			name:      "array-order-inside-enum-member-is-instance-data",
			left:      `{"enum":[{"type":["a","b"]}]}`,
			right:     `{"enum":[{"type":["b","a"]}]}`,
			wantEqual: false,
		},
		{
			name:      "required-key-inside-enum-member-is-instance-data",
			left:      `{"enum":[{"required":["a","b"]}]}`,
			right:     `{"enum":[{"required":["b","a"]}]}`,
			wantEqual: false,
		},
		{
			name:      "prefix-item-order-is-positional",
			left:      `{"prefixItems":[{"type":"string"},{"type":"integer"}]}`,
			right:     `{"prefixItems":[{"type":"integer"},{"type":"string"}]}`,
			wantEqual: false,
		},
		{
			name:      "tuple-items-order-is-positional",
			left:      `{"items":[{"type":"string"},{"type":"integer"}]}`,
			right:     `{"items":[{"type":"integer"},{"type":"string"}]}`,
			wantEqual: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := canonicalSchemaFixture(t, tt.left)
			right := canonicalSchemaFixture(t, tt.right)
			if got := left == right; got != tt.wantEqual {
				t.Fatalf("canonical equality = %t, want %t\nleft:  %s\nright: %s", got, tt.wantEqual, left, right)
			}
		})
	}
}

func canonicalSchemaFixture(t *testing.T, raw string) string {
	t.Helper()
	return canonicalSchemaJSON(t, decodeSchemaJSON(t, raw))
}

func TestSchemaCanonicalizationPreservesLargeIntegerPrecision(t *testing.T) {
	left := canonicalSchemaFixture(t, `{"const":9007199254740992}`)
	right := canonicalSchemaFixture(t, `{"const":9007199254740993}`)
	if left == right {
		t.Fatalf("distinct exact integer constraints collapsed to %s", left)
	}
}
