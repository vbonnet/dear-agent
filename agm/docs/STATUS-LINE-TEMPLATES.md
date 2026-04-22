# AGM Status Line Template Gallery

## Pre-Made Templates

### Default Template
```
{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | #[fg={{.ContextColor}}]{{.ContextPercent}}%#[default] | {{.Branch}} (+{{.Uncommitted}}) | {{.SessionName}}
```
**Example Output:**
```
🤖 DONE | 73% | main (+3) | my-session
```

### Minimal Template
```
{{.AgentIcon}} {{.State}} | {{.ContextPercent}}%
```
**Example Output:**
```
🤖 DONE | 73%
```

### Compact Template
```
{{.AgentIcon}} #[fg={{.StateColor}}]●#[default] {{.ContextPercent}}% | {{.Branch}}
```
**Example Output:**
```
🤖 ● 73% | main
```

### Multi-Agent Template
```
{{.AgentIcon}}{{.AgentType}} | #[fg={{.StateColor}}]{{.State}}#[default] | {{.ContextPercent}}%
```
**Example Output:**
```
🤖claude | DONE | 73%
✨gemini | WORKING | 82%
```

### Full Template
```
{{.AgentIcon}} #[fg={{.StateColor}}]{{.State}}#[default] | CTX:#[fg={{.ContextColor}}]{{.ContextPercent}}%#[default] | {{.Branch}}(+{{.Uncommitted}}) | {{.SessionName}}
```
**Example Output:**
```
🤖 DONE | CTX:73% | main(+3) | my-session
```

## Custom Template Examples

### Context-Focused
```
CTX:#[fg={{.ContextColor}}]{{.ContextPercent}}%#[default] | {{.AgentIcon}}{{.State}}
```

### Git-Focused
```
{{.Branch}} +{{.Uncommitted}} | {{.AgentIcon}}{{.State}}
```

### Status-Only
```
#[fg={{.StateColor}}]{{.SessionName}}: {{.State}}#[default]
```

## Agent Icons

Customize in config.yaml:
```yaml
agent_icons:
  claude: "C"    # or 🤖
  gemini: "G"    # or ✨
  gpt: "P"       # or 🧠
  opencode: "O"  # or 💻
```
