#!/bin/bash
# Quick resource health check — outputs JSON summary.
# Designed for scheduled tasks and /loop ticks.
# Falls back to raw ps/sysctl if resource-monitor binary is not installed.
#
# Usage:
#   resource-check.sh [--kill]
#
# Output: one JSON line to stdout + appends to ~/.agm/logs/resource-monitor.jsonl

set -euo pipefail

KILL_FLAG=""
if [[ "${1:-}" == "--kill" ]]; then
  KILL_FLAG="--kill"
fi

# Prefer the compiled binary.
if command -v resource-monitor &>/dev/null; then
  exec resource-monitor --json $KILL_FLAG
fi

# Fallback: pure shell JSON summary.
log() { echo >&2 "[resource-check] $*"; }

page_size=$(sysctl -n hw.pagesize 2>/dev/null || echo 4096)
total_bytes=$(sysctl -n hw.memsize)
total_mb=$(( total_bytes / 1024 / 1024 ))

free_pages=$(vm_stat | awk '/Pages free:/ { gsub(/\./,""); print $3 }')
free_mb=$(( (free_pages * page_size) / 1024 / 1024 ))
used_mb=$(( total_mb - free_mb ))
used_pct=$(awk "BEGIN { printf \"%.0f\", ($used_mb / $total_mb) * 100 }")

swap_raw=$(sysctl -n vm.swapusage 2>/dev/null || echo "total = 0M  used = 0M")
swap_used=$(echo "$swap_raw" | grep -oE 'used = [0-9.]+M' | grep -oE '[0-9]+')
swap_total=$(echo "$swap_raw" | grep -oE 'total = [0-9.]+M' | grep -oE '[0-9]+')
swap_used=${swap_used:-0}
swap_total=${swap_total:-0}

# Top watched processes (name, count, rss_mb)
proc_json=$(ps -axo rss,comm 2>/dev/null | awk '
  NR==1 { next }
  {
    rss=$1; name=$2
    gsub(/.*\//, "", name)
    for (w in watched) {
      if (name ~ watched[w]) {
        cnt[w]++; mem[w] += rss
      }
    }
  }
  BEGIN {
    watched["claude"]="claude"
    watched["gopls"]="gopls"
    watched["llama"]="llama-server"
    watched["agm"]="agm-mcp-server"
  }
  END {
    printf "["
    sep=""
    for (w in cnt) {
      printf "%s{\"name\":\"%s\",\"count\":%d,\"rss_mb\":%d}", sep, w, cnt[w], mem[w]/1024
      sep=","
    }
    printf "]"
  }
')

# Alerts
alerts="[]"
if (( used_pct > 80 )); then
  alerts="[\"memory ${used_pct}% used (${used_mb}/${total_mb} MB)\"]"
fi
if (( swap_used > 1024 )); then
  extra="\"swap high: ${swap_used}/${swap_total} MB\""
  if [[ "$alerts" == "[]" ]]; then
    alerts="[$extra]"
  else
    alerts="${alerts%]},${extra}]"
  fi
fi

ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
json=$(printf '{"ts":"%s","mem_total_mb":%d,"mem_free_mb":%d,"mem_used_pct":%s,"swap_used_mb":%s,"swap_total_mb":%s,"processes":%s,"orphans":[],"zombies":[],"alerts":%s}' \
  "$ts" "$total_mb" "$free_mb" "$used_pct" "$swap_used" "$swap_total" "$proc_json" "$alerts")

# Log to file.
log_file="${HOME}/.agm/logs/resource-monitor.jsonl"
mkdir -p "$(dirname "$log_file")"
echo "$json" >> "$log_file"

echo "$json"
