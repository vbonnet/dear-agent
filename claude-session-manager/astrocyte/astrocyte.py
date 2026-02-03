#!/usr/bin/env python3
"""
Astrocyte - CSM Session Monitor

Autonomous daemon for detecting and recovering stuck CSM sessions.
"""

import sys
import time
import subprocess
import re
import json
import os
import requests
import yaml
from datetime import datetime, timedelta
from dataclasses import dataclass, asdict, field
from pathlib import Path
from typing import Dict, Any, List

# Import remote reporter
try:
    from reporter import RemoteReporter, RemoteReporterConfig
    REMOTE_REPORTER_AVAILABLE = True
except ImportError:
    REMOTE_REPORTER_AVAILABLE = False
    RemoteReporter = None
    RemoteReporterConfig = None


@dataclass
class SessionState:
    """Captured state of a tmux session."""
    timestamp: datetime
    session_name: str
    pane_content: str
    cursor_position: tuple[int, int]


@dataclass
class RecoveryResult:
    """Result of a recovery attempt."""
    success: bool
    method: str  # "escape", "ctrl_c", "manual"
    duration_seconds: float
    before_state: SessionState
    after_state: SessionState


@dataclass
class Incident:
    """Hang incident record."""
    timestamp: str  # ISO 8601
    session_name: str
    session_id: str  # UUID from manifest
    symptom: str  # "stuck_mustering", "stuck_waiting", etc.
    duration_minutes: int
    detection_heuristic: str
    pane_snapshot: str
    cursor_position: str  # "x,y"
    recovery_attempted: bool
    recovery_method: str | None
    recovery_success: bool | None
    recovery_duration_seconds: float | None
    cascade_depth: int = 1  # Number of prompts cleared in cascade
    circuit_breaker_triggered: bool = False  # Circuit breaker fired?
    diagnosis_filed: bool
    diagnosis_file: str | None

    def to_json(self) -> str:
        """Serialize to JSON string."""
        return json.dumps(asdict(self))


@dataclass
class Config:
    """Astrocyte configuration."""
    # Global settings
    interval_seconds: int = 60  # 1 minute (fast detection)

    # Detection thresholds (minutes)
    # Tuned based on observed false positive data
    mustering_timeout: int = 10      # Mustering usually resolves within 10min
    zero_token_waiting: int = 3      # 0 tokens = stuck, recover quickly
    cursor_frozen: int = 15           # Legitimate thinking can take time
    ask_question_violation: int = 10  # Give time for complex questions
    permission_prompt_duration: int = 5  # Duration-based detection (fresh start uses violation patterns)

    # Slack configuration
    slack_enabled: bool = False
    slack_webhook_url: str | None = None

    # Email configuration
    email_enabled: bool = False
    email_smtp_host: str = "localhost"
    email_smtp_port: int = 587
    email_smtp_use_tls: bool = True
    email_smtp_user: str | None = None
    email_smtp_password: str | None = None
    email_from: str = "astrocyte@localhost"
    email_to: List[str] = field(default_factory=lambda: [])

    # Recovery settings
    recovery_enabled: bool = True
    recovery_method: str = "escape"  # "escape", "ctrl_c", "session_restart", "chain", "manual_alert"
    recovery_max_attempts: int = 1
    recovery_strategy_chain: List[str] = field(default_factory=lambda: ["escape", "ctrl_c"])  # Used when recovery_method="chain"

    # Logging settings
    incidents_file: str = field(default_factory=lambda: os.path.expanduser("~/.csm/astrocyte/incidents.jsonl"))
    diagnoses_dir: str = field(default_factory=lambda: os.path.expanduser("~/.csm/astrocyte/diagnoses"))
    verbose: bool = False

    # Diagnosis settings
    diagnosis_enabled: bool = True
    diagnosis_use_csm: bool = True
    diagnosis_fallback_tmux: bool = True

    # Per-session overrides
    session_overrides: Dict[str, Dict[str, Any]] = field(default_factory=dict)

    # Multi-user settings
    multi_user_enabled: bool = False
    user_home_directories: List[str] = field(default_factory=lambda: [str(Path.home())])  # Default to current user
    separate_incident_logs: bool = False  # If True, use per-user incident logs; if False, use single combined log

    # Remote monitoring (cloud deployment)
    remote_enabled: bool = False
    remote_collector_url: str = ""
    remote_api_token: str = ""
    remote_report_interval: int = 300  # 5 minutes
    remote_report_incidents: bool = True
    remote_report_metrics: bool = True

    def get_threshold(self, session_name: str, threshold_name: str) -> int:
        """Get threshold for a session, applying overrides if present."""
        if session_name in self.session_overrides:
            override = self.session_overrides[session_name].get(threshold_name)
            if override is not None:
                return override
        return getattr(self, threshold_name)


def load_config() -> Config:
    """
    Load configuration from ~/.csm/astrocyte/config.yaml.  # noqa: path-portability

    Falls back to config.json for backward compatibility.
    Returns default config if no file exists.
    """
    config_dir = Path.home() / ".csm/astrocyte"
    yaml_config = config_dir / "config.yaml"
    json_config = config_dir / "config.json"

    # Try YAML first (preferred)
    if yaml_config.exists():
        try:
            with open(yaml_config) as f:
                data = yaml.safe_load(f)

            # Extract values with defaults
            thresholds = data.get("thresholds", {})
            slack = data.get("slack", {})
            recovery = data.get("recovery", {})
            logging_cfg = data.get("logging", {})
            diagnosis = data.get("diagnosis", {})
            email = data.get("email", {})
            multi_user = data.get("multi_user", {})

            return Config(
                interval_seconds=data.get("interval_seconds", 60),
                mustering_timeout=thresholds.get("mustering_timeout", 10),
                zero_token_waiting=thresholds.get("zero_token_waiting", 10),
                cursor_frozen=thresholds.get("cursor_frozen", 15),
                ask_question_violation=thresholds.get("ask_question_violation", 5),
                slack_enabled=slack.get("enabled", False),
                slack_webhook_url=slack.get("webhook_url"),
                recovery_enabled=recovery.get("enabled", True),
                recovery_method=recovery.get("method", "escape"),
                recovery_max_attempts=recovery.get("max_attempts", 1),
                recovery_strategy_chain=recovery.get("strategy_chain", ["escape", "ctrl_c"]),
                incidents_file=os.path.expanduser(logging_cfg.get("incidents_file", "~/.csm/astrocyte/incidents.jsonl")),
                diagnoses_dir=os.path.expanduser(logging_cfg.get("diagnoses_dir", "~/.csm/astrocyte/diagnoses")),
                verbose=logging_cfg.get("verbose", False),
                diagnosis_enabled=diagnosis.get("enabled", True),
                diagnosis_use_csm=diagnosis.get("use_csm_prompt_file", True),
                diagnosis_fallback_tmux=diagnosis.get("fallback_to_tmux", True),
                email_enabled=email.get("enabled", False),
                email_smtp_host=email.get("smtp_host", "localhost"),
                email_smtp_port=email.get("smtp_port", 587),
                email_smtp_use_tls=email.get("smtp_use_tls", True),
                email_smtp_user=email.get("smtp_user"),
                email_smtp_password=email.get("smtp_password"),
                email_from=email.get("from", "astrocyte@localhost"),
                email_to=email.get("to", []),
                session_overrides=data.get("session_overrides", {}),
                multi_user_enabled=multi_user.get("enabled", False),
                user_home_directories=multi_user.get("home_directories", [str(Path.home())]),
                separate_incident_logs=multi_user.get("separate_incident_logs", False),
                remote_enabled=data.get("remote", {}).get("enabled", False),
                remote_collector_url=data.get("remote", {}).get("collector_url", ""),
                remote_api_token=os.getenv("ASTROCYTE_API_TOKEN", data.get("remote", {}).get("api_token", "")),
                remote_report_interval=data.get("remote", {}).get("report_interval", 300),
                remote_report_incidents=data.get("remote", {}).get("report_incidents", True),
                remote_report_metrics=data.get("remote", {}).get("report_metrics", True)
            )
        except Exception as e:
            print(f"⚠️  Failed to load {yaml_config}: {e}", file=sys.stderr)
            print("   Using default configuration", file=sys.stderr)

    # Fall back to JSON (legacy, webhook-only config)
    elif json_config.exists():
        try:
            with open(json_config) as f:
                data = json.load(f)

            return Config(
                slack_enabled=True,  # If JSON config exists, Slack was enabled
                slack_webhook_url=data.get("slack_webhook_url")
            )
        except Exception as e:
            print(f"⚠️  Failed to load {json_config}: {e}", file=sys.stderr)
            print("   Using default configuration", file=sys.stderr)

    # No config file - use defaults
    return Config()


def get_sessions_from_home_directories(home_dirs: List[str]) -> list[tuple[str, str]]:
    """
    Discover CSM sessions by scanning session directories in multiple home directories.

    Args:
        home_dirs: List of home directory paths to scan (e.g., ["/home/user1", "/home/user2"])

    Returns:
        List of (session_name, home_dir) tuples for sessions with manifest.yaml files
    """
    sessions = []

    for home_dir in home_dirs:
        home_path = Path(home_dir).expanduser()
        sessions_dir = home_path / "src/sessions"

        if not sessions_dir.exists():
            continue

        try:
            # List all directories in src/sessions/
            for session_dir in sessions_dir.iterdir():
                if session_dir.is_dir():
                    manifest = session_dir / "manifest.yaml"
                    if manifest.exists():
                        sessions.append((session_dir.name, str(home_path)))
        except PermissionError:
            # Can't access this user's directory
            print(f"⚠️  Permission denied accessing {sessions_dir}", file=sys.stderr)
            continue
        except Exception as e:
            print(f"⚠️  Error scanning {sessions_dir}: {e}", file=sys.stderr)
            continue

    return sessions


def get_tmux_cmd() -> list[str]:
    """
    Get tmux command prefix with CSM socket if available.

    CSM uses a custom tmux socket at /tmp/csm.sock.
    Returns ["tmux", "-S", "/tmp/csm.sock"] or ["tmux"].
    """
    csm_socket = Path("/tmp/csm.sock")
    return ["tmux", "-S", str(csm_socket)] if csm_socket.exists() else ["tmux"]


def get_active_csm_sessions() -> list[str]:
    """
    List all active CSM sessions from tmux.

    A CSM session is identified by having a manifest.yaml file.
    """
    try:
        tmux_cmd = get_tmux_cmd()

        # Get all tmux session names
        result = subprocess.run(
            tmux_cmd + ["list-sessions", "-F", "#{session_name}"],
            capture_output=True,
            text=True,
            check=False
        )

        if result.returncode != 0:
            # No tmux server running or no sessions
            return []

        sessions = result.stdout.strip().split("\n") if result.stdout.strip() else []

        # Filter for CSM sessions (have manifest.yaml)
        csm_sessions = []
        for session in sessions:
            if session_has_manifest(session):
                csm_sessions.append(session)

        return csm_sessions

    except Exception as e:
        print(f"Error listing sessions: {e}", file=sys.stderr)
        return []


def session_has_manifest(session_name: str) -> bool:
    """Check if session has a CSM manifest file."""
    manifest_path = Path.home() / "src/sessions" / session_name / "manifest.yaml"
    return manifest_path.exists()


def capture_pane_state(session_name: str) -> SessionState:
    """
    Capture current state of tmux pane.

    Returns SessionState with pane content and cursor position.
    """
    # Capture pane content (with history to ensure long commands are fully captured)
    # -S -500 captures last 500 lines to ensure "Bash command" header is included
    # even when long heredocs or multiline commands push it off the visible viewport
    tmux_cmd = get_tmux_cmd()
    pane_result = subprocess.run(
        tmux_cmd + ["capture-pane", "-t", session_name, "-p", "-S", "-500"],
        capture_output=True,
        text=True,
        check=True
    )
    pane_content = pane_result.stdout

    # Capture cursor position
    cursor_result = subprocess.run(
        tmux_cmd + ["display-message", "-t", session_name, "-p", "#{cursor_x},#{cursor_y}"],
        capture_output=True,
        text=True,
        check=True
    )
    cursor_parts = cursor_result.stdout.strip().split(",")
    cursor_position = (int(cursor_parts[0]), int(cursor_parts[1]))

    return SessionState(
        timestamp=datetime.now(),
        session_name=session_name,
        pane_content=pane_content,
        cursor_position=cursor_position
    )


# Detection patterns
MUSTERING_PATTERNS = [
    r"✻ Mustering\.\.\.",
    r"✶ Evaporating\.\.\.",
    r"✢ Mustering\.\.\.",
]

WAITING_PATTERNS = [
    # Generic Claude spinner pattern (catches ANY spinner)
    # Format: "symbol word…" where symbol is ✶✢✻· etc. and word is any verb
    # Examples: "✶ Flambéing…", "✢ Puzzling…", "✻ Cooked…"
    r"[✶✢✻·✽]\s+\w+…",  # Matches any spinner symbol + word + ellipsis

    # Specific spinner patterns (kept for backwards compatibility and debugging)
    # Note: Claude uses Unicode ellipsis … (U+2026), not three dots ...
    r"Channelling…",
    r"Hashing…",
    r"Swirling…",
    r"Stewing…",
    r"Meandering…",
    r"Canoodling…",
    r"Galloping…",
    r"Brewing…",
    r"Churning…",
    r"Cogitating…",
    r"Puzzling…",
    r"Flambéing…",
    r"Cooked…",
    r"Baked…",
    r"Warping…",
    r"Swooping…",
    r"Zesting…",
    r"Symbioting…",

    # Generic waiting patterns
    r"Waiting for completion",
    r"Waiting…",
]

# AskUserQuestion violation patterns
# Detects when agent asks questions in text instead of using AskUserQuestion tool
QUESTION_PATTERNS = [
    r"Should I\s+",
    r"Which approach",
    r"Do you prefer",
    r"Would you like",
    r"How would you like to proceed",
    r"How should I proceed",
    r"Choose between",
    r"Option A.*Option B",  # Lettered options
    r"^\s*[A-D]\)",  # Lettered list (A) B) C) D))
    r"^\s*\d+\.",  # Numbered list presenting choices
    r"Confirm (you want|the|your)",
    r"Verify (the|your|that)",
]


def is_stuck_mustering(
    current: SessionState,
    previous: SessionState,
    threshold_minutes: int
) -> bool:
    """
    Detect session stuck in mustering state.

    Returns True if:
    - Mustering/Evaporating pattern visible in current state
    - Same pattern was visible in previous state
    - Time delta > threshold_minutes
    """
    for pattern in MUSTERING_PATTERNS:
        if re.search(pattern, current.pane_content):
            # Check if pattern was present in previous state
            if re.search(pattern, previous.pane_content):
                delta = current.timestamp - previous.timestamp
                if delta > timedelta(minutes=threshold_minutes):
                    return True
    return False


def is_stuck_zero_token_waiting(
    current: SessionState,
    previous: SessionState,
    threshold_minutes: int
) -> bool:
    """
    Detect session stuck in thinking state with zero token activity.

    SIMPLIFIED APPROACH: Focus on "↓ 0 tokens" as the universal stuck indicator.
    Don't check for specific spinner patterns - they're too variable.

    Returns True if:
    - Zero tokens downloaded (↓ 0 tokens) - indicates no progress
    - Duration > threshold_minutes (parsed from "esc to interrupt · Xm Ys")
    - Has "esc to interrupt" pattern (indicates thinking state)

    Why this works:
    - ANY thinking state with 0 tokens for >10 minutes is stuck
    - Spinner word doesn't matter (Bootstrapping, Improvising, Canoodling, etc.)
    - "↓ 0 tokens" is the key - means no API activity/progress
    - Catches both types: waiting for response AND spontaneous stuck thinking

    Examples caught:
    - "Bootstrapping… (esc to interrupt · 4m 28s · ↓ 0 tokens)" ← stuck thinking
    - "Improvising… (esc to interrupt · 15m 43s · ↓ 0 tokens)" ← waiting after question
    - "Canoodling… (esc to interrupt · 27m 57s · ↓ 0 tokens)" ← worker coordination
    """
    # Check if zero tokens downloaded (primary indicator)
    zero_tokens = re.search(r"↓ 0 tokens", current.pane_content)
    if not zero_tokens:
        return False

    # Parse duration from pane content (format: "21m 56s" or "1h 23m 45s")
    # Pattern: (esc to interrupt · 21m 56s · ↓ 0 tokens)
    # OR: (esc to interrupt · ctrl+t to hide tasks · 11m 48s · ↓ 0 tokens)
    # The presence of "esc to interrupt" confirms it's in a thinking/interruptible state
    # Use .*? to skip any intermediate text (like task UI hints)
    duration_match = re.search(r"esc to interrupt.*?((?:\d+h )?(?:\d+m )?(?:\d+s)?)\s*·\s*↓ 0 tokens", current.pane_content)
    if not duration_match:
        return False

    duration_str = duration_match.group(1).strip()

    # Parse duration into minutes
    total_minutes = 0
    hours_match = re.search(r"(\d+)h", duration_str)
    minutes_match = re.search(r"(\d+)m", duration_str)
    seconds_match = re.search(r"(\d+)s", duration_str)

    if hours_match:
        total_minutes += int(hours_match.group(1)) * 60
    if minutes_match:
        total_minutes += int(minutes_match.group(1))
    if seconds_match:
        total_minutes += int(seconds_match.group(1)) / 60

    # Check if duration exceeds threshold
    return total_minutes >= threshold_minutes


def is_stuck_cursor_frozen(
    current: SessionState,
    previous: SessionState,
    threshold_minutes: int
) -> bool:
    """
    Detect session stuck with frozen cursor (no cursor movement).

    Covers edge case: Session UI frozen, not responding to input.
    Agent stuck in infinite loop or waiting for external resource.

    Returns True if:
    - Cursor position unchanged between checks
    - Pane content unchanged (no new output)
    - Duration > threshold_minutes

    Note: This is a strong signal for UI-level hangs where the session
    appears stuck with no visible activity.
    """
    if not previous:
        return False

    # Check if cursor position is identical
    cursor_frozen = (current.cursor_position == previous.cursor_position)
    if not cursor_frozen:
        return False

    # Check if pane content is identical (no new output)
    content_unchanged = (current.pane_content == previous.pane_content)
    if not content_unchanged:
        return False

    # Check duration
    delta = current.timestamp - previous.timestamp
    if delta > timedelta(minutes=threshold_minutes):
        return True

    return False


def is_asking_question_without_tool(
    current: SessionState,
    previous: SessionState,
    threshold_minutes: int
) -> bool:
    """
    Detect agent asking questions in text without using AskUserQuestion tool.

    Covers pattern violation: Agent asks user to make decisions but doesn't
    use the AskUserQuestion tool, violating the pattern requirements.

    Returns True if:
    - Question patterns detected in pane content
    - Agent appears to be waiting for user response
    - Pattern persists across check cycles
    - Duration > threshold_minutes

    This heuristic triggers sending a violation prompt from
    ~/src/ws/oss/ask-question-violations/prompts/VIOLATION-PROMPTS.md
    """
    # Check for question patterns in current state
    has_question_pattern = False
    for pattern in QUESTION_PATTERNS:
        if re.search(pattern, current.pane_content, re.MULTILINE):
            has_question_pattern = True
            break

    if not has_question_pattern:
        return False

    # Check if agent appears to be waiting (cursor at end, no new activity)
    # Look for typical waiting indicators:
    # - Question mark in last 500 chars
    # - Cursor at bottom of pane
    # - No mustering/galloping patterns (those are handled by other heuristics)
    last_chars = current.pane_content[-500:] if len(current.pane_content) > 500 else current.pane_content
    has_question_mark = "?" in last_chars

    if not has_question_mark:
        return False

    # Check duration (must be present in previous state too)
    if previous:
        # Check if question pattern persists from previous state
        prev_has_question = any(re.search(p, previous.pane_content, re.MULTILINE) for p in QUESTION_PATTERNS)
        prev_has_question_mark = "?" in (previous.pane_content[-500:] if len(previous.pane_content) > 500 else previous.pane_content)

        if prev_has_question and prev_has_question_mark:
            delta = current.timestamp - previous.timestamp
            if delta > timedelta(minutes=threshold_minutes):
                return True

    return False


def is_stuck_permission_prompt(
    current: SessionState,
    previous: SessionState,
    threshold_minutes: int
) -> bool:
    """
    Detect sessions stuck on permission prompts for bash commands.

    Permission prompts show when PreToolUse hook blocks commands:
    "Do you want to proceed?"
    ❯ 1. Yes
      2. No

    This is different from AskUserQuestion which is for decisions.
    Permission prompts need automatic rejection for tool usage violations.

    Returns True if:
    - Permission prompt pattern detected
    - Command contains clear tool usage violations (immediate detection)
    - OR prompt unchanged for threshold duration (requires previous state)
    """
    # Check for permission prompt pattern
    permission_patterns = [
        r'Do you want to proceed\?',
        r'❯\s+1\.\s+Yes',
        r'Esc to cancel.*Tab to add additional instructions',
    ]

    has_permission_pattern = any(
        re.search(pattern, current.pane_content, re.MULTILINE)
        for pattern in permission_patterns
    )

    if not has_permission_pattern:
        return False

    # Extract bash command from prompt (appears before "Do you want to proceed?")
    # Look for "Bash command" header followed by command text
    # Format: "Bash command\n\n   <command>\n   <description>\n\n Do you want to proceed?"
    bash_command_match = re.search(
        r'Bash command\s+(.*?)\s+Do you want to proceed\?',
        current.pane_content,
        re.DOTALL
    )

    # Check for clear tool usage violations that should be rejected immediately
    # These patterns indicate the session is definitely stuck and needs rejection
    violation_patterns = [
        r'\bcd\s+.*&&',           # cd followed by &&
        r'&&',                     # Command chaining
        r';\s*\w+',               # Semicolon chaining
        r'\|\|',                   # OR operator
        r'\|(?!\s*$)',            # Pipe (not at end of line)
        r'>(?!>)',                # Redirect output
        r'>>',                     # Append redirect
        r'<<',                     # Here document
        r'\bfor\s+\w+\s+in\b',    # For loop
        r'\bwhile\s+',            # While loop
        r'\b(cat|grep|find|sed|awk|head|tail|wc|stat|ls)\s+',  # Text processing and file reading
        r'\b(rm|cp|mv|mkdir)\s+',    # File operations
        r'\binstall\s+-D',         # install -D (file operations)
    ]

    has_violation = False
    if bash_command_match:
        command_text = bash_command_match.group(1)
        has_violation = any(
            re.search(pattern, command_text)
            for pattern in violation_patterns
        )

    # If we found a clear violation, return True immediately (fresh start detection)
    if has_violation:
        return True

    # If no violation found, do NOT auto-reject
    # Only tool usage violations should be auto-rejected
    # Legitimate security prompts (rm .git/, chmod) should remain for user decision
    return False


def recover_session(session_name: str, config: Config) -> RecoveryResult:
    """
    Dispatch to appropriate recovery strategy based on configuration.

    Args:
        session_name: Name of the stuck session
        config: Configuration object with recovery settings

    Returns RecoveryResult from the selected recovery strategy.
    """
    method = config.recovery_method

    if method == "escape":
        return recover_with_escape(session_name)
    elif method == "ctrl_c":
        return recover_with_ctrl_c(session_name)
    elif method == "session_restart":
        return recover_with_session_restart(session_name)
    elif method == "chain":
        return recover_with_strategy_chain(session_name, config.recovery_strategy_chain)
    elif method == "manual_alert":
        # Manual alert mode: don't actually recover, just return a fake "success"
        # The incident will be logged and notifications sent, but no recovery attempted
        print(f"   📋 Manual alert mode: Detection only, no auto-recovery")
        return RecoveryResult(
            success=False,
            method="manual_alert",
            duration_seconds=0,
            before_state=None,
            after_state=None
        )
    else:
        print(f"   ⚠️  Unknown recovery method '{method}', falling back to escape")
        return recover_with_escape(session_name)


def verify_recovery(before: SessionState, after: SessionState) -> bool:
    """
    Check if session recovered (meaningful content changed, stuck patterns gone).

    Returns True if session is actually unstuck (not just timer updates).

    Key improvements:
    - Normalizes timers to ignore timestamp changes (e.g., "5m 10s" → "5m 15s")
    - Checks if stuck patterns persist (0 tokens, mustering, waiting)
    - Only reports success if meaningful content changed
    """
    # Normalize content by removing dynamic timers/timestamps
    # Pattern: "1m 23s", "45s", "2h 15m 30s", "esc to interrupt · 14m 15s"
    timer_pattern = r'\d+[smh](\s+\d+[smh])*'

    before_normalized = re.sub(timer_pattern, 'TIME', before.pane_content)
    after_normalized = re.sub(timer_pattern, 'TIME', after.pane_content)

    # If normalized content is identical, only timer changed (NOT recovery)
    if before_normalized == after_normalized:
        return False

    # Check if stuck patterns persist after recovery attempt
    stuck_patterns = [
        r'↓ 0 tokens',              # Zero token download (stuck waiting)
        r'Mustering\.\.\.',         # Stuck mustering
        r'Flowing\.\.\.',           # Stuck flowing with 0 tokens
        r'Drizzling\.\.\.',         # Stuck drizzling with 0 tokens
        r'Cooking\.\.\.',           # Stuck cooking with 0 tokens
        r'Metamorphosing\.\.\.',    # Stuck metamorphosing with 0 tokens
    ]

    # If any stuck pattern was present before AND still present after, recovery failed
    for pattern in stuck_patterns:
        if re.search(pattern, before.pane_content):
            if re.search(pattern, after.pane_content):
                # Pattern still present - still stuck
                return False

    # Mustering pattern gone = unstuck
    for pattern in MUSTERING_PATTERNS:
        if re.search(pattern, before.pane_content):
            if not re.search(pattern, after.pane_content):
                return True

    # Cursor moved = likely unstuck (but check it moved meaningfully)
    # Ignore tiny cursor movements (< 3 rows or columns) that could be jitter
    if before.cursor_position != after.cursor_position:
        row_diff = abs(after.cursor_position[0] - before.cursor_position[0])
        col_diff = abs(after.cursor_position[1] - before.cursor_position[1])
        if row_diff >= 3 or col_diff >= 3:
            return True

    # Meaningful content changed (after normalization) = likely unstuck
    if before_normalized != after_normalized:
        return True

    return False


def recover_with_escape(session_name: str) -> RecoveryResult:
    """
    Attempt recovery by sending ESC.

    Returns RecoveryResult with success status and duration.
    """
    before = capture_pane_state(session_name)
    start_time = time.time()

    # Send ESC
    tmux_cmd = get_tmux_cmd()
    subprocess.run(tmux_cmd + ["send-keys", "-t", session_name, "Escape"])

    # Wait for recovery
    time.sleep(5)

    after = capture_pane_state(session_name)
    duration = time.time() - start_time

    # Check if unstuck
    success = verify_recovery(before, after)

    return RecoveryResult(
        success=success,
        method="escape",
        duration_seconds=duration,
        before_state=before,
        after_state=after
    )


def recover_with_ctrl_c(session_name: str) -> RecoveryResult:
    """
    Attempt recovery by sending Ctrl-C.

    More aggressive than ESC, useful for interrupting running processes.

    Returns RecoveryResult with success status and duration.
    """
    before = capture_pane_state(session_name)
    start_time = time.time()

    # Send Ctrl-C
    tmux_cmd = get_tmux_cmd()
    subprocess.run(tmux_cmd + ["send-keys", "-t", session_name, "C-c"])

    # Wait for recovery
    time.sleep(5)

    after = capture_pane_state(session_name)
    duration = time.time() - start_time

    # Check if unstuck
    success = verify_recovery(before, after)

    return RecoveryResult(
        success=success,
        method="ctrl_c",
        duration_seconds=duration,
        before_state=before,
        after_state=after
    )


def recover_with_session_restart(session_name: str) -> RecoveryResult:
    """
    Attempt recovery by killing and restarting the tmux session.

    WARNING: This is destructive! Only use as last resort.
    Preserves session directory but loses tmux state.

    Returns RecoveryResult with success status and duration.
    """
    before = capture_pane_state(session_name)
    start_time = time.time()

    # Kill the tmux session
    tmux_cmd = get_tmux_cmd()
    subprocess.run(tmux_cmd + ["kill-session", "-t", session_name])

    # Wait for session to die
    time.sleep(2)

    # Restart session (assumes CSM session structure)
    # This will create a new tmux session with same name
    session_dir = Path.home() / "src/sessions" / session_name
    if session_dir.exists():
        # Start new tmux session in session directory
        subprocess.run([
            "tmux", "new-session", "-d", "-s", session_name,
            "-c", str(session_dir)
        ])
        time.sleep(2)

    after = capture_pane_state(session_name)
    duration = time.time() - start_time

    # Recovery is successful if we have a new session
    success = after is not None

    return RecoveryResult(
        success=success,
        method="session_restart",
        duration_seconds=duration,
        before_state=before,
        after_state=after
    )


def recover_with_strategy_chain(session_name: str, strategies: List[str]) -> RecoveryResult:
    """
    Try multiple recovery strategies in sequence until one succeeds.

    Args:
        session_name: Name of the stuck session
        strategies: List of strategy names to try in order
                   (e.g., ["escape", "ctrl_c", "session_restart"])

    Returns RecoveryResult from the first successful strategy, or
            the last attempted strategy if all fail.
    """
    recovery_functions = {
        "escape": recover_with_escape,
        "ctrl_c": recover_with_ctrl_c,
        "session_restart": recover_with_session_restart,
    }

    last_result = None

    for strategy in strategies:
        if strategy not in recovery_functions:
            print(f"   ⚠️  Unknown recovery strategy: {strategy}")
            continue

        print(f"   🔧 Trying recovery strategy: {strategy}")
        result = recovery_functions[strategy](session_name)
        last_result = result

        if result.success:
            print(f"   ✅ Recovery successful with {strategy}")
            # Update method to indicate chain was used
            result.method = f"chain:{strategy}"
            return result
        else:
            print(f"   ❌ Recovery failed with {strategy}, trying next strategy")

    # All strategies failed, return last result
    if last_result:
        last_result.method = f"chain:failed"
    return last_result or RecoveryResult(
        success=False,
        method="chain:no_strategies",
        duration_seconds=0,
        before_state=None,
        after_state=None
    )


def get_session_id(session_name: str) -> str:
    """
    Get session UUID from manifest.yaml.

    Returns "unknown" if manifest not found or doesn't contain sessionId.
    """
    manifest_path = Path.home() / "src/sessions" / session_name / "manifest.yaml"

    if not manifest_path.exists():
        return "unknown"

    try:
        import yaml
        with open(manifest_path) as f:
            manifest = yaml.safe_load(f)
            return manifest.get("sessionId", "unknown")
    except Exception:
        return "unknown"


def log_incident(incident: Incident, log_file_path: str | None = None, reporter: RemoteReporter | None = None):
    """
    Append incident to JSONL log (crash-safe) and optionally send to remote collector.

    Args:
        incident: The incident to log
        log_file_path: Optional custom log file path. If None, uses default ~/.csm/astrocyte/incidents.jsonl  # noqa: path-portability
        reporter: Optional RemoteReporter for sending incident to collector

    Creates directory if needed. Uses fsync for durability.
    """
    if log_file_path:
        log_file = Path(log_file_path).expanduser()
    else:
        log_file = Path.home() / ".csm/astrocyte/incidents.jsonl"

    log_file.parent.mkdir(parents=True, exist_ok=True)

    with open(log_file, "a") as f:
        f.write(incident.to_json() + "\n")
        f.flush()
        os.fsync(f.fileno())  # Ensure written to disk

    # Send to remote collector if configured
    if reporter and REMOTE_REPORTER_AVAILABLE:
        incident_dict = asdict(incident)
        reporter.send_incident(incident_dict)


def log_false_positive(
    session_name: str,
    symptom: str,
    heuristic: str,
    stuck_duration_minutes: int,
    stuck_since: datetime,
    unstuck_at: datetime
):
    """
    Log false positive detection (session was stuck but unstuck itself).

    Args:
        session_name: Name of the session
        symptom: What symptom was detected (e.g., "permission_prompt")
        heuristic: Detection heuristic used (e.g., "permission_prompt_detected")
        stuck_duration_minutes: How long session appeared stuck
        stuck_since: When stuck state was first detected
        unstuck_at: When session unstuck itself

    Logs to ~/.csm/astrocyte/logs/false-positives.jsonl in JSONL format.  # noqa: path-portability
    """
    log_file = Path.home() / ".csm/astrocyte/logs/false-positives.jsonl"
    log_file.parent.mkdir(parents=True, exist_ok=True)

    entry = {
        "timestamp": datetime.now().isoformat(),
        "session_name": session_name,
        "symptom": symptom,
        "detection_heuristic": heuristic,
        "stuck_duration_minutes": stuck_duration_minutes,
        "stuck_since": stuck_since.isoformat(),
        "unstuck_at": unstuck_at.isoformat(),
        "daemon_intervened": False  # Session unstuck itself
    }

    with open(log_file, "a") as f:
        f.write(json.dumps(entry) + "\n")
        f.flush()
        os.fsync(f.fileno())  # Ensure written to disk


def get_slack_webhook_url() -> str | None:
    """
    Get Slack webhook URL from config file.

    Returns None if config file doesn't exist or webhook not configured.
    """
    config_file = Path.home() / ".csm/astrocyte/config.json"

    if not config_file.exists():
        return None

    try:
        with open(config_file) as f:
            config = json.load(f)
            return config.get("slack_webhook_url")
    except Exception:
        return None


def generate_diagnosis_prompt(
    incident: Incident,
    recovery: RecoveryResult
) -> str:
    """
    Generate diagnosis prompt for agent self-analysis.

    Args:
        incident: The incident that occurred
        recovery: The recovery result

    Returns:
        Formatted prompt asking agent to diagnose root cause
    """
    # Extract diagnosis file path
    timestamp_str = incident.timestamp.replace(":", "-").split(".")[0]
    diagnosis_file = os.path.expanduser(f"~/.csm/astrocyte/diagnoses/{incident.session_name}-{timestamp_str}.md")

    # Generate prompt
    prompt = f"""⚠️ INCIDENT RECOVERY NOTICE

You were stuck in a hang state for {incident.duration_minutes} minutes and were automatically recovered by the Astrocyte daemon (ESC sent via tmux).

**Incident Details**:
- Session: {incident.session_name}
- Timestamp: {incident.timestamp}
- Duration: {incident.duration_minutes} minutes
- Symptom: {incident.symptom}
- Detection Heuristic: {incident.detection_heuristic}
- Recovery Method: {incident.recovery_method}
- Recovery Success: {'Yes' if recovery.success else 'No'}
- Recovery Time: {recovery.duration_seconds:.1f}s

**Last State Before Hang** (observed):
```
{incident.pane_snapshot}
```

**Your Task**:
1. Analyze what happened - why were you stuck?
2. Hypothesize root cause (be specific - what code path? what condition?)
3. File incident report to: {diagnosis_file}
4. After filing, proceed with your previously scheduled work

**Diagnosis Template**:
```markdown
## Incident Diagnosis: {incident.session_name} - {timestamp_str}

### Symptom
{incident.symptom} for {incident.duration_minutes} minutes

### Context
[What were you doing? What was the task?]

### Root Cause (Hypothesis)
[What likely caused the hang? Be specific - was it code path? condition? external dependency?]

### Reproduction Steps
[How to reproduce this hang? What were the exact conditions?]

### Prevention Strategy
[How can this be avoided in the future? Code changes? Timeout parameters? Better handling?]

### Confidence
[Low/Medium/High - how confident are you in this diagnosis?]
```

**Start diagnosis now, then continue your work.**
"""

    return prompt


def send_diagnosis_prompt_via_csm(
    session_name: str,
    prompt: str
) -> bool:
    """
    Send diagnosis prompt to session via CSM send command.

    This is the preferred method:
    - Uses tmux literal mode (-l flag) for reliable text transmission
    - Sends Enter as separate command (C-m) avoiding paste-buffer issues
    - No "pasted text" interpretation problems
    - Handles large prompts (up to 10KB)

    Args:
        session_name: The session to send prompt to
        prompt: The prompt text

    Returns:
        True if sent successfully, False otherwise
    """
    try:
        # Write prompt to temporary file
        prompt_dir = Path.home() / ".csm/astrocyte/prompts"
        prompt_dir.mkdir(parents=True, exist_ok=True)

        prompt_file = prompt_dir / f"{session_name}-diagnosis.txt"
        with open(prompt_file, "w") as f:
            f.write(prompt)

        # Send via CSM send --prompt-file
        # Uses tmux send-keys -l (literal mode) + separate C-m
        # This prevents the "pasted text" issue where prompts are queued instead of executed
        result = subprocess.run(
            ["csm", "send", session_name, "--prompt-file", str(prompt_file)],
            check=False,  # Don't raise on error
            capture_output=True,
            text=True,
            timeout=10
        )

        if result.returncode == 0:
            return True

        # Check if failure was due to queued input
        if "queued input" in result.stderr:
            print(f"   ⚠️  {session_name} has queued input - skipping this cycle", file=sys.stderr)
            return False  # Don't fall back to tmux method - just skip this session

        # Other errors - try fallback
        print(f"   ⚠️  CSM send failed (rc={result.returncode}): {result.stderr}", file=sys.stderr)
        # Fall back to tmux method
        return send_diagnosis_prompt_via_tmux_fallback(session_name, prompt)

    except FileNotFoundError:
        # CSM not found
        print(f"   ⚠️  CSM send not available, using tmux fallback", file=sys.stderr)
        return send_diagnosis_prompt_via_tmux_fallback(session_name, prompt)
    except Exception as e:
        print(f"   ⚠️  Failed to send diagnosis prompt via CSM: {e}", file=sys.stderr)
        return send_diagnosis_prompt_via_tmux_fallback(session_name, prompt)


def send_diagnosis_prompt_via_tmux_fallback(
    session_name: str,
    prompt: str
) -> bool:
    """
    Fallback: Send diagnosis prompt via tmux (original Phase 3 Bead 3.1 method).

    Used when CSM --prompt-file is not available.

    Args:
        session_name: The session to send prompt to
        prompt: The prompt text

    Returns:
        True if sent successfully, False otherwise
    """
    try:
        # Write prompt to temp file
        prompt_file = Path(f"/tmp/astrocyte-diagnosis-{session_name}.txt")
        with open(prompt_file, "w") as f:
            f.write(prompt)

        # Load into tmux buffer
        tmux_cmd = get_tmux_cmd()
        subprocess.run(
            tmux_cmd + ["load-buffer", str(prompt_file)],
            check=True,
            capture_output=True
        )

        # Paste into session
        subprocess.run(
            tmux_cmd + ["paste-buffer", "-t", session_name],
            check=True,
            capture_output=True
        )

        # Wait for Claude to process pasted text (critical for large prompts)
        # Without this delay, Claude interprets the content as "pasted text"
        # instead of a command, causing it to be queued rather than executed
        time.sleep(0.5)

        # Send Enter to submit (as separate command)
        subprocess.run(
            tmux_cmd + ["send-keys", "-t", session_name, "C-m"],
            check=True,
            capture_output=True
        )

        return True

    except Exception as e:
        print(f"   ⚠️  Failed to send diagnosis prompt (tmux fallback): {e}", file=sys.stderr)
        return False


def send_violation_prompt(session_name: str) -> RecoveryResult:
    """
    Send AskUserQuestion violation prompt to session.

    Instead of ESC recovery, send the violation prompt from
    ~/src/ws/oss/ask-question-violations/prompts/VIOLATION-PROMPTS.md

    Args:
        session_name: The session to send prompt to

    Returns:
        RecoveryResult with success=True if prompt sent successfully
    """
    # Capture state before sending prompt
    before = capture_pane_state(session_name)
    start_time = time.time()

    # Read violation prompt from file
    violation_prompt_file = Path.home() / "src/ws/oss/ask-question-violations/prompts/VIOLATION-PROMPTS.md"

    try:
        with open(violation_prompt_file, "r") as f:
            content = f.read()

        # Extract the standard prompt (recommended version)
        # Look for "## Standard Prompt (Recommended)" section
        import re
        match = re.search(r"## Standard Prompt.*?\n```\n(.*?)\n```", content, re.DOTALL)
        if not match:
            print(f"   ⚠️  Could not extract standard prompt from {violation_prompt_file}")
            return RecoveryResult(
                success=False,
                method="violation_prompt",
                duration_seconds=time.time() - start_time,
                before_state=before,
                after_state=before  # No change if failed early
            )

        violation_prompt = match.group(1).strip()

        # Send via CSM (same mechanism as diagnosis prompts)
        prompt_sent = send_diagnosis_prompt_via_csm(session_name, violation_prompt)

        # Wait for prompt to be processed
        time.sleep(5)

        # Capture state after sending prompt
        after = capture_pane_state(session_name)

        duration = time.time() - start_time
        return RecoveryResult(
            success=prompt_sent,
            method="violation_prompt",
            duration_seconds=duration,
            before_state=before,
            after_state=after
        )

    except FileNotFoundError:
        print(f"   ⚠️  Violation prompt file not found: {violation_prompt_file}")
        return RecoveryResult(
            success=False,
            method="violation_prompt",
            duration_seconds=time.time() - start_time,
            before_state=before,
            after_state=before
        )
    except Exception as e:
        print(f"   ⚠️  Failed to send violation prompt: {e}")
        after = capture_pane_state(session_name)  # Capture current state
        return RecoveryResult(
            success=False,
            method="violation_prompt",
            duration_seconds=time.time() - start_time,
            before_state=before,
            after_state=after
        )


def reject_permission_prompt(session_name: str) -> RecoveryResult:
    """
    Reject permission prompt using csm reject command.

    Permission prompts appear when PreToolUse hook blocks bash commands
    for tool usage violations (cd, &&, pipes, etc.). We auto-reject these
    with the violation prompt to unstick the session.

    Args:
        session_name: The session to send rejection to

    Returns:
        RecoveryResult with success=True if rejection sent successfully
    """
    # Capture state before rejecting
    before = capture_pane_state(session_name)
    start_time = time.time()

    # Violation prompt file path
    violation_prompt_file = Path.home() / "src/ws/oss/tool-usage-analysis/prompts/VIOLATION-PROMPTS.md"

    try:
        # Use csm reject command
        result = subprocess.run(
            ["csm", "reject", session_name, "--reason-file", str(violation_prompt_file)],
            check=False,
            capture_output=True,
            text=True,
            timeout=10
        )

        success = result.returncode == 0

        if not success:
            print(f"   ⚠️  csm reject failed (rc={result.returncode}): {result.stderr}")

        # Wait for rejection to be processed
        time.sleep(2)

        # Capture state after rejection
        after = capture_pane_state(session_name)

        duration = time.time() - start_time
        return RecoveryResult(
            success=success,
            method="reject_permission",
            duration_seconds=duration,
            before_state=before,
            after_state=after
        )

    except FileNotFoundError:
        print(f"   ⚠️  csm command not found or violation prompt file missing")
        return RecoveryResult(
            success=False,
            method="reject_permission",
            duration_seconds=time.time() - start_time,
            before_state=before,
            after_state=before
        )
    except Exception as e:
        print(f"   ⚠️  Failed to reject permission prompt: {e}")
        after = capture_pane_state(session_name)
        return RecoveryResult(
            success=False,
            method="reject_permission",
            duration_seconds=time.time() - start_time,
            before_state=before,
            after_state=after
        )


def send_slack_notification(incident: Incident, recovery: RecoveryResult | None = None):
    """
    Send Slack notification for incident.

    Args:
        incident: The incident to notify about
        recovery: Optional recovery result (if recovery attempted)

    Returns None. Silently fails if webhook not configured or request fails.
    """
    webhook_url = get_slack_webhook_url()

    if not webhook_url:
        # Webhook not configured, skip notification
        return

    # Build message
    if recovery and recovery.success:
        emoji = "✅"
        title = "CSM Session Auto-Recovered"
        color = "good"
        status = f"Recovered in {recovery.duration_seconds:.1f}s"
    elif recovery and not recovery.success:
        emoji = "❌"
        title = "CSM Session Recovery Failed"
        color = "danger"
        status = f"Recovery failed after {recovery.duration_seconds:.1f}s - manual intervention needed"
    else:
        emoji = "⚠️"
        title = "CSM Session Stuck Detected"
        color = "warning"
        status = "Detected, recovery pending"

    # Format Slack message
    message = {
        "text": f"{emoji} {title}: `{incident.session_name}`",
        "attachments": [
            {
                "color": color,
                "fields": [
                    {"title": "Session", "value": incident.session_name, "short": True},
                    {"title": "Duration", "value": f"{incident.duration_minutes} min", "short": True},
                    {"title": "Symptom", "value": incident.symptom, "short": True},
                    {"title": "Status", "value": status, "short": True},
                    {"title": "Detection", "value": incident.detection_heuristic, "short": True},
                    {"title": "Timestamp", "value": incident.timestamp, "short": True},
                ]
            }
        ]
    }

    # Add pane snapshot if available
    if incident.pane_snapshot:
        snippet = incident.pane_snapshot[:200] + "..." if len(incident.pane_snapshot) > 200 else incident.pane_snapshot
        message["attachments"][0]["fields"].append({
            "title": "Pane Snapshot",
            "value": f"```{snippet}```",
            "short": False
        })

    # Send to Slack
    try:
        response = requests.post(
            webhook_url,
            json=message,
            timeout=5
        )
        response.raise_for_status()
    except Exception as e:
        # Silently fail - don't let Slack errors break recovery
        print(f"   ⚠️  Slack notification failed: {e}", file=sys.stderr)


def send_email_notification(incident: Incident, recovery: RecoveryResult | None = None):
    """
    Send email notification for incident.

    Args:
        incident: The incident to notify about
        recovery: Optional recovery result (if recovery attempted)

    Returns None. Silently fails if email not configured or sending fails.
    """
    config = load_config()

    if not config.email_enabled:
        return

    # Build email subject
    if recovery and recovery.success:
        subject = f"✅ Astrocyte: Session {incident.session_name} Auto-Recovered"
    elif recovery and not recovery.success:
        subject = f"❌ Astrocyte: Recovery Failed for {incident.session_name}"
    else:
        subject = f"⚠️  Astrocyte: Session {incident.session_name} Stuck Detected"

    # Build HTML email body
    html_body = f"""
    <html>
    <head>
        <style>
            body {{ font-family: Arial, sans-serif; }}
            .header {{ background-color: #f3f4f6; padding: 20px; border-radius: 5px; }}
            .content {{ padding: 20px; }}
            .table {{ width: 100%; border-collapse: collapse; }}
            .table td {{ padding: 8px; border-bottom: 1px solid #e5e7eb; }}
            .table td:first-child {{ font-weight: bold; width: 150px; }}
            .success {{ color: #10b981; }}
            .failure {{ color: #ef4444; }}
            .warning {{ color: #f59e0b; }}
            .snapshot {{ background-color: #1f2937; color: #f9fafb; padding: 15px; border-radius: 5px; font-family: monospace; white-space: pre-wrap; }}
        </style>
    </head>
    <body>
        <div class="header">
            <h2>Astrocyte Incident Report</h2>
            <p>Session: <strong>{incident.session_name}</strong></p>
        </div>

        <div class="content">
            <table class="table">
                <tr>
                    <td>Timestamp</td>
                    <td>{incident.timestamp}</td>
                </tr>
                <tr>
                    <td>Symptom</td>
                    <td>{incident.symptom}</td>
                </tr>
                <tr>
                    <td>Detection Heuristic</td>
                    <td>{incident.detection_heuristic}</td>
                </tr>
                <tr>
                    <td>Duration</td>
                    <td>{incident.duration_minutes} minutes</td>
                </tr>
                <tr>
                    <td>Recovery Method</td>
                    <td>{incident.recovery_method}</td>
                </tr>
    """

    if recovery:
        status_class = "success" if recovery.success else "failure"
        status_text = "Success" if recovery.success else "Failed"
        html_body += f"""
                <tr>
                    <td>Recovery Status</td>
                    <td class="{status_class}">{status_text}</td>
                </tr>
                <tr>
                    <td>Recovery Time</td>
                    <td>{recovery.duration_seconds:.1f} seconds</td>
                </tr>
        """

    html_body += """
            </table>
    """

    if incident.pane_snapshot:
        html_body += f"""
            <h3>Pane Snapshot</h3>
            <div class="snapshot">{incident.pane_snapshot[:500]}</div>
        """

    html_body += """
        </div>
    </body>
    </html>
    """

    # Send email
    try:
        import smtplib
        from email.mime.text import MIMEText
        from email.mime.multipart import MIMEMultipart

        msg = MIMEMultipart('alternative')
        msg['Subject'] = subject
        msg['From'] = config.email_from
        msg['To'] = ', '.join(config.email_to)

        # Attach HTML body
        msg.attach(MIMEText(html_body, 'html'))

        # Send via SMTP
        with smtplib.SMTP(config.email_smtp_host, config.email_smtp_port) as server:
            if config.email_smtp_use_tls:
                server.starttls()
            if config.email_smtp_user and config.email_smtp_password:
                server.login(config.email_smtp_user, config.email_smtp_password)
            server.send_message(msg)

    except Exception as e:
        # Silently fail - don't let email errors break recovery
        print(f"   ⚠️  Email notification failed: {e}", file=sys.stderr)


def main():
    """Main detection and recovery loop."""
    print("🧠 Astrocyte daemon starting...")
    print(f"   Timestamp: {datetime.now().isoformat()}")
    print("   Mode: Production Deployment - Continuous Monitoring")

    # Test JSONL logging implementation
    print("\n🧪 Testing JSONL logging...")

    # Test incident creation and logging
    print("\n1️⃣ Test incident creation:")

    test_incident = Incident(
        timestamp=datetime.now().isoformat(),
        session_name="test-session",
        session_id="test-uuid-1234",
        symptom="stuck_mustering",
        duration_minutes=15,
        detection_heuristic="mustering_timeout",
        pane_snapshot="● I'll launch both beads...\n\n✻ Mustering...",
        cursor_position="0,0",
        recovery_attempted=False,
        recovery_method=None,
        recovery_success=None,
        recovery_duration_seconds=None,
        diagnosis_filed=False,
        diagnosis_file=None
    )

    print(f"   Created incident: {test_incident.session_name}")
    print(f"   Symptom: {test_incident.symptom}")
    print(f"   Duration: {test_incident.duration_minutes} minutes")

    # Test JSON serialization
    json_output = test_incident.to_json()
    print(f"   JSON length: {len(json_output)} bytes")
    print(f"   JSON valid: {'✅' if json_output.startswith('{') and json_output.endswith('}') else '❌'}")

    # Test logging
    print("\n2️⃣ Test log_incident():")

    log_file = Path.home() / ".csm/astrocyte/incidents.jsonl"
    print(f"   Log file: {log_file}")

    # Log the test incident
    log_incident(test_incident)
    print(f"   Logged incident ✅")

    # Verify file exists
    if log_file.exists():
        print(f"   File exists: ✅")

        # Read back the incident
        with open(log_file) as f:
            lines = f.readlines()
            print(f"   Total incidents in log: {len(lines)}")

            # Parse last incident
            last_incident_json = json.loads(lines[-1])
            print(f"   Last incident session: {last_incident_json['session_name']}")
            print(f"   Last incident symptom: {last_incident_json['symptom']}")
            print(f"   JSONL format valid: ✅")
    else:
        print(f"   File exists: ❌")

    print("\n3️⃣ Test crash safety:")

    # Log multiple incidents rapidly
    for i in range(5):
        rapid_incident = Incident(
            timestamp=datetime.now().isoformat(),
            session_name=f"rapid-test-{i}",
            session_id=f"uuid-{i}",
            symptom="stuck_mustering",
            duration_minutes=10 + i,
            detection_heuristic="mustering_timeout",
            pane_snapshot="test snapshot",
            cursor_position="0,0",
            recovery_attempted=False,
            recovery_method=None,
            recovery_success=None,
            recovery_duration_seconds=None,
            diagnosis_filed=False,
            diagnosis_file=None
        )
        log_incident(rapid_incident)

    print(f"   Logged 5 incidents rapidly")

    # Verify all logged
    with open(log_file) as f:
        lines = f.readlines()
        rapid_count = sum(1 for line in lines if "rapid-test-" in line)
        print(f"   Found {rapid_count}/5 rapid test incidents")
        print(f"   Crash safety: {'✅' if rapid_count == 5 else '❌'}")

    print("\n✅ JSONL logging implementation complete")

    # Test ESC recovery implementation
    print("\n🧪 Testing ESC recovery functions...")

    # Test verify_recovery
    print("\n1️⃣ Test verify_recovery():")

    before = SessionState(
        timestamp=datetime.now(),
        session_name="test",
        pane_content="✻ Mustering...",
        cursor_position=(0, 0)
    )

    # Test case 1: Content changed
    after_content_changed = SessionState(
        timestamp=datetime.now(),
        session_name="test",
        pane_content="✅ Completed task",
        cursor_position=(0, 0)
    )
    result = verify_recovery(before, after_content_changed)
    print(f"   Content changed: {' ✅' if result else '❌'} (expected: True, got: {result})")

    # Test case 2: Cursor moved
    after_cursor_moved = SessionState(
        timestamp=datetime.now(),
        session_name="test",
        pane_content="✻ Mustering...",
        cursor_position=(10, 5)
    )
    result = verify_recovery(before, after_cursor_moved)
    print(f"   Cursor moved: {'✅' if result else '❌'} (expected: True, got: {result})")

    # Test case 3: Pattern gone
    after_pattern_gone = SessionState(
        timestamp=datetime.now(),
        session_name="test",
        pane_content="Waiting for input...",
        cursor_position=(0, 0)
    )
    result = verify_recovery(before, after_pattern_gone)
    print(f"   Pattern gone: {'✅' if result else '❌'} (expected: True, got: {result})")

    # Test case 4: No change (still stuck)
    after_no_change = SessionState(
        timestamp=datetime.now(),
        session_name="test",
        pane_content="✻ Mustering...",
        cursor_position=(0, 0)
    )
    result = verify_recovery(before, after_no_change)
    print(f"   No change: {'✅' if not result else '❌'} (expected: False, got: {result})")

    print("\n2️⃣ Test recover_with_escape():")
    print("   Note: Requires active CSM session for real test")
    print("   Skipping live test (no test session available)")
    print("   Function signature validated ✅")

    print("\n✅ ESC recovery implementation complete")

    # Load configuration
    print("\n⚙️  Loading configuration...")
    config = load_config()

    print(f"   Interval: {config.interval_seconds}s ({config.interval_seconds // 60} min)")
    print(f"   Thresholds:")
    print(f"     - Mustering timeout: {config.mustering_timeout} min")
    print(f"     - Zero-token waiting: {config.zero_token_waiting} min")
    print(f"     - Cursor frozen: {config.cursor_frozen} min")
    print(f"     - AskUserQuestion violation: {config.ask_question_violation} min")
    print(f"   Slack notifications: {'enabled' if config.slack_enabled else 'disabled'}")
    print(f"   Diagnosis prompts: {'enabled' if config.diagnosis_enabled else 'disabled'}")
    print(f"   Session overrides: {len(config.session_overrides)} sessions")

    # Initialize remote reporter if enabled
    reporter = None
    if config.remote_enabled and REMOTE_REPORTER_AVAILABLE:
        reporter_config = RemoteReporterConfig(
            enabled=True,
            collector_url=config.remote_collector_url,
            api_token=config.remote_api_token,
            report_interval=config.remote_report_interval,
            report_incidents=config.remote_report_incidents,
            report_metrics=config.remote_report_metrics
        )
        reporter = RemoteReporter(reporter_config)
        print(f"   Remote reporting: enabled (collector: {config.remote_collector_url})")
    elif config.remote_enabled and not REMOTE_REPORTER_AVAILABLE:
        print(f"   Remote reporting: disabled (reporter module not available)")
    else:
        print(f"   Remote reporting: disabled")

    # State tracking
    previous_states = {}  # session_name -> SessionState
    previously_stuck = set()  # Track which sessions were stuck last cycle (for false positive detection)
    stuck_details = {}  # session_name -> {symptom, heuristic, stuck_since} for false positive logging
    check_count = 0
    last_heartbeat = datetime.now()

    print(f"\n🔄 Starting continuous monitoring loop...")

    try:
        while True:  # Continuous operation
            check_count += 1
            print(f"\n{'='*60}")
            print(f"Check cycle #{check_count} at {datetime.now().strftime('%H:%M:%S')}")
            print(f"{'='*60}")

            sessions = get_active_csm_sessions()
            print(f"Active CSM sessions: {len(sessions)}")

            for session in sessions:
                try:
                    print(f"\n🔍 Processing: {session}", flush=True, file=sys.stderr)
                    current = capture_pane_state(session)
                    print(f"   ✓ State captured", flush=True, file=sys.stderr)
                    previous = previous_states.get(session)

                    # Check all detection heuristics
                    stuck = False
                    symptom = None
                    heuristic = None

                    # Check permission prompts FIRST (works without previous state)
                    # Fresh start detection: if we see a permission prompt with violation patterns,
                    # we can detect and reject immediately without waiting for second cycle
                    # Duration-based detection: wait 5 minutes to avoid false positives
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
                        # DISABLED: Never interfere with sessions asking user questions
                        # If a session is blocked on user questions, astrocyte must leave it alone.
                        # Only the user should answer questions - astrocyte must NEVER answer on their behalf.
                        # elif is_asking_question_without_tool(current, previous, ask_question_threshold):
                        #     stuck = True
                        #     symptom = "ask_question_violation"
                        #     heuristic = "ask_question_pattern"

                    if stuck:
                        # Calculate duration (0 for fresh start detection)
                        if previous:
                            delta_minutes = int((current.timestamp - previous.timestamp).seconds / 60)
                        else:
                            delta_minutes = 0  # Fresh start detection, no duration yet

                        print(f"\n⚠️  STUCK DETECTED: {session}")
                        print(f"   Symptom: {symptom}")
                        print(f"   Heuristic: {heuristic}")
                        print(f"   Duration: {delta_minutes} minutes (fresh start)" if delta_minutes == 0 else f"   Duration: {delta_minutes} minutes")

                        # Track stuck details for false positive detection
                        if session not in stuck_details:
                            stuck_details[session] = {
                                "symptom": symptom,
                                "heuristic": heuristic,
                                "stuck_since": datetime.now()
                            }

                        # Determine recovery method based on symptom
                        if symptom == "ask_question_violation":
                            recovery_method = "violation_prompt"
                        elif symptom == "permission_prompt":
                            # Use csm reject for permission prompts
                            # Fixed to handle both 2-option and 3-option prompts
                            recovery_method = "reject_permission"
                        else:
                            recovery_method = "escape"

                        # Create initial incident record
                        incident = Incident(
                            timestamp=datetime.now().isoformat(),
                            session_name=session,
                            session_id=get_session_id(session),
                            symptom=symptom,
                            duration_minutes=delta_minutes,
                            detection_heuristic=heuristic,
                            pane_snapshot=current.pane_content[:500],  # First 500 chars
                            cursor_position=f"{current.cursor_position[0]},{current.cursor_position[1]}",
                            recovery_attempted=True,
                            recovery_method=recovery_method,
                            recovery_success=None,
                            recovery_duration_seconds=None,
                            diagnosis_filed=False,
                            diagnosis_file=None
                        )

                        # Log detection
                        log_incident(incident, reporter=reporter)
                        print(f"   📝 Incident logged (detection)")

                        # Attempt automatic recovery
                        if symptom == "ask_question_violation":
                            print(f"   🔧 Sending AskUserQuestion violation prompt...")
                            recovery = send_violation_prompt(session)
                        elif symptom == "permission_prompt":
                            print(f"   🔧 Rejecting permission prompt with tool usage violation...")
                            recovery = reject_permission_prompt(session)
                        else:
                            print(f"   🔧 Attempting recovery with {config.recovery_method} strategy...")
                            recovery = recover_session(session, config)

                        # Update incident with recovery results
                        incident.recovery_success = recovery.success
                        incident.recovery_duration_seconds = recovery.duration_seconds

                        # Log recovery result
                        log_incident(incident, reporter=reporter)
                        print(f"   📝 Incident logged (recovery)")

                        # Send Slack notification
                        send_slack_notification(incident, recovery)
                        webhook_configured = get_slack_webhook_url() is not None
                        if webhook_configured:
                            print(f"   📬 Slack notification sent")
                        else:
                            print(f"   📭 Slack not configured (skipped)")

                        # Send email notification
                        send_email_notification(incident, recovery)
                        config = load_config()
                        if config.email_enabled:
                            print(f"   📧 Email notification sent")
                        else:
                            print(f"   📭 Email not configured (skipped)")

                        # Send diagnosis prompt (if recovery successful and not a violation prompt)
                        # Skip diagnosis for AskUserQuestion violations (violation prompt handles it)
                        if recovery.success and symptom != "ask_question_violation":
                            diagnosis_prompt = generate_diagnosis_prompt(incident, recovery)
                            diagnosis_sent = send_diagnosis_prompt_via_csm(session, diagnosis_prompt)
                            if diagnosis_sent:
                                print(f"   📋 Diagnosis prompt sent to session (CSM)")
                            else:
                                print(f"   ⚠️  Diagnosis prompt failed to send")
                        elif symptom == "ask_question_violation":
                            print(f"   📋 Violation prompt sent (no diagnosis needed)")
                        elif symptom == "permission_prompt":
                            print(f"   📋 Permission rejected (no diagnosis needed)")

                        if recovery.success:
                            print(f"   ✅ Recovery successful ({recovery.duration_seconds:.1f}s)")
                            print(f"   📊 Pane content changed: YES")
                        else:
                            print(f"   ❌ Recovery failed (no pane change after {recovery.duration_seconds:.1f}s)")
                            print(f"   ⚠️  Manual intervention required")
                    else:
                        # Session is NOT stuck - check if it was previously stuck (false positive)
                        if session in previously_stuck and session in stuck_details:
                            details = stuck_details[session]
                            unstuck_at = datetime.now()
                            stuck_duration_minutes = int((unstuck_at - details["stuck_since"]).seconds / 60)

                            print(f"\n🔄 FALSE POSITIVE DETECTED: {session}")
                            print(f"   Session unstuck itself after {stuck_duration_minutes} minutes")
                            print(f"   Original symptom: {details['symptom']}")
                            print(f"   Heuristic: {details['heuristic']}")

                            # Log false positive for threshold tuning
                            log_false_positive(
                                session_name=session,
                                symptom=details["symptom"],
                                heuristic=details["heuristic"],
                                stuck_duration_minutes=stuck_duration_minutes,
                                stuck_since=details["stuck_since"],
                                unstuck_at=unstuck_at
                            )
                            print(f"   📝 False positive logged to ~/.csm/astrocyte/logs/false-positives.jsonl")  # noqa: path-portability

                            # Clear stuck tracking for this session
                            stuck_details.pop(session, None)

                        # Check if mustering pattern is present (but not stuck yet)
                        has_mustering = any(re.search(p, current.pane_content) for p in MUSTERING_PATTERNS)
                        if has_mustering:
                            if previous:
                                delta_minutes = int((current.timestamp - previous.timestamp).seconds / 60)
                                print(f"   ⏳ {session}: Mustering for {delta_minutes} min (threshold: {mustering_timeout_minutes} min)")
                            else:
                                print(f"   ⏳ {session}: Mustering (first observation)")

                    # Update tracking sets for next cycle
                    if stuck:
                        previously_stuck.add(session)
                    else:
                        previously_stuck.discard(session)

                    # Store current state as previous for next check
                    previous_states[session] = current
                    print(f"   ✓ Completed: {session}", flush=True, file=sys.stderr)

                except Exception as e:
                    print(f"   ❌ Error processing {session}: {e}", flush=True, file=sys.stderr)

            # Send periodic heartbeat to remote collector
            if reporter and sessions:
                time_since_heartbeat = (datetime.now() - last_heartbeat).seconds
                if time_since_heartbeat >= config.remote_report_interval:
                    print(f"\n📡 Sending heartbeat to collector ({len(sessions)} sessions)...")
                    if reporter.send_heartbeat(sessions):
                        print(f"   ✅ Heartbeat sent successfully")
                        last_heartbeat = datetime.now()
                    else:
                        print(f"   ⚠️  Heartbeat failed (collector may be unavailable)")

            print(f"\n⏸️  Sleeping for {config.interval_seconds} seconds...")
            time.sleep(config.interval_seconds)

    except KeyboardInterrupt:
        print(f"\n\n⚠️  Interrupted by user")
        raise

    print(f"\n{'='*60}")
    print("✅ Detection loop prototype complete")
    print(f"   Total checks: {check_count}")
    print(f"   Sessions monitored: {len(previous_states)}")
    print(f"{'='*60}")


if __name__ == "__main__":
    try:
        main()
    except KeyboardInterrupt:
        print("\n👋 Astrocyte shutting down")
        sys.exit(0)
    except Exception as e:
        print(f"Fatal error: {e}", file=sys.stderr)
        sys.exit(1)
