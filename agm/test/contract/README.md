# Contract Tests

The `contract` build tag contains two distinct boundaries:

- `TestActiveHarnessRegistryContract` is credential-free and always verifies
  the production registry for `claude-code`, `codex-cli`, `agy`, and
  `opencode-cli`.
- Provider-hosted scenarios are optional probes for Claude, OpenCode, and the
  deprecated Gemini compatibility adapter. Each probe skips only when its own
  binary, credential, server, or quota is unavailable.

Codex lifecycle behavior is CLI/tmux-backed rather than OpenAI-API-backed. Its
real source-binary coverage therefore lives in
`agm/test/integration/isolated/codex_lifecycle_test.go`, not in a
mock HTTP Pact.

## Running

From the repository root, run the portable active-harness contract:

```bash
go test -tags=contract ./agm/test/contract \
  -run '^TestActiveHarnessRegistryContract$'
```

Compile and run the complete contract-tagged package:

```bash
go test -tags=contract ./agm/test/contract/...
```

Without live provider prerequisites, the optional provider-hosted tests report
explicit skips while the active registry contract still executes. CI runs the
portable contract on every event and the complete credential-free graph on the
daily schedule and manual dispatch.

## Optional live prerequisites

- Claude tests require their documented Claude CLI credential environment.
- OpenCode tests require `OPENCODE_SERVER_URL` and a healthy server.
- Gemini compatibility tests require `GOOGLE_API_KEY` and consume the shared
  test quota.

These probes invoke AGM commands through `agm/test/helpers`. They are not a
substitute for the isolated source-built Codex lifecycle or portable adapter
parity tests.

## Retired mock Pact suite

The former `agm/test/contracts` package only configured Pact mock servers and
asserted that their generated base URLs were non-empty. It did not call AGM's
provider clients, required an unscheduled native `libpact_ffi`, and caused the
generic `contract` tag to fail at link time. It was removed instead of being
reported as adapter-contract coverage.

## References

- [Contract specification](SPEC.md)
- [Integration parity suite](../integration/AGENT_PARITY_TEST_SUITE.md)
- [Test helpers](../helpers/README.md)
