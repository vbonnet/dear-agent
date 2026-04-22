# Manual Testing Guide - Health Hook v2

## Prerequisites
```bash
# Ensure v2 hook is installed
ls -lh ~/bin/engram-health-hook-v2.sh

# Check current system load
uptime
```

## Test 1: Stress Detection

**Expected**: Hook exits quickly when system is overloaded

```bash
# Check CPU count and threshold
nproc  # Example: 8 CPUs
# Threshold = 8 × 2 = 16

# Check current load
awk '{print $1}' /proc/loadavg  # Example: 13.5

# Run hook and measure time
time ~/bin/engram-health-hook-v2.sh
# Expected: <50ms if load > threshold
```

**Pass criteria**: If load > threshold, hook completes in <50ms

---

## Test 2: Rate Limiting

**Expected**: Second run within 5 minutes is blocked

```bash
# Clean rate limit file
rm -f ~/.engram/cache/health-hook-last-run

# First run
time ~/bin/engram-health-hook-v2.sh

# Check rate limit file was created
cat ~/.engram/cache/health-hook-last-run
# Should show Unix timestamp

# Immediate second run
time ~/bin/engram-health-hook-v2.sh
# Expected: <10ms (rate limited)

# Verify timestamp unchanged
cat ~/.engram/cache/health-hook-last-run
# Should be same as before
```

**Pass criteria**: Second run completes in <10ms, timestamp unchanged

---

## Test 3: Cache Handling

**Expected**: Hook handles missing cache gracefully

```bash
# Backup cache
mv ~/.engram/cache/health-check.json ~/.engram/cache/health-check.json.backup

# Run hook
time ~/bin/engram-health-hook-v2.sh
# Expected: <25ms, silent exit

# Restore cache
mv ~/.engram/cache/health-check.json.backup ~/.engram/cache/health-check.json
```

**Pass criteria**: No errors, completes quickly

---

## Test 4: Warning Display

**Expected**: Warnings from cache are displayed

```bash
# Create mock cache with warning
cat > ~/.engram/cache/health-check-test.json << 'EOF'
{
  "ttl": 300,
  "summary": {
    "warnings": 1,
    "errors": 0
  },
  "checks": [
    {
      "status": "warning",
      "message": "Hook scripts missing: engram-health-hook.sh"
    }
  ]
}
EOF

# Temporarily use test cache
mv ~/.engram/cache/health-check.json ~/.engram/cache/health-check.json.real
mv ~/.engram/cache/health-check-test.json ~/.engram/cache/health-check.json

# Run hook
~/bin/engram-health-hook-v2.sh
# Expected output:
# ⚠️  Engram health degraded:
#   - Hook scripts missing: engram-health-hook.sh
#
# Run 'engram doctor --fix' to apply safe fixes

# Restore real cache
mv ~/.engram/cache/health-check.json.real ~/.engram/cache/health-check.json
```

**Pass criteria**: Warning message displayed correctly

---

## Test 5: Performance Under Load

**Expected**: Hook remains fast even under system stress

```bash
# Simulate high CPU load (if stress-ng available)
# Skip if you don't have stress-ng installed
which stress-ng && stress-ng --cpu 16 --timeout 60s &

# Run hook multiple times
for i in {1..10}; do
    time ~/bin/engram-health-hook-v2.sh
done

# Check average time
# Expected: <50ms under stress (stress detection triggers)
```

**Pass criteria**: Consistent <50ms performance under load

---

## Test 6: Integration with Claude

**Expected**: Hook works correctly when called by Claude SessionStart

```bash
# Check Claude settings for hook configuration
grep -A 5 "engram-health" ~/.claude/settings.json

# Start new Claude session and observe:
# 1. No noticeable delay during session start
# 2. Health warnings (if any) appear in output
# 3. No error messages

# Check Claude logs if available
# Look for hook execution time in telemetry
```

**Pass criteria**: Session starts quickly, no errors

---

## Troubleshooting

### Hook takes >100ms
- Check if stress detection is working: `bash -x ~/bin/engram-health-hook-v2.sh`
- Verify /proc/loadavg is readable: `cat /proc/loadavg`
- Check for filesystem issues: `df -h ~/.engram/cache`

### Rate limiting not working
- Verify cache directory exists: `ls -la ~/.engram/cache/`
- Check file permissions: `ls -l ~/.engram/cache/health-hook-last-run`
- Test timestamp update: `date +%s > ~/.engram/cache/test.txt && cat ~/.engram/cache/test.txt`

### No warnings displayed
- Verify cache exists: `ls -la ~/.engram/cache/health-check.json`
- Check cache contents: `cat ~/.engram/cache/health-check.json | jq .`
- Run `engram doctor` to regenerate cache

---

## Success Criteria Summary

| Test | Criteria | Status |
|------|----------|--------|
| Stress detection | <50ms under high load | ⬜ |
| Rate limiting | <10ms on repeat run | ⬜ |
| Cache handling | Graceful with missing cache | ⬜ |
| Warning display | Messages shown correctly | ⬜ |
| Performance | Consistent timing | ⬜ |
| Claude integration | No session start delay | ⬜ |

**Ready for deployment**: All tests pass ✓
