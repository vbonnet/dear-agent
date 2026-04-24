#!/bin/bash
# Render C4 Component diagram to PNG and SVG

set -e

DIAGRAM_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIAGRAM_DIR"

echo "Rendering C4 Component diagram..."

# Render to PNG
d2 --theme=200 --layout=elk c4-component-devlog.d2 c4-component-devlog.png
echo "✓ Created c4-component-devlog.png"

# Render to SVG
d2 --theme=200 --layout=elk c4-component-devlog.d2 c4-component-devlog.svg
echo "✓ Created c4-component-devlog.svg"

echo ""
echo "Diagram rendering complete!"
echo "Files created:"
echo "  - c4-component-devlog.png"
echo "  - c4-component-devlog.svg"
