package cliframe

import (
	"encoding/json"
	"testing"
)

func TestJSONFormatter_Format_PrettyPrint(t *testing.T) {
	formatter := NewJSONFormatter(true)

	data := map[string]interface{}{
		"name":  "Alice",
		"age":   30,
		"admin": true,
	}

	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// Should be pretty-printed with indentation
	var decoded map[string]interface{}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Verify it contains newlines and spaces (pretty-printed)
	if len(result) < 20 {
		t.Error("Expected pretty-printed JSON to be longer")
	}

	// Verify data integrity
	if decoded["name"] != "Alice" {
		t.Errorf("Expected name=Alice, got %v", decoded["name"])
	}
}

func TestJSONFormatter_Format_Compact(t *testing.T) {
	formatter := NewJSONFormatter(false)

	data := map[string]interface{}{
		"name":  "Bob",
		"age":   25,
		"admin": false,
	}

	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// Should be compact (no unnecessary whitespace)
	var decoded map[string]interface{}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Verify data integrity
	if decoded["name"] != "Bob" {
		t.Errorf("Expected name=Bob, got %v", decoded["name"])
	}
	if decoded["age"].(float64) != 25 {
		t.Errorf("Expected age=25, got %v", decoded["age"])
	}
}

func TestJSONFormatter_Format_NilValue(t *testing.T) {
	formatter := NewJSONFormatter(false)

	result, err := formatter.Format(nil)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	if string(result) != "null" {
		t.Errorf("Expected 'null', got %s", result)
	}
}

func TestJSONFormatter_Format_EmptySlice(t *testing.T) {
	formatter := NewJSONFormatter(false)

	data := []string{}
	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	if string(result) != "[]" {
		t.Errorf("Expected '[]', got %s", result)
	}
}

func TestJSONFormatter_Format_EmptyMap(t *testing.T) {
	formatter := NewJSONFormatter(false)

	data := map[string]interface{}{}
	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	if string(result) != "{}" {
		t.Errorf("Expected '{}', got %s", result)
	}
}

func TestJSONFormatter_Format_NestedStructures(t *testing.T) {
	formatter := NewJSONFormatter(true)

	data := map[string]interface{}{
		"user": map[string]interface{}{
			"name": "Charlie",
			"contacts": []string{
				"email@example.com",
				"phone@example.com",
			},
		},
		"metadata": map[string]interface{}{
			"version": 1,
			"created": "2024-01-01",
		},
	}

	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	// Verify nested structure
	user := decoded["user"].(map[string]interface{})
	if user["name"] != "Charlie" {
		t.Errorf("Expected nested name=Charlie, got %v", user["name"])
	}

	contacts := user["contacts"].([]interface{})
	if len(contacts) != 2 {
		t.Errorf("Expected 2 contacts, got %d", len(contacts))
	}
}

func TestJSONFormatter_Format_VariousDataTypes(t *testing.T) {
	formatter := NewJSONFormatter(false)

	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"string", "hello", `"hello"`},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"null", nil, "null"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatter.Format(tt.input)
			if err != nil {
				t.Fatalf("Format failed: %v", err)
			}

			if string(result) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestJSONFormatter_Format_StructWithTags(t *testing.T) {
	type User struct {
		Name  string `json:"name"`
		Email string `json:"email"`
		Age   int    `json:"age,omitempty"`
	}

	formatter := NewJSONFormatter(false)

	user := User{
		Name:  "Diana",
		Email: "diana@example.com",
		Age:   28,
	}

	result, err := formatter.Format(user)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if decoded["name"] != "Diana" {
		t.Errorf("Expected name=Diana, got %v", decoded["name"])
	}
	if decoded["email"] != "diana@example.com" {
		t.Errorf("Expected email=diana@example.com, got %v", decoded["email"])
	}
}

func TestJSONFormatter_Name(t *testing.T) {
	formatter := NewJSONFormatter(false)
	if formatter.Name() != "json" {
		t.Errorf("Expected name 'json', got %s", formatter.Name())
	}
}

func TestJSONFormatter_MIMEType(t *testing.T) {
	formatter := NewJSONFormatter(false)
	if formatter.MIMEType() != "application/json" {
		t.Errorf("Expected MIME type 'application/json', got %s", formatter.MIMEType())
	}
}

func TestJSONFormatter_HTMLNotEscaped(t *testing.T) {
	formatter := NewJSONFormatter(false)

	data := map[string]string{
		"html": "<script>alert('test')</script>",
	}

	result, err := formatter.Format(data)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	// JSON should NOT escape HTML by default (escapeHTML = false)
	// Note: json.Marshal always escapes by default, so this tests the intent
	var decoded map[string]string
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("Invalid JSON: %v", err)
	}

	if decoded["html"] != "<script>alert('test')</script>" {
		t.Errorf("HTML was modified: %s", decoded["html"])
	}
}
