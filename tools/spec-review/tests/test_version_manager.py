#!/usr/bin/env python3
"""
Test Suite for Version Manager (Python)
Tests semantic versioning, compatibility checking, and conflict resolution

Usage: python -m pytest tests/test_version_manager.py -v
"""

import json
import pytest
import tempfile
from pathlib import Path

import sys
sys.path.insert(0, str(Path(__file__).parent.parent))

from lib.version_manager import (
    VersionManager,
    Version,
    ChangeType,
    CompatibilityStatus,
    ConstraintSatisfaction
)


@pytest.fixture
def vm():
    """Fixture to create a VersionManager instance"""
    return VersionManager()


@pytest.fixture
def test_skills_dir():
    """Fixture to create a temporary skills directory"""
    with tempfile.TemporaryDirectory() as tmpdir:
        tmpdir_path = Path(tmpdir)

        # Create test skill 1
        skill_a_dir = tmpdir_path / "skill-a"
        skill_a_dir.mkdir()
        skill_a_yml = skill_a_dir / "skill.yml"
        skill_a_yml.write_text("""name: skill-a
version: "1.2.3"
description: Test skill A
""")

        # Create test skill 2
        skill_b_dir = tmpdir_path / "skill-b"
        skill_b_dir.mkdir()
        skill_b_yml = skill_b_dir / "skill.yml"
        skill_b_yml.write_text("""name: skill-b
version: "2.0.0"
description: Test skill B
""")

        yield tmpdir_path


class TestVersionParsing:
    """Test version parsing functionality"""

    def test_parse_valid_version_basic(self, vm):
        """Test parsing basic semantic versions"""
        v = vm.parse_version("1.2.3")
        assert v.major == 1
        assert v.minor == 2
        assert v.patch == 3
        assert v.prerelease == ""
        assert v.build == ""

    def test_parse_valid_version_with_zeros(self, vm):
        """Test parsing version with zeros"""
        v = vm.parse_version("0.0.1")
        assert v.major == 0
        assert v.minor == 0
        assert v.patch == 1

    def test_parse_valid_version_large_numbers(self, vm):
        """Test parsing version with large numbers"""
        v = vm.parse_version("10.20.30")
        assert v.major == 10
        assert v.minor == 20
        assert v.patch == 30

    def test_parse_version_with_prerelease(self, vm):
        """Test parsing version with prerelease"""
        v = vm.parse_version("1.2.3-alpha.1")
        assert v.major == 1
        assert v.minor == 2
        assert v.patch == 3
        assert v.prerelease == "alpha.1"

    def test_parse_version_with_build(self, vm):
        """Test parsing version with build metadata"""
        v = vm.parse_version("1.2.3+build.123")
        assert v.major == 1
        assert v.minor == 2
        assert v.patch == 3
        assert v.build == "build.123"

    def test_parse_version_complete(self, vm):
        """Test parsing complete version with prerelease and build"""
        v = vm.parse_version("1.2.3-beta.2+build.456")
        assert v.major == 1
        assert v.minor == 2
        assert v.patch == 3
        assert v.prerelease == "beta.2"
        assert v.build == "build.456"

    def test_parse_invalid_version_short(self, vm):
        """Test parsing invalid short version"""
        with pytest.raises(ValueError):
            vm.parse_version("1.2")

    def test_parse_invalid_version_prefix(self, vm):
        """Test parsing invalid version with v prefix"""
        with pytest.raises(ValueError):
            vm.parse_version("v1.2.3")

    def test_parse_invalid_version_extra_segment(self, vm):
        """Test parsing invalid version with extra segment"""
        with pytest.raises(ValueError):
            vm.parse_version("1.2.3.4")

    def test_parse_invalid_version_letters(self, vm):
        """Test parsing invalid version with letters"""
        with pytest.raises(ValueError):
            vm.parse_version("abc.def.ghi")


class TestVersionComparison:
    """Test version comparison functionality"""

    def test_compare_versions_less_than(self, vm):
        """Test comparing versions (less than)"""
        result = vm.compare_versions("1.2.3", "1.2.4")
        assert result == -1

    def test_compare_versions_equal(self, vm):
        """Test comparing versions (equal)"""
        result = vm.compare_versions("1.2.3", "1.2.3")
        assert result == 0

    def test_compare_versions_greater_than(self, vm):
        """Test comparing versions (greater than)"""
        result = vm.compare_versions("1.2.4", "1.2.3")
        assert result == 1

    def test_compare_versions_major_difference(self, vm):
        """Test comparing versions with major difference"""
        result = vm.compare_versions("2.0.0", "1.9.9")
        assert result == 1

    def test_compare_versions_minor_difference(self, vm):
        """Test comparing versions with minor difference"""
        result = vm.compare_versions("1.3.0", "1.2.9")
        assert result == 1

    def test_version_object_equality(self, vm):
        """Test Version object equality"""
        v1 = vm.parse_version("1.2.3")
        v2 = vm.parse_version("1.2.3")
        assert v1 == v2

    def test_version_object_less_than(self, vm):
        """Test Version object less than"""
        v1 = vm.parse_version("1.2.3")
        v2 = vm.parse_version("1.2.4")
        assert v1 < v2

    def test_version_object_greater_than(self, vm):
        """Test Version object greater than"""
        v1 = vm.parse_version("1.2.4")
        v2 = vm.parse_version("1.2.3")
        assert v1 > v2


class TestCompatibilityChecking:
    """Test compatibility checking functionality"""

    def test_compatibility_patch_update(self, vm):
        """Test compatibility for patch update"""
        compat = vm.check_compatibility("1.2.3", "1.2.4")
        assert compat == CompatibilityStatus.COMPATIBLE

    def test_compatibility_minor_update(self, vm):
        """Test compatibility for minor update"""
        compat = vm.check_compatibility("1.2.3", "1.3.0")
        assert compat == CompatibilityStatus.COMPATIBLE

    def test_compatibility_major_update(self, vm):
        """Test compatibility for major update (breaking)"""
        compat = vm.check_compatibility("1.9.9", "2.0.0")
        assert compat == CompatibilityStatus.BREAKING

    def test_compatibility_multiple_major_update(self, vm):
        """Test compatibility for multiple major version jump"""
        compat = vm.check_compatibility("2.0.0", "3.0.0")
        assert compat == CompatibilityStatus.BREAKING

    def test_compatibility_zero_major(self, vm):
        """Test compatibility for 0.x versions"""
        compat = vm.check_compatibility("0.1.0", "0.2.0")
        assert compat == CompatibilityStatus.COMPATIBLE


class TestVersionValidation:
    """Test version validation functionality"""

    def test_validate_valid_version(self, vm, capsys):
        """Test validating a valid version"""
        result = vm.validate_version("1.2.3")
        assert result is True
        captured = capsys.readouterr()
        assert "✓ Valid version" in captured.out

    def test_validate_invalid_version(self, vm, capsys):
        """Test validating an invalid version"""
        result = vm.validate_version("1.2")
        assert result is False
        captured = capsys.readouterr()
        assert "✗ Invalid version" in captured.out


class TestConstraintChecking:
    """Test constraint checking functionality"""

    def test_constraint_exact_match_satisfied(self, vm):
        """Test exact match constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.3", "=1.2.3")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_exact_match_not_satisfied(self, vm):
        """Test exact match constraint (not satisfied)"""
        satisfied = vm.check_constraint("1.2.4", "=1.2.3")
        assert satisfied == ConstraintSatisfaction.NOT_SATISFIED

    def test_constraint_greater_than_satisfied(self, vm):
        """Test greater than constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.4", ">1.2.3")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_greater_than_not_satisfied(self, vm):
        """Test greater than constraint (not satisfied)"""
        satisfied = vm.check_constraint("1.2.3", ">1.2.3")
        assert satisfied == ConstraintSatisfaction.NOT_SATISFIED

    def test_constraint_greater_equal_satisfied(self, vm):
        """Test greater than or equal constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.4", ">=1.2.3")
        assert satisfied == ConstraintSatisfaction.SATISFIED
        satisfied = vm.check_constraint("1.2.3", ">=1.2.3")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_greater_equal_not_satisfied(self, vm):
        """Test greater than or equal constraint (not satisfied)"""
        satisfied = vm.check_constraint("1.2.2", ">=1.2.3")
        assert satisfied == ConstraintSatisfaction.NOT_SATISFIED

    def test_constraint_less_than_satisfied(self, vm):
        """Test less than constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.3", "<1.2.4")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_less_equal_satisfied(self, vm):
        """Test less than or equal constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.3", "<=1.2.3")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_caret_satisfied(self, vm):
        """Test caret constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.5", "^1.2.0")
        assert satisfied == ConstraintSatisfaction.SATISFIED
        satisfied = vm.check_constraint("1.3.0", "^1.2.0")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_caret_not_satisfied(self, vm):
        """Test caret constraint (not satisfied)"""
        satisfied = vm.check_constraint("2.0.0", "^1.2.0")
        assert satisfied == ConstraintSatisfaction.NOT_SATISFIED

    def test_constraint_tilde_satisfied(self, vm):
        """Test tilde constraint (satisfied)"""
        satisfied = vm.check_constraint("1.2.5", "~1.2.0")
        assert satisfied == ConstraintSatisfaction.SATISFIED

    def test_constraint_tilde_not_satisfied(self, vm):
        """Test tilde constraint (not satisfied)"""
        satisfied = vm.check_constraint("1.3.0", "~1.2.0")
        assert satisfied == ConstraintSatisfaction.NOT_SATISFIED


class TestBreakingChanges:
    """Test breaking change detection"""

    def test_is_breaking_change_patch(self, vm):
        """Test is_breaking_change for patch update"""
        assert vm.is_breaking_change("1.2.3", "1.2.4") is False

    def test_is_breaking_change_minor(self, vm):
        """Test is_breaking_change for minor update"""
        assert vm.is_breaking_change("1.2.3", "1.3.0") is False

    def test_is_breaking_change_major(self, vm):
        """Test is_breaking_change for major update"""
        assert vm.is_breaking_change("1.9.9", "2.0.0") is True

    def test_is_breaking_change_multiple_major(self, vm):
        """Test is_breaking_change for multiple major jump"""
        assert vm.is_breaking_change("2.5.8", "3.0.0") is True


class TestVersionSuggestions:
    """Test version suggestion functionality"""

    def test_suggest_next_patch(self, vm):
        """Test suggesting next patch version"""
        next_ver = vm.suggest_next_version("1.2.3", ChangeType.PATCH)
        assert next_ver == "1.2.4"

    def test_suggest_next_minor(self, vm):
        """Test suggesting next minor version"""
        next_ver = vm.suggest_next_version("1.2.3", ChangeType.MINOR)
        assert next_ver == "1.3.0"

    def test_suggest_next_major(self, vm):
        """Test suggesting next major version"""
        next_ver = vm.suggest_next_version("1.2.3", ChangeType.MAJOR)
        assert next_ver == "2.0.0"

    def test_suggest_next_major_from_zero(self, vm):
        """Test suggesting next major from 0.x"""
        next_ver = vm.suggest_next_version("0.0.1", ChangeType.MAJOR)
        assert next_ver == "1.0.0"


class TestConflictResolution:
    """Test version conflict resolution"""

    def test_resolve_conflict_compatible_higher_first(self, vm, capsys):
        """Test resolving compatible conflict (higher version first)"""
        resolved, ok = vm.resolve_version_conflict("test-skill", "1.3.0", "1.2.0")
        assert ok is True
        assert resolved == "1.3.0"

    def test_resolve_conflict_compatible_higher_second(self, vm, capsys):
        """Test resolving compatible conflict (higher version second)"""
        resolved, ok = vm.resolve_version_conflict("test-skill", "1.2.0", "1.3.0")
        assert ok is True
        assert resolved == "1.3.0"

    def test_resolve_conflict_compatible_equal(self, vm, capsys):
        """Test resolving compatible conflict (equal versions)"""
        resolved, ok = vm.resolve_version_conflict("test-skill", "1.2.5", "1.2.5")
        assert ok is True
        assert resolved == "1.2.5"

    def test_resolve_conflict_breaking(self, vm, capsys):
        """Test resolving breaking conflict (unresolvable)"""
        resolved, ok = vm.resolve_version_conflict("test-skill", "1.9.9", "2.0.0")
        assert ok is False
        assert resolved is None
        captured = capsys.readouterr()
        assert "Breaking change detected" in captured.out

    def test_resolve_conflict_multiple_major(self, vm, capsys):
        """Test resolving conflict with multiple major jump"""
        resolved, ok = vm.resolve_version_conflict("test-skill", "2.0.0", "3.0.0")
        assert ok is False
        assert resolved is None


class TestCompatibilityMatrix:
    """Test compatibility matrix generation"""

    def test_generate_matrix(self, vm, test_skills_dir, capsys):
        """Test generating compatibility matrix"""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.json', delete=False) as f:
            output_file = Path(f.name)

        try:
            matrix = vm.generate_compatibility_matrix(test_skills_dir, output_file)

            # Check matrix structure
            assert "generated" in matrix
            assert "skills" in matrix
            assert "skill-a" in matrix["skills"]
            assert "skill-b" in matrix["skills"]

            # Check skill-a details
            assert matrix["skills"]["skill-a"]["version"] == "1.2.3"
            assert matrix["skills"]["skill-a"]["major"] == 1
            assert "^1.0.0" in matrix["skills"]["skill-a"]["compatible_with"]

            # Check skill-b details
            assert matrix["skills"]["skill-b"]["version"] == "2.0.0"
            assert matrix["skills"]["skill-b"]["major"] == 2

            # Check file was created
            assert output_file.exists()

            # Check file contents
            with open(output_file, 'r') as f:
                file_data = json.load(f)
                assert file_data == matrix

        finally:
            if output_file.exists():
                output_file.unlink()


class TestMigrationGuide:
    """Test migration guide functionality"""

    def test_get_migration_guide_not_breaking(self, vm, capsys):
        """Test getting migration guide for non-breaking change"""
        result = vm.get_migration_guide("test-skill", "1.2.3", "1.3.0")
        assert result is None
        captured = capsys.readouterr()
        assert "No migration guide needed" in captured.out

    def test_get_migration_guide_breaking_not_found(self, vm, test_skills_dir, capsys):
        """Test getting migration guide when not found"""
        result = vm.get_migration_guide("skill-a", "1.0.0", "2.0.0", test_skills_dir)
        assert result is None
        captured = capsys.readouterr()
        assert "Migration guide not found" in captured.out

    def test_get_migration_guide_breaking_found(self, vm, test_skills_dir, capsys):
        """Test getting migration guide when found"""
        # Create migration guide
        skill_a_dir = test_skills_dir / "skill-a"
        migration_guide = skill_a_dir / "MIGRATION-v1-to-v2.md"
        migration_guide.write_text("# Migration Guide v1 to v2\n")

        result = vm.get_migration_guide("skill-a", "1.0.0", "2.0.0", test_skills_dir)
        assert result is not None
        assert result.exists()
        assert result.name == "MIGRATION-v1-to-v2.md"


class TestVersionObject:
    """Test Version object functionality"""

    def test_version_str(self, vm):
        """Test Version string representation"""
        v = vm.parse_version("1.2.3")
        assert str(v) == "1.2.3"

    def test_version_str_with_prerelease(self, vm):
        """Test Version string with prerelease"""
        v = vm.parse_version("1.2.3-alpha.1")
        assert str(v) == "1.2.3-alpha.1"

    def test_version_str_with_build(self, vm):
        """Test Version string with build"""
        v = vm.parse_version("1.2.3+build.456")
        assert str(v) == "1.2.3+build.456"

    def test_version_str_complete(self, vm):
        """Test Version string with all components"""
        v = vm.parse_version("1.2.3-beta.2+build.789")
        assert str(v) == "1.2.3-beta.2+build.789"

    def test_version_repr(self, vm):
        """Test Version repr"""
        v = vm.parse_version("1.2.3")
        assert repr(v) == "Version(1.2.3)"


if __name__ == "__main__":
    pytest.main([__file__, "-v", "--tb=short"])
