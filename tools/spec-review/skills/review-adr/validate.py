#!/usr/bin/env python3
"""Quick validation script for review-adr skill.
Tests basic functionality without pytest.
"""

import sys
import tempfile
from pathlib import Path

# Add paths
SKILL_DIR = Path(__file__).parent
PLUGIN_ROOT = SKILL_DIR.parent.parent
sys.path.insert(0, str(SKILL_DIR))
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))

from review_adr import ADRDocument, ADRValidator, generate_report
from cli_abstraction import CLIAbstraction


def test_basic_validation():
    """Test basic ADR validation."""
    print("=== Testing Basic ADR Validation ===\n")

    # Create test ADR
    test_adr = """# ADR-001: Use PostgreSQL

## Status
Accepted

## Context
We need a reliable database for user data with ACID guarantees.

## Decision
Use PostgreSQL as primary database.

## Consequences

### Positive
- ACID guarantees
- Strong consistency

### Negative
- Vertical scaling limits
- Higher ops complexity
"""

    with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
        f.write(test_adr)
        f.flush()
        temp_path = f.name

    try:
        # Validate
        cli = CLIAbstraction()
        validator = ADRValidator(cli)
        result = validator.validate_adr(temp_path)

        print(f"Success: {result['success']}")
        print(f"Score: {result['score_10']}/10 ({result['score_100']}/100)")
        print(f"Passed: {result['passed']}")
        print(f"Reviews: {len(result['reviews'])}")

        # Generate report
        print("\n--- Report Preview ---")
        report = generate_report(result, "markdown")
        print(report[:500])
        print("...")

        return result['success']

    finally:
        Path(temp_path).unlink()


def test_cli_adapters():
    """Test CLI adapter imports."""
    print("\n=== Testing CLI Adapters ===\n")

    adapters = ['claude-code', 'gemini', 'opencode', 'codex']
    for adapter in adapters:
        adapter_file = SKILL_DIR / "cli-adapters" / f"{adapter}.py"
        if adapter_file.exists():
            print(f"✓ {adapter}.py exists")
        else:
            print(f"✗ {adapter}.py missing")
            return False

    return True


def test_section_parsing():
    """Test ADR section parsing."""
    print("\n=== Testing Section Parsing ===\n")

    test_content = """# ADR-001: Test

## Status
Accepted

## Context
Test context

## Decision
Test decision

## Consequences
Test consequences
"""

    adr = ADRDocument("test.md", test_content)

    required = ['status', 'context', 'decision', 'consequences']
    for section in required:
        if adr.has_section(section):
            print(f"✓ Section '{section}' parsed")
        else:
            print(f"✗ Section '{section}' missing")
            return False

    return True


def test_cli_detection():
    """Test CLI detection."""
    print("\n=== Testing CLI Detection ===\n")

    cli = CLIAbstraction()
    print(f"Detected CLI: {cli.cli_type}")
    print(f"Batch size: {cli.get_batch_size()}")
    print(f"Supports caching: {cli.supports_feature('caching')}")

    return True


def main():
    """Run all validation tests."""
    print("review-adr Skill Validation")
    print("=" * 50)

    tests = [
        ("CLI Detection", test_cli_detection),
        ("Section Parsing", test_section_parsing),
        ("CLI Adapters", test_cli_adapters),
        ("Basic Validation", test_basic_validation),
    ]

    results = []
    for name, test_func in tests:
        try:
            result = test_func()
            results.append((name, result))
        except Exception as e:
            print(f"\n✗ {name} failed with error: {e}")
            results.append((name, False))

    # Summary
    print("\n" + "=" * 50)
    print("VALIDATION SUMMARY")
    print("=" * 50)

    passed = sum(1 for _, r in results if r)
    total = len(results)

    for name, result in results:
        status = "✓ PASS" if result else "✗ FAIL"
        print(f"{status}: {name}")

    print(f"\nTotal: {passed}/{total} tests passed")

    if passed == total:
        print("\n✓ All validations passed!")
        return 0
    else:
        print("\n✗ Some validations failed")
        return 1


if __name__ == "__main__":
    sys.exit(main())
