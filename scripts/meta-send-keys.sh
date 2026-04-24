#!/usr/bin/env bash
# meta-send-keys.sh — Break-glass tmux send-keys wrapper for meta-orchestrator only.
#
# Attempts agm send msg 3 times before falling back to raw tmux send-keys.
# Every invocation is logged to the audit log and alerts are written on fallback.
#
# Usage:
#   meta-send-keys.sh --emergency-reason "REASON" --target SESSION --prompt "MSG"

set -euo pipefail

# --- Constants ---------------------------------------------------------------
AUDIT_LOG="${HOME}/.agm/logs/sendkeys-audit.log"
ALERT_FILE="${HOME}/.agm/alerts/sendkeys-used.txt"
AGM_SEND_RETRIES=3
RETRY_DELAY=2

# --- Arg parsing -------------------------------------------------------------
EMERGENCY_REASON=""
TARGET=""
PROMPT=""
SENDER="${AGM_SESSION_NAME:-meta-orchestrator}"

usage() {
  cat <<EOF
Usage: $(basename "$0") --emergency-reason REASON --target SESSION --prompt MSG [--sender NAME]

Break-glass tmux send-keys wrapper. Requires --emergency-reason flag.
Attempts agm send msg ${AGM_SEND_RETRIES} times before falling back to tmux send-keys.

Options:
  --emergency-reason  REQUIRED. Why send-keys fallback may be needed.
  --target            Target tmux session name.
  --prompt            Message to deliver.
  --sender            Sender name (default: \$AGM_SESSION_NAME or meta-orchestrator).
EOF
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --emergency-reason) EMERGENCY_REASON="$2"; shift 2 ;;
    --target)           TARGET="$2";           shift 2 ;;
    --prompt)           PROMPT="$2";           shift 2 ;;
    --sender)           SENDER="$2";           shift 2 ;;
    -h|--help)          usage ;;
    *)                  echo "Unknown flag: $1" >&2; usage ;;
  esac
done

# --- Validation --------------------------------------------------------------
if [[ -z "${EMERGENCY_REASON}" ]]; then
  echo "ERROR: --emergency-reason is required." >&2
  exit 1
fi
if [[ -z "${TARGET}" ]]; then
  echo "ERROR: --target is required." >&2
  exit 1
fi
if [[ -z "${PROMPT}" ]]; then
  echo "ERROR: --prompt is required." >&2
  exit 1
fi

# --- Helpers -----------------------------------------------------------------
timestamp() { date -u +"%Y-%m-%dT%H:%M:%SZ"; }

log_audit() {
  local status="$1" method="$2" detail="${3:-}"
  mkdir -p "$(dirname "${AUDIT_LOG}")"
  printf '%s | status=%s method=%s target=%s sender=%s reason="%s" detail="%s"\n' \
    "$(timestamp)" "${status}" "${method}" "${TARGET}" "${SENDER}" \
    "${EMERGENCY_REASON}" "${detail}" >> "${AUDIT_LOG}"
}

write_alert() {
  mkdir -p "$(dirname "${ALERT_FILE}")"
  cat >> "${ALERT_FILE}" <<EOF
---
timestamp: $(timestamp)
target: ${TARGET}
sender: ${SENDER}
reason: ${EMERGENCY_REASON}
method: tmux-send-keys
---
EOF
}

# --- Attempt agm send msg (3 retries) ---------------------------------------
agm_succeeded=false
for attempt in $(seq 1 "${AGM_SEND_RETRIES}"); do
  echo "Attempt ${attempt}/${AGM_SEND_RETRIES}: agm send msg ${TARGET}" >&2
  if agm send msg "${TARGET}" --prompt "${PROMPT}" --sender "${SENDER}" 2>/dev/null; then
    log_audit "OK" "agm-send-msg" "attempt=${attempt}"
    agm_succeeded=true
    echo "Delivered via agm send msg (attempt ${attempt})." >&2
    break
  fi
  if [[ "${attempt}" -lt "${AGM_SEND_RETRIES}" ]]; then
    sleep "${RETRY_DELAY}"
  fi
done

if "${agm_succeeded}"; then
  exit 0
fi

# --- Fallback: tmux send-keys ------------------------------------------------
echo "WARNING: agm send msg failed after ${AGM_SEND_RETRIES} attempts. Falling back to tmux send-keys." >&2

log_audit "FALLBACK" "tmux-send-keys" "agm-retries-exhausted"
write_alert

if tmux send-keys -t "${TARGET}" "${PROMPT}" Enter 2>/dev/null; then
  log_audit "OK" "tmux-send-keys" "fallback-delivered"
  echo "Delivered via tmux send-keys (break-glass)." >&2
  exit 0
else
  log_audit "FAIL" "tmux-send-keys" "tmux-send-keys-failed"
  echo "ERROR: Both agm send msg and tmux send-keys failed." >&2
  exit 1
fi
