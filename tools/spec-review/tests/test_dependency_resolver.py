#!/usr/bin/env python3
"""
Test suite for dependency_resolver.py
Tests dependency graph resolution, circular dependency detection, and automatic loading.
"""

import sys
import tempfile
import unittest
from pathlib import Path
from typing import Dict, List

# Add lib directory to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from dependency_resolver import (
    SkillDependencyResolver,
    CircularDependencyError,
    DependencyNotFoundError,
)


class TestDependencyResolver(unittest.TestCase):
    """Test cases for SkillDependencyResolver."""

    def setUp(self):
        """Set up test fixtures."""
        # Create temporary test environment
        self.test_dir = tempfile.mkdtemp()
        self.test_skills_dir = Path(self.test_dir) / "skills"
        self.test_skills_dir.mkdir(parents=True)

        # Create test skills
        self._create_test_skills()

        # Initialize resolver with test directory
        self.resolver = SkillDependencyResolver(plugin_root=Path(self.test_dir))

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

    def _create_test_skills(self):
        """Create test skills directory structure."""
        # Skill A (no dependencies)
        skill_a_dir = self.test_skills_dir / "skill-a"
        skill_a_dir.mkdir()
        (skill_a_dir / "skill.yml").write_text("""
name: skill-a
version: 1.0.0
description: Test skill A
dependencies: []
""")
        (skill_a_dir / "skill_a.py").write_text("# Skill A module\nSKILL_A_LOADED = True\n")

        # Skill B (depends on skill-a)
        skill_b_dir = self.test_skills_dir / "skill-b"
        skill_b_dir.mkdir()
        (skill_b_dir / "skill.yml").write_text("""
name: skill-b
version: 1.0.0
description: Test skill B
dependencies:
  - skill-a
""")
        (skill_b_dir / "skill_b.py").write_text("# Skill B module\nSKILL_B_LOADED = True\n")

        # Skill C (depends on skill-b and skill-a)
        skill_c_dir = self.test_skills_dir / "skill-c"
        skill_c_dir.mkdir()
        (skill_c_dir / "skill.yml").write_text("""
name: skill-c
version: 1.0.0
description: Test skill C
dependencies:
  - skill-b
  - skill-a
""")
        (skill_c_dir / "skill_c.py").write_text("# Skill C module\nSKILL_C_LOADED = True\n")

        # Skill with circular dependency (circular-1 -> circular-2 -> circular-1)
        skill_circular_1_dir = self.test_skills_dir / "skill-circular-1"
        skill_circular_1_dir.mkdir()
        (skill_circular_1_dir / "skill.yml").write_text("""
name: skill-circular-1
version: 1.0.0
description: Test circular dependency 1
dependencies:
  - skill-circular-2
""")
        (skill_circular_1_dir / "skill_circular_1.py").write_text(
            "# Circular 1 module\nCIRCULAR_1_LOADED = True\n"
        )

        skill_circular_2_dir = self.test_skills_dir / "skill-circular-2"
        skill_circular_2_dir.mkdir()
        (skill_circular_2_dir / "skill.yml").write_text("""
name: skill-circular-2
version: 1.0.0
description: Test circular dependency 2
dependencies:
  - skill-circular-1
""")
        (skill_circular_2_dir / "skill_circular_2.py").write_text(
            "# Circular 2 module\nCIRCULAR_2_LOADED = True\n"
        )

        # Skill with missing dependency
        skill_broken_dir = self.test_skills_dir / "skill-broken"
        skill_broken_dir.mkdir()
        (skill_broken_dir / "skill.yml").write_text("""
name: skill-broken
version: 1.0.0
description: Test skill with missing dependency
dependencies:
  - skill-nonexistent
""")
        (skill_broken_dir / "skill_broken.py").write_text(
            "# Broken module\nBROKEN_LOADED = True\n"
        )

        # Skill with external dependencies (should be filtered out)
        skill_external_dir = self.test_skills_dir / "skill-external"
        skill_external_dir.mkdir()
        (skill_external_dir / "skill.yml").write_text("""
name: skill-external
version: 1.0.0
description: Test skill with external dependencies
dependencies:
  - cli-abstraction
  - anthropic
  - skill-a
""")
        (skill_external_dir / "skill_external.py").write_text(
            "# External module\nEXTERNAL_LOADED = True\n"
        )

    def test_init_discovery(self):
        """Test that skills are discovered on initialization."""
        self.assertIn("skill-a", self.resolver.skill_paths)
        self.assertIn("skill-b", self.resolver.skill_paths)
        self.assertIn("skill-c", self.resolver.skill_paths)
        self.assertEqual(len(self.resolver.skill_paths), 7)  # 7 test skills

    def test_parse_dependencies(self):
        """Test dependency parsing from skill.yml."""
        # skill-a has no dependencies
        self.assertEqual(self.resolver.get_dependencies("skill-a"), [])

        # skill-b depends on skill-a
        self.assertEqual(self.resolver.get_dependencies("skill-b"), ["skill-a"])

        # skill-c depends on skill-b and skill-a
        deps_c = self.resolver.get_dependencies("skill-c")
        self.assertIn("skill-a", deps_c)
        self.assertIn("skill-b", deps_c)

        # skill-external: external deps filtered out, only skill-a remains
        deps_ext = self.resolver.get_dependencies("skill-external")
        self.assertEqual(deps_ext, ["skill-a"])
        self.assertNotIn("cli-abstraction", deps_ext)
        self.assertNotIn("anthropic", deps_ext)

    def test_has_dependencies(self):
        """Test has_dependencies method."""
        self.assertFalse(self.resolver.has_dependencies("skill-a"))
        self.assertTrue(self.resolver.has_dependencies("skill-b"))
        self.assertTrue(self.resolver.has_dependencies("skill-c"))

    def test_detect_circular_dependency_valid(self):
        """Test circular dependency detection on valid dependencies."""
        # Should not raise for skill-c (valid dependency chain)
        try:
            self.resolver.detect_circular_dependency("skill-c")
        except CircularDependencyError:
            self.fail("skill-c should not have circular dependencies")

    def test_detect_circular_dependency_invalid(self):
        """Test circular dependency detection on circular dependencies."""
        # Should raise for skill-circular-1
        with self.assertRaises(CircularDependencyError) as context:
            self.resolver.detect_circular_dependency("skill-circular-1")

        self.assertIn("Circular dependency", str(context.exception))

    def test_resolve_dependency_order(self):
        """Test topological sorting of dependencies."""
        # Get load order for skill-c
        order = self.resolver.resolve_dependency_order("skill-c")

        # Check all skills are present
        self.assertIn("skill-a", order)
        self.assertIn("skill-b", order)
        self.assertIn("skill-c", order)

        # Check order: skill-a should come before skill-b
        # skill-b should come before skill-c
        pos_a = order.index("skill-a")
        pos_b = order.index("skill-b")
        pos_c = order.index("skill-c")

        self.assertLess(pos_a, pos_b, "skill-a should come before skill-b")
        self.assertLess(pos_b, pos_c, "skill-b should come before skill-c")

    def test_load_skill(self):
        """Test loading a single skill."""
        # Load skill-a
        self.resolver.load_skill("skill-a")

        # Check it's marked as loaded
        self.assertTrue(self.resolver.is_skill_loaded("skill-a"))

        # Check module was imported
        self.assertIn("skill_a", sys.modules)

    def test_load_skill_nonexistent(self):
        """Test loading a nonexistent skill raises error."""
        with self.assertRaises(DependencyNotFoundError):
            self.resolver.load_skill("skill-nonexistent")

    def test_load_skill_with_dependencies(self):
        """Test loading a skill with dependencies."""
        # Load skill-c (should also load skill-a and skill-b)
        self.resolver.load_skill_with_dependencies("skill-c")

        # All three should be loaded
        self.assertTrue(self.resolver.is_skill_loaded("skill-a"))
        self.assertTrue(self.resolver.is_skill_loaded("skill-b"))
        self.assertTrue(self.resolver.is_skill_loaded("skill-c"))

    def test_load_skill_with_circular_dependency(self):
        """Test loading a skill with circular dependency raises error."""
        with self.assertRaises(CircularDependencyError):
            self.resolver.load_skill_with_dependencies("skill-circular-1")

    def test_get_reverse_dependencies(self):
        """Test getting reverse dependencies."""
        # skill-a is depended on by skill-b and skill-c
        reverse_deps = self.resolver.get_reverse_dependencies("skill-a")

        self.assertIn("skill-b", reverse_deps)
        self.assertIn("skill-c", reverse_deps)
        self.assertIn("skill-external", reverse_deps)

        # skill-b is depended on by skill-c
        reverse_deps_b = self.resolver.get_reverse_dependencies("skill-b")
        self.assertIn("skill-c", reverse_deps_b)

    def test_validate_dependencies_valid(self):
        """Test validation passes for valid dependencies."""
        # Create a new temporary directory with only valid skills
        import tempfile
        import shutil

        valid_test_dir = tempfile.mkdtemp()
        valid_skills_dir = Path(valid_test_dir) / "skills"
        valid_skills_dir.mkdir(parents=True)

        # Copy only valid skills
        for skill in ["skill-a", "skill-b", "skill-c"]:
            shutil.copytree(
                self.test_skills_dir / skill,
                valid_skills_dir / skill
            )

        try:
            valid_resolver = SkillDependencyResolver(plugin_root=Path(valid_test_dir))

            # Should pass validation
            self.assertTrue(valid_resolver.validate_dependencies())
        finally:
            shutil.rmtree(valid_test_dir, ignore_errors=True)

    def test_validate_dependencies_invalid(self):
        """Test validation fails for invalid dependencies."""
        # skill-broken has missing dependency
        # Validation should fail
        result = self.resolver.validate_dependencies()
        self.assertFalse(result)

    def test_get_dependency_graph(self):
        """Test getting the complete dependency graph."""
        graph = self.resolver.get_dependency_graph()

        self.assertIsInstance(graph, dict)
        self.assertIn("skill-a", graph)
        self.assertIn("skill-b", graph)
        self.assertIn("skill-c", graph)

    def test_get_all_skills(self):
        """Test getting all discovered skills."""
        skills = self.resolver.get_all_skills()

        self.assertIn("skill-a", skills)
        self.assertIn("skill-b", skills)
        self.assertIn("skill-c", skills)
        self.assertEqual(len(skills), 7)


class TestDependencyResolverEdgeCases(unittest.TestCase):
    """Test edge cases and error handling."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()
        self.test_skills_dir = Path(self.test_dir) / "skills"
        self.test_skills_dir.mkdir(parents=True)

    def tearDown(self):
        """Clean up test fixtures."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

    def test_empty_skills_directory(self):
        """Test resolver with empty skills directory."""
        resolver = SkillDependencyResolver(plugin_root=Path(self.test_dir))

        self.assertEqual(len(resolver.get_all_skills()), 0)
        self.assertTrue(resolver.validate_dependencies())

    def test_skill_with_invalid_yaml(self):
        """Test handling of skills with invalid YAML."""
        skill_dir = self.test_skills_dir / "invalid-skill"
        skill_dir.mkdir()
        (skill_dir / "skill.yml").write_text("invalid: yaml: content: [[[")

        resolver = SkillDependencyResolver(plugin_root=Path(self.test_dir))

        # Should not crash, just skip invalid skill or use empty deps
        self.assertIn("invalid-skill", resolver.skill_paths)

    def test_skill_without_yaml(self):
        """Test handling of skills without skill.yml."""
        skill_dir = self.test_skills_dir / "no-yaml-skill"
        skill_dir.mkdir()
        (skill_dir / "no_yaml_skill.py").write_text("# No YAML\n")

        resolver = SkillDependencyResolver(plugin_root=Path(self.test_dir))

        # Skill should not be discovered without skill.yml
        self.assertNotIn("no-yaml-skill", resolver.skill_paths)

    def test_deep_dependency_chain(self):
        """Test resolver with deep dependency chain."""
        # Create a chain: skill-1 -> skill-2 -> skill-3 -> skill-4 -> skill-5
        for i in range(1, 6):
            skill_dir = self.test_skills_dir / f"skill-{i}"
            skill_dir.mkdir()

            deps = f"  - skill-{i+1}\n" if i < 5 else ""
            (skill_dir / "skill.yml").write_text(f"""
name: skill-{i}
version: 1.0.0
dependencies:
{deps}
""")
            (skill_dir / f"skill_{i}.py").write_text(f"# Skill {i}\n")

        resolver = SkillDependencyResolver(plugin_root=Path(self.test_dir))

        # Should resolve without error
        order = resolver.resolve_dependency_order("skill-1")

        # All skills should be in order
        self.assertEqual(len(order), 5)

        # skill-5 should come first, skill-1 last
        self.assertEqual(order[0], "skill-5")
        self.assertEqual(order[-1], "skill-1")


def run_tests():
    """Run all tests."""
    # Create test suite
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()

    # Add all tests
    suite.addTests(loader.loadTestsFromTestCase(TestDependencyResolver))
    suite.addTests(loader.loadTestsFromTestCase(TestDependencyResolverEdgeCases))

    # Run tests
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)

    # Return exit code
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(run_tests())
