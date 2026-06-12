package codegen

import (
	"reflect"
	"testing"
)

// --- ParseFieldTag ---

func TestParseFieldTag_SimpleField(t *testing.T) {
	ft, err := ParseFieldTag("session_id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.Name != "session_id" {
		t.Errorf("Name = %q, want session_id", ft.Name)
	}
	if ft.Pos != -1 {
		t.Errorf("Pos = %d, want -1", ft.Pos)
	}
}

func TestParseFieldTag_PositionalArg(t *testing.T) {
	ft, err := ParseFieldTag("run_id,pos=0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.Pos != 0 {
		t.Errorf("Pos = %d, want 0", ft.Pos)
	}
}

func TestParseFieldTag_Required(t *testing.T) {
	ft, err := ParseFieldTag("name,required")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ft.Required {
		t.Error("Required should be true")
	}
}

func TestParseFieldTag_EnumValues(t *testing.T) {
	ft, err := ParseFieldTag("state,enum=pending|running|done")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"pending", "running", "done"}
	if !reflect.DeepEqual(ft.Enum, want) {
		t.Errorf("Enum = %v, want %v", ft.Enum, want)
	}
}

func TestParseFieldTag_DefaultValue(t *testing.T) {
	ft, err := ParseFieldTag("limit,default=50")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.Default != "50" {
		t.Errorf("Default = %q, want 50", ft.Default)
	}
}

func TestParseFieldTag_OmitSurfaces(t *testing.T) {
	ft, err := ParseFieldTag("internal,omit=cli|mcp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ft.Omit["cli"] {
		t.Error("should omit cli")
	}
	if !ft.Omit["mcp"] {
		t.Error("should omit mcp")
	}
	if ft.Omit["skill"] {
		t.Error("should not omit skill")
	}
}

func TestParseFieldTag_Hidden(t *testing.T) {
	ft, err := ParseFieldTag("token,hidden")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ft.Hidden {
		t.Error("Hidden should be true")
	}
}

func TestParseFieldTag_ShortFlag(t *testing.T) {
	ft, err := ParseFieldTag("verbose,short=v")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ft.Short != "v" {
		t.Errorf("Short = %q, want v", ft.Short)
	}
}

func TestParseFieldTag_EmptyTag(t *testing.T) {
	_, err := ParseFieldTag("")
	if err == nil {
		t.Error("expected error for empty tag")
	}
}

func TestParseFieldTag_EmptyName(t *testing.T) {
	_, err := ParseFieldTag(",required")
	if err == nil {
		t.Error("expected error for empty field name")
	}
}

func TestParseFieldTag_UnknownOption(t *testing.T) {
	_, err := ParseFieldTag("field,unknown=val")
	if err == nil {
		t.Error("expected error for unknown option")
	}
}

func TestParseFieldTag_PosRequiresValue(t *testing.T) {
	_, err := ParseFieldTag("field,pos")
	if err == nil {
		t.Error("expected error when pos has no value")
	}
}

func TestParseFieldTag_PosNonInteger(t *testing.T) {
	_, err := ParseFieldTag("field,pos=abc")
	if err == nil {
		t.Error("expected error when pos is not an integer")
	}
}

// --- snakeToPascal ---

func TestSnakeToPascal(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"list_sessions", "ListSessions"},
		{"session", "Session"},
		{"get_session_by_id", "GetSessionById"},
		{"", ""},
		{"_leading", "Leading"},
	}
	for _, tc := range cases {
		got := snakeToPascal(tc.in)
		if got != tc.want {
			t.Errorf("snakeToPascal(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// --- BuildOpIR ---

type testRequest struct {
	SessionID string `ef:"session_id,pos=0" json:"session_id" desc:"session identifier"`
	Limit     int    `ef:"limit,default=10" json:"limit" desc:"max results"`
	Internal  string `ef:"internal,omit=cli" json:"internal" desc:"internal field"`
}

func TestBuildOpIR_Fields(t *testing.T) {
	op := Op{
		Name:        "list_sessions",
		Description: "List sessions",
		RequestType: "TestRequest",
	}
	reqTypes := map[string]reflect.Type{
		"TestRequest": reflect.TypeFor[testRequest](),
	}
	ir, err := BuildOpIR(op, reqTypes, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}
	if ir.VarName != "ListSessions" {
		t.Errorf("VarName = %q, want ListSessions", ir.VarName)
	}
	if len(ir.Fields) != 3 {
		t.Fatalf("want 3 fields, got %d", len(ir.Fields))
	}
}

func TestBuildOpIR_CLIFieldsOmitsInternal(t *testing.T) {
	op := Op{
		Name:        "list_sessions",
		Description: "List sessions",
		RequestType: "TestRequest",
	}
	reqTypes := map[string]reflect.Type{
		"TestRequest": reflect.TypeFor[testRequest](),
	}
	ir, err := BuildOpIR(op, reqTypes, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}
	cliFields := ir.CLIFields()
	for _, f := range cliFields {
		if f.GoName == "Internal" {
			t.Error("CLIFields should not include the omit=cli field")
		}
	}
}

func TestBuildOpIR_PositionalFields(t *testing.T) {
	op := Op{
		Name:        "list_sessions",
		RequestType: "TestRequest",
	}
	reqTypes := map[string]reflect.Type{
		"TestRequest": reflect.TypeFor[testRequest](),
	}
	ir, err := BuildOpIR(op, reqTypes, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}
	pos := ir.PositionalFields()
	if len(pos) != 1 {
		t.Fatalf("want 1 positional field, got %d", len(pos))
	}
	if pos[0].GoName != "SessionID" {
		t.Errorf("positional field = %q, want SessionID", pos[0].GoName)
	}
}

func TestBuildOpIR_MissingRequestType(t *testing.T) {
	op := Op{
		Name:        "bad_op",
		RequestType: "Nonexistent",
	}
	_, err := BuildOpIR(op, map[string]reflect.Type{}, nil)
	if err == nil {
		t.Error("expected error for missing request type")
	}
}

func TestBuildOpIR_MCPDescriptionFallback(t *testing.T) {
	op := Op{
		Name:        "my_op",
		Description: "base description",
	}
	ir, err := BuildOpIR(op, nil, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}
	if ir.MCPDescription != "base description" {
		t.Errorf("MCPDescription = %q, want 'base description'", ir.MCPDescription)
	}
}

func TestBuildOpIR_MCPDescriptionOverride(t *testing.T) {
	op := Op{
		Name:        "my_op",
		Description: "base description",
		MCP:         &MCPSurface{Description: "mcp-specific"},
	}
	ir, err := BuildOpIR(op, nil, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}
	if ir.MCPDescription != "mcp-specific" {
		t.Errorf("MCPDescription = %q, want 'mcp-specific'", ir.MCPDescription)
	}
}

// --- Category.Valid ---

func TestCategoryValid(t *testing.T) {
	if !CategoryRead.Valid() {
		t.Error("CategoryRead should be valid")
	}
	if !CategoryMutation.Valid() {
		t.Error("CategoryMutation should be valid")
	}
	if !CategoryMeta.Valid() {
		t.Error("CategoryMeta should be valid")
	}
	if Category("bogus").Valid() {
		t.Error("bogus category should be invalid")
	}
}
