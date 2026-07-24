# agm/internal/codexarchive - Requirements Specification (EARS)

## BDD Traceability

- Feature: `agm/test/bdd/features/legacy_spec_bdd_linkage_guardrails.feature`

## EARS Requirements

**CAX-01** When the session harness is not `codex-cli`, the archive bridge shall skip Codex-side archival.

**CAX-02** When a Codex session id is persisted in the AGM manifest, the archive bridge shall archive that exact Codex thread id before attempting transcript directory discovery.

**CAX-03** When no persisted Codex session id is available, the archive bridge shall resolve the Codex archive target from saved Codex transcripts whose cwd matches the AGM working directory candidates.

**CAX-04** When a persisted Codex thread id is archived, the archive bridge shall invoke the public `codex archive --remote unix:// <thread-id>` command, unless `AGM_CODEX_REMOTE` explicitly overrides that endpoint; it shall not use the app-server archive request.

**CAX-05** When neither a persisted Codex id nor a transcript cwd match can identify the Codex session, the archive bridge shall return an error without guessing a target.

**CAX-06** While archiving a Codex thread, the archive bridge shall not start, stop, or otherwise change device-global remote-control state.
