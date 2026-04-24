# AGM Performance Tuning Guide

**Version**: 1.0
**Last Updated**: 2026-02-20
**Target Audience**: Performance engineers, power users

---

## Table of Contents

- [Overview](#overview)
- [Performance Baseline](#performance-baseline)
- [Daemon Optimization](#daemon-optimization)
- [Queue Performance](#queue-performance)
- [Session Management](#session-management)
- [Tmux Optimization](#tmux-optimization)
- [Disk I/O Optimization](#disk-io-optimization)
- [Memory Management](#memory-management)
- [Network Latency](#network-latency)
- [Benchmarking Tools](#benchmarking-tools)
- [Performance Monitoring](#performance-monitoring)
- [Troubleshooting Performance Issues](#troubleshooting-performance-issues)

---

## Overview

### Performance Goals

| Metric | Target | Measurement |
|--------|--------|-------------|
| Message delivery latency | <500ms | Enqueue → delivery |
| Daemon poll overhead | <100ms | Per poll cycle |
| Queue throughput | 100 msg/min | Sustained rate |
| Session creation time | <2s | `agm new` completion |
| Session resume time | <1s | `agm resume` attachment |
| Memory footprint | <50MB | Daemon + 10 sessions |
| CPU usage | <1% | Idle daemon |
| Disk I/O | <10KB/s | Steady state |

### Performance Architecture

```
┌─────────────────────────────────────────────────┐
│               Performance Layers                 │
├─────────────────────────────────────────────────┤
│ Layer 1: Daemon Polling (poll interval tuning)  │
│ Layer 2: Queue Operations (SQLite optimization) │
│ Layer 3: State Detection (caching, batching)    │
│ Layer 4: Message Delivery (tmux optimization)   │
│ Layer 5: Disk I/O (filesystem, SSD tuning)      │
└─────────────────────────────────────────────────┘
```

---

## Performance Baseline

### Establishing Baseline Metrics

Run baseline tests before optimization:

```bash
# Create test environment
mkdir -p ~/agm-perf-test
cd ~/agm-perf-test

# Start fresh daemon
agm daemon stop
rm -rf ~/.agm/queue.db
agm daemon start

# Baseline 1: Session creation time
time agm new perf-test-1 --detached
# Expected: <2s

# Baseline 2: Message send time
time agm send perf-test-1 "Test message"
# Expected: <100ms (enqueue time)

# Baseline 3: Queue poll time
# (Monitor daemon logs for poll cycle duration)
tail -f ~/.agm/logs/daemon/daemon.log | grep "poll cycle"

# Baseline 4: Daemon memory
ps aux | grep agm-daemon | awk '{print $6/1024 " MB"}'
# Expected: <50MB

# Baseline 5: Queue throughput
~/agm-perf-test/benchmark-throughput.sh
# Expected: >100 msg/min
```

### Benchmark Scripts

Create `~/agm-perf-test/benchmark-throughput.sh`:

```bash
#!/bin/bash
set -euo pipefail

# Throughput benchmark: measure messages/minute

SESSIONS=10
MESSAGES_PER_SESSION=10
START_TIME=$(date +%s)

# Create test sessions
for i in $(seq 1 $SESSIONS); do
  agm new "perf-session-$i" --detached
done

# Send messages
for i in $(seq 1 $SESSIONS); do
  for j in $(seq 1 $MESSAGES_PER_SESSION); do
    agm send "perf-session-$i" "Message $j" &
  done
done
wait

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))
TOTAL_MESSAGES=$((SESSIONS * MESSAGES_PER_SESSION))
THROUGHPUT=$((TOTAL_MESSAGES * 60 / DURATION))

echo "Total messages: $TOTAL_MESSAGES"
echo "Duration: ${DURATION}s"
echo "Throughput: $THROUGHPUT msg/min"

# Cleanup
for i in $(seq 1 $SESSIONS); do
  agm session delete "perf-session-$i" --force
done
```

Run benchmark:

```bash
chmod +x ~/agm-perf-test/benchmark-throughput.sh
~/agm-perf-test/benchmark-throughput.sh
```

---

## Daemon Optimization

### Poll Interval Tuning

The daemon poll interval directly affects message delivery latency:

**Formula:**
```
Average Latency = Poll Interval / 2
Max Latency = Poll Interval
Throughput ≈ Queue Size / Poll Interval
```

**Tuning Guidelines:**

```bash
# Low latency (interactive workloads)
agm daemon start --poll-interval 5s
# Pros: 2.5s average latency, 5s max latency
# Cons: Higher CPU usage (~0.5%)

# Balanced (default, recommended)
agm daemon start --poll-interval 30s
# Pros: Low CPU (<0.1%), good for most use cases
# Cons: 15s average latency, 30s max latency

# Batch processing (high throughput, low priority)
agm daemon start --poll-interval 2m
# Pros: Minimal CPU (<0.01%), handles large queues
# Cons: 60s average latency, 120s max latency
```

**Dynamic Poll Interval (Future Feature):**

```yaml
# ~/.config/agm/daemon-config.yaml
poll_interval:
  idle: 30s          # When queue is empty
  active: 5s         # When messages pending
  backoff: true      # Exponential backoff on errors
  min: 5s
  max: 2m
```

### Concurrent Delivery

Enable concurrent message delivery (future feature):

```yaml
# ~/.config/agm/daemon-config.yaml
delivery:
  concurrent: true
  max_workers: 5      # Parallel deliveries
  batch_size: 10      # Process 10 messages per poll
```

**Expected Impact:**
- Throughput: 100 → 500 msg/min (5x improvement)
- CPU usage: 0.1% → 0.5% (5x increase)
- Memory: 10MB → 20MB (2x increase)

### State Detection Caching

Cache session states to reduce filesystem I/O:

```yaml
# ~/.config/agm/daemon-config.yaml
state_cache:
  enabled: true
  ttl: 10s           # Cache state for 10s
  max_size: 1000     # Cache up to 1000 sessions
```

**Expected Impact:**
- Manifest reads: 100/min → 10/min (90% reduction)
- Poll cycle time: 100ms → 20ms (80% reduction)

---

## Queue Performance

### SQLite Optimization

AGM uses SQLite for the message queue. Optimize with PRAGMA directives:

```bash
# Open queue database
sqlite3 ~/.agm/queue.db

# Enable WAL mode (better concurrent access)
PRAGMA journal_mode=WAL;
-- Output: wal

# Increase cache size (default 2MB → 10MB)
PRAGMA cache_size=-10000;
-- Output: -10000

# Synchronous mode (trades durability for speed)
PRAGMA synchronous=NORMAL;  # Default: FULL
-- Output: normal

# Temporary storage in memory
PRAGMA temp_store=MEMORY;
-- Output: 2

# Page size (default 4096, optimal for SSD)
PRAGMA page_size=4096;
-- Output: 4096

# Auto-vacuum (reclaim space automatically)
PRAGMA auto_vacuum=INCREMENTAL;
-- Output: 2
```

**Apply optimizations automatically:**

Create `~/agm-perf-test/optimize-queue.sh`:

```bash
#!/bin/bash
sqlite3 ~/.agm/queue.db <<EOF
PRAGMA journal_mode=WAL;
PRAGMA cache_size=-10000;
PRAGMA synchronous=NORMAL;
PRAGMA temp_store=MEMORY;
ANALYZE;
EOF
echo "Queue optimizations applied"
```

### Index Optimization

AGM creates indexes automatically, but verify they exist:

```sql
-- Check existing indexes
SELECT name, sql FROM sqlite_master WHERE type='index';

-- Expected indexes:
-- idx_pending: WHERE status = 'pending'
-- idx_cleanup: WHERE status IN ('delivered', 'failed')
-- idx_queue_status_priority: (status, priority DESC, created_at)

-- Create missing indexes
CREATE INDEX IF NOT EXISTS idx_queue_status_priority
ON message_queue(status, priority DESC, created_at);

CREATE INDEX IF NOT EXISTS idx_queue_cleanup
ON message_queue(status, delivered_at)
WHERE status IN ('delivered', 'failed');
```

### Query Performance Analysis

Profile slow queries:

```bash
# Enable query logging
sqlite3 ~/.agm/queue.db "PRAGMA query_only=ON;"

# Explain query plan
sqlite3 ~/.agm/queue.db <<EOF
EXPLAIN QUERY PLAN
SELECT * FROM message_queue
WHERE status = 'pending'
ORDER BY priority DESC, created_at ASC
LIMIT 100;
EOF

# Expected output:
# SEARCH TABLE message_queue USING INDEX idx_queue_status_priority (status=?)
```

### Batch Operations

Batch queue operations reduce transaction overhead:

```bash
# Instead of: (N separate transactions)
for msg in $(seq 1 100); do
  agm send session "Message $msg"
done

# Use: (Single transaction, future feature)
agm send session --batch <<EOF
Message 1
Message 2
...
Message 100
EOF
```

---

## Session Management

### Manifest Caching

Cache session manifests to reduce filesystem reads:

```bash
# Generate manifest cache
agm session list --format json > /tmp/agm-sessions-cache.json

# Use cache for reads (update every 5 min)
*/5 * * * * agm session list --format json > /tmp/agm-sessions-cache.json
```

**Expected Impact:**
- Manifest reads: 1000/min → 100/min (90% reduction)
- Session list time: 500ms → 50ms (90% reduction)

### State Hook Optimization

Optimize state management hooks for faster execution:

**Before (slow hook):**

```bash
#!/bin/bash
# ~/.claude/hooks/session-start/agm-state-ready

# Slow: Reads entire manifest, updates state
MANIFEST_PATH=$(agm admin get-manifest-path "$CLAUDE_SESSION_NAME")
STATE=$(jq -r '.state' "$MANIFEST_PATH")
echo "{\"state\": \"DONE\"}" | jq -s '.[0] * .[1]' "$MANIFEST_PATH" - > /tmp/manifest.json
mv /tmp/manifest.json "$MANIFEST_PATH"
```

**After (fast hook):**

```bash
#!/bin/bash
# ~/.claude/hooks/session-start/agm-state-ready-fast

# Fast: Direct state update via agm CLI
agm admin set-state "$CLAUDE_SESSION_NAME" DONE --no-validation
```

**Expected Impact:**
- Hook execution time: 500ms → 50ms (90% reduction)
- Session startup time: 3s → 2.5s (15% reduction)

### Lazy Session Loading

Load session details only when needed:

```bash
# Slow: Load all manifests upfront
agm session list --format table

# Fast: Load metadata only (no manifest parsing)
agm session list --metadata-only --format table
```

---

## Tmux Optimization

### Tmux Configuration

Optimize `~/.tmux.conf` for AGM usage:

```conf
# Increase history limit (reduce scrollback searches)
set-option -g history-limit 50000

# Faster command sequences (reduce escape time)
set -s escape-time 0

# Increase message display time
set -g display-time 4000

# Aggressive resize (better for multi-monitor)
setw -g aggressive-resize on

# Status update interval (default 15s)
set -g status-interval 5

# Disable visual bell (reduces overhead)
set -g visual-activity off
set -g visual-bell off
set -g visual-silence off

# Disable automatic window renaming
setw -g automatic-rename off
set -g allow-rename off
```

### Tmux Session Management

Reduce tmux session overhead:

```bash
# Use single tmux server for all AGM sessions
export TMUX_TMPDIR=~/.tmux

# Increase server socket buffer
tmux set-option -g buffer-limit 20

# Disable unused features
tmux set-option -g mouse off
tmux set-option -g visual-activity off
```

### Tmux Command Batching

Batch tmux commands to reduce socket overhead:

```bash
# Instead of: (Multiple tmux invocations)
tmux send-keys -t session1 "command1" C-m
tmux send-keys -t session2 "command2" C-m
tmux send-keys -t session3 "command3" C-m

# Use: (Single tmux batch, future feature)
tmux <<EOF
send-keys -t session1 "command1" C-m
send-keys -t session2 "command2" C-m
send-keys -t session3 "command3" C-m
EOF
```

---

## Disk I/O Optimization

### Filesystem Selection

AGM performs best on modern filesystems with good metadata performance:

| Filesystem | Performance | Notes |
|------------|-------------|-------|
| **ext4** | Good | Default for most Linux distros |
| **XFS** | Excellent | Best for large files, high concurrency |
| **Btrfs** | Good | Copy-on-write may add overhead |
| **ZFS** | Excellent | Use ARC cache for metadata |
| **APFS** | Good | macOS default, optimized for SSD |
| **NFS** | Poor | Avoid for `~/.agm/` directory |

### SSD Optimization

If using SSD, enable TRIM and optimize mount options:

```bash
# Enable TRIM (periodic or continuous)
sudo systemctl enable fstrim.timer

# Add mount options to /etc/fstab
# /dev/sda1 / ext4 defaults,noatime,discard 0 1

# Verify SSD optimization
sudo hdparm -I /dev/sda | grep TRIM
```

### Directory Layout

Optimize directory layout for I/O patterns:

```bash
# Default layout
~/.agm/
├── sessions/          # High read frequency
├── queue.db           # High read/write frequency
└── logs/              # High write frequency

# Optimized layout (separate mount points)
~/.agm/
├── sessions/          # Fast SSD (read-heavy)
├── queue.db           # Fastest SSD (read/write-heavy)
└── logs/ -> /var/log/agm/  # Separate disk (write-heavy)
```

### Reduce fsync() Calls

AGM uses SQLite's `synchronous=FULL` mode by default (safest). For better performance:

```bash
# Set synchronous=NORMAL (faster, slight durability tradeoff)
sqlite3 ~/.agm/queue.db "PRAGMA synchronous=NORMAL;"

# Or synchronous=OFF (fastest, no durability guarantees)
# Only use for non-critical workloads
sqlite3 ~/.agm/queue.db "PRAGMA synchronous=OFF;"
```

**Durability vs. Performance Tradeoff:**

| Mode | Durability | Performance | Use Case |
|------|------------|-------------|----------|
| **FULL** | Highest | Slowest | Production (default) |
| **NORMAL** | High | Fast | Most use cases |
| **OFF** | None | Fastest | Testing, non-critical |

---

## Memory Management

### Daemon Memory Footprint

Reduce daemon memory usage:

```bash
# Profile memory usage
ps aux | grep agm-daemon
# Output: USER PID %CPU %MEM VSZ RSS ...

# Expected baseline: 10-20MB RSS (resident set size)

# Reduce cache sizes if memory-constrained
sqlite3 ~/.agm/queue.db "PRAGMA cache_size=-5000;"  # 5MB (default 10MB)

# Disable state caching (saves ~5MB)
# Edit ~/.config/agm/daemon-config.yaml:
state_cache:
  enabled: false
```

### Queue Memory Limits

Limit queue size to prevent memory exhaustion:

```yaml
# ~/.config/agm/daemon-config.yaml
queue:
  max_pending: 1000       # Max messages in PENDING state
  max_total: 10000        # Max total messages in database
  auto_clean: true        # Auto-clean when limits reached
  clean_threshold: 0.9    # Clean when 90% full
```

### Tmux Memory Optimization

Reduce tmux memory per session:

```conf
# ~/.tmux.conf

# Reduce history limit (saves ~1MB per session)
set-option -g history-limit 10000  # Down from 50000

# Reduce status line updates
set -g status-interval 30  # Up from 5s

# Disable scrollback buffer for non-interactive sessions
set -g history-limit 0
```

---

## Network Latency

### Remote Session Performance

When using AGM with remote sessions (SSH + tmux):

**Optimize SSH Connection:**

```bash
# ~/.ssh/config
Host remote-server
  HostName server.example.com
  User myuser
  Compression yes
  ServerAliveInterval 60
  ServerAliveCountMax 3
  ControlMaster auto
  ControlPath ~/.ssh/control-%r@%h:%p
  ControlPersist 10m
```

**Reduce Remote Polling:**

```bash
# On remote server, increase poll interval
ssh remote-server 'agm daemon restart --poll-interval 2m'

# Use local daemon for remote sessions (future feature)
agm daemon start --remote-sessions remote-server
```

### API Latency

When using AGM MCP server (future feature):

```yaml
# ~/.config/agm/mcp-config.yaml
http:
  timeout: 5s           # Request timeout
  keepalive: 30s        # Keep connections alive
  max_idle_conns: 10    # Connection pool size

cache:
  session_list: 5s      # Cache session list for 5s
  session_state: 10s    # Cache state for 10s
```

---

## Benchmarking Tools

### Built-in Benchmarks

AGM provides built-in benchmark commands:

```bash
# Benchmark queue operations
agm daemon benchmark --operations 1000
# Output:
#   Enqueue: 1000 ops in 500ms (2000 ops/s)
#   Dequeue: 1000 ops in 300ms (3333 ops/s)

# Benchmark session operations
agm session benchmark --sessions 100
# Output:
#   Create: 100 sessions in 10s (10 sess/s)
#   List: 100 sessions in 200ms
#   Resume: 100 sessions in 5s (20 sess/s)

# Benchmark message delivery
agm send benchmark --messages 100 --sessions 10
# Output:
#   Enqueue: 100 messages in 1s (100 msg/s)
#   Delivery: 100 messages in 30s (3.3 msg/s)
```

### Custom Benchmarks

Create custom benchmarks for specific workloads:

**Benchmark: Parallel Session Creation**

```bash
#!/bin/bash
# parallel-session-create.sh

SESSIONS=50
START=$(date +%s.%N)

for i in $(seq 1 $SESSIONS); do
  agm new "perf-$i" --detached &
done
wait

END=$(date +%s.%N)
DURATION=$(echo "$END - $START" | bc)
RATE=$(echo "$SESSIONS / $DURATION" | bc -l)

echo "Created $SESSIONS sessions in ${DURATION}s"
echo "Rate: $RATE sessions/s"
```

**Benchmark: Message Roundtrip Latency**

```bash
#!/bin/bash
# message-latency.sh

# Create test session
agm new latency-test --detached

# Send message with timestamp
START=$(date +%s.%N)
agm send latency-test "$(date +%s.%N)"

# Poll until delivered
while true; do
  STATUS=$(agm daemon status --queue --format json | jq -r '.queue.pending')
  if [ "$STATUS" -eq 0 ]; then
    END=$(date +%s.%N)
    LATENCY=$(echo "($END - $START) * 1000" | bc)
    echo "Message delivered in ${LATENCY}ms"
    break
  fi
  sleep 0.1
done

# Cleanup
agm session delete latency-test --force
```

---

## Performance Monitoring

### Real-Time Monitoring

Monitor AGM performance in real-time:

```bash
# Watch daemon resource usage
watch -n 1 'ps aux | grep agm-daemon | grep -v grep'

# Watch queue size
watch -n 5 'agm daemon status --queue'

# Watch session count
watch -n 5 'agm session list --format simple | wc -l'

# Watch disk I/O
iotop -p $(cat ~/.agm/daemon.pid)

# Watch network I/O (if using remote sessions)
nethogs
```

### Metrics Collection

Collect performance metrics for analysis:

```bash
# Create metrics collector script
cat > ~/agm-metrics.sh <<'EOF'
#!/bin/bash
while true; do
  TIMESTAMP=$(date +%s)
  QUEUE_JSON=$(agm daemon status --queue --format json)
  PENDING=$(echo "$QUEUE_JSON" | jq -r '.queue.pending')
  DELIVERED=$(echo "$QUEUE_JSON" | jq -r '.queue.delivered')
  FAILED=$(echo "$QUEUE_JSON" | jq -r '.queue.failed')
  MEM=$(ps aux | grep agm-daemon | grep -v grep | awk '{print $6}')
  CPU=$(ps aux | grep agm-daemon | grep -v grep | awk '{print $3}')

  echo "$TIMESTAMP,$PENDING,$DELIVERED,$FAILED,$MEM,$CPU" >> ~/.agm/metrics.csv
  sleep 60
done
EOF

chmod +x ~/agm-metrics.sh

# Run collector in background
nohup ~/agm-metrics.sh &
```

**Analyze collected metrics:**

```bash
# Plot queue growth over time
gnuplot <<EOF
set datafile separator ","
set xdata time
set timefmt "%s"
set format x "%H:%M"
set xlabel "Time"
set ylabel "Messages"
set title "AGM Queue Metrics"
plot "~/.agm/metrics.csv" using 1:2 with lines title "Pending", \
     "" using 1:3 with lines title "Delivered"
EOF
```

---

## Troubleshooting Performance Issues

### Slow Message Delivery

**Symptom:** Messages taking >5 minutes to deliver

**Diagnosis:**

```bash
# Check daemon poll interval
grep "poll cycle" ~/.agm/logs/daemon/daemon.log | tail -10
# Expected: ~30s between polls

# Check target session state
agm session get-state target-session
# Expected: DONE (not WORKING or OFFLINE)

# Check daemon CPU usage
top -p $(cat ~/.agm/daemon.pid)
# Expected: <1% CPU
```

**Solution:**

```bash
# Reduce poll interval
agm daemon restart --poll-interval 10s

# Force session to DONE if stuck
agm admin set-state target-session DONE

# Check for daemon deadlock
strace -p $(cat ~/.agm/daemon.pid)
```

### High CPU Usage

**Symptom:** agm-daemon using >10% CPU

**Diagnosis:**

```bash
# Profile CPU usage
perf top -p $(cat ~/.agm/daemon.pid)

# Check poll interval
grep "poll cycle" ~/.agm/logs/daemon/daemon.log | tail -1

# Check queue size
sqlite3 ~/.agm/queue.db "SELECT COUNT(*) FROM message_queue;"
```

**Solution:**

```bash
# Increase poll interval
agm daemon restart --poll-interval 1m

# Clean large queue
agm daemon clean --older-than 1d

# Optimize SQLite
~/agm-perf-test/optimize-queue.sh
```

### High Disk I/O

**Symptom:** High disk write rate (>1MB/s)

**Diagnosis:**

```bash
# Monitor disk I/O
iotop -p $(cat ~/.agm/daemon.pid)

# Check log file size
du -h ~/.agm/logs/daemon/daemon.log

# Check queue database size
du -h ~/.agm/queue.db*
```

**Solution:**

```bash
# Reduce logging verbosity
agm daemon restart --log-level warn

# Enable log rotation
logrotate ~/.agm/logrotate.conf

# Move logs to separate disk
mkdir /var/log/agm
ln -s /var/log/agm ~/.agm/logs
```

### Memory Leaks

**Symptom:** Daemon memory usage growing >500MB

**Diagnosis:**

```bash
# Monitor memory over time
watch -n 60 'ps aux | grep agm-daemon | grep -v grep | awk "{print \$6}"'

# Check for goroutine leaks (if Go profiling enabled)
curl http://localhost:6060/debug/pprof/goroutine?debug=1

# Check SQLite cache size
sqlite3 ~/.agm/queue.db "PRAGMA cache_size;"
```

**Solution:**

```bash
# Restart daemon (temporary fix)
agm daemon restart

# Reduce SQLite cache
sqlite3 ~/.agm/queue.db "PRAGMA cache_size=-5000;"

# Disable state caching
# Edit ~/.config/agm/daemon-config.yaml: state_cache.enabled=false
```

---

## Best Practices Summary

### Quick Wins (Low Effort, High Impact)

1. **Enable SQLite WAL mode**: `sqlite3 ~/.agm/queue.db "PRAGMA journal_mode=WAL;"`
2. **Tune poll interval**: `agm daemon start --poll-interval 10s` (for low latency)
3. **Optimize tmux config**: Add recommended settings to `~/.tmux.conf`
4. **Clean old messages**: `agm daemon clean --older-than 7d` (weekly)
5. **Use SSD**: Store `~/.agm/` on SSD, not HDD

### Advanced Optimizations (High Effort, High Impact)

1. **Implement state caching**: Reduce manifest reads by 90%
2. **Batch operations**: Send messages in batches (future feature)
3. **Concurrent delivery**: Enable parallel message delivery (future feature)
4. **Separate disk for logs**: Reduce I/O contention
5. **Profile and optimize hooks**: Reduce hook execution time to <50ms

### Configuration Template

Recommended production configuration (`~/.config/agm/daemon-config.yaml`):

```yaml
poll_interval: 10s           # Low latency

queue:
  max_pending: 1000
  max_total: 10000
  auto_clean: true

state_cache:
  enabled: true
  ttl: 10s
  max_size: 1000

logging:
  level: info
  rotation:
    enabled: true
    max_size: 100MB
    max_age: 30d

sqlite:
  journal_mode: WAL
  cache_size: -10000         # 10MB
  synchronous: NORMAL
  temp_store: MEMORY
```

---

## Appendix: Performance Checklist

Before reporting performance issues, complete this checklist:

- [ ] Verified AGM version (`agm version`)
- [ ] Checked daemon status (`agm daemon status`)
- [ ] Measured baseline metrics (session create, message send)
- [ ] Enabled SQLite WAL mode
- [ ] Optimized tmux configuration
- [ ] Cleaned old messages from queue
- [ ] Reviewed daemon logs for errors
- [ ] Monitored resource usage (CPU, memory, disk)
- [ ] Tested with minimal configuration
- [ ] Reproduced issue in isolated environment

---

**Maintained by**: AGM Performance Team
**Benchmarks Updated**: Weekly
**Feedback**: Submit performance issues at https://github.com/vbonnet/dear-agent/issues/performance
