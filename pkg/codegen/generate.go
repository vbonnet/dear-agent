package codegen

import (
	"fmt"
	"os"
	"reflect"
	"strings"
)

// GenerateConfig configures the code generator.
type GenerateConfig struct {
	Ops           []Op                    // Operations to generate surfaces for
	RequestTypes  map[string]reflect.Type // Map of RequestType name -> reflect.Type
	ResponseTypes map[string]reflect.Type // Map of ResponseType name -> reflect.Type
	OutDir        string                  // Output directory
	Package       string                  // Package name for generated Go files
	CLIBinary     string                  // CLI binary name (e.g., "agm") for skill templates
	BuildIgnore   bool                    // If true, prepend //go:build ignore to generated Go files
}

// Generate produces CLI, MCP, Skill, and parity test files from Op definitions.
func Generate(cfg GenerateConfig) error {
	irs, err := BuildIRs(cfg.Ops, cfg.RequestTypes, cfg.ResponseTypes)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	if err := GenerateCLI(irs, cfg.OutDir, cfg.Package, cfg.BuildIgnore); err != nil {
		return fmt.Errorf("generating CLI: %w", err)
	}

	if err := GenerateMCP(irs, cfg.OutDir, cfg.Package, cfg.BuildIgnore); err != nil {
		return fmt.Errorf("generating MCP: %w", err)
	}

	if err := GenerateSkills(irs, cfg.OutDir); err != nil {
		return fmt.Errorf("generating Skills: %w", err)
	}

	if err := GenerateParity(irs, cfg.OutDir, cfg.Package, cfg.BuildIgnore); err != nil {
		return fmt.Errorf("generating Parity: %w", err)
	}

	return nil
}

// BuildIRs constructs OpIR slices from Ops and reflected types.
func BuildIRs(ops []Op, reqTypes, respTypes map[string]reflect.Type) ([]OpIR, error) {
	var irs []OpIR
	for _, op := range ops {
		ir, err := BuildOpIR(op, reqTypes, respTypes)
		if err != nil {
			return nil, fmt.Errorf("building IR for %s: %w", op.Name, err)
		}
		irs = append(irs, *ir)
	}
	return irs, nil
}

// BuildOpIR constructs an OpIR from an Op and reflected types.
func BuildOpIR(op Op, reqTypes, respTypes map[string]reflect.Type) (*OpIR, error) {
	ir := &OpIR{
		Op:           op,
		VarName:      snakeToPascal(op.Name),
		RequestType:  op.RequestType,
		ResponseType: op.ResponseType,
		HandlerFunc:  op.HandlerFunc,
	}

	// Set MCP description: prefer MCPSurface.Description, fall back to Op.Description.
	ir.MCPDescription = op.Description
	if op.MCP != nil && op.MCP.Description != "" {
		ir.MCPDescription = op.MCP.Description
	}

	// Parse request type fields via reflection.
	if op.RequestType != "" {
		rt, ok := reqTypes[op.RequestType]
		if !ok {
			return nil, fmt.Errorf("request type %q not found in RequestTypes map", op.RequestType)
		}
		fields, err := parseStructFields(rt)
		if err != nil {
			return nil, fmt.Errorf("parsing fields of %s: %w", op.RequestType, err)
		}
		ir.Fields = fields
	}

	return ir, nil
}

// parseStructFields extracts FieldIR values from a reflected struct type.
func parseStructFields(t reflect.Type) ([]FieldIR, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected struct type, got %s", t.Kind())
	}

	var fields []FieldIR
	for i := range t.NumField() {
		sf := t.Field(i)

		// Skip unexported fields.
		if !sf.IsExported() {
			continue
		}

		efTag := sf.Tag.Get("ef")
		if efTag == "" {
			continue // No ef tag means not a codegen-managed field.
		}

		parsed, err := ParseFieldTag(efTag)
		if err != nil {
			return nil, fmt.Errorf("field %s: %w", sf.Name, err)
		}

		desc := ParseDescTag(sf.Tag.Get("desc"))
		jsonName := jsonFieldName(sf)
		goType := goTypeName(sf.Type)
		flagName := strings.ReplaceAll(parsed.Name, "_", "-")

		fir := FieldIR{
			GoName:       sf.Name,
			GoType:       goType,
			JSONName:     jsonName,
			FlagName:     flagName,
			FlagType:     flagTypeName(goType),
			Description:  desc,
			Tag:          *parsed,
			IsPositional: parsed.Pos >= 0,
			Pos:          parsed.Pos,
			Required:     parsed.Required,
			Hidden:       parsed.Hidden,
			Default:      parsed.Default,
			Enum:         parsed.Enum,
			OmitCLI:      parsed.Omit["cli"],
			OmitMCP:      parsed.Omit["mcp"],
			OmitSkill:    parsed.Omit["skill"],
		}

		fields = append(fields, fir)
	}

	return fields, nil
}

// snakeToPascal converts a snake_case name to PascalCase.
// e.g., "list_sessions" -> "ListSessions"
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		b.WriteString(strings.ToUpper(p[:1]))
		b.WriteString(p[1:])
	}
	return b.String()
}

// jsonFieldName extracts the JSON field name from a struct field's json tag.
// Falls back to the Go field name if no json tag is present.
func jsonFieldName(sf reflect.StructField) string {
	tag := sf.Tag.Get("json")
	if tag == "" || tag == "-" {
		return sf.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return sf.Name
	}
	return name
}

// goTypeName returns a Go type name string from a reflect.Type.
func goTypeName(t reflect.Type) string {
	switch t.Kind() { //nolint:exhaustive // reflect.Kind has too many values; default handles the rest
	case reflect.Slice:
		return "[]" + goTypeName(t.Elem())
	case reflect.Ptr:
		return "*" + goTypeName(t.Elem())
	case reflect.Map:
		return "map[" + goTypeName(t.Key()) + "]" + goTypeName(t.Elem())
	default:
		return t.Name()
	}
}

// flagTypeName maps a Go type string to a Cobra flag getter suffix.
func flagTypeName(goType string) string {
	switch goType {
	case "string":
		return "String"
	case "int":
		return "Int"
	case "int64":
		return "Int64"
	case "bool":
		return "Bool"
	case "float64":
		return "Float64"
	case "[]string":
		return "StringSlice"
	case "[]int":
		return "IntSlice"
	default:
		return "String"
	}
}
