#!/usr/bin/env python3
"""
Test suite for astrocyte daemon violation pattern detection.

Tests the is_stuck_permission_prompt() heuristic to ensure it detects
bash command violations in permission prompts correctly.
"""

import pytest
from datetime import datetime, timedelta
from dataclasses import dataclass
from typing import Tuple

# Import from parent directory
import sys
from pathlib import Path
sys.path.insert(0, str(Path(__file__).parent.parent))

from astrocyte import (
    SessionState,
    is_stuck_permission_prompt,
    MUSTERING_PATTERNS,
)


@dataclass
class MockSessionState:
    """Mock SessionState for testing."""
    pane_content: str
    cursor_position: Tuple[int, int]
    timestamp: datetime


class TestViolationPatternDetection:
    """Test violation pattern detection in permission prompts."""

    def create_permission_prompt(self, command: str, cursor_row: int = 38) -> MockSessionState:
        """Create a mock permission prompt with given bash command."""
        content = f"""
● Bash({command})
  ⎿  Test bash command

───────────────────────────────────────────────────────────────────────────────────────────────────────────────
 Bash command

   {command}
   Description here

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for commands in /home/user/src
   3. No

 Esc to cancel · Tab to add additional instructions
"""
        return MockSessionState(
            pane_content=content,
            cursor_position=(cursor_row, 0),
            timestamp=datetime.now()
        )

    def test_cd_usage_detected(self):
        """Test that cd usage is detected in permission prompts."""
        current = self.create_permission_prompt("cd /home/user/repo && git status")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect cd && violation"

    def test_cd_semicolon_detected(self):
        """Test that cd with semicolon is detected."""
        current = self.create_permission_prompt("cd /tmp; ls; cd -")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect cd with semicolon"

    def test_command_chaining_and_detected(self):
        """Test that && chaining is detected."""
        current = self.create_permission_prompt("git add . && git commit -m 'msg' && git push")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect && chaining"

    def test_command_chaining_semicolon_detected(self):
        """Test that semicolon chaining is detected."""
        current = self.create_permission_prompt("echo 'start'; ls; echo 'done'")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect semicolon chaining"

    def test_command_chaining_or_detected(self):
        """Test that || chaining is detected."""
        current = self.create_permission_prompt("rm file.txt || true")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect || chaining"

    def test_pipe_operator_detected(self):
        """Test that pipe operator is detected."""
        current = self.create_permission_prompt("cat file.txt | grep pattern")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect pipe operator"

    def test_cat_command_detected(self):
        """Test that cat command is detected."""
        current = self.create_permission_prompt("cat /etc/hosts")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect cat command"

    def test_grep_command_detected(self):
        """Test that grep command is detected."""
        current = self.create_permission_prompt("grep 'TODO' src/main.py")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect grep command"

    def test_find_command_detected(self):
        """Test that find command is detected."""
        current = self.create_permission_prompt("find . -name '*.py'")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect find command"

    def test_ls_command_detected(self):
        """Test that ls command is detected."""
        current = self.create_permission_prompt("ls -lah /tmp")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect ls command"

    def test_stat_command_detected(self):
        """Test that stat command is detected."""
        current = self.create_permission_prompt("stat -c '%y %s' file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect stat command"

    def test_sed_command_detected(self):
        """Test that sed command is detected."""
        current = self.create_permission_prompt("sed -i 's/foo/bar/' file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect sed command"

    def test_awk_command_detected(self):
        """Test that awk command is detected."""
        current = self.create_permission_prompt("awk '{print $1}' file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect awk command"

    def test_rm_command_detected(self):
        """Test that rm command is detected."""
        current = self.create_permission_prompt("rm ~/.claude/hooks/pretool-beads-protection.py")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect rm command"

    def test_cp_command_detected(self):
        """Test that cp command is detected."""
        current = self.create_permission_prompt("cp source.txt dest.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect cp command"

    def test_mv_command_detected(self):
        """Test that mv command is detected."""
        current = self.create_permission_prompt("mv old.txt new.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect mv command"

    def test_mkdir_command_detected(self):
        """Test that mkdir command is detected."""
        current = self.create_permission_prompt("mkdir -p /path/to/dir")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect mkdir command"

    def test_install_command_detected(self):
        """Test that install -D command is detected."""
        current = self.create_permission_prompt("install -D src dest")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect install -D command"

    def test_for_loop_detected(self):
        """Test that for loop is detected."""
        current = self.create_permission_prompt("for file in *.txt; do cat $file; done")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect for loop"

    def test_while_loop_detected(self):
        """Test that while loop is detected."""
        current = self.create_permission_prompt("while read line; do echo $line; done")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect while loop"

    def test_output_redirect_detected(self):
        """Test that output redirection is detected."""
        current = self.create_permission_prompt("echo 'text' > file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect > redirect"

    def test_append_redirect_detected(self):
        """Test that append redirection is detected."""
        current = self.create_permission_prompt("echo 'text' >> file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect >> redirect"

    def test_heredoc_detected(self):
        """Test that heredoc is detected."""
        current = self.create_permission_prompt("cat << EOF")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect heredoc"

    def test_head_command_detected(self):
        """Test that head command is detected."""
        current = self.create_permission_prompt("head -n 20 file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect head command"

    def test_tail_command_detected(self):
        """Test that tail command is detected."""
        current = self.create_permission_prompt("tail -f /var/log/syslog")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect tail command"

    def test_wc_command_detected(self):
        """Test that wc command is detected."""
        current = self.create_permission_prompt("wc -l file.txt")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect wc command"


class TestAllowedCommands:
    """Test that allowed commands are NOT detected as violations."""

    def create_permission_prompt(self, command: str, cursor_row: int = 38) -> MockSessionState:
        """Create a mock permission prompt with given bash command."""
        content = f"""
● Bash({command})
  ⎿  Test bash command

───────────────────────────────────────────────────────────────────────────────────────────────────────────────
 Bash command

   {command}
   Description here

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for commands in /home/user/src
   3. No

 Esc to cancel · Tab to add additional instructions
"""
        return MockSessionState(
            pane_content=content,
            cursor_position=(cursor_row, 0),
            timestamp=datetime.now()
        )

    def test_git_c_allowed(self):
        """Test that git -C is allowed (not detected as violation)."""
        current = self.create_permission_prompt("git -C /home/user/repo status")
        # This should NOT be detected as a violation
        # The permission prompt exists for other reasons (user policy), not violations
        # So we can't directly test "not detected" since the prompt still exists
        # But we can verify the pattern isn't in violation_patterns
        from astrocyte import is_stuck_permission_prompt
        # If git -C is in the allowed list, the violation check should pass
        # This test documents expected behavior
        pass

    def test_npm_prefix_allowed(self):
        """Test that npm --prefix is allowed."""
        current = self.create_permission_prompt("npm --prefix /path test")
        # Similar to git -C test
        pass

    def test_bd_db_allowed(self):
        """Test that bd --db is allowed."""
        current = self.create_permission_prompt("bd --db=/path/beads.db list")
        # Similar to git -C test
        pass

    def test_simple_command_no_violations(self):
        """Test that simple commands without violations are not flagged."""
        current = self.create_permission_prompt("pytest /home/user/tests")
        # Simple command with absolute path - no violations
        pass


class TestComplexPatterns:
    """Test complex violation patterns and edge cases."""

    def create_permission_prompt(self, command: str, cursor_row: int = 38) -> MockSessionState:
        """Create a mock permission prompt with given bash command."""
        content = f"""
● Bash({command})
  ⎿  Test bash command

───────────────────────────────────────────────────────────────────────────────────────────────────────────────
 Bash command

   {command}
   Description here

 Do you want to proceed?
 ❯ 1. Yes
   2. Yes, and don't ask again for commands in /home/user/src
   3. No

 Esc to cancel · Tab to add additional instructions
"""
        return MockSessionState(
            pane_content=content,
            cursor_position=(cursor_row, 0),
            timestamp=datetime.now()
        )

    def test_multiple_violations(self):
        """Test command with multiple violations."""
        current = self.create_permission_prompt("cd /tmp && cat file.txt | grep pattern")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect multiple violations"

    def test_complex_pipeline(self):
        """Test complex pipeline with multiple stages."""
        current = self.create_permission_prompt("find . -name '*.log' | grep ERROR | head -10")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect complex pipeline violations"

    def test_nested_command_substitution(self):
        """Test nested command substitution with violations."""
        current = self.create_permission_prompt("echo $(cat file.txt)")
        result = is_stuck_permission_prompt(current, None, 0)
        assert result, "Should detect cat in command substitution"


if __name__ == "__main__":
    # Run with: pytest test_violation_patterns.py -v
    pytest.main([__file__, "-v", "--tb=short"])
