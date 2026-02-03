#!/usr/bin/env python3
"""
Recovery Escalation Tracking for Astrocyte

Tracks recent recovery attempts to prevent infinite loops where ESC "succeeds"
but the session gets stuck again immediately.

Strategy:
- If session gets stuck again within 5 minutes of successful recovery,
  skip strategies that were already tried and escalate to next one
- This prevents ESC → unstuck → stuck → ESC → unstuck → stuck loops

File format: ~/.csm/astrocyte/recovery-escalation.jsonl
{"session": "name", "timestamp": "2026-02-03T21:30:00Z", "method": "escape", "success": true}
"""

import json
import os
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional, List
import logging

logger = logging.getLogger(__name__)

ESCALATION_HISTORY_FILE = Path.home() / ".csm" / "astrocyte" / "recovery-escalation.jsonl"
ESCALATION_WINDOW_MINUTES = 5  # If stuck again within 5 minutes, escalate


def ensure_history_file_exists() -> None:
    """Create escalation history file if it doesn't exist."""
    ESCALATION_HISTORY_FILE.parent.mkdir(parents=True, exist_ok=True)
    if not ESCALATION_HISTORY_FILE.exists():
        ESCALATION_HISTORY_FILE.touch()


def record_recovery_attempt(session_name: str, method: str, success: bool) -> None:
    """
    Record recovery attempt to history file.

    Args:
        session_name: Name of session
        method: Recovery method used ("escape", "ctrl_c", "session_restart", etc.)
        success: Whether recovery succeeded
    """
    try:
        ensure_history_file_exists()

        event = {
            "session": session_name,
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "method": method,
            "success": success
        }

        with open(ESCALATION_HISTORY_FILE, "a") as f:
            f.write(json.dumps(event) + "\n")

        logger.info(f"Recorded recovery: {session_name} - {method} - {'success' if success else 'failure'}")

    except Exception as e:
        logger.error(f"Failed to record recovery attempt: {e}")


def get_recent_recovery_methods(session_name: str, window_minutes: int = ESCALATION_WINDOW_MINUTES) -> List[str]:
    """
    Get list of recovery methods tried within the last N minutes.

    Args:
        session_name: Name of session to check
        window_minutes: Time window in minutes (default: 5)

    Returns:
        List of method names that were tried recently (e.g., ["escape", "ctrl_c"])
    """
    try:
        if not ESCALATION_HISTORY_FILE.exists():
            return []

        cutoff_time = datetime.utcnow() - timedelta(minutes=window_minutes)
        recent_methods = []

        with open(ESCALATION_HISTORY_FILE, "r") as f:
            for line in f:
                try:
                    event = json.loads(line.strip())
                    if event.get("session") == session_name:
                        timestamp_str = event.get("timestamp", "")
                        timestamp = datetime.fromisoformat(timestamp_str.replace("Z", ""))

                        if timestamp >= cutoff_time:
                            method = event.get("method", "")
                            if method and method not in recent_methods:
                                recent_methods.append(method)

                except (json.JSONDecodeError, ValueError) as e:
                    logger.warning(f"Skipping malformed line in escalation history: {e}")
                    continue

        return recent_methods

    except Exception as e:
        logger.error(f"Failed to read escalation history: {e}")
        return []


def get_escalated_chain(full_chain: List[str], session_name: str) -> List[str]:
    """
    Get escalated recovery chain that skips recently-tried methods.

    Args:
        full_chain: Full recovery chain (e.g., ["escape", "ctrl_c", "session_restart"])
        session_name: Name of session to check

    Returns:
        Escalated chain with already-tried methods removed
        (e.g., if "escape" was tried recently, returns ["ctrl_c", "session_restart"])
    """
    recent_methods = get_recent_recovery_methods(session_name)

    if not recent_methods:
        # No recent attempts, use full chain
        return full_chain

    # Filter out recently-tried methods
    escalated = [method for method in full_chain if method not in recent_methods]

    if not escalated:
        # All methods were tried recently, use full chain anyway
        # (This handles the case where we've tried everything and need to try again)
        return full_chain

    logger.info(f"Escalating recovery for {session_name}: skipping {recent_methods}, trying {escalated}")
    return escalated


def cleanup_old_entries(days: int = 7) -> None:
    """
    Remove entries older than N days from history file.

    Args:
        days: Keep entries from last N days (default: 7)
    """
    try:
        if not ESCALATION_HISTORY_FILE.exists():
            return

        cutoff_time = datetime.utcnow() - timedelta(days=days)
        kept_entries = []

        with open(ESCALATION_HISTORY_FILE, "r") as f:
            for line in f:
                try:
                    event = json.loads(line.strip())
                    timestamp_str = event.get("timestamp", "")
                    timestamp = datetime.fromisoformat(timestamp_str.replace("Z", ""))

                    if timestamp >= cutoff_time:
                        kept_entries.append(line.strip())

                except (json.JSONDecodeError, ValueError):
                    continue

        # Rewrite file with only recent entries
        with open(ESCALATION_HISTORY_FILE, "w") as f:
            for entry in kept_entries:
                f.write(entry + "\n")

        logger.info(f"Cleaned up escalation history: kept {len(kept_entries)} entries")

    except Exception as e:
        logger.error(f"Failed to cleanup escalation history: {e}")
