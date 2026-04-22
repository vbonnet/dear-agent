"""
PluginsList tool — scans engram plugin directories and returns plugin metadata.
"""

import logging
from pathlib import Path
from typing import Any, Dict, List

logger = logging.getLogger("engram-mcp-server.tools.plugins_list")

# Attempt yaml import; fall back to manual parsing if unavailable
try:
    import yaml as _yaml  # type: ignore

    def _load_yaml(text: str) -> Dict:
        return _yaml.safe_load(text) or {}

except ImportError:
    _yaml = None  # type: ignore

    def _load_yaml(text: str) -> Dict:  # type: ignore[misc]
        """Minimal YAML key: value parser (no nesting needed for plugin.yaml)."""
        result: Dict[str, str] = {}
        for line in text.splitlines():
            if ":" in line and not line.strip().startswith("#"):
                key, _, val = line.partition(":")
                result[key.strip()] = val.strip().strip('"').strip("'")
        return result


class PluginsList:
    """List installed Engram plugins."""

    # Standard plugin search paths relative to ~/.engram
    _PLUGIN_DIRS = [
        Path.home() / ".engram" / "core" / "plugins",
        Path.home() / ".engram" / "user" / "plugins",
    ]

    def __init__(self, engram_root: Path):
        self.engram_root = Path(engram_root)

    def list_plugins(self) -> Dict[str, Any]:
        """Scan plugin directories and return plugin metadata.

        Returns:
            Dict with keys: plugins (list), count, searched_paths.
        """
        plugins: List[Dict[str, str]] = []
        searched: List[str] = []

        for base in self._PLUGIN_DIRS:
            searched.append(str(base))
            location = "core" if "core" in base.parts else "user"

            if not base.exists():
                logger.debug("Plugin directory not found: %s", base)
                continue

            for plugin_dir in sorted(base.iterdir()):
                if not plugin_dir.is_dir():
                    continue

                plugin_yaml = plugin_dir / "plugin.yaml"
                if not plugin_yaml.exists():
                    logger.debug("No plugin.yaml in %s", plugin_dir)
                    continue

                try:
                    meta = _load_yaml(plugin_yaml.read_text(encoding="utf-8"))
                except Exception as exc:
                    logger.warning("Could not read %s: %s", plugin_yaml, exc)
                    continue

                plugins.append({
                    "name": meta.get("name", plugin_dir.name),
                    "type": meta.get("type", meta.get("pattern", "unknown")),
                    "description": meta.get("description", ""),
                    "version": meta.get("version", ""),
                    "location": location,
                    "path": str(plugin_dir),
                })

        return {
            "plugins": plugins,
            "count": len(plugins),
            "searched_paths": searched,
        }
