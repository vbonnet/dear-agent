# agm/internal/codexarchive - Requirements Specification (EARS)

## EARS Requirements

**CAX-01** When the session harness is not `codex-cli`, the archive bridge shall skip Codex-side archival.

**CAX-02** When a Codex session id is persisted in the AGM manifest, the archive bridge shall archive that exact Codex thread id before attempting transcript directory discovery.

**CAX-03** When no persisted Codex session id is available, the archive bridge shall resolve the Codex archive target from saved Codex transcripts whose cwd matches the AGM working directory candidates.

**CAX-04** When Codex app-server archival fails or app-server remote control is unavailable for a persisted Codex thread id, the archive bridge shall fall back to the Codex CLI archive command.

**CAX-05** When neither a persisted Codex id nor a transcript cwd match can identify the Codex session, the archive bridge shall return an error without guessing a target.
