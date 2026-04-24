#!/bin/bash
# Pre-commit hook for diagram validation
# Install: cp templates/pre-commit-diagram-validation.sh .git/hooks/pre-commit
# Make executable: chmod +x .git/hooks/pre-commit

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "🔍 Validating diagrams..."

# Find staged diagram files
STAGED_DIAGRAMS=$(git diff --cached --name-only --diff-filter=ACM | grep -E '\.(d2|dsl|mmd)$' || true)

if [ -z "$STAGED_DIAGRAMS" ]; then
  echo "${GREEN}✓${NC} No diagram changes to validate"
  exit 0
fi

FAILED=0

for diagram in $STAGED_DIAGRAMS; do
  echo ""
  echo "Checking $diagram..."

  # 1. Syntax validation
  EXT="${diagram##*.}"
  case $EXT in
    d2)
      if ! d2 compile --dry-run "$diagram" >/dev/null 2>&1; then
        echo "${RED}✗${NC} D2 syntax error in $diagram"
        FAILED=1
        continue
      fi
      ;;
    dsl)
      if command -v structurizr-cli >/dev/null 2>&1; then
        if ! structurizr-cli validate "$diagram" >/dev/null 2>&1; then
          echo "${RED}✗${NC} Structurizr DSL syntax error in $diagram"
          FAILED=1
          continue
        fi
      fi
      ;;
    mmd)
      if ! mmdc --quiet --input "$diagram" --output /tmp/test.png 2>/dev/null; then
        echo "${RED}✗${NC} Mermaid syntax error in $diagram"
        FAILED=1
        continue
      fi
      ;;
  esac

  # 2. Sync validation (if diagram-sync available)
  if command -v python3 >/dev/null 2>&1 && [ -f "skills/diagram-sync/diagram_sync.py" ]; then
    SYNC_RESULT=$(python3 skills/diagram-sync/diagram_sync.py "$diagram" . --json 2>/dev/null || echo '{"sync_score": 0}')
    SYNC_SCORE=$(echo "$SYNC_RESULT" | python3 -c "import sys, json; print(json.load(sys.stdin).get('sync_score', 0))")

    if (( $(echo "$SYNC_SCORE < 70" | bc -l) )); then
      echo "${YELLOW}⚠${NC}  Sync score: $SYNC_SCORE/100 (below 70%)"
      echo "${YELLOW}⚠${NC}  Consider updating diagram to match codebase"
      # Warning only, don't fail commit
    else
      echo "${GREEN}✓${NC} Sync score: $SYNC_SCORE/100"
    fi
  fi

  echo "${GREEN}✓${NC} $diagram validated"
done

if [ $FAILED -eq 1 ]; then
  echo ""
  echo "${RED}✗ Diagram validation failed${NC}"
  echo "Fix syntax errors before committing"
  exit 1
fi

echo ""
echo "${GREEN}✓ All diagrams validated${NC}"
exit 0
