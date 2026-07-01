package speccoverage

import (
	"path/filepath"
	"testing"
)

func TestParitySurfacesHaveSpecAndBDD(t *testing.T) {
	t.Parallel()
	findings, err := Validate(repoRoot())
	if err != nil {
		t.Fatal(err)
	}
	for _, finding := range findings {
		t.Error(finding.Error())
	}
}

func TestParitySurfaceRegistryIsNotEmpty(t *testing.T) {
	t.Parallel()
	if len(ParitySurfaces()) == 0 {
		t.Fatal("parity surface registry is empty")
	}
}

func TestUnregisteredParityFeaturesIsEmpty(t *testing.T) {
	t.Parallel()
	missing, err := UnregisteredParityFeatures(repoRoot())
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) > 0 {
		t.Fatalf("unregistered parity features: %v", missing)
	}
}

func TestValidateSurfaceDoesNotReadEmptyPaths(t *testing.T) {
	t.Parallel()
	findings := validateSurface(repoRoot(), Surface{Name: "empty surface"})
	if len(findings) != 3 {
		t.Fatalf("expected three structural findings, got %d: %v", len(findings), findings)
	}
	for _, finding := range findings {
		if finding.Path != "" {
			t.Fatalf("empty-path finding should not attempt file read: %v", finding)
		}
	}
}

func repoRoot() string {
	return filepath.Clean(filepath.Join("..", ".."))
}
