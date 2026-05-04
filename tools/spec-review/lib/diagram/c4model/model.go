// Package c4model provides C4 Model semantic framework for architecture diagrams
package c4model

// Level represents the C4 Model abstraction level
type Level int

// C4 abstraction levels.
const (
	// LevelContext is the System Context level (highest).
	LevelContext Level = 1
	// LevelContainer is the Container level (apps and data stores).
	LevelContainer Level = 2
	// LevelComponent is the Component level (internal structure).
	LevelComponent Level = 3
	// LevelCode is the Code level (classes and functions, rarely used).
	LevelCode Level = 4
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

// C4 element type values for ElementType.
const (
	// TypePerson is a Level 1 (Context) human actor.
	TypePerson ElementType = "Person"
	// TypeSoftwareSystem is a Level 1 (Context) software system.
	TypeSoftwareSystem ElementType = "SoftwareSystem"
	// TypeExternalSystem is a Level 1 (Context) external software system.
	TypeExternalSystem ElementType = "ExternalSystem"

	// TypeContainer is a Level 2 (Container) deployable unit.
	TypeContainer ElementType = "Container"
	// TypeDatabase is a Level 2 (Container) data store.
	TypeDatabase ElementType = "Database"
	// TypeQueue is a Level 2 (Container) message queue.
	TypeQueue ElementType = "Queue"

	// TypeComponent is a Level 3 (Component) internal building block.
	TypeComponent ElementType = "Component"

	// TypeClass is a Level 4 (Code) class.
	TypeClass ElementType = "Class"
	// TypeInterface is a Level 4 (Code) interface.
	TypeInterface ElementType = "Interface"
)

// RelationshipType represents the type of relationship between elements
type RelationshipType string

// Relationship type values for RelationshipType.
const (
	// RelUses is a generic "uses" relationship.
	RelUses RelationshipType = "Uses"
	// RelReadsFrom indicates the source reads data from the destination.
	RelReadsFrom RelationshipType = "ReadsFrom"
	// RelWritesTo indicates the source writes data to the destination.
	RelWritesTo RelationshipType = "WritesTo"
	// RelSendsTo indicates the source sends messages to the destination.
	RelSendsTo RelationshipType = "SendsTo"
	// RelDependsOn indicates a build/runtime dependency.
	RelDependsOn RelationshipType = "DependsOn"
	// RelExtends indicates type extension (inheritance).
	RelExtends RelationshipType = "Extends"
	// RelImplements indicates the source implements the destination interface.
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
