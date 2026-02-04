"""
Unit tests for astrocyte_messaging module.

Tests cover:
- Message formatting (_format_tagged_message)
- Validation (_validate_message)
- Logging (_log_message, _setup_message_logger)
- Sending (_send_via_csm)
- Integration (send_tagged_message end-to-end)

Target: 80%+ code coverage
"""

import hashlib
import logging
import os
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime, timezone
from io import StringIO
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch, call

# Import module under test
import astrocyte_messaging


class TestFormatting(unittest.TestCase):
    """Test message formatting with <system-reminder> tags."""

    def test_format_includes_system_reminder_tags(self):
        """Message should be wrapped in <system-reminder> block."""
        result = astrocyte_messaging._format_tagged_message(
            "Test message",
            "diagnosis",
            "test-session"
        )

        self.assertIn("<system-reminder>", result)
        self.assertIn("</system-reminder>", result)

    def test_format_includes_source_attribution(self):
        """Message should include source attribution."""
        result = astrocyte_messaging._format_tagged_message(
            "Test message",
            "diagnosis",
            "test-session"
        )

        self.assertIn("Source: astrocyte-daemon", result)

    def test_format_includes_metadata_fields(self):
        """Message should include Type, Session, and Timestamp metadata."""
        result = astrocyte_messaging._format_tagged_message(
            "Test message",
            "diagnosis",
            "test-session"
        )

        self.assertIn("Type: diagnosis", result)
        self.assertIn("Session: test-session", result)
        self.assertIn("Timestamp:", result)

    def test_format_preserves_message_content(self):
        """Original message content should be preserved."""
        message = "Multi-line\nmessage\nwith special chars: <>&\""
        result = astrocyte_messaging._format_tagged_message(
            message,
            "diagnosis",
            "test-session"
        )

        self.assertIn(message, result)

    def test_format_includes_iso8601_timestamp(self):
        """Timestamp should be in ISO8601 format with timezone."""
        result = astrocyte_messaging._format_tagged_message(
            "Test",
            "diagnosis",
            "test-session"
        )

        # Extract timestamp line
        timestamp_line = [line for line in result.split("\n") if "Timestamp:" in line][0]
        timestamp_str = timestamp_line.split("Timestamp:")[1].strip()

        # Should be parseable as ISO8601
        parsed = datetime.fromisoformat(timestamp_str)
        self.assertIsNotNone(parsed.tzinfo)  # Has timezone


class TestValidation(unittest.TestCase):
    """Test input validation (fail-fast)."""

    def test_empty_message_raises_valueerror(self):
        """Empty message should raise ValueError."""
        with self.assertRaises(ValueError) as cm:
            astrocyte_messaging._validate_message("session", "", "diagnosis")

        self.assertIn("Message cannot be empty", str(cm.exception))

    def test_whitespace_only_message_raises_valueerror(self):
        """Whitespace-only message should raise ValueError."""
        with self.assertRaises(ValueError) as cm:
            astrocyte_messaging._validate_message("session", "   \n  ", "diagnosis")

        self.assertIn("Message cannot be empty", str(cm.exception))

    def test_missing_source_tag_raises_valueerror(self):
        """Message without source tag should raise ValueError."""
        with self.assertRaises(ValueError) as cm:
            astrocyte_messaging._validate_message(
                "session",
                "<system-reminder>No source tag</system-reminder>",
                "diagnosis"
            )

        self.assertIn("Message missing attribution tag", str(cm.exception))

    def test_invalid_message_type_raises_valueerror(self):
        """Invalid message type should raise ValueError with valid types."""
        with self.assertRaises(ValueError) as cm:
            astrocyte_messaging._validate_message(
                "session",
                "<system-reminder>Source: astrocyte-daemon</system-reminder>",
                "invalid_type"
            )

        error_msg = str(cm.exception)
        self.assertIn("Invalid message type: invalid_type", error_msg)
        self.assertIn("violation_prompt", error_msg)
        self.assertIn("diagnosis", error_msg)
        self.assertIn("notification", error_msg)

    def test_empty_session_name_raises_valueerror(self):
        """Empty session name should raise ValueError."""
        with self.assertRaises(ValueError) as cm:
            astrocyte_messaging._validate_message(
                "",
                "<system-reminder>Source: astrocyte-daemon</system-reminder>",
                "diagnosis"
            )

        self.assertIn("Session name cannot be empty", str(cm.exception))

    def test_valid_message_passes(self):
        """Valid message should not raise exception."""
        try:
            astrocyte_messaging._validate_message(
                "test-session",
                "<system-reminder>Source: astrocyte-daemon\nTest</system-reminder>",
                "diagnosis"
            )
        except ValueError:
            self.fail("Valid message should not raise ValueError")


class TestLogging(unittest.TestCase):
    """Test logging functionality (fail-safe)."""

    def setUp(self):
        """Reset module-level logger before each test."""
        astrocyte_messaging._logger = None

    @patch('astrocyte_messaging.os.makedirs')
    @patch('astrocyte_messaging.RotatingFileHandler')
    @patch('astrocyte_messaging.logging.StreamHandler')
    def test_logger_setup_creates_file_and_stdout_handlers(self, mock_stream, mock_file, mock_makedirs):
        """Logger should have both file and stdout handlers."""
        logger = astrocyte_messaging._setup_message_logger()

        self.assertEqual(logger.name, "astrocyte.messaging")
        self.assertEqual(logger.level, logging.INFO)
        self.assertFalse(logger.propagate)

        # File handler created
        mock_makedirs.assert_called_once()
        mock_file.assert_called_once()

        # Stdout handler created
        mock_stream.assert_called_once()

    @patch('astrocyte_messaging.os.chmod')
    @patch('astrocyte_messaging.os.makedirs')
    @patch('astrocyte_messaging.RotatingFileHandler')
    def test_logger_sets_file_permissions_0600(self, mock_file_handler, mock_makedirs, mock_chmod):
        """Log file should have 0600 permissions (owner read/write only)."""
        astrocyte_messaging._setup_message_logger()

        # Should set 0600 permissions on log file
        mock_chmod.assert_called()
        chmod_call = mock_chmod.call_args
        self.assertEqual(chmod_call[0][1], 0o600)

    @patch('astrocyte_messaging.os.makedirs', side_effect=OSError("Permission denied"))
    @patch('astrocyte_messaging.sys.stderr', new_callable=StringIO)
    def test_logger_setup_fails_gracefully_on_file_error(self, mock_stderr, mock_makedirs):
        """Logger setup should warn but continue on file creation error."""
        logger = astrocyte_messaging._setup_message_logger()

        # Should still create logger
        self.assertIsNotNone(logger)

        # Should warn to stderr
        stderr_output = mock_stderr.getvalue()
        self.assertIn("Warning: Could not setup file logging", stderr_output)
        self.assertIn("stdout-only logging", stderr_output)

    @patch('astrocyte_messaging._get_message_logger')
    def test_log_message_includes_session_type_length_hash(self, mock_get_logger):
        """Log entry should include session, type, length, and hash."""
        mock_logger = Mock()
        mock_get_logger.return_value = mock_logger

        message = "Test message content"
        astrocyte_messaging._log_message("test-session", "diagnosis", message)

        # Check logger.info was called
        mock_logger.info.assert_called_once()
        log_call = mock_logger.info.call_args[0][0]

        self.assertIn("session=test-session", log_call)
        self.assertIn("type=diagnosis", log_call)
        self.assertIn(f"length={len(message)}", log_call)
        self.assertIn("hash=", log_call)

    @patch('astrocyte_messaging._get_message_logger')
    def test_log_message_hash_is_first_8_chars_of_sha256(self, mock_get_logger):
        """Hash should be first 8 characters of SHA-256."""
        mock_logger = Mock()
        mock_get_logger.return_value = mock_logger

        message = "Test message"
        expected_hash = hashlib.sha256(message.encode()).hexdigest()[:8]

        astrocyte_messaging._log_message("test-session", "diagnosis", message)

        log_call = mock_logger.info.call_args[0][0]
        self.assertIn(f"hash={expected_hash}", log_call)

    @patch('astrocyte_messaging._get_message_logger', side_effect=OSError("Log write failed"))
    @patch('astrocyte_messaging.sys.stderr', new_callable=StringIO)
    def test_log_message_fails_safely_on_error(self, mock_stderr, mock_get_logger):
        """Log failure should warn to stderr but not raise."""
        try:
            astrocyte_messaging._log_message("test-session", "diagnosis", "message")
        except Exception:
            self.fail("Log failure should not raise exception")

        # Should warn to stderr
        stderr_output = mock_stderr.getvalue()
        self.assertIn("Warning: Failed to log message", stderr_output)


class TestSending(unittest.TestCase):
    """Test message sending via csm command."""

    @patch('astrocyte_messaging.subprocess.run')
    def test_send_small_message_uses_prompt_flag(self, mock_run):
        """Messages <10KB should use --prompt flag."""
        mock_run.return_value = Mock(returncode=0)

        small_message = "Short message"
        astrocyte_messaging._send_via_csm("test-session", small_message)

        # Check subprocess.run called with --prompt
        mock_run.assert_called_once()
        call_args = mock_run.call_args[0][0]

        self.assertEqual(call_args[0], "csm")
        self.assertEqual(call_args[1], "send")
        self.assertEqual(call_args[2], "test-session")
        self.assertEqual(call_args[3], "--prompt")
        self.assertEqual(call_args[4], small_message)

    @patch('astrocyte_messaging.subprocess.run')
    @patch('astrocyte_messaging.tempfile.NamedTemporaryFile')
    @patch('astrocyte_messaging.os.unlink')
    def test_send_large_message_uses_prompt_file(self, mock_unlink, mock_tempfile, mock_run):
        """Messages ≥10KB should use --prompt-file with temp file."""
        mock_run.return_value = Mock(returncode=0)

        # Create mock temp file
        mock_file = Mock()
        mock_file.name = "/tmp/test-prompt.txt"
        mock_file.__enter__ = Mock(return_value=mock_file)
        mock_file.__exit__ = Mock(return_value=False)
        mock_tempfile.return_value = mock_file

        large_message = "X" * 10_000  # Exactly 10KB
        astrocyte_messaging._send_via_csm("test-session", large_message)

        # Check temp file created
        mock_tempfile.assert_called_once()

        # Check subprocess.run called with --prompt-file
        mock_run.assert_called_once()
        call_args = mock_run.call_args[0][0]

        self.assertEqual(call_args[0], "csm")
        self.assertEqual(call_args[1], "send")
        self.assertEqual(call_args[2], "test-session")
        self.assertEqual(call_args[3], "--prompt-file")
        self.assertEqual(call_args[4], "/tmp/test-prompt.txt")

        # Check temp file deleted after send
        mock_unlink.assert_called_once_with("/tmp/test-prompt.txt")

    @patch('astrocyte_messaging.subprocess.run', side_effect=subprocess.CalledProcessError(1, 'csm'))
    def test_send_propagates_calledprocesserror(self, mock_run):
        """subprocess.CalledProcessError should be propagated."""
        with self.assertRaises(subprocess.CalledProcessError):
            astrocyte_messaging._send_via_csm("test-session", "message")


class TestIntegration(unittest.TestCase):
    """Test end-to-end send_tagged_message() flow."""

    @patch('astrocyte_messaging._send_via_csm')
    @patch('astrocyte_messaging._log_message')
    def test_send_tagged_message_orchestrates_format_validate_log_send(self, mock_log, mock_send):
        """send_tagged_message should orchestrate: format → validate → log → send."""
        astrocyte_messaging.send_tagged_message(
            "test-session",
            "Test message",
            "diagnosis"
        )

        # Log should be called
        mock_log.assert_called_once()
        log_call = mock_log.call_args[0]
        self.assertEqual(log_call[0], "test-session")
        self.assertEqual(log_call[1], "diagnosis")

        # Send should be called with tagged message
        mock_send.assert_called_once()
        send_call = mock_send.call_args[0]
        self.assertEqual(send_call[0], "test-session")

        tagged_message = send_call[1]
        self.assertIn("<system-reminder>", tagged_message)
        self.assertIn("Source: astrocyte-daemon", tagged_message)
        self.assertIn("Test message", tagged_message)

    @patch('astrocyte_messaging._send_via_csm')
    def test_validation_failure_blocks_send(self, mock_send):
        """Validation failure should prevent send."""
        with self.assertRaises(ValueError):
            astrocyte_messaging.send_tagged_message(
                "",  # Empty session name
                "Test message",
                "diagnosis"
            )

        # Send should NOT be called
        mock_send.assert_not_called()

    @patch('astrocyte_messaging._send_via_csm')
    @patch('astrocyte_messaging._get_message_logger', side_effect=OSError("Log write failed"))
    def test_log_failure_does_not_block_send(self, mock_get_logger, mock_send):
        """Log failure should not prevent send (fail-safe)."""
        try:
            astrocyte_messaging.send_tagged_message(
                "test-session",
                "Test message",
                "diagnosis"
            )
        except Exception:
            self.fail("Log failure should not block send")

        # Send should still be called despite log failure
        mock_send.assert_called_once()


if __name__ == '__main__':
    unittest.main()
