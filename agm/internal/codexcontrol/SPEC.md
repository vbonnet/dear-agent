# agm/internal/codexcontrol - Requirements Specification (EARS)

## EARS Requirements

**CCTL-01** When a client starts remote control, the package shall invoke `codex remote-control start --json` and return any command failure with stderr context.

**CCTL-02** When a client sends an app-server request, the package shall initialize the JSON-RPC proxy before sending thread operations.

**CCTL-03** When creating a Codex thread, the package shall pass the requested cwd, model, and `workspace-write` sandbox to `thread/start`.

**CCTL-04** When `thread/start` returns no thread id, the package shall report an error instead of returning incomplete metadata.

**CCTL-05** When setting or archiving a Codex thread, the package shall reject an empty thread id before invoking Codex app-server.

**CCTL-06** When reading app-server responses, the package shall ignore notifications and responses for other ids until the requested response id is received.
