# pre-merge-commit Git Hook

A Git hook that runs CI checks before allowing merges to `main` or `master` branches.

## Features

- Detects merge commits targeting `main` or `master` branches
- Runs GitHub Actions workflows locally using `act`
- Automatically rolls back merge on CI failure
- Provides clear feedback with emojis for better UX

## Installation

### Global Installation (Recommended)

Build and install the hook globally:

```bash
# From the ai-tools repository root
go build -o ~/.local/bin/agm-pre-merge-commit ./cmd/agm-hooks/pre-merge-commit

# Make it executable
chmod +x ~/.local/bin/agm-pre-merge-commit
```

Then install in each repository:

```bash
cd /path/to/your/repo
ln -s ~/.local/bin/agm-pre-merge-commit .git/hooks/pre-merge-commit
```

### Per-Repository Installation

Install directly in a single repository:

```bash
# From the ai-tools repository root
go build -o /path/to/your/repo/.git/hooks/pre-merge-commit ./cmd/agm-hooks/pre-merge-commit

# Make it executable
chmod +x /path/to/your/repo/.git/hooks/pre-merge-commit
```

## How It Works

The hook triggers during `git merge` operations and:

1. Checks if a merge is in progress by looking for `.git/MERGE_HEAD`
2. Determines the target branch being merged into
3. If targeting `main` or `master`:
   - Runs `act pull_request` to execute GitHub Actions workflows locally
   - On success: allows the merge to proceed
   - On failure: runs `git reset --merge` to safely abort the merge

## Requirements

- **act**: The [nektos/act](https://github.com/nektos/act) tool must be installed
- **Docker**: act requires Docker to run workflows

Install act:

```bash
# macOS
brew install act

# Linux
curl https://raw.githubusercontent.com/nektos/act/master/install.sh | sudo bash
```

## Usage Examples

### Successful Merge

```bash
$ git checkout main
$ git merge feature-branch
🔍 Merge to main detected - running CI checks...
[act output showing workflow execution]
✅ CI checks passed - merge allowed
```

### Failed Merge (with rollback)

```bash
$ git checkout main
$ git merge buggy-feature
🔍 Merge to main detected - running CI checks...
[act output showing workflow execution]
❌ CI checks failed (exit 1)
[error output]
🔄 Rolling back merge...
```

### Merge to Feature Branch (no CI check)

```bash
$ git checkout feature/dev
$ git merge other-feature
[merge proceeds without CI checks]
```

## Testing

Run the test suite:

```bash
go test ./cmd/agm-hooks/pre-merge-commit/... -v
```

Run with coverage:

```bash
go test ./cmd/agm-hooks/pre-merge-commit/... -cover
```

## Configuration

The hook uses the following defaults:

- **Event Type**: `pull_request`
- **Workflow Path**: `.github/workflows/`
- **Working Directory**: Current directory (`.`)

To customize, modify the `main.go` file and rebuild.

## Troubleshooting

### Hook not triggering

Ensure the hook is:
1. Executable: `chmod +x .git/hooks/pre-merge-commit`
2. Named correctly: `.git/hooks/pre-merge-commit` (no `.sh` extension)

### act command not found

Install act using the instructions in the Requirements section.

### Docker not available

Ensure Docker daemon is running:

```bash
docker version
```

### Workflow not found

Ensure your repository has workflows in `.github/workflows/` directory.

## Exit Codes

- `0`: Success (merge allowed)
- `1`: CI checks failed or infrastructure error (merge aborted)

## Safety

The hook uses `git reset --merge` to rollback, which is safe and:
- Preserves uncommitted changes in the working directory
- Aborts the merge cleanly
- Does not lose any data

## Related

- [internal/ci/act/executor.go](../../../internal/ci/act/executor.go) - Act executor implementation
- [Task 1.8 Specification](../../../EXECUTION_PLAN.md) - Original task requirements
