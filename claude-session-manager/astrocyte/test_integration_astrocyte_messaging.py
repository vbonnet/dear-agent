"""
Integration tests for astrocyte_messaging module.

Tests Python → Go (csm send) → tmux coordination.

Requirements:
- tmux must be installed
- csm command must be available
- Ability to create/destroy tmux sessions

Run manually with: pytest test_integration_astrocyte_messaging.py -v

These tests create temporary tmux sessions and verify message delivery.
"""

import os
import subprocess
import time
import unittest
from pathlib import Path

import astrocyte_messaging


class TestPythonGoTmuxCoordination(unittest.TestCase):
    """Test message delivery through Python → csm (Go) → tmux pipeline."""

    @classmethod
    def setUpClass(cls):
        """Verify prerequisites before running tests."""
        # Check tmux available
        try:
            subprocess.run(["tmux", "-V"], check=True, capture_output=True)
        except (FileNotFoundError, subprocess.CalledProcessError):
            raise unittest.SkipTest("tmux not available")

        # Check csm available
        try:
            subprocess.run(["csm", "--version"], check=True, capture_output=True)
        except (FileNotFoundError, subprocess.CalledProcessError):
            raise unittest.SkipTest("csm command not available")

    def setUp(self):
        """Create temporary tmux session for each test."""
        self.session_name = f"test-astrocyte-{os.getpid()}-{int(time.time())}"

        # Create detached tmux session
        subprocess.run(
            ["tmux", "new-session", "-d", "-s", self.session_name, "cat"],
            check=True
        )

        # Wait for session to initialize
        time.sleep(0.5)

    def tearDown(self):
        """Destroy temporary tmux session after each test."""
        try:
            subprocess.run(
                ["tmux", "kill-session", "-t", self.session_name],
                check=False,  # Don't fail if session already gone
                capture_output=True
            )
        except Exception:
            pass  # Best effort cleanup

    def _get_pane_content(self) -> str:
        """Capture current pane content from tmux session."""
        result = subprocess.run(
            ["tmux", "capture-pane", "-t", self.session_name, "-p"],
            check=True,
            capture_output=True,
            text=True
        )
        return result.stdout

    def test_message_delivery_to_tmux_session(self):
        """Message should be delivered to tmux session via csm send."""
        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            "Integration test message",
            "notification"
        )

        # Wait for message to appear
        time.sleep(1)

        # Verify message delivered
        pane_content = self._get_pane_content()

        # Should contain tagged message elements
        self.assertIn("system-reminder", pane_content)
        self.assertIn("Source: astrocyte-daemon", pane_content)
        self.assertIn("Integration test message", pane_content)
        self.assertIn("Type: notification", pane_content)

    def test_multiline_message_preserved(self):
        """Multi-line messages should be preserved through delivery."""
        multiline_message = """Line 1: First line
Line 2: Second line
Line 3: Third line
Line 4: Fourth line"""

        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            multiline_message,
            "diagnosis"
        )

        # Wait for message
        time.sleep(1)

        # Verify all lines preserved
        pane_content = self._get_pane_content()

        self.assertIn("Line 1: First line", pane_content)
        self.assertIn("Line 2: Second line", pane_content)
        self.assertIn("Line 3: Third line", pane_content)
        self.assertIn("Line 4: Fourth line", pane_content)

    def test_special_characters_preserved(self):
        """Special characters should be preserved through delivery."""
        special_chars_message = """Special chars test:
- Quotes: "double" 'single'
- Brackets: [square] (round) {curly} <angle>
- Symbols: @#$%^&*()_+-=
- Backslash: \\ (single backslash)
- Unicode: ✓ ✗ → ← ↑ ↓"""

        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            special_chars_message,
            "notification"
        )

        # Wait for message
        time.sleep(1)

        # Verify special characters preserved
        pane_content = self._get_pane_content()

        self.assertIn('"double" \'single\'', pane_content)
        self.assertIn('[square] (round) {curly}', pane_content)
        self.assertIn('@#$%^&*()_+-=', pane_content)

    def test_large_message_via_prompt_file(self):
        """Large messages (>10KB) should use --prompt-file successfully."""
        # Create large message (15KB)
        large_message = "X" * 15_000

        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            large_message,
            "diagnosis"
        )

        # Wait for message
        time.sleep(2)  # Large messages may take longer

        # Verify delivered (check for source attribution)
        pane_content = self._get_pane_content()

        self.assertIn("Source: astrocyte-daemon", pane_content)
        self.assertIn("Type: diagnosis", pane_content)

    def test_logging_creates_messages_log(self):
        """Message send should create log entry in messages.log."""
        # Clear existing log if present
        log_file = Path.home() / ".agm/astrocyte/logs/messages.log"
        if log_file.exists():
            # Record initial size
            initial_size = log_file.stat().st_size
        else:
            initial_size = 0

        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            "Log test message",
            "notification"
        )

        # Wait for logging
        time.sleep(0.5)

        # Verify log file exists and grew
        self.assertTrue(log_file.exists(), "messages.log should exist")

        if initial_size > 0:
            final_size = log_file.stat().st_size
            self.assertGreater(final_size, initial_size, "Log should grow")

        # Verify log content
        with open(log_file, "r") as f:
            log_content = f.read()

        # Should contain session name and type
        self.assertIn(f"session={self.session_name}", log_content)
        self.assertIn("type=notification", log_content)

    def test_invalid_session_raises_error(self):
        """Sending to non-existent session should raise error."""
        nonexistent_session = "nonexistent-session-12345"

        # Should raise subprocess.CalledProcessError
        with self.assertRaises(subprocess.CalledProcessError):
            astrocyte_messaging.send_tagged_message(
                nonexistent_session,
                "Test message",
                "notification"
            )


class TestMessageFormatCompliance(unittest.TestCase):
    """Test message format compliance with D4 requirements."""

    def setUp(self):
        """Create temporary tmux session."""
        self.session_name = f"test-format-{os.getpid()}-{int(time.time())}"

        subprocess.run(
            ["tmux", "new-session", "-d", "-s", self.session_name, "cat"],
            check=True
        )
        time.sleep(0.5)

    def tearDown(self):
        """Destroy temporary tmux session."""
        try:
            subprocess.run(
                ["tmux", "kill-session", "-t", self.session_name],
                check=False,
                capture_output=True
            )
        except Exception:
            pass

    def _get_pane_content(self) -> str:
        """Capture pane content."""
        result = subprocess.run(
            ["tmux", "capture-pane", "-t", self.session_name, "-p"],
            check=True,
            capture_output=True,
            text=True
        )
        return result.stdout

    def test_message_format_matches_d4_specification(self):
        """Delivered message should match D4 format specification."""
        # Send message
        astrocyte_messaging.send_tagged_message(
            self.session_name,
            "Format compliance test",
            "violation_prompt"
        )

        time.sleep(1)

        # Verify format matches D4 spec
        pane_content = self._get_pane_content()

        # Required elements from D4
        self.assertIn("<system-reminder>", pane_content)
        self.assertIn("**This message is from Astrocyte Daemon**", pane_content)
        self.assertIn("Format compliance test", pane_content)
        self.assertIn("---", pane_content)
        self.assertIn("Source: astrocyte-daemon", pane_content)
        self.assertIn("Type: violation_prompt", pane_content)
        self.assertIn(f"Session: {self.session_name}", pane_content)
        self.assertIn("Timestamp:", pane_content)
        self.assertIn("</system-reminder>", pane_content)


if __name__ == '__main__':
    unittest.main()
