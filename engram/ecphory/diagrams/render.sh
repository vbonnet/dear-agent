#!/bin/bash
# Render D2 diagrams to PNG and SVG

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "Rendering C4 Component diagram..."

# Render PNG with elk layout (better for complex hierarchical diagrams)
d2 --layout elk --theme 200 \
  "$SCRIPT_DIR/c4-component-ecphory.d2" \
  "$SCRIPT_DIR/c4-component-ecphory.png"

# Render SVG
d2 --layout elk --theme 200 \
  "$SCRIPT_DIR/c4-component-ecphory.d2" \
  "$SCRIPT_DIR/c4-component-ecphory.svg"

echo "✓ Rendered c4-component-ecphory.png"
echo "✓ Rendered c4-component-ecphory.svg"
