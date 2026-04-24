package cliframe

import (
	"encoding/json"
)

// JSONFormatter implements OutputFormatter for JSON
type JSONFormatter struct {
	prettyPrint bool
	escapeHTML  bool
}

// NewJSONFormatter creates a JSON formatter
func NewJSONFormatter(prettyPrint bool) *JSONFormatter {
	return &JSONFormatter{
		prettyPrint: prettyPrint,
		escapeHTML:  false, // Don't escape HTML by default for CLI output
	}
}

// Format implements OutputFormatter.Format
func (f *JSONFormatter) Format(v interface{}) ([]byte, error) {
	if f.prettyPrint {
		return json.MarshalIndent(v, "", "  ")
	}
	return json.Marshal(v)
}

// Name implements OutputFormatter.Name
func (f *JSONFormatter) Name() string {
	return "json"
}

// MIMEType implements OutputFormatter.MIMEType
func (f *JSONFormatter) MIMEType() string {
	return "application/json"
}
