#!/usr/bin/env python3
"""
Astrocyte Daemon - Production deployment without test code.

This is the clean production entry point. The main astrocyte.py includes
test code for validation. This script skips tests and goes straight to monitoring.
"""

import sys
import time
import subprocess
import re
import json
import os
import requests
import logging
from logging.handlers import RotatingFileHandler
from datetime import datetime, timedelta
from dataclasses import dataclass, asdict
from pathlib import Path

# Import all core functions from astrocyte.py
sys.path.insert(0, str(Path(__file__).parent))
from astrocyte import (
    get_active_csm_sessions,
    capture_pane_state,
    is_stuck_mustering,
    is_stuck_zero_token_waiting,
    is_stuck_cursor_frozen,
    is_asking_question_without_tool,
    is_stuck_permission_prompt,
    recover_session,
    send_violation_prompt,
    reject_permission_prompt,
    log_incident,
    get_session_id,
    send_slack_notification,
    send_email_notification,
    generate_diagnosis_prompt,
    send_diagnosis_prompt_via_csm,
    load_config,
    Incident
)


def setup_logging(log_dir: Path, verbose: bool = False):
    """
    Configure logging to both file and console.

    Args:
        log_dir: Directory for log files (e.g., ~/.csm/astrocyte/logs)
        verbose: If True, set DEBUG level; otherwise INFO
    """
    log_dir.mkdir(parents=True, exist_ok=True)
    log_file = log_dir / "daemon.log"

    # Create logger
    logger = logging.getLogger("astrocyte")
    logger.setLevel(logging.DEBUG if verbose else logging.INFO)

    # Remove existing handlers to avoid duplicates
    logger.handlers.clear()

    # File handler with rotation (10MB max, keep 5 backups)
    file_handler = RotatingFileHandler(
        log_file,
        maxBytes=10 * 1024 * 1024,  # 10MB
        backupCount=5
    )
    file_handler.setLevel(logging.DEBUG)
    file_formatter = logging.Formatter(
        '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )
    file_handler.setFormatter(file_formatter)
    logger.addHandler(file_handler)

    # Console handler (only INFO and above to avoid clutter)
    console_handler = logging.StreamHandler(sys.stdout)
    console_handler.setLevel(logging.INFO)
    console_formatter = logging.Formatter('%(message)s')
    console_handler.setFormatter(console_formatter)
    logger.addHandler(console_handler)

    return logger


def main():
    """Production monitoring loop - no test code."""
    # Create necessary directories
    base_dir = Path.home() / ".csm/astrocyte"
    base_dir.mkdir(parents=True, exist_ok=True)

    log_dir = base_dir / "logs"
    log_dir.mkdir(parents=True, exist_ok=True)

    diagnoses_dir = base_dir / "diagnoses"
    diagnoses_dir.mkdir(parents=True, exist_ok=True)

    # Load configuration first (to get verbose setting)
    config = load_config()

    # Set up logging
    logger = setup_logging(log_dir, verbose=config.verbose)

    # Startup banner
    print("🧠 Astrocyte daemon starting...")
    print(f"   Timestamp: {datetime.now().isoformat()}")
    print(f"   Mode: Production Deployment - Continuous Monitoring")
    print(f"   Incidents: ~/.csm/astrocyte/incidents.jsonl")
    print(f"   Diagnoses: ~/.csm/astrocyte/diagnoses/")
    print(f"   Debug logs: ~/.csm/astrocyte/logs/daemon.log")

    logger.info("="*60)
    logger.info("Astrocyte daemon starting")
    logger.info(f"Timestamp: {datetime.now().isoformat()}")
    logger.info(f"PID: {os.getpid()}")
    logger.info(f"Python: {sys.version}")
    logger.info("="*60)

    print(f"\n✅ Astrocyte initialized and ready")

    # Log configuration
    print(f"\n⚙️  Configuration loaded:")

    print(f"   Interval: {config.interval_seconds}s ({config.interval_seconds // 60} min)")
    print(f"   Thresholds:")
    print(f"     - Mustering timeout: {config.mustering_timeout} min")
    print(f"     - Zero-token waiting: {config.zero_token_waiting} min")
    print(f"     - Cursor frozen: {config.cursor_frozen} min")
    print(f"     - AskUserQuestion violation: {config.ask_question_violation} min")
    print(f"   Slack notifications: {'enabled' if config.slack_enabled else 'disabled'}")
    print(f"   Session overrides: {len(config.session_overrides)} sessions")

    logger.info(f"Configuration: interval={config.interval_seconds}s, " +
                f"mustering={config.mustering_timeout}min, " +
                f"zero_token={config.zero_token_waiting}min, " +
                f"cursor_frozen={config.cursor_frozen}min, " +
                f"ask_question={config.ask_question_violation}min")
    logger.info(f"Slack: {'enabled' if config.slack_enabled else 'disabled'}")
    logger.debug(f"Session overrides: {config.session_overrides}")

    # State tracking
    previous_states = {}
    check_count = 0

    print(f"\n🔄 Starting continuous monitoring loop...")
    print(f"   Press Ctrl-C to stop")

    logger.info("Starting continuous monitoring loop")

    try:
        while True:
            check_count += 1
            print(f"\n{'='*60}")
            print(f"Check cycle #{check_count} at {datetime.now().strftime('%H:%M:%S')}")
            print(f"{'='*60}")

            logger.debug(f"Check cycle #{check_count} starting")

            sessions = get_active_csm_sessions()
            print(f"Active CSM sessions: {len(sessions)}")

            logger.info(f"Active CSM sessions: {len(sessions)} - {sessions if sessions else '(none)'}")

            for session in sessions:
                try:
                    current = capture_pane_state(session)
                    previous = previous_states.get(session)

                    # Check all detection heuristics
                    stuck = False
                    symptom = None
                    heuristic = None

                    # Check permission prompts FIRST (works without previous state)
                    # Fresh start detection: if we see a permission prompt with violation patterns,
                    # we can detect and reject immediately without waiting for second cycle
                    permission_threshold = config.get_threshold(session, "permission_prompt_duration")
                    if is_stuck_permission_prompt(current, previous, permission_threshold):
                        stuck = True
                        symptom = "permission_prompt"
                        heuristic = "permission_prompt_detected"

                    # Other detection heuristics require previous state for comparison
                    if not stuck and previous:
                        # Get session-specific thresholds (with overrides)
                        mustering_threshold = config.get_threshold(session, "mustering_timeout")
                        zero_token_threshold = config.get_threshold(session, "zero_token_waiting")
                        cursor_frozen_threshold = config.get_threshold(session, "cursor_frozen")
                        ask_question_threshold = config.get_threshold(session, "ask_question_violation")

                        if is_stuck_mustering(current, previous, mustering_threshold):
                            stuck = True
                            symptom = "stuck_mustering"
                            heuristic = "mustering_timeout"
                        elif is_stuck_zero_token_waiting(current, previous, zero_token_threshold):
                            stuck = True
                            symptom = "stuck_zero_token_waiting"
                            heuristic = "zero_token_galloping"
                        elif is_stuck_cursor_frozen(current, previous, cursor_frozen_threshold):
                            stuck = True
                            symptom = "stuck_cursor_frozen"
                            heuristic = "cursor_frozen"
                        elif is_asking_question_without_tool(current, previous, ask_question_threshold):
                            stuck = True
                            symptom = "ask_question_violation"
                            heuristic = "ask_question_pattern"

                    if stuck:
                        # Calculate duration (0 for fresh start detection)
                        if previous:
                            delta_minutes = int((current.timestamp - previous.timestamp).seconds / 60)
                        else:
                            delta_minutes = 0  # Fresh start detection, no duration yet

                        print(f"\n⚠️  STUCK DETECTED: {session}")
                        print(f"   Symptom: {symptom}")
                        print(f"   Heuristic: {heuristic}")
                        print(f"   Duration: {delta_minutes} minutes")

                        logger.warning(f"STUCK DETECTED: session={session}, symptom={symptom}, " +
                                     f"heuristic={heuristic}, duration={delta_minutes}min")
                        logger.debug(f"Pane snapshot (first 200 chars): {current.pane_content[:200]}")
                        logger.debug(f"Cursor position: {current.cursor_position}")

                        # Determine recovery method based on symptom
                        if symptom == "ask_question_violation":
                            recovery_method = "violation_prompt"
                        elif symptom == "permission_prompt":
                            recovery_method = "reject_permission"
                        else:
                            recovery_method = "escape"

                        # Create incident record
                        incident = Incident(
                            timestamp=datetime.now().isoformat(),
                            session_name=session,
                            session_id=get_session_id(session),
                            symptom=symptom,
                            duration_minutes=delta_minutes,
                            detection_heuristic=heuristic,
                            pane_snapshot=current.pane_content[:500],
                            cursor_position=f"{current.cursor_position[0]},{current.cursor_position[1]}",
                            recovery_attempted=True,
                            recovery_method=recovery_method,
                            recovery_success=None,
                            recovery_duration_seconds=None,
                            diagnosis_filed=False,
                            diagnosis_file=None
                        )

                        # Log detection
                        log_incident(incident)
                        print(f"   📝 Incident logged (detection)")

                        # Attempt recovery
                        if symptom == "ask_question_violation":
                            print(f"   🔧 Sending AskUserQuestion violation prompt...")
                            logger.info(f"Sending violation prompt to session={session}")
                            recovery = send_violation_prompt(session)
                        elif symptom == "permission_prompt":
                            print(f"   🔧 Rejecting permission prompt with tool usage violation...")
                            logger.info(f"Rejecting permission prompt for session={session}")
                            recovery = reject_permission_prompt(session)
                        else:
                            print(f"   🔧 Attempting recovery with {config.recovery_method} strategy...")
                            logger.info(f"Attempting recovery with {config.recovery_method} for session={session}")
                            recovery = recover_session(session, config)

                        # Update incident with recovery results
                        incident.recovery_success = recovery.success
                        incident.recovery_duration_seconds = recovery.duration_seconds

                        # Log recovery result
                        log_incident(incident)
                        print(f"   📝 Incident logged (recovery)")
                        logger.info(f"Recovery result: success={recovery.success}, " +
                                  f"duration={recovery.duration_seconds:.1f}s")

                        # Send Slack notification
                        send_slack_notification(incident, recovery)
                        print(f"   📬 Slack notification sent (if configured)")

                        # Send email notification
                        send_email_notification(incident, recovery)
                        print(f"   📧 Email notification sent (if configured)")

                        # Send diagnosis prompt (if recovery successful and not a violation prompt)
                        # Skip diagnosis for AskUserQuestion violations (violation prompt handles it)
                        # Skip diagnosis for permission prompts (rejection is automated, no agent diagnosis needed)
                        if recovery.success and symptom not in ["ask_question_violation", "permission_prompt"]:
                            diagnosis_prompt = generate_diagnosis_prompt(incident, recovery)
                            diagnosis_sent = send_diagnosis_prompt_via_csm(session, diagnosis_prompt)
                            if diagnosis_sent:
                                print(f"   📋 Diagnosis prompt sent to session")
                                logger.info(f"Diagnosis prompt sent successfully to {session}")
                            else:
                                print(f"   ⚠️  Diagnosis prompt failed to send")
                                logger.warning(f"Failed to send diagnosis prompt to {session}")
                        elif symptom == "ask_question_violation":
                            print(f"   📋 Violation prompt sent (no diagnosis needed)")
                            logger.info(f"Violation prompt sent to {session}, skipping diagnosis")
                        elif symptom == "permission_prompt":
                            print(f"   📋 Permission rejected (no diagnosis needed)")
                            logger.info(f"Permission prompt rejected for {session}, skipping diagnosis")

                        if recovery.success:
                            print(f"   ✅ Recovery successful ({recovery.duration_seconds:.1f}s)")
                            print(f"   📊 Pane content changed: YES")
                        else:
                            print(f"   ❌ Recovery failed (no pane change)")
                            print(f"   ⚠️  Manual intervention required")
                            logger.error(f"Recovery FAILED for {session} - manual intervention needed")

                    # Store current state for next check
                    previous_states[session] = current

                except Exception as e:
                    print(f"   ❌ Error processing {session}: {e}")
                    logger.error(f"Error processing session {session}: {e}", exc_info=True)

            logger.debug(f"Check cycle #{check_count} complete, sleeping for {config.interval_seconds}s")
            print(f"\n⏸️  Sleeping for {config.interval_seconds} seconds...")
            time.sleep(config.interval_seconds)

    except KeyboardInterrupt:
        print(f"\n\n⚠️  Interrupted by user")
        print(f"   Total checks: {check_count}")
        print(f"   Sessions monitored: {len(previous_states)}")
        print("\n👋 Astrocyte shutting down")

        logger.info("="*60)
        logger.info("Daemon shutdown requested (KeyboardInterrupt)")
        logger.info(f"Total check cycles: {check_count}")
        logger.info(f"Sessions monitored: {len(previous_states)}")
        logger.info("="*60)


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        sys.exit(0)
    except Exception as e:
        # Try to log the fatal error if logger is available
        try:
            logger = logging.getLogger("astrocyte")
            if logger.hasHandlers():
                logger.critical(f"FATAL ERROR: {e}", exc_info=True)
        except:
            pass  # Logger not set up yet, just print

        print(f"Fatal error: {e}", file=sys.stderr)
        import traceback
        traceback.print_exc()
        sys.exit(1)
