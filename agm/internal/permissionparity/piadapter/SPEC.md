# Pi Authorization Adapter Specification

<!-- Last audited at: 2026-07-21 -->

## Purpose

`piadapter` owns the native Pi `tool_call` decision model and the embedded,
dependency-free extension installed in AGM-private storage.

## EARS Requirements

**PI-AUTH-01** When Pi calls a built-in or extension tool, the adapter shall map it to a shared or canonical extension permission category and apply anchored exact or explicit-wildcard matching.

**PI-AUTH-02** When plan mode receives any tool other than read, grep, find, or list, the adapter shall block it before allowlist evaluation, including tools added by an extension.

**PI-AUTH-03** When default mode receives an unmatched call, the adapter shall ask only through an interactive UI and otherwise fail closed.

**PI-AUTH-04** When the extension or a per-session permission policy is installed, the system shall reject symlink trust boundaries and atomically write the normalized content to a regular file with owner-only permissions.

**PI-AUTH-05** When a trusted repository hook rejects a tool call, the extension shall block the call before applying auto or allowlist authorization.

**PI-AUTH-06** When the extension publishes readiness, the system shall expose the current AGM mode and ready or working state through one stable status token.

**PI-AUTH-07** When repository lifecycle hooks execute, the extension shall project shared hook metadata and shall enforce structured block results by consuming rejected user input, canceling rejected compaction, blocking rejected tools, or delivering bounded Stop feedback instead of relying only on process exit status; on POSIX, the extension shall launch each hook beneath a trusted detached supervisor group leader, send structured start and cleanup requests over an authenticated control channel that the hook cannot inherit, preserve the real hook status and bounded output, and require the live supervisor to terminate its own current process group on normal completion, output exhaustion, or timeout before a bounded output drain; the parent shall never signal a numeric process identity, shall accept cleanup completion only after both a token-bound acknowledgement and an observed supervisor `SIGKILL` exit, and shall fail closed and settle within a fixed bound after any other exit, control-channel loss, or supervisor-identity loss.

**PI-AUTH-08** When a successful repository hook emits additional context, the extension shall retain that context without skipping later hooks, and a later rejection shall still fail closed.

**PI-AUTH-09** When Pi reports completion of the conventional `subagent` extension tool, the extension shall project `SubagentStop` and shall return blocking remediation to the parent Pi turn through the bounded follow-up path.

**PI-AUTH-10** When AGM launches or resumes Pi with a resolved allowlist, the system shall pass an AGM-private policy file path instead of pasting policy JSON through the bounded terminal input queue, and the extension shall fail closed if that file is unreadable or malformed.

**PI-AUTH-11** When the extension publishes managed status for an AGM launch, the system shall append the caller-provided unique launch ID so lifecycle readiness can distinguish current process output from stale pane history.

**PI-AUTH-12** When a Pi Bash call contains unquoted shell control, redirection, command-substitution, or command-chaining syntax, the adapter shall not pre-approve the call from an AGM allowlist pattern and shall route it through the interactive or fail-closed unmatched decision.

## Traceability

- Package tests: `agm/internal/permissionparity/pi_test.go`
- BDD: `agm/test/bdd/features/permission_parity.feature`
