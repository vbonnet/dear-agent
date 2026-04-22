#!/bin/bash
# Git pre-commit hook: Block commits containing denylisted terms
#
# This hook reads a denylist of terms/patterns from .agm/term-denylist.txt
# and blocks commits if any staged changes contain matching terms.
#
# Denylist format: one term/regex per line, blank lines and # comments ignored

set -e

# Find .agm/term-denylist.txt in repository root
REPO_ROOT=$(git rev-parse --show-toplevel)
DENYLIST_FILE="$REPO_ROOT/.agm/term-denylist.txt"

# If denylist doesn't exist, skip validation
if [ ! -f "$DENYLIST_FILE" ]; then
    exit 0
fi

# Get the staged diff
STAGED_DIFF=$(git diff --cached)

# Track if we found any violations
VIOLATIONS_FOUND=0
VIOLATION_DETAILS=""

# Read denylist file line by line
while IFS= read -r line || [ -n "$line" ]; do
    # Skip empty lines and comments
    [[ -z "$line" || "$line" =~ ^[[:space:]]*# ]] && continue

    # Trim whitespace
    term=$(echo "$line" | xargs)

    # Search for the term in staged diff
    if echo "$STAGED_DIFF" | grep -q "$term"; then
        VIOLATIONS_FOUND=1
        # Get details of which files contain the violation
        matching_files=$(git diff --cached --name-only --diff-filter=ACM | while read file; do
            if git show ":$file" 2>/dev/null | grep -q "$term"; then
                echo "  - $file"
            fi
        done)

        VIOLATION_DETAILS="$VIOLATION_DETAILS❌ Term '$term' found in:
$matching_files
"
    fi
done < "$DENYLIST_FILE"

# If violations found, block commit
if [ $VIOLATIONS_FOUND -eq 1 ]; then
    echo "🚫 Commit blocked: Denylisted terms detected" >&2
    echo "" >&2
    echo "$VIOLATION_DETAILS" >&2
    echo "Please remove these terms before committing." >&2
    echo "To ignore this check, edit .agm/term-denylist.txt" >&2
    exit 1
fi

exit 0
