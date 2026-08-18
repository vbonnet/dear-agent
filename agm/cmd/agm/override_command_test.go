package main

import "testing"

func TestOverrideCommandDoesNotExposeStandaloneAuthorize(t *testing.T) {
	for _, command := range overrideCmd.Commands() {
		if command.Name() == "authorize" {
			t.Fatal("override authorize must not record or consume a use outside a launch boundary")
		}
	}
}
