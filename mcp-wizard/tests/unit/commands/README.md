# Health Command Tests
# noqa: docs-quality

## Overview

This directory contains comprehensive unit tests for health monitoring and diagnostic commands in the mcp-wizard Context Broker.

**Test Coverage**: 90%+ statement coverage requirement for health/doctor/session-start code (oss-n1nq.15-v2)

**Test Files**:
- `health.test.ts` - Unit tests for health command
- `doctor.test.ts` - Unit tests for doctor command

**Related Tests**:
- `../hooks/session-start.test.ts` - Unit tests for SessionStart hook
- `../../integration/health-diagnostics.test.ts` - Integration tests for health workflows
- `../../e2e/health/health-workflow.test.ts` - E2E tests for complete user workflows

## Running Tests

### All Tests
```bash
npm run test
```

### Watch Mode (TDD Workflow)
```bash
npm run test:watch
```

### With Coverage
```bash
npm run test:coverage
```

### Specific Test File
```bash
npm test -- tests/unit/commands/health.test.ts
```

### Integration Tests Only
```bash
npm run test:integration
```

### E2E Tests Only
```bash
npm run test:e2e
```

## Test Structure

### Unit Tests (`tests/unit/`)

Unit tests verify individual functions and methods in isolation. All external dependencies are mocked.

**Structure**:
```typescript
describe('Component Name', () => {
  beforeEach(() => {
    // Clear mocks before each test
  });

  describe('functionName()', () => {
    it('should do expected behavior');
  });
});
```

**Key Principles**:
- Test one function/method per describe block
- Use descriptive test names ("should ...")
- Clear all mocks in beforeEach
- Assert on behavior, not implementation

### Integration Tests (`tests/integration/`)

Integration tests verify interactions between multiple components.

**Coverage**:
- Complete health check workflow
- Component coordination
- Error propagation across components

### E2E Tests (`tests/e2e/`)

E2E tests verify complete user-facing workflows from invocation to output.

**Coverage**:
- Full CLI command execution
- User-facing output validation
- Performance requirements

## Mock Setup

### Available Mocks

The test suite uses Jest manual mocks in `tests/__mocks__/`:

1. **keytar** - Token storage (existing)
2. **ora** - Progress spinner (existing)
3. **inquirer** - CLI prompts (existing)
4. **open** - Browser opening (existing)
5. **child_process** - Process execution (NEW)
6. **https** - HTTPS requests (NEW)
7. **http** - HTTP requests (NEW)

### Using Mocks

Import mock helper functions:

```typescript
import { __clearMockStore as __clearKeytarMocks } from '../../__mocks__/keytar';
import { __clearProcessMocks, __setupProcessRunning, __setupProcessNotFound } from '../../__mocks__/child_process';
import { __clearNetworkMocks, __setupNetworkSuccess, __setupNetworkError } from '../../__mocks__/https';
```

### Mock Reset Pattern

**Always** clear mocks in `beforeEach`:

```typescript
beforeEach(() => {
  __clearKeytarMocks();
  __clearProcessMocks();
  __clearNetworkMocks();
});
```

### Mock Helper Functions

**child_process mocks**:
- `__clearProcessMocks()` - Reset to default state
- `__setupProcessRunning()` - Process executes successfully
- `__setupProcessNotFound()` - Process not found (ENOENT)
- `__setupProcessError()` - Process fails with error
- `__setupProcessTimeout()` - Process times out

**Network mocks (https/http)**:
- `__clearNetworkMocks()` - Reset to default state
- `__setupNetworkSuccess()` - HTTP request succeeds (200 OK)
- `__setupNetworkTimeout()` - HTTP request times out
- `__setupNetworkError()` - HTTP request fails (connection error)
- `__setupDNSFailure()` - DNS resolution fails

**keytar mocks**:
- `__clearMockStore()` - Clear all stored tokens

## Test Scenarios

### Predefined Health Scenarios

The `tests/fixtures/health/health-scenarios.ts` file defines reusable test scenarios:

```typescript
import { ALL_HEALTHY, TOKEN_EXPIRED, MCP_DOWN, NETWORK_FAILURE } from '../../fixtures/health/health-scenarios';

// Use in tests
it('should handle all healthy scenario', async () => {
  ALL_HEALTHY.mockSetup();
  const result = await checkHealth();
  expect(result.overall).toBe('healthy');
});
```

**Available Scenarios**:
1. `ALL_HEALTHY` - All systems operational
2. `TOKEN_EXPIRED` - Authentication token expired
3. `MCP_DOWN` - MCP process not running
4. `NETWORK_FAILURE` - Network connectivity issues
5. `INTENT_ANALYZER_DEGRADED` - Intent analyzer slow/degraded
6. `MIXED_STATES` - Some checks healthy, others failing

## Adding New Tests

### 1. Create Test File

Follow existing naming convention: `<feature>.test.ts`

### 2. Add File Header Comment

```typescript
/**
 * Unit Tests for <Feature Name>
 *
 * Tests <brief description of functionality>
 *
 * Requirements:
 * - Requirement 1
 * - Requirement 2
 *
 * Coverage Target: 90%+ statement coverage
 */
```

### 3. Import Dependencies

```typescript
import { functionToTest } from '../../../src/path/to/module';
import { __clearMockStore } from '../../__mocks__/keytar';
// ... other imports
```

### 4. Structure Tests

```typescript
describe('Component Name', () => {
  beforeEach(() => {
    // Clear mocks
  });

  describe('method()', () => {
    it('should handle expected case', () => {
      // Arrange
      // Act
      // Assert
    });

    it('should handle error case', () => {
      // Arrange
      // Act
      // Assert
    });
  });
});
```

### 5. Run Tests and Check Coverage

```bash
npm run test:coverage
```

Ensure 90%+ coverage on new code.

## Coverage Requirements

### Per-File Thresholds

Coverage thresholds are enforced on health/doctor/session-start code via `jest.config.js`:

```javascript
coverageThreshold: {
  './src/commands/health.ts': {
    statements: 90,
    branches: 80,
    functions: 90,
  },
  './src/commands/doctor.ts': {
    statements: 90,
    branches: 80,
    functions: 90,
  },
  './src/hooks/session-start.ts': {
    statements: 90,
    branches: 80,
    functions: 90,
  },
}
```

### Viewing Coverage Reports

After running `npm run test:coverage`:

1. **Terminal output**: Coverage summary displayed
2. **HTML report**: Open `coverage/index.html` in browser for detailed view

### Coverage Best Practices

- **Aim for 100%** on critical paths
- **Document exclusions** (if uncoverable code exists)
- **Test edge cases** (not just happy path)
- **Cover error handling** (catch blocks, error states)

## Troubleshooting

### Tests Failing

1. **Check mock setup**: Ensure mocks cleared in beforeEach
2. **Check imports**: Verify correct paths
3. **Check async/await**: Use `async` and `await` for Promise-based code
4. **Check assertions**: Verify expected values match actual

### Coverage Not Meeting Threshold

1. **Identify uncovered lines**: Check HTML coverage report
2. **Add missing test cases**: Cover uncovered branches/functions
3. **Review conditionals**: Test both true and false branches
4. **Test error paths**: Don't forget error handling

### Mocks Not Working

1. **Clear mocks**: Ensure `__clearXXX()` called in beforeEach
2. **Check mock implementation**: Verify helper functions exist
3. **Check jest.mock()**: For core modules, use jest.mock() in test file

## Best Practices

1. **One assertion per test** (when practical)
2. **Descriptive test names** ("should do X when Y")
3. **Arrange-Act-Assert pattern**
4. **Mock all external dependencies**
5. **Clean up after tests** (restore spies, clear mocks)
6. **Test behavior, not implementation**
7. **Keep tests simple and readable**

## Resources

- [Jest Documentation](https://jestjs.io/docs/getting-started)
- [Jest Mock Functions](https://jestjs.io/docs/mock-functions)
- [Jest Coverage](https://jestjs.io/docs/configuration#collectcoveragefrom-array)
- [Bead Requirements](../../docs/oss-n1nq.15-v2.md) (if exists)

## Questions or Issues

For questions about these tests or to report issues:

1. Check existing tests for examples
2. Review this README
3. Run `npm test -- --help` for Jest options
4. Contact the Phase 4-v2 team
