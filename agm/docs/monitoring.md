# AGM Daemon Monitoring & Alerting

This document describes the monitoring and alerting capabilities built into the AGM daemon.

## Overview

The AGM daemon includes comprehensive monitoring that tracks:
- Queue depth and delivery metrics
- Delivery latency statistics
- State detection accuracy
- Overall daemon health

## Metrics Collected

### Delivery Metrics
- **Total Messages Delivered**: Count of successfully delivered messages
- **Total Messages Failed**: Count of permanently failed messages
- **Total Delivery Attempts**: Count of all delivery attempts (including retries)
- **Success Rate**: Percentage of successful deliveries
- **Delivery Latency**: Min/Max/Average time to deliver a message

### Queue Metrics
- **Current Queue Depth**: Number of messages waiting in queue
- **Queue Stats by Status**: Count of queued/delivered/failed messages

### State Detection Metrics
- **State Detection Accuracy**: Count of successful state detections by state type
- **State Detection Errors**: Count of failed state detections

### Daemon Health Metrics
- **Uptime**: How long the daemon has been running
- **Last Poll Time**: When the daemon last checked for messages
- **Last Poll Duration**: How long the last poll cycle took

## Health Check Command

Check daemon health and metrics:

```bash
agm session daemon health
```

This command displays:
- Overall health status (healthy/degraded/critical)
- Daemon running status and PID
- Queue statistics
- Recommendations for any issues

Example output:

```
=== AGM Daemon Health Status ===

✓ Overall Status: HEALTHY
  Daemon Running: true
  PID: 12345

=== Queue Statistics ===
  Queued Messages: 5
  Delivered Messages: 142
  Failed Messages: 2

=== Recommendations ===
  - System is operating normally.

Logs: ~/.agm/logs/daemon/daemon.log
```

## Alert Rules

The daemon continuously evaluates alert rules and logs warnings when thresholds are exceeded.

### Queue Depth Alerts

**Warning**: Queue depth > 50 messages
- Indicates the daemon may be falling behind
- Review daemon logs for delivery issues

**Critical**: Queue depth > 100 messages
- Queue is backing up significantly
- Check for:
  - Recipient sessions stuck in WORKING state
  - Network/system issues
  - State detection problems

### Delivery Success Rate Alerts

**Warning**: Success rate < 75% (after 10+ attempts)
- Some messages are failing to deliver
- Review dead letter queue: `agm queue dlq`

**Critical**: Success rate < 50% (after 10+ attempts)
- Majority of messages are failing
- Check:
  - Session state detection
  - Tmux connectivity
  - Message format issues

### Delivery Latency Alerts

**Warning**: Average latency > 10 seconds
- Deliveries are slower than normal
- Monitor system load

**Critical**: Average latency > 30 seconds
- Delivery performance is severely degraded
- Check:
  - System resource utilization
  - Network latency
  - Daemon configuration

### Daemon Polling Alerts

**Critical**: No poll activity for > 5 minutes
- Daemon may have crashed or hung
- Restart daemon: `agm session daemon restart`

### State Detection Error Rate Alerts

**Warning**: Error rate > 10% (after 10+ detections)
- Some sessions are difficult to detect
- Review session manifests

**Critical**: Error rate > 25% (after 10+ detections)
- State detection is frequently failing
- Check:
  - Tmux session configuration
  - Session manifest validity
  - File system permissions

## Monitoring in Logs

All alerts are automatically logged to the daemon log file:

```bash
tail -f ~/.agm/logs/daemon/daemon.log
```

Alert log format:

```
[LEVEL] Message: value (threshold: threshold_value)
```

Example:

```
2026-02-20 10:15:30 [WARNING] Queue depth exceeds warning threshold: 55 (threshold: 50)
2026-02-20 10:16:00 [INFO] Delivered message msg-123 to session-a (latency: 245ms)
```

## Integration Points

### Programmatic Access

The monitoring system can be integrated with external tools:

```go
// Get daemon health status
pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")
queue, _ := messages.NewMessageQueue()
health, _ := daemon.GetHealthStatus(pidFile, queue)

// Check health level
if health.HealthStatusLevel == "critical" {
    // Take action
}

// Access metrics from running daemon
d := daemon.NewDaemon(cfg)
metrics := d.GetMetrics()
fmt.Printf("Queue depth: %d\n", metrics.CurrentQueueDepth)
```

### Custom Alert Rules

Add custom alert rules by extending the default rules:

```go
customRules := daemon.GetDefaultAlertRules()
customRules = append(customRules, daemon.AlertRule{
    Name: "Very High Queue",
    Condition: func(m daemon.MetricsSnapshot) *daemon.Alert {
        if m.CurrentQueueDepth > 200 {
            return &daemon.Alert{
                Level:     daemon.AlertLevelCritical,
                Timestamp: time.Now(),
                Message:   "Queue depth extremely high",
                Metric:    "queue_depth",
                Value:     m.CurrentQueueDepth,
                Threshold: 200,
            }
        }
        return nil
    },
})
```

## Dashboard Integration

While the daemon doesn't include a built-in dashboard, metrics can be exported to monitoring systems:

### Prometheus-style Metrics

Metrics are available programmatically and can be exposed via HTTP endpoint:

```
agm_daemon_uptime_seconds
agm_messages_delivered_total
agm_messages_failed_total
agm_delivery_latency_seconds{quantile="0.5"}
agm_delivery_latency_seconds{quantile="0.95"}
agm_delivery_latency_seconds{quantile="0.99"}
agm_queue_depth_current
agm_state_detection_errors_total
```

### Log-based Monitoring

Use log aggregation tools (e.g., Loki, Elasticsearch) to:
- Track alert frequency over time
- Visualize delivery patterns
- Correlate daemon events with system metrics

## Performance Considerations

- Metrics collection adds minimal overhead (<1ms per operation)
- Latency tracking maintains a rolling window of last 100 deliveries
- Alert evaluation occurs once per poll cycle (30 seconds)
- No external dependencies required

## Troubleshooting

### High Queue Depth

1. Check if recipient sessions are in DONE state:
   ```bash
   agm session status <session-name>
   ```

2. Review daemon logs for delivery errors:
   ```bash
   grep -i "error\|failed" ~/.agm/logs/daemon/daemon.log | tail -20
   ```

3. Check dead letter queue:
   ```bash
   agm queue dlq
   ```

### High Failure Rate

1. Verify message format and content
2. Check session state transitions
3. Review tmux connectivity:
   ```bash
   tmux list-sessions
   ```

### Daemon Not Responding

1. Check daemon status:
   ```bash
   agm session daemon status
   ```

2. Review daemon logs for crashes:
   ```bash
   tail -50 ~/.agm/logs/daemon/daemon.log
   ```

3. Restart daemon:
   ```bash
   agm session daemon restart
   ```

## Best Practices

1. **Regular Health Checks**: Run `agm session daemon health` daily
2. **Log Monitoring**: Set up alerts on daemon log errors
3. **Queue Depth Limits**: Keep queue depth below 50 for optimal performance
4. **Retention**: Clean up old delivered/failed messages regularly
5. **Metrics Review**: Periodically review success rates and latencies

## Future Enhancements

Potential monitoring improvements:
- HTTP endpoint for metrics export
- Real-time dashboard
- Integration with Prometheus/Grafana
- Email/Slack notifications for critical alerts
- Historical metrics database
- Performance trending analysis
