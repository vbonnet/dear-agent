# agm/internal/codexcontrol - Requirements Specification (EARS)

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

## EARS Requirements

**CCTL-01** When a client starts remote control, the package shall invoke `codex remote-control start --json`, accept a valid daemon-status JSON response only when `exec.ErrWaitDelay` reports inherited stdout delayed an otherwise successful process completion, and otherwise return command failures with stderr context.

**CCTL-02** When a client sends an app-server request, the package shall perform the proxy's WebSocket HTTP Upgrade and initialize JSON-RPC before sending thread operations.

**CCTL-03** When creating a Codex thread, the package shall pass the requested cwd, model, and `workspace-write` sandbox to `thread/start`.

**CCTL-04** When `thread/start` returns no thread id, the package shall report an error instead of returning incomplete metadata.

**CCTL-05** When setting or archiving a Codex thread, the package shall reject an empty thread id before invoking Codex app-server.

**CCTL-06** When reading app-server responses, the package shall ignore notifications and responses for other ids until the requested response id is received.
