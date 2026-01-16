# GitHub Actions Workflows

## CSM E2E Installation Tests

### Setup Requirements

The `csm-e2e-install.yml` workflow requires access to the private `vbonnet/engram` repository.

#### Configure Repository Secret

1. **Create a Personal Access Token (PAT)**:
   - Go to https://github.com/settings/tokens
   - Click "Generate new token" → "Generate new token (classic)"
   - Name: `ENGRAM_ACCESS_TOKEN`
   - Scopes: Select `repo` (Full control of private repositories)
   - Click "Generate token" and copy the token

2. **Add Secret to Repository**:
   - Go to https://github.com/vbonnet/ai-tools/settings/secrets/actions
   - Click "New repository secret"
   - Name: `ENGRAM_ACCESS_TOKEN`
   - Value: Paste the PAT from step 1
   - Click "Add secret"

#### Workflow Behavior

- **With `ENGRAM_ACCESS_TOKEN`**: Checks out both ai-tools and engram, runs full E2E installation tests
- **Without secret**: Fallback to `GITHUB_TOKEN` (will fail if engram is private)

### What Gets Tested

- **Ubuntu 22.04**: CSM installation from local source
- **Debian 12**: CSM installation from local source

Each test verifies:
1. Binary builds successfully
2. CSM command is available in PATH
3. `csm version` command works
4. Binary is installed to correct location (~/go/bin/csm)
