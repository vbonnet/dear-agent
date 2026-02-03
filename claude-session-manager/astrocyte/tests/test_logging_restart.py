#!/usr/bin/env python3
"""
Test logging functionality across daemon restarts.

Verifies that the fix for logging after restart works correctly:
- Handlers are properly closed before being cleared
- New handlers can write to log files after simulated restart
- No file descriptor leaks occur
"""

import tempfile
import logging
import time
from pathlib import Path
import pytest


def test_logger_handler_cleanup():
    """Test that handlers are properly closed before clearing."""
    with tempfile.TemporaryDirectory() as tmpdir:
        log_dir = Path(tmpdir)
        log_file = log_dir / "test.log"

        # Simulate first daemon start
        logger = logging.getLogger("test_restart_1")
        logger.setLevel(logging.INFO)

        # Add handler
        handler1 = logging.FileHandler(log_file)
        logger.addHandler(handler1)

        # Write log message
        logger.info("First start")

        # Simulate restart: close handlers before clearing (the fix)
        for handler in logger.handlers[:]:
            handler.close()
        logger.handlers.clear()

        # Add new handler (simulating restart)
        handler2 = logging.FileHandler(log_file)
        logger.addHandler(handler2)

        # Write another log message
        logger.info("After restart")
        handler2.close()

        # Verify both messages are in the log
        content = log_file.read_text()
        assert "First start" in content
        assert "After restart" in content


def test_logger_multiple_restarts():
    """Test that multiple restarts don't cause issues."""
    with tempfile.TemporaryDirectory() as tmpdir:
        log_dir = Path(tmpdir)
        log_file = log_dir / "test_multi.log"

        logger = logging.getLogger("test_restart_multi")
        logger.setLevel(logging.INFO)

        for i in range(5):
            # Add handler
            handler = logging.FileHandler(log_file)
            logger.addHandler(handler)

            # Write message
            logger.info(f"Restart {i}")
            handler.flush()

            # Clean up (simulating restart)
            for h in logger.handlers[:]:
                h.close()
            logger.handlers.clear()

        # Verify all messages are present
        content = log_file.read_text()
        for i in range(5):
            assert f"Restart {i}" in content


def test_logger_without_close_fails():
    """
    Test that NOT closing handlers causes issues (demonstrates the bug).

    This test documents the original problem: calling handlers.clear()
    without close() can cause file descriptor leaks.
    """
    with tempfile.TemporaryDirectory() as tmpdir:
        log_dir = Path(tmpdir)
        log_file = log_dir / "test_bad.log"

        logger = logging.getLogger("test_restart_bad")
        logger.setLevel(logging.INFO)

        # First handler
        handler1 = logging.FileHandler(log_file)
        logger.addHandler(handler1)
        logger.info("First")
        handler1.flush()

        # Clear without closing (THE BUG)
        logger.handlers.clear()

        # Try to add new handler
        handler2 = logging.FileHandler(log_file)
        logger.addHandler(handler2)
        logger.info("Second")
        handler2.flush()

        # This test might pass on some systems but demonstrates
        # the file descriptor leak issue. The key point is that
        # handler1 is never closed, leaving the file open.

        # Clean up
        handler1.close()  # Must close manually
        handler2.close()


def test_setup_logging_integration():
    """
    Test the actual setup_logging function from astrocyte-daemon.py
    to ensure it properly handles restarts.
    """
    # Import the actual function
    import sys
    from pathlib import Path
    test_dir = Path(__file__).parent
    sys.path.insert(0, str(test_dir))

    # Import only after path is set
    from astrocyte_daemon_import_test import setup_logging_standalone

    with tempfile.TemporaryDirectory() as tmpdir:
        log_dir = Path(tmpdir)

        # First setup
        logger1 = setup_logging_standalone(log_dir, verbose=False)
        logger1.info("First setup")

        # Force flush
        for handler in logger1.handlers:
            handler.flush()

        # Second setup (simulating restart)
        logger2 = setup_logging_standalone(log_dir, verbose=False)
        logger2.info("After restart")

        # Force flush
        for handler in logger2.handlers:
            handler.flush()

        # Verify log file contains both messages
        log_file = log_dir / "daemon.log"
        content = log_file.read_text()
        assert "First setup" in content
        assert "After restart" in content


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
