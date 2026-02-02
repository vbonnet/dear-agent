"""
Shared pytest fixtures for permission prompt detection tests.
"""

import pytest
from unittest.mock import Mock, patch
from pathlib import Path


@pytest.fixture
def mock_tmux():
    """Mock tmux capture-pane subprocess call."""
    with patch('subprocess.run') as mock_run:
        # Default return value (can be overridden in tests)
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
    """Mock ESC key sending to track calls."""
    calls = []

    with patch('astrocyte.send_esc_key') as mock_send:
        mock_send.side_effect = lambda session: calls.append(session)
        yield calls
