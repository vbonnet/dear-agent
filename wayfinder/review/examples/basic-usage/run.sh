#!/bin/bash

# Basic Usage Example - Multi-Persona Code Review
# This script demonstrates a simple CLI usage

set -e

echo "============================================"
echo "  Multi-Persona Code Review - Basic Usage"
echo "============================================"
echo ""

# Check if API key is configured
if [ -z "$ANTHROPIC_API_KEY" ] && [ -z "$VERTEX_PROJECT_ID" ]; then
  echo "❌ Error: No API credentials configured"
  echo ""
  echo "Please set one of the following:"
  echo "  export ANTHROPIC_API_KEY=your_key          # For Anthropic Claude"
  echo "  export VERTEX_PROJECT_ID=your_project      # For VertexAI"
  echo ""
  exit 1
fi

# Get the plugin directory
PLUGIN_DIR="$(cd "$(dirname "$0")/../.." && pwd)"

echo "📍 Plugin directory: $PLUGIN_DIR"
echo "📄 Reviewing: sample-code.ts"
echo ""

# Run the review
cd "$PLUGIN_DIR"

if [ ! -d "dist" ]; then
  echo "⚠️  Plugin not built. Building now..."
  npm run build
  echo ""
fi

echo "🔍 Running multi-persona review..."
echo ""

# Execute the CLI
npx multi-persona-review examples/basic-usage/sample-code.ts \
  --mode quick \
  --format text

echo ""
echo "✅ Review complete!"
echo ""
echo "Next steps:"
echo "  - Try: npx multi-persona-review sample-code.ts --mode thorough"
echo "  - Try: npx multi-persona-review sample-code.ts --format json"
echo "  - See: examples/ci-cd-integration/ for GitHub Actions setup"
