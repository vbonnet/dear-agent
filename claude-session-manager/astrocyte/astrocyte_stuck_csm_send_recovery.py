#!/usr/bin/env python3
"""
Stuck csm send Process Recovery for Astrocyte

This module provides detection and recovery for stuck csm send processes
that block session input for hours.

Problem: csm send processes can hang for 10+ hours, blocking all session input
including Ctrl+C. This prevents normal recovery mechanisms from working.

Solution: Detect csm send processes older than 10 minutes targeting a specific
session and kill them (along with their tmux send-keys children) before
attempting other recovery methods.
"""

import os
import re
import signal
import subprocess
import time
from typing import List, Tuple, Optional
import logging

logger = logging.getLogger(__name__)


def find_stuck_csm_send_processes(session_name: str, min_age_seconds: int = 600) -> List[Tuple[int, float]]:
    """
    Find csm send processes targeting this session that are older than min_age_seconds.

    Args:
        session_name: Name of the tmux session to check
        min_age_seconds: Minimum age in seconds to consider a process stuck (default: 600 = 10 minutes)

    Returns:
        List of (pid, age_seconds) tuples for stuck processes

    Implementation:
        Uses 'ps aux' to find processes matching "csm send <session_name>"
        Parses elapsed time (etime) to determine process age
        Returns processes older than threshold
    """
    stuck_processes = []

    try:
        # Use ps to find csm send processes with elapsed time
        # Format: user,pid,etime,args
        cmd = ['ps', 'aux']
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=5)

        if result.returncode != 0:
            logger.error(f"Failed to run ps command: {result.stderr}")
            return stuck_processes

        # Parse ps output looking for csm send processes
        for line in result.stdout.splitlines():
            # Look for "csm send <session_name>" in the command line
            if f'csm send {session_name}' not in line and f'csm send "{session_name}"' not in line:
                continue

            # Parse the line to extract PID and elapsed time
            # ps aux format: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
            parts = line.split(None, 10)
            if len(parts) < 11:
                continue

            try:
                pid = int(parts[1])
                # The START column (parts[8]) contains elapsed time for long-running processes
                # or start time for recent processes. We need to use a different approach.

                # Use ps -p PID -o etime= to get precise elapsed time
                etime_cmd = ['ps', '-p', str(pid), '-o', 'etime=']
                etime_result = subprocess.run(etime_cmd, capture_output=True, text=True, timeout=2)

                if etime_result.returncode != 0:
                    continue

                etime_str = etime_result.stdout.strip()
                age_seconds = parse_elapsed_time(etime_str)

                if age_seconds >= min_age_seconds:
                    logger.info(f"Found stuck csm send process: PID {pid}, age {age_seconds}s ({etime_str})")
                    stuck_processes.append((pid, age_seconds))

            except (ValueError, IndexError) as e:
                logger.warning(f"Failed to parse ps line: {line}, error: {e}")
                continue

    except subprocess.TimeoutExpired:
        logger.error("ps command timed out")
    except Exception as e:
        logger.error(f"Error finding stuck csm send processes: {e}")

    return stuck_processes


def parse_elapsed_time(etime_str: str) -> float:
    """
    Parse ps elapsed time string to seconds.

    Format examples:
        "12:34" -> 12 minutes 34 seconds
        "1:23:45" -> 1 hour 23 minutes 45 seconds
        "2-03:45:56" -> 2 days 3 hours 45 minutes 56 seconds
        "10-04:12:34" -> 10 days 4 hours 12 minutes 34 seconds

    Returns:
        Total seconds as float
    """
    etime_str = etime_str.strip()

    # Handle days format: "DD-HH:MM:SS"
    if '-' in etime_str:
        days_part, time_part = etime_str.split('-', 1)
        days = int(days_part)
        etime_str = time_part
    else:
        days = 0

    # Split time parts
    parts = etime_str.split(':')

    if len(parts) == 2:
        # Format: "MM:SS"
        minutes, seconds = parts
        hours = 0
    elif len(parts) == 3:
        # Format: "HH:MM:SS"
        hours, minutes, seconds = parts
    else:
        logger.warning(f"Unexpected elapsed time format: {etime_str}")
        return 0.0

    try:
        total_seconds = (
            days * 86400 +
            int(hours) * 3600 +
            int(minutes) * 60 +
            int(seconds)
        )
        return float(total_seconds)
    except ValueError as e:
        logger.warning(f"Failed to parse elapsed time '{etime_str}': {e}")
        return 0.0


def find_child_processes(parent_pid: int) -> List[int]:
    """
    Find all child processes of a given parent PID.

    Args:
        parent_pid: Parent process ID

    Returns:
        List of child PIDs
    """
    child_pids = []

    try:
        # Use ps --ppid to find children
        cmd = ['ps', '--ppid', str(parent_pid), '-o', 'pid=']
        result = subprocess.run(cmd, capture_output=True, text=True, timeout=2)

        if result.returncode == 0:
            for line in result.stdout.splitlines():
                try:
                    child_pid = int(line.strip())
                    child_pids.append(child_pid)
                except ValueError:
                    continue

    except Exception as e:
        logger.warning(f"Failed to find child processes of {parent_pid}: {e}")

    return child_pids


def kill_process_tree(pid: int, signal_num: int = signal.SIGKILL) -> bool:
    """
    Kill a process and all its children.

    Args:
        pid: Process ID to kill
        signal_num: Signal to send (default: SIGKILL)

    Returns:
        True if successful, False otherwise
    """
    try:
        # Find and kill children first
        children = find_child_processes(pid)
        for child_pid in children:
            try:
                logger.info(f"Killing child process {child_pid}")
                os.kill(child_pid, signal_num)
            except ProcessLookupError:
                logger.debug(f"Child process {child_pid} already dead")
            except PermissionError:
                logger.error(f"Permission denied killing child process {child_pid}")
                return False
            except Exception as e:
                logger.warning(f"Failed to kill child process {child_pid}: {e}")

        # Kill parent
        try:
            logger.info(f"Killing parent process {pid}")
            os.kill(pid, signal_num)
            return True
        except ProcessLookupError:
            logger.debug(f"Process {pid} already dead")
            return True
        except PermissionError:
            logger.error(f"Permission denied killing process {pid}")
            return False

    except Exception as e:
        logger.error(f"Failed to kill process tree for {pid}: {e}")
        return False


def kill_stuck_csm_send(session_name: str, min_age_seconds: int = 600) -> Tuple[bool, float]:
    """
    Kill stuck csm send processes and their children blocking the session.

    This is the main recovery function that should be called before other
    recovery methods (like ESC or Ctrl+C).

    Args:
        session_name: Name of the tmux session
        min_age_seconds: Minimum age to consider a process stuck (default: 600s = 10min)

    Returns:
        (success, duration) tuple:
            success: True if stuck processes were found and killed, False otherwise
            duration: Time taken to complete the operation in seconds

    Example:
        >>> success, duration = kill_stuck_csm_send("autonomous-swarm-coordinator")
        >>> if success:
        ...     print(f"Killed stuck processes in {duration:.2f}s")
    """
    start_time = time.time()

    # Find stuck processes
    stuck_processes = find_stuck_csm_send_processes(session_name, min_age_seconds)

    if not stuck_processes:
        logger.debug(f"No stuck csm send processes found for session '{session_name}'")
        duration = time.time() - start_time
        return (False, duration)

    logger.warning(f"Found {len(stuck_processes)} stuck csm send processes for '{session_name}'")

    # Kill each stuck process and its children
    killed_count = 0
    for pid, age in stuck_processes:
        logger.info(f"Attempting to kill stuck csm send process {pid} (age: {age:.0f}s)")

        if kill_process_tree(pid):
            killed_count += 1
            logger.info(f"Successfully killed process {pid}")
        else:
            logger.error(f"Failed to kill process {pid}")

    # Wait a moment for processes to fully terminate
    time.sleep(2)

    # Verify processes are dead
    still_alive = []
    for pid, _ in stuck_processes:
        try:
            # Check if process still exists
            os.kill(pid, 0)  # Signal 0 just checks if process exists
            still_alive.append(pid)
            logger.warning(f"Process {pid} still alive after kill attempt")
        except ProcessLookupError:
            # Process is dead (expected)
            pass
        except Exception as e:
            logger.warning(f"Error checking if process {pid} is dead: {e}")

    duration = time.time() - start_time
    success = killed_count > 0 and len(still_alive) == 0

    if success:
        logger.info(f"Successfully killed {killed_count} stuck csm send processes in {duration:.2f}s")
    else:
        logger.warning(f"Killed {killed_count}/{len(stuck_processes)} processes, {len(still_alive)} still alive")

    return (success, duration)


def integrate_with_astrocyte_recovery(session_name: str) -> bool:
    """
    Integration point for Astrocyte's recovery chain.

    This function should be called FIRST in the recovery sequence, before
    attempting ESC or Ctrl+C recovery.

    Args:
        session_name: Name of the session to recover

    Returns:
        True if stuck processes were found and killed, False otherwise

    Usage in astrocyte recovery chain:
        def recover_stuck_session(session_name):
            # Step 1: Kill stuck csm send processes (if any)
            if kill_stuck_csm_send_recovery(session_name):
                logger.info("Killed stuck csm send processes")
                time.sleep(1)  # Let session clear

            # Step 2: Try ESC (gentle recovery)
            success = send_escape_key(session_name)
            if success:
                return True

            # Step 3: Try Ctrl+C (aggressive recovery)
            success = send_ctrl_c(session_name)
            return success
    """
    success, duration = kill_stuck_csm_send(session_name)

    if success:
        logger.info(f"Recovery: killed stuck csm send processes in {duration:.2f}s")

    return success


# Self-test functionality
def self_test():
    """
    Run self-tests to verify the module works correctly.
    """
    print("Running self-tests for stuck csm send recovery...")

    # Test 1: Parse elapsed time
    test_cases = [
        ("12:34", 754.0),           # 12 min 34 sec
        ("1:23:45", 5025.0),        # 1 hour 23 min 45 sec
        ("2-03:45:56", 186356.0),   # 2 days 3 hours 45 min 56 sec = 2*86400 + 3*3600 + 45*60 + 56
        ("10-04:12:34", 879154.0),  # 10 days 4 hours 12 min 34 sec = 10*86400 + 4*3600 + 12*60 + 34
    ]

    print("\nTest 1: Elapsed time parsing")
    for etime_str, expected in test_cases:
        result = parse_elapsed_time(etime_str)
        status = "PASS" if result == expected else "FAIL"
        print(f"  {status}: parse_elapsed_time('{etime_str}') = {result} (expected {expected})")

    # Test 2: Find stuck processes (should find none in normal operation)
    print("\nTest 2: Find stuck csm send processes")
    stuck = find_stuck_csm_send_processes("test-session-that-does-not-exist")
    print(f"  Found {len(stuck)} stuck processes (expected 0 for non-existent session)")

    # Test 3: Child process finding
    print("\nTest 3: Find child processes")
    # Use PID 1 (init) which usually has children
    children = find_child_processes(1)
    print(f"  Found {len(children)} children of PID 1")

    print("\nSelf-test complete!")


if __name__ == "__main__":
    # Set up logging for standalone execution
    logging.basicConfig(
        level=logging.INFO,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )

    import sys

    if len(sys.argv) > 1:
        if sys.argv[1] == "--test":
            self_test()
        elif sys.argv[1] == "--help":
            print(__doc__)
            print("\nUsage:")
            print("  python astrocyte_stuck_csm_send_recovery.py --test")
            print("  python astrocyte_stuck_csm_send_recovery.py --help")
            print("  python astrocyte_stuck_csm_send_recovery.py <session-name>")
        else:
            session_name = sys.argv[1]
            print(f"Checking for stuck csm send processes for session: {session_name}")
            success, duration = kill_stuck_csm_send(session_name)
            if success:
                print(f"SUCCESS: Killed stuck processes in {duration:.2f}s")
                sys.exit(0)
            else:
                print(f"No stuck processes found (checked in {duration:.2f}s)")
                sys.exit(1)
    else:
        print(__doc__)
        print("\nRun with --help for usage information")
