# AGM Daemon Operations Runbook

This runbook provides step-by-step procedures for operating and troubleshooting the AGM daemon.

## Table of Contents

1. [Daily Operations](#daily-operations)
2. [Health Monitoring](#health-monitoring)
3. [Alert Response Procedures](#alert-response-procedures)
4. [Troubleshooting](#troubleshooting)
5. [Maintenance Tasks](#maintenance-tasks)
6. [Emergency Procedures](#emergency-procedures)

---

## Daily Operations

### Morning Health Check

Perform daily health check:

```bash
# Check daemon status
agm session daemon status

# Check detailed health metrics
agm session daemon health

# Review recent logs
tail -50 ~/.agm/logs/daemon/daemon.log
```

**Expected**: Daemon running, queue depth < 50, no critical alerts.

### Monitor Queue Depth

Check queue statistics:

```bash
agm queue stats
```

**Normal Range**: 0-50 queued messages

**Action Required**:
- 50-100: Monitor closely, investigate if sustained
- >100: Critical - follow [High Queue Depth](#high-queue-depth-100) procedure

---

## Health Monitoring

### Health Status Levels

**HEALTHY**
- Daemon running normally
- Queue depth < 50
- No active alerts

**DEGRADED**
- Queue depth 50-100
- Minor performance issues
- Monitor closely

**CRITICAL**
- Daemon not running, OR
- Queue depth > 100, OR
- Multiple critical alerts

### Key Metrics to Monitor

```bash
# View all metrics via health command
agm session daemon health
```

Monitor these thresholds:

| Metric | Warning | Critical | Action |
|--------|---------|----------|--------|
| Queue Depth | > 50 | > 100 | Clear queue backlog |
| Success Rate | < 75% | < 50% | Check delivery errors |
| Avg Latency | > 10s | > 30s | Check system load |
| Last Poll Age | N/A | > 5min | Restart daemon |
| Error Rate | > 10% | > 25% | Fix state detection |

---

## Alert Response Procedures

### High Queue Depth (>100)

**Symptoms**: Critical alert in logs, `agm session daemon health` shows critical status

**Diagnosis**:
```bash
# 1. Check queue stats
agm queue stats

# 2. List pending messages
agm queue list

# 3. Check recipient sessions
agm session list
```

**Resolution**:

1. **Check recipient session states**:
   ```bash
   # For each queued message recipient
   agm session status <session-name>
   ```
   - If stuck in WORKING: Wait or manually transition to DONE
   - If OFFLINE: Start the session

2. **Clear stuck messages** (if appropriate):
   ```bash
   # Review dead letter queue
   agm queue dlq

   # Requeue failed messages (if needed)
   agm queue requeue <message-id>
   ```

3. **Increase daemon polling** (temporary fix):
   - Edit daemon config to reduce poll interval
   - Restart daemon

4. **Scale recipients**:
   - Start additional recipient sessions
   - Distribute load across more agents

**Prevention**:
- Keep recipient sessions in DONE state
- Monitor session state transitions
- Scale recipient capacity proactively

### Low Success Rate (<75%)

**Symptoms**: Warning/critical alert for success_rate metric

**Diagnosis**:
```bash
# 1. Check dead letter queue
agm queue dlq

# 2. Review daemon logs for errors
grep -i "failed\|error" ~/.agm/logs/daemon/daemon.log | tail -50

# 3. Check specific failed messages
agm queue inspect <message-id>
```

**Resolution**:

1. **Identify failure patterns**:
   - Same recipient failing? → Check session health
   - All messages failing? → Daemon configuration issue
   - Specific message types? → Message format issue

2. **Fix common issues**:
   ```bash
   # Session not responding
   agm session restart <session-name>

   # Tmux session disconnected
   tmux attach -t <session-name>

   # Manifest corruption
   agm session validate <session-name>
   ```

3. **Requeue failed messages**:
   ```bash
   # After fixing underlying issue
   agm queue requeue-all
   ```

**Prevention**:
- Validate message formats before sending
- Monitor session health proactively
- Implement message schema validation

### High Delivery Latency (>10s)

**Symptoms**: Warning/critical alert for avg_latency metric

**Diagnosis**:
```bash
# 1. Check system load
top
uptime

# 2. Check daemon performance
agm session daemon health

# 3. Review slow deliveries in logs
grep "latency:" ~/.agm/logs/daemon/daemon.log | tail -20
```

**Resolution**:

1. **Reduce system load**:
   ```bash
   # Find resource-heavy processes
   top -o %CPU

   # Kill unnecessary processes
   kill <pid>
   ```

2. **Optimize daemon**:
   - Reduce poll interval if too frequent
   - Check for slow state detection
   - Review message sizes

3. **Scale infrastructure**:
   - Add more CPU/memory
   - Distribute across multiple daemons
   - Optimize tmux configuration

**Prevention**:
- Monitor system resources continuously
- Set resource limits on sessions
- Use message batching for bulk operations

### Daemon Not Polling (>5min)

**Symptoms**: Critical alert "Daemon has not polled for over 5 minutes"

**Diagnosis**:
```bash
# 1. Check if daemon is running
agm session daemon status

# 2. Check daemon logs
tail -100 ~/.agm/logs/daemon/daemon.log

# 3. Check system resources
df -h
free -h
```

**Resolution**:

**IMMEDIATE ACTION**:
```bash
# Restart daemon
agm session daemon restart

# Verify restart successful
agm session daemon status
```

**Root Cause Analysis**:

1. **Daemon crashed**:
   - Review logs for panic/crash
   - Check system OOM (out of memory) events
   - Report bug if reproducible

2. **Daemon hung**:
   - Check for deadlock in logs
   - Review long-running operations
   - Kill and restart if unresponsive

3. **System issue**:
   - Disk full → Clean up old files
   - Out of memory → Increase RAM or reduce load
   - Network issues → Check connectivity

**Prevention**:
- Set up external process monitoring (systemd, supervisor)
- Implement daemon auto-restart
- Monitor system resources proactively

### High State Detection Error Rate (>10%)

**Symptoms**: Warning/critical alert for state_detection_error_rate

**Diagnosis**:
```bash
# 1. Check which sessions are failing
grep "Cannot detect state" ~/.agm/logs/daemon/daemon.log | tail -20

# 2. Validate affected sessions
agm session validate <session-name>

# 3. Check tmux connectivity
tmux list-sessions
```

**Resolution**:

1. **Fix session manifests**:
   ```bash
   # Regenerate manifest
   agm session init <session-name>

   # Verify tmux session exists
   tmux has-session -t <tmux-session-name>
   ```

2. **Fix tmux issues**:
   ```bash
   # Restart tmux server (CAUTION: kills all sessions)
   tmux kill-server
   tmux new-session -d -s test

   # Or recreate specific session
   tmux kill-session -t <session-name>
   agm session create <session-name>
   ```

3. **Update state detection logic**:
   - Review session output patterns
   - Update state detection rules
   - File bug report if detection failing consistently

**Prevention**:
- Validate session manifests after creation
- Use consistent session naming
- Monitor tmux server health

---

## Troubleshooting

### Daemon Won't Start

**Error**: "Daemon already running" or "Failed to write PID file"

**Fix**:
```bash
# Check if daemon actually running
ps aux | grep agm-daemon

# If not running, remove stale PID file
rm ~/.agm/daemon.pid

# Try starting again
agm session daemon start
```

### Messages Not Delivering

**Checklist**:

1. Is daemon running?
   ```bash
   agm session daemon status
   ```

2. Is message in queue?
   ```bash
   agm queue list
   ```

3. Is recipient session DONE?
   ```bash
   agm session status <recipient>
   ```

4. Check daemon logs:
   ```bash
   grep <message-id> ~/.agm/logs/daemon/daemon.log
   ```

### Queue Database Corruption

**Symptoms**: Database errors in logs, queue commands failing

**Fix**:
```bash
# Backup existing database
cp ~/.config/agm/message_queue.db ~/.config/agm/message_queue.db.backup

# Check database integrity
sqlite3 ~/.config/agm/message_queue.db "PRAGMA integrity_check;"

# If corrupted, restore from backup or recreate
# WARNING: This loses queue state
rm ~/.config/agm/message_queue.db
agm queue init
```

### High Memory Usage

**Diagnosis**:
```bash
# Check daemon memory
ps aux | grep agm-daemon

# Check queue size
agm queue stats
```

**Fix**:
```bash
# Clean up old messages
agm queue cleanup --days 7

# Restart daemon to free memory
agm session daemon restart
```

---

## Maintenance Tasks

### Daily

- [ ] Check daemon health status
- [ ] Review queue depth
- [ ] Check for critical alerts in logs

### Weekly

- [ ] Review delivery success rates
- [ ] Clean up delivered messages (>7 days old)
- [ ] Review dead letter queue
- [ ] Check disk space usage

### Monthly

- [ ] Analyze performance trends
- [ ] Review and tune alert thresholds
- [ ] Update documentation
- [ ] Review daemon logs for recurring issues

### Quarterly

- [ ] Performance optimization review
- [ ] Capacity planning
- [ ] Disaster recovery drill
- [ ] Update monitoring dashboards

---

## Emergency Procedures

### Complete System Failure

**Symptoms**: Daemon won't start, database corrupted, everything broken

**Recovery**:

1. **Stop everything**:
   ```bash
   agm session daemon stop
   pkill -9 agm-daemon  # Force kill if needed
   ```

2. **Backup current state**:
   ```bash
   mkdir -p ~/agm-backup-$(date +%Y%m%d)
   cp -r ~/.agm ~/agm-backup-$(date +%Y%m%d)/
   cp -r ~/.config/agm ~/agm-backup-$(date +%Y%m%d)/
   ```

3. **Rebuild from scratch**:
   ```bash
   # Clean slate
   rm -rf ~/.agm
   rm -rf ~/.config/agm

   # Reinitialize
   agm session daemon start
   ```

4. **Restore critical data**:
   - Manually requeue important messages
   - Recreate session configurations
   - Verify state before resuming operations

### Data Loss Recovery

**If queue database lost**:

1. Check for automatic backups
2. Reconstruct from daemon logs:
   ```bash
   # Extract sent messages from logs
   grep "Enqueued message" ~/.agm/logs/daemon/daemon.log
   ```
3. Manual message replay if necessary

### Security Incident

**If compromise suspected**:

1. **Immediate shutdown**:
   ```bash
   agm session daemon stop
   ```

2. **Audit logs**:
   ```bash
   grep -E "(error|failed|unauthorized)" ~/.agm/logs/daemon/daemon.log
   ```

3. **Check for unauthorized access**:
   - Review daemon start times
   - Check for unexpected messages
   - Audit session access patterns

4. **Recovery**:
   - Change credentials
   - Rebuild system from known-good state
   - Enable additional security measures

---

## Escalation Contacts

- **Primary**: Development team via GitHub issues
- **Secondary**: System administrator
- **Emergency**: On-call engineer

## Additional Resources

- [Monitoring Documentation](./monitoring.md)
- [Architecture Documentation](./architecture.md)
- [Message Queue API](./message-queue-api.md)
- [GitHub Issues](https://github.com/vbonnet/ai-tools/issues)
