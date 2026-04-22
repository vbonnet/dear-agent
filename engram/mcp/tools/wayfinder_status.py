"""
WayfinderStatus tool — reads WAYFINDER-STATUS.md from a project directory.
"""

import logging
import re
from pathlib import Path
from typing import Any, Dict

logger = logging.getLogger("engram-mcp-server.tools.wayfinder_status")

_RE_PHASE = re.compile(r"Current Phase:\s*\*\*(.+?)\*\*")
_RE_PROGRESS = re.compile(r"Progress:\s*(.+?)$", re.MULTILINE)
_RE_STATUS = re.compile(r"Status:\s*(.+?)$", re.MULTILINE)


class WayfinderStatus:
    """Read Wayfinder phase status from a project directory."""

    def get_status(self, project_path: str) -> Dict[str, Any]:
        """Parse WAYFINDER-STATUS.md from project_path.

        Args:
            project_path: Path to the project directory (~ is expanded).

        Returns:
            Dict with keys: project, phase, progress, status, source_file.

        Raises:
            ValueError: If WAYFINDER-STATUS.md is not found.
        """
        path = Path(project_path).expanduser().resolve()

        status_file = path / "WAYFINDER-STATUS.md"
        if not status_file.exists():
            raise ValueError(
                f"WAYFINDER-STATUS.md not found in {path}"
            )

        text = status_file.read_text(encoding="utf-8")

        phase_match = _RE_PHASE.search(text)
        progress_match = _RE_PROGRESS.search(text)
        status_match = _RE_STATUS.search(text)

        return {
            "project": str(path),
            "phase": phase_match.group(1).strip() if phase_match else "Unknown",
            "progress": progress_match.group(1).strip() if progress_match else "Unknown",
            "status": status_match.group(1).strip() if status_match else "Unknown",
            "source_file": str(status_file),
        }
