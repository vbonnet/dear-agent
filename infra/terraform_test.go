// Package infra holds the Go-side gates for this OpenTofu root.
//
// These tests are the local half of `.github/workflows/terraform-lint.yml`, so
// `make test-affected` catches a formatting or validation regression before CI
// does. They are the reusable pattern for any future OpenTofu root in this
// repository: shell out to the real tool, skip cleanly when it is absent, and
// assert on its exit status and output rather than reimplementing its rules.
//
// A terratest-style assertion over a saved plan needs an inventory this public
// repository deliberately does not carry, so plan-level assertions belong with
// the fixture that supplies one, not here.
package infra

import (
	"os/exec"
	"strings"
	"testing"
)

// requireTool skips the test when the binary is not installed. CI installs both
// tools explicitly, so a skip here never hides a CI failure; it only keeps a
// developer machine without OpenTofu from reporting a false red.
func requireTool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s is not installed; the terraform-lint workflow runs this gate in CI", name)
	}
	return path
}

func TestTerraformIsCanonicallyFormatted(t *testing.T) {
	tofu := requireTool(t, "tofu")

	out, err := exec.Command(tofu, "fmt", "-check", "-recursive", "-list=true", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("tofu fmt -check reported unformatted files (run `tofu fmt -recursive infra`):\n%s", out)
	}
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("tofu fmt -check listed files:\n%s", out)
	}
}

func TestTerraformConfigurationValidates(t *testing.T) {
	tofu := requireTool(t, "tofu")

	// -backend=false keeps this credential-free: it initialises providers and
	// modules without contacting the real state backend.
	if out, err := exec.Command(tofu, "init", "-backend=false", "-input=false", "-no-color").CombinedOutput(); err != nil {
		t.Fatalf("tofu init -backend=false failed: %v\n%s", err, out)
	}

	out, err := exec.Command(tofu, "validate", "-no-color").CombinedOutput()
	if err != nil {
		t.Fatalf("tofu validate failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "The configuration is valid") {
		t.Fatalf("tofu validate did not report a valid configuration:\n%s", out)
	}
}
