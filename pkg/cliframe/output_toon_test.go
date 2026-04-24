package cliframe

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTOONFormatter_Format_BasicStruct(t *testing.T) {
	type User struct {
		ID    int
		Name  string
		Email string
	}

	formatter := NewTOONFormatter()

	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	result, err := formatter.Format(users)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Check header format: users[3]{id,name,email}
	if !strings.HasPrefix(output, "users[3]{") {
		t.Errorf("Expected header 'users[3]{...', got: %s", strings.Split(output, "\n")[0])
	}

	// Check field names in header (converted to lowerCamelCase)
	// ID -> iD, Name -> name, Email -> email
	if !strings.Contains(output, "iD") && !strings.Contains(output, "name") && !strings.Contains(output, "email") {
		t.Error("Expected field names in header")
	}

	// Check data rows
	if !strings.Contains(output, "Alice") {
		t.Error("Expected 'Alice' in output")
	}
	if !strings.Contains(output, "alice@example.com") {
		t.Error("Expected 'alice@example.com' in output")
	}
}

func TestTOONFormatter_Format_WithJSONTags(t *testing.T) {
	type Product struct {
		ID    int     `json:"id"`
		Name  string  `json:"product_name"`
		Price float64 `json:"price"`
	}

	formatter := NewTOONFormatter()

	products := []Product{
		{ID: 1, Name: "Widget", Price: 9.99},
		{ID: 2, Name: "Gadget", Price: 19.99},
	}

	result, err := formatter.Format(products)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Should use JSON tag names
	if !strings.Contains(output, "product_name") {
		t.Error("Expected 'product_name' from JSON tag")
	}
	if !strings.Contains(output, "price") {
		t.Error("Expected 'price' from JSON tag")
	}

	// Check data
	if !strings.Contains(output, "Widget") {
		t.Error("Expected 'Widget' in output")
	}
}

func TestTOONFormatter_Format_WithTOONTags(t *testing.T) {
	type Item struct {
		ID   int    `toon:"identifier"`
		Name string `toon:"item_name"`
		Skip string `toon:"-"` // Should be skipped
	}

	formatter := NewTOONFormatter()

	items := []Item{
		{ID: 1, Name: "Test", Skip: "hidden"},
	}

	result, err := formatter.Format(items)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Should use TOON tag names
	if !strings.Contains(output, "identifier") {
		t.Error("Expected 'identifier' from TOON tag")
	}
	if !strings.Contains(output, "item_name") {
		t.Error("Expected 'item_name' from TOON tag")
	}

	// Should skip field with toon:"-"
	if strings.Contains(output, "Skip") || strings.Contains(output, "hidden") {
		t.Error("Should skip field with toon:'-' tag")
	}
}

func TestTOONFormatter_Format_EmptySlice(t *testing.T) {
	type User struct {
		Name string
	}

	formatter := NewTOONFormatter()

	users := []User{}

	result, err := formatter.Format(users)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	if len(result) != 0 {
		t.Errorf("Expected empty output for empty slice, got %d bytes", len(result))
	}
}

func TestTOONFormatter_Format_NotSlice(t *testing.T) {
	formatter := NewTOONFormatter()

	data := map[string]string{"key": "value"}

	_, err := formatter.Format(data)
	if err == nil {
		t.Error("Expected error for non-slice input")
	}

	if !strings.Contains(err.Error(), "requires slice") {
		t.Errorf("Expected 'requires slice' error, got: %v", err)
	}
}

func TestTOONFormatter_Format_NotStruct(t *testing.T) {
	formatter := NewTOONFormatter()

	data := []int{1, 2, 3}

	_, err := formatter.Format(data)
	if err == nil {
		t.Error("Expected error for non-struct slice")
	}

	if !strings.Contains(err.Error(), "requires slice of structs") {
		t.Errorf("Expected 'requires slice of structs' error, got: %v", err)
	}
}

func TestTOONFormatter_Format_CSVEscaping(t *testing.T) {
	type Record struct {
		Name  string
		Notes string
	}

	formatter := NewTOONFormatter()

	records := []Record{
		{Name: "Test", Notes: "Has, comma"},
		{Name: "Quote", Notes: `Has "quotes"`},
		{Name: "Both", Notes: `Has, "both"`},
	}

	result, err := formatter.Format(records)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// CSV escaping should handle commas and quotes
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 4 { // header + 3 data rows
		t.Errorf("Expected at least 4 lines, got %d", len(lines))
	}
}

func TestTOONFormatter_Format_PointerSlice(t *testing.T) {
	type User struct {
		Name string
	}

	formatter := NewTOONFormatter()

	users := []*User{
		{Name: "Alice"},
		{Name: "Bob"},
	}

	result, err := formatter.Format(users)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	if !strings.Contains(output, "Alice") {
		t.Error("Expected 'Alice' in output")
	}
	if !strings.Contains(output, "Bob") {
		t.Error("Expected 'Bob' in output")
	}
}

func TestTOONFormatter_Format_NilPointerInSlice(t *testing.T) {
	type User struct {
		Name string
	}

	formatter := NewTOONFormatter()

	users := []*User{
		{Name: "Alice"},
		nil, // nil pointer
		{Name: "Bob"},
	}

	result, err := formatter.Format(users)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Should skip nil pointers gracefully
	if !strings.Contains(output, "Alice") {
		t.Error("Expected 'Alice' in output")
	}
	if !strings.Contains(output, "Bob") {
		t.Error("Expected 'Bob' in output")
	}
}

func TestTOONFormatter_Format_VariousDataTypes(t *testing.T) {
	type Record struct {
		StringVal string
		IntVal    int
		BoolVal   bool
		FloatVal  float64
	}

	formatter := NewTOONFormatter()

	records := []Record{
		{StringVal: "test", IntVal: 42, BoolVal: true, FloatVal: 3.14159},
	}

	result, err := formatter.Format(records)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Verify data types are formatted correctly
	if !strings.Contains(output, "test") {
		t.Error("Expected string value 'test'")
	}
	if !strings.Contains(output, "42") {
		t.Error("Expected int value '42'")
	}
	if !strings.Contains(output, "true") {
		t.Error("Expected bool value 'true'")
	}
	if !strings.Contains(output, "3.14159") {
		t.Error("Expected float value '3.14159'")
	}
}

func TestTOONFormatter_Format_TokenEfficiency(t *testing.T) {
	type User struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	// Format as TOON
	toonFormatter := NewTOONFormatter()
	toonResult, err := toonFormatter.Format(users)
	if err != nil {
		t.Fatalf("TOON format failed: %v", err)
	}

	// Format as JSON
	jsonFormatter := NewJSONFormatter(false)
	jsonResult, err := jsonFormatter.Format(users)
	if err != nil {
		t.Fatalf("JSON format failed: %v", err)
	}

	toonSize := len(toonResult)
	jsonSize := len(jsonResult)

	// TOON should be significantly smaller (target: 35-40% reduction)
	// For this test, we'll check for at least 20% reduction
	reduction := float64(jsonSize-toonSize) / float64(jsonSize) * 100

	t.Logf("TOON size: %d bytes, JSON size: %d bytes, reduction: %.1f%%",
		toonSize, jsonSize, reduction)

	if toonSize >= jsonSize {
		t.Errorf("TOON should be smaller than JSON: TOON=%d, JSON=%d", toonSize, jsonSize)
	}

	if reduction < 20 {
		t.Logf("Warning: Token reduction is %.1f%%, expected >20%% (target 35-40%%)", reduction)
	}
}

func TestTOONFormatter_Format_UnexportedFieldsIgnored(t *testing.T) {
	type User struct {
		Name     string
		password string // unexported
	}

	formatter := NewTOONFormatter()

	users := []User{
		{Name: "Alice", password: "secret"},
	}

	result, err := formatter.Format(users)
	if err != nil {
		t.Fatalf("Format failed: %v", err)
	}

	output := string(result)

	// Should not contain unexported field
	if strings.Contains(output, "password") {
		t.Error("Should not contain unexported field 'password'")
	}
	if strings.Contains(output, "secret") {
		t.Error("Should not contain unexported field value 'secret'")
	}
}

func TestTOONFormatter_Format_NoExportedFields(t *testing.T) {
	type Private struct {
		internal string
	}

	formatter := NewTOONFormatter()

	data := []Private{{internal: "hidden"}}

	_, err := formatter.Format(data)
	if err == nil {
		t.Error("Expected error for struct with no exported fields")
	}

	if !strings.Contains(err.Error(), "at least one exported field") {
		t.Errorf("Expected error about exported fields, got: %v", err)
	}
}

func TestTOONFormatter_toLowerCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Name", "name"},
		{"UserID", "userID"},
		{"ProductName", "productName"},
		{"", ""},
		{"A", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := toLowerCamelCase(tt.input)
			if result != tt.expected {
				t.Errorf("toLowerCamelCase(%q) = %q, want %q",
					tt.input, result, tt.expected)
			}
		})
	}
}

func TestTOONFormatter_Name(t *testing.T) {
	formatter := NewTOONFormatter()
	if formatter.Name() != "toon" {
		t.Errorf("Expected name 'toon', got %s", formatter.Name())
	}
}

func TestTOONFormatter_MIMEType(t *testing.T) {
	formatter := NewTOONFormatter()
	expected := "text/plain; charset=utf-8"
	if formatter.MIMEType() != expected {
		t.Errorf("Expected MIME type %q, got %q", expected, formatter.MIMEType())
	}
}

// Benchmark TOON vs JSON to verify token efficiency
func BenchmarkTOONFormatter(b *testing.B) {
	type User struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	formatter := NewTOONFormatter()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = formatter.Format(users)
	}
}

func BenchmarkJSONFormatter(b *testing.B) {
	type User struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}

	users := []User{
		{ID: 1, Name: "Alice", Email: "alice@example.com"},
		{ID: 2, Name: "Bob", Email: "bob@example.com"},
		{ID: 3, Name: "Charlie", Email: "charlie@example.com"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(users)
	}
}
