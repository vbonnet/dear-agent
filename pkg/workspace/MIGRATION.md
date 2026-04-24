# Workspace Protocol Migration Guide

## Overview

This guide helps migrate from the workspace detection prototype to the full workspace protocol with CLI, environment isolation, and git integration.

## What's Changed

### Before (Prototype)
- ✅ Basic workspace detection (6-priority)
- ✅ YAML config loading
- ❌ No CLI interface
- ❌ No environment isolation
- ❌ No git config integration
- ❌ No unified registry
- ❌ Per-component configs (duplicated)

### After (Full Protocol)
- ✅ All prototype features preserved
- ✅ CLI with 10 commands + JSON output
- ✅ Environment variable isolation (`.env` files)
- ✅ Git config integration helpers
- ✅ Unified registry (`~/.workspace/registry.yaml`)
- ✅ 7-level settings cascade
- ✅ Core protocol settings (6 required)
- ✅ Multi-language support (via CLI)

## Migration Steps

### Step 1: Initialize Registry

**If you have no existing config:**

```bash
# Initialize new registry
workspace init

# Output:
# Workspace registry initialized successfully
#   Path: ~/.workspace/registry.yaml
```

**If you have existing component configs:**

The registry can coexist with component configs. The protocol uses:
- **Registry**: Single source of truth for workspace definitions
- **Component configs**: Tool-specific overrides and customization

### Step 2: Add Workspaces

```bash
# Add your workspaces
workspace add oss --root ~/projects/myworkspace --enabled
workspace add acme --root ~/src/ws/acme --enabled --set-default

# Verify
workspace list
```

**Example registry after adding workspaces:**

```yaml
# ~/.workspace/registry.yaml
version: 1
protocol_version: "1.0.0"
default_workspace: acme

default_settings:
  log_level: info
  log_format: text

workspaces:
  - name: oss
    root: ~/projects/myworkspace
    enabled: true
    output_dir: ./output

  - name: acme
    root: ~/src/ws/acme
    enabled: true
    output_dir: ~/src/ws/acme/output
```

### Step 3: Migrate Environment Variables

**Before**: Project-level `.env` files

```bash
# Old structure
./.env
~/src/ws/acme/.env
```

**After**: Workspace-level env files in central location

```bash
# New structure
~/.workspace/envs/oss.env
~/.workspace/envs/acme.env
```

**Migration:**

```bash
# Copy existing .env files
cp ./.env ~/.workspace/envs/oss.env
cp ~/src/ws/acme/.env ~/.workspace/envs/acme.env

# Add workspace variables
cat >> ~/.workspace/envs/oss.env <<EOF
export WORKSPACE_ROOT=~/projects/myworkspace
export WORKSPACE_NAME=oss
export WORKSPACE_ENABLED=true
EOF

# Set correct permissions
chmod 600 ~/.workspace/envs/*.env
```

**Verify:**

```bash
# Generate activation script
workspace activate oss

# Output includes:
# export WORKSPACE_ROOT=~/projects/myworkspace
# export WORKSPACE_NAME=oss
# export OPENAI_API_KEY=...
```

### Step 4: Set Up Git Config (Optional)

**Manual setup** (recommended for understanding):

```bash
# Create workspace-specific git configs
cat > ~/.gitconfig-oss <<EOF
[user]
    name = User Name
    email = personal@example.com
    signingkey = ~/.ssh/id_ed25519_personal

[commit]
    gpgsign = true
EOF

cat > ~/.gitconfig-acme <<EOF
[user]
    name = User Name (Acme Corp)
    email = user@acme.com
    signingkey = ~/.ssh/acme_id_ed25519

[commit]
    gpgsign = true
EOF

# Add includeIf to global git config
cat >> ~/.gitconfig <<EOF

[includeIf "gitdir:./"]
    path = ~/.gitconfig-oss

[includeIf "gitdir:~/src/ws/acme/"]
    path = ~/.gitconfig-acme
EOF
```

**Programmatic setup** (using helpers):

```go
// In your Go code
gitMgr := workspace.NewGitConfigManager()

// Create workspace git config
config := workspace.GitConfig{
    UserName:   "User Name",
    UserEmail:  "personal@example.com",
    SigningKey: "~/.ssh/id_ed25519_personal",
    CommitSign: true,
}
configPath, err := gitMgr.GenerateGitConfigFile("oss", config)

// Add includeIf to global config
err = gitMgr.AddGitIncludeIf("~/projects/myworkspace", configPath)
```

**Verify:**

```bash
# Test git config
cd ./project
git config user.email
# Output: personal@example.com

cd ~/src/ws/acme/project
git config user.email
# Output: user@acme.com

# Run health check
workspace doctor
```

### Step 5: Update Component Configs (Optional)

Components can reference the registry and add overrides:

```yaml
# ~/.agm/config.yaml

# Reference registry
workspace_registry: ~/.workspace/registry.yaml

# Component global settings (Level 4)
log_level: error
ui_theme: dark

# Workspace-specific overrides (Level 5)
workspace_overrides:
  oss:
    log_level: trace              # Override for oss workspace
    agm_cache_size: 5000          # Component-specific setting

  acme:
    log_level: warn               # Override for acme workspace
    agm_cache_size: 1000
```

## Breaking Changes

### None

All existing functionality is preserved. The extension is 100% backward compatible:

- ✅ All 83 existing tests pass
- ✅ Existing detection algorithm works unchanged
- ✅ YAML config format compatible
- ✅ No API changes to exported functions

New features are additive only.

## Updated Workflow

### Before (Prototype)

```go
// In Go code
import "github.com/vbonnet/engram/core/pkg/workspace"

// Detect workspace
detector, _ := workspace.NewDetector("~/.agm/config.yaml")
ws, _ := detector.Detect(pwd, "")

// Use workspace
outputDir := ws.OutputDir
```

### After (Full Protocol)

**Option 1: Use Go library (existing code works unchanged)**

```go
// Exact same code as before
import "github.com/vbonnet/engram/core/pkg/workspace"

detector, _ := workspace.NewDetector("~/.agm/config.yaml")
ws, _ := detector.Detect(pwd, "")
```

**Option 2: Use CLI from any language**

```typescript
// TypeScript example
import { execSync } from 'child_process';

const output = execSync('workspace detect').toString();
const workspace = JSON.parse(output);
console.log(workspace.name); // "oss"
```

```python
# Python example
import subprocess, json

output = subprocess.check_output(['workspace', 'detect'])
workspace = json.loads(output)
print(workspace['name'])  # "oss"
```

**Option 3: Use with environment activation**

```bash
# Activate workspace in shell
eval "$(workspace activate oss)"

# Environment is now set
echo $WORKSPACE_ROOT       # ~/projects/myworkspace
echo $WORKSPACE_NAME       # oss
echo $OPENAI_API_KEY       # sk-personal-key-123
```

**Option 4: Use settings resolver**

```go
// Use 7-level cascade
registry, _ := workspace.LoadRegistry("")
ws, _ := registry.GetWorkspaceByName("oss")

resolver := workspace.NewSettingsResolver(registry, ws)

// Resolve with cascade
logLevel := resolver.ResolveSetting("log_level", "info")

// Debug cascade
cascade := resolver.GetCascade("log_level", "info")
for _, level := range cascade {
    if level.Active {
        fmt.Printf("Level %d (%s): %v\n", level.Level, level.Source, level.Value)
    }
}
```

## Validation

After migration, verify everything works:

```bash
# 1. Check registry
workspace list
workspace validate oss
workspace validate acme

# 2. Test detection
cd ./project
workspace detect
# Should output: "oss"

cd ~/src/ws/acme/project
workspace detect
# Should output: "acme"

# 3. Test activation
eval "$(workspace activate oss)"
echo $WORKSPACE_NAME  # Should be "oss"

# 4. Test git config
cd ./project
git config user.email  # Should match oss email

cd ~/src/ws/acme/project
git config user.email  # Should match acme email

# 5. Run health check
workspace doctor
# All checks should pass
```

## Troubleshooting

### Issue: "Registry not found"

```bash
# Solution: Initialize registry
workspace init
```

### Issue: "No workspace detected"

```bash
# Check: Are you in a workspace directory?
pwd

# Check: Is workspace in registry?
workspace list

# Solution: Add workspace
workspace add myworkspace --root $(pwd)
```

### Issue: "Git email is wrong"

```bash
# Check: What git sees
git config user.email

# Check: What workspace expects
workspace doctor

# Solution: Verify includeIf patterns
cat ~/.gitconfig | grep includeIf

# Pattern must match directory
# Wrong: [includeIf "gitdir:~/projects/myworkspace"]  (missing trailing /)
# Right: [includeIf "gitdir:./"]
```

### Issue: "Environment variables not loaded"

```bash
# Check: Does env file exist?
ls -la ~/.workspace/envs/

# Check: Correct permissions?
ls -la ~/.workspace/envs/oss.env
# Should be: -rw------- (0600)

# Solution: Fix permissions
chmod 600 ~/.workspace/envs/*.env

# Solution: Re-activate
eval "$(workspace activate oss)"
```

### Issue: "Validation fails"

```bash
# Get detailed errors
workspace validate oss --format json | jq '.results'

# Common fixes:
# - Workspace root doesn't exist: mkdir -p <root>
# - Permissions wrong: chmod 600 ~/.workspace/envs/*.env
# - Git config missing: Set up includeIf
```

## Rollback (If Needed)

The protocol is backward compatible, so rollback is simple:

1. **Remove registry** (optional):
   ```bash
   rm -rf ~/.workspace/
   ```

2. **Revert to component configs** (if you changed them):
   ```bash
   git checkout -- ~/.agm/config.yaml
   ```

3. **Existing code continues to work**: No code changes needed

## Summary

Migration is **opt-in and gradual**:

1. ✅ Initialize registry: `workspace init`
2. ✅ Add workspaces: `workspace add ...`
3. ⏸️ Migrate env vars (optional): Copy to `~/.workspace/envs/`
4. ⏸️ Set up git config (optional): Add includeIf
5. ⏸️ Update component configs (optional): Add overrides

**Key insight**: You can adopt features incrementally. The protocol works at any stage.

## Next Steps

1. Read: `pkg/workspace/README.md` - Full feature documentation
2. Try: CLI commands - `workspace --help`
3. Explore: Settings cascade - `workspace settings get oss log_level`
4. Health check: `workspace doctor`

## Questions?

See specifications in:
- `specs/workspace-protocol.md`
- `specs/workspace-protocol-cli.md`
- Other spec files in the same directory
