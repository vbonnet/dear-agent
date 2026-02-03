---
hook_name: "mcp-wizard session-start"
hook_type: "session-start"
language: "TypeScript"
location: "~/src/ws/oss/repos/ai-tools/main/mcp-wizard/src/commands/session-start.ts"
created_at: "2026-01-17"
last_updated: "2026-02-02"
---

# mcp-wizard session-start - Specification

## Overview

**Purpose**: Proactive MCP authentication health check at shell session startup, providing early warning of token expiration and automated refresh capability.

**Trigger**: Shell initialization (.bashrc/.zshrc), manual invocation, or Claude Code session-start hook integration

**Critical Path**: No - This is an optional enhancement that improves UX but is not required for Claude Code operation

**Failure Mode**: Graceful degradation - logs warning but does not block shell startup or Claude Code session initialization. Exit code 1 indicates unhealthy tokens requiring user action.

---

## Functional Requirements

### FR1: MCP Configuration Discovery

**Description**: Discovers all Okta-authenticated MCP configurations from multiple sources (mcp-wizard config and Claude Code config) and merges them with deduplication.

**Inputs**:
- `~/.config/mcp-wizard/config.json`: mcp-wizard primary config file (JSON)
- `~/.claude/.mcp.json`: Claude Code MCP configuration (JSON, may reference mcp-wizard config via MCP_WIZARD_CONFIG env var)

**Outputs**:
- Array of `MCPConfig` objects containing: name, serviceName, oktaDomain, clientId, scopes, auth type
- Empty array if no configs found (graceful fallback)

**Success Criteria**:
- [ ] Reads both config sources without errors
- [ ] Filters for Okta-authenticated MCPs only (auth === 'okta')
- [ ] Deduplicates by serviceName (wizard config takes priority)
- [ ] Handles missing/invalid JSON gracefully (returns empty array)

**Error Handling**:
- Missing config files: Return empty array, show warning message
- Invalid JSON: Log warning to stderr, return empty array
- Missing required fields: Skip invalid MCP entries, continue with valid ones

### FR2: Health Check Execution

**Description**: Runs comprehensive health checks (Token Health, MCP Processes, Network Connectivity, Intent Analyzer) in parallel with 5-minute caching for performance.

**Inputs**:
- `force` option (boolean): Bypass cache and run fresh checks
- Discovered MCP configurations from FR1

**Outputs**:
- Array of `HealthCheckResult` objects with: name, status (healthy/degraded/unhealthy), message, details, last_check timestamp
- Cached results if available and not forced

**Success Criteria**:
- [ ] All 4 health checks complete within 5 seconds (with cache)
- [ ] Cache hit reduces execution time to <500ms
- [ ] Parallel execution via Promise.all()
- [ ] Token Health check covers all Okta MCPs (shared token)

**Error Handling**:
- Network timeout (3s): Mark endpoint as unreachable, continue with other checks
- Process check failure: Return unhealthy status, include error in details
- Token retrieval failure: Return unhealthy status, suggest re-authentication

### FR3: Status Formatting and Display

**Description**: Formats health check results into user-friendly output with color coding and actionable guidance.

**Inputs**:
- Array of `HealthCheckResult` from FR2
- `verbose` flag (boolean): Show detailed health info vs. summary
- Discovered MCP count from FR1

**Outputs**:
- Stdout: Colored status message (green checkmark, yellow warning, red error)
- Stderr: None (all output to stdout for shell startup compatibility)
- Exit code: 0 (healthy/degraded), 1 (unhealthy)

**Success Criteria**:
- [ ] Healthy: Green checkmark with MCP count (e.g., "✓ MCP Health: 4 MCPs authenticated")
- [ ] Degraded: Yellow warning with TTL (e.g., "⚠ MCP Health: Token expiring soon (3m)")
- [ ] Unhealthy: Red error with remediation (e.g., "✗ MCP Health: Token expired or invalid")
- [ ] Verbose mode shows: Token status, MCP list, expiration time

**Error Handling**:
- No MCPs configured: Yellow warning, suggest running `mcp-wizard setup`, exit 0
- Unknown status: Default to unhealthy, show generic error message

### FR4: Automatic Token Refresh

**Description**: Optionally refreshes expired Okta tokens using Device Flow (headless) or PKCE Flow (interactive) when `--auto-refresh` flag is provided.

**Inputs**:
- `autoRefresh` flag (boolean): Enable automatic token refresh
- Okta domain and client ID from config
- Current token health status from FR2

**Outputs**:
- Refreshed token stored in OS keychain (via auth.ts)
- Success/failure message to stdout
- Updated health check results after refresh
- Exit code: 0 (success), 1 (failure)

**Success Criteria**:
- [ ] Detects headless environment (SSH, tmux) and uses Device Flow
- [ ] Detects interactive environment and uses PKCE Flow
- [ ] Re-checks health after refresh to confirm success
- [ ] Provides manual fallback instructions on failure

**Error Handling**:
- Missing Okta config: Return false, log error, suggest running setup
- Device Flow timeout: Return false, show manual auth instructions
- Network error during refresh: Return false, suggest checking connectivity
- Token still unhealthy after refresh: Return false, suggest manual re-auth

---

## Non-Functional Requirements

### NFR1: Performance

**Target**: <500ms for cached health checks, <2s for fresh checks

**Rationale**: Shell startup should be imperceptible (<500ms). Fresh checks may contact Okta API (network latency).

**Measurement**:
- Time command wrapping: `time mcp-wizard session-start`
- Unit test benchmarks for config discovery (~10ms target)
- Integration test benchmarks for cached vs. fresh checks

### NFR2: Reliability

**Uptime Target**: 99.9% graceful degradation (never block shell startup)

**Failure Recovery**: All errors return exit 0 or 1 (never crash). Errors logged to stderr but shell continues loading.

### NFR3: Maintainability

**Code Complexity**: Target cyclomatic complexity <10 per function (currently achieved via small, focused functions)

**Test Coverage**: ≥80% statement coverage for session-start command and hooks module

**Documentation**:
- Inline JSDoc comments for all exported functions
- SESSIONSTART-HOOK.md user guide (comprehensive)
- This SPEC.md for implementation reference

---

## Interface Specification

### Command-Line Interface

**Invocation**:
```bash
mcp-wizard session-start [OPTIONS]
```

**Arguments**: None

**Options**:
- `--verbose`, `-v`: Show detailed health check results (token status, MCP list, expiration)
- `--auto-refresh`, `-a`: Automatically refresh expired tokens using Device/PKCE Flow
- `--help`, `-h`: Show help text

**Exit Codes**:
- `0`: Healthy or degraded (warning), safe to continue
- `1`: Unhealthy (requires user action to re-authenticate)

**Standard Output**:
```
# Healthy (exit 0)
✓ MCP Health: 4 MCPs authenticated

# Degraded (exit 0)
⚠ MCP Health: Token expiring soon (3m)
Run `mcp-wizard auth` to refresh

# Unhealthy (exit 1)
✗ MCP Health: Token expired or invalid
Run `mcp-wizard auth` to re-authenticate

# Verbose mode (exit 0)
MCP Health Status:
Token: ✓ authenticated (expires in 2h 15m)
MCPs configured: GoogleDocs, Atlassian, Slack, Glean
Expiration: 2h 15m
```

**Standard Error**:
```
# Only used for critical errors (config parsing, etc.)
⚠️  Invalid JSON in Claude Code config: Unexpected token } in JSON at position 123
```

### Environment Variables

**Read**:
- `HOME`: User home directory (for config path resolution)
- `MCP_WIZARD_CONFIG`: Override config path (optional, used by Claude Code integration)
- `NODE_ENV`: Detect test environment (optional)

**Set**: None (session-start is read-only)

### File System

**Reads**:
- `~/.config/mcp-wizard/config.json`: Primary mcp-wizard config (JSON)
- `~/.claude/.mcp.json`: Claude Code MCP config (JSON, optional)
- `~/.config/mcp-wizard/.health-cache.json`: Health check cache (JSON, 5min TTL)
- OS Keychain: Google OAuth token (via keytar library)

**Writes**:
- `~/.config/mcp-wizard/.health-cache.json`: Health check results cache (JSON, atomic write)
- OS Keychain: Refreshed token if `--auto-refresh` used (via auth.ts)

**Creates**: None (all files created by other commands: setup, health)

---

## Integration Points

### Claude Code Integration

**Hook Registration**: Via shell integration in `.bashrc`/`.zshrc`, not directly by Claude Code CLI

**Execution Context**: Shell initialization (user login, new terminal window/tab)

**Data Flow**:
```
[Shell startup] → [mcp-wizard session-start] → [Config discovery] → [Health checks]
→ [Format output] → [Display status] → [Optional auto-refresh] → [Exit 0/1]
```

### External Dependencies

**Required**:
- Node.js: ≥18.0.0 (TypeScript runtime, ES2022 features)
- npm packages (see package.json):
  - `commander`: ^12.0.0 (CLI framework)
  - `chalk`: ^5.3.0 (Terminal color output)
  - `ora`: ^7.0.0 (Spinner UI)
  - `keytar`: ^7.9.0 (OS keychain access for token storage)
  - `googleapis`: ^148.0.0 (Google OAuth token refresh)

**Optional**:
- `~/.config/mcp-wizard/config.json`: If missing, shows setup prompt
- `~/.claude/.mcp.json`: If missing, only uses wizard config

### Internal Module Dependencies

**config-discovery.ts**:
- `discoverMCPs()`: FR1 implementation
- `readWizardConfig()`: Reads mcp-wizard config
- `readClaudeConfig()`: Reads Claude Code config
- `mergeMCPs()`: Deduplicates configs

**health-checks.ts**:
- `runAllHealthChecks()`: FR2 implementation
- `checkTokenHealth()`: Validates Google OAuth token
- `checkMCPProcesses()`: Checks if MCP processes running via pgrep
- `checkNetworkConnectivity()`: Tests OAuth/API endpoints
- `checkIntentAnalyzer()`: Tests keyword matching accuracy

**health-cache.ts**:
- `getCached()`: Retrieve cached health results
- `setCached()`: Store health results with 5min TTL

**auth.ts**:
- `authenticate()`: OAuth flow (Device Flow or PKCE)
- `getOktaToken()`: Retrieve token from keychain

**user-config.ts**:
- `loadConfig()`: Load mcp-wizard config
- `saveConfig()`: Save config changes

---

## Test Specification

### Unit Tests

**Coverage Target**: ≥80% statement coverage

**Test Cases**:

#### TC1: Execute at session initialization
- **Scenario**: Verify hook executes health check on invocation
- **Given**: Mock health module returns healthy status
- **When**: `onSessionStart()` is called
- **Then**: Health check is invoked exactly once

#### TC2: Graceful config discovery failure
- **Scenario**: Missing config files should not throw errors
- **Given**: No config files exist
- **When**: `discoverMCPs()` is called
- **Then**: Returns empty array, logs warning, no exceptions

#### TC3: Health check caching
- **Scenario**: Cached results should be reused within 5min TTL
- **Given**: Fresh health check completed <5min ago
- **When**: `runAllHealthChecks({ force: false })` called
- **Then**: Cached results returned, no API calls made

#### TC4: Token TTL formatting
- **Scenario**: Convert seconds to human-readable format
- **Given**: Token TTL = 8100 seconds
- **When**: `formatExpiration()` called
- **Then**: Returns "2h 15m"

#### TC5: Verbose output formatting
- **Scenario**: Verbose mode shows detailed status
- **Given**: Token healthy with 135min TTL, 4 MCPs configured
- **When**: `formatHealthStatus(mcps, tokenHealth, true)` called
- **Then**: Outputs multi-line status with token, MCPs, expiration

#### TC6: Auto-refresh missing config
- **Scenario**: Auto-refresh fails gracefully if Okta config missing
- **Given**: Config missing `okta_domain` or `okta_client_id`
- **When**: `handleAutoRefresh()` called
- **Then**: Returns false, logs error, suggests running setup

### Integration Tests

**Scope**: Multi-module workflows (config discovery + health checks + formatting)

**Test Cases**:

#### ITC1: End-to-end healthy scenario
- **Scenario**: Full workflow with healthy token and MCPs
- **Components**: Config discovery → Health checks → Formatting → Exit
- **Given**: Valid config, healthy token (120min TTL), all MCP processes running
- **When**: `sessionStart({ verbose: false })` executed
- **Then**: Exit 0, stdout shows "✓ MCP Health: 4 MCPs authenticated"

#### ITC2: End-to-end degraded scenario
- **Scenario**: Token expiring soon (warning state)
- **Components**: Config discovery → Health checks → Formatting → Exit
- **Given**: Valid config, token expiring in 3min, all MCPs running
- **When**: `sessionStart({ verbose: false })` executed
- **Then**: Exit 0, stdout shows "⚠ MCP Health: Token expiring soon (3m)"

#### ITC3: End-to-end unhealthy scenario
- **Scenario**: Expired token requiring re-auth
- **Components**: Config discovery → Health checks → Formatting → Exit
- **Given**: Valid config, expired token, MCPs running
- **When**: `sessionStart({ verbose: false })` executed
- **Then**: Exit 1, stdout shows "✗ MCP Health: Token expired or invalid"

#### ITC4: Multi-source config merging
- **Scenario**: Merge wizard and Claude configs with deduplication
- **Components**: readWizardConfig() + readClaudeConfig() + mergeMCPs()
- **Given**: Wizard config has GoogleDocs, Claude config has GoogleDocs + Atlassian
- **When**: `discoverMCPs()` executed
- **Then**: Returns [GoogleDocs (wizard), Atlassian (Claude)] - wizard takes priority

### E2E Tests

**Scope**: Full CLI invocation with real filesystem and mocked network

**Test Cases**:

#### E2E1: Shell integration smoke test
- **Scenario**: Verify CLI works as shell hook
- **Setup**: Create test config at `~/.config/mcp-wizard/config.json`, mock keytar to return valid token
- **Execution**: `mcp-wizard session-start` via child_process.exec()
- **Verification**: Exit code 0, stdout contains "✓ MCP Health", execution time <2s
- **Cleanup**: Remove test config, clear mocks

#### E2E2: Auto-refresh workflow
- **Scenario**: Automatic token refresh on expired token
- **Setup**: Create test config, mock keytar to return expired token, mock authenticate() to succeed
- **Execution**: `mcp-wizard session-start --auto-refresh`
- **Verification**: Exit code 0, stdout shows "✓ Token refreshed successfully", new token stored
- **Cleanup**: Remove test config, clear mocks

#### E2E3: Performance benchmark (cached)
- **Scenario**: Verify cached health checks complete in <500ms
- **Setup**: Create test config, pre-populate cache with fresh results
- **Execution**: `time mcp-wizard session-start`
- **Verification**: Real time <500ms, cache hit logged (if --trace enabled)
- **Cleanup**: Remove test config and cache

### BDD Tests (Future Enhancement for Go Rewrite)

**Framework**: Ginkgo/Gomega (Go testing framework)

**Feature**: Session-start health monitoring

**Scenarios**:

#### Scenario 1: Healthy authentication state
```gherkin
Given I have 4 MCPs configured (GoogleDocs, Atlassian, Slack, Glean)
And my Google OAuth token is valid with 120 minutes remaining
When I run "mcp-wizard session-start"
Then the exit code should be 0
And stdout should contain "✓ MCP Health: 4 MCPs authenticated"
And the command should complete in less than 500ms
```

**Go Implementation Template**:
```go
Describe("Session Start Hook", func() {
    Context("when authentication is healthy", func() {
        It("should show success message and exit 0", func() {
            // Setup: Create test config, mock token
            cfg := createTestConfig(4) // 4 MCPs
            mockToken := createValidToken(120 * time.Minute)

            // Execute
            cmd := exec.Command("mcp-wizard", "session-start")
            output, err := cmd.CombinedOutput()
            exitCode := cmd.ProcessState.ExitCode()

            // Verify
            Expect(exitCode).To(Equal(0))
            Expect(string(output)).To(ContainSubstring("✓ MCP Health: 4 MCPs authenticated"))
        })
    })
})
```

---

## Edge Cases & Error Scenarios

### Edge Case 1: No MCPs Configured

**Description**: First-time user with no MCP setup yet

**Example**:
```bash
$ mcp-wizard session-start
⚠️  No MCP configuration found. Run `mcp-wizard setup` to configure MCPs.
```

**Expected Behavior**: Yellow warning, exit 0 (not an error), suggest setup command

**Test Coverage**: TC2 (unit test), E2E1 variant

### Edge Case 2: Mixed Health States

**Description**: Some MCPs healthy, others degraded (e.g., 2/4 processes alive)

**Example**:
```bash
# Token healthy, but only 2/4 MCP processes running
$ mcp-wizard session-start
⚠ MCP Health: Degraded (2/4 processes alive)
```

**Expected Behavior**: Degraded status (yellow warning), exit 0 (not blocking)

**Test Coverage**: ITC2 variant (integration test)

### Edge Case 3: Cache Corruption

**Description**: Health cache file exists but contains invalid JSON

**Example**:
```bash
# Corrupted cache file
$ cat ~/.config/mcp-wizard/.health-cache.json
{ "invalid": json content }

$ mcp-wizard session-start
# Silently falls back to fresh check, no error shown
```

**Expected Behavior**: Ignore cache, run fresh health check, overwrite cache with valid results

**Test Coverage**: TC3 variant (unit test for cache read failure)

### Error Scenario 1: Network Timeout

**Trigger**: OAuth endpoint unreachable (VPN down, network issue)

**Error Message**:
```
⚠ MCP Health: Network error (check connection)
```

**Recovery**: Exit 0 (degraded, not critical), suggest checking network/VPN

**Test Coverage**: ITC4 (integration test with mocked fetch timeout)

### Error Scenario 2: Keychain Access Denied

**Trigger**: OS keychain locked or permission denied (rare on macOS/Linux)

**Error Message**:
```
✗ MCP Health: No Google OAuth token found
Run `mcp-wizard auth` to re-authenticate
```

**Recovery**: Exit 1 (unhealthy), suggest running auth command

**Test Coverage**: TC6 (unit test with mocked keytar failure)

### Error Scenario 3: Auto-Refresh Device Flow Timeout

**Trigger**: User doesn't complete Device Flow within timeout (5min)

**Error Message**:
```
✗ Token refresh failed
Run `mcp-wizard auth` to re-authenticate manually
Error: Device Flow timeout
```

**Recovery**: Exit 1, provide manual auth instructions

**Test Coverage**: E2E2 variant (with mocked authenticate() timeout)

---

## Performance Characteristics

### Benchmarks

**Typical Case** (cached health check):
- **Input**: 4 MCPs configured, cache valid (<5min old)
- **Expected Time**: <500ms total
  - Config discovery: ~10ms
  - Cache retrieval: ~5ms
  - Formatting: ~5ms
  - Display: ~5ms
- **Measurement**: Jest benchmark test with `performance.now()`

**Worst Case** (fresh health check):
- **Input**: 4 MCPs configured, cache expired or `--force` used
- **Expected Time**: <2s total
  - Config discovery: ~10ms
  - Token health check: ~50ms (keychain read)
  - MCP process checks: ~100ms (4 pgrep calls)
  - Network connectivity: ~500ms (3 endpoints, parallel)
  - Intent analyzer: ~10ms (local computation)
  - Formatting: ~5ms
- **Measurement**: E2E benchmark test with `time` command

### Resource Usage

**Memory**: <50MB RSS (Node.js process overhead + small data structures)

**CPU**: <100ms total CPU time (mostly I/O bound: network, keychain, fs)

**Disk I/O**:
- 2-3 reads (config files + cache)
- 1 write (cache update if fresh check)
- All <5KB files (negligible disk impact)

**Network**:
- 0 requests if cached (<5min old)
- 3 HEAD requests if fresh (Okta, Google, Atlassian endpoints)
- <1KB total payload

---

## Security Considerations

### Input Validation

**Untrusted Inputs**:
- Config files (`~/.config/mcp-wizard/config.json`, `~/.claude/.mcp.json`): JSON schema validation, graceful degradation on invalid JSON
- Environment variables (`MCP_WIZARD_CONFIG`): Path validation, no shell expansion
- Cache file (`~/.config/mcp-wizard/.health-cache.json`): JSON parsing with try/catch, regenerate on corruption

**Sanitization**: All user-facing strings (config values) are displayed as-is but never executed as shell commands. No eval() or similar dynamic code execution.

### Privilege Requirements

**Required Permissions**:
- Read `~/.config/mcp-wizard/` and `~/.claude/` (user-owned directories)
- Read/write OS keychain (via keytar, requires user's keychain password on first access)
- Execute `pgrep` command (standard Unix utility, no sudo required)

**Privilege Escalation**: None. Runs as user, never requests sudo or elevated privileges.

### Vulnerability Surface

**Attack Vectors**:
- Config file tampering: User can modify their own config (expected behavior, not a vulnerability)
- Cache poisoning: Cache file is user-owned and only used for performance (not security-critical)
- Keychain compromise: Relies on OS keychain security (macOS Keychain, Linux Secret Service)

**Mitigations**:
- No remote code execution: All code is local, no downloads or external scripts
- No shell injection: All commands use child_process.execFile() with array args (no shell expansion)
- Token security: Tokens stored in OS keychain, never logged or displayed in plaintext

---

## Maintenance & Troubleshooting

### Common Issues

#### Issue 1: "No MCP configuration found"

**Symptoms**: Warning message on every shell startup

**Diagnosis**: Check if `~/.config/mcp-wizard/config.json` exists

**Resolution**: Run `mcp-wizard setup` to create config

#### Issue 2: "Token expired or invalid"

**Symptoms**: Red error message on shell startup

**Diagnosis**: Check token via `mcp-wizard health --verbose`

**Resolution**: Run `mcp-wizard auth` or `mcp-wizard session-start --auto-refresh`

#### Issue 3: Slow shell startup (>1s delay)

**Symptoms**: Terminal takes noticeably longer to load

**Diagnosis**: Cache miss or network latency. Check if cache exists and is fresh.

**Resolution**:
- Cache should auto-populate after first run
- Use silent mode to suppress output: `mcp-wizard session-start 2>/dev/null || true`
- Check network connectivity if consistently slow

### Logging

**Log Location**:
- Normal mode: No logs (output to stdout only)
- Trace mode: `--trace` flag sends logs to stderr
- File logging: `--trace --log-file debug.log` writes JSON Lines format

**Log Level**:
- Default: Minimal (status messages only)
- Verbose: `--verbose` shows detailed health check results
- Trace: `--trace` shows OAuth flows, cache hits, timing info

**Log Format**:
```
# Stdout (normal mode)
✓ MCP Health: 4 MCPs authenticated

# Stderr (trace mode)
[2026-02-02T21:00:00.123Z] [INFO] Config discovery started
[2026-02-02T21:00:00.145Z] [DEBUG] Found 4 MCPs in wizard config
[2026-02-02T21:00:00.150Z] [INFO] Health check cache hit (age: 2m)
```

### Debugging

**Enable Debug Mode**:
```bash
# Trace to stderr
mcp-wizard session-start --trace

# Trace to file
mcp-wizard session-start --trace --log-file /tmp/debug.log

# Verbose output (detailed health status)
mcp-wizard session-start --verbose

# Force fresh check (bypass cache)
mcp-wizard health --force
```

**Debug Output**:
- Trace mode shows: config discovery, cache hits/misses, API call timing, OAuth flows
- Verbose mode shows: token expiration time, MCP list, network endpoint status

**Debug Flags**:
- `--verbose`: Detailed health status
- `--trace`: Low-level execution trace (from index.ts)
- `--force`: Bypass cache (via `health` command, not `session-start`)

---

## Migration Notes: TypeScript → Go (Phase 3)

### TypeScript-Specific Features Requiring Go Equivalents

**1. npm Package Dependencies**:
- `commander` (CLI framework) → `cobra` or `urfave/cli` in Go
- `chalk` (terminal colors) → `fatih/color` or `gookit/color`
- `ora` (spinner UI) → `briandowns/spinner`
- `keytar` (OS keychain) → `zalando/go-keyring` or `99designs/keyring`
- `googleapis` (Google OAuth) → `golang.org/x/oauth2` + `google.golang.org/api`

**2. Async/Await Patterns**:
- TypeScript uses `async/await` extensively for I/O operations
- Go uses goroutines + channels or sync.WaitGroup for parallel execution
- Example: `Promise.all()` for parallel health checks → Go `errgroup` pattern

**3. JSON Parsing**:
- TypeScript uses dynamic `any` types for config parsing
- Go requires struct definitions with json tags
- Need to define Go structs for: MCPConfig, HealthCheckResult, Config schema

**4. OS Keychain Access**:
- `keytar` library provides cross-platform keychain access (macOS Keychain, Linux Secret Service, Windows Credential Manager)
- Go equivalent: `zalando/go-keyring` (simpler) or `99designs/keyring` (more features)
- Must maintain compatibility with existing token storage format

**5. Process Management**:
- TypeScript uses `child_process.exec()` for pgrep
- Go can use `os/exec` package with similar semantics
- Example: `exec.Command("pgrep", "-f", pattern).Output()`

### Go Library Replacements

| TypeScript Package | Go Library | Notes |
|-------------------|------------|-------|
| commander | cobra | Industry standard for Go CLIs |
| chalk | fatih/color | Simpler API, similar features |
| ora | briandowns/spinner | Spinner UI with customization |
| keytar | zalando/go-keyring | Cross-platform, simpler than 99designs |
| googleapis | golang.org/x/oauth2 | Google's official Go OAuth library |
| inquirer | AlecAivazis/survey | Interactive prompts (if needed) |
| node:fs/promises | os + io/ioutil | Standard library file I/O |
| node:child_process | os/exec | Standard library process execution |

### MCP Protocol Details for Go Implementation

**1. Health Check Response Format**:
```typescript
interface HealthCheckResult {
  name: string;                     // Go: string
  status: 'healthy' | 'degraded' | 'unhealthy'; // Go: custom HealthStatus type (iota enum)
  message: string;                  // Go: string
  details?: Record<string, any>;    // Go: map[string]interface{} or custom struct
  last_check: Date;                 // Go: time.Time
}
```

**2. MCP Config Schema**:
```typescript
interface MCPConfig {
  name: string;           // Go: string
  serviceName: string;    // Go: string
  oktaDomain: string;     // Go: string
  clientId: string;       // Go: string
  scopes: string[];       // Go: []string
  auth: string;           // Go: string
}
```

**3. Cache Format**:
- TypeScript: JSON file with Date objects serialized as ISO strings
- Go: JSON file with time.Time using RFC3339 format
- Must maintain backward compatibility during transition (read TS cache, write Go cache)

**4. Token Storage Format**:
- keytar stores tokens as strings in OS keychain
- Service name: "okta" (misnomer, actually Google OAuth)
- Account name: From config (e.g., "company.okta.com")
- Must preserve exact service/account names for compatibility

### Test Migration Strategy

**1. Unit Tests** (TypeScript Jest → Go testing package):
```go
// TypeScript Jest
it('should discover MCPs from wizard config', async () => {
  const mcps = await readWizardConfig();
  expect(mcps).toHaveLength(4);
});

// Go equivalent
func TestReadWizardConfig(t *testing.T) {
  mcps, err := ReadWizardConfig()
  assert.NoError(t, err)
  assert.Len(t, mcps, 4)
}
```

**2. Integration Tests** (Jest → Ginkgo/Gomega BDD):
```go
// TypeScript Jest
describe('End-to-end healthy scenario', () => {
  it('should exit 0 with success message', async () => {
    // ...
  });
});

// Go Ginkgo
var _ = Describe("Session Start Integration", func() {
  Context("when authentication is healthy", func() {
    It("should exit 0 with success message", func() {
      // ...
    })
  })
})
```

**3. E2E Tests** (Jest + child_process → Go os/exec):
```go
// TypeScript
const { stdout, exitCode } = await exec('mcp-wizard session-start');

// Go equivalent
cmd := exec.Command("mcp-wizard", "session-start")
output, err := cmd.CombinedOutput()
exitCode := cmd.ProcessState.ExitCode()
```

**4. Mocking Strategy**:
- TypeScript: Jest mocks for keytar, googleapis, fs
- Go: Interfaces + mock implementations (e.g., KeychainInterface, ConfigLoaderInterface)
- Example: Define `type KeychainProvider interface { Get(service, account) (string, error) }`

### Build & Distribution

**TypeScript**:
- Build: `npm run build` (tsc compiles to dist/)
- Distribution: npm package, global install via `npm install -g`
- Entry point: `bin/mcp-wizard.js` → `#!/usr/bin/env node`

**Go**:
- Build: `go build -o mcp-wizard cmd/mcp-wizard/main.go`
- Distribution: Single binary, no runtime dependencies
- Entry point: Compiled binary (no shebang needed)
- Cross-compilation: `GOOS=linux GOARCH=amd64 go build`

**Migration Path**:
1. Create Go implementation alongside TypeScript (parallel development)
2. Add integration tests comparing TS and Go outputs (equivalence testing)
3. Feature flag to switch between TS and Go implementations
4. Deprecate TypeScript version after Go reaches feature parity
5. Remove TypeScript code in final cleanup phase

---

## Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-01-17 | Initial implementation (Phase 4-v2) | mcp-wizard team |
| 1.1 | 2026-02-02 | SPEC.md created for Phase 2 documentation | Claude Sonnet 4.5 |

---

## References

**Related Documents**:
- Implementation: `~/src/ws/oss/repos/ai-tools/main/mcp-wizard/src/commands/session-start.ts`
- Tests: `~/src/ws/oss/repos/ai-tools/main/mcp-wizard/tests/unit/hooks/session-start.test.ts`
- User Guide: `~/src/ws/oss/repos/ai-tools/main/mcp-wizard/docs/SESSIONSTART-HOOK.md`
- Main README: `~/src/ws/oss/repos/ai-tools/main/mcp-wizard/README.md`

**External References**:
- MCP Protocol Spec: https://modelcontextprotocol.io/
- Google OAuth 2.0: https://developers.google.com/identity/protocols/oauth2
- Commander.js: https://github.com/tj/commander.js
- Jest Testing: https://jestjs.io/
- Ginkgo (Go BDD): https://onsi.github.io/ginkgo/

---

## Notes

**TypeScript → Go Migration Complexity**: MEDIUM-HIGH

**Rationale**:
- **Medium complexity**: Core logic is straightforward (config discovery, health checks, formatting)
- **High complexity**: OAuth integration (googleapis dependency), OS keychain access (keytar), parallel async patterns

**Migration Challenges**:
1. **OAuth Token Management**: TypeScript uses `googleapis` library with automatic token refresh. Go requires manual OAuth 2.0 flow implementation or finding equivalent library.
2. **OS Keychain**: `keytar` provides cross-platform keychain access. Go's `zalando/go-keyring` is simpler but may have compatibility issues. Extensive testing required.
3. **Async Patterns**: TypeScript's `async/await` and `Promise.all()` are idiomatic. Go's goroutines require careful error handling (errgroup pattern).
4. **Config Parsing**: TypeScript uses dynamic `any` types for JSON. Go requires explicit struct definitions (more boilerplate but better type safety).

**Recommended Approach**:
1. Start with config discovery module (lowest complexity, no external dependencies)
2. Implement health-checks module with mocked dependencies
3. Tackle OAuth integration last (highest risk, requires most testing)
4. Create equivalence tests comparing TS and Go outputs before full migration

**Future Enhancements**:
- **V2: MCP-specific health checks**: Check GitHub/Atlassian MCPs separately (not just Okta token)
- **V3: Health check plugins**: Allow custom health checks via plugin system
- **V4: Remediation automation**: Auto-fix common issues (restart dead processes, clear stale cache)

**Known Limitations**:
- **V1: Okta-only**: Only checks Okta-authenticated MCPs (GoogleDocs, Atlassian, Slack, Glean). GitHub and other OAuth MCPs not covered.
- **V1: Process existence only**: Cannot ping MCP processes via stdio (process ownership constraint). May show "alive" for hung processes.
- **V1: Single-token assumption**: Assumes all Okta MCPs share one Google OAuth token (true for current setup but fragile).
