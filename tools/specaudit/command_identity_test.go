package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCommandIdentityDoesNotDependOnSourceCheckout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"help"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help exit code = %d, stderr = %q", code, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("help stderr = %q, want empty", stderr.String())
	}
	for _, command := range []string{
		"specaudit guard",
		"specaudit inventory",
		"specaudit validate",
		"specaudit render",
	} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("help omitted logical command identity %q", command)
		}
	}
	assertPortableCommandIdentity(t, "help", stdout.String())

	_, inventoryReport, _ := auditFixture(t)
	if got := inventoryReport.Methodology.Collector; got != "specaudit inventory" {
		t.Fatalf("collector identity = %q, want %q", got, "specaudit inventory")
	}
	if got := inventoryReport.Methodology.Reproduce; len(got) != 1 || !strings.HasPrefix(got[0], "specaudit inventory ") {
		t.Fatalf("reproduction identity = %#v, want one logical specaudit command", got)
	}
	assertPortableCommandIdentity(t, "inventory methodology", strings.Join(append(
		[]string{inventoryReport.Methodology.Collector},
		inventoryReport.Methodology.Reproduce...,
	), "\n"))
}

func assertPortableCommandIdentity(t *testing.T, field, value string) {
	t.Helper()
	for _, checkoutRelative := range []string{"go run", "./tools/specaudit", "tools/specaudit"} {
		if strings.Contains(value, checkoutRelative) {
			t.Errorf("%s contains checkout-relative command identity %q: %q", field, checkoutRelative, value)
		}
	}
}
