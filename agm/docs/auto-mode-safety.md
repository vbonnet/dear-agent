# Auto Mode Safety Guide for AGM

## What is Auto Mode?

Auto mode eliminates interactive permission prompts by routing tool-use decisions
through a background classifier. Instead of asking you to approve each file edit or
shell command, the classifier decides whether the action is consistent with what you
asked the agent to do.

**Requirements:**

- Anthropic Team plan (or higher)
- Claude Sonnet 4.6 or Claude Opus 4.6
- Not available on Haiku or any claude-3 model family

## How Auto Mode Interacts with Permission Rules

Auto mode does not replace your permission rules -- it layers on top of them.
The resolution order matters:

1. **Deny rules are ALWAYS respected.** Deny rules resolve before the classifier
   ever sees the action. If you deny a tool, auto mode will not bypass it. Period.

2. **Blanket allow rules are dropped on entry.** When auto mode activates, broad
   allow rules such as `Bash(*)`, `Bash(python*)`, and `Agent` rules are removed.
   Narrow, command-specific rules like `Bash(npm test)` are preserved.

3. **The classifier handles the rest.** Any action not resolved by a deny or a
   surviving narrow allow rule is sent to the background classifier, which checks
   whether the action is consistent with the user's request.

## AGM Usage

```bash
# Create a session in auto mode
agm session new my-session --harness claude-code --mode auto

# Switch a running session to auto mode
agm send mode auto my-session

# Switch back to default (interactive) mode
agm send mode default my-session
```

## Safety Recommendations

- **Keep deny rules for destructive operations.** Examples: `git push --force`,
  `rm -rf /`, database drops, production deploys. Deny rules are the strongest
  guarantee -- they cannot be overridden by auto mode.

- **Understand the protection spectrum.** Auto mode provides more protection than
  `bypassPermissions` (which skips all checks) but less than full manual review
  (where you approve every action).

- **Know what the classifier blocks by default:**
  - Piping remote scripts to a shell (`curl|bash`)
  - Production deployments
  - Mass file deletion
  - Force-pushing to protected branches

- **Know what the classifier allows by default:**
  - Local file reads and writes within the project
  - Installing declared dependencies (package.json, requirements.txt, etc.)
  - Pushing to the current working branch

## Fallback Behavior

The classifier includes circuit-breaker logic to prevent runaway sessions:

| Trigger | Threshold | Effect |
|---|---|---|
| Consecutive blocks | 3 in a row | Auto mode pauses |
| Total blocks | 20 in a session | Auto mode pauses |

**Interactive sessions (TUI):** A notification appears in the status area. Approve
the prompted action to reset the block counters and resume.

**Non-interactive sessions (`-p` flag):** The session aborts. Design your prompts
to stay within the classifier's comfort zone, or add narrow allow rules for
expected operations.

## Configuration

Administrators can extend the classifier's trust boundary for known-safe
infrastructure:

- Set `autoMode.environment` in managed settings to declare trusted tools, paths,
  or commands specific to your organization.

- View the default classifier rules at any time:

```bash
claude auto-mode defaults
```
