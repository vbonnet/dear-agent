# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- OSS infrastructure setup (CONTRIBUTING.md, GitHub templates)
- Comprehensive examples for basic usage, CI/CD, custom personas, and programmatic API
- Documentation improvements

## [0.1.0] - 2025-11-25

### Added
- Initial release of multi-persona code review plugin
- Support for Anthropic Claude API
- Support for Google Vertex AI (Gemini and Claude models)
- Multiple output formats (text, JSON, GitHub PR comments)
- Persona-based review system with 8 core personas
- Cost tracking with multiple sink options
- Deduplication of findings across personas
- Comprehensive test suite (279 tests)
- CLI interface with flexible configuration
- Parallel persona execution
- Custom persona support via YAML files

### Security
- API key management via environment variables
- Secure credential handling for Google Cloud

[Unreleased]: https://github.com/wayfinder/multi-persona-review/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/wayfinder/multi-persona-review/releases/tag/v0.1.0
