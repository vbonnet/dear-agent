package cliframe

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
)

// Alignment represents column alignment
type Alignment int

const (
	// AlignLeft aligns text to the left
	AlignLeft Alignment = iota
	// AlignRight aligns text to the right
	AlignRight
	// AlignCenter centers text
	AlignCenter
)

// BorderStyle represents table border style
type BorderStyle int

const (
	// BorderASCII uses ASCII characters (+--+--+)
	BorderASCII BorderStyle = iota
	// BorderUnicode uses Unicode box-drawing characters (┌──┬──┐)
	BorderUnicode
	// BorderNone renders no borders (compact mode)
	BorderNone
)

// TableOptions configures table rendering
type TableOptions struct {
	Headers     []string    // Column headers (auto-detected if empty)
	Alignments  []Alignment // Column alignments (auto-detected if empty)
	MaxColWidth int         // Maximum column width (truncate with ...)
	BorderStyle BorderStyle // Border style (ascii, unicode, none)
}

// TableFormatter implements OutputFormatter for human-readable tables
type TableFormatter struct {
	maxWidth     int
	colorEnabled bool
	compact      bool
}

// NewTableFormatter creates a table formatter
func NewTableFormatter(opts ...FormatterOption) *TableFormatter {
	config := &formatterConfig{
		colorEnabled: true,
		maxWidth:     0,
		compact:      false,
	}

	for _, opt := range opts {
		opt(config)
	}

	return &TableFormatter{
		maxWidth:     config.maxWidth,
		colorEnabled: config.colorEnabled,
		compact:      config.compact,
	}
}

// Format implements OutputFormatter.Format
// Expects []struct or []map[string]interface{}
func (f *TableFormatter) Format(v interface{}) ([]byte, error) {
	val := reflect.ValueOf(v)

	// Must be a slice
	if val.Kind() != reflect.Slice {
		return nil, fmt.Errorf("table format requires slice, got %T", v)
	}

	if val.Len() == 0 {
		return []byte("No results\n"), nil
	}

	// Get element type
	elemType := val.Type().Elem()
	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	// Extract headers and rows
	var headers []string
	var rows [][]string

	switch elemType.Kind() { //nolint:exhaustive // reflect.Kind has too many values; default handles the rest
	case reflect.Struct:
		headers, rows = f.formatStructSlice(val)
	case reflect.Map:
		headers, rows = f.formatMapSlice(val)
	default:
		return nil, fmt.Errorf("table format requires slice of structs or maps, got slice of %s", elemType.Kind())
	}

	// Render table
	return f.renderTable(headers, rows), nil
}

// formatStructSlice extracts headers and rows from slice of structs
func (f *TableFormatter) formatStructSlice(val reflect.Value) ([]string, [][]string) {
	if val.Len() == 0 {
		return nil, nil
	}

	// Get first element to extract field names
	firstElem := val.Index(0)
	if firstElem.Kind() == reflect.Pointer {
		firstElem = firstElem.Elem()
	}

	elemType := firstElem.Type()
	var headers []string

	// Extract field names
	for i := 0; i < elemType.NumField(); i++ {
		field := elemType.Field(i)
		if !field.IsExported() {
			continue
		}

		// Use json tag if available, otherwise field name
		name := field.Name
		if tag := field.Tag.Get("json"); tag != "" && tag != "-" {
			// Parse json tag (handle "name,omitempty" format)
			if idx := strings.Index(tag, ","); idx > 0 {
				name = tag[:idx]
			} else {
				name = tag
			}
		}
		headers = append(headers, name)
	}

	// Extract rows
	rows := make([][]string, val.Len())
	for i := 0; i < val.Len(); i++ {
		elem := val.Index(i)
		if elem.Kind() == reflect.Pointer {
			elem = elem.Elem()
		}

		row := make([]string, len(headers))
		fieldIdx := 0
		for j := 0; j < elem.NumField(); j++ {
			if !elem.Type().Field(j).IsExported() {
				continue
			}
			row[fieldIdx] = f.formatValue(elem.Field(j))
			fieldIdx++
		}
		rows[i] = row
	}

	return headers, rows
}

// formatMapSlice extracts headers and rows from slice of maps
func (f *TableFormatter) formatMapSlice(val reflect.Value) ([]string, [][]string) {
	if val.Len() == 0 {
		return nil, nil
	}

	// Get headers from first map
	firstMap := val.Index(0)
	keys := firstMap.MapKeys()
	headers := make([]string, len(keys))
	for i, key := range keys {
		headers[i] = fmt.Sprintf("%v", key.Interface())
	}

	// Extract rows
	rows := make([][]string, val.Len())
	for i := 0; i < val.Len(); i++ {
		mapVal := val.Index(i)
		row := make([]string, len(headers))
		for j, header := range headers {
			value := mapVal.MapIndex(reflect.ValueOf(header))
			if value.IsValid() {
				row[j] = fmt.Sprintf("%v", value.Interface())
			}
		}
		rows[i] = row
	}

	return headers, rows
}

// formatValue converts a reflect.Value to string
func (f *TableFormatter) formatValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	switch v.Kind() { //nolint:exhaustive // reflect.Kind has too many values; default handles the rest
	case reflect.Pointer:
		if v.IsNil() {
			return ""
		}
		return f.formatValue(v.Elem())
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.2f", v.Float())
	default:
		return fmt.Sprintf("%v", v.Interface())
	}
}

// renderTable renders headers and rows as ASCII table
func (f *TableFormatter) renderTable(headers []string, rows [][]string) []byte {
	var buf bytes.Buffer

	// Calculate column widths
	colWidths := f.calculateColumnWidths(headers, rows)

	// Render table components
	f.renderTopBorder(&buf, colWidths)
	f.renderHeaderRow(&buf, headers, colWidths)
	f.renderHeaderSeparator(&buf, colWidths)
	f.renderDataRows(&buf, rows, colWidths)
	f.renderBottomBorder(&buf, colWidths)

	return buf.Bytes()
}

// calculateColumnWidths computes width for each column
func (f *TableFormatter) calculateColumnWidths(headers []string, rows [][]string) []int {
	colWidths := make([]int, len(headers))

	// Start with header widths
	for i, header := range headers {
		colWidths[i] = len(header)
	}

	// Update with row data widths
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Apply max width if set
	if f.maxWidth > 0 {
		for i := range colWidths {
			if colWidths[i] > f.maxWidth {
				colWidths[i] = f.maxWidth
			}
		}
	}

	return colWidths
}

// renderTopBorder renders the top border (compact mode skips this)
func (f *TableFormatter) renderTopBorder(buf *bytes.Buffer, colWidths []int) {
	if f.compact {
		return
	}
	buf.WriteString("+")
	for _, width := range colWidths {
		buf.WriteString(strings.Repeat("-", width+2))
		buf.WriteString("+")
	}
	buf.WriteString("\n")
}

// renderHeaderRow renders the header row
func (f *TableFormatter) renderHeaderRow(buf *bytes.Buffer, headers []string, colWidths []int) {
	buf.WriteString("|")
	for i, header := range headers {
		cell := f.truncate(header, colWidths[i])
		buf.WriteString(" ")
		buf.WriteString(f.pad(cell, colWidths[i], AlignLeft))
		buf.WriteString(" |")
	}
	buf.WriteString("\n")
}

// renderHeaderSeparator renders the separator between header and data
func (f *TableFormatter) renderHeaderSeparator(buf *bytes.Buffer, colWidths []int) {
	if f.compact {
		return
	}
	buf.WriteString("+")
	for _, width := range colWidths {
		buf.WriteString(strings.Repeat("=", width+2))
		buf.WriteString("+")
	}
	buf.WriteString("\n")
}

// renderDataRows renders all data rows
func (f *TableFormatter) renderDataRows(buf *bytes.Buffer, rows [][]string, colWidths []int) {
	for _, row := range rows {
		buf.WriteString("|")
		for i, cell := range row {
			if i < len(colWidths) {
				truncated := f.truncate(cell, colWidths[i])
				buf.WriteString(" ")
				align := f.getCellAlignment(cell)
				buf.WriteString(f.pad(truncated, colWidths[i], align))
				buf.WriteString(" |")
			}
		}
		buf.WriteString("\n")
	}
}

// getCellAlignment determines alignment based on cell content
func (f *TableFormatter) getCellAlignment(cell string) Alignment {
	if isNumeric(cell) {
		return AlignRight
	}
	return AlignLeft
}

// renderBottomBorder renders the bottom border (compact mode skips this)
func (f *TableFormatter) renderBottomBorder(buf *bytes.Buffer, colWidths []int) {
	if f.compact {
		return
	}
	buf.WriteString("+")
	for _, width := range colWidths {
		buf.WriteString(strings.Repeat("-", width+2))
		buf.WriteString("+")
	}
	buf.WriteString("\n")
}

// truncate truncates string to maxWidth, appending "..." if truncated
func (f *TableFormatter) truncate(s string, maxWidth int) string {
	if len(s) <= maxWidth {
		return s
	}
	if maxWidth < 3 {
		return s[:maxWidth]
	}
	return s[:maxWidth-3] + "..."
}

// pad pads string to width with alignment
func (f *TableFormatter) pad(s string, width int, align Alignment) string {
	if len(s) >= width {
		return s
	}

	padding := width - len(s)
	switch align {
	case AlignLeft:
		return s + strings.Repeat(" ", padding)
	case AlignRight:
		return strings.Repeat(" ", padding) + s
	case AlignCenter:
		leftPad := padding / 2
		rightPad := padding - leftPad
		return strings.Repeat(" ", leftPad) + s + strings.Repeat(" ", rightPad)
	default:
		return s
	}
}

// isNumeric checks if string represents a number
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			if c != '.' && c != '-' && c != '+' {
				return false
			}
		}
	}
	return true
}

// Name implements OutputFormatter.Name
func (f *TableFormatter) Name() string {
	return "table"
}

// MIMEType implements OutputFormatter.MIMEType
func (f *TableFormatter) MIMEType() string {
	return "text/plain"
}
