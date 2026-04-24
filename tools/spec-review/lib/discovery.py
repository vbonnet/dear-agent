#!/usr/bin/env python3
"""Marketplace Discovery Library
Provides skill search, filtering, and discovery functionality.
Supports tag-based filtering, CLI-specific listings, and compatibility checks.
"""

import json
import os
from pathlib import Path
from typing import List, Dict, Any, Optional, Literal
from cli_detector import detect_cli, CLIType


class MarketplaceDiscovery:
    """Marketplace discovery and search functionality."""

    def __init__(self, marketplace_root: Optional[Path] = None, cli_type: Optional[CLIType] = None):
        """Initialize marketplace discovery.

        Args:
            marketplace_root: Path to marketplace root (auto-detected if None)
            cli_type: CLI type (auto-detected if None)
        """
        if marketplace_root is None:
            # Auto-detect marketplace root (parent of lib/)
            marketplace_root = Path(__file__).parent.parent

        self.marketplace_root = Path(marketplace_root)
        self.marketplace_json = self.marketplace_root / "marketplace.json"
        self.cli_type = cli_type or detect_cli()
        self._marketplace_data: Optional[Dict[str, Any]] = None

    def _load_marketplace_data(self) -> Dict[str, Any]:
        """Load marketplace.json data.

        Returns:
            Marketplace data dictionary
        """
        if self._marketplace_data is None:
            if not self.marketplace_json.exists():
                return {"skills": [], "metadata": {}, "compatibility": {}}

            with open(self.marketplace_json, 'r', encoding='utf-8') as f:
                self._marketplace_data = json.load(f)

        return self._marketplace_data

    def search_skills(self, query: str) -> List[Dict[str, Any]]:
        """Search skills by name, description, or tags.

        Args:
            query: Search query string

        Returns:
            List of matching skills
        """
        data = self._load_marketplace_data()
        query_lower = query.lower()
        results = []

        for skill in data.get("skills", []):
            # Search in name, description, and tags
            if (
                query_lower in skill.get("name", "").lower() or
                query_lower in skill.get("description", "").lower() or
                any(query_lower in tag.lower() for tag in skill.get("tags", []))
            ):
                results.append(skill)

        return results

    def filter_skills_by_tags(
        self,
        tags: List[str],
        logic: Literal["AND", "OR"] = "OR"
    ) -> List[Dict[str, Any]]:
        """Filter skills by tags with AND/OR logic.

        Args:
            tags: List of tags to filter by
            logic: Filter logic ("AND" or "OR")

        Returns:
            List of matching skills
        """
        data = self._load_marketplace_data()
        results = []
        tags_lower = [tag.lower() for tag in tags]

        for skill in data.get("skills", []):
            skill_tags = [tag.lower() for tag in skill.get("tags", [])]

            if logic == "AND":
                # All tags must be present
                if all(tag in skill_tags for tag in tags_lower):
                    results.append(skill)
            else:
                # Any tag can be present (OR logic)
                if any(tag in skill_tags for tag in tags_lower):
                    results.append(skill)

        return results

    def list_compatible_skills(self, cli_type: Optional[CLIType] = None) -> List[Dict[str, Any]]:
        """List skills compatible with specified CLI.

        Args:
            cli_type: CLI type (uses current CLI if None)

        Returns:
            List of compatible skills
        """
        data = self._load_marketplace_data()
        cli = cli_type or self.cli_type
        results = []

        for skill in data.get("skills", []):
            # Check cli_support field (list format)
            if cli in skill.get("cli_support", []):
                results.append(skill)
            # Also check cli_adapters field (dict format)
            elif cli in skill.get("cli_adapters", {}):
                results.append(skill)

        return results

    def generate_compatibility_matrix(self) -> str:
        """Generate compatibility matrix as markdown table.

        Returns:
            Markdown table string
        """
        data = self._load_marketplace_data()
        skills = data.get("skills", [])
        clis = ["claude-code", "gemini-cli", "opencode", "codex"]

        if not skills:
            return "No skills found in marketplace."

        # Build markdown table
        lines = [
            "# Skill Compatibility Matrix",
            "",
            "| Skill | Claude Code | Gemini CLI | OpenCode | Codex |",
            "|-------|-------------|------------|----------|-------|"
        ]

        for skill in skills:
            skill_name = skill.get("name", "Unknown")
            row = [f"| {skill_name}"]

            for cli in clis:
                # Check both cli_support (list) and cli_adapters (dict)
                is_supported = (
                    cli in skill.get("cli_support", []) or
                    cli in skill.get("cli_adapters", {})
                )
                row.append("✅" if is_supported else "❌")

            lines.append(" | ".join(row) + " |")

        lines.extend([
            "",
            "**Legend:** ✅ Supported | ❌ Not Supported"
        ])

        return "\n".join(lines)

    def get_skill_details(self, skill_name: str) -> Optional[Dict[str, Any]]:
        """Get detailed information about a skill.

        Args:
            skill_name: Name of the skill

        Returns:
            Skill details dictionary or None if not found
        """
        data = self._load_marketplace_data()

        for skill in data.get("skills", []):
            if skill.get("name") == skill_name:
                return skill

        return None

    def list_all_tags(self) -> List[str]:
        """List all unique tags across all skills.

        Returns:
            Sorted list of unique tags
        """
        data = self._load_marketplace_data()
        tags = set()

        for skill in data.get("skills", []):
            tags.update(skill.get("tags", []))

        return sorted(tags)

    def list_skills_by_category(self, category: str) -> List[Dict[str, Any]]:
        """List skills in a specific category.

        Args:
            category: Category name

        Returns:
            List of skills in that category
        """
        data = self._load_marketplace_data()
        results = []

        for skill in data.get("skills", []):
            if skill.get("category") == category:
                results.append(skill)

        return results

    def recommend_skills(self, context: str) -> List[Dict[str, Any]]:
        """Recommend skills based on context.

        Args:
            context: Context string (e.g., "git", "github", "documentation")

        Returns:
            List of recommended skills, sorted by relevance
        """
        data = self._load_marketplace_data()
        context_lower = context.lower()
        scored_skills = []

        for skill in data.get("skills", []):
            relevance = 0

            # Check tags (higher weight)
            if any(context_lower in tag.lower() for tag in skill.get("tags", [])):
                relevance += 2

            # Check description
            if context_lower in skill.get("description", "").lower():
                relevance += 1

            if relevance > 0:
                skill_with_score = skill.copy()
                skill_with_score["_relevance"] = relevance
                scored_skills.append(skill_with_score)

        # Sort by relevance (descending)
        scored_skills.sort(key=lambda s: s["_relevance"], reverse=True)
        return scored_skills

    def print_skill_summary(self, skill_name: str) -> str:
        """Format skill summary for display.

        Args:
            skill_name: Name of the skill

        Returns:
            Formatted skill summary
        """
        skill = self.get_skill_details(skill_name)

        if skill is None:
            return f"Skill '{skill_name}' not found"

        lines = [
            f"=== Skill: {skill_name} ===",
            "",
            f"Description: {skill.get('description', 'N/A')}",
            f"Version: {skill.get('version', 'N/A')}",
            f"Category: {skill.get('category', 'N/A')}",
            "",
            "Tags:"
        ]

        for tag in skill.get("tags", []):
            lines.append(f"  - {tag}")

        lines.extend([
            "",
            "Supported CLIs:"
        ])

        # Check both cli_support and cli_adapters
        supported_clis = set(skill.get("cli_support", []))
        supported_clis.update(skill.get("cli_adapters", {}).keys())

        for cli in sorted(supported_clis):
            lines.append(f"  - {cli}")

        lines.extend([
            "",
            f"Entry Point: {skill.get('entry_point', 'N/A')}"
        ])

        # Add dependencies if present
        dependencies = skill.get("dependencies", [])
        if dependencies:
            lines.extend([
                "",
                "Dependencies:"
            ])
            for dep in dependencies:
                lines.append(f"  - {dep}")

        return "\n".join(lines)

    def list_all_skills(self, format: Literal["json", "text"] = "text") -> str:
        """List all available skills.

        Args:
            format: Output format ("json" or "text")

        Returns:
            Formatted list of skills
        """
        data = self._load_marketplace_data()
        skills = data.get("skills", [])

        if format == "json":
            return json.dumps(skills, indent=2)

        lines = ["=== Available Skills ===", ""]

        for skill in skills:
            name = skill.get("name", "Unknown")
            version = skill.get("version", "N/A")
            description = skill.get("description", "No description")
            lines.append(f"- {name} ({version}): {description}")

        return "\n".join(lines)

    def is_skill_compatible(
        self,
        skill_name: str,
        cli_type: Optional[CLIType] = None
    ) -> bool:
        """Check if skill is compatible with specified CLI.

        Args:
            skill_name: Name of the skill
            cli_type: CLI type (uses current CLI if None)

        Returns:
            True if compatible, False otherwise
        """
        skill = self.get_skill_details(skill_name)
        if skill is None:
            return False

        cli = cli_type or self.cli_type

        # Check both cli_support (list) and cli_adapters (dict)
        return (
            cli in skill.get("cli_support", []) or
            cli in skill.get("cli_adapters", {})
        )

    def get_cli_features(
        self,
        skill_name: str,
        cli_type: Optional[CLIType] = None
    ) -> List[str]:
        """Get CLI-specific features for a skill.

        Args:
            skill_name: Name of the skill
            cli_type: CLI type (uses current CLI if None)

        Returns:
            List of feature names
        """
        cli = cli_type or self.cli_type

        # Try to load from skill.yml if it exists
        skill = self.get_skill_details(skill_name)
        if skill is None:
            return []

        skill_path = self.marketplace_root / skill.get("path", "")
        skill_yml = skill_path / "skill.yml"

        if skill_yml.exists():
            try:
                import yaml
                with open(skill_yml, 'r', encoding='utf-8') as f:
                    skill_data = yaml.safe_load(f)
                    cli_features = skill_data.get("cli_features", {})
                    return cli_features.get(cli, [])
            except ImportError:
                # yaml not available, fall back to empty list
                pass
            except Exception:
                # Error reading file
                pass

        return []

    def get_categories(self) -> List[str]:
        """Get all unique categories.

        Returns:
            Sorted list of unique categories
        """
        data = self._load_marketplace_data()
        categories = set()

        for skill in data.get("skills", []):
            if "category" in skill:
                categories.add(skill["category"])

        return sorted(categories)


# Module-level default instance
discovery = MarketplaceDiscovery()


def main():
    """CLI interface for discovery library."""
    import sys

    if len(sys.argv) < 2:
        print("Usage: discovery.py <command> [args...]")
        print("\nCommands:")
        print("  list              - List all skills")
        print("  search <query>    - Search skills")
        print("  tags              - List all tags")
        print("  matrix            - Show compatibility matrix")
        print("  info <skill>      - Show skill details")
        print("  compatible [cli]  - List compatible skills")
        sys.exit(1)

    command = sys.argv[1]

    if command == "list":
        print(discovery.list_all_skills())
    elif command == "search" and len(sys.argv) > 2:
        query = sys.argv[2]
        results = discovery.search_skills(query)
        print(f"Found {len(results)} skill(s):")
        for skill in results:
            print(f"  - {skill['name']}: {skill['description']}")
    elif command == "tags":
        tags = discovery.list_all_tags()
        print("Available tags:")
        for tag in tags:
            print(f"  - {tag}")
    elif command == "matrix":
        print(discovery.generate_compatibility_matrix())
    elif command == "info" and len(sys.argv) > 2:
        skill_name = sys.argv[2]
        print(discovery.print_skill_summary(skill_name))
    elif command == "compatible":
        cli = sys.argv[2] if len(sys.argv) > 2 else None
        skills = discovery.list_compatible_skills(cli)
        cli_name = cli or discovery.cli_type
        print(f"Skills compatible with {cli_name}:")
        for skill in skills:
            print(f"  - {skill['name']}")
    else:
        print(f"Unknown command: {command}")
        sys.exit(1)


if __name__ == "__main__":
    main()
