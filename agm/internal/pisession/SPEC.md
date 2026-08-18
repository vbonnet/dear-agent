# Pi Native Session Identity Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`pisession` is the single owner of Pi native session identity, private storage,
transcript discovery, bounded JSONL parsing, and import invariants. Callers use
this deep module instead of reconstructing Pi paths or selecting recent files.

## EARS Requirements

**PI-SESSION-01** When AGM accepts a Pi session ID, the system shall require a safe bounded identifier before using it as an argument, path component, or metadata key.

**PI-SESSION-02** When AGM prepares, discovers, or resumes Pi storage, the system shall use an absolute AGM-owned non-symlink directory with owner-only permissions.

**PI-SESSION-03** When AGM discovers, reads, or exports a Pi transcript, the system shall inspect bounded regular JSONL files without following symlinks and shall match the exact session ID in the transcript header.

**PI-SESSION-04** When more than one transcript claims the same Pi session ID, the system shall reject the ambiguous identity rather than select the newest file.

**PI-SESSION-05** When AGM imports a Pi transcript, the system shall validate total and per-line bounds, every JSONL record as a typed object, header identity, and absolute working directory and shall copy it atomically into private storage without overwriting an existing transcript.

**PI-SESSION-06** When AGM reads Pi messages, the system shall return bounded user, assistant, and tool-result text while ignoring internal thinking and tool invocation payloads.

**PI-SESSION-07** When AGM reports Pi context or cost, the system shall read bounded native usage records, use the latest assistant prompt footprint for context, and sum provider-reported native costs without substituting another harness's pricing.

**PI-SESSION-08** When AGM imports, reports usage for, or cold-resumes Pi history, the system shall preserve the native provider separately from the complete opaque model ID even when that model ID begins with the provider name, and shall leave the override empty when the transcript establishes no model.

**PI-SESSION-09** When a Pi coding-agent directory is supplied for create, import, or cold resume, the system shall normalize it to an absolute path, require an existing non-symlink directory, and preserve the validated path as native session metadata; an explicitly present per-session adapter value shall take precedence over the adapter process environment, create and import shall persist field presence when no directory is supplied so Pi's native default remains intentional, and metadata that genuinely predates that marker shall retain the invoking environment fallback for compatibility.

## Traceability

- Package tests: `agm/internal/pisession/session_test.go`
- Import tests: `agm/internal/importer/importer_test.go`
- History tests: `agm/internal/history/paths_test.go`
- BDD: `agm/test/bdd/features/agm_conversation_discovery_guardrails.feature`
