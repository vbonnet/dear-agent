> **SUPERSEDED** (2026-03-26): This document describes the old Python-era implementation.
> This Python monitoring utility was never migrated to Go.
> The bypass-rate monitoring approach may still be valuable but the implementation is outdated.
> Kept for historical reference.

---
hook_name: "monitor-bypass-rate"
hook_type: "utility"
language: "Python"
location: "~/.claude/hooks/monitor-bypass-rate.py"
created_at: "2026-01-26"
last_updated: "2026-02-02"
---

# monitor-bypass-rate - Specification

## Overview

**Purpose**: Analyzes violation and bypass logs to calculate and report the bypass rate for the pretool-bash-blocker hook, providing visibility into security policy effectiveness.

**Trigger**: Manual invocation (can be run on-demand or scheduled via cron for periodic monitoring)

**Critical Path**: No - This is a monitoring/reporting utility, not on the critical execution path for Claude Code operation

**Failure Mode**: Logs warning but does not affect Claude Code operation. If script fails, monitoring data is unavailable but hook execution continues normally.

---

## Functional Requirements

### FR1: Log File Parsing

**Description**: Parse structured log files to extract violation and bypass entries with timestamps, types, commands, and permissions.

**Inputs**:
- `~/.claude-tool-violations.log`: Log file containing blocked violations from pretool-bash-blocker
- `~/.claude-tool-bypasses.log`: Log file containing bypassed violations (allowed by permissions but violate rules)

**Outputs**:
- List of parsed violation entries (timestamp, type, command, permission)
- List of parsed bypass entries (timestamp, type, command, permission)

**Success Criteria**:
- [ ] Correctly parses multi-line log entries with structured format
- [ ] Extracts all fields: timestamp, violation type, command, permission
- [ ] Handles missing log files gracefully (returns empty list)
- [ ] Handles malformed log entries without crashing

**Error Handling**:
- Missing log files: Return empty list, no error
- Malformed entries: Skip entry, continue parsing
- File read errors: Propagate exception to caller

**Log Entry Format**:
```
[2026-01-26 12:00:00] VIOLATION: CD_USAGE
  Tool: Bash
  Command: cd /tmp
  Pattern: ^cd\s
  ---
```

### FR2: Bypass Rate Calculation

**Description**: Calculate bypass rate as the ratio of bypassed violations to total violations (blocked + bypassed).

**Inputs**:
- `violations`: List of blocked violation entries
- `bypasses`: List of bypassed violation entries

**Outputs**:
- `bypass_rate`: Float between 0.0 and 1.0 representing percentage of bypassed violations

**Success Criteria**:
- [ ] Correctly calculates: bypasses / (violations + bypasses)
- [ ] Handles zero total (no violations or bypasses) gracefully (returns 0.0)
- [ ] Returns float for percentage formatting

**Error Handling**:
- Zero violations and bypasses: Return 0.0
- Negative counts (invalid): Should not occur due to input validation

**Formula**: `bypass_rate = len(bypasses) / (len(violations) + len(bypasses))`

### FR3: Time-Based Filtering

**Description**: Filter log entries to only include those within a specified time window (e.g., last 7 days).

**Inputs**:
- `entries`: List of parsed log entries with timestamps
- `since`: Time delta string (e.g., "7d", "24h", "1w") or None for all entries

**Outputs**:
- Filtered list of entries within the time window

**Success Criteria**:
- [ ] Supports time units: hours (h), days (d), weeks (w)
- [ ] Correctly filters entries based on timestamp
- [ ] Returns all entries if since=None
- [ ] Validates time format and provides clear error messages

**Error Handling**:
- Invalid time format: Raise ValueError with helpful message
- Invalid time unit: Raise ValueError listing valid units
- None/missing since: Return all entries (no filtering)

**Supported Formats**: `"7d"`, `"24h"`, `"1w"`, `"30d"`, etc.

### FR4: Text Report Generation

**Description**: Generate human-readable text report showing bypass rate, violation counts, and top offenders.

**Inputs**:
- `violations`: List of violation entries
- `bypasses`: List of bypass entries

**Outputs**:
- Formatted text report to stdout with:
  - Violation counts (blocked, bypassed, total)
  - Bypass rate percentage
  - Top 3 bypassed violation types (if bypasses exist)
  - Top 3 permissions causing bypasses (if bypasses exist)
  - Alert if bypass rate > 10%
  - Success indicator if bypass rate < 10%

**Success Criteria**:
- [ ] Clear, readable formatting with aligned columns
- [ ] Percentages formatted to 1 decimal place
- [ ] Top offenders show count and percentage
- [ ] Alert triggers at configurable threshold (default 10%)
- [ ] Provides actionable recommendations in alert

**Error Handling**:
- Empty logs: Display 0% bypass rate with no top offenders
- Missing permission fields: Skip permission analysis gracefully

**Report Format**:
```
======================================================================
Bypass Rate Report
======================================================================
Violations blocked:  370
Violations bypassed: 0
Total violations:    370
Bypass rate:         0.0%

✅ Bypass rate is within acceptable threshold (<10%)
======================================================================
```

### FR5: JSON Report Generation

**Description**: Generate machine-readable JSON report for integration with monitoring systems.

**Inputs**:
- `violations`: List of violation entries
- `bypasses`: List of bypass entries

**Outputs**:
- JSON object to stdout with:
  - `violations_blocked`: Integer count
  - `violations_bypassed`: Integer count
  - `total_violations`: Integer count
  - `bypass_rate`: Float (0.0-1.0)
  - `alert`: Boolean (true if > threshold)
  - `top_bypassed_types`: Dictionary (if bypasses exist)
  - `top_bypass_permissions`: Dictionary (if bypasses exist)

**Success Criteria**:
- [ ] Valid JSON syntax
- [ ] All numeric fields are correct types (int/float)
- [ ] Alert field is boolean
- [ ] Optional fields omitted if no bypasses

**Error Handling**:
- Empty logs: Return minimal valid JSON with zero counts
- JSON encoding errors: Should not occur with simple types

**JSON Format**:
```json
{
  "violations_blocked": 370,
  "violations_bypassed": 0,
  "total_violations": 370,
  "bypass_rate": 0.0,
  "alert": false
}
```

---

## Non-Functional Requirements

### NFR1: Performance

**Target**: <500ms execution time for typical log files (<10K entries)

**Rationale**: Script is run manually or via cron, not on critical path. Sub-second response is sufficient for good UX.

**Measurement**: `time python3 monitor-bypass-rate.py` with typical log files

**Optimization Notes**:
- Single-pass log parsing (streaming)
- Counter for efficient aggregation
- No disk writes (read-only)

### NFR2: Reliability

**Uptime Target**: 99% success rate (tolerate occasional malformed log entries)

**Failure Recovery**:
- Missing log files: Report 0% bypass rate (valid state)
- Malformed entries: Skip and continue parsing
- File read errors: Exit with error message
- Invalid arguments: Exit with usage help

### NFR3: Maintainability

**Code Complexity**: Low complexity (single file, clear functions, type hints)

**Test Coverage**: Target ≥80% statement coverage for Go rewrite

**Documentation**:
- Inline docstrings for all functions
- Type hints for all function signatures
- Clear usage help text
- Examples in comments

---

## Interface Specification

### Command-Line Interface

**Invocation**:
```bash
monitor-bypass-rate.py [OPTIONS]
```

**Arguments**:
- `--json`: (Optional, boolean) Output JSON format instead of text
- `--since SINCE`: (Optional, string) Only show entries since time delta (e.g., "7d", "24h", "1w")
- `-h, --help`: Show usage help

**Exit Codes**:
- `0`: Success, bypass rate ≤ 10%
- `1`: Alert, bypass rate > 10%
- Non-zero: Error (invalid arguments, file read failure)

**Standard Output**:
```
Text format (default):
======================================================================
Bypass Rate Report
======================================================================
Violations blocked:  370
Violations bypassed: 0
Total violations:    370
Bypass rate:         0.0%

✅ Bypass rate is within acceptable threshold (<10%)
======================================================================

JSON format (--json):
{
  "violations_blocked": 370,
  "violations_bypassed": 0,
  "total_violations": 370,
  "bypass_rate": 0.0,
  "alert": false
}
```

**Standard Error**:
```
Error messages for:
- Invalid time format: "Invalid time format: xyz. Use format like '7d', '24h', '1w'"
- File read errors: "Error reading log file: [filepath]"
- Argument errors: argparse standard error messages
```

### Environment Variables

**Read**:
- None (uses hardcoded log paths)

**Set**:
- None

**Future Enhancement**: Support environment variables for log paths:
- `VIOLATIONS_LOG`: Override default violations log path
- `BYPASSES_LOG`: Override default bypasses log path
- `BYPASS_ALERT_THRESHOLD`: Override 10% default threshold

### File System

**Reads**:
- `~/.claude-tool-violations.log`: Violations log from pretool-bash-blocker (optional, defaults to empty)
- `~/.claude-tool-bypasses.log`: Bypasses log from pretool-bash-blocker (optional, defaults to empty)

**Writes**:
- None (read-only analysis tool)

**Creates**:
- None

**File Format**: Multi-line structured text with entry separator (`---`)

---

## Integration Points

### Claude Code Integration

**Hook Registration**: Not registered as a hook (standalone utility script)

**Execution Context**: Manual invocation by user or scheduled via cron

**Data Flow**:
```
[pretool-bash-blocker] → [Violation logs] → [monitor-bypass-rate.py] → [Report to stdout/file]
```

**Integration with pretool-bash-blocker**:
- Reads violation logs written by pretool-bash-blocker
- Reads bypass logs written by pretool-bash-blocker's bypass detection
- No direct code coupling (log-based integration)

### External Dependencies

**Required** (Python Standard Library):
- `os`: File path expansion, existence checks
- `re`: Regular expression parsing of log entries
- `sys`: Exit codes, argv handling
- `json`: JSON output formatting
- `argparse`: Command-line argument parsing
- `datetime`: Timestamp parsing, time delta calculations
- `timedelta`: Time window filtering
- `collections.Counter`: Efficient top-N aggregation
- `typing`: Type hints (List, Dict, Optional)

**Optional**:
- None (all dependencies are Python stdlib, no external packages)

**Python Version**: Requires Python 3.6+ (for type hints and f-strings)

---

## Test Specification

### Unit Tests

**Coverage Target**: ≥80% statement coverage (for Go rewrite)

**Test Cases**:

#### TC1: Parse Valid Log Entry
- **Scenario**: Parse log entry with all fields present
- **Given**: Log file with complete violation entry
- **When**: `parse_log()` called with filepath and entry_type
- **Then**: Returns list with one entry containing timestamp, type, command, permission

#### TC2: Parse Malformed Log Entry
- **Scenario**: Handle log entry with missing fields
- **Given**: Log file with incomplete entry (missing Command: line)
- **When**: `parse_log()` called
- **Then**: Returns entry with empty command field, continues parsing

#### TC3: Missing Log File
- **Scenario**: Handle missing log file gracefully
- **Given**: Non-existent log file path
- **When**: `parse_log()` called
- **Then**: Returns empty list, no exception raised

#### TC4: Calculate Bypass Rate - Normal Case
- **Scenario**: Calculate bypass rate with violations and bypasses
- **Given**: violations=[1,2,3], bypasses=[4,5]
- **When**: `calculate_bypass_rate()` called
- **Then**: Returns 0.4 (2 / (3 + 2))

#### TC5: Calculate Bypass Rate - Zero Total
- **Scenario**: Handle zero violations and bypasses
- **Given**: violations=[], bypasses=[]
- **When**: `calculate_bypass_rate()` called
- **Then**: Returns 0.0 (not division by zero)

#### TC6: Parse Time Delta - Valid Formats
- **Scenario**: Parse valid time delta strings
- **Given**: Input "7d", "24h", "1w"
- **When**: `parse_time_delta()` called
- **Then**: Returns correct timedelta objects

#### TC7: Parse Time Delta - Invalid Format
- **Scenario**: Reject invalid time delta format
- **Given**: Input "xyz" or "7x"
- **When**: `parse_time_delta()` called
- **Then**: Raises ValueError with helpful message

#### TC8: Filter By Time - Within Window
- **Scenario**: Filter entries within time window
- **Given**: Entries from last 3 days, since=7d
- **When**: `filter_by_time()` called
- **Then**: Returns all entries (all within 7 days)

#### TC9: Filter By Time - Outside Window
- **Scenario**: Exclude entries outside time window
- **Given**: Entries from 10 days ago, since=7d
- **When**: `filter_by_time()` called
- **Then**: Returns empty list (all outside 7 days)

#### TC10: Text Report - No Bypasses
- **Scenario**: Generate report with zero bypasses
- **Given**: violations=[1,2,3], bypasses=[]
- **When**: `generate_report()` called with format="text"
- **Then**: Outputs report with 0.0% bypass rate, no top offenders, success message

#### TC11: Text Report - Alert Triggered
- **Scenario**: Generate report with high bypass rate
- **Given**: violations=[1], bypasses=[9]
- **When**: `generate_report()` called
- **Then**: Outputs report with 90% bypass rate, alert message, recommendations

#### TC12: JSON Report - Complete
- **Scenario**: Generate JSON report with all fields
- **Given**: violations=[1,2], bypasses=[3,4]
- **When**: `generate_report()` called with format="json"
- **Then**: Outputs valid JSON with all fields, alert=false

### Integration Tests

**Scope**: End-to-end log parsing and report generation with real log files

**Test Cases**:

#### ITC1: Parse Real Violation Log
- **Scenario**: Parse actual violation log from pretool-bash-blocker
- **Components**: File I/O, log parser, data structures
- **Given**: Sample violation log file with 10 entries
- **When**: Script executed with real log path
- **Then**: Correctly parses all 10 entries with accurate fields

#### ITC2: Generate Text Report from Real Logs
- **Scenario**: Full text report generation workflow
- **Components**: Log parser, bypass calculator, text formatter
- **Given**: Sample violation and bypass log files
- **When**: Script executed with --text (default)
- **Then**: Generates formatted report with correct counts and percentages

#### ITC3: Generate JSON Report from Real Logs
- **Scenario**: Full JSON report generation workflow
- **Components**: Log parser, bypass calculator, JSON formatter
- **Given**: Sample violation and bypass log files
- **When**: Script executed with --json
- **Then**: Outputs valid JSON parseable by `jq`, `json.loads()`

#### ITC4: Time Filtering with Real Logs
- **Scenario**: Filter real log entries by time window
- **Components**: Log parser, time filter, report generator
- **Given**: Log files with entries from last 30 days
- **When**: Script executed with --since 7d
- **Then**: Report only includes entries from last 7 days

### E2E Tests

**Scope**: Full command-line execution in realistic scenarios

**Test Cases**:

#### E2E1: Fresh Installation - No Logs
- **Scenario**: Run script on fresh system with no log files
- **Setup**: Remove/rename log files if present
- **Execution**: `python3 monitor-bypass-rate.py`
- **Verification**:
  - Exit code 0
  - Report shows 0/0 violations
  - No error messages
- **Cleanup**: Restore log files

#### E2E2: High Bypass Rate Alert
- **Scenario**: Trigger alert with high bypass rate
- **Setup**: Create test logs with violations=10, bypasses=50
- **Execution**: `python3 monitor-bypass-rate.py`
- **Verification**:
  - Exit code 1 (alert)
  - Report shows 83.3% bypass rate
  - Alert message displayed
  - Recommendations included
- **Cleanup**: Remove test logs

#### E2E3: JSON Output for Monitoring System
- **Scenario**: Generate JSON for integration with monitoring
- **Setup**: Realistic log files
- **Execution**: `python3 monitor-bypass-rate.py --json | jq .`
- **Verification**:
  - Valid JSON output
  - All fields present
  - Parseable by standard JSON tools
- **Cleanup**: None (read-only)

#### E2E4: Time Window Filtering
- **Scenario**: Generate report for last week only
- **Setup**: Log files with 30 days of entries
- **Execution**: `python3 monitor-bypass-rate.py --since 7d`
- **Verification**:
  - Counts only include last 7 days
  - Older entries excluded
  - Correct bypass rate for time window
- **Cleanup**: None (read-only)

### BDD Tests (For Go Rewrite)

**Framework**: Ginkgo/Gomega

**Feature**: Bypass Rate Monitoring and Alerting

**Scenarios**:

#### Scenario 1: Normal Operation - Low Bypass Rate
```gherkin
Given violation log contains 100 entries
And bypass log contains 5 entries
When monitor-bypass-rate is executed
Then bypass rate should be 4.8%
And alert should not be triggered
And exit code should be 0
And report should show success indicator
```

#### Scenario 2: Alert Triggered - High Bypass Rate
```gherkin
Given violation log contains 50 entries
And bypass log contains 50 entries
When monitor-bypass-rate is executed
Then bypass rate should be 50%
And alert should be triggered
And exit code should be 1
And report should include recommendations
```

#### Scenario 3: Missing Log Files - Graceful Degradation
```gherkin
Given violation log does not exist
And bypass log does not exist
When monitor-bypass-rate is executed
Then bypass rate should be 0%
And no errors should be raised
And exit code should be 0
```

#### Scenario 4: Time-Based Filtering
```gherkin
Given violation log contains entries from last 30 days
And bypass log contains entries from last 30 days
When monitor-bypass-rate is executed with --since 7d
Then only entries from last 7 days should be included
And bypass rate should reflect filtered data
```

**Implementation** (Go with Ginkgo):
```go
Describe("Bypass Rate Monitoring", func() {
    Context("when bypass rate is low", func() {
        It("should report success and exit 0", func() {
            violations := generateTestViolations(100)
            bypasses := generateTestBypasses(5)

            rate := calculateBypassRate(violations, bypasses)
            Expect(rate).To(BeNumerically("<", 0.10))

            alert := rate > 0.10
            Expect(alert).To(BeFalse())
        })
    })

    Context("when bypass rate is high", func() {
        It("should trigger alert and exit 1", func() {
            violations := generateTestViolations(50)
            bypasses := generateTestBypasses(50)

            rate := calculateBypassRate(violations, bypasses)
            Expect(rate).To(BeNumerically(">=", 0.10))

            alert := rate > 0.10
            Expect(alert).To(BeTrue())
        })
    })

    Context("when log files are missing", func() {
        It("should handle gracefully with zero rate", func() {
            violations := parseLog("/nonexistent/violations.log", "VIOLATION")
            bypasses := parseLog("/nonexistent/bypasses.log", "BYPASS")

            Expect(violations).To(BeEmpty())
            Expect(bypasses).To(BeEmpty())

            rate := calculateBypassRate(violations, bypasses)
            Expect(rate).To(Equal(0.0))
        })
    })
})
```

---

## Edge Cases & Error Scenarios

### Edge Case 1: Empty Log Files

**Description**: Log files exist but contain no entries (0 bytes or only whitespace)

**Example**:
```bash
touch ~/.claude-tool-violations.log  # Empty file
python3 monitor-bypass-rate.py
```

**Expected Behavior**: Return 0% bypass rate, no errors, success message

**Test Coverage**: TC3: Missing Log File, E2E1: Fresh Installation

### Edge Case 2: Very Large Log Files

**Description**: Log files contain thousands of entries (weeks/months of data)

**Example**:
```bash
# 10K+ entries
python3 monitor-bypass-rate.py
```

**Expected Behavior**:
- Parse all entries (no truncation)
- Complete in <5 seconds
- Report accurate counts
- Memory usage <100MB

**Test Coverage**: Performance testing (not in unit tests, manual verification)

### Edge Case 3: Malformed Timestamp

**Description**: Log entry has invalid timestamp format

**Example**:
```
[INVALID-TIMESTAMP] VIOLATION: CD_USAGE
  Command: cd /tmp
  ---
```

**Expected Behavior**: Skip entry, continue parsing, log debug message

**Test Coverage**: TC2: Parse Malformed Log Entry

### Edge Case 4: 100% Bypass Rate

**Description**: All violations are bypassed (no blocks)

**Example**:
```bash
# violations=0, bypasses=100
python3 monitor-bypass-rate.py
```

**Expected Behavior**:
- Report 100% bypass rate
- Trigger alert
- Exit code 1
- Show top bypassed types and permissions

**Test Coverage**: TC11: Text Report - Alert Triggered (with adjusted ratios)

### Edge Case 5: Partial Log Entry at EOF

**Description**: Log file ends without `---` separator

**Example**:
```
[2026-01-26 12:00:00] VIOLATION: CD_USAGE
  Command: cd /tmp
# Missing --- separator at end
```

**Expected Behavior**: Skip incomplete entry, report all complete entries

**Test Coverage**: TC2: Parse Malformed Log Entry

### Error Scenario 1: Invalid --since Format

**Trigger**: User provides invalid time format to --since

**Error Message**:
```
Invalid time format: xyz. Use format like '7d', '24h', '1w'
```

**Recovery**: Exit with usage help, user corrects command

**Test Coverage**: TC7: Parse Time Delta - Invalid Format

### Error Scenario 2: File Permission Denied

**Trigger**: Log files exist but are not readable (permission denied)

**Error Message**:
```
PermissionError: [Errno 13] Permission denied: '~/.claude-tool-violations.log'
```

**Recovery**: Check file permissions, fix with chmod

**Test Coverage**: Not covered (OS-level error, difficult to test portably)

### Error Scenario 3: Disk Full During Read

**Trigger**: Disk fills up while reading log file

**Error Message**:
```
OSError: [Errno 28] No space left on device
```

**Recovery**: Free up disk space, re-run script

**Test Coverage**: Not covered (OS-level error)

---

## Performance Characteristics

### Benchmarks

**Typical Case**:
- **Input**: 100 violations, 10 bypasses (typical daily usage)
- **Expected Time**: <100ms
- **Measurement**: `time python3 monitor-bypass-rate.py` (wall clock time)

**Worst Case**:
- **Input**: 10,000 violations, 1,000 bypasses (months of accumulated data)
- **Expected Time**: <5 seconds
- **Measurement**: `time python3 monitor-bypass-rate.py` with large synthetic logs

**Go Rewrite Performance Targets**:
- Typical case: <10ms (10x faster than Python)
- Worst case: <500ms (10x faster than Python)
- Memory: <10MB RSS (Python: ~30MB)

### Resource Usage

**Memory**: ~20-30MB RSS for Python (includes interpreter overhead)

**CPU**: Minimal, single-threaded parsing (I/O bound, not CPU bound)

**Disk I/O**:
- 2 reads: violations log + bypasses log
- 0 writes (read-only)
- Sequential reads (optimal for spinning disks and SSDs)

**Network**: None

---

## Security Considerations

### Input Validation

**Untrusted Inputs**:
- `violations log`: Trusted (written by pretool-bash-blocker, owned by user)
- `bypasses log`: Trusted (written by pretool-bash-blocker, owned by user)
- `--since argument`: Validated against regex pattern, limited to numeric + unit

**Sanitization**:
- Log file paths: Expanded via `os.path.expanduser()` (trusted function)
- Time delta parsing: Regex validation before parsing
- Log entry parsing: Regex matching, no eval/exec

### Privilege Requirements

**Required Permissions**: Read access to `~/.claude-tool-violations.log` and `~/.claude-tool-bypasses.log`

**Privilege Escalation**: None (runs as user, no sudo/root required)

### Vulnerability Surface

**Attack Vectors**:
- Log injection: Malicious log entries with crafted timestamps/commands
- Path traversal: Not applicable (hardcoded log paths)
- Command injection: Not applicable (no shell execution)
- Regular expression DoS: Unlikely (simple regex patterns, bounded input)

**Mitigations**:
- No code execution based on log content
- No shell commands invoked
- No network access
- Read-only operation (no file writes)
- Runs in user context (no privileged operations)

---

## Maintenance & Troubleshooting

### Common Issues

#### Issue 1: "No such file or directory" - Missing Log Files

**Symptoms**: Script exits with FileNotFoundError (older Python versions) or returns empty results

**Diagnosis**:
```bash
ls -lah ~/.claude-tool-violations.log ~/.claude-tool-bypasses.log
```

**Resolution**:
- Expected behavior: Script handles missing files gracefully
- If using older Python (<3.8), update to Python 3.8+
- Logs are created by pretool-bash-blocker when violations occur

#### Issue 2: Zero Violations and Bypasses

**Symptoms**: Report shows 0/0 violations, 0% bypass rate

**Diagnosis**:
```bash
# Check if logs exist and contain data
wc -l ~/.claude-tool-violations.log ~/.claude-tool-bypasses.log

# Check if logs are being written
tail -f ~/.claude-tool-violations.log  # In another terminal, trigger a violation
```

**Resolution**:
- If logs are empty: No violations have occurred (expected for well-configured system)
- If logs don't exist: Pretool-bash-blocker hasn't run yet or logging is disabled
- If logs exist but aren't updating: Check pretool-bash-blocker configuration

#### Issue 3: Incorrect Bypass Rate

**Symptoms**: Bypass rate seems wrong compared to manual count

**Diagnosis**:
```bash
# Count violations manually
grep "^\[" ~/.claude-tool-violations.log | grep "VIOLATION:" | wc -l

# Count bypasses manually
grep "^\[" ~/.claude-tool-bypasses.log | grep "BYPASS:" | wc -l

# Check for malformed entries
grep "^\[" ~/.claude-tool-violations.log | grep -v "VIOLATION:"
```

**Resolution**:
- Malformed entries: Clean up log files or fix pretool-bash-blocker logging
- Time filtering confusion: Check if using --since (only counts filtered entries)
- Log rotation: Logs may have been rotated, script only sees current log file

### Logging

**Log Location**: None (script does not create logs, only reads them)

**Log Level**: Not applicable (no logging framework used)

**Log Format**: N/A

**Future Enhancement**: Add optional debug mode to log parsing details:
```bash
python3 monitor-bypass-rate.py --debug  # Log each parsed entry, time filters, etc.
```

### Debugging

**Enable Debug Mode**: Not currently implemented

**Debug Output**: For manual debugging, add print statements:
```python
# In parse_log():
print(f"DEBUG: Parsed entry: {current_entry}")

# In filter_by_time():
print(f"DEBUG: Filtering {len(entries)} entries with cutoff {cutoff}")
```

**Debug Flags**: Future enhancement suggestions:
- `--debug`: Enable verbose debug output
- `--dry-run`: Parse logs but don't generate report (syntax check)
- `--validate`: Validate log file format without generating report

---

## Version History

| Version | Date | Changes | Author |
|---------|------|---------|--------|
| 1.0 | 2026-01-26 | Initial implementation (Python) | Wayfinder: close-permission-bypass-loophole |
| 1.1 | 2026-02-02 | SPEC created for Go rewrite planning | Claude Code |

---

## References

**Related Documents**:
- [Implementation]: `~/.claude/hooks/monitor-bypass-rate.py` (Python, current)
- [Design Doc]: `the git history/S6-design.md`
- [Implementation Doc]: `the git history/S8-implementation-complete.md`
- [Validation Doc]: `the git history/S9-validation.md`
- [Related Hook]: `~/.claude/hooks/pretool-bash-blocker.py` (writes logs this script reads)

**External References**:
- Python argparse: https://docs.python.org/3/library/argparse.html
- Python datetime: https://docs.python.org/3/library/datetime.html
- Python Counter: https://docs.python.org/3/library/collections.html#collections.Counter
- Log format specification: Defined by pretool-bash-blocker implementation

---

## Notes

**Context**: Created during "close-permission-bypass-loophole" Wayfinder project to monitor effectiveness of permission cleanup. Provides visibility into how often pretool-bash-blocker rules are bypassed by user permissions.

**Use Case**: Run manually after making permission changes or schedule via cron for periodic monitoring:
```bash
# Manual check
python3 ~/.claude/hooks/monitor-bypass-rate.py

# Daily cron (9 AM)
0 9 * * * python3 ~/.claude/hooks/monitor-bypass-rate.py --json >> ~/.claude/bypass-rate-history.jsonl

# Weekly alert check
0 9 * * 1 python3 ~/.claude/hooks/monitor-bypass-rate.py || mail -s "Bypass Rate Alert" admin@example.com
```

**Future Enhancements**:
- **Historical Trending**: Track bypass rate over time, store in database or JSONL file
- **Alerting Integration**: Send alerts via email, Slack, PagerDuty when threshold exceeded
- **Per-Permission Breakdown**: Show bypass rate for each permission rule (requires log format enhancement)
- **Recommendation Engine**: Suggest specific permission changes based on bypass patterns
- **Interactive Mode**: Allow user to drill down into specific violation types or time periods
- **Configuration File**: Support `~/.claude/monitor-bypass-rate.conf` for custom thresholds, log paths
- **Multiple Threshold Levels**: Warning (5%), alert (10%), critical (25%)

**Known Limitations**:
- **Log Rotation**: Script only analyzes current log files, doesn't handle rotated logs (`.1`, `.2`, etc.)
  - Workaround: Use `--since` to limit time window, or manually concatenate rotated logs
- **Single Tool Support**: Only monitors Bash tool bypasses (doesn't analyze other tools)
  - Workaround: Extend pretool-bash-blocker to log other tools, or create separate monitor scripts
- **No Persistent State**: Each run is independent, no memory of previous rates
  - Workaround: Use cron + JSONL output to build historical database
- **Text Parsing Fragility**: Log format changes in pretool-bash-blocker break parsing
  - Workaround: Maintain log format compatibility or version log format

---

## Python → Go Migration Notes

### Python-Specific Features Requiring Go Equivalents

**1. Regular Expression Parsing**:
- Python: `re.match()`, `re.search()` with simple patterns
- Go equivalent: `regexp` package, compile patterns with `regexp.MustCompile()`
- Migration complexity: Low (Go regex is similar, may need to adjust patterns)

**2. Type Hints**:
- Python: `List[Dict]`, `Optional[str]` (runtime ignored, but used for documentation)
- Go equivalent: Explicit types: `[]ViolationEntry`, `*string` (compile-time enforced)
- Migration complexity: Low (Go types are more strict, which is beneficial)

**3. Collections.Counter**:
- Python: `Counter(items).most_common(n)` for top-N aggregation
- Go equivalent: Map + sort, or use a third-party library
- Migration complexity: Low (simple to implement with map and sort)

**4. Datetime Parsing**:
- Python: `datetime.strptime(timestamp, "%Y-%m-%d %H:%M:%S")`
- Go equivalent: `time.Parse("2006-01-02 15:04:05", timestamp)`
- Migration complexity: Low (Go time package is powerful)

**5. Argparse**:
- Python: `argparse` module for CLI arguments
- Go equivalent: `flag` package or `cobra` for more advanced CLI
- Migration complexity: Low (flag is simpler, cobra is more feature-rich)

**6. JSON Serialization**:
- Python: `json.dumps(obj, indent=2)`
- Go equivalent: `json.MarshalIndent(obj, "", "  ")`
- Migration complexity: Low (Go JSON is straightforward with struct tags)

### Python Libraries Needing Go Replacements

**Standard Library (All Available in Go)**:
- `os` → `os` package (path expansion, file existence)
- `sys` → `os` package (exit codes, args)
- `re` → `regexp` package
- `json` → `encoding/json` package
- `datetime` → `time` package

**No External Dependencies**: All Python dependencies are stdlib, so no external Go packages required (except optionally cobra for CLI, which is recommended for production CLI tools).

### Monitoring/Metrics Patterns for Go Implementation

**Recommended Go Packages**:
- `github.com/spf13/cobra`: Production-grade CLI framework (optional but recommended)
- `github.com/onsi/ginkgo/v2`: BDD testing framework
- `github.com/onsi/gomega`: Matcher library for Ginkgo

**Go Struct Design**:
```go
type ViolationEntry struct {
    Timestamp  time.Time
    Type       string
    Command    string
    Permission string
}

type Report struct {
    ViolationsBlocked  int              `json:"violations_blocked"`
    ViolationsBypassed int              `json:"violations_bypassed"`
    TotalViolations    int              `json:"total_violations"`
    BypassRate         float64          `json:"bypass_rate"`
    Alert              bool             `json:"alert"`
    TopBypassedTypes   map[string]int   `json:"top_bypassed_types,omitempty"`
    TopBypassPerms     map[string]int   `json:"top_bypass_permissions,omitempty"`
}
```

**Performance Optimizations for Go**:
- Use `bufio.Scanner` for line-by-line log parsing (more efficient than `ioutil.ReadFile`)
- Pre-compile regex patterns at init time
- Use `strings.Builder` for report formatting (more efficient than concatenation)
- Consider concurrent log parsing if files are very large (not needed for typical usage)

**Testing Strategy for Go**:
- Table-driven tests for parsing logic
- Ginkgo BDD tests for scenarios (alert triggered, time filtering, etc.)
- Benchmarks for performance regression testing
- Test fixtures: Sample log files in `testdata/`

**Example Go Test Structure**:
```
monitor-bypass-rate/
├── cmd/
│   └── monitor-bypass-rate/
│       └── main.go
├── internal/
│   ├── parser/
│   │   ├── parser.go
│   │   ├── parser_test.go
│   │   └── parser_suite_test.go (Ginkgo)
│   ├── calculator/
│   │   ├── calculator.go
│   │   └── calculator_test.go
│   └── reporter/
│       ├── reporter.go
│       ├── reporter_test.go
│       └── reporter_suite_test.go (Ginkgo)
├── testdata/
│   ├── violations-sample.log
│   ├── bypasses-sample.log
│   └── malformed-sample.log
└── README.md
```

**Go Rewrite Success Criteria**:
- [ ] All Python unit tests ported to Go (table-driven + Ginkgo)
- [ ] Performance: <10ms typical case (10x faster than Python)
- [ ] Binary size: <5MB (statically linked)
- [ ] Zero external dependencies (except testing frameworks)
- [ ] Feature parity: All CLI flags, output formats, edge cases
- [ ] Documentation: Godoc for all exported functions
