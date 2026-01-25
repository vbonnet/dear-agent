/**
 * Mock keytar module for testing
 * Uses jest.fn() to support .mockImplementation(), .toHaveBeenCalledWith(), etc.
 */

// In-memory storage for default implementation
const mockStore: Map<string, Map<string, string>> = new Map();

// Default implementations (saved as references so they can be restored after jest.clearAllMocks())
const setPasswordImpl = async (service: string, account: string, password: string): Promise<void> => {
  if (!mockStore.has(service)) {
    mockStore.set(service, new Map());
  }
  mockStore.get(service)!.set(account, password);
};

const getPasswordImpl = async (service: string, account: string): Promise<string | null> => {
  const serviceStore = mockStore.get(service);
  if (!serviceStore) return null;
  return serviceStore.get(account) || null;
};

const deletePasswordImpl = async (service: string, account: string): Promise<boolean> => {
  const serviceStore = mockStore.get(service);
  if (!serviceStore || !serviceStore.has(account)) return false;
  serviceStore.delete(account);
  return true;
};

// Export mocks with default implementations
export const setPassword = jest.fn(setPasswordImpl);
export const getPassword = jest.fn(getPasswordImpl);
export const deletePassword = jest.fn(deletePasswordImpl);

/**
 * Test helper: Clear all mock data and reset call history
 * Call this in beforeEach() to ensure test isolation
 */
export function __clearMockStore(): void {
  mockStore.clear();
  // Reset mock call history and restore default implementations
  setPassword.mockClear();
  getPassword.mockClear();
  deletePassword.mockClear();

  // Restore implementations (in case jest.clearAllMocks() was called)
  setPassword.mockImplementation(setPasswordImpl);
  getPassword.mockImplementation(getPasswordImpl);
  deletePassword.mockImplementation(deletePasswordImpl);
}

/**
 * Test helper: Setup healthy token scenario
 * Token exists and is valid (not expired)
 */
export function __setupTokenHealthy(): void {
  getPassword.mockImplementation(async (service: string, account: string) => {
    return JSON.stringify({
      type: 'authorized_user',
      client_id: 'test-client-id',
      client_secret: 'test-secret',
      refresh_token: 'test-refresh-token',
      access_token: 'test-access-token',
      expiry_date: Date.now() + 3600000 // 1 hour from now
    });
  });
}
