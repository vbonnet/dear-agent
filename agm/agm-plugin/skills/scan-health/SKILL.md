---
name: scan-health
description: Run system health checks — disk, CPU, session count, heartbeats
arguments: none
---

Check system health and report anomalies:
1. Disk: df -h /home — alert if >80% used
2. CPU: read /proc/loadavg — alert if load > 10
3. Sessions: agm -C "$HOME" session list | wc -l — alert if > 20
4. Report: "Health OK" or list specific warnings

This replaces ad-hoc health checks in orchestrator scan loops.
