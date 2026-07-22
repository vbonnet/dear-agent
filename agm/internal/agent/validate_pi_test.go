package agent

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizePiHarnessAlias(t *testing.T) {
	t.Parallel()
	if got := NormalizeHarnessName("pi"); got != "pi-cli" {
		t.Fatalf("NormalizeHarnessName(pi) = %q", got)
	}
}

func TestValidatePiAvailabilityUsesBinaryOnly(t *testing.T) {
	original := lookPath
	t.Cleanup(func() { lookPath = original })
	lookPath = func(file string) (string, error) {
		if file != "pi" {
			t.Fatalf("lookPath(%q), want pi", file)
		}
		return "/test/bin/pi", nil
	}
	if err := ValidateHarnessAvailability("pi"); err != nil {
		t.Fatalf("ValidateHarnessAvailability(pi): %v", err)
	}

	lookPath = func(string) (string, error) { return "", errors.New("missing") }
	err := ValidateHarnessAvailability("pi-cli")
	if err == nil || !strings.Contains(err.Error(), "npm install") || !strings.Contains(err.Error(), "pi") {
		t.Fatalf("missing Pi diagnostic = %v", err)
	}
}
