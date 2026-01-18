# GitHub Actions Workflows

## CSM E2E Installation Tests

The `csm-e2e-install.yml` workflow tests CSM installation from source across multiple Linux distributions.

No special setup or secrets required - the workflow runs automatically on push/PR.

### What Gets Tested

- **Ubuntu 22.04**: CSM installation from local source
- **Debian 12**: CSM installation from local source

Each test verifies:
1. Binary builds successfully
2. CSM command is available in PATH
3. `csm version` command works
4. Binary is installed to correct location (~/go/bin/csm)
