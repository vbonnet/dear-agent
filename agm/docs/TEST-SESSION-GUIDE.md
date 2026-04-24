# Test Session Guide

## Quick Reference

### Isolated Test Sessions (Recommended)

Create ephemeral test sessions that won't clutter your production workspace:

```bash
# Creates in ~/sessions-test/ - isolated from production
agm session new --test my-experiment
```

**Benefits:**
- Isolated in `~/sessions-test/` directory
- Tmux session prefixed with `agm-test-`
- Not tracked in AGM database (ephemeral)
- Automatically cleaned up by test infrastructure
- Perfect for quick experiments, CI/CD tests, temporary work

### Test Workspace (Alternative)

For longer-running test sessions that need persistence:

```bash
# Creates in test workspace (still tracked in database)
agm session new my-test --workspace=test
```

**Benefits:**
- Tracked in AGM database for history
- Organized in dedicated test workspace
- Useful for multi-day test projects

### Legitimate Test Names in Production

Sometimes you need a production session about testing a feature:

```bash
# Session about testing a feature (legitimate work)
agm session new my-test-feature  # OK - doesn't contain "test" as standalone word

# If you MUST use "test" in production session name — two options:

# Option A: interactive (human only) — omit flag and choose option 3 at the prompt
agm session new test-auth-flow
# > Use --test flag (required for test scenarios)
#   Cancel and rename to non-test name
# > Create anyway (production session, human override)  ← select this

# Option B: non-interactive / scripted — use the flag directly
agm session new test-auth-flow --allow-test-name
```

**Warning:** Use production overrides sparingly. Most test-related work should use `--test` flag.

## Comparison Table

| Method | Location | Database Tracked | Tmux Prefix | Best For |
|--------|----------|------------------|-------------|----------|
| `--test` flag | `~/sessions-test/` | No | `agm-test-` | Quick experiments, CI/CD tests, throwaway work |
| `--workspace=test` | `~/src/ws/test/` | Yes | `agm-` | Longer test sessions needing persistence |
| `--allow-test-name` | `~/.claude/sessions/` | Yes | `agm-` | Production work about testing (rare) |
| Production | `~/.claude/sessions/` | Yes | `agm-` | Real work (default) |

## Test Pattern Detection

AGM automatically detects when session names contain "test" and prompts you to use `--test` flag:

```bash
$ agm session new test-experiment

⚠️  Test Pattern Detected - Action Required

Session name 'test-experiment' contains 'test' but --test flag not set.

❌ Production workspace blocked for test sessions

Why this matters:
  • Test sessions pollute production workspace
  • Appear in 'agm session list' forever
  • Create data cleanup burden

Options:
  1. Use --test flag → Isolated test workspace
  2. Rename session → Remove 'test' from name
  3. Force production creation (human override only)

For scripts: MUST use --test flag explicitly

> Use --test flag (required for test scenarios)
  Cancel and rename to non-test name
  Create anyway (production session, human override)
```

## Examples

### Quick Experiment

```bash
# Start isolated test session
agm session new --test quick-api-check

# Work in session...

# Exit and forget - automatically cleaned up
agm session kill quick-api-check
```

### CI/CD Testing

```bash
#!/bin/bash
# test-runner.sh

# Create test session (exits if name doesn't use --test)
agm session new --test ci-validation-$BUILD_ID --detached

# Run tests in session
agm session exec ci-validation-$BUILD_ID "npm test"

# Cleanup
agm session kill ci-validation-$BUILD_ID
```

### Persistent Test Project

```bash
# Create test workspace session (tracked, persistent)
agm session new integration-test-suite --workspace=test

# Work over multiple days...

# Archive when done
agm session archive integration-test-suite
```

### Legitimate Production Session

```bash
# Working on authentication testing feature
agm session new auth-testing-implementation  # No prompt - "test" not standalone

# If you really need "test-" prefix in production
agm session new test-harness-refactor --allow-test-name
```

## Cleanup

If you have existing test sessions in production workspace, use the cleanup command:

```bash
# Preview sessions that would be cleaned
agm admin cleanup-test-sessions --dry-run

# Interactively select and clean up test sessions
agm admin cleanup-test-sessions

# Automated cleanup (scripts only)
agm admin cleanup-test-sessions --auto-yes --message-threshold 5
```

**Note:** Cleanup creates backups in `~/.agm/backups/sessions/` before deletion.

## Migration Guide

### If You Have Existing Test Sessions

1. **Audit current test sessions:**
   ```bash
   agm admin cleanup-test-sessions --dry-run
   ```

2. **Review message counts:**
   - Sessions with < 5 messages are usually safe to delete
   - Sessions with significant conversation should be preserved

3. **Execute cleanup:**
   ```bash
   agm admin cleanup-test-sessions
   ```
   Interactive prompt lets you select which sessions to delete.

4. **Verify cleanup:**
   ```bash
   agm session list --workspace oss | grep test
   # Should return no results
   ```

### Going Forward

**DO:**
- ✅ Use `--test` flag for all test scenarios
- ✅ Use descriptive names: `my-feature-testing` instead of `test-feature`
- ✅ Use `--workspace=test` for multi-day test projects

**DON'T:**
- ❌ Create sessions named `test-*` without `--test` flag
- ❌ Override test pattern detection unless absolutely necessary
- ❌ Let test sessions accumulate in production workspace

## Technical Details

### Test Session Isolation

When using `--test` flag:

1. **Session Directory:** Created in `~/sessions-test/` instead of `~/.claude/sessions/`
2. **Tmux Session Name:** Prefixed with `agm-test-` (e.g., `agm-test-my-experiment`)
3. **Database:** Not tracked in AGM database (ephemeral)
4. **Cleanup:** Automatically removed by test infrastructure

### Pattern Detection

AGM detects test patterns using case-insensitive substring matching:

- ✅ **Triggers prompt:** `test-foo`, `foo-test`, `Test`, `MyTest`, `testing`
- ❌ **Does not trigger:** `latest`, `contest`, `attest` (no standalone "test")

### Override Flags

- `--test`: Creates isolated test session (recommended)
- `--allow-test-name`: Bypasses test pattern detection for scripts/non-interactive use
- **Interactive option 3** ("Create anyway"): Human-only TUI override, equivalent to `--allow-test-name`

**Priority:** `--test` flag takes precedence over `--allow-test-name` and interactive option 3

## Troubleshooting

### "Test Pattern Detected" prompt appearing unexpectedly

**Cause:** Session name contains "test" substring (case-insensitive).

**Solutions:**
1. Add `--test` flag: `agm session new --test my-session`
2. Rename without "test": `agm session new my-experiment`
3. Override (rare): `agm session new test-name --allow-test-name`

### Hook blocking session creation

**Cause:** PreToolUse hook detected test pattern without `--test` flag.

**Solutions:**
1. Add `--test` flag to command
2. Rename session to avoid "test" substring
3. Add `--allow-test-name` flag

### Cannot find test sessions

**Cause:** Test sessions created with `--test` are in `~/sessions-test/`, not production workspace.

**Solution:**
```bash
# List test sessions
ls ~/sessions-test/

# Or use tmux directly
tmux ls | grep agm-test-
```

## Best Practices

1. **Use descriptive names:** `auth-flow-debugging` instead of `test1`
2. **Isolate experiments:** Always use `--test` for throwaway work
3. **Clean up regularly:** Run cleanup command monthly
4. **Document long-running tests:** Use `--workspace=test` with descriptive names
5. **Respect the guardrails:** If AGM prompts about test pattern, there's probably a better approach

## See Also

- [Session Lifecycle Guide](SESSION-LIFECYCLE.md)
- [Workspace Management](WORKSPACE.md)
- [Cleanup Command Reference](../cmd/agm/CLEANUP.md)
