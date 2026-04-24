# Term Denylist Pre-Commit Hook

Prevent accidental commits of internal terminology, secrets patterns, or banned phrases.

## Overview

The term denylist pre-commit hook reads a configurable list of forbidden terms and blocks commits if any staged changes contain matches. This helps maintain code quality and prevent sensitive information from being committed.

## Setup

### 1. Create a Denylist File

Copy the example denylist to `.agm/term-denylist.txt`:

```bash
cp .agm/term-denylist.txt.example .agm/term-denylist.txt
```

Edit `.agm/term-denylist.txt` to add your organization's terms:

```
# Internal terminology
TODO_REMOVE
HACK_FIXME

# Secret patterns
password=
API_KEY=
secret_key
```

### 2. Register the Hook

Add the hook to your git repository's pre-commit hooks:

```bash
# Option A: Manual setup
cp agm/hooks/precommit-term-denylist.sh .git/hooks/pre-commit

# Option B: Use AGM hook manager
agm hook register precommit-term-denylist.sh
```

### 3. Make Hook Executable

```bash
chmod +x .git/hooks/pre-commit
```

## Denylist Format

- **One pattern per line**
- **Blank lines are ignored**
- **Comments start with `#`**
- **Patterns are regex** (matched with grep)

### Example Denylist

```
# Internal markers
TODO_REMOVE
HACK_FIXME
TEMP_INTERNAL

# Credential patterns
password=
secret_key=
api_key=
AWS_SECRET

# Regex patterns for specific formats
sk-[a-zA-Z0-9]{20,}        # OpenAI key format
AKIA[0-9A-Z]{16}           # AWS access key format
ghp_[a-zA-Z0-9]{36}        # GitHub personal token format
```

## Usage

When you try to commit with denylisted terms in staged changes:

```bash
$ git commit -m "Add new feature"
🚫 Commit blocked: Denylisted terms detected

❌ Term 'TODO_REMOVE' found in:
  - src/handler.go

Please remove these terms before committing.
To ignore this check, edit .agm/term-denylist.txt
```

### Bypassing the Hook

To skip this hook for a specific commit:

```bash
git commit --no-verify -m "message"
```

⚠️ **Note**: Use `--no-verify` sparingly and only when you have a good reason.

## Behavior

- **If `.agm/term-denylist.txt` doesn't exist**: Hook is skipped silently
- **If a match is found**: Commit is blocked with details of which file contains the violation
- **Patterns are regex**: Use grep regex syntax for pattern matching
- **Case-sensitive**: By default, patterns are case-sensitive

## Security Considerations

- **This is not a secret scanner**: For production secret scanning, use dedicated tools like git-secrets or TruffleHog
- **Regex patterns can be complex**: Test your patterns locally before adding them
- **False positives are possible**: Keep your denylist maintained and remove overly broad patterns

## Troubleshooting

### Hook doesn't block commits
- Check that `.agm/term-denylist.txt` exists
- Verify the file has proper permissions: `ls -la .agm/term-denylist.txt`
- Check that patterns use valid grep regex syntax
- Test a pattern manually: `git diff --cached | grep "your_pattern"`

### Too many false positives
- Refine your regex patterns to be more specific
- Comment out overly broad patterns with `#`
- Use anchors (`^`, `$`) and word boundaries (`\b`) to match exact terms

### Need case-insensitive matching
- Modify the hook to add `-i` flag to grep:
  ```bash
  if echo "$STAGED_DIFF" | grep -i "$term"; then
  ```

## See Also

- [git-secrets](https://github.com/awslabs/git-secrets) - AWS tool for preventing secrets in commits
- [Gitleaks](https://github.com/gitleaks/gitleaks) - Scan git repos for secrets
- [TruffleHog](https://github.com/trufflesecurity/trufflehog) - Find and verify secrets
