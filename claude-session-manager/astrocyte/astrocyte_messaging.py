"""
Astrocyte Message Attribution and Logging Module

This module provides centralized message sending for the Astrocyte daemon,
ensuring all messages are tagged with source attribution and logged for
audit trail purposes.

All Astrocyte messages to Claude sessions should route through this module
to enforce architectural invariants (tagging + logging).
"""

import hashlib
import logging
import os
import subprocess
import sys
import tempfile
from datetime import datetime, timezone
from logging.handlers import RotatingFileHandler
from typing import Optional


# Module-level logger (lazy initialization)
_logger: Optional[logging.Logger] = None


def send_tagged_message(
    session_name: str,
    message: str,
    message_type: str
) -> None:
    """
    Send tagged message to Claude session.

    Single entry point for all Astrocyte message sending.
    Enforces tagging, logging, and validation.

    Args:
        session_name: Target Claude session name
        message: Message content (will be wrapped in <system-reminder>)
        message_type: One of ["violation_prompt", "diagnosis", "notification"]

    Raises:
        ValueError: If validation fails (empty message, invalid type, etc.)
        subprocess.CalledProcessError: If csm send command fails

    Example:
        send_tagged_message(
            session_name="my-session",
            message="Permission denied: Cannot write to /etc/passwd",
            message_type="violation_prompt"
        )
    """
    # 1. Format message with attribution tags
    tagged_message = _format_tagged_message(message, message_type, session_name)

    # 2. Validate inputs (fail-fast)
    _validate_message(session_name, tagged_message, message_type)

    # 3. Log message (fail-safe)
    _log_message(session_name, message_type, tagged_message)

    # 4. Send via csm command
    _send_via_csm(session_name, tagged_message)


def _format_tagged_message(
    message: str,
    message_type: str,
    session_name: str
) -> str:
    """
    Wrap message in <system-reminder> block with metadata.

    Args:
        message: Raw message content
        message_type: One of [violation_prompt, diagnosis, notification]
        session_name: Target session name

    Returns:
        Tagged message string with <system-reminder> wrapper

    Example:
        >>> _format_tagged_message(
        ...     "Permission denied",
        ...     "violation_prompt",
        ...     "my-session"
        ... )
        '<system-reminder>\\n**This message is from Astrocyte Daemon** ...\\n</system-reminder>'
    """
    timestamp = datetime.now(timezone.utc).isoformat()

    return f"""<system-reminder>
**This message is from Astrocyte Daemon** (automated monitoring system)

{message}

---
Source: astrocyte-daemon
Type: {message_type}
Session: {session_name}
Timestamp: {timestamp}
</system-reminder>"""


def _validate_message(
    session_name: str,
    tagged_message: str,
    message_type: str
) -> None:
    """
    Validate message inputs (fail-fast).

    Checks:
    1. Message not empty
    2. Source tag present (security: prevents untagged bypass)
    3. Message type valid enum
    4. Session name not empty

    Args:
        session_name: Session identifier
        tagged_message: Message with <system-reminder> wrapper
        message_type: Expected type from enum

    Raises:
        ValueError: With actionable error message if validation fails

    Example:
        >>> _validate_message("", "msg", "violation_prompt")
        ValueError: Session name cannot be empty
    """
    if not tagged_message or not tagged_message.strip():
        raise ValueError("Message cannot be empty")

    # Security: Ensure tag is present (prevents bypass)
    if "Source: astrocyte-daemon" not in tagged_message:
        raise ValueError("Message missing attribution tag")

    # Type safety: Enum validation
    valid_types = ["violation_prompt", "diagnosis", "notification"]
    if message_type not in valid_types:
        raise ValueError(
            f"Invalid message type: {message_type}. "
            f"Must be one of: {', '.join(valid_types)}"
        )

    if not session_name or not session_name.strip():
        raise ValueError("Session name cannot be empty")


def _log_message(
    session_name: str,
    message_type: str,
    message: str
) -> None:
    """
    Log message send event (fail-safe).

    Logs to:
    - File: ~/.agm/astrocyte/logs/messages.log
    - Stdout: For real-time monitoring

    On error: Warns to stderr, never raises (message delivery > audit trail)

    Args:
        session_name: Session identifier
        message_type: Message type (for filtering)
        message: Full tagged message (for hash)

    Example:
        >>> _log_message("my-session", "violation_prompt", "...")
        # Output: 2026-02-04 20:00:00 UTC - astrocyte.messaging - INFO - SEND session=my-session ...
    """
    try:
        logger = _get_message_logger()

        # Calculate content hash (first 8 chars of SHA-256)
        message_hash = hashlib.sha256(message.encode()).hexdigest()[:8]

        logger.info(
            f"SEND session={session_name} "
            f"type={message_type} "
            f"length={len(message)} "
            f"hash={message_hash}"
        )
    except (IOError, OSError) as e:
        # Fail-safe: Warn but don't block send
        sys.stderr.write(f"Warning: Failed to log message: {e}\n")


def _send_via_csm(session_name: str, tagged_message: str) -> None:
    """
    Send message via csm send command.

    Strategy:
    - Small messages (<10KB): Use --prompt flag
    - Large messages (≥10KB): Use --prompt-file with temp file

    Args:
        session_name: Target session
        tagged_message: Full message with <system-reminder>

    Raises:
        subprocess.CalledProcessError: If csm send fails

    Example:
        >>> _send_via_csm("my-session", "<system-reminder>...</system-reminder>")
        # Executes: csm send my-session --prompt "..."
    """
    logger = _get_message_logger()

    # Threshold: 10KB (agm send msg supports larger via --prompt-file)
    if len(tagged_message) < 10_000:
        # Small message: Use --prompt flag
        cmd = ["agm", "send", "msg", session_name, "--prompt", tagged_message]
        logger.debug(f"Executing command: agm send msg {session_name} --prompt [message_len={len(tagged_message)}]")

        import time
        import os
        start_time = time.time()
        try:
            env = os.environ.copy()
            env["AGM_SENDER"] = "astrocyte-daemon"
            result = subprocess.run(
                cmd,
                check=True,
                capture_output=True,
                text=True,
                env=env
            )
            duration = time.time() - start_time
            logger.debug(f"Command succeeded in {duration:.2f}s")
        except subprocess.CalledProcessError as e:
            duration = time.time() - start_time
            logger.error(f"Command failed after {duration:.2f}s: agm send msg {session_name}")
            logger.error(f"Exit code: {e.returncode}")
            logger.error(f"stdout: {e.stdout}")
            logger.error(f"stderr: {e.stderr}")
            raise
    else:
        # Large message: Use temp file
        with tempfile.NamedTemporaryFile(
            mode='w',
            suffix='.txt',
            delete=False
        ) as f:
            f.write(tagged_message)
            temp_path = f.name

        try:
            cmd = ["agm", "send", "msg", session_name, "--prompt-file", temp_path]
            logger.debug(f"Executing command: agm send msg {session_name} --prompt-file {temp_path} [message_len={len(tagged_message)}]")

            import time
            import os
            start_time = time.time()
            try:
                env = os.environ.copy()
                env["AGM_SENDER"] = "astrocyte-daemon"
                result = subprocess.run(
                    cmd,
                    check=True,
                    capture_output=True,
                    text=True,
                    env=env
                )
                duration = time.time() - start_time
                logger.debug(f"Command succeeded in {duration:.2f}s")
            except subprocess.CalledProcessError as e:
                duration = time.time() - start_time
                logger.error(f"Command failed after {duration:.2f}s: agm send msg {session_name} --prompt-file")
                logger.error(f"Exit code: {e.returncode}")
                logger.error(f"stdout: {e.stdout}")
                logger.error(f"stderr: {e.stderr}")
                raise
        finally:
            os.unlink(temp_path)  # Cleanup


def _get_message_logger() -> logging.Logger:
    """
    Get or create logger for astrocyte.messaging (idempotent).

    Returns:
        Configured logger instance

    Example:
        >>> logger = _get_message_logger()
        >>> logger.info("Test message")
    """
    global _logger
    if _logger is None:
        _logger = _setup_message_logger()
    return _logger


def _setup_message_logger() -> logging.Logger:
    """
    Setup logger with file + stdout handlers (fail-safe).

    File handler:
    - Location: ~/.agm/astrocyte/logs/messages.log
    - Rotation: 10MB, 5 backups
    - Permissions: 0600 (owner read/write only)
    - Fail-safe: If file setup fails, warn and use stdout only

    Stdout handler:
    - Always added (fallback if file fails)
    - Level: INFO

    Returns:
        Configured logger instance

    Example:
        >>> logger = _setup_message_logger()
        >>> logger.info("Test message")
        # Logs to file + stdout
    """
    logger = logging.getLogger("astrocyte.messaging")
    logger.setLevel(logging.INFO)
    logger.propagate = False  # Don't propagate to root logger

    # File handler (fail-safe)
    try:
        log_dir = os.path.expanduser("~/.agm/astrocyte/logs")
        os.makedirs(log_dir, mode=0o700, exist_ok=True)

        log_file = os.path.join(log_dir, "messages.log")

        file_handler = RotatingFileHandler(
            log_file,
            maxBytes=10 * 1024 * 1024,  # 10MB
            backupCount=5,
            mode='a'
        )
        file_handler.setLevel(logging.INFO)
        file_handler.setFormatter(
            logging.Formatter(
                '%(asctime)s UTC - %(name)s - %(levelname)s - %(message)s',
                datefmt='%Y-%m-%d %H:%M:%S'
            )
        )
        logger.addHandler(file_handler)

        # Set file permissions (0600)
        os.chmod(log_file, 0o600)

    except (IOError, OSError) as e:
        # Warn but continue (stdout-only logging)
        sys.stderr.write(
            f"Warning: Could not setup file logging: {e}\n"
            f"Falling back to stdout-only logging\n"
        )

    # Stdout handler (always add)
    stdout_handler = logging.StreamHandler(sys.stdout)
    stdout_handler.setLevel(logging.INFO)
    stdout_handler.setFormatter(
        logging.Formatter(
            '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
            datefmt='%Y-%m-%d %H:%M:%S'
        )
    )
    logger.addHandler(stdout_handler)

    return logger
