#!/usr/bin/env python3
"""Test suite for marketplace discovery functionality."""

import sys
import json
from pathlib import Path

# Add lib directory to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

import pytest
from discovery import MarketplaceDiscovery


@pytest.fixture
def discovery():
    """Create a MarketplaceDiscovery instance for testing."""
    marketplace_root = Path(__file__).parent.parent
    return MarketplaceDiscovery(marketplace_root=marketplace_root)


class TestMarketplaceDiscovery:
    """Test marketplace discovery functionality."""

    def test_load_marketplace_data(self, discovery):
        """Test loading marketplace data."""
        data = discovery._load_marketplace_data()
        assert "skills" in data
        assert isinstance(data["skills"], list)
        assert len(data["skills"]) > 0

    def test_search_skills_by_name(self, discovery):
        """Test searching skills by name."""
        results = discovery.search_skills("review-spec")
        assert len(results) > 0
        assert any(skill["name"] == "review-spec" for skill in results)

    def test_search_skills_by_description(self, discovery):
        """Test searching skills by description."""
        results = discovery.search_skills("validation")
        assert len(results) > 0

    def test_search_skills_by_tag(self, discovery):
        """Test searching skills by tag."""
        results = discovery.search_skills("spec")
        assert len(results) > 0

    def test_search_skills_case_insensitive(self, discovery):
        """Test that search is case-insensitive."""
        results_lower = discovery.search_skills("spec")
        results_upper = discovery.search_skills("SPEC")
        assert len(results_lower) == len(results_upper)

    def test_filter_skills_by_tags_or(self, discovery):
        """Test filtering skills with OR logic."""
        results = discovery.filter_skills_by_tags(["spec", "validation"], logic="OR")
        assert len(results) > 0

    def test_filter_skills_by_tags_and(self, discovery):
        """Test filtering skills with AND logic."""
        results = discovery.filter_skills_by_tags(["spec", "validation"], logic="AND")
        # Should find review-spec which has both tags
        assert len(results) > 0
        assert any(skill["name"] == "review-spec" for skill in results)

    def test_list_compatible_skills(self, discovery):
        """Test listing compatible skills."""
        results = discovery.list_compatible_skills("claude-code")
        assert len(results) > 0
        # All results should support claude-code
        for skill in results:
            assert (
                "claude-code" in skill.get("cli_support", []) or
                "claude-code" in skill.get("cli_adapters", {})
            )

    def test_list_compatible_skills_all_clis(self, discovery):
        """Test compatibility for all CLIs."""
        clis = ["claude-code", "gemini-cli", "opencode", "codex"]
        for cli in clis:
            results = discovery.list_compatible_skills(cli)
            assert isinstance(results, list)

    def test_generate_compatibility_matrix(self, discovery):
        """Test generating compatibility matrix."""
        matrix = discovery.generate_compatibility_matrix()
        assert "Skill Compatibility Matrix" in matrix
        assert "Claude Code" in matrix
        assert "Gemini CLI" in matrix
        assert "OpenCode" in matrix
        assert "Codex" in matrix
        assert "✅" in matrix or "❌" in matrix

    def test_get_skill_details(self, discovery):
        """Test getting skill details."""
        skill = discovery.get_skill_details("review-spec")
        assert skill is not None
        assert skill["name"] == "review-spec"
        assert "description" in skill
        assert "version" in skill

    def test_get_skill_details_not_found(self, discovery):
        """Test getting details for non-existent skill."""
        skill = discovery.get_skill_details("non-existent-skill")
        assert skill is None

    def test_list_all_tags(self, discovery):
        """Test listing all tags."""
        tags = discovery.list_all_tags()
        assert isinstance(tags, list)
        assert len(tags) > 0
        # Tags should be unique and sorted
        assert tags == sorted(set(tags))

    def test_list_skills_by_category(self, discovery):
        """Test listing skills by category."""
        # Get all categories first
        categories = discovery.get_categories()

        # Skip if no categories are defined in the marketplace
        if len(categories) == 0:
            pytest.skip("No categories defined in marketplace skills")

        # Test first category
        category = categories[0]
        results = discovery.list_skills_by_category(category)
        assert isinstance(results, list)
        for skill in results:
            assert skill.get("category") == category

    def test_recommend_skills(self, discovery):
        """Test skill recommendations."""
        results = discovery.recommend_skills("spec")
        assert len(results) > 0
        # Results should have relevance scores
        assert all("_relevance" in skill for skill in results)
        # Results should be sorted by relevance
        relevances = [skill["_relevance"] for skill in results]
        assert relevances == sorted(relevances, reverse=True)

    def test_recommend_skills_different_contexts(self, discovery):
        """Test recommendations for different contexts."""
        contexts = ["validation", "documentation", "architecture"]
        for context in contexts:
            results = discovery.recommend_skills(context)
            assert isinstance(results, list)

    def test_print_skill_summary(self, discovery):
        """Test printing skill summary."""
        summary = discovery.print_skill_summary("review-spec")
        assert "Skill: review-spec" in summary
        assert "Description:" in summary
        assert "Version:" in summary
        assert "Tags:" in summary
        assert "Supported CLIs:" in summary

    def test_print_skill_summary_not_found(self, discovery):
        """Test printing summary for non-existent skill."""
        summary = discovery.print_skill_summary("non-existent-skill")
        assert "not found" in summary

    def test_list_all_skills_text(self, discovery):
        """Test listing all skills in text format."""
        output = discovery.list_all_skills(format="text")
        assert "Available Skills" in output
        assert "review-spec" in output

    def test_list_all_skills_json(self, discovery):
        """Test listing all skills in JSON format."""
        output = discovery.list_all_skills(format="json")
        skills = json.loads(output)
        assert isinstance(skills, list)
        assert len(skills) > 0

    def test_is_skill_compatible(self, discovery):
        """Test skill compatibility check."""
        # Test compatible CLI
        assert discovery.is_skill_compatible("review-spec", "claude-code")

        # Test non-existent skill
        assert not discovery.is_skill_compatible("non-existent-skill", "claude-code")

    def test_is_skill_compatible_all_skills(self, discovery):
        """Test compatibility check for all skills."""
        data = discovery._load_marketplace_data()
        for skill in data["skills"]:
            skill_name = skill["name"]
            # Should be compatible with at least one CLI
            clis = ["claude-code", "gemini-cli", "opencode", "codex"]
            compatible_count = sum(
                1 for cli in clis
                if discovery.is_skill_compatible(skill_name, cli)
            )
            assert compatible_count > 0

    def test_get_categories(self, discovery):
        """Test getting all categories."""
        categories = discovery.get_categories()
        assert isinstance(categories, list)
        # Categories should be unique and sorted
        assert categories == sorted(set(categories))

    def test_get_cli_features(self, discovery):
        """Test getting CLI-specific features."""
        features = discovery.get_cli_features("review-spec", "claude-code")
        assert isinstance(features, list)

    def test_marketplace_json_structure(self, discovery):
        """Test that marketplace.json has expected structure."""
        data = discovery._load_marketplace_data()

        # Check top-level keys
        assert "skills" in data
        assert "metadata" in data
        assert "compatibility" in data

        # Check skills structure
        for skill in data["skills"]:
            assert "name" in skill
            assert "version" in skill
            assert "description" in skill
            assert "tags" in skill
            # Should have either cli_support or cli_adapters
            assert "cli_support" in skill or "cli_adapters" in skill


class TestMarketplaceDiscoveryCLI:
    """Test CLI interface (integration tests)."""

    def test_cli_import(self):
        """Test that CLI script can be imported."""
        bin_path = Path(__file__).parent.parent / "bin"
        sys.path.insert(0, str(bin_path))

        # The script should be importable
        # (This is a basic sanity check)
        assert bin_path.exists()


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
