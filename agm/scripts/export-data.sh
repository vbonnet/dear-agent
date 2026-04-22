#!/bin/bash
# AGM Pre-Uninstall Hook
# Exports session data before component uninstall

set -e

BACKUP_FILE=$1

echo "AGM Data Export (Pre-Uninstall)"
echo "Backup file: ${BACKUP_FILE}"
echo

# Ensure backup directory exists
BACKUP_DIR=$(dirname "${BACKUP_FILE}")
mkdir -p "${BACKUP_DIR}"

# Step 1: Export data from Dolt database
# NOTE: In actual implementation, this would:
#   1. Connect to workspace Dolt database
#   2. Export all agm_* tables to SQL dump
#   3. Create tarball with SQL dump + metadata
echo "✓ Exporting session data from Dolt (placeholder)"

# Step 2: Include session history files (if any legacy JSONL files remain)
SESSION_DIR="${HOME}/.agm/sessions"
if [ -d "${SESSION_DIR}" ]; then
    echo "✓ Including legacy session files from ${SESSION_DIR}"
fi

# Step 3: Create backup tarball
# NOTE: In actual implementation, create tarball with:
#   - SQL dump of agm_sessions, agm_messages, agm_tool_calls, etc.
#   - metadata.json with component version, export timestamp
#   - restore.sh script for re-import
echo "✓ Creating backup tarball (placeholder)"

# Step 4: Generate restore instructions
cat > "${BACKUP_DIR}/RESTORE.txt" << 'EOF'
# AGM Data Restore Instructions

To restore this backup:

1. Re-install AGM component:
   component-installer install agm

2. Import data from backup:
   agm-restore /path/to/backup/agm-export-<timestamp>.tar.gz

3. Verify restoration:
   agm list

Note: Restore will only work if AGM component is installed in the workspace.
EOF

echo "✓ Generated restore instructions: ${BACKUP_DIR}/RESTORE.txt"

echo
echo "✅ Data export complete"
echo "Backup saved to: ${BACKUP_FILE}"
echo "Restore instructions: ${BACKUP_DIR}/RESTORE.txt"
