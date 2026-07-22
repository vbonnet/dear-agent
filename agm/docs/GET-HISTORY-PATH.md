# AGM Conversation History Paths

`agm session get-history-path` reports the native conversation-storage locations
for an AGM session. The session's **harness**, not its model, determines those
locations.

Active harnesses are Claude Code, Codex CLI, Antigravity (`agy`), OpenCode, and Pi.
Gemini CLI remains available as a deprecated compatibility harness.

## Usage

```bash
# Current tmux session, or the latest Claude session outside tmux
agm session get-history-path

# Named AGM session
agm session get-history-path my-session

# Machine-readable output
agm session get-history-path my-session --json

# Also require every reported path or path pattern to match on disk
agm session get-history-path my-session --verify
```

Without a session argument, the command uses the current tmux session. Outside
tmux, its fallback discovery is Claude-specific. Pass an AGM session name when
requesting AGY, Codex, OpenCode, Pi, or deprecated Gemini history outside that
session's tmux pane.

## Harness storage

### Claude Code

Claude encodes the working directory by replacing every non-alphanumeric
character with `-`:

```text
~/.claude/projects/<encoded-working-directory>/<conversation-id>.jsonl
~/.claude/projects/<encoded-working-directory>/sessions-index.json
```

The session manifest must contain `context.project`.

### Codex CLI

Codex rollout files are organized by date:

```text
~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl
```

AGM extracts a `YYYY-MM-DD` component from the identifier when present and
otherwise uses the current date.

### Antigravity (`agy`)

AGY uses its native conversation ID directly. For a named session AGM reads
`agy.conversation_id` from the manifest; it does not enter Claude UUID
discovery.

```text
~/.gemini/antigravity-cli/conversations/<conversation-id>.db
~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/transcript.jsonl
~/.gemini/antigravity-cli/brain/<conversation-id>/.system_generated/logs/transcript_full.jsonl
```

If a named AGY manifest has no native conversation ID, the command fails with a
request to reassociate or re-import the session.

### OpenCode

```text
${OPENCODE_DATA_DIR}/storage/message/<session-id>/
${OPENCODE_DATA_DIR}/storage/session/
```

When `OPENCODE_DATA_DIR` is unset, the base directory is
`~/.local/share/opencode`.

### Pi

AGM persists Pi sessions under an owner-only directory and records the exact
native session ID and JSONL path in the manifest. The command verifies that the
transcript header matches that identity before returning either path:

```text
<manifest.pi.session_dir>/
<manifest.pi.transcript_path>
```

AGM does not choose the newest Pi transcript. A missing identity, header
mismatch, duplicate native ID, or unverified path is an error.

### Gemini CLI (deprecated compatibility)

Gemini hashes the working directory with SHA-256 and uses the first eight hex
characters:

```text
~/.gemini/tmp/<8-character-hash>/chats/
~/.gemini/tmp/<8-character-hash>/logs.json
```

The session manifest must contain `context.project`.

## Output

Human-readable output identifies the session, harness, legacy UUID field, and
each candidate location:

```text
Session: agy-project
Harness: agy
UUID:    6bf900c2-3e3b-4f13-89fa-e31efb21ff48

Conversation History:
  /Users/example/.gemini/antigravity-cli/conversations/6bf900c2-3e3b-4f13-89fa-e31efb21ff48.db
  /Users/example/.gemini/antigravity-cli/brain/6bf900c2-3e3b-4f13-89fa-e31efb21ff48/.system_generated/logs/transcript.jsonl
  /Users/example/.gemini/antigravity-cli/brain/6bf900c2-3e3b-4f13-89fa-e31efb21ff48/.system_generated/logs/transcript_full.jsonl
```

The JSON shape is:

```json
{
  "session_name": "agy-project",
  "session_id": "agm-session-id",
  "harness": "agy",
  "uuid": "6bf900c2-3e3b-4f13-89fa-e31efb21ff48",
  "paths": ["..."],
  "exists": true,
  "metadata": {
    "harness": "agy",
    "app_dir": "/Users/example/.gemini/antigravity-cli",
    "conversation_id": "6bf900c2-3e3b-4f13-89fa-e31efb21ff48"
  }
}
```

`uuid` is retained for output compatibility. For AGY and OpenCode it carries
the harness-native conversation or session identifier and is not necessarily
an RFC 4122 UUID.

Without `--verify`, `exists` means path construction succeeded. With
`--verify`, it is true only when every reported path exists; glob patterns must
match at least one entry.

Useful scripting examples:

```bash
# First candidate path
agm session get-history-path my-session --json | jq -r '.paths[0]'

# Harness type
agm session get-history-path my-session --json | jq -r '.harness'

# Require all reported locations to exist
agm session get-history-path my-session --verify --json | jq -e '.exists'
```

## Errors

Path-construction errors emitted with `--json` use these codes:

| Code | Meaning |
|------|---------|
| `HARNESS_UNKNOWN` | The harness is empty or unsupported. |
| `UUID_MISSING` | The required native conversation/session identifier is empty. |
| `WORKING_DIR_MISSING` | Claude or deprecated Gemini path construction lacks a working directory. |
| `PATH_CONSTRUCTION_FAILED` | An unexpected filesystem or platform error occurred. |

Errors resolving the named AGM session or its native AGY conversation ID occur
before path construction and are returned as command errors.

Common checks:

```bash
agm session list --format json | jq '.[] | {name, harness, agy: .agy.conversation_id, claude: .claude.uuid}'
ls -la ~/.gemini/antigravity-cli/conversations
ls -la ~/.gemini/antigravity-cli/brain
```

## Development

The production implementation and its tests live in
`agm/internal/history/paths.go`, `agm/cmd/agm/get_history_path.go`, and their
adjacent test files. From the repository root:

```bash
go test ./agm/internal/history ./agm/cmd/agm
go test -race ./agm/internal/history
go test ./agm/test/integration -run '^TestGetHistoryPaths_'
go test ./agm/test/bdd/steps -run TestPackageCompiles
```

When adding a harness, update the living history SPEC and BDD production-wiring
scenario along with the implementation.

**Last audited**: 2026-07-20
