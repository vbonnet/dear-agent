package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// A shadowed store is the one failure the refresher cannot repair, so check
// mode must name the operator step instead of printing an ordinary status line.
func TestPrintStatusReportsShadowedStore(t *testing.T) {
	var buf bytes.Buffer
	printStatus(&buf, auth.TokenStatus{
		HasToken:        false,
		HasRefreshToken: false,
		PrimaryStore:    auth.StoreKeychain,
		Shadowed:        true,
	})
	out := buf.String()
	if !strings.Contains(out, "SHADOWED") {
		t.Fatalf("status output did not report the shadow:\n%s", out)
	}
	if !strings.Contains(out, "/login") {
		t.Fatalf("status output did not name the operator remedy:\n%s", out)
	}
	if !strings.Contains(out, "store=keychain") {
		t.Fatalf("status output did not name the store the CLI reads:\n%s", out)
	}
}

// A healthy file-backed credential must not emit the alarm.
func TestPrintStatusQuietWhenNotShadowed(t *testing.T) {
	var buf bytes.Buffer
	printStatus(&buf, auth.TokenStatus{
		HasToken:        true,
		HasRefreshToken: true,
		Fresh:           true,
		PrimaryStore:    auth.StoreFile,
	})
	if strings.Contains(buf.String(), "SHADOWED") {
		t.Fatalf("healthy status reported a shadow:\n%s", buf.String())
	}
}
