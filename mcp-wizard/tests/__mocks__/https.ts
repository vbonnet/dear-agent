/**
 * Mock https module for testing
 * Uses jest.fn() to support .mockImplementation(), .toHaveBeenCalledWith(), etc.
 */

// Default implementations for https mocks
const requestImpl = (): any => {
  // Default: return mock ClientRequest-like object
  return {
    on: jest.fn(),
    write: jest.fn(),
    end: jest.fn(),
  };
};

const getImpl = (url: string, callback: (res: any) => void): any => {
  // Default: successful response
  const mockResponse = {
    statusCode: 200,
    on: jest.fn((event, handler) => {
      if (event === 'data') {
        handler(Buffer.from('{"status":"ok"}'));
      }
      if (event === 'end') {
        handler();
      }
    }),
  };
  callback(mockResponse);
  return mockResponse;
};

// Export mocks with default implementations
export const request = jest.fn(requestImpl);
export const get = jest.fn(getImpl);

/**
 * Test helper: Clear all mock data and reset call history
 * Call this in beforeEach() to ensure test isolation
 */
export function __clearNetworkMocks(): void {
  request.mockClear();
  get.mockClear();

  // Restore default implementations
  request.mockImplementation(requestImpl);
  get.mockImplementation(getImpl);
}

/**
 * Test helper: Setup network success scenario
 * HTTP request completes successfully with 200 OK
 */
export function __setupNetworkSuccess(): void {
  get.mockImplementation((url, callback) => {
    const mockResponse = {
      statusCode: 200,
      on: jest.fn((event, handler) => {
        if (event === 'data') handler(Buffer.from('{"status":"healthy"}'));
        if (event === 'end') handler();
      }),
    };
    callback(mockResponse);
    return mockResponse;
  });
}

/**
 * Test helper: Setup network timeout scenario
 * HTTP request times out
 */
export function __setupNetworkTimeout(): void {
  get.mockImplementation((url, callback) => {
    const mockRequest = {
      on: jest.fn((event, handler) => {
        if (event === 'timeout') {
          setTimeout(() => handler(), 10);
        }
      }),
      setTimeout: jest.fn(),
      abort: jest.fn(),
    };
    return mockRequest;
  });
}

/**
 * Test helper: Setup network error scenario
 * HTTP request fails with connection error
 */
export function __setupNetworkError(): void {
  get.mockImplementation((url, callback) => {
    const mockRequest = {
      on: jest.fn((event, handler) => {
        if (event === 'error') {
          setTimeout(() => handler(new Error('ECONNREFUSED')), 10);
        }
      }),
    };
    return mockRequest;
  });
}

/**
 * Test helper: Setup DNS failure scenario
 * HTTP request fails with DNS resolution error
 */
export function __setupDNSFailure(): void {
  get.mockImplementation((url, callback) => {
    const mockRequest = {
      on: jest.fn((event, handler) => {
        if (event === 'error') {
          const error = new Error('getaddrinfo ENOTFOUND') as NodeJS.ErrnoException;
          error.code = 'ENOTFOUND';
          setTimeout(() => handler(error), 10);
        }
      }),
    };
    return mockRequest;
  });
}
