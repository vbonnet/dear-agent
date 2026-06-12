package corpus

import (
	"testing"
)

func TestIsCorpusCallosumAvailable(t *testing.T) {
	// Without CORPUS_CALLOSUM_BIN set, should be unavailable.
	t.Setenv("CORPUS_CALLOSUM_BIN", "")
	if isCorpusCallosumAvailable() {
		t.Error("isCorpusCallosumAvailable() = true without CORPUS_CALLOSUM_BIN, want false")
	}

	// With CORPUS_CALLOSUM_BIN pointing at a non-existent binary, still unavailable.
	t.Setenv("CORPUS_CALLOSUM_BIN", "/nonexistent/corpus-callosum")
	if isCorpusCallosumAvailable() {
		t.Error("isCorpusCallosumAvailable() = true with bad CORPUS_CALLOSUM_BIN, want false")
	}
}

func TestRegisterEngramSchemas_GracefulDegradation(t *testing.T) {
	// Test that registration gracefully handles missing cc CLI
	// Save PATH

	// Set empty PATH to simulate cc not being available
	t.Setenv("PATH", "")

	// Should not error when cc is unavailable (graceful degradation)
	err := RegisterEngramSchemas("test-workspace")
	if err != nil {
		t.Errorf("RegisterEngramSchemas should gracefully handle missing cc CLI, got error: %v", err)
	}
}

func TestUnregisterEngramSchemas_GracefulDegradation(t *testing.T) {
	// Save PATH

	// Set empty PATH
	t.Setenv("PATH", "")

	// Should not error when cc is unavailable
	err := UnregisterEngramSchemas("test-workspace")
	if err != nil {
		t.Errorf("UnregisterEngramSchemas should gracefully handle missing cc CLI, got error: %v", err)
	}
}

func TestGetRegistrationStatus_GracefulDegradation(t *testing.T) {
	// Save PATH

	// Set empty PATH
	t.Setenv("PATH", "")

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
