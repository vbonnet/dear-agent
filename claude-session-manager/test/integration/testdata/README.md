# Test Fixtures

This directory contains test data fixtures for integration tests.

## Manifests

Test manifest files for validation testing:

### `manifests/valid-v2.yaml`
Valid v2 schema manifest with all required fields. Used to test successful manifest parsing.

### `manifests/missing-session-id.yaml`
Invalid manifest missing the `session_id` field. Used to test error handling for incomplete manifests.

### `manifests/invalid-schema.yaml`
Old v1 schema manifest. Used to test schema version validation and migration logic.

## Usage

Load fixtures in tests:

```go
manifestPath := "testdata/manifests/valid-v2.yaml"
manifest, err := manifest.Read(manifestPath)
```
