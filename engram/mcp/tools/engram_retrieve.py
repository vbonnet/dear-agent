"""
EngramRetrieve tool — wraps the engram retrieve CLI.
"""

import json
import logging
import shutil
import subprocess
from pathlib import Path
from typing import Any, Dict, List

logger = logging.getLogger("engram-mcp-server.tools.engram_retrieve")

_ENGRAM_CLI = shutil.which("engram") or str(Path.home() / "go/bin/engram")


class EngramRetrieve:
    """Retrieve engrams via the engram CLI."""

    def __init__(self, engram_root: Path):
        self.engram_root = Path(engram_root)

    def retrieve(
        self,
        query: str,
        type_filter: str = "all",
        top_k: int = 5,
    ) -> Dict[str, Any]:
        """Run engram retrieve and return structured results.

        Args:
            query: Search query string.
            type_filter: "ai", "why", or "all" (file-extension filter).
            top_k: Maximum number of results to return.

        Returns:
            Dict with keys: query, type_filter, results (list), count.
        """
        cmd = [
            _ENGRAM_CLI,
            "retrieve",
            "--query", query,
            "--limit", str(top_k),
            "--format", "json",
        ]

        logger.debug("Running: %s", " ".join(cmd))

        try:
            proc = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30,
            )
        except FileNotFoundError:
            return _error_result(query, type_filter, f"engram CLI not found: {_ENGRAM_CLI}")
        except subprocess.TimeoutExpired:
            return _error_result(query, type_filter, "engram retrieve timed out after 30s")

        if proc.returncode != 0:
            msg = (proc.stderr or proc.stdout or "unknown error").strip()
            logger.warning("engram retrieve failed (exit %d): %s", proc.returncode, msg)
            return _error_result(query, type_filter, msg)

        # Parse JSON output
        raw = proc.stdout.strip()
        try:
            data = json.loads(raw) if raw else []
        except json.JSONDecodeError:
            # Fall back: treat stdout as plain text, wrap as a single result
            return {
                "query": query,
                "type_filter": type_filter,
                "results": [{"content": raw}],
                "count": 1,
            }

        # Normalise: CLI may return a list or {"results": [...]}
        if isinstance(data, dict):
            items: List[Dict] = data.get("results", [data])
        elif isinstance(data, list):
            items = data
        else:
            items = [{"content": str(data)}]

        # Apply type_filter (ai → .ai.md, why → .why.md)
        if type_filter in ("ai", "why"):
            suffix = f".{type_filter}.md"
            items = [
                item for item in items
                if _matches_suffix(item, suffix)
            ]

        return {
            "query": query,
            "type_filter": type_filter,
            "results": items[:top_k],
            "count": len(items[:top_k]),
        }


def _matches_suffix(item: Dict, suffix: str) -> bool:
    """Return True if the item's file path ends with suffix."""
    for key in ("file", "path", "source", "name"):
        val = item.get(key, "")
        if isinstance(val, str) and val.endswith(suffix):
            return True
    return False


def _error_result(query: str, type_filter: str, message: str) -> Dict[str, Any]:
    return {
        "query": query,
        "type_filter": type_filter,
        "error": message,
        "results": [],
        "count": 0,
    }
