package dolt

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidatePrefix(t *testing.T) {
	tests := []struct {
		name    string
		prefix  string
		wantErr bool
		errType error
	}{
		{
			name:    "valid prefix",
			prefix:  "agm_",
			wantErr: false,
		},
		{
			name:    "valid short prefix",
			prefix:  "wf_",
			wantErr: false,
		},
		{
			name:    "valid long prefix",
			prefix:  "engram_",
			wantErr: false,
		},
		{
			name:    "empty prefix",
			prefix:  "",
			wantErr: true,
		},
		{
			name:    "missing underscore",
			prefix:  "agm",
			wantErr: true,
		},
		{
			name:    "uppercase",
			prefix:  "AGM_",
			wantErr: true,
		},
		{
			name:    "reserved dolt prefix",
			prefix:  "dolt_",
			wantErr: true,
			errType: ErrReservedPrefix,
		},
		{
			name:    "reserved mysql prefix",
			prefix:  "mysql_",
			wantErr: true,
			errType: ErrReservedPrefix,
		},
		{
			name:    "too short",
			prefix:  "a_",
			wantErr: true,
		},
		{
			name:    "contains special chars",
			prefix:  "agm-test_",
			wantErr: true,
		},
		{
			name:    "contains space",
			prefix:  "agm test_",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePrefix(tt.prefix)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestReservedPrefixes(t *testing.T) {
	// Ensure all reserved prefixes are properly defined
	assert.Contains(t, ReservedPrefixes, "dolt_")
	assert.Contains(t, ReservedPrefixes, "mysql_")
	assert.Contains(t, ReservedPrefixes, "information_schema_")
	assert.Contains(t, ReservedPrefixes, "performance_schema_")
	assert.Contains(t, ReservedPrefixes, "sys_")

	// Test that all reserved prefixes are rejected
	for _, prefix := range ReservedPrefixes {
		err := validatePrefix(prefix)
		assert.ErrorIs(t, err, ErrReservedPrefix, "reserved prefix should be rejected: %s", prefix)
	}
}

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr bool
	}{
		{
			name: "valid manifest",
			json: `{
				"name": "agm",
				"version": "1.0.0",
				"description": "Agent-Generated Messaging",
				"storage": {
					"engine": "dolt",
					"prefix": "agm_"
				},
				"dependencies": [],
				"migrations": []
			}`,
			wantErr: false,
		},
		{
			name:    "invalid json",
			json:    "{invalid}",
			wantErr: true,
		},
		{
			name:    "empty json",
			json:    "{}",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := ParseManifest(tt.json)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manifest)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manifest)
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest *ComponentManifest
		wantErr  bool
	}{
		{
			name: "valid manifest",
			manifest: &ComponentManifest{
				Name:    "agm",
				Version: "1.0.0",
				Storage: ComponentStorage{
					Engine: "dolt",
					Prefix: "agm_",
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			manifest: &ComponentManifest{
				Version: "1.0.0",
				Storage: ComponentStorage{
					Engine: "dolt",
					Prefix: "agm_",
				},
			},
			wantErr: true,
		},
		{
			name: "missing version",
			manifest: &ComponentManifest{
				Name: "agm",
				Storage: ComponentStorage{
					Engine: "dolt",
					Prefix: "agm_",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid storage engine",
			manifest: &ComponentManifest{
				Name:    "agm",
				Version: "1.0.0",
				Storage: ComponentStorage{
					Engine: "postgres",
					Prefix: "agm_",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid prefix",
			manifest: &ComponentManifest{
				Name:    "agm",
				Version: "1.0.0",
				Storage: ComponentStorage{
					Engine: "dolt",
					Prefix: "agm", // Missing underscore
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifest(tt.manifest)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestComponentInfo_Marshaling(t *testing.T) {
	manifest := ComponentManifest{
		Name:    "agm",
		Version: "1.0.0",
		Storage: ComponentStorage{
			Engine: "dolt",
			Prefix: "agm_",
		},
		Dependencies: []ComponentDependency{
			{
				Name:     "engram",
				Version:  "^2.0.0",
				Required: true,
			},
		},
	}

	manifestJSON, err := json.Marshal(manifest)
	require.NoError(t, err)

	info := &ComponentInfo{
		Name:     "agm",
		Version:  "1.0.0",
		Prefix:   "agm_",
		Status:   string(StatusInstalled),
		Manifest: string(manifestJSON),
	}

	// Verify we can unmarshal the manifest back
	var parsedManifest ComponentManifest
	err = json.Unmarshal([]byte(info.Manifest), &parsedManifest)
	require.NoError(t, err)
	assert.Equal(t, "agm", parsedManifest.Name)
	assert.Equal(t, "1.0.0", parsedManifest.Version)
}

func TestComponentStatus(t *testing.T) {
	tests := []struct {
		status ComponentStatus
		valid  bool
	}{
		{StatusInstalled, true},
		{StatusUninstalled, true},
		{StatusError, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if tt.valid {
				assert.Contains(t, []ComponentStatus{StatusInstalled, StatusUninstalled, StatusError}, tt.status)
			}
		})
	}
}

// Test component dependency structure
func TestComponentDependency(t *testing.T) {
	dep := ComponentDependency{
		Name:             "engram",
		Version:          "^2.0.0",
		Required:         true,
		TablesReferenced: []string{"engram_sessions", "engram_entries"},
	}

	assert.Equal(t, "engram", dep.Name)
	assert.True(t, dep.Required)
	assert.Len(t, dep.TablesReferenced, 2)
}

// Test migration info structure
func TestComponentMigrationInfo(t *testing.T) {
	mig := ComponentMigrationInfo{
		Version:       1,
		Name:          "initial_schema",
		File:          "001_initial_schema.sql",
		Description:   "Create initial tables",
		Checksum:      "sha256:abc123",
		DependsOn:     []int{},
		TablesCreated: []string{"agm_sessions", "agm_messages"},
	}

	assert.Equal(t, 1, mig.Version)
	assert.Equal(t, "initial_schema", mig.Name)
	assert.Len(t, mig.TablesCreated, 2)
}

// Benchmark tests
func BenchmarkValidatePrefix(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = validatePrefix("agm_")
	}
}

func BenchmarkParseManifest(b *testing.B) {
	json := `{
		"name": "agm",
		"version": "1.0.0",
		"storage": {"engine": "dolt", "prefix": "agm_"}
	}`

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ParseManifest(json)
	}
}
