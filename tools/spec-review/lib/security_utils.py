#!/usr/bin/env python3
"""
Security utilities for diagram-as-code skills.

Provides security validation functions to prevent:
- Path traversal attacks (../ sequences)
- Symlink following vulnerabilities
- File size DoS attacks
- Invalid file extensions
- Unsafe directory creation

All diagram-as-code skills MUST use these utilities for file operations.
"""

import os
from pathlib import Path
from typing import Set, Optional


# Security constants
MAX_DIAGRAM_SIZE = 10 * 1024 * 1024  # 10MB limit for diagram files
ALLOWED_DIAGRAM_EXTENSIONS = {'.d2', '.mmd', '.mermaid', '.dsl', '.structurizr', '.puml', '.plantuml'}
ALLOWED_OUTPUT_EXTENSIONS = {'.svg', '.png', '.pdf', '.json', '.txt', '.md'}


class SecurityError(Exception):
    """Raised when a security validation fails."""
    pass


class PathTraversalError(SecurityError):
    """Raised when path traversal is detected."""
    pass


class SymlinkError(SecurityError):
    """Raised when an unauthorized symlink is detected."""
    pass


class FileSizeError(SecurityError):
    """Raised when a file exceeds size limits."""
    pass


class InvalidExtensionError(SecurityError):
    """Raised when file has an invalid extension."""
    pass


def validate_path(path: str, allowed_base: str, follow_symlinks: bool = False) -> str:
    """
    Validate that a path is within the allowed base directory.

    This function prevents path traversal attacks by ensuring the resolved
    path is within the allowed base directory. It also optionally prevents
    symlink following.

    Args:
        path: User-provided path to validate
        allowed_base: Base directory that path must be within
        follow_symlinks: Whether to allow symlinks (default: False)

    Returns:
        Validated absolute path

    Raises:
        PathTraversalError: If path escapes allowed_base
        SymlinkError: If path is a symlink and follow_symlinks=False
        FileNotFoundError: If allowed_base doesn't exist

    Examples:
        >>> validate_path("/tmp/diagrams/file.d2", "/tmp")
        "/tmp/diagrams/file.d2"

        >>> validate_path("/tmp/../etc/passwd", "/tmp")
        PathTraversalError: Path /tmp/../etc/passwd escapes allowed directory /tmp
    """
    # Convert to Path objects for safe resolution
    path_obj = Path(path)
    base_obj = Path(allowed_base).resolve()

    # Ensure base directory exists
    if not base_obj.exists():
        raise FileNotFoundError(f"Allowed base directory does not exist: {allowed_base}")

    if not base_obj.is_dir():
        raise NotADirectoryError(f"Allowed base must be a directory: {allowed_base}")

    try:

        # Check for symlinks before resolving if not allowed
        if not follow_symlinks and path_obj.exists() and path_obj.is_symlink():
            raise SymlinkError(f"Symlinks not allowed: {path}")

        # Resolve to absolute path (follows symlinks if they exist)
        resolved = path_obj.resolve()

        # Validate path is within allowed base
        try:
            resolved.relative_to(base_obj)
        except ValueError:
            raise PathTraversalError(
                f"Path {path} escapes allowed directory {allowed_base}"
            )

        return str(resolved)

    except (OSError, RuntimeError) as e:
        # Catch resolution errors (infinite loops, etc.)
        raise SecurityError(f"Path resolution failed: {e}")


def validate_diagram_path(path: str, allowed_base: str) -> str:
    """
    Validate diagram file path with extension checking.

    Args:
        path: Path to diagram file
        allowed_base: Base directory for diagrams

    Returns:
        Validated absolute path

    Raises:
        PathTraversalError: If path escapes allowed_base
        SymlinkError: If path is a symlink
        InvalidExtensionError: If file extension is not allowed

    Examples:
        >>> validate_diagram_path("/tmp/diagram.d2", "/tmp")
        "/tmp/diagram.d2"

        >>> validate_diagram_path("/tmp/malicious.exe", "/tmp")
        InvalidExtensionError: Invalid diagram extension: .exe
    """
    # First validate path security
    validated_path = validate_path(path, allowed_base, follow_symlinks=False)

    # Then validate extension
    ext = Path(validated_path).suffix.lower()
    if ext not in ALLOWED_DIAGRAM_EXTENSIONS:
        raise InvalidExtensionError(
            f"Invalid diagram extension: {ext}. "
            f"Allowed: {', '.join(sorted(ALLOWED_DIAGRAM_EXTENSIONS))}"
        )

    return validated_path


def validate_output_path(path: str, allowed_base: str) -> str:
    """
    Validate output file path with extension checking.

    Args:
        path: Path to output file
        allowed_base: Base directory for outputs

    Returns:
        Validated absolute path

    Raises:
        PathTraversalError: If path escapes allowed_base
        SymlinkError: If path is a symlink
        InvalidExtensionError: If file extension is not allowed

    Examples:
        >>> validate_output_path("/tmp/out/diagram.svg", "/tmp")
        "/tmp/out/diagram.svg"

        >>> validate_output_path("/tmp/../etc/shadow", "/tmp")
        PathTraversalError: Path /tmp/../etc/shadow escapes allowed directory /tmp
    """
    # First validate path security
    validated_path = validate_path(path, allowed_base, follow_symlinks=False)

    # Then validate extension
    ext = Path(validated_path).suffix.lower()
    if ext and ext not in ALLOWED_OUTPUT_EXTENSIONS:
        raise InvalidExtensionError(
            f"Invalid output extension: {ext}. "
            f"Allowed: {', '.join(sorted(ALLOWED_OUTPUT_EXTENSIONS))}"
        )

    return validated_path


def safe_read_file(path: str, max_size: Optional[int] = None) -> str:
    """
    Safely read a file with size validation.

    Args:
        path: Path to file (must already be validated)
        max_size: Maximum file size in bytes (default: MAX_DIAGRAM_SIZE)

    Returns:
        File contents as string

    Raises:
        FileSizeError: If file exceeds max_size
        FileNotFoundError: If file doesn't exist

    Examples:
        >>> safe_read_file("/tmp/diagram.d2")
        "graph { ... }"

        >>> safe_read_file("/tmp/huge.d2")
        FileSizeError: File too large: 50000000 bytes (max 10485760)
    """
    if max_size is None:
        max_size = MAX_DIAGRAM_SIZE

    # Check file exists
    if not os.path.exists(path):
        raise FileNotFoundError(f"File not found: {path}")

    # Check file size before reading
    file_size = os.path.getsize(path)
    if file_size > max_size:
        raise FileSizeError(
            f"File too large: {file_size} bytes (max {max_size})"
        )

    # Read file
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()


def safe_create_directory(path: str, allowed_base: str, mode: int = 0o755) -> str:
    """
    Safely create a directory with validation.

    Args:
        path: Directory path to create
        allowed_base: Base directory that path must be within
        mode: Directory permissions (default: 0o755)

    Returns:
        Validated absolute path to created directory

    Raises:
        PathTraversalError: If path escapes allowed_base
        SymlinkError: If path is a symlink

    Examples:
        >>> safe_create_directory("/tmp/output/diagrams", "/tmp")
        "/tmp/output/diagrams"

        >>> safe_create_directory("/tmp/../etc/malicious", "/tmp")
        PathTraversalError: Path /tmp/../etc/malicious escapes allowed directory /tmp
    """
    # Validate path (but allow non-existent paths)
    path_obj = Path(path)
    base_obj = Path(allowed_base).resolve()

    # Ensure base directory exists
    if not base_obj.exists():
        raise FileNotFoundError(f"Allowed base directory does not exist: {allowed_base}")

    if not base_obj.is_dir():
        raise NotADirectoryError(f"Allowed base must be a directory: {allowed_base}")

    # Check for symlink BEFORE resolving
    if path_obj.exists() and path_obj.is_symlink():
        raise SymlinkError(f"Path is a symlink: {path}")

    # Resolve path (without requiring it to exist)
    # We need to resolve the parent and then append the final component
    if path_obj.exists():
        resolved = path_obj.resolve()
    else:
        # For non-existent paths, resolve parent and append name
        parent = path_obj.parent
        if parent.exists():
            resolved = parent.resolve() / path_obj.name
        else:
            # Recursively resolve parent components
            resolved = Path(os.path.abspath(path))

    # Validate resolved path is within base
    try:
        resolved.relative_to(base_obj)
    except ValueError:
        raise PathTraversalError(
            f"Path {path} escapes allowed directory {allowed_base}"
        )

    # Create directory
    os.makedirs(resolved, exist_ok=True, mode=mode)

    return str(resolved)


def is_safe_path(path: str, allowed_base: str) -> bool:
    """
    Check if a path is safe without raising exceptions.

    Args:
        path: Path to check
        allowed_base: Base directory

    Returns:
        True if path is safe, False otherwise

    Examples:
        >>> is_safe_path("/tmp/diagrams/file.d2", "/tmp")
        True

        >>> is_safe_path("/tmp/../etc/passwd", "/tmp")
        False
    """
    try:
        validate_path(path, allowed_base, follow_symlinks=False)
        return True
    except SecurityError:
        return False


def get_safe_temp_dir() -> str:
    """
    Get a safe temporary directory for diagram operations.

    Returns:
        Path to safe temporary directory (/tmp or equivalent)

    Examples:
        >>> get_safe_temp_dir()
        "/tmp"
    """
    import tempfile
    return tempfile.gettempdir()
