# GitHub Actions Workflows

## AGM E2E Installation Tests

The `csm-e2e-install.yml` workflow tests AGM installation from source across multiple Linux distributions.

No special setup or secrets required - the workflow runs automatically on push/PR.

### What Gets Tested

- **Ubuntu 22.04**: AGM installation from local source
- **Debian 12**: AGM installation from local source

Each test verifies:
1. Binary builds successfully
2. AGM command is available in PATH
3. `csm version` command works
4. Binary is installed to correct location (~/go/bin/csm)
