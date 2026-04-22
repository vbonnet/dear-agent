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

type Category string

const (
	CategoryRead     Category = "read"
	CategoryMutation Category = "mutation"
	CategoryMeta     Category = "meta"
)

func (c Category) Valid() bool {
	switch c {
	case CategoryRead, CategoryMutation, CategoryMeta:
		return true
	}
	return false
}

type CLISurface struct {
	CommandPath   string
	Use           string
	Aliases       []string
	Args          string
	OutputFormats []string
	Hidden        bool
}

type MCPSurface struct {
	ToolName    string
	Description string
}

type SkillSurface struct {
	SlashCommand string
	AllowedTools string
	ActionVerb   string
	OutputTable  []string
}
