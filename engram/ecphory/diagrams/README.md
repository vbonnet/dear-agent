# Ecphory Architecture Diagrams

## C4 Component Diagram

**Source**: `c4-component-ecphory.d2`

This diagram shows the internal component architecture of the Ecphory module, including:
- 3-tier retrieval system (Filter → Rank → Budget)
- Multi-provider support (Anthropic, Vertex AI Claude, Vertex AI Gemini, Local)
- Failure boosting components (Context Detector, Failure Booster)
- Component relationships and data flow
- External dependencies

## Rendering

To render the diagrams to PNG and SVG, run:

```bash
./render.sh
```

Or manually:

```bash
# PNG (recommended for documentation)
d2 --layout elk --theme 200 c4-component-ecphory.d2 c4-component-ecphory.png

# SVG (recommended for web/interactive)
d2 --layout elk --theme 200 c4-component-ecphory.d2 c4-component-ecphory.svg
```

### Layout Options

- `elk` - Hierarchical layout (best for C4 diagrams with containers)
- `dagre` - Alternative directed graph layout
- `tala` - Force-directed layout

### Theme Options

- `200` - Cool gray (professional)
- `0` - Neutral default
- `100` - Flagship colorful
- `300` - Dark theme

## Files

- `c4-component-ecphory.d2` - D2 source (editable)
- `c4-component-ecphory.png` - Rendered PNG (generated)
- `c4-component-ecphory.svg` - Rendered SVG (generated)
- `render.sh` - Rendering script
- `README.md` - This file
