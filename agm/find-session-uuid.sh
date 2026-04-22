#!/bin/bash
# find-session-uuid.sh - Find Claude session UUIDs based on timestamp
#
# NOTE: As of AGM v2.x, new sessions automatically capture the UUID.
# This script is for legacy sessions or orphaned sessions created before auto-capture.
#
# Usage:
#   ./find-session-uuid.sh <session-name> <timestamp>
#   ./find-session-uuid.sh <session-name>  # Uses creation time from manifest
#
# Examples:
#   ./find-session-uuid.sh lock 1734458928000
#   ./find-session-uuid.sh lock  # Auto-detects from manifest

set -euo pipefail

SESSION_NAME="${1:-}"
TIMESTAMP="${2:-}"
SESSIONS_DIR="${AGM_SESSIONS_DIR:-$HOME/src/sessions}"
HISTORY_FILE="$HOME/.claude/history.jsonl"
WINDOW_MINUTES=10

usage() {
    echo "Usage: $0 <session-name> [timestamp-ms]"
    echo ""
    echo "Find Claude session UUID(s) around the time a AGM session was created."
    echo ""
    echo "Arguments:"
    echo "  session-name    Name of the AGM session (e.g., 'lock', 'atlassian-mcp')"
    echo "  timestamp-ms    Optional: Unix timestamp in milliseconds"
    echo "                  If not provided, reads from session manifest"
    echo ""
    echo "Examples:"
    echo "  $0 lock 1734458928000"
    echo "  $0 lock  # Reads timestamp from manifest"
    exit 1
}

if [ -z "$SESSION_NAME" ]; then
    usage
fi

# Find manifest
MANIFEST=""
if [ -f "$SESSIONS_DIR/session-$SESSION_NAME/manifest.yaml" ]; then
    MANIFEST="$SESSIONS_DIR/session-$SESSION_NAME/manifest.yaml"
elif [ -d "$SESSIONS_DIR" ]; then
    # Search for session with this name (handles timestamped names)
    for dir in "$SESSIONS_DIR"/session-*"$SESSION_NAME"*/; do
        if [ -f "$dir/manifest.yaml" ]; then
            MANIFEST="$dir/manifest.yaml"
            break
        fi
    done
fi

# Get timestamp
if [ -z "$TIMESTAMP" ]; then
    if [ -z "$MANIFEST" ]; then
        echo "Error: Session '$SESSION_NAME' not found in $SESSIONS_DIR"
        echo "Please provide timestamp manually."
        exit 1
    fi

    # Extract created_at from manifest and convert to milliseconds
    CREATED_AT=$(grep '^created_at:' "$MANIFEST" | awk '{print $2}')
    if [ -z "$CREATED_AT" ]; then
        echo "Error: Could not find created_at in $MANIFEST"
        exit 1
    fi

    # Convert ISO 8601 timestamp to milliseconds
    TIMESTAMP=$(date -d "$CREATED_AT" +%s%3N 2>/dev/null || echo "")
    if [ -z "$TIMESTAMP" ]; then
        echo "Error: Could not parse timestamp: $CREATED_AT"
        exit 1
    fi

    echo "Session: $SESSION_NAME"
    echo "Created: $CREATED_AT"
    echo "Timestamp: $TIMESTAMP ms"
else
    echo "Session: $SESSION_NAME"
    echo "Timestamp: $TIMESTAMP ms (provided)"
fi

# Check if history file exists
if [ ! -f "$HISTORY_FILE" ]; then
    echo "Error: Claude history not found at $HISTORY_FILE"
    exit 1
fi

# Calculate time window
START_TIME=$((TIMESTAMP - WINDOW_MINUTES * 60 * 1000))
END_TIME=$((TIMESTAMP + WINDOW_MINUTES * 60 * 1000))

echo "Searching ±$WINDOW_MINUTES minutes:"
echo "  From: $(date -d @$((START_TIME/1000)) '+%Y-%m-%d %H:%M:%S')"
echo "  To:   $(date -d @$((END_TIME/1000)) '+%Y-%m-%d %H:%M:%S')"
echo ""

# Find sessions in time range
echo "Claude sessions found:"
awk -v start="$START_TIME" -v end="$END_TIME" '
    /"timestamp":/ {
        match($0, /"timestamp":([0-9]+)/, ts)
        match($0, /"sessionId":"([^"]+)"/, sid)
        if (ts[1] >= start && ts[1] <= end && sid[1] != "") {
            sessions[sid[1]] = ts[1]
        }
    }
    END {
        for (uuid in sessions) {
            # Convert timestamp to readable format (avoid scientific notation)
            ts_sec = int(sessions[uuid]/1000)
            cmd = "date -d @" ts_sec " \047+%Y-%m-%d %H:%M:%S\047"
            cmd | getline time_str
            close(cmd)
            print "  " time_str " - " uuid
        }
    }
' "$HISTORY_FILE" | sort

UUIDS=$(awk -v start="$START_TIME" -v end="$END_TIME" '
    /"timestamp":/ {
        match($0, /"timestamp":([0-9]+)/, ts)
        match($0, /"sessionId":"([^"]+)"/, sid)
        if (ts[1] >= start && ts[1] <= end && sid[1] != "") {
            sessions[sid[1]] = ts[1]
        }
    }
    END {
        for (uuid in sessions) {
            print uuid
        }
    }
' "$HISTORY_FILE" | sort -u)

COUNT=$(echo "$UUIDS" | grep -c . || echo "0")

echo ""
if [ "$COUNT" -eq 0 ]; then
    echo "No Claude sessions found in time window."
    echo "Try widening the search window or check if the session was created."
elif [ "$COUNT" -eq 1 ]; then
    UUID=$(echo "$UUIDS" | head -1)
    echo "Found exactly 1 UUID: $UUID"
    echo ""
    echo "To associate this UUID with the session, run:"
    echo "  agm session associate $SESSION_NAME --uuid $UUID"
else
    echo "Found $COUNT UUIDs. You may need to manually identify the correct one."
    echo "Check the timestamps and your shell history to narrow it down."
fi
