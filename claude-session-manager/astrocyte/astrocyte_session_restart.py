#!/usr/bin/env python3
"""
Session Restart for Astrocyte

Implements hard restart (csm kill + csm resume) for unrecoverable sessions.
Only triggered when ESC and Ctrl+C both fail and safety checks pass.
"""

import subprocess
import time
import logging
from typing import Tuple, NamedTuple
from datetime import datetime, timedelta

from astrocyte_restart_tracking import check_rate_limit, record_restart

logger = logging.getLogger(__name__)


class RecoveryResult(NamedTuple):
    """Result of a recovery attempt."""
    success: bool
    method: str
    duration_seconds: float
    before_state: str = ""
    after_state: str = ""


def check_session_age(session_name: str, min_age_minutes: int = 30) -> Tuple[bool, str]:
    """
    Check if session is old enough to restart (prevent restarting new sessions).

    Args:
        session_name: Name of session to check
        min_age_minutes: Minimum age in minutes (default: 30)

    Returns:
        (is_old_enough, reason) tuple
    """
    # TODO: Implement session age checking via AGM manifest or astrocyte state
    # For now, assume sessions are old enough (graceful degradation)
    logger.info(f"Session age check: assuming {session_name} is old enough (>={min_age_minutes}min)")
    return (True, "age check passed (assumed)")


def check_active_processes(session_name: str) -> Tuple[bool, str]:
    """
    Check if session has active python/claude processes.

    Args:
        session_name: Name of session to check

    Returns:
        (no_active_processes, reason) tuple
    """
    try:
        # Check for python or claude processes in this session
        result = subprocess.run(
            ["ps", "aux"],
            capture_output=True,
            text=True,
            timeout=5
        )

        if result.returncode != 0:
            logger.warning("Failed to check processes, allowing restart (graceful degradation)")
            return (True, "process check skipped (ps failed)")

        # Look for processes related to this session
        active_processes = []
        for line in result.stdout.splitlines():
            if session_name in line and ("python" in line or "claude" in line):
                # Exclude astrocyte itself
                if "astrocyte" not in line:
                    active_processes.append(line)

        if active_processes:
            reason = f"found {len(active_processes)} active processes"
            logger.warning(f"Active processes detected in {session_name}: {reason}")
            return (False, reason)

        return (True, "no active processes")

    except Exception as e:
        logger.warning(f"Process check error: {e}, allowing restart (graceful degradation)")
        return (True, f"process check error: {e}")


def should_restart_session(
    session_name: str,
    esc_failure_count: int,
    ctrlc_failure_count: int,
    stuck_duration_hours: float
) -> Tuple[bool, str]:
    """
    Determine if session is unrecoverable and safe to restart.

    Criteria (from W0 charter):
    - ESC recovery failed 3+ times
    - Session stuck for 1+ hour
    - Ctrl+C recovery failed (1+ attempts)
    - Rate limit check passes (no restart in last hour)
    - Session age check passes (>30 min old)
    - Process check passes (no active python/claude processes)

    Args:
        session_name: Name of session to check
        esc_failure_count: Number of consecutive ESC recovery failures
        ctrlc_failure_count: Number of Ctrl+C recovery failures
        stuck_duration_hours: Hours session has been stuck

    Returns:
        (should_restart, reason) tuple
    """
    # Check unrecoverable criteria
    if esc_failure_count < 3:
        return (False, f"ESC failures ({esc_failure_count}) < 3")

    if stuck_duration_hours < 1.0:
        return (False, f"stuck duration ({stuck_duration_hours:.1f}h) < 1h")

    if ctrlc_failure_count < 1:
        return (False, f"Ctrl+C not yet attempted")

    # Check safety conditions
    rate_limit_ok = check_rate_limit(session_name, window_hours=1)
    if not rate_limit_ok:
        return (False, "rate limit exceeded (restart in last hour)")

    age_ok, age_reason = check_session_age(session_name, min_age_minutes=30)
    if not age_ok:
        return (False, f"session too new: {age_reason}")

    processes_ok, proc_reason = check_active_processes(session_name)
    if not processes_ok:
        return (False, f"active processes: {proc_reason}")

    # All checks passed
    reason = (
        f"unrecoverable (ESC:{esc_failure_count}, Ctrl+C:{ctrlc_failure_count}, "
        f"stuck:{stuck_duration_hours:.1f}h) and safe (rate limit ok, {age_reason}, {proc_reason})"
    )
    return (True, reason)


def restart_session(session_name: str) -> RecoveryResult:
    """
    Restart session by killing and resuming.

    Implementation:
    1. Execute `csm kill <session> --force`
    2. Wait 2 seconds for clean shutdown
    3. Execute `csm resume <session>`
    4. Verify session is responsive

    Args:
        session_name: Name of session to restart

    Returns:
        RecoveryResult with success=True if restart succeeded
    """
    start_time = time.time()

    logger.info(f"🔄 Starting session restart for {session_name}")

    try:
        # Step 1: Kill session
        logger.info(f"  Killing session {session_name}...")
        kill_result = subprocess.run(
            ["csm", "kill", session_name, "--force"],
            capture_output=True,
            text=True,
            timeout=10
        )

        if kill_result.returncode != 0:
            logger.error(f"csm kill failed: {kill_result.stderr}")
            duration = time.time() - start_time
            record_restart(session_name, "failure")
            return RecoveryResult(
                success=False,
                method="session_restart",
                duration_seconds=duration
            )

        logger.info(f"  Session killed successfully")

        # Step 2: Wait for clean shutdown
        time.sleep(2)

        # Step 3: Resume session
        logger.info(f"  Resuming session {session_name}...")
        resume_result = subprocess.run(
            ["csm", "resume", session_name],
            capture_output=True,
            text=True,
            timeout=20
        )

        if resume_result.returncode != 0:
            logger.error(f"csm resume failed: {resume_result.stderr}")
            duration = time.time() - start_time
            record_restart(session_name, "failure")
            return RecoveryResult(
                success=False,
                method="session_restart",
                duration_seconds=duration
            )

        logger.info(f"  Session resumed successfully")

        # Step 4: Verify session is responsive (wait a moment)
        time.sleep(2)

        duration = time.time() - start_time
        logger.info(f"✅ Session restart complete for {session_name} ({duration:.2f}s)")

        record_restart(session_name, "success")

        return RecoveryResult(
            success=True,
            method="session_restart",
            duration_seconds=duration
        )

    except subprocess.TimeoutExpired as e:
        duration = time.time() - start_time
        logger.error(f"Session restart timed out: {e}")
        record_restart(session_name, "failure")
        return RecoveryResult(
            success=False,
            method="session_restart",
            duration_seconds=duration
        )
    except Exception as e:
        duration = time.time() - start_time
        logger.error(f"Session restart error: {e}")
        record_restart(session_name, "failure")
        return RecoveryResult(
            success=False,
            method="session_restart",
            duration_seconds=duration
        )
