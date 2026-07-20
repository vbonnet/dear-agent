package steps

import "testing"

func TestRetiredWayfinderPatternMatchesPrefixedIdentifiers(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "uppercase identifier", value: "S" + "8BuildVerification", want: true},
		{name: "lowercase key", value: "s" + "9_validation_depth", want: true},
		{name: "retired dotted path", value: "design.security", want: true},
		{name: "canonical phase", value: "BUILD", want: false},
		{name: "ordinary lowercase version", value: "api/v1", want: false},
		{name: "ordinary version", value: "revision-v2", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retiredWayfinderPattern.MatchString(tt.value); got != tt.want {
				t.Fatalf("retiredWayfinderPattern.MatchString(%q) = %t, want %t", tt.value, got, tt.want)
			}
		})
	}
}
