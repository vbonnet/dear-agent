package main

import (
	"encoding/json"
	"sort"
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
			var schema map[string]any
			if err := json.Unmarshal([]byte(tt.raw), &schema); err != nil {
				t.Fatalf("decode raw schema fixture: %v", err)
			}
			nodes := make(map[string]contractSchemaNode)
			canonicalizeSchema(t, "/", schema, false, nodes)
			if got := itemSchemaValue(nodes["/"].ItemSchema); got != tt.want {
				t.Fatalf("canonical item schema = %q, want %q", got, tt.want)
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
	var schema map[string]any
	if err := json.Unmarshal([]byte(raw), &schema); err != nil {
		t.Fatalf("decode raw schema fixture: %v", err)
	}
	return canonicalSchemaJSON(t, schema)
}
