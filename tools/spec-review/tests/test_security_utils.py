#!/usr/bin/env python3
"""
Unit tests for security_utils module.

Tests all security validation functions to ensure they properly prevent:
- Path traversal attacks
- Symlink following
- File size DoS
- Invalid file extensions
"""

import os
import sys
import tempfile
from pathlib import Path
import pytest

# Add lib to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))
from security_utils import (
    validate_path,
    validate_diagram_path,
    validate_output_path,
    safe_read_file,
    safe_create_directory,
    is_safe_path,
    PathTraversalError,
    SymlinkError,
    FileSizeError,
    InvalidExtensionError,
    SecurityError,
    MAX_DIAGRAM_SIZE,
)


class TestValidatePath:
    """Test validate_path function."""

    def test_valid_path_within_base(self):
        """Valid path within base directory should pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            test_path = os.path.join(tmpdir, "subdir", "file.txt")
            result = validate_path(test_path, tmpdir)
            assert result.startswith(tmpdir)

    def test_path_traversal_blocked(self):
        """Path traversal with ../ should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            attack_path = os.path.join(tmpdir, "..", "..", "etc", "passwd")
            with pytest.raises(PathTraversalError):
                validate_path(attack_path, tmpdir)

    def test_absolute_path_outside_base_blocked(self):
        """Absolute path outside base should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            attack_path = "/etc/passwd"
            with pytest.raises(PathTraversalError):
                validate_path(attack_path, tmpdir)

    def test_symlink_blocked_by_default(self):
        """Symlinks should be blocked by default."""
        with tempfile.TemporaryDirectory() as tmpdir:
            real_file = Path(tmpdir) / "real.txt"
            real_file.touch()
            symlink = Path(tmpdir) / "link.txt"
            symlink.symlink_to(real_file)

            with pytest.raises(SymlinkError):
                validate_path(str(symlink), tmpdir, follow_symlinks=False)

    def test_symlink_allowed_when_enabled(self):
        """Symlinks should be allowed when follow_symlinks=True."""
        with tempfile.TemporaryDirectory() as tmpdir:
            real_file = Path(tmpdir) / "real.txt"
            real_file.touch()
            symlink = Path(tmpdir) / "link.txt"
            symlink.symlink_to(real_file)

            # Should not raise when follow_symlinks=True
            result = validate_path(str(symlink), tmpdir, follow_symlinks=True)
            assert result == str(real_file.resolve())

    def test_symlink_to_outside_blocked(self):
        """Symlink pointing outside base should be blocked even if allowed."""
        with tempfile.TemporaryDirectory() as tmpdir:
            symlink = Path(tmpdir) / "evil.txt"
            symlink.symlink_to("/etc/passwd")

            with pytest.raises(PathTraversalError):
                validate_path(str(symlink), tmpdir, follow_symlinks=True)

    def test_nonexistent_base_raises_error(self):
        """Non-existent base directory should raise error."""
        with pytest.raises(FileNotFoundError):
            validate_path("/tmp/file.txt", "/nonexistent/base")

    def test_file_as_base_raises_error(self):
        """File as base (not directory) should raise error."""
        with tempfile.NamedTemporaryFile() as tmpfile:
            with pytest.raises(NotADirectoryError):
                validate_path("/tmp/file.txt", tmpfile.name)


class TestValidateDiagramPath:
    """Test validate_diagram_path function."""

    def test_valid_d2_diagram(self):
        """Valid .d2 diagram should pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            diagram_path = os.path.join(tmpdir, "diagram.d2")
            Path(diagram_path).touch()
            result = validate_diagram_path(diagram_path, tmpdir)
            assert result.endswith(".d2")

    def test_valid_mermaid_diagram(self):
        """Valid .mmd diagram should pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            diagram_path = os.path.join(tmpdir, "diagram.mmd")
            Path(diagram_path).touch()
            result = validate_diagram_path(diagram_path, tmpdir)
            assert result.endswith(".mmd")

    def test_invalid_extension_blocked(self):
        """Invalid extension should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            malicious_path = os.path.join(tmpdir, "malicious.exe")
            with pytest.raises(InvalidExtensionError):
                validate_diagram_path(malicious_path, tmpdir)

    def test_path_traversal_blocked(self):
        """Path traversal should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            attack_path = os.path.join(tmpdir, "..", "..", "etc", "passwd.d2")
            with pytest.raises(PathTraversalError):
                validate_diagram_path(attack_path, tmpdir)

    def test_symlink_blocked(self):
        """Symlinks should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            real_file = Path(tmpdir) / "real.d2"
            real_file.touch()
            symlink = Path(tmpdir) / "link.d2"
            symlink.symlink_to(real_file)

            with pytest.raises(SymlinkError):
                validate_diagram_path(str(symlink), tmpdir)


class TestValidateOutputPath:
    """Test validate_output_path function."""

    def test_valid_svg_output(self):
        """Valid .svg output should pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "output.svg")
            result = validate_output_path(output_path, tmpdir)
            assert result.endswith(".svg")

    def test_valid_png_output(self):
        """Valid .png output should pass."""
        with tempfile.TemporaryDirectory() as tmpdir:
            output_path = os.path.join(tmpdir, "output.png")
            result = validate_output_path(output_path, tmpdir)
            assert result.endswith(".png")

    def test_invalid_output_extension_blocked(self):
        """Invalid output extension should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            malicious_path = os.path.join(tmpdir, "malicious.exe")
            with pytest.raises(InvalidExtensionError):
                validate_output_path(malicious_path, tmpdir)

    def test_path_traversal_blocked(self):
        """Path traversal should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            attack_path = os.path.join(tmpdir, "..", "..", "etc", "shadow")
            with pytest.raises(PathTraversalError):
                validate_output_path(attack_path, tmpdir)


class TestSafeReadFile:
    """Test safe_read_file function."""

    def test_read_normal_file(self):
        """Normal file should be read successfully."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.d2', delete=False) as f:
            content = "test content"
            f.write(content)
            temp_path = f.name

        try:
            result = safe_read_file(temp_path)
            assert result == content
        finally:
            os.unlink(temp_path)

    def test_large_file_blocked(self):
        """File larger than MAX_DIAGRAM_SIZE should be blocked."""
        with tempfile.NamedTemporaryFile(mode='wb', suffix='.d2', delete=False) as f:
            # Write 20MB (exceeds 10MB limit)
            f.write(b"A" * (20 * 1024 * 1024))
            temp_path = f.name

        try:
            with pytest.raises(FileSizeError):
                safe_read_file(temp_path)
        finally:
            os.unlink(temp_path)

    def test_custom_size_limit(self):
        """Custom size limit should be respected."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.d2', delete=False) as f:
            f.write("A" * 1000)  # 1KB
            temp_path = f.name

        try:
            # Should fail with 500 byte limit
            with pytest.raises(FileSizeError):
                safe_read_file(temp_path, max_size=500)
        finally:
            os.unlink(temp_path)

    def test_nonexistent_file_raises_error(self):
        """Non-existent file should raise FileNotFoundError."""
        with pytest.raises(FileNotFoundError):
            safe_read_file("/nonexistent/file.d2")


class TestSafeCreateDirectory:
    """Test safe_create_directory function."""

    def test_create_directory_within_base(self):
        """Directory within base should be created."""
        with tempfile.TemporaryDirectory() as tmpdir:
            new_dir = os.path.join(tmpdir, "subdir", "nested")
            result = safe_create_directory(new_dir, tmpdir)
            assert os.path.isdir(result)
            assert result.startswith(tmpdir)

    def test_path_traversal_blocked(self):
        """Path traversal should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            attack_path = os.path.join(tmpdir, "..", "..", "etc", "malicious")
            with pytest.raises(PathTraversalError):
                safe_create_directory(attack_path, tmpdir)

    def test_existing_directory_ok(self):
        """Existing directory should not raise error."""
        with tempfile.TemporaryDirectory() as tmpdir:
            existing = os.path.join(tmpdir, "existing")
            os.makedirs(existing)
            result = safe_create_directory(existing, tmpdir)
            assert result == str(Path(existing).resolve())

    def test_symlink_blocked(self):
        """Symlink should be blocked."""
        with tempfile.TemporaryDirectory() as tmpdir:
            real_dir = Path(tmpdir) / "real"
            real_dir.mkdir()
            symlink = Path(tmpdir) / "link"
            symlink.symlink_to(real_dir)

            with pytest.raises(SymlinkError):
                safe_create_directory(str(symlink), tmpdir)


class TestIsSafePath:
    """Test is_safe_path function."""

    def test_safe_path_returns_true(self):
        """Safe path should return True."""
        with tempfile.TemporaryDirectory() as tmpdir:
            safe_path = os.path.join(tmpdir, "file.txt")
            assert is_safe_path(safe_path, tmpdir) is True

    def test_unsafe_path_returns_false(self):
        """Unsafe path should return False."""
        with tempfile.TemporaryDirectory() as tmpdir:
            unsafe_path = "/etc/passwd"
            assert is_safe_path(unsafe_path, tmpdir) is False

    def test_traversal_path_returns_false(self):
        """Path traversal should return False."""
        with tempfile.TemporaryDirectory() as tmpdir:
            traversal_path = os.path.join(tmpdir, "..", "..", "etc", "passwd")
            assert is_safe_path(traversal_path, tmpdir) is False


class TestSecurityIntegration:
    """Integration tests combining multiple security features."""

    def test_complete_attack_chain_blocked(self):
        """Complete attack chain should be blocked at every step."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Attack 1: Path traversal
            with pytest.raises(PathTraversalError):
                validate_diagram_path("../../etc/passwd.d2", tmpdir)

            # Attack 2: Symlink to external file
            symlink = Path(tmpdir) / "evil.d2"
            symlink.symlink_to("/etc/passwd")
            with pytest.raises(SymlinkError):
                validate_diagram_path(str(symlink), tmpdir)

            # Attack 3: Invalid extension
            with pytest.raises(InvalidExtensionError):
                validate_diagram_path(os.path.join(tmpdir, "evil.exe"), tmpdir)

    def test_legitimate_workflow_allowed(self):
        """Legitimate workflow should work without issues."""
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create diagram
            diagram_path = os.path.join(tmpdir, "diagram.d2")
            Path(diagram_path).write_text("graph { A -> B }")

            # Validate diagram path
            validated = validate_diagram_path(diagram_path, tmpdir)
            assert validated == str(Path(diagram_path).resolve())

            # Read diagram
            content = safe_read_file(validated)
            assert "A -> B" in content

            # Create output directory
            output_dir = os.path.join(tmpdir, "output")
            output_dir_created = safe_create_directory(output_dir, tmpdir)
            assert os.path.isdir(output_dir_created)

            # Validate output path
            output_path = os.path.join(output_dir, "diagram.svg")
            validated_output = validate_output_path(output_path, tmpdir)
            assert validated_output.endswith(".svg")


def test_security_constants():
    """Test security constants are properly defined."""
    assert MAX_DIAGRAM_SIZE == 10 * 1024 * 1024  # 10MB
    from security_utils import ALLOWED_DIAGRAM_EXTENSIONS, ALLOWED_OUTPUT_EXTENSIONS
    assert '.d2' in ALLOWED_DIAGRAM_EXTENSIONS
    assert '.mmd' in ALLOWED_DIAGRAM_EXTENSIONS
    assert '.svg' in ALLOWED_OUTPUT_EXTENSIONS
    assert '.png' in ALLOWED_OUTPUT_EXTENSIONS


if __name__ == "__main__":
    pytest.main([__file__, "-v", "-s"])
