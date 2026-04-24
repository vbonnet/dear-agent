# Skill Dependency Resolution System (Python)

## Overview

The skill dependency resolution system provides automatic dependency management for skills in the spec-review-marketplace. It handles:

- **Dependency graph construction** from `skill.yml` files
- **Circular dependency detection** to prevent infinite loops
- **Topological sorting** for correct load order
- **Automatic skill loading** when dependencies are required

## Architecture

### Components

1. **lib/dependency_resolver.py** - Core dependency resolution library
2. **skill.yml** - Skill metadata files declaring dependencies
3. **SkillDependencyResolver** - Main resolver class

### Dependency Graph

The system builds a directed graph where:
- Nodes = Skills
- Edges = Dependencies (A → B means "A depends on B")

Example:
```
review-spec → create-spec
create-spec → review-architecture
```

## Declaring Dependencies

Dependencies are declared in each skill's `skill.yml` file:

### Format 1: List (Recommended)

```yaml
name: my-skill
version: 1.0.0
description: My awesome skill
dependencies:
  - dependency-skill-1
  - dependency-skill-2
```

### Format 2: Inline Array

```yaml
name: my-skill
version: 1.0.0
dependencies: [dependency-skill-1, dependency-skill-2]
```

### Format 3: No Dependencies

```yaml
name: my-skill
version: 1.0.0
dependencies: []
```

### External Dependencies

External dependencies (Python packages) are automatically filtered out:

```yaml
name: my-skill
dependencies:
  - cli-abstraction  # Filtered (external library)
  - anthropic        # Filtered (external library)
  - other-skill      # Kept (skill dependency)
```

## Usage

### Basic Usage

```python
from lib.dependency_resolver import SkillDependencyResolver

# Create resolver instance (auto-discovers skills)
resolver = SkillDependencyResolver()

# Load a skill with all its dependencies
resolver.load_skill_with_dependencies("review-spec")
```

### Using Convenience Functions

```python
from lib.dependency_resolver import (
    load_skill_with_dependencies,
    print_dependency_tree,
    validate_dependencies
)

# Load skill using default resolver
load_skill_with_dependencies("review-spec")

# Print dependency tree
print_dependency_tree("review-spec")

# Validate all dependencies
if validate_dependencies():
    print("✓ All dependencies valid")
```

## API Reference

### Class: `SkillDependencyResolver`

#### Constructor

```python
resolver = SkillDependencyResolver(plugin_root: Optional[Path] = None)
```

Initialize resolver. If `plugin_root` is None, auto-detects from file location.

#### Methods

##### `has_dependencies(skill_name: str) -> bool`
Check if a skill has dependencies.

```python
if resolver.has_dependencies("review-spec"):
    print("Has dependencies")
```

##### `get_dependencies(skill_name: str) -> List[str]`
Get list of dependencies for a skill.

```python
deps = resolver.get_dependencies("review-spec")
print(f"Dependencies: {', '.join(deps)}")
```

##### `is_skill_loaded(skill_name: str) -> bool`
Check if a skill is already loaded.

```python
if resolver.is_skill_loaded("review-spec"):
    print("Already loaded")
```

##### `detect_circular_dependency(skill_name: str)`
Check for circular dependencies.

```python
from lib.dependency_resolver import CircularDependencyError

try:
    resolver.detect_circular_dependency("my-skill")
    print("No circular dependencies")
except CircularDependencyError as e:
    print(f"Error: {e}")
```

##### `resolve_dependency_order(skill_name: str) -> List[str]`
Get skills in load order (topological sort).

```python
order = resolver.resolve_dependency_order("review-spec")
print(f"Load order: {' → '.join(order)}")
```

##### `load_skill(skill_name: str)`
Load a single skill.

```python
from lib.dependency_resolver import DependencyNotFoundError

try:
    resolver.load_skill("review-spec")
except DependencyNotFoundError as e:
    print(f"Error: {e}")
```

##### `load_skill_with_dependencies(skill_name: str)`
Load a skill and all its dependencies.

```python
try:
    resolver.load_skill_with_dependencies("review-spec")
    print("✓ Loaded successfully")
except (CircularDependencyError, DependencyNotFoundError) as e:
    print(f"Error: {e}")
```

##### `print_dependency_tree(skill_name: str, indent_level: int = 0)`
Print dependency tree for debugging.

```python
resolver.print_dependency_tree("review-spec")
# Output:
# review-spec
#   create-spec
#     review-architecture
```

##### `get_reverse_dependencies(target_skill: str) -> List[str]`
Get skills that depend on this skill.

```python
reverse = resolver.get_reverse_dependencies("review-architecture")
print(f"Depended on by: {', '.join(reverse)}")
```

##### `validate_dependencies() -> bool`
Validate that all dependencies exist.

```python
if resolver.validate_dependencies():
    print("✓ All dependencies valid")
else:
    print("✗ Invalid dependencies found")
```

##### `get_dependency_graph() -> Dict[str, List[str]]`
Get the complete dependency graph.

```python
graph = resolver.get_dependency_graph()
for skill, deps in graph.items():
    print(f"{skill}: {deps}")
```

##### `get_all_skills() -> List[str]`
Get list of all discovered skills.

```python
skills = resolver.get_all_skills()
print(f"Available skills: {', '.join(sorted(skills))}")
```

## Examples

### Example 1: Simple Skill with Dependencies

**File: skills/my-skill/skill.yml**
```yaml
name: my-skill
version: 1.0.0
description: A skill that depends on review-architecture
dependencies:
  - review-architecture
```

**File: skills/my-skill/my_skill.py**
```python
#!/usr/bin/env python3
"""My awesome skill."""

import sys
from pathlib import Path

# Add lib to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))

from dependency_resolver import load_skill_with_dependencies

# Auto-load dependencies
load_skill_with_dependencies("my-skill")

def main():
    # Now you can use functions from review-architecture
    print("My skill is running!")

if __name__ == "__main__":
    main()
```

### Example 2: Complex Dependency Chain

```yaml
# skills/skill-a/skill.yml
name: skill-a
dependencies: []

# skills/skill-b/skill.yml
name: skill-b
dependencies:
  - skill-a

# skills/skill-c/skill.yml
name: skill-c
dependencies:
  - skill-b
  - skill-a
```

```python
from lib.dependency_resolver import SkillDependencyResolver

resolver = SkillDependencyResolver()

# Load order when loading skill-c
order = resolver.resolve_dependency_order("skill-c")
print(order)  # ['skill-a', 'skill-b', 'skill-c']
```

### Example 3: Error Handling

```python
from lib.dependency_resolver import (
    SkillDependencyResolver,
    CircularDependencyError,
    DependencyNotFoundError
)

resolver = SkillDependencyResolver()

try:
    # Try to load a skill
    resolver.load_skill_with_dependencies("my-skill")

except CircularDependencyError as e:
    print(f"Circular dependency: {e}")
    # Handle circular dependency

except DependencyNotFoundError as e:
    print(f"Missing dependency: {e}")
    # Handle missing dependency

except Exception as e:
    print(f"Unexpected error: {e}")
    # Handle other errors
```

### Example 4: CLI Usage

The resolver includes a CLI interface:

```bash
# List all skills and their dependencies
python3 lib/dependency_resolver.py list

# Print dependency tree
python3 lib/dependency_resolver.py tree review-spec

# Validate dependencies
python3 lib/dependency_resolver.py validate

# Load a skill (for testing)
python3 lib/dependency_resolver.py load review-spec
```

## Best Practices

### 1. Use Type Hints

```python
from typing import List
from lib.dependency_resolver import SkillDependencyResolver

def process_skills(resolver: SkillDependencyResolver) -> List[str]:
    """Process skills with proper typing."""
    return resolver.get_all_skills()
```

### 2. Handle Exceptions Properly

```python
from lib.dependency_resolver import (
    CircularDependencyError,
    DependencyNotFoundError
)

def safe_load_skill(skill_name: str) -> bool:
    """Safely load a skill with error handling."""
    try:
        resolver.load_skill_with_dependencies(skill_name)
        return True
    except CircularDependencyError:
        print(f"Circular dependency in {skill_name}")
        return False
    except DependencyNotFoundError:
        print(f"Missing dependency for {skill_name}")
        return False
```

### 3. Validate Before Deployment

```python
#!/usr/bin/env python3
"""Pre-deployment validation script."""

import sys
from lib.dependency_resolver import get_resolver

def main():
    resolver = get_resolver()

    if not resolver.validate_dependencies():
        print("✗ Invalid dependencies detected")
        sys.exit(1)

    print("✓ All dependencies valid")
    sys.exit(0)

if __name__ == "__main__":
    main()
```

### 4. Use Default Resolver for Simple Cases

```python
# Instead of creating new instances
from lib.dependency_resolver import load_skill_with_dependencies

# Use convenience function with default resolver
load_skill_with_dependencies("my-skill")
```

### 5. Create Custom Resolver for Testing

```python
from pathlib import Path
from lib.dependency_resolver import SkillDependencyResolver

def test_my_skill():
    """Test with custom test environment."""
    test_root = Path("/tmp/test-skills")
    resolver = SkillDependencyResolver(plugin_root=test_root)

    # Test with isolated environment
    assert resolver.validate_dependencies()
```

## Testing

### Unit Tests

Run the test suite:

```bash
cd engram/plugins/spec-review-marketplace
python3 tests/test_dependency_resolver.py
```

Or with pytest:

```bash
pytest tests/test_dependency_resolver.py -v
```

### Manual Testing

```python
from lib.dependency_resolver import SkillDependencyResolver

# Create resolver
resolver = SkillDependencyResolver()

# Check discovered skills
print("Skills:", resolver.get_all_skills())

# Print dependency tree
resolver.print_dependency_tree("review-spec")

# Validate dependencies
assert resolver.validate_dependencies()
```

## Error Messages

### CircularDependencyError

```
CircularDependencyError: Circular dependency detected: skill-a -> skill-b -> skill-c -> skill-a
```

**Solution:** Refactor skills to break the cycle.

### DependencyNotFoundError

```
DependencyNotFoundError: Unknown skill: nonexistent-skill. Available skills: review-spec, review-adr, ...
```

**Solution:** Check skill name spelling or create the missing skill.

### Missing Main Script

```
DependencyNotFoundError: Main script not found for skill: my-skill
```

**Solution:** Create `my_skill.py` or `my-skill.py` in the skill directory.

## Performance

- **Discovery:** O(n) where n = number of skills
- **Circular detection:** O(V + E) where V = skills, E = dependencies
- **Topological sort:** O(V + E)
- **Load time:** O(V + E) + import time

For typical marketplace sizes (10-50 skills), initialization is < 200ms.

## Troubleshooting

### Skill Not Discovered

**Problem:** Skill doesn't appear in dependency graph

**Debug:**
```python
resolver = SkillDependencyResolver()
print("Skills dir:", resolver.skills_dir)
print("Discovered:", resolver.get_all_skills())
```

**Solutions:**
1. Ensure `skill.yml` exists
2. Check YAML syntax
3. Verify directory structure: `skills/skill-name/skill.yml`

### Import Errors

**Problem:** `ModuleNotFoundError` when loading skills

**Debug:**
```python
import sys
print("Python path:", sys.path)
```

**Solutions:**
1. Ensure skill directory is added to `sys.path`
2. Check main script naming (use underscores not hyphens)
3. Verify `__init__.py` if using packages

### YAML Parsing Errors

**Problem:** Dependencies not parsed correctly

**Debug:**
```python
import yaml
with open("skills/my-skill/skill.yml") as f:
    print(yaml.safe_load(f))
```

**Solutions:**
1. Validate YAML syntax
2. Use lists or inline arrays for dependencies
3. Check indentation (use spaces, not tabs)

## Advanced Usage

### Custom Plugin Root

```python
from pathlib import Path
from lib.dependency_resolver import SkillDependencyResolver

# Use custom plugin root
custom_root = Path("/path/to/custom/plugins")
resolver = SkillDependencyResolver(plugin_root=custom_root)
```

### Programmatic Dependency Analysis

```python
def analyze_dependencies(resolver):
    """Analyze dependency graph for insights."""
    graph = resolver.get_dependency_graph()

    # Find skills with most dependencies
    most_deps = max(graph.items(), key=lambda x: len(x[1]))
    print(f"Most dependencies: {most_deps[0]} ({len(most_deps[1])} deps)")

    # Find skills with no dependencies
    no_deps = [s for s, d in graph.items() if len(d) == 0]
    print(f"Leaf skills: {', '.join(no_deps)}")

    # Find most depended-on skills
    dep_counts = {}
    for skill, deps in graph.items():
        for dep in deps:
            dep_counts[dep] = dep_counts.get(dep, 0) + 1

    most_used = max(dep_counts.items(), key=lambda x: x[1])
    print(f"Most used: {most_used[0]} ({most_used[1]} dependents)")
```

### Integration with CI/CD

```python
#!/usr/bin/env python3
"""CI/CD validation script."""

import sys
from lib.dependency_resolver import get_resolver

def ci_validate():
    """Validate dependencies in CI pipeline."""
    resolver = get_resolver()

    # Validate dependencies exist
    if not resolver.validate_dependencies():
        print("::error::Invalid dependencies detected")
        return False

    # Check for circular dependencies
    for skill in resolver.get_all_skills():
        try:
            resolver.detect_circular_dependency(skill)
        except Exception as e:
            print(f"::error::Circular dependency in {skill}: {e}")
            return False

    print("::notice::All dependency checks passed")
    return True

if __name__ == "__main__":
    sys.exit(0 if ci_validate() else 1)
```

## Related Documentation

- [CLI Abstraction Layer (Python)](./lib/cli_abstraction.py)
- [Skill Development Guide](./README.md)
- [Plugin Architecture](./marketplace.json)
- [Bash Version](../multi-persona-review-marketplace/DEPENDENCY-RESOLUTION.md)
