package parser

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/tools/spec-review/lib/diagram/c4model"
)

// writeTemp writes content to a temp file with the given extension and returns
// its path. The file is cleaned up by t.Cleanup.
func writeTemp(t *testing.T, content, ext string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "diagram*"+ext)
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// ---------------------------------------------------------------------------
// New / unknown format
// ---------------------------------------------------------------------------

func TestNew_SetsFormat(t *testing.T) {
	p := New("d2")
	assert.Equal(t, "d2", p.Format)
}

func TestParse_UnsupportedFormat(t *testing.T) {
	p := New("xml")
	_, err := p.Parse(writeTemp(t, "", ".xml"))
	assert.ErrorContains(t, err, "unsupported format")
}

func TestParse_MissingFile(t *testing.T) {
	p := New("d2")
	_, err := p.Parse("/nonexistent/path/diagram.d2")
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// D2 parser
// ---------------------------------------------------------------------------

func TestParseD2_Empty(t *testing.T) {
	d, err := New("d2").Parse(writeTemp(t, "", ".d2"))
	require.NoError(t, err)
	assert.Empty(t, d.Elements)
	assert.Empty(t, d.Relationships)
}

func TestParseD2_CommentsOnly(t *testing.T) {
	content := "# This is a comment\n# Another comment\n"
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	assert.Empty(t, d.Elements)
}

func TestParseD2_PersonElement(t *testing.T) {
	// D2 person pattern requires shape: person on the same line as the {.
	content := "user: { shape: person\n  label: \"End User\"\n}\n"
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	require.Len(t, d.Elements, 1)
	assert.Equal(t, c4model.TypePerson, d.Elements[0].Type)
	assert.Equal(t, "user", d.Elements[0].ID)
}

func TestParseD2_SoftwareSystem(t *testing.T) {
	content := `api: {
  label: "API Service"
}`
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	require.Len(t, d.Elements, 1)
	assert.Equal(t, c4model.TypeSoftwareSystem, d.Elements[0].Type)
	assert.Equal(t, "API Service", d.Elements[0].Name)
}

func TestParseD2_Relationship(t *testing.T) {
	content := `user: {
  shape: person
}
api: {
  label: "API"
}
user -> api: "Calls"
`
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	assert.Len(t, d.Elements, 2)
	require.Len(t, d.Relationships, 1)
	assert.Equal(t, "Calls", d.Relationships[0].Description)
}

func TestParseD2_RelationshipUnknownElements(t *testing.T) {
	// When source/target IDs don't exist in the element map, the relationship
	// is silently dropped (no panic).
	content := "ghost1 -> ghost2: \"test\"\n"
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	assert.Empty(t, d.Relationships)
}

func TestParseD2_ContextLevel(t *testing.T) {
	content := `sys: {
  label: "My System"
}
`
	d, err := New("d2").Parse(writeTemp(t, content, ".d2"))
	require.NoError(t, err)
	assert.Equal(t, c4model.LevelContext, d.Level)
}

// ---------------------------------------------------------------------------
// Structurizr parser
// ---------------------------------------------------------------------------

func TestParseStructurizr_Empty(t *testing.T) {
	d, err := New("structurizr").Parse(writeTemp(t, "", ".dsl"))
	require.NoError(t, err)
	assert.Empty(t, d.Elements)
}

func TestParseStructurizr_CommentsSkipped(t *testing.T) {
	content := "// workspace comment\n# hash comment\n"
	d, err := New("structurizr").Parse(writeTemp(t, content, ".dsl"))
	require.NoError(t, err)
	assert.Empty(t, d.Elements)
}

func TestParseStructurizr_AllElementTypes(t *testing.T) {
	content := `u = person "Alice"
s = softwareSystem "Orders"
c = container "API"
comp = component "Handler"
`
	d, err := New("structurizr").Parse(writeTemp(t, content, ".dsl"))
	require.NoError(t, err)
	require.Len(t, d.Elements, 4)
	types := map[c4model.ElementType]bool{}
	for _, e := range d.Elements {
		types[e.Type] = true
	}
	assert.True(t, types[c4model.TypePerson])
	assert.True(t, types[c4model.TypeSoftwareSystem])
	assert.True(t, types[c4model.TypeContainer])
	assert.True(t, types[c4model.TypeComponent])
}

func TestParseStructurizr_Relationship(t *testing.T) {
	content := `u = person "User"
s = softwareSystem "System"
u -> s "Uses"
`
	d, err := New("structurizr").Parse(writeTemp(t, content, ".dsl"))
	require.NoError(t, err)
	require.Len(t, d.Relationships, 1)
	assert.Equal(t, "Uses", d.Relationships[0].Description)
}

func TestParseStructurizr_RelationshipUnknownElements(t *testing.T) {
	content := "ghost -> phantom \"link\"\n"
	d, err := New("structurizr").Parse(writeTemp(t, content, ".dsl"))
	require.NoError(t, err)
	assert.Empty(t, d.Relationships)
}

func TestParseStructurizr_ComponentLevel(t *testing.T) {
	content := "h = component \"Handler\"\n"
	d, err := New("structurizr").Parse(writeTemp(t, content, ".dsl"))
	require.NoError(t, err)
	assert.Equal(t, c4model.LevelComponent, d.Level)
}

// ---------------------------------------------------------------------------
// Mermaid parser
// ---------------------------------------------------------------------------

func TestParseMermaid_Empty(t *testing.T) {
	d, err := New("mermaid").Parse(writeTemp(t, "", ".mmd"))
	require.NoError(t, err)
	assert.Empty(t, d.Elements)
}

func TestParseMermaid_AllElementTypes(t *testing.T) {
	content := `C4Context
Person(u, "User")
System(s, "System")
C4Container
Container(c, "API")
Component(comp, "Handler")
`
	d, err := New("mermaid").Parse(writeTemp(t, content, ".mmd"))
	require.NoError(t, err)
	require.Len(t, d.Elements, 4)
}

func TestParseMermaid_Relationship(t *testing.T) {
	content := `Person(u, "User")
System(s, "System")
Rel(u, s, "Uses")
`
	d, err := New("mermaid").Parse(writeTemp(t, content, ".mmd"))
	require.NoError(t, err)
	require.Len(t, d.Relationships, 1)
	assert.Equal(t, "Uses", d.Relationships[0].Description)
	assert.Equal(t, c4model.RelUses, d.Relationships[0].Type)
}

func TestParseMermaid_RelationshipUnknownElements(t *testing.T) {
	content := "Rel(a, b, \"test\")\n"
	d, err := New("mermaid").Parse(writeTemp(t, content, ".mmd"))
	require.NoError(t, err)
	assert.Empty(t, d.Relationships)
}

// ---------------------------------------------------------------------------
// detectC4Level
// ---------------------------------------------------------------------------

func TestDetectC4Level_Empty(t *testing.T) {
	assert.Equal(t, c4model.LevelContext, detectC4Level(nil))
}

func TestDetectC4Level_PersonOnly(t *testing.T) {
	elems := []*c4model.Element{{Type: c4model.TypePerson}}
	assert.Equal(t, c4model.LevelContext, detectC4Level(elems))
}

func TestDetectC4Level_ContainerTakesPrecedenceOverSystem(t *testing.T) {
	elems := []*c4model.Element{
		{Type: c4model.TypeSoftwareSystem},
		{Type: c4model.TypeContainer},
	}
	assert.Equal(t, c4model.LevelContainer, detectC4Level(elems))
}

func TestDetectC4Level_ComponentTakesHighestPrecedence(t *testing.T) {
	elems := []*c4model.Element{
		{Type: c4model.TypeSoftwareSystem},
		{Type: c4model.TypeContainer},
		{Type: c4model.TypeComponent},
	}
	assert.Equal(t, c4model.LevelComponent, detectC4Level(elems))
}

// ---------------------------------------------------------------------------
// DetectFormat
// ---------------------------------------------------------------------------

func TestDetectFormat_ByExtension(t *testing.T) {
	assert.Equal(t, "d2", DetectFormat(writeTemp(t, "", ".d2")))
	assert.Equal(t, "structurizr", DetectFormat(writeTemp(t, "", ".dsl")))
	assert.Equal(t, "mermaid", DetectFormat(writeTemp(t, "", ".mmd")))
	assert.Equal(t, "mermaid", DetectFormat(writeTemp(t, "", ".mermaid")))
}

func TestDetectFormat_ByContent(t *testing.T) {
	assert.Equal(t, "structurizr", DetectFormat(writeTemp(t, "workspace {\n}", ".txt")))
	assert.Equal(t, "mermaid", DetectFormat(writeTemp(t, "C4Context\n", ".txt")))
	assert.Equal(t, "d2", DetectFormat(writeTemp(t, "shape: person\n", ".txt")))
}

func TestDetectFormat_Unknown(t *testing.T) {
	assert.Equal(t, "unknown", DetectFormat(writeTemp(t, "no clues here", ".txt")))
}

func TestDetectFormat_MissingFile(t *testing.T) {
	// Should return "unknown" when the file cannot be read.
	result := DetectFormat(filepath.Join(t.TempDir(), "missing.txt"))
	assert.Equal(t, "unknown", result)
}
