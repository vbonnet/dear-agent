# AGM Tests

This directory contains tests for the Agent Session Manager (AGM).

## Test Types

### Unit Tests (Automated)

Unit tests are located alongside the source code and can be run with:

```bash
cd main/agm
go test ./... -v
```

### Manual Integration Tests (NOT for CI/CD)

Integration tests that require tmux, Claude CLI, and interactive sessions.

**⚠️ These tests are NOT run automatically in CI/CD pipelines.**

#### `manual-e2e-test.sh`

End-to-end integration test that validates core AGM functionality.

#### `manual-e2e-huh-test.sh`

End-to-end integration test that validates the charmbracelet/huh UI migration.

**What it tests:**
- Build verification with huh dependencies
- Custom UI file deletion (spinner.go, prompts.go)
- Code migration completeness (old references removed)
- Spinner migration (huh.NewSpinner in new.go, resume.go)
- Unit tests passing with huh
- Full workflow integration

**Prerequisites:**
- `tmux` installed and accessible
- `claude` CLI command available
- `agm` binary built with huh integration and test subcommands

**How to run:**

```bash
# From the AGM repository root
./tests/manual-e2e-huh-test.sh
```

**When to run:**
- After making changes to UI code (spinners, prompts)
- Before releases to validate huh integration
- To verify migration completeness

---

#### `manual-e2e-test.sh` (Core AGM Functionality)

**What it tests:**
- Session creation with tmux
- UUID association
- Session archiving
- Archive directory scanning (validates fix for invisible archived sessions)

**Prerequisites:**
- `tmux` installed and accessible
- `claude` CLI command available
- `agm` binary built and in PATH (`~/go/bin/agm`)

**How to run:**

```bash
# From the AGM repository root
./tests/manual-e2e-test.sh
```

**Expected output:**
```
╔════════════════════════════════════════════════════════════╗
║       AGM End-to-End Manual Integration Test              ║
╚════════════════════════════════════════════════════════════╝

>>> Checking prerequisites
✓ tmux found: tmux 3.4
✓ claude command found
✓ agm found: agm version 2.0.0-dev

>>> Test 1: Create tmux session and start Claude
✓ Tmux session created
✓ Claude started in tmux session
✓ Manifest file exists

>>> Test 2: Associate UUID with session
✓ UUID associated successfully
✓ UUID verified in manifest

>>> Test 3: Archive session
✓ Session archived successfully
✓ Session moved to .archive-old-format/
✓ Manifest lifecycle field set to 'archived'

>>> Test 4: Verify archived session visibility
✓ Archived session appears in 'agm list --all'
✓ Archived session hidden from 'agm list' (correct)
✓ agm get-uuid works for archived session

╔════════════════════════════════════════════════════════════╗
║               ALL TESTS PASSED ✓                           ║
╚════════════════════════════════════════════════════════════╝
```

**When to run:**
- After making changes to archive functionality
- After modifying manifest scanning logic
- Before releases to validate core workflows
- To validate fixes (e.g., commit 77a754e fixed archive visibility)

**Exit codes:**
- `0` - All tests passed
- `1` - Test failed
- `2` - Prerequisites not met

**Cleanup:**
The script automatically cleans up test sessions on exit (success or failure).

## Adding New Tests

### Unit Tests
Add Go test files alongside source code:
- `internal/manifest/manifest_test.go`
- `cmd/csm/associate_test.go`

### Manual Integration Tests
1. Create script in `tests/` with prefix `manual-`
2. Add `#!/bin/bash` shebang
3. Include clear warning: `⚠️ DO NOT RUN IN CI/CD`
4. Document in this README
5. Make executable: `chmod +x tests/manual-*.sh`

## CI/CD Integration

**Automated tests only:** CI/CD pipelines run `go test ./...`

**Manual tests excluded:** Any test prefixed with `manual-` is excluded from automated runs.
