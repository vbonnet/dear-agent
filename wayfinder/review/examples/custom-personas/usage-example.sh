#!/bin/bash

# Custom Personas Usage Example
# Demonstrates how to use custom personas with multi-persona-review

set -e

echo "============================================"
echo "  Custom Personas - Usage Example"
echo "============================================"
echo ""

# Get the plugin directory
PLUGIN_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
EXAMPLE_DIR="$(dirname "$0")"

echo "📍 Plugin directory: $PLUGIN_DIR"
echo "📍 Example directory: $EXAMPLE_DIR"
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

# Setup: Copy custom persona to project personas directory
echo "📦 Setting up custom persona..."
mkdir -p "$PLUGIN_DIR/.wayfinder/personas"
cp "$EXAMPLE_DIR/custom-security-expert.ai.md" "$PLUGIN_DIR/.wayfinder/personas/"
echo "✅ Custom persona installed to .wayfinder/personas/"
echo ""

# List available personas
echo "📋 Available personas:"
echo ""
cd "$PLUGIN_DIR"
npx multi-persona-review --list-personas | grep -E "(cloud-security|security-engineer|code-health)" || true
echo ""

# Example 1: Use custom persona only
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Example 1: Use custom persona only"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

cat > /tmp/test-aws-code.ts << 'EOF'
import * as s3 from '@aws-cdk/aws-s3';
import * as ec2 from '@aws-cdk/aws-ec2';

// Bad: Public S3 bucket without encryption
const bucket = new s3.Bucket(this, 'MyBucket', {
  publicReadAccess: true,
  encryption: s3.BucketEncryption.UNENCRYPTED
});

// Bad: Security group open to the world
const sg = new ec2.SecurityGroup(this, 'MySG', { vpc });
sg.addIngressRule(
  ec2.Peer.anyIpv4(),
  ec2.Port.tcp(22),
  'SSH from anywhere'
);

// Bad: Hardcoded credentials
const apiKey = 'AKIAIOSFODNN7EXAMPLE';
const password = 'MySecretPassword123';
EOF

echo "🔍 Reviewing with cloud-security-specialist only..."
echo ""

npx multi-persona-review /tmp/test-aws-code.ts \
  --personas cloud-security-specialist \
  --mode custom

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Example 2: Mix custom and built-in personas"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "🔍 Reviewing with cloud-security-specialist + security-engineer..."
echo ""

npx multi-persona-review /tmp/test-aws-code.ts \
  --personas cloud-security-specialist,security-engineer \
  --mode custom

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Example 3: JSON output for CI/CD"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

echo "🔍 Generating JSON report..."
echo ""

npx multi-persona-review /tmp/test-aws-code.ts \
  --personas cloud-security-specialist \
  --format json \
  > /tmp/review-results.json

echo "✅ Results saved to /tmp/review-results.json"
echo ""
echo "Sample output:"
cat /tmp/review-results.json | jq '.' | head -20
echo ""

# Cleanup
rm -f /tmp/test-aws-code.ts /tmp/review-results.json

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ Examples complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Key takeaways:"
echo "  1. Custom personas go in .wayfinder/personas/"
echo "  2. Use --personas to select specific personas"
echo "  3. Mix custom and built-in personas as needed"
echo "  4. Use --format json for programmatic use"
echo ""
echo "Next steps:"
echo "  - Edit custom-security-expert.ai.md to customize"
echo "  - Create personas for your specific tech stack"
echo "  - See ../programmatic-api/ for library usage"
