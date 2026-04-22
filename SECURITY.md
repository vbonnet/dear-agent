# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest (main) | ✅ |
| older releases | ❌ |

We only actively fix security issues in the latest release. If you are running an older version, upgrade before reporting.

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Report security issues by emailing the maintainers directly or by opening a [private security advisory](https://github.com/vbonnet/dear-agent/security/advisories/new) on GitHub.

Include:
- A description of the vulnerability
- Steps to reproduce
- The impact you believe it has
- Any suggested fix if you have one

We will acknowledge the report within 72 hours and provide an estimated timeline for a fix.

## Scope

Issues we consider in scope:

- Credential or secret exposure in logs, outputs, or generated files
- Privilege escalation through agent sessions or hooks
- Injection vulnerabilities in shell command construction
- Information disclosure from session metadata or event logs
- Bypass of the circuit breaker or safety gates in ways that could cause harm

Issues out of scope:

- Vulnerabilities in underlying AI provider CLIs (Claude Code, Gemini CLI, etc.) — report those to their respective projects
- Issues requiring physical access to the host machine
- Social engineering attacks against project maintainers

## Security Design Notes

dear-agent runs with the same privileges as the user who invokes it. It:
- Does not run as root
- Does not open network listeners by default (the MCP server binds to localhost only)
- Does not store credentials — API keys are read from environment variables and never logged
- Enforces an audit trail via `dear-diary` (append-only JSONL, never automatically deleted)

Hooks are shell scripts that execute with user-level permissions. Review any third-party hooks before installing them.
