#!/usr/bin/env python3
"""Codebase Analyzer - Extract structure and context from existing codebase."""

import os
import re
from pathlib import Path
from typing import Dict, List, Optional, Set
from dataclasses import dataclass, field


@dataclass
class FileInfo:
    """Information about a file in the codebase."""
    path: str
    relative_path: str
    extension: str
    size: int
    language: str


@dataclass
class CodebaseAnalysis:
    """Analysis results for a codebase."""
    project_path: str
    project_name: str
    primary_language: str
    languages: Dict[str, int] = field(default_factory=dict)
    file_count: int = 0
    directory_structure: List[str] = field(default_factory=list)
    key_files: List[FileInfo] = field(default_factory=list)
    technologies: Set[str] = field(default_factory=set)
    patterns: Set[str] = field(default_factory=set)
    dependencies: Dict[str, List[str]] = field(default_factory=dict)
    readme_content: Optional[str] = None
    existing_docs: List[str] = field(default_factory=list)


class CodebaseAnalyzer:
    """Analyze codebase structure and extract key information."""

    # File extensions to language mapping
    LANGUAGE_MAP = {
        '.py': 'Python',
        '.js': 'JavaScript',
        '.ts': 'TypeScript',
        '.go': 'Go',
        '.rs': 'Rust',
        '.java': 'Java',
        '.rb': 'Ruby',
        '.php': 'PHP',
        '.cpp': 'C++',
        '.c': 'C',
        '.h': 'C/C++',
        '.hpp': 'C++',
        '.cs': 'C#',
        '.swift': 'Swift',
        '.kt': 'Kotlin',
        '.sh': 'Shell',
        '.bash': 'Bash',
        '.yml': 'YAML',
        '.yaml': 'YAML',
        '.json': 'JSON',
        '.md': 'Markdown',
    }

    # Technology detection patterns (file-based)
    TECHNOLOGY_PATTERNS = {
        'package.json': 'Node.js',
        'requirements.txt': 'Python',
        'Pipfile': 'Python/Pipenv',
        'poetry.lock': 'Python/Poetry',
        'go.mod': 'Go',
        'Cargo.toml': 'Rust',
        'pom.xml': 'Java/Maven',
        'build.gradle': 'Java/Gradle',
        'Gemfile': 'Ruby',
        'composer.json': 'PHP/Composer',
        'Dockerfile': 'Docker',
        'docker-compose.yml': 'Docker Compose',
        '.github/workflows': 'GitHub Actions',
        'Makefile': 'Make',
        'CMakeLists.txt': 'CMake',
    }

    # Files to ignore during analysis
    IGNORE_PATTERNS = {
        '__pycache__', '.git', '.svn', 'node_modules', 'vendor',
        '.venv', 'venv', 'env', 'build', 'dist', '.pytest_cache',
        '.mypy_cache', '.tox', 'coverage', '.coverage', '.idea',
        '.vscode', '*.pyc', '*.pyo', '*.pyd', '.DS_Store',
    }

    def __init__(self, max_depth: int = 5, max_files: int = 1000):
        """Initialize analyzer.

        Args:
            max_depth: Maximum directory depth to analyze
            max_files: Maximum number of files to analyze
        """
        self.max_depth = max_depth
        self.max_files = max_files

    def analyze(self, project_path: str) -> CodebaseAnalysis:
        """Analyze codebase at the given path.

        Args:
            project_path: Path to project root

        Returns:
            CodebaseAnalysis object with extracted information
        """
        project_path = os.path.abspath(project_path)
        if not os.path.exists(project_path):
            raise ValueError(f"Project path does not exist: {project_path}")

        analysis = CodebaseAnalysis(
            project_path=project_path,
            project_name=os.path.basename(project_path),
            primary_language="Unknown",  # Will be determined during analysis
        )

        # Analyze directory structure
        self._analyze_structure(project_path, analysis)

        # Detect technologies
        self._detect_technologies(project_path, analysis)

        # Identify key files
        self._identify_key_files(project_path, analysis)

        # Read README if exists
        self._read_readme(project_path, analysis)

        # Find existing documentation
        self._find_existing_docs(project_path, analysis)

        # Determine primary language
        if analysis.languages:
            analysis.primary_language = max(
                analysis.languages.items(),
                key=lambda x: x[1]
            )[0]

        return analysis

    def _should_ignore(self, path: str) -> bool:
        """Check if path should be ignored.

        Args:
            path: Path to check

        Returns:
            True if should ignore, False otherwise
        """
        path_str = str(path)
        for pattern in self.IGNORE_PATTERNS:
            if pattern.startswith('*'):
                if path_str.endswith(pattern[1:]):
                    return True
            elif pattern in path_str:
                return True
        return False

    def _analyze_structure(self, project_path: str, analysis: CodebaseAnalysis):
        """Analyze directory structure and file types.

        Args:
            project_path: Project root path
            analysis: Analysis object to populate
        """
        for root, dirs, files in os.walk(project_path):
            # Filter out ignored directories
            dirs[:] = [d for d in dirs if not self._should_ignore(d)]

            # Check depth
            depth = root[len(project_path):].count(os.sep)
            if depth > self.max_depth:
                continue

            # Add to directory structure
            rel_path = os.path.relpath(root, project_path)
            if rel_path != '.':
                analysis.directory_structure.append(rel_path)

            # Analyze files
            for file in files:
                if self._should_ignore(file):
                    continue

                if analysis.file_count >= self.max_files:
                    return

                file_path = os.path.join(root, file)
                ext = os.path.splitext(file)[1].lower()

                # Count by language
                if ext in self.LANGUAGE_MAP:
                    lang = self.LANGUAGE_MAP[ext]
                    analysis.languages[lang] = analysis.languages.get(lang, 0) + 1

                analysis.file_count += 1

    def _detect_technologies(self, project_path: str, analysis: CodebaseAnalysis):
        """Detect technologies used in the project.

        Args:
            project_path: Project root path
            analysis: Analysis object to populate
        """
        for pattern, tech in self.TECHNOLOGY_PATTERNS.items():
            check_path = os.path.join(project_path, pattern)
            if os.path.exists(check_path):
                analysis.technologies.add(tech)

        # Detect architectural patterns
        if os.path.exists(os.path.join(project_path, 'tests')) or \
           os.path.exists(os.path.join(project_path, 'test')):
            analysis.patterns.add('Testing')

        if os.path.exists(os.path.join(project_path, 'docs')):
            analysis.patterns.add('Documentation')

        if os.path.exists(os.path.join(project_path, '.github')):
            analysis.patterns.add('CI/CD')

    def _identify_key_files(self, project_path: str, analysis: CodebaseAnalysis):
        """Identify key files in the codebase.

        Args:
            project_path: Project root path
            analysis: Analysis object to populate
        """
        key_file_names = {
            'README.md', 'README.rst', 'README',
            'SPEC.md', 'ARCHITECTURE.md', 'DESIGN.md',
            'main.py', 'app.py', 'server.py', '__init__.py',
            'main.go', 'main.rs', 'index.js', 'index.ts',
            'package.json', 'go.mod', 'Cargo.toml',
        }

        for root, _, files in os.walk(project_path):
            depth = root[len(project_path):].count(os.sep)
            if depth > 2:  # Only check top 2 levels for key files
                continue

            for file in files:
                if file in key_file_names:
                    file_path = os.path.join(root, file)
                    rel_path = os.path.relpath(file_path, project_path)
                    ext = os.path.splitext(file)[1].lower()
                    size = os.path.getsize(file_path)

                    file_info = FileInfo(
                        path=file_path,
                        relative_path=rel_path,
                        extension=ext,
                        size=size,
                        language=self.LANGUAGE_MAP.get(ext, 'Unknown')
                    )
                    analysis.key_files.append(file_info)

    def _read_readme(self, project_path: str, analysis: CodebaseAnalysis):
        """Read README file if it exists.

        Args:
            project_path: Project root path
            analysis: Analysis object to populate
        """
        readme_names = ['README.md', 'README.rst', 'README.txt', 'README']
        for name in readme_names:
            readme_path = os.path.join(project_path, name)
            if os.path.exists(readme_path):
                try:
                    with open(readme_path, 'r', encoding='utf-8') as f:
                        analysis.readme_content = f.read()
                    break
                except Exception:
                    pass

    def _find_existing_docs(self, project_path: str, analysis: CodebaseAnalysis):
        """Find existing documentation files.

        Args:
            project_path: Project root path
            analysis: Analysis object to populate
        """
        doc_patterns = ['*.md', '*.rst', '*.txt']
        doc_dirs = ['docs', 'doc', 'documentation']

        for doc_dir in doc_dirs:
            doc_path = os.path.join(project_path, doc_dir)
            if os.path.exists(doc_path):
                for root, _, files in os.walk(doc_path):
                    for file in files:
                        if file.endswith(('.md', '.rst', '.txt')):
                            rel_path = os.path.relpath(
                                os.path.join(root, file),
                                project_path
                            )
                            analysis.existing_docs.append(rel_path)

    def get_summary(self, analysis: CodebaseAnalysis) -> str:
        """Get a human-readable summary of the analysis.

        Args:
            analysis: CodebaseAnalysis object

        Returns:
            Summary string
        """
        lines = [
            f"Project: {analysis.project_name}",
            f"Primary Language: {analysis.primary_language}",
            f"Total Files: {analysis.file_count}",
            "",
            "Languages:",
        ]

        for lang, count in sorted(
            analysis.languages.items(),
            key=lambda x: x[1],
            reverse=True
        ):
            lines.append(f"  - {lang}: {count} files")

        if analysis.technologies:
            lines.append("")
            lines.append("Technologies:")
            for tech in sorted(analysis.technologies):
                lines.append(f"  - {tech}")

        if analysis.patterns:
            lines.append("")
            lines.append("Patterns:")
            for pattern in sorted(analysis.patterns):
                lines.append(f"  - {pattern}")

        if analysis.key_files:
            lines.append("")
            lines.append("Key Files:")
            for file_info in analysis.key_files[:10]:
                lines.append(f"  - {file_info.relative_path}")

        return "\n".join(lines)


if __name__ == "__main__":
    import sys

    if len(sys.argv) < 2:
        print("Usage: python codebase_analyzer.py <project_path>")
        sys.exit(1)

    analyzer = CodebaseAnalyzer()
    analysis = analyzer.analyze(sys.argv[1])
    print(analyzer.get_summary(analysis))
