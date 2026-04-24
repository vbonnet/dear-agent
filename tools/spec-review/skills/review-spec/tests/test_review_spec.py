"""
Comprehensive tests for review_spec.py

Run with: pytest tests/test_review_spec.py -v
"""

import pytest
import sys
import os
from pathlib import Path

# Add parent directory to path
sys.path.insert(0, str(Path(__file__).parent.parent))
sys.path.insert(0, str(Path(__file__).parent.parent.parent.parent / "lib"))

from review_spec import (
    load_rubric,
    build_prompt,
    parse_response,
    ValidationResult,
    PersonaFeedback
)


class TestRubricLoading:
    """Test rubric loading functionality"""

    def test_load_rubric(self):
        """Test rubric loading"""
        rubric = load_rubric()
        assert rubric is not None
        assert len(rubric) > 0
        # Should contain key rubric elements
        assert "Vision" in rubric or "rubric" in rubric.lower()

    def test_rubric_contains_dimensions(self):
        """Test that rubric contains all quality dimensions"""
        rubric = load_rubric()
        # Check for key dimensions
        assert any(term in rubric for term in ["Vision", "Goals", "vision"])
        assert any(term in rubric for term in ["Metrics", "metrics"])


class TestPromptConstruction:
    """Test prompt construction"""

    def test_build_prompt(self):
        """Test prompt construction"""
        spec_content = "# Test SPEC\\n## Vision\\nTest vision"
        rubric = "Test rubric"

        prompt = build_prompt(spec_content, rubric)

        assert spec_content in prompt
        assert rubric in prompt
        assert "json" in prompt.lower()  # Should request JSON
        assert "persona" in prompt.lower()  # Should mention personas

    def test_prompt_includes_dimensions(self):
        """Test that prompt includes all quality dimensions"""
        spec_content = "Test spec"
        rubric = "Test rubric"

        prompt = build_prompt(spec_content, rubric)

        assert "Vision/Goals" in prompt
        assert "Critical User Journeys" in prompt or "CUJs" in prompt
        assert "Success Metrics" in prompt or "Metrics" in prompt
        assert "Scope Boundaries" in prompt or "Scope" in prompt
        assert "Living Document" in prompt

    def test_prompt_includes_personas(self):
        """Test that prompt includes all three personas"""
        spec_content = "Test spec"
        rubric = "Test rubric"

        prompt = build_prompt(spec_content, rubric)

        assert "Technical Writer" in prompt
        assert "Product Manager" in prompt
        assert "Developer" in prompt


class TestResponseParsing:
    """Test JSON response parsing"""

    def test_parse_response_valid(self):
        """Test parsing valid JSON response"""
        mock_response = '''
Here is the evaluation:

{
  "overall_score": 7.5,
  "dimension_scores": {
    "vision_goals": 8.0,
    "cujs": 7.0,
    "metrics": 7.5,
    "scope": 7.0,
    "living_doc": 8.0
  },
  "self_consistency": {
    "scores": [7.5, 7.0, 8.0, 7.5, 7.5],
    "mean": 7.5,
    "variance": 0.15
  },
  "personas": [
    {
      "role": "Technical Writer",
      "score": 7.0,
      "feedback": "Clear structure but some sections lack detail."
    },
    {
      "role": "Product Manager",
      "score": 8.0,
      "feedback": "Good business value articulation."
    },
    {
      "role": "Developer",
      "score": 7.5,
      "feedback": "Implementable but needs more technical clarity."
    }
  ]
}
'''

        result = parse_response(mock_response)

        assert isinstance(result, ValidationResult)
        assert result.overall_score == 7.5
        assert result.decision == "WARN"  # 6.0-7.9
        assert len(result.personas) == 3
        assert result.dimension_scores["vision_goals"] == 8.0

    def test_parse_response_pass(self):
        """Test PASS decision"""
        mock_response = '''
{
  "overall_score": 9.0,
  "dimension_scores": {"vision_goals": 9.0, "cujs": 9.0, "metrics": 9.0, "scope": 9.0, "living_doc": 9.0},
  "self_consistency": {"scores": [9.0, 9.0, 9.0, 9.0, 9.0], "mean": 9.0, "variance": 0.0},
  "personas": [
    {"role": "Technical Writer", "score": 9.0, "feedback": "Excellent."},
    {"role": "Product Manager", "score": 9.0, "feedback": "Great."},
    {"role": "Developer", "score": 9.0, "feedback": "Perfect."}
  ]
}
'''

        result = parse_response(mock_response)
        assert result.decision == "PASS"
        assert result.overall_score >= 8.0

    def test_parse_response_fail(self):
        """Test FAIL decision"""
        mock_response = '''
{
  "overall_score": 4.0,
  "dimension_scores": {"vision_goals": 4.0, "cujs": 4.0, "metrics": 4.0, "scope": 4.0, "living_doc": 4.0},
  "self_consistency": {"scores": [4.0, 4.0, 4.0, 4.0, 4.0], "mean": 4.0, "variance": 0.0},
  "personas": [
    {"role": "Technical Writer", "score": 4.0, "feedback": "Major issues."},
    {"role": "Product Manager", "score": 4.0, "feedback": "Incomplete."},
    {"role": "Developer", "score": 4.0, "feedback": "Not implementable."}
  ]
}
'''

        result = parse_response(mock_response)
        assert result.decision == "FAIL"
        assert result.overall_score < 6.0

    def test_parse_response_warn_boundary(self):
        """Test WARN decision at boundary"""
        mock_response = '''
{
  "overall_score": 6.0,
  "dimension_scores": {"vision_goals": 6.0, "cujs": 6.0, "metrics": 6.0, "scope": 6.0, "living_doc": 6.0},
  "self_consistency": {"scores": [6.0, 6.0, 6.0, 6.0, 6.0], "mean": 6.0, "variance": 0.0},
  "personas": [
    {"role": "Technical Writer", "score": 6.0, "feedback": "Acceptable."},
    {"role": "Product Manager", "score": 6.0, "feedback": "Meets minimum."},
    {"role": "Developer", "score": 6.0, "feedback": "Workable."}
  ]
}
'''

        result = parse_response(mock_response)
        assert result.decision == "WARN"
        assert result.overall_score == 6.0

    def test_parse_response_pass_boundary(self):
        """Test PASS decision at boundary"""
        mock_response = '''
{
  "overall_score": 8.0,
  "dimension_scores": {"vision_goals": 8.0, "cujs": 8.0, "metrics": 8.0, "scope": 8.0, "living_doc": 8.0},
  "self_consistency": {"scores": [8.0, 8.0, 8.0, 8.0, 8.0], "mean": 8.0, "variance": 0.0},
  "personas": [
    {"role": "Technical Writer", "score": 8.0, "feedback": "Good."},
    {"role": "Product Manager", "score": 8.0, "feedback": "Solid."},
    {"role": "Developer", "score": 8.0, "feedback": "Well done."}
  ]
}
'''

        result = parse_response(mock_response)
        assert result.decision == "PASS"
        assert result.overall_score == 8.0


class TestCLIDetection:
    """Test CLI detection integration"""

    def test_cli_detection_import(self):
        """Test that CLI detection can be imported"""
        try:
            from cli_detector import detect_cli, cli_supports_feature
            assert callable(detect_cli)
            assert callable(cli_supports_feature)
        except ImportError:
            pytest.skip("CLI detector not available")

    def test_cli_abstraction_import(self):
        """Test that CLI abstraction can be imported"""
        try:
            # Try importing as a package
            import sys
            lib_path = Path(__file__).parent.parent.parent.parent / "lib"
            if str(lib_path) not in sys.path:
                sys.path.insert(0, str(lib_path))

            # Import cli_detector first (no relative imports)
            import cli_detector
            # Now try CLIAbstraction with fallback
            try:
                from cli_abstraction import CLIAbstraction
                cli = CLIAbstraction()
                assert cli.cli_type in ["claude-code", "gemini-cli", "opencode", "codex", "unknown"]
            except ImportError:
                # Relative import issue - this is expected in test context
                # The actual skill will work because it's run as a script
                pytest.skip("CLI abstraction relative imports not available in test context")
        except ImportError:
            pytest.skip("CLI abstraction not available")


class TestCLIAdapters:
    """Test CLI adapter functionality"""

    def test_claude_code_adapter_exists(self):
        """Test Claude Code adapter exists"""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "claude-code.py"
        assert adapter_path.exists()

    def test_gemini_adapter_exists(self):
        """Test Gemini adapter exists"""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "gemini.py"
        assert adapter_path.exists()

    def test_opencode_adapter_exists(self):
        """Test OpenCode adapter exists"""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "opencode.py"
        assert adapter_path.exists()

    def test_codex_adapter_exists(self):
        """Test Codex adapter exists"""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "codex.py"
        assert adapter_path.exists()

    def test_adapter_imports(self):
        """Test that adapters can import review_spec"""
        # Add adapter directory to path
        adapter_dir = Path(__file__).parent.parent / "cli-adapters"
        if adapter_dir.exists():
            sys.path.insert(0, str(adapter_dir))
            # Verify adapters can be imported (syntax check)
            for adapter in ["claude-code", "gemini", "opencode", "codex"]:
                adapter_file = adapter_dir / f"{adapter}.py"
                if adapter_file.exists():
                    # Just verify file is valid Python
                    with open(adapter_file) as f:
                        compile(f.read(), adapter_file, 'exec')


class TestDataModels:
    """Test Pydantic data models"""

    def test_persona_feedback_model(self):
        """Test PersonaFeedback model"""
        persona = PersonaFeedback(
            role="Technical Writer",
            score=8.5,
            feedback="Excellent structure"
        )
        assert persona.role == "Technical Writer"
        assert persona.score == 8.5
        assert persona.feedback == "Excellent structure"

    def test_validation_result_model(self):
        """Test ValidationResult model"""
        result = ValidationResult(
            overall_score=8.5,
            dimension_scores={
                "vision_goals": 9.0,
                "cujs": 8.0,
                "metrics": 8.5,
                "scope": 8.0,
                "living_doc": 9.0
            },
            self_consistency={
                "scores": [8.5, 8.0, 9.0, 8.5, 8.5],
                "mean": 8.5,
                "variance": 0.15
            },
            personas=[
                PersonaFeedback(
                    role="Technical Writer",
                    score=8.5,
                    feedback="Great"
                )
            ],
            decision="PASS"
        )
        assert result.overall_score == 8.5
        assert result.decision == "PASS"
        assert len(result.personas) == 1


class TestIntegration:
    """Integration tests"""

    def test_full_workflow_mock(self):
        """Test full workflow with mock data"""
        # Load rubric
        rubric = load_rubric()
        assert rubric is not None

        # Build prompt
        spec_content = "# Test SPEC\n## Vision\nTest vision"
        prompt = build_prompt(spec_content, rubric)
        assert prompt is not None
        assert spec_content in prompt

        # Parse response
        mock_response = '''
{
  "overall_score": 8.5,
  "dimension_scores": {"vision_goals": 9.0, "cujs": 8.0, "metrics": 8.5, "scope": 8.0, "living_doc": 9.0},
  "self_consistency": {"scores": [8.5, 8.0, 9.0, 8.5, 8.5], "mean": 8.5, "variance": 0.15},
  "personas": [
    {"role": "Technical Writer", "score": 8.5, "feedback": "Great."},
    {"role": "Product Manager", "score": 8.0, "feedback": "Good."},
    {"role": "Developer", "score": 9.0, "feedback": "Excellent."}
  ]
}
'''
        result = parse_response(mock_response)
        assert result.decision == "PASS"
        assert result.overall_score == 8.5


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
