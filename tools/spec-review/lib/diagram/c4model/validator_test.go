package c4model

import (
	"testing"
)

func TestValidator_ContextDiagram(t *testing.T) {
	tests := []struct {
		name      string
		diagram   *Diagram
		wantValid bool
		wantScore float64
	}{
		{
			name: "valid context diagram",
			diagram: &Diagram{
				Name:  "System Context",
				Level: LevelContext,
				Elements: []*Element{
					{ID: "1", Name: "User", Type: TypePerson},
					{ID: "2", Name: "System", Type: TypeSoftwareSystem},
				},
				Relationships: []*Relationship{
					{
						ID:          "r1",
						Source:      &Element{ID: "1", Name: "User"},
						Destination: &Element{ID: "2", Name: "System"},
						Type:        RelUses,
					},
				},
			},
			wantValid: true,
			wantScore: 100.0,
		},
		{
			name: "missing person",
			diagram: &Diagram{
				Name:  "Invalid Context",
				Level: LevelContext,
				Elements: []*Element{
					{ID: "2", Name: "System", Type: TypeSoftwareSystem},
				},
			},
			wantValid: false,
			wantScore: 90.0,
		},
		{
			name: "missing software system",
			diagram: &Diagram{
				Name:  "Invalid Context",
				Level: LevelContext,
				Elements: []*Element{
					{ID: "1", Name: "User", Type: TypePerson},
				},
			},
			wantValid: false,
			wantScore: 90.0,
		},
		{
			name: "wrong element type",
			diagram: &Diagram{
				Name:  "Invalid Context",
				Level: LevelContext,
				Elements: []*Element{
					{ID: "1", Name: "User", Type: TypePerson},
					{ID: "2", Name: "System", Type: TypeSoftwareSystem},
					{ID: "3", Name: "Container", Type: TypeContainer}, // Wrong for L1
				},
			},
			wantValid: false,
			wantScore: 90.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(false)
			result := v.Validate(tt.diagram)

			if result.Valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", result.Valid, tt.wantValid)
				for _, err := range result.Errors {
					t.Logf("  Error: %s", err.Error())
				}
			}

			if result.Score != tt.wantScore {
				t.Errorf("Validate() score = %v, want %v", result.Score, tt.wantScore)
			}
		})
	}
}

func TestValidator_ContainerDiagram(t *testing.T) {
	focusSystem := &Element{ID: "sys1", Name: "My System", Type: TypeSoftwareSystem}

	tests := []struct {
		name      string
		diagram   *Diagram
		wantValid bool
	}{
		{
			name: "valid container diagram",
			diagram: &Diagram{
				Name:  "Container Diagram",
				Level: LevelContainer,
				Focus: focusSystem,
				Elements: []*Element{
					{ID: "c1", Name: "Web App", Type: TypeContainer, Parent: focusSystem},
					{ID: "c2", Name: "Database", Type: TypeDatabase, Parent: focusSystem},
				},
			},
			wantValid: true,
		},
		{
			name: "missing focus",
			diagram: &Diagram{
				Name:  "Invalid Container",
				Level: LevelContainer,
				Elements: []*Element{
					{ID: "c1", Name: "Web App", Type: TypeContainer},
				},
			},
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator(false)
			result := v.Validate(tt.diagram)

			if result.Valid != tt.wantValid {
				t.Errorf("Validate() valid = %v, want %v", result.Valid, tt.wantValid)
				for _, err := range result.Errors {
					t.Logf("  Error: %s", err.Error())
				}
			}
		})
	}
}

func TestIsElementAllowed(t *testing.T) {
	tests := []struct {
		elementType ElementType
		level       Level
		want        bool
	}{
		{TypePerson, LevelContext, true},
		{TypeSoftwareSystem, LevelContext, true},
		{TypeContainer, LevelContext, false},
		{TypeContainer, LevelContainer, true},
		{TypeComponent, LevelComponent, true},
		{TypeComponent, LevelContainer, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.elementType)+"@"+tt.level.String(), func(t *testing.T) {
			if got := IsElementAllowed(tt.elementType, tt.level); got != tt.want {
				t.Errorf("IsElementAllowed(%v, %v) = %v, want %v",
					tt.elementType, tt.level, got, tt.want)
			}
		})
	}
}

func TestIsRelationshipAllowed(t *testing.T) {
	tests := []struct {
		relType RelationshipType
		level   Level
		want    bool
	}{
		{RelUses, LevelContext, true},
		{RelReadsFrom, LevelContext, false},
		{RelReadsFrom, LevelContainer, true},
		{RelWritesTo, LevelContainer, true},
		{RelDependsOn, LevelComponent, true},
		{RelDependsOn, LevelContext, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.relType)+"@"+tt.level.String(), func(t *testing.T) {
			if got := IsRelationshipAllowed(tt.relType, tt.level); got != tt.want {
				t.Errorf("IsRelationshipAllowed(%v, %v) = %v, want %v",
					tt.relType, tt.level, got, tt.want)
			}
		})
	}
}
