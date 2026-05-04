package codegen

// Op defines a single operation that can be exposed across multiple API surfaces.
type Op struct {
	Name          string
	Description   string
	Category      Category
	CLI           *CLISurface
	MCP           *MCPSurface
	Skill         *SkillSurface
	RequestType   string
	ResponseType  string
	HandlerFunc   string
	Deprecated    bool
	DeprecatedMsg string
	ManualSkill   bool
	Stub          bool
}

// Category groups operations by side-effect class (read/mutation/meta).
type Category string

// Recognized Category values.
const (
	// CategoryRead marks read-only operations.
	CategoryRead Category = "read"
	// CategoryMutation marks operations that mutate state.
	CategoryMutation Category = "mutation"
	// CategoryMeta marks meta/introspection operations.
	CategoryMeta Category = "meta"
)

// Valid reports whether c is a recognized Category value.
func (c Category) Valid() bool {
	switch c {
	case CategoryRead, CategoryMutation, CategoryMeta:
		return true
	}
	return false
}

// CLISurface describes how an Op is exposed as a CLI command.
type CLISurface struct {
	CommandPath   string
	Use           string
	Aliases       []string
	Args          string
	OutputFormats []string
	Hidden        bool
}

// MCPSurface describes how an Op is exposed as an MCP tool.
type MCPSurface struct {
	ToolName    string
	Description string
}

// SkillSurface describes how an Op is exposed as a Claude Skill.
type SkillSurface struct {
	SlashCommand string
	AllowedTools string
	ActionVerb   string
	OutputTable  []string
}
