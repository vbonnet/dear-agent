package c4model

import (
	"fmt"
)

// ValidationError represents a C4 compliance validation error
type ValidationError struct {
	Level   Level
	Message string
	Element *Element
	Rel     *Relationship
}

func (e *ValidationError) Error() string {
	if e.Element != nil {
		return fmt.Sprintf("[%s] %s (element: %s)", e.Level, e.Message, e.Element.Name)
	}
	if e.Rel != nil {
		return fmt.Sprintf("[%s] %s (relationship: %s -> %s)", e.Level, e.Message,
			e.Rel.Source.Name, e.Rel.Destination.Name)
	}
	return fmt.Sprintf("[%s] %s", e.Level, e.Message)
}

// ValidationResult contains the result of C4 validation
type ValidationResult struct {
	Valid  bool
	Errors []*ValidationError
	Score  float64 // 0-100 score
}

// Validator validates C4 model compliance
type Validator struct {
	strict bool // If true, treats warnings as errors
}

// NewValidator creates a new C4 validator
func NewValidator(strict bool) *Validator {
	return &Validator{strict: strict}
}

// Validate performs comprehensive C4 compliance validation
func (v *Validator) Validate(diagram *Diagram) *ValidationResult {
	result := &ValidationResult{
		Valid:  true,
		Errors: make([]*ValidationError, 0),
		Score:  100.0,
	}

	// Rule 1: Diagram must have a level
	if diagram.Level < LevelContext || diagram.Level > LevelCode {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   diagram.Level,
			Message: "Invalid C4 level (must be 1-4)",
		})
	}

	// Rule 2: Level 1 (Context) must have at least one Person and one Software System
	if diagram.Level == LevelContext {
		v.validateContextDiagram(diagram, result)
	}

	// Rule 3: Level 2 (Container) must have a parent Software System
	if diagram.Level == LevelContainer {
		v.validateContainerDiagram(diagram, result)
	}

	// Rule 4: Level 3 (Component) must have a parent Container
	if diagram.Level == LevelComponent {
		v.validateComponentDiagram(diagram, result)
	}

	// Rule 5: Validate all elements are appropriate for the level
	v.validateElements(diagram, result)

	// Rule 6: Validate all relationships are appropriate for the level
	v.validateRelationships(diagram, result)

	// Rule 7: Check for orphaned elements (no relationships)
	v.validateConnectivity(diagram, result)

	// Calculate final score and validity
	result.Valid = len(result.Errors) == 0
	result.Score = v.calculateScore(result)

	return result
}

func (v *Validator) validateContextDiagram(diagram *Diagram, result *ValidationResult) {
	hasPerson := false
	hasSystem := false

	for _, elem := range diagram.Elements {
		if elem.Type == TypePerson {
			hasPerson = true
		}
		if elem.Type == TypeSoftwareSystem {
			hasSystem = true
		}
	}

	if !hasPerson {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelContext,
			Message: "Context diagram must have at least one Person",
		})
	}

	if !hasSystem {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelContext,
			Message: "Context diagram must have at least one Software System",
		})
	}
}

func (v *Validator) validateContainerDiagram(diagram *Diagram, result *ValidationResult) {
	if diagram.Focus == nil {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelContainer,
			Message: "Container diagram must have a focus (parent Software System)",
		})
		return
	}

	if diagram.Focus.Type != TypeSoftwareSystem {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelContainer,
			Message: "Container diagram focus must be a Software System",
			Element: diagram.Focus,
		})
	}

	// All containers should belong to the focus system
	for _, elem := range diagram.Elements {
		if elem.Type == TypeContainer || elem.Type == TypeDatabase || elem.Type == TypeQueue {
			if elem.Parent != diagram.Focus {
				result.Errors = append(result.Errors, &ValidationError{
					Level: LevelContainer,
					Message: fmt.Sprintf("Container '%s' must belong to focus system '%s'",
						elem.Name, diagram.Focus.Name),
					Element: elem,
				})
			}
		}
	}
}

func (v *Validator) validateComponentDiagram(diagram *Diagram, result *ValidationResult) {
	if diagram.Focus == nil {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelComponent,
			Message: "Component diagram must have a focus (parent Container)",
		})
		return
	}

	if diagram.Focus.Type != TypeContainer {
		result.Errors = append(result.Errors, &ValidationError{
			Level:   LevelComponent,
			Message: "Component diagram focus must be a Container",
			Element: diagram.Focus,
		})
	}

	// All components should belong to the focus container
	for _, elem := range diagram.Elements {
		if elem.Type == TypeComponent {
			if elem.Parent != diagram.Focus {
				result.Errors = append(result.Errors, &ValidationError{
					Level: LevelComponent,
					Message: fmt.Sprintf("Component '%s' must belong to focus container '%s'",
						elem.Name, diagram.Focus.Name),
					Element: elem,
				})
			}
		}
	}
}

func (v *Validator) validateElements(diagram *Diagram, result *ValidationResult) {
	for _, elem := range diagram.Elements {
		if !IsElementAllowed(elem.Type, diagram.Level) {
			result.Errors = append(result.Errors, &ValidationError{
				Level: diagram.Level,
				Message: fmt.Sprintf("Element type '%s' not allowed at level %d (%s)",
					elem.Type, diagram.Level, diagram.Level.String()),
				Element: elem,
			})
		}

		// Validate element has required fields
		if elem.Name == "" {
			result.Errors = append(result.Errors, &ValidationError{
				Level:   diagram.Level,
				Message: "Element must have a name",
				Element: elem,
			})
		}
	}
}

func (v *Validator) validateRelationships(diagram *Diagram, result *ValidationResult) {
	for _, rel := range diagram.Relationships {
		if !IsRelationshipAllowed(rel.Type, diagram.Level) {
			result.Errors = append(result.Errors, &ValidationError{
				Level: diagram.Level,
				Message: fmt.Sprintf("Relationship type '%s' not allowed at level %d (%s)",
					rel.Type, diagram.Level, diagram.Level.String()),
				Rel: rel,
			})
		}

		// Validate relationship has source and destination
		if rel.Source == nil {
			result.Errors = append(result.Errors, &ValidationError{
				Level:   diagram.Level,
				Message: "Relationship must have a source",
				Rel:     rel,
			})
		}

		if rel.Destination == nil {
			result.Errors = append(result.Errors, &ValidationError{
				Level:   diagram.Level,
				Message: "Relationship must have a destination",
				Rel:     rel,
			})
		}
	}
}

func (v *Validator) validateConnectivity(diagram *Diagram, result *ValidationResult) {
	// Check for orphaned elements (elements with no relationships)
	// This is a warning, not an error, unless in strict mode
	connected := make(map[string]bool)

	for _, rel := range diagram.Relationships {
		if rel.Source != nil {
			connected[rel.Source.ID] = true
		}
		if rel.Destination != nil {
			connected[rel.Destination.ID] = true
		}
	}

	for _, elem := range diagram.Elements {
		if !connected[elem.ID] && v.strict {
			result.Errors = append(result.Errors, &ValidationError{
				Level:   diagram.Level,
				Message: fmt.Sprintf("Element '%s' has no relationships (orphaned)", elem.Name),
				Element: elem,
			})
		}
	}
}

func (v *Validator) calculateScore(result *ValidationResult) float64 {
	if len(result.Errors) == 0 {
		return 100.0
	}

	// Deduct points based on error severity
	// Each error costs 10 points
	deduction := float64(len(result.Errors)) * 10.0
	score := 100.0 - deduction

	if score < 0 {
		score = 0
	}

	return score
}

// ValidateQuick performs basic C4 compliance check
func ValidateQuick(diagram *Diagram) bool {
	validator := NewValidator(false)
	result := validator.Validate(diagram)
	return result.Valid
}
