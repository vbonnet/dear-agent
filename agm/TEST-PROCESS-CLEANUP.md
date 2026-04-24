# Test Process Cleanup - Automated Mitigation

## Problem
Go tests spawn Claude processes with `claude --add-dir /tmp/go-test-script*`
that occasionally fail to terminate, leading to:
- Memory exhaustion (36-85 orphaned processes = 12-30GB RAM)
- System load spikes (454-490)
- SSH lag and freezes
- OOM kills and crashes

## Incidents
- **2026-02-11 21:30**: 85 orphaned processes, load 454
- **2026-02-11 22:45**: 36 orphaned processes, load 490

## Automatic Cleanup (Temporary Mitigation)

**Script:** ~/bin/cleanup-orphaned-tests.sh
**Schedule:** Every 5 minutes via cron
**Action:** `pkill -9 -f "claude --add-dir /tmp/go-test-script"`
**Logs:** ~/monitoring-logs/test-cleanup.log

This is a **temporary workaround** until root cause is fixed.

## Root Cause (To Be Fixed)

Test processes are not terminating properly after tests complete.
Likely causes:
1. Test timeout not enforced
2. Process cleanup not in test teardown
3. Signal handling issues in test framework
4. Parent process dies before cleaning up children

## TODO: Fix Test Lifecycle

### Investigation Needed:
- [ ] Why do test processes outlive their parent tests?
- [ ] Are tests timing out or completing normally?
- [ ] Is cleanup code in teardown being skipped?
- [ ] Do we need explicit process group cleanup?

### Proposed Fixes:
1. **Add explicit cleanup in test teardown**
   - Track spawned Claude PIDs
   - Kill on test completion or panic
   - Use defer for cleanup

2. **Add test timeouts**
   - Individual test timeout: 2 minutes
   - Overall suite timeout: 15 minutes
   - Force kill on timeout

3. **Use process groups**
   - Create new process group for each test
   - Kill entire group on cleanup
   - Prevents orphaning

4. **Add monitoring to tests**
   - Log when Claude processes spawn
   - Log when they should terminate
   - Alert on orphans

### Example Fix (Go):
```go
func TestSomething(t *testing.T) {
    cmd := exec.Command("claude", "--add-dir", tmpDir)
    cmd.Start()
    
    // Track for cleanup
    defer func() {
        if cmd.Process != nil {
            cmd.Process.Kill()
            cmd.Wait()
        }
    }()
    
    // Test logic...
}
```

## Monitoring
Automated monitoring logs orphan count every minute:
- File: ~/monitoring-logs/system-monitor.log
- Alerts: ~/monitoring-logs/alerts.log (when count >5)

## Related
- Incident reports: ~/\*INCIDENT\*.md, ~/\*CRISIS\*.md
- Monitoring guide: ~/MONITORING_GUIDE.md
- Disabled hooks: ~/HOOKS_DISABLED_2026-02-11.md
