package main

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

// These tests live in package main on purpose: the agm binary registers
// sandbox providers via the blank imports in new.go
// (_ ".../sandbox/apfs", _ ".../sandbox/bubblewrap", etc.). Each provider
// registers itself in an init(); if a blank import is dropped the code
// still compiles, but `agm new` fails at runtime with
// "provider not available: <name>".
//
// That is exactly BL-011: "agm session creation fails with
// 'provider not available: apfs'", which recurred across sessions and was
// never filed *because no test guarded the binary's wiring* —
// internal/sandbox/factory_test.go has its own blank import, so it stays
// green even when the agm binary regresses. A test in package main links
// the same imports the binary does, so it catches the regression.

// TestSandboxProviderWiring_DefaultProviderResolves guards the default
// `agm new` path (--sandbox-provider=auto -> sandbox.NewProvider()).
func TestSandboxProviderWiring_DefaultProviderResolves(t *testing.T) {
	info, err := sandbox.DetectPlatform()
	require.NoError(t, err)
	t.Logf("platform=%s recommended=%s", info.OS, info.Recommended)

	p, err := sandbox.NewProvider()
	require.NoErrorf(t, err,
		"default sandbox provider %q is not linked into the agm binary; "+
			"verify the blank imports in agm/cmd/agm/new.go (BL-011)",
		info.Recommended)
	require.NotNil(t, p)
}

// TestSandboxProviderWiring_APFSRegisteredOnDarwin asserts the exact
// recurring failure cannot return: apfs unresolved on macOS, where it is
// the recommended (default) provider.
func TestSandboxProviderWiring_APFSRegisteredOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("apfs is the Darwin provider; runtime is %s", runtime.GOOS)
	}
	p, err := sandbox.NewProviderForPlatform("apfs")
	require.NoError(t, err,
		"apfs provider not linked into the agm binary on darwin — this is "+
			"the BL-011 regression; restore _ \"...sandbox/apfs\" in new.go")
	require.NotNil(t, p)
}
