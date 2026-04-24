#!/usr/bin/env bash
set -euo pipefail

# AGM (ai-tools) Uninstall Script
# Removes: binaries, hooks, commands, config, cache, systemd units, settings entries

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

KEEP_CONFIG=false
DRY_RUN=false

for arg in "$@"; do
    case "$arg" in
        --keep-config) KEEP_CONFIG=true ;;
        --dry-run) DRY_RUN=true ;;
        --help|-h)
            echo "Usage: uninstall.sh [--keep-config] [--dry-run]"
            echo ""
            echo "Options:"
            echo "  --keep-config  Keep configuration files (~/.agm/config.yaml, ~/.claude/settings.json)"
            echo "  --dry-run      Show what would be removed without actually removing anything"
            exit 0
            ;;
    esac
done

log_info()    { printf "${YELLOW}  %s${NC}\n" "$1"; }
log_success() { printf "${GREEN}  %s${NC}\n" "$1"; }
log_remove()  { printf "${RED}  - %s${NC}\n" "$1"; }

do_rm() {
    if [ "$DRY_RUN" = true ]; then
        log_remove "[dry-run] Would remove: $1"
    else
        rm -rf "$1" 2>/dev/null && log_remove "Removed: $1" || true
    fi
}

echo "=== AGM Uninstall ==="
if [ "$DRY_RUN" = true ]; then
    echo "(dry-run mode — no changes will be made)"
fi
echo ""

# 1. Remove AGM binary
log_info "Removing AGM binary..."
do_rm "$HOME/go/bin/agm"

# 2. Remove configure-claude-settings binary
log_info "Removing configure-claude-settings binary..."
do_rm "$HOME/go/bin/configure-claude-settings"

# 3. Remove AGM hooks from ~/.claude/hooks/
log_info "Removing AGM hooks..."
AGM_HOOKS=(
    "posttool-agm-state-notify"
    "pretool-agm-mode-tracker"
    "agm-pretool-test-session-guard"
    "session-start/agm-state-ready"
    "session-start/agm-plan-continuity"
)
for hook in "${AGM_HOOKS[@]}"; do
    do_rm "$HOME/.claude/hooks/$hook"
done

# 4. Remove AGM slash commands
log_info "Removing AGM commands..."
if [ -d "$HOME/.claude/commands" ]; then
    for cmd_file in "$HOME/.claude/commands"/agm-*.md; do
        [ -f "$cmd_file" ] && do_rm "$cmd_file"
    done
fi

# 5. Remove hook entries from settings.json
log_info "Removing AGM hook entries from settings.json..."
if command -v configure-claude-settings >/dev/null 2>&1 && [ "$DRY_RUN" = false ]; then
    for hook in "${AGM_HOOKS[@]}"; do
        hook_path="~/.claude/hooks/$hook"
        # Determine event type from hook name
        case "$hook" in
            posttool-*) event="PostToolUse" ;;
            pretool-*)  event="PreToolUse" ;;
            session-start*) event="SessionStart" ;;
            *) continue ;;
        esac
        configure-claude-settings remove-hook "$event" "$hook_path" 2>/dev/null || true
    done
    log_success "Cleaned settings.json hook entries"
elif [ "$DRY_RUN" = true ]; then
    log_remove "[dry-run] Would remove AGM hook entries from ~/.claude/settings.json"
else
    log_info "configure-claude-settings not found — manually edit ~/.claude/settings.json"
fi

# 6. Remove cache
log_info "Removing AGM cache..."
do_rm "$HOME/.agm/mode-cache"
do_rm "$HOME/.agm/ready-*"

# 7. Remove systemd units (if installed)
log_info "Removing systemd units..."
if [ -f "$HOME/.config/systemd/user/agm-monitor.service" ]; then
    if [ "$DRY_RUN" = false ]; then
        systemctl --user stop agm-monitor.service 2>/dev/null || true
        systemctl --user disable agm-monitor.service 2>/dev/null || true
    fi
    do_rm "$HOME/.config/systemd/user/agm-monitor.service"
    do_rm "$HOME/.config/systemd/user/agm-monitor.timer"
fi

# 8. Remove config (unless --keep-config)
if [ "$KEEP_CONFIG" = true ]; then
    log_info "Keeping configuration files (--keep-config)"
else
    log_info "Removing AGM configuration..."
    do_rm "$HOME/.agm/config.yaml"
    do_rm "$HOME/.agm/sessions"
fi

echo ""
if [ "$DRY_RUN" = true ]; then
    log_info "Dry run complete. No changes were made."
else
    log_success "AGM uninstall complete."
fi
echo ""
echo "Note: The ~/.agm directory may still contain session data."
echo "To fully remove: rm -rf ~/.agm"
