# Security Fixes Implementation Guide

**Target**: Diagram-as-Code Skills Security Vulnerabilities
**Priority**: Critical (Deploy Phase 1 fixes immediately)

---

## Quick Reference: Secure Coding Patterns

### ❌ VULNERABLE Pattern

```python
# DON'T: This allows path traversal and symlink attacks
import os

def process_file(user_path: str):
    path = os.path.abspath(user_path)  # ❌ VULNERABLE
    with open(path, 'r') as f:         # ❌ Can read ANY file
        return f.read()
```

### ✅ SECURE Pattern

```python
# DO: This prevents path traversal and symlink attacks
from pathlib import Path

def process_file(user_path: str, allowed_base: str) -> str:
    """Process file with security validation."""
    # 1. Resolve symlinks and normalize path
    resolved = Path(user_path).resolve()
    base = Path(allowed_base).resolve()

    # 2. Validate path is within allowed directory
    try:
        resolved.relative_to(base)
    except ValueError:
        raise ValueError(f"Path {user_path} is outside allowed directory")

    # 3. Check file size before reading
    file_size = resolved.stat().st_size
    if file_size > 10_000_000:  # 10MB
        raise ValueError(f"File too large: {file_size} bytes")

    # 4. Read file safely
    with open(resolved, 'r') as f:
        return f.read()
```

---

## Phase 1: Critical Path Security Fixes

### Fix 1: create_diagrams.py

**File**: `plugins/spec-review-marketplace/skills/create-diagrams/create_diagrams.py`

**Current Code** (lines 44-56):
```python
# VULNERABLE
def create_diagrams(
    codebase_path: str,
    output_dir: str,
    ...
):
    # Resolve paths
    codebase_path = os.path.abspath(codebase_path)  # ❌ VULNERABLE
    output_dir = os.path.abspath(output_dir)        # ❌ VULNERABLE

    # Validate inputs
    if not os.path.exists(codebase_path):
        raise FileNotFoundError(f"Codebase path not found: {codebase_path}")
```

**Fixed Code**:
```python
# SECURE
from pathlib import Path

def validate_path(user_path: str, allowed_base: str, must_exist: bool = True) -> str:
    """
    Validate path is within allowed base directory.

    Args:
        user_path: User-provided path
        allowed_base: Base directory that paths must be under
        must_exist: Whether path must already exist

    Returns:
        Validated absolute path

    Raises:
        ValueError: If path escapes allowed directory
        FileNotFoundError: If must_exist=True and path doesn't exist
    """
    # Resolve to absolute path (handles symlinks)
    resolved = Path(user_path).resolve()
    base = Path(allowed_base).resolve()

    # Validate containment
    try:
        resolved.relative_to(base)
    except ValueError:
        raise ValueError(
            f"Security: Path '{user_path}' is outside allowed directory '{allowed_base}'"
        )

    # Check existence if required
    if must_exist and not resolved.exists():
        raise FileNotFoundError(f"Path not found: {user_path}")

    return str(resolved)


def create_diagrams(
    codebase_path: str,
    output_dir: str,
    format: DiagramFormat = "d2",
    level: C4Level = "all",
    language: Optional[str] = None,
    template: Optional[str] = None,
) -> dict:
    """Generate C4 Model diagrams from codebase analysis."""

    # Get current working directory as allowed base
    cwd = os.getcwd()

    # Validate codebase path (must exist, must be under cwd)
    codebase_path = validate_path(codebase_path, cwd, must_exist=True)

    # Validate it's a directory
    if not os.path.isdir(codebase_path):
        raise NotADirectoryError(f"Codebase path must be a directory: {codebase_path}")

    # Validate output directory (create if doesn't exist)
    # Allow /tmp for output, but validate it's not a system directory
    if output_dir.startswith('/tmp/'):
        output_dir = str(Path(output_dir).resolve())
    else:
        output_dir = validate_path(output_dir, cwd, must_exist=False)

    # Create output directory
    os.makedirs(output_dir, exist_ok=True)

    # Rest of function...
```

---

### Fix 2: review_diagrams.py

**File**: `plugins/spec-review-marketplace/skills/review-diagrams/review_diagrams.py`

**Current Code** (lines 44-48):
```python
# VULNERABLE
def review_diagram(diagram_path: str, ...):
    # Resolve paths
    diagram_path = os.path.abspath(diagram_path)  # ❌ VULNERABLE

    if not os.path.exists(diagram_path):
        raise FileNotFoundError(f"Diagram not found: {diagram_path}")
```

**Current Code** (lines 200-202):
```python
# VULNERABLE - No size check
def review_with_persona(...):
    # Read diagram content
    with open(diagram_path, 'r') as f:
        diagram_content = f.read()  # ❌ Can read huge files
```

**Fixed Code**:
```python
from pathlib import Path

MAX_DIAGRAM_SIZE = 10 * 1024 * 1024  # 10MB

def read_diagram_safe(path: str) -> str:
    """
    Read diagram file with size validation.

    Args:
        path: Path to diagram file

    Returns:
        Diagram content

    Raises:
        ValueError: If file is too large
    """
    path_obj = Path(path)
    file_size = path_obj.stat().st_size

    if file_size > MAX_DIAGRAM_SIZE:
        raise ValueError(
            f"Diagram file too large: {file_size} bytes (max {MAX_DIAGRAM_SIZE})"
        )

    with open(path_obj, 'r') as f:
        return f.read()


def review_diagram(
    diagram_path: str,
    rubric_path: Optional[str] = None,
    personas: Optional[List[Persona]] = None,
    validate_only: bool = False,
) -> Dict:
    """Review diagram quality using multi-persona approach."""

    # Get current working directory as allowed base
    cwd = os.getcwd()

    # Validate diagram path
    diagram_path = validate_path(diagram_path, cwd, must_exist=True)

    # Load rubric
    if rubric_path is None:
        rubric_path = find_rubric()
    else:
        rubric_path = validate_path(rubric_path, cwd, must_exist=True)

    with open(rubric_path, 'r') as f:
        rubric = yaml.safe_load(f)

    # Rest of function...


def review_with_persona(diagram_path: str, ...) -> Tuple[float, str]:
    """Review diagram from specific persona perspective."""

    # Read diagram content with size validation
    diagram_content = read_diagram_safe(diagram_path)  # ✅ SECURE

    # Rest of function...
```

---

### Fix 3: render_diagrams.py

**File**: `plugins/spec-review-marketplace/skills/render-diagrams/render_diagrams.py`

**Current Code** (lines 327-329):
```python
# Minimal validation
if not args.input_file.exists():
    console.print(f"[red]✗ File not found:[/red] {args.input_file}")
    return 1
```

**Fixed Code**:
```python
def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(...)
    args = parser.parse_args()

    # Validate arguments
    if not args.validate_only and not args.output_file:
        parser.error("output_file is required unless --validate-only is specified")

    # Security: Validate input path
    cwd = os.getcwd()
    try:
        input_path = validate_path(str(args.input_file), cwd, must_exist=True)
        args.input_file = Path(input_path)
    except ValueError as e:
        console.print(f"[red]✗ Security error:[/red] {e}")
        return 1

    if not args.input_file.exists():
        console.print(f"[red]✗ File not found:[/red] {args.input_file}")
        return 1

    # Security: Validate output path (if provided)
    if args.output_file:
        # Allow /tmp for output
        if str(args.output_file).startswith('/tmp/'):
            output_path = str(Path(args.output_file).resolve())
        else:
            try:
                output_path = validate_path(str(args.output_file), cwd, must_exist=False)
            except ValueError as e:
                console.print(f"[red]✗ Security error:[/red] {e}")
                return 1
        args.output_file = Path(output_path)

    # Security: Check file size
    try:
        file_size = args.input_file.stat().st_size
        if file_size > MAX_DIAGRAM_SIZE:
            console.print(
                f"[red]✗ File too large:[/red] {file_size} bytes "
                f"(max {MAX_DIAGRAM_SIZE})"
            )
            return 1
    except Exception as e:
        console.print(f"[red]✗ Error checking file size:[/red] {e}")
        return 1

    # Rest of function...
```

---

### Fix 4: diagram_sync.py

**File**: `plugins/spec-review-marketplace/skills/diagram-sync/diagram_sync.py`

**Current Code**: No path validation at all

**Fixed Code**:
```python
from pathlib import Path

def sync_diagram(
    diagram_path: str,
    codebase_path: str,
    generate_patches: bool = False,
    json_output: bool = False,
) -> Dict:
    """Compare diagram against codebase and detect drift."""

    # Security: Validate paths
    cwd = os.getcwd()

    try:
        diagram_path = validate_path(diagram_path, cwd, must_exist=True)
        codebase_path = validate_path(codebase_path, cwd, must_exist=True)
    except ValueError as e:
        return {
            "success": False,
            "error": f"Security error: {e}",
        }

    # Detect diagram format
    format = detect_format(diagram_path)
    if format == "unknown":
        raise ValueError(f"Unknown diagram format: {diagram_path}")

    # Rest of function...
```

---

## Phase 2: Additional Security Improvements

### Improvement 1: Binary Path Hardening

**Current Code** (all skills):
```python
# VULNERABLE: Trusts PATH environment variable
result = subprocess.run(
    ["which", "create-diagrams"],
    capture_output=True,
    text=True,
    check=True,
)
```

**Fixed Code**:
```python
import shutil

def find_binary_secure(binary_name: str) -> str:
    """
    Find binary in trusted paths only.

    Args:
        binary_name: Name of binary to find

    Returns:
        Path to binary

    Raises:
        FileNotFoundError: If binary not found in trusted paths
    """
    # Trusted search paths (in order of preference)
    trusted_paths = [
        "/usr/local/bin",
        "/usr/bin",
        str(Path.home() / "go" / "bin"),
        str(Path.home() / ".local" / "bin"),
    ]

    # Search trusted paths
    for path in trusted_paths:
        binary_path = Path(path) / binary_name
        if binary_path.exists() and os.access(binary_path, os.X_OK):
            return str(binary_path)

    # Not found in trusted paths
    raise FileNotFoundError(
        f"Binary '{binary_name}' not found in trusted paths: {trusted_paths}"
    )
```

---

### Improvement 2: Codebase Size Limits

**Add to create_diagrams.py**:
```python
MAX_CODEBASE_FILES = 10_000
MAX_CODEBASE_SIZE = 100 * 1024 * 1024  # 100MB

def validate_codebase_size(codebase_path: str) -> None:
    """
    Validate codebase size is reasonable.

    Args:
        codebase_path: Path to codebase directory

    Raises:
        ValueError: If codebase is too large
    """
    file_count = 0
    total_size = 0

    for root, dirs, files in os.walk(codebase_path):
        # Skip hidden directories
        dirs[:] = [d for d in dirs if not d.startswith('.')]

        for file in files:
            file_count += 1
            if file_count > MAX_CODEBASE_FILES:
                raise ValueError(
                    f"Codebase has too many files (>{MAX_CODEBASE_FILES})"
                )

            try:
                file_path = os.path.join(root, file)
                total_size += os.path.getsize(file_path)

                if total_size > MAX_CODEBASE_SIZE:
                    raise ValueError(
                        f"Codebase too large: {total_size} bytes (max {MAX_CODEBASE_SIZE})"
                    )
            except OSError:
                # Skip files we can't stat
                continue
```

---

## Testing Your Fixes

### Run Security Test Suite

```bash
# From project root
python3 -m pytest plugins/spec-review-marketplace/tests/test_security.py -v

# Should see:
# ✓ 14 passed - All security tests pass
```

### Manual Security Testing

```bash
# Test 1: Path traversal blocked
create-diagrams --codebase ../../../etc --output /tmp/out
# Expected: ValueError - path outside allowed directory

# Test 2: Symlink attack blocked
ln -s /etc/passwd /tmp/evil.d2
review-diagrams /tmp/evil.d2
# Expected: ValueError - path outside allowed directory (realpath follows symlink)

# Test 3: Large file blocked
dd if=/dev/zero of=/tmp/huge.d2 bs=1M count=100
review-diagrams /tmp/huge.d2
# Expected: ValueError - file too large

# Test 4: Command injection still prevented
render-diagrams "test.d2; rm -rf /" output.svg
# Expected: Filename treated as literal, renders normally
```

---

## Deployment Checklist

- [ ] Phase 1: Apply path validation fixes to all 4 skills
- [ ] Add `validate_path()` helper function to shared library
- [ ] Add `read_diagram_safe()` helper function to shared library
- [ ] Update all `os.path.abspath()` calls to use `validate_path()`
- [ ] Add file size checks before reading diagrams
- [ ] Run security test suite (all tests must pass)
- [ ] Run existing skill tests (ensure no regressions)
- [ ] Manual security testing (verify fixes work)
- [ ] Update skill documentation with security notes
- [ ] Deploy to production
- [ ] Monitor for security issues

---

## Support & Questions

For security questions or to report vulnerabilities:
- Email: security@engram.dev
- Private: Use GitHub Security Advisories
- Public: Open GitHub issue with `security` label

**Do NOT publicly disclose critical vulnerabilities before patching.**

---

**Last Updated**: 2026-03-17
**Next Review**: 2026-04-01 (after Phase 1 deployment)
