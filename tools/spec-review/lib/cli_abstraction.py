#!/usr/bin/env python3
"""CLI Abstraction Layer for Python
Provides unified interface for common operations across different AI coding assistant CLIs.
Supports: Claude Code, Gemini CLI, OpenCode, Codex
"""

import os
import subprocess
from pathlib import Path
from typing import Optional, Any
from .cli_detector import detect_cli, cli_supports_feature, CLIType


class CLIAbstraction:
    """Abstraction layer for CLI operations."""

    def __init__(self, cli_type: Optional[CLIType] = None):
        """Initialize CLI abstraction.

        Args:
            cli_type: CLI type (auto-detected if None)
        """
        self.cli_type = cli_type or detect_cli()

    def read_file(self, file_path: str | Path) -> str:
        """Read file using CLI-specific method.

        Args:
            file_path: Path to file

        Returns:
            File contents
        """
        file_path = str(file_path)

        if self.cli_type == "claude-code":
            # Claude Code has Read tool - output instruction for LLM
            return f"!read {file_path}"
        else:
            # Other CLIs: standard file read
            with open(file_path, 'r', encoding='utf-8') as f:
                return f.read()

    def write_file(self, file_path: str | Path, content: str) -> None:
        """Write file using CLI-specific method.

        Args:
            file_path: Path to file
            content: Content to write
        """
        file_path = str(file_path)

        if self.cli_type == "claude-code":
            # Claude Code has Write tool - output instruction for LLM
            print(f"!write {file_path}")
            print(content)
        else:
            # Other CLIs: standard file write
            with open(file_path, 'w', encoding='utf-8') as f:
                f.write(content)

    def invoke_tool(self, tool_name: str, *args: Any) -> str:
        """Invoke tool using CLI-specific format.

        Args:
            tool_name: Name of tool to invoke
            *args: Tool arguments

        Returns:
            Tool invocation string
        """
        args_str = " ".join(str(arg) for arg in args)

        if self.cli_type == "claude-code":
            # Claude Code: !tool_name arg1 arg2
            return f"!{tool_name} {args_str}"
        elif self.cli_type == "gemini-cli":
            # Gemini CLI: @tool tool_name arg1 arg2
            return f"@tool {tool_name} {args_str}"
        elif self.cli_type in ("opencode", "codex"):
            # OpenCode/Codex: MCP-style invocation
            encoded_args = "&".join(str(arg) for arg in args)
            return f"mcp://{tool_name}?{encoded_args}"
        else:
            # Fallback
            return f"{tool_name} {args_str}"

    def prompt_user(self, message: str) -> str:
        """Prompt user for input using CLI-specific method.

        Args:
            message: Prompt message

        Returns:
            User response
        """
        if self.cli_type == "claude-code":
            # Claude Code: use AskUserQuestion tool
            return f'!AskUserQuestion "{message}"'
        else:
            # Other CLIs: standard input
            return input(message)

    def execute_bash(self, command: str, description: str = "Execute command") -> str:
        """Execute bash command using CLI-specific method.

        Args:
            command: Command to execute
            description: Command description

        Returns:
            Command output or invocation string
        """
        if self.cli_type == "claude-code":
            # Claude Code: use Bash tool
            return f'!bash "{command}" "{description}"'
        else:
            # Other CLIs: direct execution
            result = subprocess.run(
                command,
                shell=True,
                capture_output=True,
                text=True
            )
            return result.stdout

    def cache_prompt(self, cache_key: str, prompt_content: str) -> str:
        """Cache prompt for reuse (Claude Code specific).

        Args:
            cache_key: Cache key
            prompt_content: Prompt content

        Returns:
            Cached or raw prompt
        """
        if cli_supports_feature("caching", self.cli_type):
            # Cache is supported - return cached version
            return f"[CACHE:{cache_key}]{prompt_content}"
        else:
            # No caching support - return prompt as-is
            return prompt_content

    def get_batch_size(self) -> int:
        """Get optimal batch size for CLI.

        Returns:
            Batch size
        """
        batch_sizes = {
            "claude-code": 10,
            "gemini-cli": 20,
            "opencode": 5,
            "codex": 5,
        }
        return batch_sizes.get(self.cli_type, 10)

    def supports_feature(self, feature: str) -> bool:
        """Check if CLI supports a specific feature.

        Args:
            feature: Feature name

        Returns:
            True if supported, False otherwise
        """
        return cli_supports_feature(feature, self.cli_type)


# Module-level default instance
cli = CLIAbstraction()


if __name__ == "__main__":
    print(f"CLI Type: {cli.cli_type}")
    print(f"Batch Size: {cli.get_batch_size()}")
    print(f"Supports caching: {cli.supports_feature('caching')}")
