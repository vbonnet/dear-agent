#!/usr/bin/env python3
"""
Restart State Tracking for Astrocyte

Tracks session restart history to enable rate limiting and prevent infinite restart loops.

File format: ~/.csm/astrocyte/restart-history.jsonl
{"session": "name", "timestamp": "2026-02-03T19:57:00Z", "outcome": "success"}
"""

import json
import os
from datetime import datetime, timedelta
from pathlib import Path
from typing import Optional
import logging

logger = logging.getLogger(__name__)

RESTART_HISTORY_FILE = Path.home() / ".csm" / "astrocyte" / "restart-history.jsonl"


def ensure_history_file_exists() -> None:
    """Create restart history file if it doesn't exist."""
    RESTART_HISTORY_FILE.parent.mkdir(parents=True, exist_ok=True)
    if not RESTART_HISTORY_FILE.exists():
        RESTART_HISTORY_FILE.touch()


def record_restart(session_name: str, outcome: str) -> None:
    """
    Append restart event to history file.

    Args:
        session_name: Name of session that was restarted
        outcome: "success" or "failure"
    """
    try:
        ensure_history_file_exists()

        event = {
            "session": session_name,
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "outcome": outcome
        }

        with open(RESTART_HISTORY_FILE, "a") as f:
            f.write(json.dumps(event) + "\n")

        logger.info(f"Recorded restart: {session_name} - {outcome}")

    except Exception as e:
        logger.error(f"Failed to record restart: {e}")


def get_last_restart_time(session_name: str) -> Optional[datetime]:
    """
    Get most recent restart timestamp for session.

    Args:
        session_name: Name of session to check

    Returns:
        datetime of last restart, or None if never restarted
    """
    try:
        if not RESTART_HISTORY_FILE.exists():
            return None

        last_restart = None

        with open(RESTART_HISTORY_FILE, "r") as f:
            for line in f:
                try:
                    event = json.loads(line.strip())
                    if event.get("session") == session_name:
                        timestamp_str = event.get("timestamp", "")
                        # Parse ISO 8601 timestamp (remove trailing Z)
                        timestamp = datetime.fromisoformat(timestamp_str.replace("Z", ""))
                        if last_restart is None or timestamp > last_restart:
                            last_restart = timestamp
                except (json.JSONDecodeError, ValueError) as e:
                    logger.warning(f"Skipping malformed line in restart history: {e}")
                    continue

        return last_restart

    except Exception as e:
        logger.error(f"Failed to read restart history: {e}")
        return None


def check_rate_limit(session_name: str, window_hours: int = 1) -> bool:
    """
    Check if restart is allowed (no restart in last N hours).

    Args:
        session_name: Name of session to check
        window_hours: Rate limit window in hours (default: 1)

    Returns:
        True if restart allowed (rate limit not exceeded), False otherwise
    """
    last_restart = get_last_restart_time(session_name)

    if last_restart is None:
        # Never restarted before - allowed
        return True

    now = datetime.utcnow()
    time_since_last = now - last_restart
    window = timedelta(hours=window_hours)

    if time_since_last < window:
        logger.warning(
            f"Rate limit exceeded: {session_name} restarted {time_since_last.total_seconds():.0f}s ago "
            f"(window: {window_hours}h)"
        )
        return False

    return True
