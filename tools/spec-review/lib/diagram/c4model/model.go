// Package c4model provides C4 Model semantic framework for architecture diagrams
package c4model

// Level represents the C4 Model abstraction level
type Level int

const (
	LevelContext   Level = 1 // System Context - highest level
	LevelContainer Level = 2 // Container - apps and data stores
	LevelComponent Level = 3 // Component - internal structure
	LevelCode      Level = 4 // Code - classes and functions (rarely used)
)

func (l Level) String() string {
	switch l {
	case LevelContext:
		return "Context"
	case LevelContainer:
		return "Container"
	case LevelComponent:
		return "Component"
	case LevelCode:
		return "Code"
	default:
		return "Unknown"
	}
}

// ElementType represents the type of C4 element
type ElementType string

const (
	// Level 1 (Context) elements
	TypePerson         ElementType = "Person"
	TypeSoftwareSystem ElementType = "SoftwareSystem"
	TypeExternalSystem ElementType = "ExternalSystem"

	// Level 2 (Container) elements
	TypeContainer ElementType = "Container"
	TypeDatabase  ElementType = "Database"
	TypeQueue     ElementType = "Queue"

	// Level 3 (Component) elements
	TypeComponent ElementType = "Component"

	// Level 4 (Code) elements
	TypeClass     ElementType = "Class"
	TypeInterface ElementType = "Interface"
)

// RelationshipType represents the type of relationship between elements
type RelationshipType string

const (
	RelUses       RelationshipType = "Uses"
	RelReadsFrom  RelationshipType = "ReadsFrom"
	RelWritesTo   RelationshipType = "WritesTo"
	RelSendsTo    RelationshipType = "SendsTo"
	RelDependsOn  RelationshipType = "DependsOn"
	RelExtends    RelationshipType = "Extends"
	RelImplements RelationshipType = "Implements"
)

// Element represents a C4 model element
type Element struct {
	ID          string
	Name        string
	Type        ElementType
	Description string
	Technology  string
	Tags        []string
	Parent      *Element // For nested elements
	Children    []*Element
}

// Relationship represents a connection between two elements
type Relationship struct {
	ID          string
	Source      *Element
	Destination *Element
	Type        RelationshipType
	Description string
	Technology  string
	Tags        []string
}

// Diagram represents a C4 diagram
type Diagram struct {
	Name          string
	Description   string
	Level         Level
	Elements      []*Element
	Relationships []*Relationship
	Focus         *Element // The system/container being detailed
}

// AllowedElements returns the element types allowed at each C4 level
func AllowedElements(level Level) []ElementType {
	switch level {
	case LevelContext:
		return []ElementType{TypePerson, TypeSoftwareSystem, TypeExternalSystem}
	case LevelContainer:
		return []ElementType{TypeContainer, TypeDatabase, TypeQueue}
	case LevelComponent:
		return []ElementType{TypeComponent}
	case LevelCode:
		return []ElementType{TypeClass, TypeInterface}
	default:
		return []ElementType{}
	}
}

// AllowedRelationships returns the relationship types allowed at each C4 level
func AllowedRelationships(level Level) []RelationshipType {
	switch level {
	case LevelContext:
		return []RelationshipType{RelUses}
	case LevelContainer:
		return []RelationshipType{RelUses, RelReadsFrom, RelWritesTo, RelSendsTo}
	case LevelComponent:
		return []RelationshipType{RelUses, RelDependsOn}
	case LevelCode:
		return []RelationshipType{RelDependsOn, RelExtends, RelImplements}
	default:
		return []RelationshipType{}
	}
}

// IsElementAllowed checks if an element type is allowed at a given level
func IsElementAllowed(elementType ElementType, level Level) bool {
	allowed := AllowedElements(level)
	for _, t := range allowed {
		if t == elementType {
			return true
		}
	}
	return false
}

// IsRelationshipAllowed checks if a relationship type is allowed at a given level
func IsRelationshipAllowed(relType RelationshipType, level Level) bool {
	allowed := AllowedRelationships(level)
	for _, t := range allowed {
		if t == relType {
			return true
		}
	}
	return false
}
