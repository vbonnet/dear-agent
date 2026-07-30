//go:build linux

package main

import (
	"debug/elf"
	"strings"
	"testing"
)

func TestValidateELFProgramHeadersRejectsDynamicLoader(t *testing.T) {
	static := []*elf.Prog{{
		ProgHeader: elf.ProgHeader{Type: elf.PT_LOAD},
	}}
	if err := validateELFProgramHeaders(static); err != nil {
		t.Fatalf("static program headers rejected: %v", err)
	}
	dynamic := append([]*elf.Prog{}, static...)
	dynamic = append(dynamic, &elf.Prog{
		ProgHeader: elf.ProgHeader{Type: elf.PT_INTERP},
	})
	if err := validateELFProgramHeaders(dynamic); err == nil ||
		!strings.Contains(err.Error(), "dynamic ELF interpreter") {
		t.Fatalf("dynamic program headers error = %v", err)
	}
}

func TestValidateLinuxProcessIsolationRequiresAdminOnlyPtrace(t *testing.T) {
	for _, scope := range []string{"0\n", "1\n"} {
		if err := validateLinuxProcessIsolation([]byte(scope), 0); err == nil ||
			!strings.Contains(err.Error(), "ptrace_scope >= 2") {
			t.Fatalf("ptrace scope %q error = %v", scope, err)
		}
	}
	for _, scope := range []string{"2\n", "3\n"} {
		if err := validateLinuxProcessIsolation([]byte(scope), 0); err != nil {
			t.Fatalf("ptrace scope %q rejected: %v", scope, err)
		}
	}
}

func TestValidateLinuxProcessIsolationRejectsActiveTracer(t *testing.T) {
	if err := validateLinuxProcessIsolation([]byte("2\n"), 4242); err == nil ||
		!strings.Contains(err.Error(), "currently traced by PID 4242") {
		t.Fatalf("active tracer error = %v", err)
	}
}
