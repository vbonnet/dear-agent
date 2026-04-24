# Dependency Resolution Examples (Python)

This document provides practical examples of using the Python skill dependency resolution system.

## Example 1: Basic Skill Loading

### Scenario
You have a skill `analyze-spec` that depends on `review-spec` to validate specifications.

### Setup

**skills/analyze-spec/skill.yml:**
```yaml
name: analyze-spec
version: 1.0.0
description: Advanced spec analysis using review-spec
dependencies:
  - review-spec
```

**skills/analyze-spec/analyze_spec.py:**
```python
#!/usr/bin/env python3
"""Advanced spec analysis tool."""

import sys
from pathlib import Path

# Add lib to path
sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))

from dependency_resolver import load_skill_with_dependencies

# Load dependencies automatically
load_skill_with_dependencies("analyze-spec")

# Now review_spec module is available
import review_spec

def analyze_spec(spec_path: str) -> dict:
    """
    Analyze a SPEC.md file with additional metrics.

    Args:
        spec_path: Path to SPEC.md file

    Returns:
        Analysis results
    """
    print(f"Analyzing {spec_path}")

    # Use review-spec functionality
    result = review_spec.validate_spec(spec_path)

    # Additional analysis...
    analysis = {
        "validation": result,
        "metrics": compute_additional_metrics(spec_path)
    }

    return analysis

def compute_additional_metrics(spec_path: str) -> dict:
    """Compute additional metrics."""
    # Implementation...
    return {"complexity": 0.8, "completeness": 0.9}

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: analyze_spec.py <spec_path>")
        sys.exit(1)

    result = analyze_spec(sys.argv[1])
    print(f"Analysis complete: {result}")
```

### Usage

```bash
python3 skills/analyze-spec/analyze_spec.py SPEC.md
```

Output:
```
Loading dependency: review-spec
Loaded skill: review-spec
Loaded skill: analyze-spec
Analyzing SPEC.md
...
```

## Example 2: Multi-Level Dependencies

### Scenario
You have three skills with dependencies:
- `file-utils` (base skill, no dependencies)
- `review-architecture` (depends on `file-utils`)
- `create-architecture` (depends on `review-architecture` and `file-utils`)

### Setup

**skills/file-utils/skill.yml:**
```yaml
name: file-utils
version: 1.0.0
description: Common file utility functions
dependencies: []
```

**skills/file-utils/file_utils.py:**
```python
"""Common file utilities."""

from pathlib import Path
from typing import List, Optional

def find_markdown_files(directory: Path) -> List[Path]:
    """Find all markdown files in directory."""
    return list(directory.glob("**/*.md"))

def read_file_safe(file_path: Path) -> Optional[str]:
    """Safely read file contents."""
    try:
        return file_path.read_text(encoding='utf-8')
    except Exception as e:
        print(f"Error reading {file_path}: {e}")
        return None

def get_file_info(file_path: Path) -> dict:
    """Get file metadata."""
    stat = file_path.stat()
    return {
        "size": stat.st_size,
        "modified": stat.st_mtime,
        "path": str(file_path)
    }
```

**skills/review-architecture/skill.yml:**
```yaml
name: review-architecture
version: 1.0.0
description: Review ARCHITECTURE.md files
dependencies:
  - file-utils
```

**skills/review-architecture/review_architecture.py:**
```python
"""ARCHITECTURE.md review tool."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))
from dependency_resolver import load_skill_with_dependencies

# Load dependencies
load_skill_with_dependencies("review-architecture")

# Import dependencies
from file_utils import find_markdown_files, read_file_safe

def review_architecture(arch_path: Path) -> dict:
    """Review architecture file."""
    content = read_file_safe(arch_path)

    if not content:
        return {"error": "Could not read file"}

    # Review logic...
    return {"score": 8.5, "issues": []}

def find_architecture_files(directory: Path) -> list:
    """Find all ARCHITECTURE.md files."""
    all_md = find_markdown_files(directory)
    return [f for f in all_md if f.name == "ARCHITECTURE.md"]
```

**skills/create-architecture/skill.yml:**
```yaml
name: create-architecture
version: 1.0.0
description: Create ARCHITECTURE.md files
dependencies:
  - review-architecture
  - file-utils
```

**skills/create-architecture/create_architecture.py:**
```python
"""ARCHITECTURE.md creation tool."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))
from dependency_resolver import load_skill_with_dependencies

# Load dependencies
load_skill_with_dependencies("create-architecture")

# Import dependencies
from file_utils import get_file_info
from review_architecture import review_architecture

def create_architecture(output_path: Path, template: str = "default") -> None:
    """Create ARCHITECTURE.md file."""
    # Create content
    content = generate_architecture_content(template)

    # Write file
    output_path.write_text(content, encoding='utf-8')

    # Validate created file
    result = review_architecture(output_path)

    # Get file info
    info = get_file_info(output_path)

    print(f"Created {output_path}")
    print(f"Size: {info['size']} bytes")
    print(f"Validation score: {result['score']}")

def generate_architecture_content(template: str) -> str:
    """Generate architecture content."""
    # Implementation...
    return "# Architecture\n\n..."
```

### Load Order

```python
from lib.dependency_resolver import SkillDependencyResolver

resolver = SkillDependencyResolver()
order = resolver.resolve_dependency_order("create-architecture")
print(order)
# Output: ['file-utils', 'review-architecture', 'create-architecture']
```

### Verification

```python
from lib.dependency_resolver import SkillDependencyResolver

resolver = SkillDependencyResolver()

# Print dependency tree
resolver.print_dependency_tree("create-architecture")
# Output:
# create-architecture
#   review-architecture
#     file-utils
#   file-utils
```

## Example 3: Avoiding Circular Dependencies

### Bad Example (Circular)

❌ **DON'T DO THIS:**

**skills/skill-a/skill.yml:**
```yaml
name: skill-a
dependencies:
  - skill-b  # skill-a depends on skill-b
```

**skills/skill-b/skill.yml:**
```yaml
name: skill-b
dependencies:
  - skill-a  # skill-b depends on skill-a (CIRCULAR!)
```

**Result:**
```python
from lib.dependency_resolver import load_skill_with_dependencies, CircularDependencyError

try:
    load_skill_with_dependencies("skill-a")
except CircularDependencyError as e:
    print(e)
    # Output: Circular dependency detected: skill-a -> skill-b -> skill-a
```

### Good Example (Refactored)

✅ **DO THIS INSTEAD:**

Extract common functionality into a base skill:

**skills/common-utils/skill.yml:**
```yaml
name: common-utils
version: 1.0.0
description: Common utilities
dependencies: []
```

**skills/common-utils/common_utils.py:**
```python
"""Common utilities used by multiple skills."""

def shared_function():
    """Function used by both skill-a and skill-b."""
    return "shared data"
```

**skills/skill-a/skill.yml:**
```yaml
name: skill-a
dependencies:
  - common-utils  # Both depend on common-utils
```

**skills/skill-b/skill.yml:**
```yaml
name: skill-b
dependencies:
  - common-utils  # Not on each other
```

## Example 4: Error Handling

### Comprehensive Error Handling

```python
#!/usr/bin/env python3
"""Example with comprehensive error handling."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))

from dependency_resolver import (
    SkillDependencyResolver,
    CircularDependencyError,
    DependencyNotFoundError,
)

def safe_load_skill(skill_name: str) -> bool:
    """
    Safely load a skill with comprehensive error handling.

    Args:
        skill_name: Name of skill to load

    Returns:
        True if successful, False otherwise
    """
    try:
        resolver = SkillDependencyResolver()
        resolver.load_skill_with_dependencies(skill_name)
        print(f"✓ Successfully loaded {skill_name}")
        return True

    except CircularDependencyError as e:
        print(f"✗ Circular dependency error: {e}")
        print("  → Refactor skills to break the dependency cycle")
        return False

    except DependencyNotFoundError as e:
        print(f"✗ Missing dependency: {e}")
        print("  → Check that all dependencies are installed")
        return False

    except ImportError as e:
        print(f"✗ Import error: {e}")
        print("  → Check Python module paths and naming")
        return False

    except Exception as e:
        print(f"✗ Unexpected error: {e}")
        print("  → Check logs for details")
        return False

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: safe_load.py <skill_name>")
        sys.exit(1)

    success = safe_load_skill(sys.argv[1])
    sys.exit(0 if success else 1)
```

## Example 5: Debugging Dependencies

### Dependency Analysis Script

**scripts/analyze_dependencies.py:**
```python
#!/usr/bin/env python3
"""Analyze skill dependencies."""

import sys
from pathlib import Path
from collections import Counter

sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from dependency_resolver import SkillDependencyResolver

def analyze_dependencies():
    """Analyze and report on skill dependencies."""
    resolver = SkillDependencyResolver()

    print("=== Dependency Analysis ===\n")

    # Get all skills
    skills = resolver.get_all_skills()
    print(f"Total skills: {len(skills)}\n")

    # Analyze dependency graph
    graph = resolver.get_dependency_graph()

    # Find skills with most dependencies
    dep_counts = {skill: len(deps) for skill, deps in graph.items()}
    if dep_counts:
        most_deps = max(dep_counts.items(), key=lambda x: x[1])
        print(f"Most dependencies: {most_deps[0]} ({most_deps[1]} deps)")

    # Find skills with no dependencies (leaf skills)
    leaf_skills = [s for s, d in graph.items() if len(d) == 0]
    print(f"Leaf skills (no deps): {', '.join(sorted(leaf_skills))}\n")

    # Find most depended-on skills
    all_deps = []
    for deps in graph.values():
        all_deps.extend(deps)

    dep_counter = Counter(all_deps)
    if dep_counter:
        most_used = dep_counter.most_common(3)
        print("Most depended-on skills:")
        for skill, count in most_used:
            print(f"  {skill}: {count} dependents")

    print("\n=== Detailed Dependency Trees ===\n")

    # Print tree for each top-level skill
    for skill in sorted(skills):
        if len(dep_counts.get(skill, [])) > 0:  # Only show skills with deps
            print(f"\n{skill}:")
            resolver.print_dependency_tree(skill, indent_level=1)

if __name__ == "__main__":
    analyze_dependencies()
```

Output:
```
=== Dependency Analysis ===

Total skills: 5

Most dependencies: create-architecture (2 deps)
Leaf skills (no deps): file-utils

Most depended-on skills:
  file-utils: 3 dependents
  review-architecture: 1 dependents

=== Detailed Dependency Trees ===

create-architecture:
  create-architecture
    review-architecture
      file-utils
    file-utils

review-architecture:
  review-architecture
    file-utils
```

## Example 6: Validation Script

### Pre-Deployment Validation

**scripts/validate_dependencies.py:**
```python
#!/usr/bin/env python3
"""Validate skill dependencies before deployment."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from dependency_resolver import (
    SkillDependencyResolver,
    CircularDependencyError,
)

def validate_all_dependencies() -> bool:
    """
    Validate all skill dependencies.

    Returns:
        True if all validations pass, False otherwise
    """
    resolver = SkillDependencyResolver()

    print("=== Dependency Validation ===\n")

    # 1. Check all dependencies exist
    print("1. Checking for missing dependencies...")
    if not resolver.validate_dependencies():
        print("   ✗ Validation failed: Missing dependencies")
        return False
    print("   ✓ All dependencies exist\n")

    # 2. Check for circular dependencies
    print("2. Checking for circular dependencies...")
    all_skills = resolver.get_all_skills()

    for skill in all_skills:
        try:
            resolver.detect_circular_dependency(skill)
        except CircularDependencyError as e:
            print(f"   ✗ Circular dependency in {skill}: {e}")
            return False

    print("   ✓ No circular dependencies found\n")

    # 3. Report statistics
    print("3. Dependency Statistics:")
    graph = resolver.get_dependency_graph()

    total_deps = sum(len(deps) for deps in graph.values())
    avg_deps = total_deps / len(graph) if graph else 0

    print(f"   Total skills: {len(graph)}")
    print(f"   Total dependencies: {total_deps}")
    print(f"   Average deps per skill: {avg_deps:.2f}\n")

    print("=== ✓ All validation checks passed ===")
    return True

if __name__ == "__main__":
    success = validate_all_dependencies()
    sys.exit(0 if success else 1)
```

## Example 7: Complex Real-World Scenario

### Scenario
Building a comprehensive spec validation system with multiple analyzers.

### Dependency Graph

```
                spec-validator-comprehensive
                         |
         +---------------+---------------+
         |               |               |
  syntax-checker  semantic-analyzer  style-checker
         |               |               |
         +-------+-------+               |
                 |                       |
            text-parser              file-utils
                 |
            file-utils
```

### Setup Files

**skills/file-utils/skill.yml:**
```yaml
name: file-utils
version: 1.0.0
dependencies: []
```

**skills/text-parser/skill.yml:**
```yaml
name: text-parser
version: 1.0.0
dependencies:
  - file-utils
```

**skills/syntax-checker/skill.yml:**
```yaml
name: syntax-checker
version: 1.0.0
dependencies:
  - text-parser
```

**skills/semantic-analyzer/skill.yml:**
```yaml
name: semantic-analyzer
version: 1.0.0
dependencies:
  - text-parser
```

**skills/style-checker/skill.yml:**
```yaml
name: style-checker
version: 1.0.0
dependencies:
  - file-utils
```

**skills/spec-validator-comprehensive/skill.yml:**
```yaml
name: spec-validator-comprehensive
version: 1.0.0
dependencies:
  - syntax-checker
  - semantic-analyzer
  - style-checker
```

### Implementation

**skills/spec-validator-comprehensive/spec_validator_comprehensive.py:**
```python
#!/usr/bin/env python3
"""Comprehensive spec validator."""

import sys
from pathlib import Path
from typing import Dict, Any

sys.path.insert(0, str(Path(__file__).parent.parent.parent / "lib"))
from dependency_resolver import load_skill_with_dependencies

# Load all dependencies (6 skills total)
load_skill_with_dependencies("spec-validator-comprehensive")

# Import dependencies
import syntax_checker
import semantic_analyzer
import style_checker

def validate_spec_comprehensive(spec_path: Path) -> Dict[str, Any]:
    """
    Comprehensively validate a SPEC.md file.

    Args:
        spec_path: Path to SPEC.md file

    Returns:
        Comprehensive validation results
    """
    print(f"=== Comprehensive Spec Validation ===")
    print(f"File: {spec_path}\n")

    results = {}

    # Run syntax checking
    print("Running syntax checks...")
    results["syntax"] = syntax_checker.check_syntax(spec_path)

    # Run semantic analysis
    print("Running semantic analysis...")
    results["semantics"] = semantic_analyzer.analyze_semantics(spec_path)

    # Run style checking
    print("Running style checks...")
    results["style"] = style_checker.check_style(spec_path)

    # Aggregate results
    overall_score = calculate_overall_score(results)
    results["overall_score"] = overall_score

    print(f"\n=== Validation Complete ===")
    print(f"Overall Score: {overall_score}/10")

    return results

def calculate_overall_score(results: Dict[str, Any]) -> float:
    """Calculate overall validation score."""
    # Implementation...
    return 8.5

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: spec_validator_comprehensive.py <spec_path>")
        sys.exit(1)

    spec_path = Path(sys.argv[1])
    result = validate_spec_comprehensive(spec_path)

    # Exit with code based on score
    sys.exit(0 if result["overall_score"] >= 7.0 else 1)
```

### Verify Load Order

```python
from lib.dependency_resolver import SkillDependencyResolver

resolver = SkillDependencyResolver()
order = resolver.resolve_dependency_order("spec-validator-comprehensive")

print("Load order:")
for i, skill in enumerate(order, 1):
    print(f"  {i}. {skill}")
```

Output:
```
Load order:
  1. file-utils
  2. text-parser
  3. syntax-checker
  4. semantic-analyzer
  5. style-checker
  6. spec-validator-comprehensive
```

## Example 8: Testing with Dependency Injection

### Using Custom Resolver for Tests

```python
#!/usr/bin/env python3
"""Test example with dependency injection."""

import unittest
import tempfile
from pathlib import Path

from lib.dependency_resolver import SkillDependencyResolver

class TestWithDependencies(unittest.TestCase):
    """Test skills with custom dependency resolver."""

    def setUp(self):
        """Set up test environment."""
        # Create temporary test directory
        self.test_dir = Path(tempfile.mkdtemp())
        self.skills_dir = self.test_dir / "skills"
        self.skills_dir.mkdir()

        # Create test skills
        self._create_test_skill("base-skill", [])
        self._create_test_skill("dependent-skill", ["base-skill"])

        # Create custom resolver for testing
        self.resolver = SkillDependencyResolver(plugin_root=self.test_dir)

    def _create_test_skill(self, name: str, deps: list):
        """Create a test skill."""
        skill_dir = self.skills_dir / name
        skill_dir.mkdir()

        # Create skill.yml
        deps_yaml = "\n  - ".join([""] + deps) if deps else "[]"
        (skill_dir / "skill.yml").write_text(f"""
name: {name}
version: 1.0.0
dependencies: {deps_yaml}
""")

        # Create Python module
        (skill_dir / f"{name.replace('-', '_')}.py").write_text(
            f"# {name} module\nLOADED = True\n"
        )

    def test_dependency_loading(self):
        """Test that dependencies are loaded correctly."""
        # Load dependent skill
        self.resolver.load_skill_with_dependencies("dependent-skill")

        # Both skills should be loaded
        self.assertTrue(self.resolver.is_skill_loaded("base-skill"))
        self.assertTrue(self.resolver.is_skill_loaded("dependent-skill"))

    def tearDown(self):
        """Clean up test environment."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

if __name__ == "__main__":
    unittest.main()
```

## Best Practices Demonstrated

1. **Layered Architecture**: Base utilities → Specialized tools → High-level features
2. **Error Handling**: Comprehensive exception handling for all error cases
3. **Type Hints**: Use Python type hints for better IDE support and documentation
4. **Validation**: Pre-deployment validation scripts to catch issues early
5. **Testing**: Custom resolvers for isolated testing
6. **Documentation**: Clear docstrings and examples

## Troubleshooting Common Issues

### Issue: ModuleNotFoundError

```python
ModuleNotFoundError: No module named 'dependency_skill'
```

**Solutions:**
1. Check skill naming (use underscores in Python: `my_skill.py`)
2. Ensure skill directory is in `sys.path`
3. Verify module file exists and is named correctly

### Issue: Circular dependency

```python
CircularDependencyError: Circular dependency detected: skill-a -> skill-b -> skill-a
```

**Solutions:**
1. Identify cycle from error message
2. Refactor to extract shared code
3. Use dependency injection instead of direct imports

### Issue: YAML parsing errors

```python
yaml.scanner.ScannerError: while scanning...
```

**Solutions:**
1. Validate YAML syntax online or with `yamllint`
2. Check indentation (use spaces, not tabs)
3. Ensure proper list format for dependencies

## See Also

- [DEPENDENCY-RESOLUTION.md](./DEPENDENCY-RESOLUTION.md) - Full API documentation
- [lib/dependency_resolver.py](./lib/dependency_resolver.py) - Source code
- [tests/test_dependency_resolver.py](./tests/test_dependency_resolver.py) - Test suite
- [Bash Version](../multi-persona-review-marketplace/DEPENDENCY-EXAMPLES.md) - Bash examples
