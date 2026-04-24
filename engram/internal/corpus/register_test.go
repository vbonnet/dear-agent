package corpus

import (
	"os"
	"os/exec"
	"testing"
)

func TestIsCorpusCallosumAvailable(t *testing.T) {
	// This test checks if the function works, not if cc is actually installed
	available := isCorpusCallosumAvailable()

	// Just verify it returns a boolean without panicking
	if available {
		t.Log("Corpus callosum CLI is available")
	} else {
		t.Log("Corpus callosum CLI is not available (expected in many environments)")
	}

	// Verify by checking exec.LookPath directly
	_, err := exec.LookPath("cc")
	expectedAvailable := (err == nil)

	if available != expectedAvailable {
		t.Errorf("isCorpusCallosumAvailable() = %v, but exec.LookPath result = %v", available, expectedAvailable)
	}
}

func TestRegisterEngramSchemas_GracefulDegradation(t *testing.T) {
	// Test that registration gracefully handles missing cc CLI
	// Save PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set empty PATH to simulate cc not being available
	os.Setenv("PATH", "")

	// Should not error when cc is unavailable (graceful degradation)
	err := RegisterEngramSchemas("test-workspace")
	if err != nil {
		t.Errorf("RegisterEngramSchemas should gracefully handle missing cc CLI, got error: %v", err)
	}
}

func TestUnregisterEngramSchemas_GracefulDegradation(t *testing.T) {
	// Save PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set empty PATH
	os.Setenv("PATH", "")

	// Should not error when cc is unavailable
	err := UnregisterEngramSchemas("test-workspace")
	if err != nil {
		t.Errorf("UnregisterEngramSchemas should gracefully handle missing cc CLI, got error: %v", err)
	}
}

func TestGetRegistrationStatus_GracefulDegradation(t *testing.T) {
	// Save PATH
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)

	// Set empty PATH
	os.Setenv("PATH", "")

	// Should return false when cc is unavailable
	registered, err := GetRegistrationStatus("test-workspace")
	if err != nil {
		t.Errorf("GetRegistrationStatus should not error when cc unavailable, got: %v", err)
	}

	if registered {
		t.Error("GetRegistrationStatus should return false when cc is not available")
	}
}

func TestContainsHelper(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{
			name:   "exact match",
			s:      "engram",
			substr: "engram",
			want:   true,
		},
		{
			name:   "substring present",
			s:      "components: engram, wayfinder, agm",
			substr: "engram",
			want:   true,
		},
		{
			name:   "substring not present",
			s:      "components: wayfinder, agm",
			substr: "engram",
			want:   false,
		},
		{
			name:   "empty substring",
			s:      "test",
			substr: "",
			want:   true,
		},
		{
			name:   "empty string",
			s:      "",
			substr: "test",
			want:   false,
		},
		{
			name:   "substring longer than string",
			s:      "short",
			substr: "very long substring",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contains(tt.s, tt.substr)
			if got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}
