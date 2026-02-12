"""
BDD step definitions for astrocyte_messaging features.

Implements steps for:
- message_attribution.feature (AC1)
- send_time_logging.feature (AC2)
- architectural_enforcement.feature (AC3)
- format_validation.feature (AC4)
- python_go_coordination.feature (AC5)

Run with: pytest --cucumber-json=report.json features/
Or with pytest-bdd: pytest features/test_bdd_steps.py -v

Note: pytest-bdd must be installed:
  pip install pytest-bdd
"""

import os
import subprocess
import time
from pathlib import Path
from unittest.mock import Mock, patch

import pytest
from pytest_bdd import given, when, then, scenarios, parsers

import astrocyte_messaging


# Load all feature scenarios
scenarios('../features/message_attribution.feature')
scenarios('../features/send_time_logging.feature')
scenarios('../features/architectural_enforcement.feature')
scenarios('../features/format_validation.feature')
scenarios('../features/python_go_coordination.feature')


# Shared context fixture
@pytest.fixture
def context():
    """Shared context for BDD scenarios."""
    return {
        'messages': [],
        'errors': [],
        'last_message': None,
        'session_name': f"test-bdd-{os.getpid()}",
        'log_entries_before': 0,
        'log_entries_after': 0,
    }


# ============================================================================
# GIVEN steps (Background setup)
# ============================================================================

@given("the astrocyte_messaging module is loaded")
def module_loaded():
    """Verify astrocyte_messaging module is imported."""
    assert astrocyte_messaging is not None


@given('the log directory exists at "~/.agm/astrocyte/logs/"')  # noqa: path-portability
def log_directory_exists():
    """Ensure log directory exists."""
    log_dir = Path.home() / ".agm/astrocyte/logs"
    log_dir.mkdir(parents=True, exist_ok=True)
    assert log_dir.exists()


@given("the messages.log file is empty or missing")
def clear_messages_log(context):
    """Clear messages.log for clean test state."""
    log_file = Path.home() / ".agm/astrocyte/logs/messages.log"
    if log_file.exists():
        # Record initial line count
        with open(log_file, "r") as f:
            context['log_entries_before'] = len(f.readlines())
    else:
        context['log_entries_before'] = 0


@given("the csm send command is available")
def csm_available():
    """Verify csm command exists."""
    try:
        subprocess.run(["csm", "--version"], check=True, capture_output=True)
    except FileNotFoundError:
        pytest.skip("csm command not available")


@given("a test tmux session exists")
def create_test_session(context):
    """Create temporary tmux session."""
    try:
        subprocess.run(["tmux", "-V"], check=True, capture_output=True)
    except FileNotFoundError:
        pytest.skip("tmux not available")

    session_name = context['session_name']

    subprocess.run(
        ["tmux", "new-session", "-d", "-s", session_name, "cat"],
        check=True
    )
    time.sleep(0.5)

    yield

    # Cleanup
    try:
        subprocess.run(
            ["tmux", "kill-session", "-t", session_name],
            check=False,
            capture_output=True
        )
    except Exception:
        pass


@given(parsers.parse('I have sent {count:d} {message_type} messages'))
def send_multiple_messages(context, count, message_type):
    """Send multiple messages of specified type."""
    for i in range(count):
        with patch('astrocyte_messaging._send_via_csm'):
            message = f"Test message {i+1}"
            astrocyte_messaging.send_tagged_message(
                context['session_name'],
                message,
                message_type
            )
            context['messages'].append({
                'content': message,
                'type': message_type,
            })


# ============================================================================
# WHEN steps (Actions)
# ============================================================================

@when(parsers.parse('I send a {message_type} message "{message}"'))
def send_message(context, message_type, message):
    """Send a message with specified type."""
    with patch('astrocyte_messaging._send_via_csm'):
        formatted = astrocyte_messaging._format_tagged_message(
            message,
            message_type,
            context['session_name']
        )
        context['last_message'] = formatted
        context['message_type'] = message_type


@when("I send a well-formed message")
def send_wellformed_message(context):
    """Send a valid message."""
    with patch('astrocyte_messaging._send_via_csm'):
        try:
            astrocyte_messaging.send_tagged_message(
                context['session_name'],
                "Well-formed test message",
                "diagnosis"
            )
            context['send_succeeded'] = True
        except ValueError:
            context['send_succeeded'] = False


@when("I attempt to send a message without source attribution")
def send_untagged_message(context):
    """Attempt to send message without source tag."""
    try:
        astrocyte_messaging._validate_message(
            context['session_name'],
            "<system-reminder>No source tag</system-reminder>",
            "diagnosis"
        )
        context['error'] = None
    except ValueError as e:
        context['error'] = e


@when(parsers.parse('I attempt to send an empty message "{message}"'))
def send_empty_message(context, message):
    """Attempt to send empty message."""
    try:
        astrocyte_messaging.send_tagged_message(
            context['session_name'],
            message,
            "diagnosis"
        )
        context['error'] = None
    except ValueError as e:
        context['error'] = e


@when(parsers.parse('I attempt to send a message with type "{message_type}"'))
def send_message_with_type(context, message_type):
    """Attempt to send message with specified type."""
    try:
        formatted = astrocyte_messaging._format_tagged_message(
            "Test message",
            message_type,
            context['session_name']
        )
        astrocyte_messaging._validate_message(
            context['session_name'],
            formatted,
            message_type
        )
        context['error'] = None
        context['send_succeeded'] = True
    except ValueError as e:
        context['error'] = e
        context['send_succeeded'] = False


@when(parsers.parse('I attempt to send a message to session "{session}"'))
def send_to_session(context, session):
    """Attempt to send message to specified session."""
    try:
        with patch('astrocyte_messaging._send_via_csm'):
            astrocyte_messaging.send_tagged_message(
                session,
                "Test message",
                "diagnosis"
            )
        context['error'] = None
    except ValueError as e:
        context['error'] = e


@when(parsers.parse('I send {count:d} diagnosis messages'))
def send_n_diagnosis_messages(context, count):
    """Send N diagnosis messages."""
    log_file = Path.home() / ".agm/astrocyte/logs/messages.log"

    # Count before
    if log_file.exists():
        with open(log_file, "r") as f:
            context['log_entries_before'] = len(f.readlines())
    else:
        context['log_entries_before'] = 0

    # Send messages
    for i in range(count):
        with patch('astrocyte_messaging._send_via_csm'):
            astrocyte_messaging.send_tagged_message(
                context['session_name'],
                f"Test message {i+1}",
                "diagnosis"
            )

    time.sleep(0.5)  # Allow logging

    # Count after
    if log_file.exists():
        with open(log_file, "r") as f:
            context['log_entries_after'] = len(f.readlines())
    else:
        context['log_entries_after'] = 0


# ============================================================================
# THEN steps (Assertions)
# ============================================================================

@then(parsers.parse('the message includes "{text}"'))
def message_includes_text(context, text):
    """Verify message contains specified text."""
    assert text in context['last_message'], \
        f"Expected '{text}' in message:\n{context['last_message']}"


@then("the send operation raises ValueError")
def send_raises_valueerror(context):
    """Verify send operation raised ValueError."""
    assert context.get('error') is not None, "Expected ValueError but none was raised"
    assert isinstance(context['error'], ValueError)


@then(parsers.parse('the error message includes "{text}"'))
def error_includes_text(context, text):
    """Verify error message contains text."""
    error_msg = str(context['error'])
    assert text in error_msg, \
        f"Expected '{text}' in error:\n{error_msg}"


@then(parsers.parse('{percentage:d}% of messages include "{text}"'))
def percentage_messages_include(context, percentage, text):
    """Verify percentage of messages contain text."""
    matching = 0
    total = len(context['messages'])

    for msg in context['messages']:
        formatted = astrocyte_messaging._format_tagged_message(
            msg['content'],
            msg['type'],
            context['session_name']
        )
        if text in formatted:
            matching += 1

    actual_percentage = (matching / total * 100) if total > 0 else 0
    assert actual_percentage == percentage, \
        f"Expected {percentage}% but got {actual_percentage}%"


@then("a log entry is created in messages.log")
def log_entry_created():
    """Verify log entry exists."""
    log_file = Path.home() / ".agm/astrocyte/logs/messages.log"
    assert log_file.exists(), "messages.log should exist"

    with open(log_file, "r") as f:
        content = f.read()
        assert len(content) > 0, "messages.log should not be empty"


@then(parsers.parse('the log entry includes "{text}"'))
def log_includes_text(text):
    """Verify log entry contains text."""
    log_file = Path.home() / ".agm/astrocyte/logs/messages.log"

    with open(log_file, "r") as f:
        content = f.read()

    assert text in content, f"Expected '{text}' in log:\n{content[-500:]}"


@then(parsers.parse('messages.log contains exactly {count:d} new log entries'))
def log_has_n_entries(context, count):
    """Verify exact number of new log entries."""
    new_entries = context['log_entries_after'] - context['log_entries_before']
    assert new_entries == count, \
        f"Expected {count} new entries, got {new_entries}"


@then("no ValueError is raised")
def no_valueerror_raised(context):
    """Verify no ValueError was raised."""
    assert context.get('error') is None, \
        f"Expected no error but got: {context.get('error')}"


@then("the send operation succeeds")
def send_succeeds(context):
    """Verify send operation succeeded."""
    assert context.get('send_succeeded', False) or context.get('error') is None


@then(parsers.parse('the message type is validated as "{validity}"'))
def message_type_validated(context, validity):
    """Verify message type validation result."""
    if validity == "valid":
        assert context.get('error') is None
    else:  # invalid
        assert context.get('error') is not None


@then(parsers.parse('the send operation {result}'))
def send_operation_result(context, result):
    """Verify send operation result."""
    if result == "succeeds":
        assert context.get('error') is None
    else:  # raises
        assert context.get('error') is not None


# ============================================================================
# Placeholder steps (implementation depends on actual system state)
# ============================================================================

@then(parsers.parse('the file permissions are {perms} (owner read/write only)'))
def check_file_permissions(perms):
    """Verify file permissions."""
    log_file = Path.home() / ".agm/astrocyte/logs/messages.log"
    if log_file.exists():
        stat_info = log_file.stat()
        actual_perms = oct(stat_info.st_mode)[-3:]
        expected_perms = perms[-3:]  # Extract digits from "0600"
        assert actual_perms == expected_perms, \
            f"Expected {expected_perms} but got {actual_perms}"


# Additional placeholder steps for integration scenarios
# These would be implemented based on actual tmux/csm testing requirements
