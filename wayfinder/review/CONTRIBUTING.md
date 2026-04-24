# Contributing to Multi-Persona Review

Thank you for your interest in contributing to Multi-Persona Review! This document provides guidelines for contributing to the project.

## Code of Conduct

This project adheres to the Contributor Covenant [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code.

## Development Setup

1. **Prerequisites**:
   - Node.js >= 18.0.0
   - npm >= 9.0.0

2. **Install dependencies**:
   ```bash
   npm install
   ```

   This will also set up pre-commit hooks via Husky.

3. **Build the project**:
   ```bash
   npm run build
   ```

4. **Run tests**:
   ```bash
   npm test
   ```

5. **Type checking**:
   ```bash
   npm run type-check
   ```

## Pre-commit Hooks

This project uses Husky to enforce code quality before commits:

- **Lint-staged**: Automatically runs ESLint and Prettier on staged files
- **Commitlint**: Validates commit messages follow conventional commits format

Hooks are automatically installed when you run `npm install`. If commits are failing:

1. Ensure your commit message follows conventional commits format
2. Fix any linting errors: `npm run lint:fix`
3. Format code: `npm run format`

## Code Style Guidelines

- **TypeScript**: All code must be written in TypeScript with strict type checking
- **Formatting**: Code will be automatically formatted with Prettier (see `.prettierrc`)
- **Linting**: Code must pass ESLint checks (see `.eslintrc.json`)
- **Naming conventions**:
  - Use camelCase for variables and functions
  - Use PascalCase for classes and interfaces
  - Use UPPER_SNAKE_CASE for constants

## Testing Requirements

- **All tests must pass**: Run `npm test` before submitting
- **Add tests for new features**: All new code should include unit tests
- **Test coverage**: Maintain or improve existing coverage
- **Integration tests**: Add integration tests for new providers or major features

## Commit Message Conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <subject>

<body>

<footer>
```

**Types**:
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, no logic change)
- `refactor`: Code refactoring
- `test`: Adding or updating tests
- `chore`: Maintenance tasks

**Examples**:
```
feat(personas): add lateral thinking alternatives to findings

fix(cli): resolve path handling on Windows

docs(readme): update installation instructions
```

## Pull Request Process

1. **Fork the repository** and create a feature branch from `main`:
   ```bash
   git checkout -b feat/my-new-feature
   ```

2. **Make your changes** following the code style guidelines

3. **Add tests** for your changes

4. **Run the full test suite**:
   ```bash
   npm test
   npm run type-check
   ```

5. **Commit your changes** using conventional commit messages

6. **Push to your fork**:
   ```bash
   git push origin feat/my-new-feature
   ```

7. **Open a Pull Request** with:
   - Clear description of the changes
   - Reference to related issues (if any)
   - Test results showing all tests pass

## Pull Request Review

- PRs require at least one approval from a maintainer
- All CI checks must pass:
  - **Lint**: Code must pass ESLint with 0 errors
  - **Type Check**: TypeScript compilation must succeed
  - **Tests**: All tests must pass (300+ tests)
  - **Build**: Project must build successfully
  - **Security**: npm audit and CodeQL scans must pass
- Code coverage should not decrease
- Documentation must be updated for new features

## Continuous Integration

This project uses GitHub Actions for CI/CD:

### CI Workflow (`.github/workflows/ci.yml`)
Runs on every push and pull request:
- Matrix testing across Node.js 18, 20, 22
- Linting with ESLint
- Type checking with TypeScript compiler
- Full test suite execution
- Build verification
- Code coverage upload to Codecov

### Release Workflow (`.github/workflows/release.yml`)
Runs on pushes to `main` branch:
- Automated versioning using semantic-release
- CHANGELOG generation
- npm package publishing
- GitHub release creation

### Security Workflow (`.github/workflows/security.yml`)
Runs on push, PR, and daily schedule:
- npm audit for dependency vulnerabilities
- CodeQL analysis for code security issues

### Coverage Workflow (`.github/workflows/coverage.yml`)
Runs on push and pull requests:
- Generates test coverage reports
- Uploads coverage to Codecov
- Displays coverage summary

## Dependency Updates

We use Dependabot for automated dependency updates:
- Weekly scans for npm and GitHub Actions updates
- Grouped pull requests for production and development dependencies
- Automatic security vulnerability PRs

## Project Structure

```
multi-persona-review/
├── src/              # Source code
│   ├── cli.ts        # CLI entry point
│   ├── types.ts      # Type definitions
│   ├── review-engine.ts  # Core review engine
│   ├── formatters/   # Output formatters
│   └── ...
├── tests/            # Test files
│   ├── unit/         # Unit tests
│   └── integration/  # Integration tests
├── examples/         # Usage examples
├── docs/             # Documentation
└── dist/             # Built output
```

## Adding New Features

### Adding a New Persona
1. Create persona file in `personas/` directory
2. Follow the persona format specification
3. Add tests in `tests/unit/persona-loader.test.ts`
4. Update documentation

### Adding a New AI Provider
1. Create provider client in `src/` (e.g., `openai-client.ts`)
2. Implement the provider interface
3. Add integration tests
4. Update CLI to support new provider flag
5. Update documentation and examples

### Adding a New Output Format
1. Create formatter in `src/formatters/`
2. Implement the Formatter interface
3. Add tests in `tests/unit/formatters/`
4. Register formatter in CLI
5. Update documentation

## Reporting Bugs

- Use the [bug report template](.github/ISSUE_TEMPLATE/bug_report.md)
- Include:
  - Clear description of the bug
  - Steps to reproduce
  - Expected vs actual behavior
  - Environment details (Node version, OS, package version)
  - Relevant logs or error messages

## Requesting Features

- Use the [feature request template](.github/ISSUE_TEMPLATE/feature_request.md)
- Include:
  - Clear description of the feature
  - Use case / problem it solves
  - Proposed solution (if you have one)
  - Alternatives considered

## Security Vulnerabilities

**DO NOT** open public issues for security vulnerabilities. Instead, see [SECURITY.md](SECURITY.md) for responsible disclosure guidelines.

## License

By contributing to Multi-Persona Review, you agree that your contributions will be licensed under the Apache-2.0 License.

## Questions?

- Open a [discussion](https://github.com/wayfinder/multi-persona-review/discussions)
- Ask in the issue tracker
- Reach out to maintainers

Thank you for contributing!
