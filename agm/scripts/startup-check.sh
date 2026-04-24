#!/bin/bash
echo "[dear-agent] Startup checks..."
pkill -f "cloud-code" 2>/dev/null && echo "  Killed Cloud Code" || echo "  Cloud Code not running"
command -v agm &>/dev/null && echo "  AGM: $(agm version 2>&1 | head -1)" || echo "  WARNING: agm not found"
echo "  Load: $(uptime | awk -F'load average:' '{print $2}' | awk -F',' '{print $1}')"
