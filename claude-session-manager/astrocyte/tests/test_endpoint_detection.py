"""
Unit tests for endpoint detection logic.

Tests the is_conversation_endpoint_idle() function to ensure it correctly
identifies when a session is at a natural conversation endpoint (waiting for
user input) vs genuinely stuck/frozen.

Conservative interruption philosophy:
- Default: do not interrupt
- Only interrupt for genuine freezes (0 tokens bug, UI unresponsive)
- Better to miss an interruption than interrupt legitimate work
"""

import pytest
from datetime import datetime
from astrocyte import (
    is_conversation_endpoint_idle,
    has_completion_language,
    has_idle_prompt,
    has_pending_tool_calls,
    SessionState,
)


class TestEndpointDetection:
    """Test suite for endpoint detection logic."""

    def test_askuserquestion_prompt_detected_as_endpoint(self):
        """AskUserQuestion prompts should be detected as endpoints."""
        pane_content = """
● I need to choose an authentication approach. Which would you prefer?

  A) Passport.js (enhance existing)
  B) Auth0 (commercial service)
  C) Build from scratch

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 10),
            timestamp=datetime.now()
        )

        # Should detect as endpoint (has completion language + idle prompt)
        assert is_conversation_endpoint_idle(state) is True

    def test_completion_language_with_idle_prompt_is_endpoint(self):
        """Task completion with idle prompt should be endpoint."""
        pane_content = """
● ✅ Task completed successfully

All beads closed. Ready to proceed.

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 5),
            timestamp=datetime.now()
        )

        assert is_conversation_endpoint_idle(state) is True

    def test_active_tool_execution_not_endpoint(self):
        """Sessions with pending tool calls are not endpoints."""
        pane_content = """
● Reading file...

✶ Thinking…

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 3),
            timestamp=datetime.now()
        )

        # Has spinner pattern - NOT an endpoint
        assert is_conversation_endpoint_idle(state) is False

    def test_mustering_not_endpoint(self):
        """Mustering/thinking states are not endpoints."""
        pane_content = """
● Processing your request...

✻ Mustering...

"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 3),
            timestamp=datetime.now()
        )

        # Mustering pattern - NOT an endpoint
        assert is_conversation_endpoint_idle(state) is False

    def test_no_idle_prompt_not_endpoint(self):
        """Without idle prompt (❯), not an endpoint."""
        pane_content = """
● ✅ Task completed successfully

All done.

"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 4),
            timestamp=datetime.now()
        )

        # Has completion language but NO idle prompt - NOT endpoint
        assert is_conversation_endpoint_idle(state) is False

    def test_no_completion_language_not_endpoint(self):
        """Without completion language, not an endpoint."""
        pane_content = """
● Working on task...

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 3),
            timestamp=datetime.now()
        )

        # Has idle prompt but NO completion language - NOT endpoint
        assert is_conversation_endpoint_idle(state) is False

    def test_zero_tokens_waiting_not_endpoint(self):
        """0 tokens bug should not be detected as endpoint."""
        pane_content = """
● Processing...

Bootstrapping… (esc to interrupt · 4m 28s · ↓ 0 tokens)

"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 4),
            timestamp=datetime.now()
        )

        # 0 tokens waiting - NOT endpoint (genuine bug)
        assert is_conversation_endpoint_idle(state) is False


class TestCompletionLanguageDetection:
    """Test completion language pattern detection."""

    def test_task_completed_detected(self):
        """'Task completed' should be detected."""
        pane_content = "● ✅ Task completed successfully\n\n❯"
        assert has_completion_language(pane_content) is True

    def test_all_done_detected(self):
        """'All done' should be detected."""
        pane_content = "● All done!\n\n❯"
        assert has_completion_language(pane_content) is True

    def test_ready_to_proceed_detected(self):
        """'Ready to proceed' should be detected."""
        pane_content = "Ready to proceed.\n\n❯"
        assert has_completion_language(pane_content) is True

    def test_session_complete_detected(self):
        """'Session complete' should be detected."""
        pane_content = "Session complete. Exiting.\n\n❯"
        assert has_completion_language(pane_content) is True

    def test_checkmark_emoji_detected(self):
        """Checkmark emoji should be detected."""
        pane_content = "✅ Finished processing\n\n❯"
        assert has_completion_language(pane_content) is True

    def test_no_completion_language(self):
        """Regular text should not match completion patterns."""
        pane_content = "● Working on task...\n\n❯"
        assert has_completion_language(pane_content) is False


class TestIdlePromptDetection:
    """Test idle prompt (❯) detection."""

    def test_idle_prompt_at_end(self):
        """Idle prompt at end of pane should be detected."""
        pane_content = "● Task done\n\n❯"
        assert has_idle_prompt(pane_content) is True

    def test_idle_prompt_with_whitespace(self):
        """Idle prompt with trailing whitespace should be detected."""
        pane_content = "● Task done\n\n❯  "
        assert has_idle_prompt(pane_content) is True

    def test_no_idle_prompt(self):
        """Pane without idle prompt should not match."""
        pane_content = "● Task done\n\n✶ Thinking…"
        assert has_idle_prompt(pane_content) is False

    def test_idle_prompt_not_at_end(self):
        """Idle prompt not at end should not match (checks last 100 chars)."""
        pane_content = "❯ previous command\n\n● Working on something\n" + ("x" * 200)
        assert has_idle_prompt(pane_content) is False


class TestPendingToolCallsDetection:
    """Test pending tool call (spinner) detection."""

    def test_thinking_spinner_detected(self):
        """✶ Thinking… spinner should be detected."""
        pane_content = "● Processing\n\n✶ Thinking…"
        assert has_pending_tool_calls(pane_content) is True

    def test_mustering_spinner_detected(self):
        """✻ Mustering… spinner should be detected."""
        pane_content = "● Processing\n\n✻ Mustering…"
        assert has_pending_tool_calls(pane_content) is True

    def test_processing_spinner_detected(self):
        """✢ Processing… spinner should be detected."""
        pane_content = "● Working\n\n✢ Processing…"
        assert has_pending_tool_calls(pane_content) is True

    def test_galloping_spinner_detected(self):
        """Galloping… spinner should be detected."""
        pane_content = "● Working\n\nGalloping…"
        assert has_pending_tool_calls(pane_content) is True

    def test_no_spinner(self):
        """Pane without spinner should not match."""
        pane_content = "● Task completed\n\n❯"
        assert has_pending_tool_calls(pane_content) is False


class TestConservativeInterruption:
    """Test conservative interruption logic (integration-style)."""

    def test_askuserquestion_not_interrupted(self):
        """Session waiting for AskUserQuestion should NOT be interrupted."""
        # This is the KEY test - ensures we don't interrupt legitimate work
        pane_content = """
● I need your decision on authentication approach:

  A) Passport.js (4 hours, $50/month)
  B) Auth0 (0 hours, $2300/month)
  C) Build from scratch (20 hours, $0/month)

Which approach fits best?

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 10),
            timestamp=datetime.now()
        )

        # Must be detected as endpoint to avoid interruption
        assert is_conversation_endpoint_idle(state) is True

    def test_planning_session_not_interrupted(self):
        """Planning session at completion should NOT be interrupted."""
        pane_content = """
● ✅ Plan finalized

I've created a comprehensive implementation plan. Ready for your approval.

❯
"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 5),
            timestamp=datetime.now()
        )

        # Must be detected as endpoint
        assert is_conversation_endpoint_idle(state) is True

    def test_genuine_freeze_can_be_interrupted(self):
        """Genuine freeze (mustering for >20 min) CAN be interrupted."""
        pane_content = """
● Processing task...

✻ Mustering...

"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 3),
            timestamp=datetime.now()
        )

        # NOT an endpoint - eligible for interruption
        assert is_conversation_endpoint_idle(state) is False

    def test_zero_tokens_bug_can_be_interrupted(self):
        """0 tokens bug CAN be interrupted."""
        pane_content = """
● Bootstrapping…

Bootstrapping… (esc to interrupt · 15m 32s · ↓ 0 tokens)

"""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 4),
            timestamp=datetime.now()
        )

        # NOT an endpoint - eligible for interruption (genuine bug)
        assert is_conversation_endpoint_idle(state) is False


class TestEdgeCases:
    """Test edge cases and boundary conditions."""

    def test_partial_completion_signal(self):
        """Partial completion (only 1 of 2 signals) should NOT be endpoint."""
        # Has idle prompt but no completion language
        pane_content = "● Still working...\n\n❯"
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 3),
            timestamp=datetime.now()
        )
        assert is_conversation_endpoint_idle(state) is False

    def test_completion_with_spinner(self):
        """Completion language + spinner should NOT be endpoint."""
        # Has completion language but also has spinner (contradiction)
        pane_content = "● ✅ Task completed\n\n✶ Thinking…\n\n❯"
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 5),
            timestamp=datetime.now()
        )
        # Has spinner - NOT endpoint
        assert is_conversation_endpoint_idle(state) is False

    def test_empty_pane(self):
        """Empty pane should NOT be endpoint."""
        pane_content = ""
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 0),
            timestamp=datetime.now()
        )
        assert is_conversation_endpoint_idle(state) is False

    def test_whitespace_only_pane(self):
        """Whitespace-only pane should NOT be endpoint."""
        pane_content = "\n\n\n   \n\n"
        state = SessionState(
            session_name="test-session",
            pane_content=pane_content,
            cursor_position=(0, 2),
            timestamp=datetime.now()
        )
        assert is_conversation_endpoint_idle(state) is False


@pytest.mark.parametrize("completion_phrase", [
    "Task completed",
    "All done",
    "Ready to proceed",
    "Session complete",
    "✅",
    "Finished",
    "Complete",
])
def test_completion_language_variations(completion_phrase):
    """Test various completion language variations."""
    pane_content = f"● {completion_phrase}\n\n❯"
    assert has_completion_language(pane_content) is True


@pytest.mark.parametrize("spinner_pattern", [
    "✶ Thinking…",
    "✻ Mustering…",
    "✢ Processing…",
    "Galloping…",
    "Cogitating…",
    "Channelling…",
])
def test_spinner_pattern_variations(spinner_pattern):
    """Test various spinner pattern variations."""
    pane_content = f"● Working\n\n{spinner_pattern}"
    assert has_pending_tool_calls(pane_content) is True
