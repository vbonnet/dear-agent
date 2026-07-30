package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestFreshUnixAuthenticationArgsExcludeValidationPseudocommand(t *testing.T) {
	for _, tt := range []struct {
		name           string
		nonInteractive bool
		want           []string
	}{
		{name: "probe", nonInteractive: true, want: []string{"-k", "-n", "/usr/bin/true"}},
		{name: "prompt", want: []string{"-k", "/usr/bin/true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := freshUnixAuthenticationArgs(tt.nonInteractive)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("freshUnixAuthenticationArgs() = %q, want %q", got, tt.want)
			}
			if strings.Contains(strings.Join(got, " "), "-v") {
				t.Fatalf("fresh authentication args use sudo validation pseudocommand: %q", got)
			}
		})
	}
}

func TestOperatorGrantInstallArgsUseOneNonCachingPrivilegedInstaller(t *testing.T) {
	const path = "/etc/dear-agent/override-grants/admin.json"
	want := []string{
		"-k",
		"/bin/sh",
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	}
	if got := operatorGrantInstallArgs(path); !reflect.DeepEqual(got, want) {
		t.Fatalf("operatorGrantInstallArgs() = %q, want %q", got, want)
	}
}
