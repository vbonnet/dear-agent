package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRequireFreshAuthenticationRejectsPasswordlessSudo(t *testing.T) {
	invalidations := 0
	prompts := 0
	err := requireFreshAuthentication(
		func() error {
			invalidations++
			return nil
		},
		func() (bool, error) { return true, nil },
		func() error {
			prompts++
			return nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "passwordless sudo") {
		t.Fatalf("error = %v, want passwordless-sudo rejection", err)
	}
	if invalidations != 2 {
		t.Fatalf("invalidations = %d, want before and after probe", invalidations)
	}
	if prompts != 0 {
		t.Fatalf("prompts = %d, want no prompt after passwordless rejection", prompts)
	}
}

func TestRequireFreshAuthenticationRequiresPromptAfterInvalidation(t *testing.T) {
	sequence := []string{}
	err := requireFreshAuthentication(
		func() error {
			sequence = append(sequence, "invalidate")
			return nil
		},
		func() (bool, error) {
			sequence = append(sequence, "probe")
			return false, nil
		},
		func() error {
			sequence = append(sequence, "prompt")
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sequence, ","); got != "invalidate,probe,prompt" {
		t.Fatalf("sequence = %s", got)
	}
}

func TestRequireFreshAuthenticationInvalidatesAfterPromptFailure(t *testing.T) {
	invalidations := 0
	err := requireFreshAuthentication(
		func() error {
			invalidations++
			return nil
		},
		func() (bool, error) { return false, nil },
		func() error { return errors.New("authentication cancelled") },
	)
	if err == nil || !strings.Contains(err.Error(), "authentication cancelled") {
		t.Fatalf("error = %v, want prompt failure", err)
	}
	if invalidations != 2 {
		t.Fatalf("invalidations = %d, want before and after failed prompt", invalidations)
	}
}
