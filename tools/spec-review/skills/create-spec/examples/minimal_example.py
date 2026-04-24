#!/usr/bin/env python3
"""Minimal example demonstrating create-spec skill usage."""

import os
import sys
import tempfile
import shutil
from pathlib import Path

# Add lib directory to path
sys.path.insert(0, str(Path(__file__).parent.parent / "lib"))

from lib.codebase_analyzer import CodebaseAnalyzer
from lib.question_generator import QuestionGenerator
from lib.spec_renderer import SPECRenderer
from lib.spec_validator import SpecValidator


def create_example_project():
    """Create a minimal example project."""
    project_dir = tempfile.mkdtemp(prefix="example_project_")

    # Create basic structure
    os.makedirs(os.path.join(project_dir, "src"))
    os.makedirs(os.path.join(project_dir, "tests"))

    # Create main.py
    with open(os.path.join(project_dir, "src", "main.py"), 'w') as f:
        f.write("""#!/usr/bin/env python3
\"\"\"Example application.\"\"\"

def greet(name: str) -> str:
    \"\"\"Greet a user by name.\"\"\"
    return f"Hello, {name}!"

if __name__ == "__main__":
    print(greet("World"))
""")

    # Create README
    with open(os.path.join(project_dir, "README.md"), 'w') as f:
        f.write("""# Example Project

A simple greeting application demonstrating create-spec skill.

## Features

- Greets users by name
- Simple Python implementation
""")

    # Create requirements
    with open(os.path.join(project_dir, "requirements.txt"), 'w') as f:
        f.write("pytest>=7.0.0\n")

    return project_dir


def main():
    """Run minimal example."""
    print("="*60)
    print("CREATE-SPEC: Minimal Example")
    print("="*60)
    print()

    # Step 1: Create example project
    print("Step 1: Creating example project...")
    project_dir = create_example_project()
    print(f"  ✓ Created at: {project_dir}")
    print()

    # Step 2: Analyze codebase
    print("Step 2: Analyzing codebase...")
    analyzer = CodebaseAnalyzer()
    analysis = analyzer.analyze(project_dir)
    print(f"  ✓ Files: {analysis.file_count}")
    print(f"  ✓ Language: {analysis.primary_language}")
    print(f"  ✓ Technologies: {', '.join(analysis.technologies) if analysis.technologies else 'None'}")
    print()

    # Step 3: Generate questions and use defaults
    print("Step 3: Generating questions...")
    generator = QuestionGenerator(interactive=False)
    questions = generator.generate_questions(analysis)
    print(f"  ✓ Generated questions")
    print()

    print("Step 4: Getting default answers...")
    answers = generator.get_default_answers(questions, analysis)
    print(f"  ✓ Answers prepared")
    print()

    # Step 5: Render SPEC
    print("Step 5: Rendering SPEC.md...")
    renderer = SPECRenderer()
    spec_content = renderer.render(answers, analysis)
    print(f"  ✓ Generated {len(spec_content)} bytes")
    print()

    # Step 6: Validate
    print("Step 6: Validating SPEC...")
    validator = SpecValidator()
    result = validator.validate(spec_content)
    print(f"  Score: {result.score:.1f}/10.0")
    print(f"  Valid: {result.is_valid}")
    if result.warnings:
        print(f"  Warnings: {len(result.warnings)}")
    print()

    # Step 7: Save to file
    spec_path = os.path.join(project_dir, "SPEC.md")
    with open(spec_path, 'w') as f:
        f.write(spec_content)
    print(f"Step 7: Saved SPEC to {spec_path}")
    print()

    # Show first few lines
    print("Preview of generated SPEC.md:")
    print("-" * 60)
    lines = spec_content.split('\n')[:20]
    for line in lines:
        print(line)
    print("...")
    print("-" * 60)
    print()

    print("="*60)
    print("✓ Example completed successfully!")
    print("="*60)
    print()
    print(f"Generated files in: {project_dir}")
    print(f"  - src/main.py")
    print(f"  - README.md")
    print(f"  - requirements.txt")
    print(f"  - SPEC.md (generated)")
    print()
    print("To clean up: rm -rf {}".format(project_dir))


if __name__ == "__main__":
    main()
