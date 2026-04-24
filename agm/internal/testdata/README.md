# Test Fixtures

This directory contains test fixtures for AGM session recovery features.

## Directory Structure

- `orphan-recovery/` - Orphaned session scenarios (conversation exists, no manifest)
- `corrupted-history/` - Corrupted history.jsonl files (null bytes, malformed JSON)
- `mock-manifests/` - AGM manifest fixtures (valid, corrupted, stale)
- `file-provenance/` - History.jsonl with file modification tracking

## Usage

These fixtures support unit tests, integration tests, and BDD scenarios for:
- `agm admin find-orphans`
- `agm session import`
- `agm admin audit`
- `agm admin trace-files`
- `agm session search`
- Gate 9 (bow integration)
- `agm admin doctor` integration

## Maintenance

When adding new fixtures:
1. Document the scenario being tested
2. Include both positive and negative test cases
3. Update this README with fixture descriptions
4. Ensure fixtures are realistic (based on production patterns)

## See Also

- `TESTING.md`
- Test BDD features in `test/bdd/features/`
