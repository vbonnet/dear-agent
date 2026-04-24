# AGM RBAC Permission Profiles

This document explains AGM's role-based access control (RBAC) permission profiles and how to use them to create sessions with pre-approved permissions.

## Overview

Permission profiles allow you to create AGM sessions with pre-approved permissions based on the agent's role and responsibilities. Instead of manually approving permissions during session startup, profiles apply a set of permissions automatically based on the session's purpose.

**Benefits**:
- **Reduced Permission Churn**: Eliminate repeated permission approval prompts
- **Least Privilege**: Each role gets only the permissions it needs
- **Faster Startup**: Pre-approved permissions mean sessions start without permission dialogs
- **Consistent Security**: Standardized permission sets across all sessions of the same role

## Available Profiles

### worker
**Purpose**: General-purpose code implementation with full build toolchain

**Pre-approved Permissions**:
- Full code read/write access: `Read(~/src/**)`, `Edit(~/src/**)`, `Write(~/src/**)`
- Git access: `Bash(git:*)`
- Build tools: `Bash(go:*)`, `Bash(make:*)`, `Bash(npm:*)`, `Bash(pip:*)`, `Bash(cargo:*)`
- tmux session management: `Bash(tmux:*)`
- AGM commands: `Bash(agm:*)`, `Skill(agm:*)`
- Safe shell utilities: grep, find, ls, cat, head, tail, wc, diff

**Use When**:
- Implementing features and fixing bugs
- Running build and test commands
- Managing git workflows
- Working with multiple tools in a single session

**Example**:
```bash
agm session new my-feature-work --permission-profile=worker
```

### researcher
**Purpose**: Investigation, analysis, and research document production

**Pre-approved Permissions**:
- Full code read/write access: `Read(~/src/**)`, `Edit(~/src/**)`, `Write(~/src/**)`
- Web research: `WebSearch(*)`, `WebFetch(*)`
- Git access: `Bash(git:*)`
- Build tools: `Bash(go:*)`, `Bash(make:*)`
- tmux session management: `Bash(tmux:*)`
- AGM commands: `Bash(agm:*)`, `Skill(agm:*)`

**Use When**:
- Researching external information via web search
- Analyzing logs and documentation
- Writing research documents
- Investigating technical problems
- Gathering requirements and context

**Example**:
```bash
agm session new research-spike --permission-profile=researcher
```

### orchestrator
**Purpose**: Session coordination, worker management, and permission approvals

**Pre-approved Permissions**:
- Code read access: `Read(~/src/**)`, `Glob(~/src/**)`, `Grep(~/src/**)`
- Full git access: `Bash(git:*)`
- Full tmux access: `Bash(tmux:*)`
- AGM orchestration commands: `Bash(agm:*)`, `Skill(agm:*)`
- Safe shell utilities: grep, find, ls, cat, head, tail, wc, diff

**Use When**:
- Coordinating multiple worker sessions
- Managing session permissions and approvals
- Monitoring and controlling session execution
- Running orchestration workflows
- Collecting results from multiple sessions

**Example**:
```bash
agm session new orchestrator-main --permission-profile=orchestrator
```

### auditor
**Purpose**: Periodic health/quality checks with read-only access

**Pre-approved Permissions**:
- Code read access: `Read(~/src/**)`, `Glob(~/src/**)`, `Grep(~/src/**)`
- Git read commands: `Bash(git log *)`, `Bash(git diff *)`, `Bash(git show *)`, `Bash(git blame *)`

**Use When**:
- Auditing code quality
- Reviewing commits and changes
- Checking compliance
- Running security scans
- Verifying code standards
- **Never**: Code modifications, destructive operations, or external API calls

**Example**:
```bash
agm session new code-audit --permission-profile=auditor
```

### verifier
**Purpose**: Validation work with test runners and read-only access

**Pre-approved Permissions**:
- Code read access: `Read(~/src/**)`, `Glob(~/src/**)`, `Grep(~/src/**)`
- Test runners: `Bash(go test *)`, `Bash(npm test *)`, `Bash(pytest *)`, `Bash(make test *)`, `Bash(cargo test *)`
- Safe git operations: `Bash(git log:*)`, `Bash(git show:*)`, `Bash(git diff:*)`
- AGM session monitoring: `Bash(agm session status *)`, `Bash(agm session send *)`
- Safe shell utilities: grep, find, ls, cat, head, tail, wc, diff

**Use When**:
- Running test suites
- Validating builds
- Verifying code changes
- Checking compatibility
- **Never**: Code modifications or deployments

**Example**:
```bash
agm session new test-verification --permission-profile=verifier
```

### requester
**Purpose**: Planning and coordination with read-only access

**Pre-approved Permissions**:
- Code read access: `Read(~/src/**)`, `Glob(~/src/**)`, `Grep(~/src/**)`
- Git read access: `Bash(git:*)`
- AGM session monitoring: `Bash(agm session status *)`, `Bash(agm session list *)`
- AGM skills: `Skill(agm:*)`

**Use When**:
- Requesting features or changes
- Writing requirements
- Planning work
- Creating specifications
- **Never**: Code modifications or tool execution

**Example**:
```bash
agm session new requirements-planning --permission-profile=requester
```

### monitor
**Purpose**: Session monitoring with tmux and git read access

**Pre-approved Permissions**:
- Code read access: `Read(~/src/**)`, `Glob(~/src/**)`, `Grep(~/src/**)`
- Full tmux access: `Bash(tmux:*)`
- Full git access: `Bash(git:*)`
- AGM commands: `Bash(agm:*)`, `Skill(agm:*)`

**Use When**:
- Monitoring active sessions
- Checking session status
- Viewing tmux output
- Reviewing git changes
- **Never**: Code modifications or permission approvals

**Example**:
```bash
agm session new session-monitor --permission-profile=monitor
```

## Usage

### Creating a Session with a Profile

Use the `--permission-profile` flag when creating a new session:

```bash
agm session new <session-name> --permission-profile=<profile-name>
```

### Combining Profiles with Other Permissions

You can combine a profile with explicit permissions:

```bash
# Start with worker profile, add Docker access
agm session new dev-session --permission-profile=worker \
  --permissions-allow='Bash(docker:*)' \
  --permissions-allow='Bash(docker-compose:*)'

# Start with researcher profile, add SSH access
agm session new research-session --permission-profile=researcher \
  --permissions-allow='Bash(ssh:*)'
```

### Inheriting Parent Permissions

Sessions can inherit permissions from the parent claude session:

```bash
# Inherit pre-approved permissions from ~/.claude/settings.json
agm session new inherited-session --permission-profile=worker \
  --inherit-permissions
```

## Trust Levels

Permission profiles are assigned trust levels that affect sandboxing and security policies:

| Trust Level | Profiles | Description |
|---|---|---|
| **TrustTrusted** (Level 4) | `orchestrator`, `meta-orchestrator`, `overseer` | Highest trust, full system access allowed |
| **TrustStandard** (Level 2) | `worker`, `researcher`, `monitor`, `implementer` | Standard trust for productive work |
| **TrustSandboxed** (Level 1) | `auditor`, `verifier`, `requester` | Minimal trust, restricted to specific operations |

Lower trust levels automatically enable stricter sandboxing and reduce access to sensitive system resources.

## Permission Pattern Format

Permissions use a pattern format that supports wildcards:

```
Bash(command:*)          # All subcommands of 'command'
Bash(command arg)        # Specific command with argument
Read(~/path/*)           # Read access to directory
Read(~/path/**)          # Recursive read access
Edit(~/path/**)          # Edit access (read + write existing files)
Write(~/path/**)         # Write access (create new files)
Bash(git:*)              # All git subcommands
Bash(go build:*)         # All 'go build' variants
WebSearch(*)             # Unrestricted web search
WebFetch(*)              # Unrestricted web fetch
```

## Best Practices

1. **Match Profile to Role**: Choose the profile that most closely matches the intended work
   - Don't use `worker` for auditing → use `auditor`
   - Don't use `orchestrator` for research → use `researcher`

2. **Use Minimal Permissions**: Start with a profile and add permissions only as needed
   - Avoid using the broadest profile just in case
   - Add specific permissions with `--permissions-allow` when profile is insufficient

3. **Document Permission Choices**: When adding custom permissions, document why
   - Use commit messages or session tags to track permission decisions
   - Helps during permission audits and security reviews

4. **Prefer Existing Profiles**: Before creating custom permission sets, check existing profiles
   - Most common use cases are covered by the built-in profiles
   - Custom permissions can drift from security best practices

5. **Inherit Carefully**: Use `--inherit-permissions` only when necessary
   - Inherited permissions bypass the profile's security boundaries
   - Document why inheritance is needed
   - Consider whether a less restrictive profile would be more appropriate

## Examples

### Research and Investigation
```bash
# Open-ended research with web access
agm session new spike-investigation --permission-profile=researcher

# Add Anthropic SDK for testing
agm session new api-research --permission-profile=researcher \
  --permissions-allow='Bash(pip:*)'
```

### Implementation and Development
```bash
# Feature implementation with all build tools
agm session new feature-branch --permission-profile=worker

# Bug fix with Docker testing
agm session new bug-fix --permission-profile=worker \
  --permissions-allow='Bash(docker:*)' \
  --permissions-allow='Bash(docker-compose:*)'
```

### Quality and Validation
```bash
# Code review and validation
agm session new code-review --permission-profile=verifier

# Security audit
agm session new security-audit --permission-profile=auditor

# Compliance check
agm session new compliance-check --permission-profile=auditor \
  --permissions-allow='Bash(git log:--all)' \
  --permissions-allow='Bash(git diff:*)'
```

### Orchestration
```bash
# Multi-worker orchestration
agm session new orchestrator --permission-profile=orchestrator

# Session monitoring
agm session new monitor --permission-profile=monitor
```

## Troubleshooting

### Permission Denied Errors

If you see permission denied errors during session execution:

1. **Check Current Profile**: Review which permissions the current profile provides
2. **Add Explicit Permission**: Use `--permissions-allow` to add missing permissions
3. **Consider Different Profile**: Choose a broader profile if multiple permissions are needed
4. **Check Session Tags**: Use tags to document custom permissions

Example:
```bash
# If a researcher session needs Docker access
agm session new research-docker --permission-profile=researcher \
  --permissions-allow='Bash(docker:*)' \
  --tags='cap:docker-enabled'
```

### Too Broad Permissions

If you find yourself adding many permissions to a profile:

1. **Review the Profile**: Might be using the wrong profile
2. **Consider Broader Profile**: Perhaps `worker` is more appropriate than `researcher`
3. **Document Exception**: Use tags to track why broader permissions are needed

## See Also

- [RBAC Architecture](adr/rbac-architecture.md) - Design decisions
- [Permission Patterns](reference/permission-patterns.md) - Complete pattern reference
- [User Guide](USER_GUIDE.md) - AGM general usage
- [Security Policy](../SECURITY.md) - Security guidelines
