# Visual Regression Baselines

This directory contains baseline PNG images for visual regression testing of diagram rendering.

## Overview

Visual regression testing ensures that changes to diagram tools, layout engines, or diagram-as-code syntax don't cause unintended visual changes.

## How It Works

1. **Baseline Creation**: Render example diagrams to PNG and store in this directory
2. **PR Testing**: On pull requests, render diagrams again and compare to baselines
3. **Diff Detection**: ImageMagick pixel-by-pixel comparison detects visual changes
4. **Threshold Evaluation**:
   - < 1%: Auto-pass (acceptable aliasing/rounding differences)
   - 1-5%: Flag for review (minor changes)
   - 5-20%: Flag for review (significant changes)
   - \> 20%: Fail (critical visual regression)

## Running Locally

```bash
# Run visual regression tests
bash scripts/visual-regression.sh

# Create/update baselines
rm -rf test/visual-baselines/*
bash scripts/visual-regression.sh
git add test/visual-baselines/
git commit -m "Update visual regression baselines"
```

## Baseline Files

Baselines are named after the source diagram file:

- `examples/microservices/c4-context.d2` → `c4-context.png`
- `examples/monolith/c4-container.dsl` → `c4-container.png`
- `examples/event-driven/architecture.mmd` → `architecture.png`

## When to Update Baselines

Update baselines when:
- Diagram content intentionally changed
- Upgraded diagram tools (D2, Mermaid, Structurizr CLI)
- Layout engine updated
- Styling improvements made

**DO NOT** update baselines to "make tests pass" without reviewing the visual diff!

## Diff Images

When tests fail, diff images are created in `test/visual-diffs/`:
- `{name}-diff.png` - Highlighted differences between baseline and current

Review these images to determine if the change is intentional.

## CI/CD Integration

GitHub Actions workflow (`.github/workflows/visual-regression.yml`) runs on PRs:
- Renders diagrams
- Compares to baselines
- Uploads diff images as artifacts
- Comments on PR with results

## Thresholds

Configured in `scripts/visual-regression.sh`:

```bash
THRESHOLD_PASS=0.01    # < 1% = auto-pass
THRESHOLD_FLAG=0.05    # 1-5% = flag
THRESHOLD_BLOCK=0.20   # > 20% = fail
```

Adjust if needed for your tolerance level.

## Dependencies

- **ImageMagick**: `compare` command for pixel-diff
- **D2**: Diagram rendering
- **Mermaid CLI** (`mmdc`): Mermaid diagram rendering
- **Structurizr CLI**: Structurizr diagram rendering (optional)

Install on Ubuntu/Debian:
```bash
sudo apt-get install imagemagick
go install oss.terrastruct.com/d2@latest
npm install -g @mermaid-js/mermaid-cli
```

## Troubleshooting

### "No baseline found"
Create baseline by running the script once. It will generate baseline images automatically.

### High diff percentage
Possible causes:
- Font rendering differences (different OS/environment)
- Anti-aliasing variations
- Layout engine randomness

Solutions:
- Use consistent environment (Docker/CI)
- Increase threshold slightly
- Lock diagram tool versions

### ImageMagick security policy
If ImageMagick blocks PNG operations, edit `/etc/ImageMagick-6/policy.xml`:
```xml
<!-- Comment out or remove PNG restrictions -->
<!-- <policy domain="coder" rights="none" pattern="PNG" /> -->
```

## Future Enhancements

Consider upgrading to Percy or Chromatic for:
- Better diff UI
- Baseline versioning
- Collaborative review workflow
- Cross-browser testing

For now, ImageMagick provides free, simple, effective visual regression testing.
