#!/usr/bin/env python3
"""
Ctrl+C Recovery for Astrocyte

Sends Ctrl+C key sequence to stuck sessions as a recovery method.
User feedback: "sometimes Ctrl-C works better than ESC"
"""

import subprocess
import time
import logging
from typing import NamedTuple

logger = logging.getLogger(__name__)


class RecoveryResult(NamedTuple):
    """Result of a recovery attempt."""
    success: bool
    method: str
    duration_seconds: float
    before_state: str = ""
    after_state: str = ""


def capture_pane_state(session_name: str) -> str:
    """
    Capture current tmux pane contents.

    Args:
        session_name: Name of tmux session

    Returns:
        Pane contents as string, or empty string on error
    """
    try:
        result = subprocess.run(
            ["tmux", "capture-pane", "-p", "-t", session_name],
            capture_output=True,
            text=True,
            timeout=5
        )
        return result.stdout if result.returncode == 0 else ""
    except Exception as e:
        logger.error(f"Failed to capture pane state: {e}")
        return ""


def try_ctrlc_recovery(session_name: str) -> RecoveryResult:
    """
    Attempt to recover stuck session by sending Ctrl+C.

    Implementation:
    1. Capture pane state before
    2. Send Ctrl+C (tmux send-keys C-c)
    3. Wait 2 seconds
    4. Capture pane state after
    5. Check if state changed (recovery indicator)

    Args:
        session_name: Name of tmux session to recover

    Returns:
        RecoveryResult with success=True if pane state changed
    """
    start_time = time.time()

    logger.info(f"Attempting Ctrl+C recovery for {session_name}")

    # Capture state before
    before_state = capture_pane_state(session_name)

    try:
        # Send Ctrl+C
        result = subprocess.run(
            ["tmux", "send-keys", "-t", session_name, "C-c"],
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode != 0:
            logger.error(f"Failed to send Ctrl+C: {result.stderr}")
            duration = time.time() - start_time
            return RecoveryResult(
                success=False,
                method="ctrlc",
                duration_seconds=duration,
                before_state=before_state,
                after_state=""
            )

        # Wait for recovery
        time.sleep(2)

        # Capture state after
        after_state = capture_pane_state(session_name)

        # Check if state changed
        success = before_state != after_state and len(after_state) > 0

        duration = time.time() - start_time

        if success:
            logger.info(f"Ctrl+C recovery successful for {session_name} ({duration:.2f}s)")
        else:
            logger.warning(f"Ctrl+C recovery failed for {session_name} (no state change)")

        return RecoveryResult(
            success=success,
            method="ctrlc",
            duration_seconds=duration,
            before_state=before_state,
            after_state=after_state
        )

    except subprocess.TimeoutExpired:
        duration = time.time() - start_time
        logger.error(f"Ctrl+C recovery timed out for {session_name}")
        return RecoveryResult(
            success=False,
            method="ctrlc",
            duration_seconds=duration,
            before_state=before_state,
            after_state=""
        )
    except Exception as e:
        duration = time.time() - start_time
        logger.error(f"Ctrl+C recovery error: {e}")
        return RecoveryResult(
            success=False,
            method="ctrlc",
            duration_seconds=duration,
            before_state=before_state,
            after_state=""
        )
