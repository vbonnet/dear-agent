"""
BDD tests for astrocyte permission prompt detection.

Tests cover:
- Category A: Tool usage violations (A1-A7)
- Category B: Legitimate security prompts (B1-B3)
- Category C: Edge cases (C1-C9)
"""

import pytest
import sys
from pathlib import Path

# Add parent directory to path to import astrocyte functions
sys.path.insert(0, str(Path(__file__).parent.parent))

# Import functions to test
from astrocyte import (
    capture_pane_state,
    is_stuck_permission_prompt,
    send_esc_key,
)


# ============================================================================
# Category A: Tool Usage Violations (Should Auto-Reject)
# ============================================================================

@pytest.mark.violations
@pytest.mark.bug_regression
def test_a1_long_heredoc_violation_detected(mock_tmux, mock_esc_sender, load_fixture):
    """Test A1: Long heredoc violation detected (Bug #1 regression test)."""

    # Arrange: Load fixture
    fixture_content = load_fixture('a1-long-heredoc')
    mock_tmux.return_value.stdout = fixture_content

    # Act: Run detection logic
    session_state = capture_pane_state("test-session")
    is_violation = is_stuck_permission_prompt(session_state)

    # Assert: Detection result
    assert is_violation == True, "Long heredoc should be detected as violation"

    # Assert: Bash command header captured (even though scrolled off)
    assert "Bash command" in session_state.pane_content


@pytest.mark.violations
@pytest.mark.parametrize("fixture_name,expected_detection,description", [
    ("a2-short-heredoc", True, "Short heredoc violation"),
    ("a3-cd-violation", True, "cd && command chaining"),
    ("a4-pipe-violation", True, "cat | grep piping"),
    ("a5-cat-violation", True, "cat for file reading"),
    ("a6-cp-violation", True, "cp for file copying"),
    ("a7-find-violation", True, "find for file searching"),
])
def test_violation_scenarios(mock_tmux, load_fixture, fixture_name, expected_detection, description):
    """Test violation scenarios (A2-A7)."""
    content = load_fixture(fixture_name)
    mock_tmux.return_value.stdout = content

    is_violation = is_stuck_permission_prompt(capture_pane_state("test"))
    assert is_violation == expected_detection, f"{description} should be detected"


# ============================================================================
# Category B: Legitimate Security Prompts (Should NOT Auto-Reject)
# ============================================================================

@pytest.mark.legitimate
@pytest.mark.parametrize("fixture_name,expected_detection,description", [
    ("b1-chmod-755", False, "Legitimate chmod prompt"),
    ("b2-rm-git-dir", False, "Legitimate rm .git/ prompt"),
    ("b3-dd-command", False, "Legitimate dd prompt"),
])
def test_legitimate_security_prompts(mock_tmux, load_fixture, fixture_name, expected_detection, description):
    """Test legitimate security prompts (B1-B3) - should NOT be auto-rejected."""
    content = load_fixture(fixture_name)
    mock_tmux.return_value.stdout = content

    is_violation = is_stuck_permission_prompt(capture_pane_state("test"))
    assert is_violation == expected_detection, f"{description} should NOT be auto-rejected"


# ============================================================================
# Category C: Edge Cases
# ============================================================================

@pytest.mark.batched
def test_c8_batched_prompts_rapid_recovery(mock_tmux, mock_esc_sender, load_fixture):
    """Test C8: Batched permission prompts handled with rapid recovery."""

    # First prompt
    prompt1 = load_fixture('c8-batched-prompt-1')
    mock_tmux.return_value.stdout = prompt1

    is_violation_1 = is_stuck_permission_prompt(capture_pane_state("test"))
    send_esc_key("test")

    assert is_violation_1 == True
    assert len(mock_esc_sender) == 1

    # Second prompt (batched)
    prompt2 = load_fixture('c8-batched-prompt-2')
    mock_tmux.return_value.stdout = prompt2

    is_violation_2 = is_stuck_permission_prompt(capture_pane_state("test"))
    send_esc_key("test")

    assert is_violation_2 == True
    assert len(mock_esc_sender) == 2


@pytest.mark.false_positive
def test_c9_blocked_tool_call_not_permission_prompt(mock_tmux, load_fixture):
    """Test C9: Blocked tool call should NOT be detected as permission prompt."""

    blocked_tool_call = load_fixture('c9-blocked-tool-call')
    mock_tmux.return_value.stdout = blocked_tool_call

    is_violation = is_stuck_permission_prompt(capture_pane_state("test"))

    assert is_violation == False, "Blocked tool call should NOT be detected as permission prompt"
    assert "Do you want to proceed?" not in blocked_tool_call


@pytest.mark.edge_cases
@pytest.mark.parametrize("fixture_name,expected_detection,description", [
    ("c1-zero-char-cmd", True, "Zero-char command"),
    ("c2-unicode-text", True, "Unicode characters"),
    ("c3-control-chars", True, "Control characters"),
    ("c4-no-bash-header", False, "Missing Bash command header"),
    ("c5-scrolled-off", True, "History scrolled off (Bug #1 test)"),
    ("c6-duration-5min", False, "Duration-based rejection removed (Bug #2 test)"),
    ("c7-mixed-violations", True, "Multiple violations in one prompt"),
])
def test_edge_cases(mock_tmux, load_fixture, fixture_name, expected_detection, description):
    """Test edge case scenarios (C1-C7)."""
    content = load_fixture(fixture_name)
    mock_tmux.return_value.stdout = content

    is_violation = is_stuck_permission_prompt(capture_pane_state("test"))
    assert is_violation == expected_detection, f"{description} - detection mismatch"
