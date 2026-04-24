package codegen

import (
	"fmt"
	"strings"
)

// OpIR is the intermediate representation of a single operation,
// ready for code generation.
type OpIR struct {
	Op             Op        // The original Op definition
	VarName        string    // PascalCase name for generated identifiers (e.g., "ListSessions")
	RequestType    string    // Go type name of the request struct
	ResponseType   string    // Go type name of the response struct
	HandlerFunc    string    // Function name to call
	Fields         []FieldIR // Parsed fields from the request struct
	MCPDescription string    // MCP-specific description (falls back to Op.Description)
}

// FieldIR is the intermediate representation of a single request field.
type FieldIR struct {
	GoName       string   // Go struct field name (e.g., "Status")
	GoType       string   // Go type (e.g., "string", "int", "[]string")
	JSONName     string   // JSON field name from json tag
	FlagName     string   // CLI flag name (snake_case with - replacing _)
	FlagType     string   // Cobra flag getter type (e.g., "String", "Int", "Bool")
	Description  string   // Human-readable description from desc tag
	Tag          FieldTag // Parsed ef:"..." tag
	IsPositional bool     // True if this is a positional CLI arg
	Pos          int      // Positional index (-1 if flag)
	Required     bool     // From ef tag
	Hidden       bool     // From ef tag
	Default      string   // Default value string
	Enum         []string // Enum values
	OmitCLI      bool     // Omitted from CLI surface
	OmitMCP      bool     // Omitted from MCP surface
	OmitSkill    bool     // Omitted from Skill surface
}

// --- OpIR accessor methods ---

// OpName returns the Op name.
func (o OpIR) OpName() string { return o.Op.Name }

// OpDescription returns the Op description.
func (o OpIR) OpDescription() string { return o.Op.Description }

// CLI returns the CLI surface config.
func (o OpIR) CLI() *CLISurface { return o.Op.CLI }

// MCP returns the MCP surface config.
func (o OpIR) MCP() *MCPSurface { return o.Op.MCP }

// Skill returns the Skill surface config.
func (o OpIR) Skill() *SkillSurface { return o.Op.Skill }

// IsManualSkill returns whether the skill is manually maintained.
func (o OpIR) IsManualSkill() bool { return o.Op.ManualSkill }

// IsDeprecated returns whether the op is deprecated.
func (o OpIR) IsDeprecated() bool { return o.Op.Deprecated }

// DeprecationMsg returns the deprecation message.
func (o OpIR) DeprecationMsg() string { return o.Op.DeprecatedMsg }

// IsStub returns whether the op is a stub.
func (o OpIR) IsStub() bool { return o.Op.Stub }

// --- Field filtering methods ---

// CLIFields returns fields visible on the CLI surface.
func (o OpIR) CLIFields() []FieldIR {
	var fields []FieldIR
	for _, f := range o.Fields {
		if !f.OmitCLI {
			fields = append(fields, f)
		}
	}
	return fields
}

// MCPFields returns fields visible on the MCP surface.
func (o OpIR) MCPFields() []FieldIR {
	var fields []FieldIR
	for _, f := range o.Fields {
		if !f.OmitMCP {
			fields = append(fields, f)
		}
	}
	return fields
}

// SkillFields returns fields visible on the Skill surface.
// Hidden fields are also excluded from Skills.
func (o OpIR) SkillFields() []FieldIR {
	var fields []FieldIR
	for _, f := range o.Fields {
		if !f.OmitSkill && !f.Hidden {
			fields = append(fields, f)
		}
	}
	return fields
}

// PositionalFields returns fields that are positional CLI args, sorted by position.
func (o OpIR) PositionalFields() []FieldIR {
	var fields []FieldIR
	for _, f := range o.Fields {
		if f.IsPositional {
			fields = append(fields, f)
		}
	}
	// Sort by Pos index (insertion sort; N is small).
	for i := 1; i < len(fields); i++ {
		for j := i; j > 0 && fields[j].Pos < fields[j-1].Pos; j-- {
			fields[j], fields[j-1] = fields[j-1], fields[j]
		}
	}
	return fields
}

// FlagFields returns fields that are flags (not positional), visible on CLI.
func (o OpIR) FlagFields() []FieldIR {
	var fields []FieldIR
	for _, f := range o.Fields {
		if !f.IsPositional && !f.OmitCLI {
			fields = append(fields, f)
		}
	}
	return fields
}

// --- FieldIR methods ---

// FlagRegistration returns the Cobra flag registration call string.
// Examples:
//
//	StringP("status", "s", "active", "Filter by status")
//	IntP("limit", "n", 100, "Max results")
//	Bool("dry-run", false, "Preview without executing")
//	StringSlice("fields", nil, "Field mask")
func (f FieldIR) FlagRegistration() string {
	def := f.defaultValue()
	desc := f.Description
	if len(f.Enum) > 0 {
		desc += " (allowed: " + strings.Join(f.Enum, ", ") + ")"
	}
	hasShort := f.Tag.Short != ""

	// Resolve the flag type name. Prefer FlagType (always set by buildOpIR),
	// fall back to mapping GoType for manually constructed FieldIR values.
	ft := f.FlagType
	if ft == "" {
		ft = flagTypeName(f.GoType)
	}

	isSlice := strings.HasSuffix(ft, "Slice")

	if hasShort {
		if isSlice {
			return fmt.Sprintf("%sP(%q, %q, nil, %q)", ft, f.FlagName, f.Tag.Short, desc)
		}
		return fmt.Sprintf("%sP(%q, %q, %s, %q)", ft, f.FlagName, f.Tag.Short, def, desc)
	}
	if isSlice {
		return fmt.Sprintf("%s(%q, nil, %q)", ft, f.FlagName, desc)
	}
	return fmt.Sprintf("%s(%q, %s, %q)", ft, f.FlagName, def, desc)
}

// defaultValue returns the default value literal for use in generated code.
func (f FieldIR) defaultValue() string {
	// Resolve type for dispatch. Use FlagType if GoType is empty.
	ft := f.FlagType
	if ft == "" {
		ft = flagTypeName(f.GoType)
	}

	if f.Default != "" {
		switch ft {
		case "String":
			return fmt.Sprintf("%q", f.Default)
		case "Int", "Int64", "Float64", "Bool":
			return f.Default
		default:
			return fmt.Sprintf("%q", f.Default)
		}
	}
	switch ft {
	case "String":
		return `""`
	case "Int", "Int64", "Float64":
		return "0"
	case "Bool":
		return "false"
	case "StringSlice", "IntSlice":
		return "nil"
	default:
		return `""`
	}
}

// EnumCSV returns a comma-separated list of enum values.
func (f FieldIR) EnumCSV() string {
	return strings.Join(f.Enum, ", ")
}
