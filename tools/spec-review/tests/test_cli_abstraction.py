#!/usr/bin/env python3
"""Test CLI Abstraction Layer for Python"""

import os
import sys
from pathlib import Path

# Add parent directory to path to import lib package
sys.path.insert(0, str(Path(__file__).parent.parent))

from lib.cli_detector import detect_cli, get_cli_version, cli_supports_feature
from lib.cli_abstraction import CLIAbstraction


def test_cli_detection():
    """Test CLI detection with environment variables."""
    print("=== Test Suite: CLI Detection ===")

    # Test Claude Code detection
    os.environ["CLAUDE_CODE_VERSION"] = "1.5.0"
    assert detect_cli() == "claude-code", "Should detect Claude Code"
    print("✅ PASS: Detect Claude Code via env var")
    del os.environ["CLAUDE_CODE_VERSION"]

    # Test Gemini CLI detection
    os.environ["GEMINI_CLI_VERSION"] = "0.1.0"
    assert detect_cli() == "gemini-cli", "Should detect Gemini CLI"
    print("✅ PASS: Detect Gemini CLI via env var")
    del os.environ["GEMINI_CLI_VERSION"]

    # Test OpenCode detection
    os.environ["OPENCODE_VERSION"] = "1.0.0"
    assert detect_cli() == "opencode", "Should detect OpenCode"
    print("✅ PASS: Detect OpenCode via env var")
    del os.environ["OPENCODE_VERSION"]

    # Test Codex detection
    os.environ["CODEX_VERSION"] = "1.0.0"
    assert detect_cli() == "codex", "Should detect Codex"
    print("✅ PASS: Detect Codex via env var")
    del os.environ["CODEX_VERSION"]

    # Test unknown fallback
    assert detect_cli() == "unknown", "Should return unknown for undetected CLI"
    print("✅ PASS: Detect unknown CLI")


def test_cli_version():
    """Test CLI version retrieval."""
    print("\n=== Test Suite: CLI Version ===")

    os.environ["CLAUDE_CODE_VERSION"] = "1.5.0"
    version = get_cli_version("claude-code")
    assert version == "1.5.0", "Should return correct version"
    print("✅ PASS: Get Claude Code version")
    del os.environ["CLAUDE_CODE_VERSION"]


def test_feature_support():
    """Test feature support checks."""
    print("\n=== Test Suite: Feature Support ===")

    # Claude Code supports caching
    assert cli_supports_feature("caching", "claude-code"), "Claude Code should support caching"
    print("✅ PASS: Claude Code supports caching")

    # Gemini CLI supports batch mode
    assert cli_supports_feature("batch_mode", "gemini-cli"), "Gemini should support batch mode"
    print("✅ PASS: Gemini CLI supports batch mode")

    # OpenCode supports MCP
    assert cli_supports_feature("mcp", "opencode"), "OpenCode should support MCP"
    print("✅ PASS: OpenCode supports MCP")

    # Codex supports MCP
    assert cli_supports_feature("mcp", "codex"), "Codex should support MCP"
    print("✅ PASS: Codex supports MCP")


def test_cli_abstraction():
    """Test CLI abstraction methods."""
    print("\n=== Test Suite: CLI Abstraction ===")

    # Test Claude Code read file
    cli = CLIAbstraction("claude-code")
    result = cli.read_file("/tmp/test.txt")
    assert "!read" in result, "Claude Code should use Read tool"
    print("✅ PASS: Claude Code read file uses Read tool")

    # Test Gemini invoke tool
    cli = CLIAbstraction("gemini-cli")
    result = cli.invoke_tool("grep", "pattern", "path")
    assert "@tool" in result, "Gemini should use @tool syntax"
    print("✅ PASS: Gemini invoke tool uses @tool syntax")

    # Test OpenCode invoke tool
    cli = CLIAbstraction("opencode")
    result = cli.invoke_tool("search", "query")
    assert "mcp://" in result, "OpenCode should use MCP syntax"
    print("✅ PASS: OpenCode invoke tool uses MCP syntax")


def test_cli_optimizations():
    """Test CLI-specific optimizations."""
    print("\n=== Test Suite: CLI Optimizations ===")

    # Test batch sizes
    cli_claude = CLIAbstraction("claude-code")
    assert cli_claude.get_batch_size() == 10, "Claude Code batch size should be 10"
    print("✅ PASS: Claude Code batch size is 10")

    cli_gemini = CLIAbstraction("gemini-cli")
    assert cli_gemini.get_batch_size() == 20, "Gemini CLI batch size should be 20"
    print("✅ PASS: Gemini CLI batch size is 20")

    # Test prompt caching
    result = cli_claude.cache_prompt("test-key", "test-content")
    assert "[CACHE:test-key]" in result, "Claude Code should cache prompts"
    print("✅ PASS: Claude Code caches prompts")

    result = cli_gemini.cache_prompt("test-key", "test-content")
    assert result == "test-content", "Gemini CLI shouldn't cache prompts"
    print("✅ PASS: Gemini CLI doesn't cache prompts")


if __name__ == "__main__":
    tests_passed = 0
    tests_failed = 0

    try:
        test_cli_detection()
        test_cli_version()
        test_feature_support()
        test_cli_abstraction()
        test_cli_optimizations()
        print("\n=== Test Summary ===")
        print("✅ All tests passed!")
        sys.exit(0)
    except AssertionError as e:
        print(f"\n❌ Test failed: {e}")
        print("\n=== Test Summary ===")
        print("❌ Some tests failed")
        sys.exit(1)
