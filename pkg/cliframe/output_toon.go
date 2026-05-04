package cliframe

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// TOONFormatter implements OutputFormatter for TOON format
// (Token-Oriented Object Notation)
//
// TOON is a compact format designed for LLM token efficiency:
//   - Array header: arrayName[count]{field1,field2,...}
//   - Data rows: CSV-like format
//   - 35-40% fewer tokens than JSON
//
// Example:
//
//	users[3]{id,name,email}
//	1,Alice,alice@example.com
//	2,Bob,bob@example.com
//	3,Charlie,charlie@example.com
type TOONFormatter struct {
	quoteStrings bool // Quote all strings (default: smart quoting via CSV)
}

// NewTOONFormatter creates a TOON formatter
func NewTOONFormatter() *TOONFormatter {
	return &TOONFormatter{
		quoteStrings: false,
	}
}

// Format implements OutputFormatter.Format
// Requires []struct with uniform fields
func (f *TOONFormatter) Format(v interface{}) ([]byte, error) {
	val := reflect.ValueOf(v)

	// Must be a slice
	if val.Kind() != reflect.Slice {
		return nil, fmt.Errorf("TOON format requires slice, got %T", v)
	}

	if val.Len() == 0 {
		return []byte{}, nil
	}

	// Get element type (must be struct)
	elemType := val.Type().Elem()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return nil, fmt.Errorf("TOON format requires slice of structs, got slice of %s", elemType.Kind())
	}

	// Extract field metadata
	fields := f.extractFields(elemType)
	if len(fields) == 0 {
		return nil, fmt.Errorf("TOON format requires at least one exported field")
	}

	// Build output
	var buf bytes.Buffer

	// Header: arrayName[count]{field1,field2,...}
	arrayName := f.getArrayName(elemType)
	fmt.Fprintf(&buf, "%s[%d]{%s}\n",
		arrayName,
		val.Len(),
		f.joinFields(fields))

	// Data rows (CSV format)
	csvWriter := csv.NewWriter(&buf)

	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		if elem.Kind() == reflect.Pointer {
			if elem.IsNil() {
				continue // Skip nil pointers
			}
			elem = elem.Elem()
		}

		row := make([]string, len(fields))
		for j, field := range fields {
			fieldVal := elem.FieldByName(field.StructName)
			row[j] = f.formatValue(fieldVal)
		}

		if err := csvWriter.Write(row); err != nil {
			return nil, fmt.Errorf("TOON format error: %w", err)
		}
	}

	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		return nil, fmt.Errorf("TOON format error: %w", err)
	}

	return buf.Bytes(), nil
}

// Field represents a struct field with metadata
type Field struct {
	StructName string // Go struct field name
	TOONName   string // Name in TOON output (from tag or field name)
}

// extractFields extracts exported fields from struct type
func (f *TOONFormatter) extractFields(t reflect.Type) []Field {
	fields := make([]Field, 0, t.NumField())

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Check for toon tag
		toonName := field.Tag.Get("toon")
		if toonName == "-" {
			continue // Skip field with toon:"-"
		}

		if toonName == "" {
			// Use json tag if available, otherwise field name
			if jsonTag := field.Tag.Get("json"); jsonTag != "" && jsonTag != "-" {
				// Parse json tag (handle "name,omitempty" format)
				if idx := strings.Index(jsonTag, ","); idx > 0 {
					toonName = jsonTag[:idx]
				} else {
					toonName = jsonTag
				}
			} else {
				// Convert field name to lowerCamelCase
				toonName = toLowerCamelCase(field.Name)
			}
		}

		fields = append(fields, Field{
			StructName: field.Name,
			TOONName:   toonName,
		})
	}

	return fields
}

// getArrayName derives array name from struct type
func (f *TOONFormatter) getArrayName(t reflect.Type) string {
	name := t.Name()
	if name == "" {
		name = "items"
	}

	// Convert to lowerCamelCase and pluralize
	name = toLowerCamelCase(name)

	// Simple pluralization (add 's')
	// TODO: Handle irregular plurals (person→people, etc.)
	if !strings.HasSuffix(name, "s") {
		name += "s"
	}

	return name
}

// joinFields joins field names with commas
func (f *TOONFormatter) joinFields(fields []Field) string {
	names := make([]string, len(fields))
	for i, field := range fields {
		names[i] = field.TOONName
	}
	return strings.Join(names, ",")
}

// formatValue converts a reflect.Value to string for CSV
func (f *TOONFormatter) formatValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	switch v.Kind() { //nolint:exhaustive // reflect.Kind has too many values; default handles the rest
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10)
	case reflect.Float32, reflect.Float64:
		// Use 'f' format to avoid scientific notation
		return strconv.FormatFloat(v.Float(), 'f', -1, 64)
	case reflect.String:
		return v.String() // CSV writer handles quoting
	case reflect.Pointer:
		if v.IsNil() {
			return ""
		}
		return f.formatValue(v.Elem())
	case reflect.Struct:
		// For nested structs, use JSON representation
		// This is a fallback - TOON works best with flat structs
		return fmt.Sprintf("%+v", v.Interface())
	default:
		// For complex types, use fmt.Sprintf
		return fmt.Sprintf("%v", v.Interface())
	}
}

// toLowerCamelCase converts "FieldName" to "fieldName"
func toLowerCamelCase(s string) string {
	if s == "" {
		return ""
	}
	// Convert first character to lowercase
	runes := []rune(s)
	runes[0] += 32 // ASCII lowercase conversion
	return string(runes)
}

// Name implements OutputFormatter.Name
func (f *TOONFormatter) Name() string {
	return "toon"
}

// MIMEType implements OutputFormatter.MIMEType
func (f *TOONFormatter) MIMEType() string {
	return "text/plain; charset=utf-8"
}
