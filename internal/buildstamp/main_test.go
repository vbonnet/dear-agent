package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitGOFLAGSMatchesGoQuotedFields(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr bool
	}{
		{name: "empty", input: "", want: nil},
		{name: "ASCII separators", input: " -p=2\t-tags=x\n-v\r-buildvcs=false ", want: []string{"-p=2", "-tags=x", "-v", "-buildvcs=false"}},
		{name: "single quoted field", input: "'-toolexec=/tmp/wrapper -ldflags-helper'", want: []string{"-toolexec=/tmp/wrapper -ldflags-helper"}},
		{name: "double quoted field", input: `"-toolexec=/tmp/wrapper -ldflags-helper"`, want: []string{"-toolexec=/tmp/wrapper -ldflags-helper"}},
		{name: "double quote inside single quoted field", input: `'-tags=one " two'`, want: []string{`-tags=one " two`}},
		{name: "single quote inside double quoted field", input: `"-tags=one ' two"`, want: []string{"-tags=one ' two"}},
		{name: "adjacent quoted fields", input: "'-p=2''--ldflags=-s'", want: []string{"-p=2", "--ldflags=-s"}},
		{name: "adjacent quoted and plain fields", input: "'-p=2'--ldflags=-s", want: []string{"-p=2", "--ldflags=-s"}},
		{name: "empty quoted fields", input: `'' ""`, want: []string{"", ""}},
		{name: "interior quotes are literal", input: `-toolexec="wrapper -arg`, want: []string{`-toolexec="wrapper`, "-arg"}},
		{name: "backslashes are literal", input: `-tags=a\ b`, want: []string{`-tags=a\`, "b"}},
		{name: "unterminated single quote", input: "'-p=2", wantErr: true},
		{name: "unterminated double quote", input: `"-p=2`, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := splitGOFLAGS(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitGOFLAGS(%q) unexpectedly succeeded: %v", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitGOFLAGS(%q): %v", tc.input, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitGOFLAGS(%q) = %#v, want %#v", tc.input, got, tc.want)
			}
		})
	}
}

func TestHasLinkerFlagMatchesExactTopLevelName(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  bool
	}{
		{name: "single dash", field: "-ldflags", want: true},
		{name: "single dash value", field: "-ldflags=-s", want: true},
		{name: "double dash", field: "--ldflags", want: true},
		{name: "double dash value", field: "--ldflags=-s", want: true},
		{name: "suffix", field: "-ldflags-helper", want: false},
		{name: "double dash suffix", field: "--ldflags-helper", want: false},
		{name: "toolexec payload", field: "-toolexec=/tmp/wrapper -ldflags-helper", want: false},
		{name: "interior quote", field: `-ldflags-helper='-s`, want: false},
		{name: "three dashes", field: "---ldflags=-s", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasLinkerFlag([]string{tc.field}); got != tc.want {
				t.Fatalf("hasLinkerFlag(%q) = %v, want %v", tc.field, got, tc.want)
			}
		})
	}
}

func TestEffectiveGOFLAGSUsesByteNonemptyDirectValue(t *testing.T) {
	t.Setenv(rawGOFLAGSKey, " \t\r\n")
	t.Setenv(rawGOENVKey, filepath.Join(t.TempDir(), "must-not-be-read"))

	got, err := effectiveGOFLAGS()
	if err != nil {
		t.Fatal(err)
	}
	if got != " \t\r\n" {
		t.Fatalf("effectiveGOFLAGS() = %q, want whitespace-only direct value", got)
	}
}

func TestEffectiveGOFLAGSFallsBackToCapturedGOENV(t *testing.T) {
	goenv := filepath.Join(t.TempDir(), "goenv")
	if err := os.WriteFile(goenv, []byte("GOFLAGS='-tags=persisted tag'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rawGOFLAGSKey, "")
	t.Setenv(rawGOENVKey, goenv)

	got, err := effectiveGOFLAGS()
	if err != nil {
		t.Fatal(err)
	}
	if got != "'-tags=persisted tag'" {
		t.Fatalf("effectiveGOFLAGS() = %q, want captured GOENV value", got)
	}
}

func TestGoEnvQueryEnvironmentRemovesBootstrapValues(t *testing.T) {
	got := goEnvQueryEnvironment([]string{
		"PATH=/test/bin",
		"GOFLAGS=bootstrap-value",
		"GOENV=off",
		"OTHER=value",
	}, "/captured/goenv")
	want := []string{"PATH=/test/bin", "OTHER=value", "GOENV=/captured/goenv"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("goEnvQueryEnvironment() = %#v, want %#v", got, want)
	}
}
