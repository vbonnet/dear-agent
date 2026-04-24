package schema

import (
	"testing"
)

func TestValidateSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  map[string]interface{}
		wantErr bool
	}{
		{
			name: "valid schema",
			schema: map[string]interface{}{
				"$schema":   "https://corpus-callosum.dev/schema/v1",
				"component": "test-component",
				"version":   "1.0.0",
				"schemas": map[string]interface{}{
					"test": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "string",
							},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing component",
			schema: map[string]interface{}{
				"$schema": "https://corpus-callosum.dev/schema/v1",
				"version": "1.0.0",
				"schemas": map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "invalid component name",
			schema: map[string]interface{}{
				"$schema":   "https://corpus-callosum.dev/schema/v1",
				"component": "Test-Component",
				"version":   "1.0.0",
				"schemas":   map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "invalid version",
			schema: map[string]interface{}{
				"$schema":   "https://corpus-callosum.dev/schema/v1",
				"component": "test-component",
				"version":   "1.0",
				"schemas":   map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "empty schemas",
			schema: map[string]interface{}{
				"$schema":   "https://corpus-callosum.dev/schema/v1",
				"component": "test-component",
				"version":   "1.0.0",
				"schemas":   map[string]interface{}{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSchema(tt.schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckBackwardCompatibility(t *testing.T) {
	oldSchema := map[string]interface{}{
		"schemas": map[string]interface{}{
			"user": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"id": map[string]interface{}{
						"type": "string",
					},
					"name": map[string]interface{}{
						"type": "string",
					},
				},
				"required": []interface{}{"id", "name"},
			},
		},
	}

	tests := []struct {
		name      string
		newSchema map[string]interface{}
		mode      CompatibilityMode
		wantPass  bool
	}{
		{
			name: "add optional field with default",
			newSchema: map[string]interface{}{
				"schemas": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "string",
							},
							"name": map[string]interface{}{
								"type": "string",
							},
							"email": map[string]interface{}{
								"type":    "string",
								"default": "",
							},
						},
						"required": []interface{}{"id", "name"},
					},
				},
			},
			mode:     CompatibilityBackward,
			wantPass: true,
		},
		{
			name: "remove required field",
			newSchema: map[string]interface{}{
				"schemas": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "string",
							},
						},
						"required": []interface{}{"id"},
					},
				},
			},
			mode:     CompatibilityBackward,
			wantPass: false,
		},
		{
			name: "change field type",
			newSchema: map[string]interface{}{
				"schemas": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "integer",
							},
							"name": map[string]interface{}{
								"type": "string",
							},
						},
						"required": []interface{}{"id", "name"},
					},
				},
			},
			mode:     CompatibilityBackward,
			wantPass: false,
		},
		{
			name: "no compatibility check",
			newSchema: map[string]interface{}{
				"schemas": map[string]interface{}{
					"user": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"id": map[string]interface{}{
								"type": "integer",
							},
						},
						"required": []interface{}{"id"},
					},
				},
			},
			mode:     CompatibilityNone,
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CheckCompatibility(oldSchema, tt.newSchema, tt.mode)
			if result.Passed != tt.wantPass {
				t.Errorf("CheckCompatibility() passed = %v, want %v. Violations: %v",
					result.Passed, tt.wantPass, result.Violations)
			}
		})
	}
}

func TestValidateData(t *testing.T) {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{
				"type": "string",
			},
			"count": map[string]interface{}{
				"type":    "integer",
				"minimum": 0,
			},
		},
		"required": []interface{}{"id"},
	}

	tests := []struct {
		name    string
		data    interface{}
		wantErr bool
	}{
		{
			name: "valid data",
			data: map[string]interface{}{
				"id":    "test-123",
				"count": 5,
			},
			wantErr: false,
		},
		{
			name: "missing required field",
			data: map[string]interface{}{
				"count": 5,
			},
			wantErr: true,
		},
		{
			name: "wrong type",
			data: map[string]interface{}{
				"id":    "test-123",
				"count": "five",
			},
			wantErr: true,
		},
		{
			name: "constraint violation",
			data: map[string]interface{}{
				"id":    "test-123",
				"count": -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateData(schema, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateData() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestIsValidComponentName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid lowercase", "agm", true},
		{"valid kebab-case", "multi-word-component", true},
		{"valid with numbers", "component2", true},
		{"uppercase", "AGM", false},
		{"starts with number", "2component", false},
		{"underscore", "my_component", false},
		{"space", "my component", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidComponentName(tt.input); got != tt.want {
				t.Errorf("isValidComponentName(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidSemver(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"valid", "1.0.0", true},
		{"valid with larger numbers", "10.20.30", true},
		{"two parts", "1.0", false},
		{"four parts", "1.0.0.0", false},
		{"non-numeric", "1.0.a", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidSemver(tt.input); got != tt.want {
				t.Errorf("isValidSemver(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
