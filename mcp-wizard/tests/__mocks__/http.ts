/**
 * Mock http module for testing
 * Uses jest.fn() to support .mockImplementation(), .toHaveBeenCalledWith(), etc.
 *
 * This is a simpler version of https mock for non-secure HTTP requests
 */

// Default implementations for http mocks
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
export function __clearHttpMocks(): void {
  request.mockClear();
  get.mockClear();

  // Restore default implementations
  request.mockImplementation(requestImpl);
  get.mockImplementation(getImpl);
}
