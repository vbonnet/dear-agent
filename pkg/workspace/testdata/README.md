# Test Data

This directory contains sample configuration files for testing the workspace detection library.

## Files

- `valid_config.yaml` - A valid configuration with multiple workspaces
- `invalid_version.yaml` - Config with unsupported version number
- `duplicate_names.yaml` - Config with duplicate workspace names (should fail validation)
- `no_workspaces.yaml` - Config with empty workspaces list (should fail validation)
- `minimal_config.yaml` - Minimal valid configuration
- `with_env_vars.yaml` - Config using environment variables in paths

## Usage

These files are referenced by the test suite to verify config loading and validation behavior.
