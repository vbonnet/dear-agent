#!/usr/bin/env bash
#
# Astrocyte Installation Script
# Installs systemd service and configures production deployment

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_FILE="$SCRIPT_DIR/astrocyte.service"
SYSTEMD_DIR="$HOME/.config/systemd/user"
CONFIG_DIR="$HOME/.agm/astrocyte"

echo "🧠 Astrocyte Installation Script"
echo "================================"
echo

# Check prerequisites
echo "✓ Checking prerequisites..."

if ! command -v python3 &> /dev/null; then
    echo "❌ Error: python3 not found"
    exit 1
fi

if ! command -v tmux &> /dev/null; then
    echo "❌ Error: tmux not found"
    exit 1
fi

if [[ ! -d "$SCRIPT_DIR/.venv" ]]; then
    echo "❌ Error: Virtual environment not found at $SCRIPT_DIR/.venv"
    echo "   Run: python3 -m venv .venv && source .venv/bin/activate && pip install -r requirements.txt"
    exit 1
fi

echo "   ✅ All prerequisites satisfied"
echo

# Create config directory
echo "✓ Creating configuration directory..."
mkdir -p "$CONFIG_DIR"
mkdir -p "$CONFIG_DIR/diagnoses"
echo "   ✅ Created $CONFIG_DIR"
echo

# Copy example config if no config exists
if [[ ! -f "$CONFIG_DIR/config.yaml" ]] && [[ ! -f "$CONFIG_DIR/config.json" ]]; then
    echo "✓ Copying example configuration..."
    cp "$SCRIPT_DIR/config.example.yaml" "$CONFIG_DIR/config.yaml"
    echo "   ✅ Created $CONFIG_DIR/config.yaml"
    echo "   📝 Edit this file to customize settings"
else
    echo "✓ Configuration file already exists, skipping..."
fi
echo

# Install systemd user service
echo "✓ Installing systemd user service..."
mkdir -p "$SYSTEMD_DIR"

# Replace %i with actual username in service file
SERVICE_CONTENT=$(cat "$SERVICE_FILE" | sed "s/%i/$USER/g")
echo "$SERVICE_CONTENT" > "$SYSTEMD_DIR/astrocyte.service"

systemctl --user daemon-reload
echo "   ✅ Service installed to $SYSTEMD_DIR/astrocyte.service"
echo

# Enable and start service
echo "✓ Enabling and starting service..."
systemctl --user enable astrocyte.service
systemctl --user start astrocyte.service
echo "   ✅ Service enabled and started"
echo

# Show status
echo "✓ Service status:"
systemctl --user status astrocyte.service --no-pager || true
echo

echo "================================"
echo "✅ Installation complete!"
echo
echo "Commands:"
echo "  systemctl --user status astrocyte    # Check status"
echo "  systemctl --user stop astrocyte      # Stop daemon"
echo "  systemctl --user restart astrocyte   # Restart daemon"
echo "  journalctl --user -u astrocyte -f    # Follow logs"
echo
echo "Configuration:"
echo "  $CONFIG_DIR/config.yaml"
echo
echo "Logs:"
echo "  $CONFIG_DIR/incidents.jsonl           # Incident log"
echo "  $CONFIG_DIR/diagnoses/                # Diagnosis files"
echo "  journalctl --user -u astrocyte        # Service logs"
echo
