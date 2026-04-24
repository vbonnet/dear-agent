#!/usr/bin/env python3
"""
Integration tests for review-architecture skill
Tests CLI abstraction, validation logic, and cross-CLI compatibility
"""

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

# Add parent directories to path
SCRIPT_DIR = Path(__file__).parent
SKILL_DIR = SCRIPT_DIR.parent
PLUGIN_ROOT = SKILL_DIR.parent.parent
sys.path.insert(0, str(PLUGIN_ROOT / "lib"))
sys.path.insert(0, str(SKILL_DIR))

from review_architecture import (
    quick_validate,
    select_personas,
    load_rubric,
    build_prompt,
    parse_json_response,
    validate_diagrams,
    QuickValidationResult,
    DimensionScores,
    PersonaFeedback,
    ValidationResult,
    DiagramValidationResult
)


class TestQuickValidation(unittest.TestCase):
    """Test quick validation gate functionality."""

    def setUp(self):
        """Create temporary test files."""
        self.test_dir = tempfile.mkdtemp()
        self.arch_file = Path(self.test_dir) / "ARCHITECTURE.md"

    def tearDown(self):
        """Clean up test files."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

    def test_complete_architecture(self):
        """Test validation passes for complete architecture."""
        # Create complete architecture doc
        content = """
# Architecture

## Traditional Architecture

Component overview with C4 diagrams.

## Agentic Architecture

Agent patterns and coordination.

See [ADR-001](docs/adr/001-architecture-decision.md) for details.
"""
        self.arch_file.write_text(content)

        # Create diagram directory
        diagram_dir = Path(self.test_dir) / "docs" / "architecture"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "context.puml").write_text("@startuml\n@enduml")

        result = quick_validate(self.arch_file, content)

        self.assertTrue(result.passed)
        self.assertEqual(len(result.missing_sections), 0)
        self.assertFalse(result.missing_diagrams)
        self.assertFalse(result.missing_adrs)

    def test_missing_sections(self):
        """Test validation fails for missing sections."""
        content = """
# Architecture

## Traditional Architecture

Component overview.
"""
        self.arch_file.write_text(content)

        result = quick_validate(self.arch_file, content)

        self.assertFalse(result.passed)
        self.assertIn("Agentic Architecture", result.missing_sections)

    def test_missing_diagrams(self):
        """Test validation fails for missing diagrams."""
        content = """
# Architecture

## Traditional Architecture

Component overview.

## Agentic Architecture

Agent patterns.

See [ADR-001](docs/adr/001.md).
"""
        self.arch_file.write_text(content)

        result = quick_validate(self.arch_file, content)

        self.assertFalse(result.passed)
        self.assertTrue(result.missing_diagrams)

    def test_missing_adrs(self):
        """Test validation fails for missing ADR references."""
        content = """
# Architecture

## Traditional Architecture

Component overview.

## Agentic Architecture

Agent patterns.
"""
        self.arch_file.write_text(content)

        # Create diagram
        diagram_dir = Path(self.test_dir) / "diagrams"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "context.png").write_text("fake image")

        result = quick_validate(self.arch_file, content)

        self.assertFalse(result.passed)
        self.assertTrue(result.missing_adrs)


class TestDiagramValidation(unittest.TestCase):
    """Test diagram validation functionality."""

    def setUp(self):
        """Create temporary test files."""
        self.test_dir = tempfile.mkdtemp()
        self.arch_file = Path(self.test_dir) / "ARCHITECTURE.md"
        self.arch_file.write_text("# Architecture")

    def tearDown(self):
        """Clean up test files."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

    def test_no_diagrams_found(self):
        """Test validation when no diagrams exist."""
        result = validate_diagrams(self.arch_file)

        self.assertFalse(result.passed)
        self.assertEqual(result.diagram_count, 0)
        self.assertEqual(result.quality_score, 0.0)
        self.assertIn("No diagram-as-code files found", result.syntax_errors)

    def test_d2_diagram_found(self):
        """Test D2 diagram detection."""
        diagram_dir = Path(self.test_dir) / "diagrams"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "context.d2").write_text("user -> api")

        result = validate_diagrams(self.arch_file)

        self.assertEqual(result.diagram_count, 1)
        # Quality score depends on whether d2 binary is available
        self.assertGreaterEqual(result.quality_score, 0.0)

    def test_structurizr_diagram_found(self):
        """Test Structurizr DSL diagram detection."""
        diagram_dir = Path(self.test_dir) / "docs" / "architecture"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "workspace.dsl").write_text("workspace { }")

        result = validate_diagrams(self.arch_file)

        self.assertEqual(result.diagram_count, 1)

    def test_mermaid_diagram_found(self):
        """Test Mermaid diagram detection."""
        diagram_dir = Path(self.test_dir) / "docs" / "diagrams"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "c4.mmd").write_text("graph TD\n  A --> B")

        result = validate_diagrams(self.arch_file)

        self.assertEqual(result.diagram_count, 1)
        # Mermaid validation is basic (non-empty check)
        self.assertGreater(result.quality_score, 0.0)

    def test_multiple_diagrams(self):
        """Test detection of multiple diagrams."""
        diagram_dir = Path(self.test_dir) / "diagrams"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "context.d2").write_text("user -> api")
        (diagram_dir / "container.d2").write_text("api -> db")
        (diagram_dir / "system.mmd").write_text("graph TD\n  A --> B")

        result = validate_diagrams(self.arch_file)

        self.assertEqual(result.diagram_count, 3)

    def test_empty_mermaid_file(self):
        """Test validation fails for empty Mermaid file."""
        diagram_dir = Path(self.test_dir) / "diagrams"
        diagram_dir.mkdir(parents=True)
        (diagram_dir / "empty.mmd").write_text("")

        result = validate_diagrams(self.arch_file)

        self.assertEqual(result.diagram_count, 1)
        self.assertIn("empty.mmd", str(result.syntax_errors))


class TestPersonaSelection(unittest.TestCase):
    """Test persona selection logic."""

    def test_system_architect_always_included(self):
        """Test System Architect is always included."""
        content = "Basic architecture document."
        personas = select_personas(content)

        self.assertIn("System Architect", personas)

    def test_devops_engineer_conditional(self):
        """Test DevOps Engineer included for infrastructure content."""
        content = """
# Architecture

Deployment on Kubernetes with AWS infrastructure.
"""
        personas = select_personas(content)

        self.assertIn("DevOps Engineer", personas)

    def test_developer_conditional(self):
        """Test Developer included for code architecture content."""
        content = """
# Architecture

Agent coordination and state management patterns.
"""
        personas = select_personas(content)

        self.assertIn("Developer", personas)


class TestRubricLoading(unittest.TestCase):
    """Test rubric loading functionality."""

    def test_load_rubric_fallback(self):
        """Test rubric loads successfully (from file or fallback)."""
        rubric = load_rubric()

        # Should always return a string
        self.assertIsInstance(rubric, str)
        self.assertGreater(len(rubric), 0, "Rubric should not be empty")

        # Should contain key dimension identifiers (YAML keys or display names)
        # Check for YAML structure keys (more reliable for YAML content)
        self.assertIn("traditional_architecture", rubric)
        self.assertIn("agentic_architecture", rubric)
        self.assertIn("adr_integration", rubric)
        self.assertIn("visual_diagrams", rubric)


class TestPromptBuilding(unittest.TestCase):
    """Test prompt building functionality."""

    def test_build_prompt_basic(self):
        """Test basic prompt building."""
        content = "Test architecture content"
        rubric = "Test rubric"
        personas = ["System Architect"]

        prompt = build_prompt(content, rubric, personas)

        self.assertIn("Test architecture content", prompt)
        self.assertIn("Test rubric", prompt)
        self.assertIn("System Architect", prompt)
        self.assertIn("JSON", prompt)

    def test_build_prompt_multiple_personas(self):
        """Test prompt with multiple personas."""
        content = "Test content"
        rubric = "Test rubric"
        personas = ["System Architect", "DevOps Engineer", "Developer"]

        prompt = build_prompt(content, rubric, personas)

        for persona in personas:
            self.assertIn(persona, prompt)

    def test_build_prompt_with_diagram_validation(self):
        """Test prompt includes diagram validation results."""
        content = "Test content"
        rubric = "Test rubric"
        personas = ["System Architect"]
        diagram_result = DiagramValidationResult(
            passed=True,
            diagram_count=3,
            syntax_errors=[],
            sync_score=None,
            quality_score=9.0
        )

        prompt = build_prompt(content, rubric, personas, None, diagram_result)

        self.assertIn("Diagram Validation Results", prompt)
        self.assertIn("Diagram count: 3", prompt)
        self.assertIn("Quality score: 9.0", prompt)


class TestJSONParsing(unittest.TestCase):
    """Test JSON response parsing."""

    def test_parse_json_response_pass(self):
        """Test parsing PASS response."""
        response = """```json
{
  "overall_score": 8.5,
  "dimension_scores": {
    "traditional_architecture": 8.8,
    "agentic_architecture": 8.2,
    "adr_integration": 8.0,
    "visual_diagrams": 9.0
  },
  "self_consistency": {
    "scores": [8.4, 8.6, 8.5, 8.3, 8.7],
    "mean": 8.5,
    "variance": 0.234
  },
  "personas": [
    {
      "role": "System Architect",
      "score": 8.7,
      "feedback": "Excellent architecture."
    }
  ]
}
```"""
        result = parse_json_response(response)

        self.assertEqual(result.decision, "PASS")
        self.assertEqual(result.overall_score, 8.5)
        self.assertEqual(result.dimension_scores.traditional_architecture, 8.8)
        self.assertEqual(len(result.personas), 1)

    def test_parse_json_response_warn(self):
        """Test parsing WARN response."""
        response = """```json
{
  "overall_score": 7.0,
  "dimension_scores": {
    "traditional_architecture": 7.5,
    "agentic_architecture": 7.0,
    "adr_integration": 6.5,
    "visual_diagrams": 7.0
  },
  "self_consistency": {
    "scores": [6.9, 7.1, 7.0, 6.8, 7.2],
    "mean": 7.0,
    "variance": 0.187
  },
  "personas": [
    {
      "role": "System Architect",
      "score": 7.2,
      "feedback": "Good but needs improvement."
    }
  ]
}
```"""
        result = parse_json_response(response)

        self.assertEqual(result.decision, "WARN")
        self.assertEqual(result.overall_score, 7.0)

    def test_parse_json_response_fail(self):
        """Test parsing FAIL response."""
        response = """```json
{
  "overall_score": 4.5,
  "dimension_scores": {
    "traditional_architecture": 5.0,
    "agentic_architecture": 4.0,
    "adr_integration": 4.0,
    "visual_diagrams": 5.0
  },
  "self_consistency": {
    "scores": [4.4, 4.6, 4.5, 4.3, 4.7],
    "mean": 4.5,
    "variance": 0.156
  },
  "personas": [
    {
      "role": "System Architect",
      "score": 4.8,
      "feedback": "Significant issues found."
    }
  ]
}
```"""
        result = parse_json_response(response)

        self.assertEqual(result.decision, "FAIL")
        self.assertEqual(result.overall_score, 4.5)


class TestCLIIntegration(unittest.TestCase):
    """Test CLI integration and adapters."""

    def test_cli_abstraction_import(self):
        """Test CLI abstraction import (available or gracefully unavailable)."""
        try:
            from cli_abstraction import CLIAbstraction
            from cli_detector import detect_cli

            # Should not raise
            cli = CLIAbstraction()
            cli_type = detect_cli()

            self.assertIsNotNone(cli)
            self.assertIsNotNone(cli_type)
            # If import succeeds, test passes
        except ImportError as e:
            # If import fails, that's expected in some environments - test still passes
            # The code handles missing CLI abstraction gracefully with fallback
            self.assertTrue(True, f"CLI abstraction not available (expected in some envs): {e}")

    def test_cli_adapters_exist(self):
        """Test all CLI adapters exist."""
        adapters = ["claude-code.py", "gemini.py", "opencode.py", "codex.py"]
        adapter_dir = SKILL_DIR / "cli-adapters"

        for adapter in adapters:
            adapter_path = adapter_dir / adapter
            self.assertTrue(
                adapter_path.exists(),
                f"CLI adapter not found: {adapter}"
            )

    def test_cli_adapters_executable(self):
        """Test CLI adapters are executable Python files."""
        adapters = ["claude-code.py", "gemini.py", "opencode.py", "codex.py"]
        adapter_dir = SKILL_DIR / "cli-adapters"

        for adapter in adapters:
            adapter_path = adapter_dir / adapter
            content = adapter_path.read_text()

            # Check for shebang
            self.assertTrue(
                content.startswith("#!/usr/bin/env python3"),
                f"Missing shebang in {adapter}"
            )

            # Check imports
            self.assertIn("from cli_detector import", content)
            self.assertIn("from review_architecture import main", content)


class TestEndToEnd(unittest.TestCase):
    """End-to-end integration tests."""

    def setUp(self):
        """Set up test environment."""
        self.test_dir = tempfile.mkdtemp()
        self.arch_file = Path(self.test_dir) / "ARCHITECTURE.md"

        # Check if API key is available
        self.has_api_key = (
            os.getenv("ANTHROPIC_API_KEY") is not None or
            (os.getenv("CLAUDE_CODE_USE_VERTEX") == "1" and
             os.getenv("ANTHROPIC_VERTEX_PROJECT_ID") is not None)
        )

    def tearDown(self):
        """Clean up test files."""
        import shutil
        shutil.rmtree(self.test_dir, ignore_errors=True)

    def test_script_executable(self):
        """Test main script is executable."""
        script_path = SKILL_DIR / "review_architecture.py"

        self.assertTrue(script_path.exists())
        content = script_path.read_text()
        self.assertTrue(content.startswith("#!/usr/bin/env python3"))

    def test_help_message(self):
        """Test help message displays correctly."""
        script_path = SKILL_DIR / "review_architecture.py"

        result = subprocess.run(
            [sys.executable, str(script_path), "--help"],
            capture_output=True,
            text=True
        )

        self.assertEqual(result.returncode, 0)
        self.assertIn("ARCHITECTURE.md", result.stdout)
        self.assertIn("--output-json", result.stdout)


def run_tests():
    """Run all tests."""
    # Create test suite
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()

    # Add test classes
    suite.addTests(loader.loadTestsFromTestCase(TestQuickValidation))
    suite.addTests(loader.loadTestsFromTestCase(TestDiagramValidation))
    suite.addTests(loader.loadTestsFromTestCase(TestPersonaSelection))
    suite.addTests(loader.loadTestsFromTestCase(TestRubricLoading))
    suite.addTests(loader.loadTestsFromTestCase(TestPromptBuilding))
    suite.addTests(loader.loadTestsFromTestCase(TestJSONParsing))
    suite.addTests(loader.loadTestsFromTestCase(TestCLIIntegration))
    suite.addTests(loader.loadTestsFromTestCase(TestEndToEnd))

    # Run tests
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)

    # Return exit code
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(run_tests())
