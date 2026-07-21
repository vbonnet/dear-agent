package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseProfileWeightsStatements(t *testing.T) {
	profile := `mode: atomic
example/a.go:1.1,2.2 3 1
example/a.go:3.1,4.2 1 0
`
	got, err := parseProfile(strings.NewReader(profile))
	if err != nil {
		t.Fatal(err)
	}
	if got.Covered != 3 || got.Total != 4 || got.percent() != 75 {
		t.Fatalf("coverage = %+v (%.1f%%), want 3/4 (75%%)", got, got.percent())
	}
}

func TestParseProfileRejectsMalformedInput(t *testing.T) {
	for _, profile := range []string{
		"",
		"not a profile\n",
		"mode: set\nmalformed\n",
		"mode: set\nexample/a.go:1.1,2.2 nope 1\n",
		"mode: set\n",
	} {
		if _, err := parseProfile(strings.NewReader(profile)); err == nil {
			t.Fatalf("parseProfile(%q) succeeded", profile)
		}
	}
}

func TestLoadPolicyIsStrict(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data := `{
  "version": 1,
  "packages": [
    {"name": "ops", "package": "./agm/internal/ops", "minimum_statements": 73.0}
  ]
}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := loadPolicy(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Packages) != 1 || p.Packages[0].MinimumStatements != 73 {
		t.Fatalf("policy = %+v", p)
	}

	if err := os.WriteFile(path, []byte(`{"version":1,"unknown":true,"packages":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicy(path); err == nil {
		t.Fatal("loadPolicy accepted an unknown field")
	}

	if err := os.WriteFile(path, []byte(data+` {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPolicy(path); err == nil {
		t.Fatal("loadPolicy accepted a trailing JSON value")
	}
}

func TestValidatePolicyRejectsUnsafeFloorsAndDuplicates(t *testing.T) {
	valid := target{Name: "ops", Package: "./agm/internal/ops", MinimumStatements: 73}
	cases := []policy{
		{Version: 2, Packages: []target{valid}},
		{Version: 1},
		{Version: 1, Packages: []target{{Package: valid.Package, MinimumStatements: 73}}},
		{Version: 1, Packages: []target{{Name: "ops", Package: valid.Package, MinimumStatements: 101}}},
		{Version: 1, Packages: []target{valid, valid}},
	}
	for _, tc := range cases {
		if err := validatePolicy(tc); err == nil {
			t.Fatalf("validatePolicy(%+v) succeeded", tc)
		}
	}
}

func TestRunPreservesPackageTestFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	data := `{"version":1,"packages":[{"name":"missing","package":"./definitely-not-a-package","minimum_statements":1}]}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	var stderr bytes.Buffer
	if code := run([]string{"--policy", path}, &bytes.Buffer{}, &stderr); code != 1 {
		t.Fatalf("run() exit = %d, want 1; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "test ./definitely-not-a-package") {
		t.Fatalf("run() did not preserve package failure: %s", stderr.String())
	}
}

func TestRunCoveragePreservesContextTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := runCoverage(ctx, ".", filepath.Join(t.TempDir(), "coverage.out"), false)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("runCoverage() error = %v, want timeout", err)
	}
}
