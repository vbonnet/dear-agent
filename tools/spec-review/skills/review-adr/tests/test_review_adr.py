#!/usr/bin/env python3
"""Comprehensive tests for review-adr skill.
Tests all CLI adapters, scoring logic, and edge cases.
"""

import sys
import json
import tempfile
from pathlib import Path

# Add paths for imports
SKILL_DIR = Path(__file__).parent.parent
PLUGIN_ROOT = SKILL_DIR.parent.parent
sys.path.insert(0, str(SKILL_DIR))
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))

from review_adr import (
    ADRDocument, ADRValidator, generate_report,
    MIN_PASS_SCORE
)
from cli_abstraction import CLIAbstraction


# Test ADR documents
GOOD_ADR = """# ADR-001: Use PostgreSQL for User Data

## Status
Accepted (2026-02-01)

## Context
We need a database for user account data (credentials, profiles, preferences). Requirements:
- ACID guarantees for financial transactions
- Complex relational queries (user→orders→items joins)
- Strong consistency for auth workflows

## Decision
Use PostgreSQL as primary database for user data.

## Alternatives Considered

### MongoDB
- Pros: Schema flexibility, horizontal scaling
- Cons: Eventual consistency not acceptable for auth, weak join performance
- Rejected: ACID guarantees non-negotiable for financial transactions

### MySQL
- Pros: ACID, strong ecosystem, familiar to team
- Cons: JSON support weaker than Postgres, licensing concerns (Oracle)
- Rejected: Postgres JSON capabilities superior for profile data

## Consequences

### Positive
- ACID guarantees protect financial transaction integrity
- Complex joins support analytics queries (30% faster than document DB testing showed)
- JSON columns handle flexible profile data without schema migrations
- pg_trgm enables fast text search (300ms → 50ms for user lookup)

### Negative
- Vertical scaling limits (single-node writes, ~10K writes/sec ceiling)
- Higher ops complexity vs managed NoSQL (require DB admin expertise)
- Write-heavy workloads may hit bottleneck (current: 2K/sec, growth: 20%/quarter)
"""

POOR_ADR = """# ADR-002: Microservices Architecture

## Status
Proposed

## Decision
We will use microservices. Here's the implementation:

```python
class UserService:
    def create_user(self, data):
        if not data.get('email'):
            raise ValueError("Email required")
        auth_token = self.auth_client.generate_token(data['email'])
        self.db.users.insert(data)
```

Each service will have its own database. Use REST APIs for communication.
"""

AGENTIC_ADR = """# ADR-003: Multi-Agent Code Review System

## Status
Accepted

## Context
Need automated code review with multiple specialized perspectives.
Manual reviews bottleneck at 50+ PRs/day.

## Decision
Deploy multi-agent code review system with specialized personas.

## Agent Context
- Agent type: Multi-agent autonomous
- Autonomy level: High
- Task complexity: Complex (code analysis, security, performance)
- Scale: 100 PRs/day, 3 concurrent agents
- Criticality: Medium (human oversight for critical changes)

## Architecture
- Pattern: Parallel evaluation with consensus aggregation
- Models: Sonnet 4.5 default, Opus escalation for conflicts
- Tools: MCP (GitHub API, static analysis)
- Memory: Redis (short-term PR context), Pinecone (historical patterns)
- Coordination: Event queue (RabbitMQ)

## Validation
- Success metrics: 90% accuracy vs human reviews, <5min latency
- Evaluation strategy: Human review sample (10%), LLM-judge (90%)
- Rollback plan: Trigger on <80% accuracy, fallback to manual, 2-week migration

## Consequences

### Positive
- 3x throughput increase (50 → 150 PRs/day)
- Consistent review quality across timezones

### Negative
- $500/month LLM costs
- 2-month implementation timeline
"""


class TestADRDocument:
    """Test ADR document parsing."""

    def test_parse_sections_good_adr(self):
        """Test parsing well-formed ADR."""
        adr = ADRDocument("test.md", GOOD_ADR)

        assert adr.has_section("status")
        assert adr.has_section("context")
        assert adr.has_section("decision")
        assert adr.has_section("consequences")
        assert adr.has_section("alternatives considered")

        status = adr.get_section("status")
        assert "Accepted" in status.content

    def test_parse_sections_poor_adr(self):
        """Test parsing ADR with missing sections."""
        adr = ADRDocument("test.md", POOR_ADR)

        assert adr.has_section("status")
        assert adr.has_section("decision")
        assert not adr.has_section("context")
        assert not adr.has_section("consequences")

    def test_case_insensitive_sections(self):
        """Test case-insensitive section lookup."""
        adr = ADRDocument("test.md", GOOD_ADR)

        assert adr.has_section("Status")
        assert adr.has_section("STATUS")
        assert adr.has_section("status")


class TestADRValidator:
    """Test ADR validation logic."""

    def test_validate_section_presence_all_present(self):
        """Test section presence with all required sections."""
        cli = CLIAbstraction()
        validator = ADRValidator(cli)
        adr = ADRDocument("test.md", GOOD_ADR)

        score, feedback = validator.validate_section_presence(adr)

        assert score == 20
        assert "All required sections present" in feedback

    def test_validate_section_presence_missing_two(self):
        """Test section presence with 2 missing sections."""
        cli = CLIAbstraction()
        validator = ADRValidator(cli)
        adr = ADRDocument("test.md", POOR_ADR)

        score, feedback = validator.validate_section_presence(adr)

        assert score == 10
        assert "Missing 2 sections" in feedback

    def test_validate_section_presence_auto_fail(self):
        """Test auto-fail on 3+ missing sections."""
        minimal_adr = """# ADR
## Status
Proposed
"""
        cli = CLIAbstraction()
        validator = ADRValidator(cli)
        adr = ADRDocument("test.md", minimal_adr)

        score, feedback = validator.validate_section_presence(adr)

        assert score == 0
        assert "AUTO-FAIL" in feedback

    def test_validate_good_adr(self):
        """Test validation of good ADR."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write(GOOD_ADR)
            f.flush()
            temp_path = f.name

        try:
            cli = CLIAbstraction()
            validator = ADRValidator(cli)
            result = validator.validate_adr(temp_path)

            assert result["success"]
            assert result["score_100"] >= MIN_PASS_SCORE
            assert result["passed"]
            assert result["score_10"] >= 8
            assert len(result["reviews"]) == 3

        finally:
            Path(temp_path).unlink()

    def test_validate_poor_adr(self):
        """Test validation of poor ADR."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write(POOR_ADR)
            f.flush()
            temp_path = f.name

        try:
            cli = CLIAbstraction()
            validator = ADRValidator(cli)
            result = validator.validate_adr(temp_path)

            assert result["success"]
            assert result["score_100"] < MIN_PASS_SCORE
            assert not result["passed"]
            assert result["score_10"] < 8

        finally:
            Path(temp_path).unlink()

    def test_validate_agentic_adr(self):
        """Test validation of agentic ADR."""
        with tempfile.NamedTemporaryFile(mode='w', suffix='.md', delete=False) as f:
            f.write(AGENTIC_ADR)
            f.flush()
            temp_path = f.name

        try:
            cli = CLIAbstraction()
            validator = ADRValidator(cli)
            result = validator.validate_adr(temp_path)

            assert result["success"]
            # Agentic ADR should pass with proper sections
            assert len(result["reviews"]) == 3

        finally:
            Path(temp_path).unlink()

    def test_validate_missing_file(self):
        """Test validation of non-existent file."""
        cli = CLIAbstraction()
        validator = ADRValidator(cli)
        result = validator.validate_adr("/nonexistent/file.md")

        assert not result["success"]
        assert "error" in result

    def test_aggregate_scores(self):
        """Test score aggregation and normalization."""
        from review_adr import PersonaReview

        cli = CLIAbstraction()
        validator = ADRValidator(cli)

        reviews = [
            PersonaReview("Persona1", 40, 45, "feedback1"),
            PersonaReview("Persona2", 30, 35, "feedback2"),
            PersonaReview("Persona3", 25, 30, "feedback3"),
        ]

        normalized, raw = validator.aggregate_scores(reviews)

        assert raw == 95
        # (95 / 110) * 100 ≈ 86
        assert 85 <= normalized <= 87

    def test_map_to_ten_scale(self):
        """Test score mapping to 1-10 scale."""
        cli = CLIAbstraction()
        validator = ADRValidator(cli)

        assert validator.map_to_ten_scale(95) == 10
        assert validator.map_to_ten_scale(85) == 9
        assert validator.map_to_ten_scale(75) == 8
        assert validator.map_to_ten_scale(65) == 7
        assert validator.map_to_ten_scale(55) == 6
        assert validator.map_to_ten_scale(45) == 5
        assert validator.map_to_ten_scale(35) == 3


class TestReportGeneration:
    """Test report generation."""

    def test_generate_markdown_report(self):
        """Test markdown report generation."""
        result = {
            "success": True,
            "file": "test.md",
            "score_100": 85,
            "score_10": 9,
            "raw_score": 93,
            "passed": True,
            "threshold": 70,
            "reviews": [
                {
                    "persona": "Test Persona",
                    "score": 40,
                    "max_score": 45,
                    "feedback": "Good work"
                }
            ]
        }

        report = generate_report(result, "markdown")

        assert "# ADR Validation Report" in report
        assert "9/10" in report
        assert "✓ PASS" in report
        assert "Test Persona" in report

    def test_generate_json_report(self):
        """Test JSON report generation."""
        result = {
            "success": True,
            "file": "test.md",
            "score_100": 85,
            "score_10": 9,
            "raw_score": 93,
            "passed": True,
            "threshold": 70,
            "reviews": []
        }

        report = generate_report(result, "json")
        parsed = json.loads(report)

        assert parsed["success"]
        assert parsed["score_10"] == 9
        assert parsed["passed"]

    def test_generate_error_report(self):
        """Test error report generation."""
        result = {
            "success": False,
            "error": "File not found"
        }

        report = generate_report(result, "markdown")

        assert "ERROR" in report
        assert "File not found" in report


class TestCLIAdapters:
    """Test CLI adapter functionality."""

    def test_claude_code_adapter(self):
        """Test Claude Code adapter."""
        # Import adapter
        adapter_path = SKILL_DIR / "cli-adapters" / "claude-code.py"
        assert adapter_path.exists()

        # Test optimization function
        from cli_abstraction import CLIAbstraction
        cli = CLIAbstraction(cli_type="claude-code")

        assert cli.cli_type == "claude-code"
        assert cli.supports_feature("caching")

    def test_gemini_adapter(self):
        """Test Gemini adapter."""
        adapter_path = SKILL_DIR / "cli-adapters" / "gemini.py"
        assert adapter_path.exists()

        cli = CLIAbstraction(cli_type="gemini-cli")
        assert cli.cli_type == "gemini-cli"
        assert cli.get_batch_size() == 20

    def test_opencode_adapter(self):
        """Test OpenCode adapter."""
        adapter_path = SKILL_DIR / "cli-adapters" / "opencode.py"
        assert adapter_path.exists()

        cli = CLIAbstraction(cli_type="opencode")
        assert cli.cli_type == "opencode"
        assert cli.supports_feature("mcp")

    def test_codex_adapter(self):
        """Test Codex adapter."""
        adapter_path = SKILL_DIR / "cli-adapters" / "codex.py"
        assert adapter_path.exists()

        cli = CLIAbstraction(cli_type="codex")
        assert cli.cli_type == "codex"


def run_tests():
    """Run all tests."""
    import pytest
    return pytest.main([__file__, "-v"])


if __name__ == "__main__":
    sys.exit(run_tests())
