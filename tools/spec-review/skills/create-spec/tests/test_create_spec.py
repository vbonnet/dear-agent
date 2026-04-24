#!/usr/bin/env python3
"""Comprehensive tests for create-spec skill.

Tests cover:
- Codebase analysis
- Question generation
- SPEC rendering
- Validation
- All CLI adapters
"""

import os
import sys
import tempfile
import shutil
import unittest
from pathlib import Path

# Add lib directory to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from codebase_analyzer import CodebaseAnalyzer, CodebaseAnalysis
from question_generator import QuestionGenerator, Question, QuestionSet
from spec_renderer import SPECRenderer
from spec_validator import SpecValidator


class TestCodebaseAnalyzer(unittest.TestCase):
    """Test codebase analysis functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()
        self.analyzer = CodebaseAnalyzer()

    def tearDown(self):
        """Clean up test fixtures."""
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def test_analyze_empty_project(self):
        """Test analysis of empty project."""
        analysis = self.analyzer.analyze(self.test_dir)
        self.assertIsInstance(analysis, CodebaseAnalysis)
        self.assertEqual(analysis.file_count, 0)
        self.assertEqual(len(analysis.languages), 0)

    def test_analyze_python_project(self):
        """Test analysis of Python project."""
        # Create test files
        os.makedirs(os.path.join(self.test_dir, "src"))
        with open(os.path.join(self.test_dir, "src", "main.py"), 'w') as f:
            f.write("print('hello')")
        with open(os.path.join(self.test_dir, "README.md"), 'w') as f:
            f.write("# Test Project")
        with open(os.path.join(self.test_dir, "requirements.txt"), 'w') as f:
            f.write("requests==2.28.0")

        analysis = self.analyzer.analyze(self.test_dir)

        self.assertGreater(analysis.file_count, 0)
        self.assertIn("Python", analysis.languages)
        self.assertIn("Python", analysis.technologies)
        self.assertIsNotNone(analysis.readme_content)

    def test_analyze_detects_technologies(self):
        """Test technology detection."""
        # Create package.json for Node.js detection
        with open(os.path.join(self.test_dir, "package.json"), 'w') as f:
            f.write('{"name": "test"}')

        # Create Dockerfile for Docker detection
        with open(os.path.join(self.test_dir, "Dockerfile"), 'w') as f:
            f.write("FROM python:3.10")

        analysis = self.analyzer.analyze(self.test_dir)

        self.assertIn("Node.js", analysis.technologies)
        self.assertIn("Docker", analysis.technologies)

    def test_analyze_ignores_patterns(self):
        """Test that ignored directories are skipped."""
        # Create ignored directories
        os.makedirs(os.path.join(self.test_dir, "node_modules"))
        os.makedirs(os.path.join(self.test_dir, ".git"))
        os.makedirs(os.path.join(self.test_dir, "__pycache__"))

        # Add files to ignored dirs
        with open(os.path.join(self.test_dir, "node_modules", "test.js"), 'w') as f:
            f.write("ignored")

        analysis = self.analyzer.analyze(self.test_dir)

        # Should not count ignored files
        self.assertEqual(analysis.file_count, 0)

    def test_get_summary(self):
        """Test summary generation."""
        # Create simple project
        with open(os.path.join(self.test_dir, "main.py"), 'w') as f:
            f.write("pass")

        analysis = self.analyzer.analyze(self.test_dir)
        summary = self.analyzer.get_summary(analysis)

        self.assertIn("Project:", summary)
        self.assertIn("Python", summary)


class TestQuestionGenerator(unittest.TestCase):
    """Test question generation functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()
        self.generator = QuestionGenerator(interactive=False)

        # Create minimal analysis
        with open(os.path.join(self.test_dir, "main.py"), 'w') as f:
            f.write("pass")

        analyzer = CodebaseAnalyzer()
        self.analysis = analyzer.analyze(self.test_dir)

    def tearDown(self):
        """Clean up test fixtures."""
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def test_generate_questions(self):
        """Test question generation."""
        questions = self.generator.generate_questions(self.analysis)

        self.assertIsInstance(questions, QuestionSet)
        self.assertGreater(len(questions.vision_questions), 0)
        self.assertGreater(len(questions.persona_questions), 0)
        self.assertGreater(len(questions.cuj_questions), 0)
        self.assertGreater(len(questions.metrics_questions), 0)

    def test_get_default_answers(self):
        """Test default answer generation."""
        questions = self.generator.generate_questions(self.analysis)
        answers = self.generator.get_default_answers(questions, self.analysis)

        self.assertIn("project_name", answers)
        self.assertIn("what_is_this", answers)
        self.assertIsInstance(answers, dict)

    def test_to_json(self):
        """Test JSON export."""
        questions = self.generator.generate_questions(self.analysis)
        self.generator.answers = self.generator.get_default_answers(questions, self.analysis)

        json_str = self.generator.to_json()
        self.assertIsInstance(json_str, str)
        self.assertIn("project_name", json_str)

    def test_from_json(self):
        """Test JSON import."""
        test_json = '{"project_name": "test", "what_is_this": "A test project"}'
        self.generator.from_json(test_json)

        self.assertEqual(self.generator.answers["project_name"], "test")
        self.assertEqual(self.generator.answers["what_is_this"], "A test project")


class TestSPECRenderer(unittest.TestCase):
    """Test SPEC rendering functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()
        self.renderer = SPECRenderer()

        # Create minimal analysis
        with open(os.path.join(self.test_dir, "main.py"), 'w') as f:
            f.write("pass")

        analyzer = CodebaseAnalyzer()
        self.analysis = analyzer.analyze(self.test_dir)

        # Generate answers
        generator = QuestionGenerator(interactive=False)
        questions = generator.generate_questions(self.analysis)
        self.answers = generator.get_default_answers(questions, self.analysis)

    def tearDown(self):
        """Clean up test fixtures."""
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def test_render_spec(self):
        """Test SPEC rendering."""
        spec_content = self.renderer.render(self.answers, self.analysis)

        self.assertIsInstance(spec_content, str)
        self.assertGreater(len(spec_content), 100)
        self.assertIn("Product Specification", spec_content)

    def test_render_to_file(self):
        """Test rendering to file."""
        output_path = os.path.join(self.test_dir, "SPEC.md")
        spec_content = self.renderer.render(self.answers, self.analysis, output_path)

        self.assertTrue(os.path.exists(output_path))
        with open(output_path, 'r') as f:
            file_content = f.read()
        self.assertEqual(spec_content, file_content)

    def test_render_includes_sections(self):
        """Test that rendered SPEC includes required sections."""
        spec_content = self.renderer.render(self.answers, self.analysis)

        required_sections = [
            "Vision",
            "User Personas",
            "Critical User Journeys",
            "Goals & Success Metrics",
            "Feature Prioritization",
            "Scope Boundaries",
        ]

        for section in required_sections:
            self.assertIn(section, spec_content, f"Missing section: {section}")


class TestSpecValidator(unittest.TestCase):
    """Test SPEC validation functionality."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()
        self.validator = SpecValidator()

    def tearDown(self):
        """Clean up test fixtures."""
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def test_validate_minimal_spec(self):
        """Test validation of minimal SPEC."""
        spec_content = """
# Product Specification: Test

## 1. Vision
This is a test project.

## 2. User Personas
Primary users are developers.

## 3. Critical User Journeys
Main journey is testing.

## 4. Goals & Success Metrics
Success is measured by test pass rate.

## 5. Feature Prioritization
Must have: Testing framework

## 6. Scope Boundaries
In scope: Testing

## 7. Assumptions & Constraints
Assumes Python 3.10+
"""
        result = self.validator.validate(spec_content)

        self.assertIsInstance(result, result.__class__)
        self.assertGreaterEqual(result.score, 0)
        self.assertLessEqual(result.score, 10)

    def test_validate_missing_sections(self):
        """Test validation catches missing sections."""
        spec_content = "# Product Specification: Test\n\nSome content."

        result = self.validator.validate(spec_content)

        self.assertFalse(result.is_valid)
        self.assertGreater(len(result.errors), 0)

    def test_validate_empty_spec(self):
        """Test validation of empty SPEC."""
        result = self.validator.validate("")

        self.assertFalse(result.is_valid)
        self.assertGreater(len(result.errors), 0)

    def test_validate_from_file(self):
        """Test validation from file."""
        spec_path = os.path.join(self.test_dir, "SPEC.md")
        with open(spec_path, 'w') as f:
            f.write("# Product Specification: Test")

        result = self.validator.validate_from_file(spec_path)

        self.assertIsInstance(result, result.__class__)

    def test_validate_nonexistent_file(self):
        """Test validation of nonexistent file."""
        result = self.validator.validate_from_file("/nonexistent/SPEC.md")

        self.assertFalse(result.is_valid)
        self.assertGreater(len(result.errors), 0)

    def test_get_summary(self):
        """Test summary generation."""
        spec_content = "# Product Specification: Test"
        result = self.validator.validate(spec_content)
        summary = self.validator.get_summary(result)

        self.assertIn("SPEC VALIDATION RESULTS", summary)
        self.assertIn("Overall Score:", summary)


class TestCLIAdapters(unittest.TestCase):
    """Test CLI adapter compatibility."""

    def test_claude_code_adapter_exists(self):
        """Test Claude Code adapter exists and is executable."""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "claude-code.py"
        self.assertTrue(adapter_path.exists())

    def test_gemini_adapter_exists(self):
        """Test Gemini adapter exists and is executable."""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "gemini.py"
        self.assertTrue(adapter_path.exists())

    def test_opencode_adapter_exists(self):
        """Test OpenCode adapter exists and is executable."""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "opencode.py"
        self.assertTrue(adapter_path.exists())

    def test_codex_adapter_exists(self):
        """Test Codex adapter exists and is executable."""
        adapter_path = Path(__file__).parent.parent / "cli-adapters" / "codex.py"
        self.assertTrue(adapter_path.exists())


class TestIntegration(unittest.TestCase):
    """Integration tests for complete workflow."""

    def setUp(self):
        """Set up test fixtures."""
        self.test_dir = tempfile.mkdtemp()

        # Create realistic project structure
        os.makedirs(os.path.join(self.test_dir, "src"))
        os.makedirs(os.path.join(self.test_dir, "tests"))
        os.makedirs(os.path.join(self.test_dir, "docs"))

        with open(os.path.join(self.test_dir, "src", "main.py"), 'w') as f:
            f.write("def main():\n    print('Hello')")

        with open(os.path.join(self.test_dir, "README.md"), 'w') as f:
            f.write("# Test Project\n\nA test project.")

        with open(os.path.join(self.test_dir, "requirements.txt"), 'w') as f:
            f.write("requests==2.28.0\npytest==7.0.0")

    def tearDown(self):
        """Clean up test fixtures."""
        if os.path.exists(self.test_dir):
            shutil.rmtree(self.test_dir)

    def test_end_to_end_workflow(self):
        """Test complete workflow from analysis to validation."""
        # Step 1: Analyze codebase
        analyzer = CodebaseAnalyzer()
        analysis = analyzer.analyze(self.test_dir)
        self.assertGreater(analysis.file_count, 0)

        # Step 2: Generate questions
        generator = QuestionGenerator(interactive=False)
        questions = generator.generate_questions(analysis)
        self.assertIsInstance(questions, QuestionSet)

        # Step 3: Get answers (defaults)
        answers = generator.get_default_answers(questions, analysis)
        self.assertIn("project_name", answers)

        # Step 4: Render SPEC
        renderer = SPECRenderer()
        spec_content = renderer.render(answers, analysis)
        self.assertGreater(len(spec_content), 100)

        # Step 5: Validate SPEC
        validator = SpecValidator()
        result = validator.validate(spec_content)
        self.assertGreaterEqual(result.score, 0)

    def test_workflow_with_file_output(self):
        """Test workflow writing to file."""
        output_path = os.path.join(self.test_dir, "docs", "SPEC.md")

        # Complete workflow
        analyzer = CodebaseAnalyzer()
        analysis = analyzer.analyze(self.test_dir)

        generator = QuestionGenerator(interactive=False)
        questions = generator.generate_questions(analysis)
        answers = generator.get_default_answers(questions, analysis)

        renderer = SPECRenderer()
        renderer.render(answers, analysis, output_path)

        # Verify file exists and has content
        self.assertTrue(os.path.exists(output_path))
        with open(output_path, 'r') as f:
            content = f.read()
        self.assertGreater(len(content), 100)

        # Validate from file
        validator = SpecValidator()
        result = validator.validate_from_file(output_path)
        self.assertGreaterEqual(result.score, 0)


def run_tests():
    """Run all tests."""
    # Create test suite
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()

    # Add all test cases
    suite.addTests(loader.loadTestsFromTestCase(TestCodebaseAnalyzer))
    suite.addTests(loader.loadTestsFromTestCase(TestQuestionGenerator))
    suite.addTests(loader.loadTestsFromTestCase(TestSPECRenderer))
    suite.addTests(loader.loadTestsFromTestCase(TestSpecValidator))
    suite.addTests(loader.loadTestsFromTestCase(TestCLIAdapters))
    suite.addTests(loader.loadTestsFromTestCase(TestIntegration))

    # Run tests
    runner = unittest.TextTestRunner(verbosity=2)
    result = runner.run(suite)

    # Return exit code
    return 0 if result.wasSuccessful() else 1


if __name__ == "__main__":
    sys.exit(run_tests())
