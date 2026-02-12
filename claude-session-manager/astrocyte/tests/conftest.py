"""
Shared pytest fixtures for permission prompt detection tests.
"""

import pytest
from unittest.mock import Mock, patch
from pathlib import Path


@pytest.fixture
def mock_tmux():
    """Mock tmux capture-pane and cursor position subprocess calls."""
    with patch('subprocess.run') as mock_run:
        # Mock needs to return two values:
        # 1. Pane content (from capture-pane)
        # 2. Cursor position (from display-message)
        def side_effect_fn(*args, **kwargs):
            cmd = args[0] if args else []
            if "capture-pane" in cmd:
                # Return pane content (set by test via mock_run.return_value.stdout)
                return mock_run.return_value
            elif "display-message" in cmd:
                # Return cursor position (fixed for all tests)
                return Mock(stdout="0,10", stderr="", returncode=0)
            else:
                return Mock(stdout="", stderr="", returncode=0)

        mock_run.side_effect = side_effect_fn
        # Default return value for pane content (can be overridden in tests)
        mock_run.return_value = Mock(
            stdout="",
            stderr="",
            returncode=0
        )
        yield mock_run


@pytest.fixture
def load_fixture():
    """Load pane content from fixture files."""
    def _load(fixture_name):
        fixture_path = Path(__file__).parent / "fixtures" / f"{fixture_name}.txt"
        if not fixture_path.exists():
            raise FileNotFoundError(f"Fixture file not found: {fixture_path}")
        return fixture_path.read_text()
    return _load


@pytest.fixture
def mock_esc_sender():
    """Mock ESC key sending to track calls (no-op - send_esc_key not used in tests)."""
    calls = []
    yield calls
