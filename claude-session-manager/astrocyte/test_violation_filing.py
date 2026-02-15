#!/usr/bin/env python3
"""
Test script for violation filing functionality
Tests the new load_patterns, file_violation, and tier1_example features
"""
import sys
import os
from pathlib import Path

# Add astrocyte to path
sys.path.insert(0, str(Path(__file__).parent))

from astrocyte import load_patterns, file_violation

def test_load_patterns():
    """Test loading all pattern types"""
    print("Testing pattern loading...")

    for pattern_type in ['bash', 'beads', 'git']:
        print(f"\nLoading {pattern_type} patterns...")
        patterns = load_patterns(pattern_type)

        if patterns is None:
            print(f"  ❌ Failed to load {pattern_type} patterns")
            return False

        if 'patterns' not in patterns:
            print(f"  ❌ No 'patterns' key in {pattern_type} data")
            return False

        pattern_count = len(patterns['patterns'])
        print(f"  ✅ Loaded {pattern_count} {pattern_type} patterns")

        # Check for tier1_example in some patterns
        patterns_with_examples = sum(1 for p in patterns['patterns'] if 'tier1_example' in p)
        print(f"  ℹ️  {patterns_with_examples}/{pattern_count} patterns have tier1_example")

        # Show first pattern with tier1_example
        for p in patterns['patterns']:
            if 'tier1_example' in p:
                print(f"\n  Example pattern '{p['id']}':")
                print(f"  Reason: {p.get('reason', 'N/A')}")
                print(f"  Alternative: {p.get('alternative', 'N/A')}")
                print(f"  Tier1 Example:")
                for line in p['tier1_example'].strip().split('\n'):
                    print(f"    {line}")
                break

    return True


def test_file_violation():
    """Test violation filing"""
    print("\n\nTesting violation filing...")

    # Test filing a bash violation
    filepath = file_violation(
        pattern_id='cd-chaining',
        command='cd /repo && git push',
        session_id='test-session',
        agent_type='general-purpose',
        pattern_type='bash'
    )

    if filepath:
        print(f"  ✅ Violation filed: {filepath}")

        # Read and verify the file
        if os.path.exists(filepath):
            with open(filepath, 'r') as f:
                content = f.read()

            print(f"  ✅ Violation file exists ({len(content)} bytes)")

            # Check for required sections
            required_sections = [
                '# Violation Report:',
                '## Context',
                '## Violation Details',
                '## Why It Happened',
                '## Recovery',
                '## Proposed Fix'
            ]

            for section in required_sections:
                if section in content:
                    print(f"  ✅ Found section: {section}")
                else:
                    print(f"  ❌ Missing section: {section}")

            # Clean up test file
            os.remove(filepath)
            print(f"  ✅ Cleaned up test file")
        else:
            print(f"  ❌ Violation file not found at {filepath}")
            return False
    else:
        print(f"  ❌ Violation filing returned None")
        return False

    return True


def main():
    """Run all tests"""
    print("=" * 70)
    print("Astrocyte Violation Filing Integration Tests")
    print("=" * 70)

    tests = [
        ("Pattern Loading", test_load_patterns),
        ("Violation Filing", test_file_violation),
    ]

    results = {}
    for name, test_func in tests:
        try:
            results[name] = test_func()
        except Exception as e:
            print(f"\n❌ {name} failed with exception: {e}")
            import traceback
            traceback.print_exc()
            results[name] = False

    # Summary
    print("\n" + "=" * 70)
    print("Test Summary")
    print("=" * 70)

    passed = sum(1 for r in results.values() if r)
    total = len(results)

    for name, result in results.items():
        status = "✅ PASS" if result else "❌ FAIL"
        print(f"{status}: {name}")

    print(f"\n{passed}/{total} tests passed")

    return 0 if passed == total else 1


if __name__ == '__main__':
    sys.exit(main())
