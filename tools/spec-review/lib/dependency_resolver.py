#!/usr/bin/env python3
"""
Dependency Resolver for Python Skills
Implements dependency graph resolution with circular dependency detection.
Part of spec-review-marketplace plugin for Engram.
"""

import sys
from pathlib import Path
from typing import Dict, List, Set, Optional, Tuple
import yaml


class CircularDependencyError(Exception):
    """Raised when a circular dependency is detected."""
    pass


class DependencyNotFoundError(Exception):
    """Raised when a required dependency is not found."""
    pass


class SkillDependencyResolver:
    """
    Resolves skill dependencies and handles automatic loading.

    Supports:
    - Dependency graph construction from skill.yml files
    - Circular dependency detection
    - Topological sorting for load order
    - Automatic skill loading
    """

    def __init__(self, plugin_root: Optional[Path] = None):
        """
        Initialize the dependency resolver.

        Args:
            plugin_root: Path to plugin root directory (auto-detected if None)
        """
        if plugin_root is None:
            # Auto-detect plugin root (3 levels up from this file)
            plugin_root = Path(__file__).parent.parent

        self.plugin_root = Path(plugin_root)
        self.skills_dir = self.plugin_root / "skills"

        # Dependency graph storage
        self.skill_dependencies: Dict[str, List[str]] = {}  # skill_name -> [dep1, dep2, ...]
        self.skill_paths: Dict[str, Path] = {}  # skill_name -> path to skill directory
        self.loaded_skills: Set[str] = set()  # Set of loaded skill names
        self.loading_skills: Set[str] = set()  # Skills currently being loaded (for cycle detection)

        # Initialize by discovering skills
        self._discover_skills()

    def _discover_skills(self) -> None:
        """Discover all skills in the marketplace."""
        if not self.skills_dir.exists():
            print(f"Warning: Skills directory not found: {self.skills_dir}", file=sys.stderr)
            return

        # Scan each skill directory for skill.yml
        for skill_dir in self.skills_dir.iterdir():
            if skill_dir.is_dir():
                skill_name = skill_dir.name
                skill_yml = skill_dir / "skill.yml"

                if skill_yml.exists():
                    # Store skill path
                    self.skill_paths[skill_name] = skill_dir

                    # Parse dependencies
                    self._parse_skill_dependencies(skill_name, skill_yml)

    def _parse_skill_dependencies(self, skill_name: str, skill_yml: Path) -> None:
        """
        Parse dependencies from skill.yml file.

        Args:
            skill_name: Name of the skill
            skill_yml: Path to skill.yml file
        """
        try:
            with open(skill_yml, 'r', encoding='utf-8') as f:
                skill_config = yaml.safe_load(f)

            if not skill_config:
                self.skill_dependencies[skill_name] = []
                return

            # Extract dependencies
            # Support multiple formats:
            # 1. dependencies: [dep1, dep2]
            # 2. dependencies:
            #      - dep1
            #      - dep2
            # 3. dependencies:
            #      - name: dep1
            #      - name: dep2

            deps_raw = skill_config.get('dependencies', [])

            if isinstance(deps_raw, list):
                deps = []
                for dep in deps_raw:
                    if isinstance(dep, str):
                        # Simple string dependency
                        deps.append(dep)
                    elif isinstance(dep, dict) and 'name' in dep:
                        # Dictionary with name key
                        deps.append(dep['name'])
                    # Ignore other dependency types (external libs like cli-abstraction, anthropic, etc.)

                # Filter out known external dependencies (not skill names)
                external_deps = {'cli-abstraction', 'anthropic', 'pydantic', 'rich', 'pystache'}
                deps = [d for d in deps if d not in external_deps]

                self.skill_dependencies[skill_name] = deps
            else:
                self.skill_dependencies[skill_name] = []

        except Exception as e:
            print(f"Warning: Could not parse dependencies for {skill_name}: {e}", file=sys.stderr)
            self.skill_dependencies[skill_name] = []

    def has_dependencies(self, skill_name: str) -> bool:
        """
        Check if a skill has dependencies.

        Args:
            skill_name: Name of the skill

        Returns:
            True if skill has dependencies, False otherwise
        """
        return len(self.skill_dependencies.get(skill_name, [])) > 0

    def get_dependencies(self, skill_name: str) -> List[str]:
        """
        Get dependencies for a skill.

        Args:
            skill_name: Name of the skill

        Returns:
            List of dependency names
        """
        return self.skill_dependencies.get(skill_name, [])

    def is_skill_loaded(self, skill_name: str) -> bool:
        """
        Check if a skill is loaded.

        Args:
            skill_name: Name of the skill

        Returns:
            True if loaded, False otherwise
        """
        return skill_name in self.loaded_skills

    def mark_skill_loaded(self, skill_name: str) -> None:
        """
        Mark a skill as loaded.

        Args:
            skill_name: Name of the skill
        """
        self.loaded_skills.add(skill_name)

    def detect_circular_dependency(self, skill_name: str, visited_path: Optional[List[str]] = None) -> None:
        """
        Detect circular dependencies using DFS.

        Args:
            skill_name: Name of the skill to check
            visited_path: Path of visited skills (for error reporting)

        Raises:
            CircularDependencyError: If a circular dependency is detected
        """
        if visited_path is None:
            visited_path = []

        # Check if this skill is currently being loaded (cycle detected)
        if skill_name in self.loading_skills:
            path_str = " -> ".join(visited_path + [skill_name])
            raise CircularDependencyError(f"Circular dependency detected: {path_str}")

        # Mark as currently loading
        self.loading_skills.add(skill_name)

        try:
            # Check all dependencies recursively
            deps = self.get_dependencies(skill_name)

            for dep in deps:
                new_path = visited_path + [skill_name]
                self.detect_circular_dependency(dep, new_path)
        finally:
            # Unmark as loading
            self.loading_skills.discard(skill_name)

    def resolve_dependency_order(self, skill_name: str) -> List[str]:
        """
        Resolve dependencies in correct order (topological sort).

        Args:
            skill_name: Name of the skill

        Returns:
            List of skills in load order (dependencies first)
        """
        order = []
        visited = set()

        def dfs(current: str) -> None:
            # Skip if already visited
            if current in visited:
                return

            visited.add(current)

            # Visit all dependencies first
            deps = self.get_dependencies(current)
            for dep in deps:
                dfs(dep)

            # Add current skill after dependencies
            order.append(current)

        dfs(skill_name)
        return order

    def load_skill(self, skill_name: str) -> None:
        """
        Load a skill by importing its main module.

        Args:
            skill_name: Name of the skill

        Raises:
            DependencyNotFoundError: If skill is not found
        """
        # Check if already loaded
        if self.is_skill_loaded(skill_name):
            return

        # Get skill path
        skill_path = self.skill_paths.get(skill_name)

        if skill_path is None:
            available = ", ".join(self.skill_paths.keys())
            raise DependencyNotFoundError(
                f"Unknown skill: {skill_name}. Available skills: {available}"
            )

        # Find the main Python script
        # Try common naming patterns: skill_name.py or skillname.py
        main_script = None
        patterns = [
            skill_path / f"{skill_name}.py",
            skill_path / f"{skill_name.replace('-', '_')}.py",
        ]

        for pattern in patterns:
            if pattern.exists():
                main_script = pattern
                break

        if main_script is None:
            raise DependencyNotFoundError(
                f"Main script not found for skill: {skill_name}"
            )

        # Add skill directory to Python path
        if str(skill_path) not in sys.path:
            sys.path.insert(0, str(skill_path))

        # Import the module (this loads it)
        module_name = main_script.stem

        try:
            __import__(module_name)
            self.mark_skill_loaded(skill_name)
            print(f"Loaded skill: {skill_name}", file=sys.stderr)
        except Exception as e:
            raise DependencyNotFoundError(
                f"Failed to load skill {skill_name}: {e}"
            )

    def load_skill_with_dependencies(self, skill_name: str) -> None:
        """
        Load a skill and all its dependencies.

        Args:
            skill_name: Name of the skill

        Raises:
            CircularDependencyError: If circular dependency detected
            DependencyNotFoundError: If a dependency is not found
        """
        # Check for circular dependencies
        self.detect_circular_dependency(skill_name)

        # Resolve dependency order
        load_order = self.resolve_dependency_order(skill_name)

        # Load skills in order
        for skill in load_order:
            if not self.is_skill_loaded(skill):
                print(f"Loading dependency: {skill}", file=sys.stderr)
                self.load_skill(skill)

    def print_dependency_tree(self, skill_name: str, indent_level: int = 0) -> None:
        """
        Print dependency tree for a skill.

        Args:
            skill_name: Name of the skill
            indent_level: Current indentation level
        """
        indent = "  " * indent_level
        print(f"{indent}{skill_name}")

        deps = self.get_dependencies(skill_name)
        for dep in deps:
            self.print_dependency_tree(dep, indent_level + 1)

    def get_reverse_dependencies(self, target_skill: str) -> List[str]:
        """
        Get all skills that depend on a given skill (reverse dependencies).

        Args:
            target_skill: Name of the target skill

        Returns:
            List of skill names that depend on target_skill
        """
        reverse_deps = []

        for skill, deps in self.skill_dependencies.items():
            if target_skill in deps:
                reverse_deps.append(skill)

        return reverse_deps

    def validate_dependencies(self) -> bool:
        """
        Validate that all dependencies exist.

        Returns:
            True if all dependencies exist, False otherwise
        """
        all_valid = True

        for skill, deps in self.skill_dependencies.items():
            for dep in deps:
                if dep not in self.skill_paths:
                    print(
                        f"Error: Skill '{skill}' depends on unknown skill '{dep}'",
                        file=sys.stderr
                    )
                    all_valid = False

        return all_valid

    def get_dependency_graph(self) -> Dict[str, List[str]]:
        """
        Get the complete dependency graph.

        Returns:
            Dictionary mapping skill names to their dependencies
        """
        return self.skill_dependencies.copy()

    def get_all_skills(self) -> List[str]:
        """
        Get list of all discovered skills.

        Returns:
            List of skill names
        """
        return list(self.skill_paths.keys())


# Module-level default instance
_default_resolver: Optional[SkillDependencyResolver] = None


def get_resolver() -> SkillDependencyResolver:
    """Get or create the default resolver instance."""
    global _default_resolver
    if _default_resolver is None:
        _default_resolver = SkillDependencyResolver()
    return _default_resolver


# Convenience functions using default resolver
def load_skill_with_dependencies(skill_name: str) -> None:
    """Load a skill and all its dependencies using default resolver."""
    resolver = get_resolver()
    resolver.load_skill_with_dependencies(skill_name)


def print_dependency_tree(skill_name: str) -> None:
    """Print dependency tree using default resolver."""
    resolver = get_resolver()
    resolver.print_dependency_tree(skill_name)


def validate_dependencies() -> bool:
    """Validate all dependencies using default resolver."""
    resolver = get_resolver()
    return resolver.validate_dependencies()


if __name__ == "__main__":
    # CLI interface for testing
    import argparse

    parser = argparse.ArgumentParser(description="Skill Dependency Resolver")
    parser.add_argument("command", choices=["list", "tree", "validate", "load"],
                        help="Command to execute")
    parser.add_argument("skill", nargs="?", help="Skill name (for tree/load commands)")

    args = parser.parse_args()

    resolver = get_resolver()

    if args.command == "list":
        print("Discovered skills:")
        for skill in sorted(resolver.get_all_skills()):
            deps = resolver.get_dependencies(skill)
            if deps:
                print(f"  {skill} -> {', '.join(deps)}")
            else:
                print(f"  {skill}")

    elif args.command == "tree":
        if not args.skill:
            print("Error: Skill name required for 'tree' command", file=sys.stderr)
            sys.exit(1)
        print(f"Dependency tree for {args.skill}:")
        resolver.print_dependency_tree(args.skill)

    elif args.command == "validate":
        if resolver.validate_dependencies():
            print("✓ All dependencies are valid")
            sys.exit(0)
        else:
            print("✗ Some dependencies are invalid", file=sys.stderr)
            sys.exit(1)

    elif args.command == "load":
        if not args.skill:
            print("Error: Skill name required for 'load' command", file=sys.stderr)
            sys.exit(1)
        try:
            resolver.load_skill_with_dependencies(args.skill)
            print(f"✓ Successfully loaded {args.skill} and dependencies")
        except (CircularDependencyError, DependencyNotFoundError) as e:
            print(f"✗ Error: {e}", file=sys.stderr)
            sys.exit(1)
