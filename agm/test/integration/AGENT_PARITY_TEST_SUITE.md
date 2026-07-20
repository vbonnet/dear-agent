# Active Harness Parity Tests

The integration-tagged parity boundary has two layers:

1. `portable/active_harness_test.go` always checks the shared non-I/O adapter
   contract for every active harness: `claude-code`, `codex-cli`, `agy`, and
   `opencode-cli`.
2. Host-dependent lifecycle tests live beside their isolated harness fixtures.
   Each test owns its binary, credential, service, tmux socket, state, and
   cleanup prerequisites; an unavailable prerequisite skips only that harness.

The portable layer intentionally does not create sessions or use host
credentials. It validates canonical identity, versions, capabilities, default
and test models, aliases, and model-family coverage through the same production
adapter registry used by AGM.

The old Claude/Gemini Ginkgo matrix was removed because it globally skipped on
the OpenCode prerequisite, omitted Codex, used dummy credentials for live
operations, and accepted either success or failure in many assertions. Those
tests could report a green parity suite without exercising any adapter.

Run the portable and isolated integration graph with:

```bash
go test -tags=integration ./agm/test/integration/...
```

Run the portable parity contract alone with:

```bash
go test -tags=integration ./agm/test/integration/portable \
  -run 'TestActiveHarnessParityContract|TestHarnessPrerequisitesAreScoped'
```
