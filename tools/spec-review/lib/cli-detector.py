#!/usr/bin/env python3
"""CLI Detection Library for Python
Detects which AI coding assistant CLI is currently running.
Supports: Claude Code, Gemini CLI, OpenCode, Codex
"""

import os
import subprocess
from typing import Optional, Literal

CLIType = Literal["claude-code", "gemini-cli", "opencode", "codex", "unknown"]


def detect_cli() -> CLIType:
    """Detect the current CLI environment.

    Returns:
        CLI type: claude-code, gemini-cli, opencode, codex, or unknown
    """
    # Method 1: Check environment variables (most reliable)
    if os.getenv("CLAUDE_CODE_VERSION"):
        return "claude-code"

    if os.getenv("GEMINI_CLI_VERSION"):
        return "gemini-cli"

    if os.getenv("OPENCODE_VERSION"):
        return "opencode"

    if os.getenv("CODEX_VERSION"):
        return "codex"

    # Method 2: Check for CLI-specific markers
    if os.getenv("ANTHROPIC_API_KEY") and _command_exists("claude"):
        return "claude-code"

    # Method 3: Parent process inspection (fallback)
    try:
        ppid = os.getppid()
        result = subprocess.run(
            ["ps", "-o", "comm=", "-p", str(ppid)],
            capture_output=True,
            text=True,
            timeout=1
        )
        parent_cmd = result.stdout.strip().lower()

        if "claude" in parent_cmd:
            return "claude-code"
        elif "gemini" in parent_cmd:
            return "gemini-cli"
        elif "opencode" in parent_cmd:
            return "opencode"
        elif "codex" in parent_cmd:
            return "codex"
    except (subprocess.SubprocessError, OSError):
        pass

    # Default: unknown
    return "unknown"


def get_cli_version(cli_type: Optional[CLIType] = None) -> str:
    """Get CLI version string.

    Args:
        cli_type: CLI type (auto-detected if None)

    Returns:
        Version string or "unknown"
    """
    if cli_type is None:
        cli_type = detect_cli()

    version_map = {
        "claude-code": "CLAUDE_CODE_VERSION",
        "gemini-cli": "GEMINI_CLI_VERSION",
        "opencode": "OPENCODE_VERSION",
        "codex": "CODEX_VERSION",
    }

    env_var = version_map.get(cli_type)
    if env_var:
        return os.getenv(env_var, "unknown")

    return "unknown"


def cli_supports_feature(feature: str, cli_type: Optional[CLIType] = None) -> bool:
    """Check if current CLI supports a specific feature.

    Args:
        feature: Feature name to check
        cli_type: CLI type (auto-detected if None)

    Returns:
        True if feature is supported, False otherwise
    """
    if cli_type is None:
        cli_type = detect_cli()

    feature_support = {
        "claude-code": {
            "caching", "prompt_caching", "tools", "read_tool",
            "edit_tool", "write_tool", "multimodal"
        },
        "gemini-cli": {
            "batch_mode", "function_calling", "multimodal"
        },
        "opencode": {
            "mcp", "tool_registry"
        },
        "codex": {
            "mcp", "completion_mode"
        },
    }

    supported = feature_support.get(cli_type, set())
    return feature in supported


def _command_exists(command: str) -> bool:
    """Check if a command exists in PATH."""
    try:
        subprocess.run(
            ["which", command],
            capture_output=True,
            check=True,
            timeout=1
        )
        return True
    except (subprocess.SubprocessError, OSError):
        return False


# Module-level CLI type detection
CLI_TYPE: CLIType = detect_cli()


if __name__ == "__main__":
    print(f"Detected CLI: {CLI_TYPE}")
    print(f"Version: {get_cli_version()}")
    print(f"Supports caching: {cli_supports_feature('caching')}")
