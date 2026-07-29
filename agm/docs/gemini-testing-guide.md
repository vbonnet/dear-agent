# Gemini CLI Compatibility Testing Guide

**Quick reference for the deprecated Gemini CLI adapter**

## Scope

Gemini CLI is a known but deprecated compatibility harness. It is not part of
AGM's active cross-harness parity set. Tests preserve the concrete adapter
behaviors that still exist; they must not claim active-harness feature parity
or depend on a retired universal agent lifecycle interface.

## Current test files

```text
agm/internal/agent/
├── gemini_cli_adapter.go
├── gemini_cli_adapter_test.go
└── harnesses_test.go

agm/cmd/agm/
└── command_parity_test.go
```

There is no `gemini_parity_test.go`, `parity_comparison_test.go`, or
`TestGeminiAdapter_FeatureParity_HarnessMetadata` suite.

## Run the real tests

Run from the repository root:

```bash
# Every concrete Gemini CLI adapter test.
go test -v ./agm/internal/agent -run '^TestGeminiCLIAdapter_'

# The known-but-deprecated harness classification.
go test -v ./agm/internal/agent -run '^TestGeminiCLIIsDeprecatedButKnown$'

# The CLI parity boundary: deprecated Gemini is not an active target.
go test -v ./agm/cmd/agm -run '^TestDeprecatedGeminiIsNotActiveCommandParity$'

# The complete owning package.
go test ./agm/internal/agent
```

The anchored expressions matter: a successful `go test -run` with no matching
tests is not verification. Confirm verbose output names the intended tests.

## Covered compatibility behavior

`gemini_cli_adapter_test.go` currently covers:

- hook execution and `CommandRunHook`
- capability metadata for hooks
- rename and working-directory command translation
- invalid-command and missing-parameter errors
- stored Gemini UUID metadata
- resume with and without a UUID
- clear-history and system-prompt commands
- history-path resolution

These tests exercise concrete `GeminiCLIAdapter` behavior. Lifecycle
transactions shared by active harnesses belong to `agm/internal/ops` and its
tests; do not duplicate them as fictitious Gemini parity methods.

## Adding a compatibility test

Use the real JSON session store and concrete adapter:

```go
func TestGeminiCLIAdapter_YourBehavior(t *testing.T) {
    store, err := NewJSONSessionStore(filepath.Join(t.TempDir(), "sessions.json"))
    if err != nil {
        t.Fatal(err)
    }
    adapter, err := NewGeminiCLIAdapter(store)
    if err != nil {
        t.Fatal(err)
    }

    // Arrange stored session metadata, invoke the concrete adapter method,
    // and assert its observable file/store/command result.
    _ = adapter
}
```

Keep test data in `t.TempDir`, use `t.Setenv` for environment overrides, and
name the test `TestGeminiCLIAdapter_...` so the documented command selects it.
Do not add examples for nonexistent `GeminiConfig`, `MockSessionStore`,
`Agent.Start`, or export/import parity APIs.

## Verification

A Gemini compatibility change is verified only when:

1. the exact new or changed test name appears in verbose output;
2. `go test ./agm/internal/agent` passes; and
3. active-harness conformance and command-parity tests still treat Gemini CLI
   as deprecated rather than silently restoring it to the active set.
