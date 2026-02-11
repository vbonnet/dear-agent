#!/usr/bin/env bash
#
# Astrocyte Daemon Startup Script
# Used by cron @reboot to auto-start daemon after machine reboots

# Wait for system to be fully up
sleep 10

# Set up environment
export PATH="/usr/local/bin:/usr/bin:/bin:$PATH"

# Log directory
LOG_DIR="$HOME/.agm/astrocyte/logs"
mkdir -p "$LOG_DIR"

# Start daemon with output redirected to startup log
cd "$HOME/src/ws/oss/repos/ai-tools/main/claude-session-manager/astrocyte"
nohup python3 "$HOME/src/ws/oss/repos/ai-tools/main/claude-session-manager/astrocyte/astrocyte-daemon.py" \
    >> "$LOG_DIR/startup.log" 2>&1 &

# Log startup event
echo "$(date '+%Y-%m-%d %H:%M:%S') - Astrocyte daemon started (PID: $!)" >> "$LOG_DIR/startup.log"
