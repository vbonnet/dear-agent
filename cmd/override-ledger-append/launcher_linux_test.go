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
	dynamic := append(static, &elf.Prog{
		ProgHeader: elf.ProgHeader{Type: elf.PT_INTERP},
	})
	if err := validateELFProgramHeaders(dynamic); err == nil ||
		!strings.Contains(err.Error(), "dynamic ELF interpreter") {
		t.Fatalf("dynamic program headers error = %v", err)
	}
}
