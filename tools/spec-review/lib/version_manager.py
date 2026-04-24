#!/usr/bin/env python3
"""
Version Manager for Spec-Review Marketplace
Handles semantic versioning, compatibility checking, and version conflict resolution

Usage:
    from lib.version_manager import VersionManager

    vm = VersionManager()
    v1 = vm.parse_version("1.2.3")
    result = vm.compare_versions("1.2.3", "1.2.4")
    compat = vm.check_compatibility("1.2.3", "2.0.0")
"""

import re
import json
from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from pathlib import Path
from typing import Dict, List, Optional, Tuple
import yaml


VERSION_MANAGER_VERSION = "1.0.0"


class ChangeType(Enum):
    """Types of version changes"""
    MAJOR = "major"
    MINOR = "minor"
    PATCH = "patch"


class CompatibilityStatus(Enum):
    """Version compatibility status"""
    COMPATIBLE = "compatible"
    BREAKING = "breaking"


class ConstraintSatisfaction(Enum):
    """Constraint satisfaction status"""
    SATISFIED = "satisfied"
    NOT_SATISFIED = "not_satisfied"


@dataclass
class Version:
    """Semantic version representation"""
    major: int
    minor: int
    patch: int
    prerelease: str = ""
    build: str = ""

    def __str__(self) -> str:
        """String representation of version"""
        version_str = f"{self.major}.{self.minor}.{self.patch}"
        if self.prerelease:
            version_str += f"-{self.prerelease}"
        if self.build:
            version_str += f"+{self.build}"
        return version_str

    def __repr__(self) -> str:
        return f"Version({self})"

    def __eq__(self, other: 'Version') -> bool:
        """Check equality (ignoring build metadata)"""
        return (self.major == other.major and
                self.minor == other.minor and
                self.patch == other.patch and
                self.prerelease == other.prerelease)

    def __lt__(self, other: 'Version') -> bool:
        """Less than comparison"""
        if self.major != other.major:
            return self.major < other.major
        if self.minor != other.minor:
            return self.minor < other.minor
        if self.patch != other.patch:
            return self.patch < other.patch
        # Prerelease versions have lower precedence
        if self.prerelease and not other.prerelease:
            return True
        if not self.prerelease and other.prerelease:
            return False
        return self.prerelease < other.prerelease

    def __le__(self, other: 'Version') -> bool:
        return self == other or self < other

    def __gt__(self, other: 'Version') -> bool:
        return not (self <= other)

    def __ge__(self, other: 'Version') -> bool:
        return not (self < other)


class VersionManager:
    """Manages semantic versioning, compatibility, and conflict resolution"""

    # Semantic version regex pattern
    SEMVER_PATTERN = re.compile(
        r'^(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)'
        r'(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)'
        r'(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?'
        r'(?:\+(?P<build>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$'
    )

    def __init__(self, compatibility_matrix_file: str = "compatibility-matrix.json"):
        """Initialize version manager

        Args:
            compatibility_matrix_file: Path to compatibility matrix file
        """
        self.compatibility_matrix_file = compatibility_matrix_file

    def parse_version(self, version_str: str) -> Version:
        """Parse semantic version string

        Args:
            version_str: Version string (e.g., "1.2.3")

        Returns:
            Version object

        Raises:
            ValueError: If version format is invalid
        """
        match = self.SEMVER_PATTERN.match(version_str)
        if not match:
            raise ValueError(
                f"Invalid semantic version format: {version_str}\n"
                f"Expected format: MAJOR.MINOR.PATCH[-prerelease][+buildmetadata]"
            )

        return Version(
            major=int(match.group('major')),
            minor=int(match.group('minor')),
            patch=int(match.group('patch')),
            prerelease=match.group('prerelease') or "",
            build=match.group('build') or ""
        )

    def compare_versions(self, v1_str: str, v2_str: str) -> int:
        """Compare two semantic versions

        Args:
            v1_str: First version string
            v2_str: Second version string

        Returns:
            -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
        """
        v1 = self.parse_version(v1_str)
        v2 = self.parse_version(v2_str)

        if v1 < v2:
            return -1
        elif v1 > v2:
            return 1
        else:
            return 0

    def check_compatibility(self, old_version: str, new_version: str) -> CompatibilityStatus:
        """Check if version change is backward compatible

        Args:
            old_version: Old version string
            new_version: New version string

        Returns:
            CompatibilityStatus.COMPATIBLE or CompatibilityStatus.BREAKING
        """
        v_old = self.parse_version(old_version)
        v_new = self.parse_version(new_version)

        # Major version change indicates breaking changes
        if v_old.major != v_new.major:
            return CompatibilityStatus.BREAKING
        else:
            return CompatibilityStatus.COMPATIBLE

    def validate_version(self, version_str: str) -> bool:
        """Validate version string format

        Args:
            version_str: Version string to validate

        Returns:
            True if valid, False otherwise
        """
        try:
            self.parse_version(version_str)
            print(f"✓ Valid version: {version_str}")
            return True
        except ValueError as e:
            print(f"✗ Invalid version: {version_str}")
            print(f"  Error: {e}")
            return False

    def check_constraint(self, version_str: str, constraint: str) -> ConstraintSatisfaction:
        """Check if version satisfies a constraint

        Args:
            version_str: Version to check
            constraint: Constraint (e.g., ">=1.2.0", "^1.0.0", "~1.2.0")

        Returns:
            ConstraintSatisfaction status
        """
        # Parse constraint operator
        constraint_pattern = re.compile(r'^(\^|~|>=|<=|>|<|=)?(.+)$')
        match = constraint_pattern.match(constraint)

        if not match:
            raise ValueError(f"Invalid constraint format: {constraint}")

        operator = match.group(1) or "="
        constraint_version_str = match.group(2)

        version = self.parse_version(version_str)
        constraint_version = self.parse_version(constraint_version_str)

        # Exact match
        if operator == "=":
            satisfied = version == constraint_version

        # Greater than
        elif operator == ">":
            satisfied = version > constraint_version

        # Less than
        elif operator == "<":
            satisfied = version < constraint_version

        # Greater than or equal
        elif operator == ">=":
            satisfied = version >= constraint_version

        # Less than or equal
        elif operator == "<=":
            satisfied = version <= constraint_version

        # Caret: compatible with version (same major)
        elif operator == "^":
            satisfied = (version.major == constraint_version.major and
                        version >= constraint_version)

        # Tilde: compatible with minor version (same major.minor)
        elif operator == "~":
            satisfied = (version.major == constraint_version.major and
                        version.minor == constraint_version.minor and
                        version >= constraint_version)

        else:
            raise ValueError(f"Unknown operator: {operator}")

        return (ConstraintSatisfaction.SATISFIED if satisfied
                else ConstraintSatisfaction.NOT_SATISFIED)

    def resolve_version_conflict(
        self,
        skill_name: str,
        req_version1: str,
        req_version2: str
    ) -> Tuple[Optional[str], bool]:
        """Resolve version conflict between two requirements

        Args:
            skill_name: Name of the skill
            req_version1: Required version 1
            req_version2: Required version 2

        Returns:
            Tuple of (resolution_version, is_resolved)
        """
        print(f"⚠ Version conflict detected for skill: {skill_name}")
        print(f"  Required version 1: {req_version1}")
        print(f"  Required version 2: {req_version2}")

        # Check if versions are compatible
        compat = self.check_compatibility(req_version1, req_version2)

        if compat == CompatibilityStatus.BREAKING:
            print("✗ Breaking change detected (major version mismatch)")
            print("  Cannot automatically resolve. Manual intervention required.")
            print("  Suggestions:")
            print(f"    - Upgrade all dependencies to version {req_version2}")
            print(f"    - Use version {req_version1} until migration is complete")
            print("    - Check migration guide for breaking changes")
            return None, False

        # Use higher version if compatible
        cmp = self.compare_versions(req_version1, req_version2)
        resolution = req_version1 if cmp >= 0 else req_version2

        print(f"✓ Resolved to version: {resolution} (backward compatible)")
        return resolution, True

    def generate_compatibility_matrix(
        self,
        skills_dir: Path,
        output_file: Optional[Path] = None
    ) -> Dict:
        """Generate version compatibility matrix

        Args:
            skills_dir: Path to skills directory
            output_file: Optional output file path

        Returns:
            Compatibility matrix dictionary
        """
        if output_file is None:
            output_file = Path(self.compatibility_matrix_file)

        print("Generating version compatibility matrix...")

        matrix = {
            "generated": datetime.utcnow().isoformat() + "Z",
            "skills": {}
        }

        # Iterate through all skills
        for skill_path in skills_dir.iterdir():
            if not skill_path.is_dir():
                continue

            skill_yml = skill_path / "skill.yml"
            if not skill_yml.exists():
                continue

            skill_name = skill_path.name

            # Load skill.yml
            try:
                with open(skill_yml, 'r') as f:
                    skill_data = yaml.safe_load(f)

                version_str = skill_data.get('version')
                if not version_str:
                    continue

                version = self.parse_version(version_str)

                # Add skill entry
                matrix["skills"][skill_name] = {
                    "version": str(version),
                    "major": version.major,
                    "compatible_with": [f"^{version.major}.0.0"]
                }

            except Exception as e:
                print(f"Warning: Failed to process {skill_name}: {e}")
                continue

        # Write to file
        with open(output_file, 'w') as f:
            json.dump(matrix, f, indent=2)

        print(f"✓ Compatibility matrix generated: {output_file}")
        return matrix

    def get_migration_guide(
        self,
        skill_name: str,
        old_version: str,
        new_version: str,
        skills_dir: Path = Path("skills")
    ) -> Optional[Path]:
        """Get migration guide path for breaking changes

        Args:
            skill_name: Name of the skill
            old_version: Old version string
            new_version: New version string
            skills_dir: Skills directory path

        Returns:
            Path to migration guide if found, None otherwise
        """
        v_old = self.parse_version(old_version)
        v_new = self.parse_version(new_version)

        if v_old.major == v_new.major:
            print("No migration guide needed (backward compatible)")
            return None

        # Look for migration guide
        guide_path = (skills_dir / skill_name /
                     f"MIGRATION-v{v_old.major}-to-v{v_new.major}.md")

        if guide_path.exists():
            print(f"✓ Migration guide available: {guide_path}")
            return guide_path
        else:
            print(f"⚠ Migration guide not found: {guide_path}")
            print("Recommended: Create migration guide documenting breaking changes")
            return None

    def is_breaking_change(self, old_version: str, new_version: str) -> bool:
        """Check if version change is breaking

        Args:
            old_version: Old version string
            new_version: New version string

        Returns:
            True if breaking change, False otherwise
        """
        v_old = self.parse_version(old_version)
        v_new = self.parse_version(new_version)

        return v_old.major != v_new.major

    def suggest_next_version(self, current_version: str, change_type: ChangeType) -> str:
        """Suggest next version based on change type

        Args:
            current_version: Current version string
            change_type: Type of change (major, minor, patch)

        Returns:
            Next version string
        """
        version = self.parse_version(current_version)

        if change_type == ChangeType.MAJOR:
            next_version = Version(version.major + 1, 0, 0)
        elif change_type == ChangeType.MINOR:
            next_version = Version(version.major, version.minor + 1, 0)
        elif change_type == ChangeType.PATCH:
            next_version = Version(version.major, version.minor, version.patch + 1)
        else:
            raise ValueError(f"Invalid change type: {change_type}")

        return str(next_version)

    @staticmethod
    def help():
        """Display version manager help"""
        help_text = f"""
Version Manager v{VERSION_MANAGER_VERSION}
Semantic versioning and compatibility management for Spec-Review Marketplace

USAGE:
    from lib.version_manager import VersionManager

    vm = VersionManager()

METHODS:
    parse_version(version_str: str) -> Version
        Parse semantic version into Version object

    compare_versions(v1: str, v2: str) -> int
        Compare two versions (-1: v1<v2, 0: v1==v2, 1: v1>v2)

    check_compatibility(old_ver: str, new_ver: str) -> CompatibilityStatus
        Check if version change is backward compatible

    validate_version(version_str: str) -> bool
        Validate semantic version format

    check_constraint(version: str, constraint: str) -> ConstraintSatisfaction
        Check if version satisfies constraint (^, ~, >=, etc.)

    resolve_version_conflict(skill: str, v1: str, v2: str) -> Tuple[Optional[str], bool]
        Resolve version conflict between two requirements

    generate_compatibility_matrix(skills_dir: Path, output_file: Path = None) -> Dict
        Generate compatibility matrix for all skills

    get_migration_guide(skill: str, old_ver: str, new_ver: str) -> Optional[Path]
        Get path to migration guide for breaking changes

    is_breaking_change(old_ver: str, new_ver: str) -> bool
        Check if version change is breaking

    suggest_next_version(current_ver: str, change_type: ChangeType) -> str
        Suggest next version (ChangeType.MAJOR|MINOR|PATCH)

EXAMPLES:
    # Parse version
    vm = VersionManager()
    v = vm.parse_version("1.2.3")
    print(f"Major: {{v.major}}, Minor: {{v.minor}}, Patch: {{v.patch}}")

    # Compare versions
    result = vm.compare_versions("1.2.3", "1.2.4")
    if result == -1:
        print("1.2.3 < 1.2.4")

    # Check compatibility
    compat = vm.check_compatibility("1.2.3", "2.0.0")
    if compat == CompatibilityStatus.BREAKING:
        print("Breaking change detected")

    # Resolve conflict
    resolved, ok = vm.resolve_version_conflict("review-spec", "1.2.0", "1.3.0")

    # Generate matrix
    matrix = vm.generate_compatibility_matrix(Path("skills"))

SEMANTIC VERSIONING:
    MAJOR.MINOR.PATCH

    MAJOR: Breaking changes (incompatible API changes)
    MINOR: New features (backward compatible)
    PATCH: Bug fixes (backward compatible)

    Constraint operators:
    ^  - Compatible with version (same major)
    ~  - Compatible with minor version (same major.minor)
    >= - Greater than or equal to
    <= - Less than or equal to
    >  - Greater than
    <  - Less than
    =  - Exact version
"""
        print(help_text)


if __name__ == "__main__":
    # Display help when run directly
    VersionManager.help()
