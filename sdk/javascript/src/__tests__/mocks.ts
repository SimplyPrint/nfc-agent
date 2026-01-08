import { vi } from 'vitest';

/**
 * Create a mock fetch function
 */
export function createMockFetch(responses: Map<string, { ok: boolean; data: unknown }>) {
  return vi.fn((url: string, options?: RequestInit) => {
    const response = responses.get(url);
    if (!response) {
      return Promise.reject(new Error('Network error'));
    }
    return Promise.resolve({
      ok: response.ok,
      status: response.ok ? 200 : 400,
      json: () => Promise.resolve(response.data),
    } as Response);
  });
}

/**
 * Create a mock WebSocket class
 */
export function createMockWebSocket() {
  const instances: MockWebSocketInstance[] = [];

  class MockWebSocketInstance {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSING = 2;
    static CLOSED = 3;

    url: string;
    readyState = MockWebSocketInstance.CONNECTING;
    private listeners: Map<string, ((...args: unknown[]) => void)[]> = new Map();

    constructor(url: string) {
      this.url = url;
      instances.push(this);
      // Simulate async connection
      setTimeout(() => {
        this.readyState = MockWebSocketInstance.OPEN;
        this.dispatchEvent('open', {});
      }, 0);
    }

    addEventListener(event: string, callback: (...args: unknown[]) => void) {
      const list = this.listeners.get(event) || [];
      list.push(callback);
      this.listeners.set(event, list);
    }

    removeEventListener(event: string, callback: (...args: unknown[]) => void) {
      const list = this.listeners.get(event) || [];
      const index = list.indexOf(callback);
      if (index !== -1) {
        list.splice(index, 1);
      }
    }

    dispatchEvent(event: string, data: unknown) {
      const list = this.listeners.get(event) || [];
      for (const callback of list) {
        callback(data);
      }
    }

    send = vi.fn((data: string) => {
      // Parse and echo back a response
      const message = JSON.parse(data);
      // Subclass can override this behavior
    });

    close() {
      this.readyState = MockWebSocketInstance.CLOSED;
      this.dispatchEvent('close', {});
    }

    // Test helper to simulate receiving a message
    receiveMessage(data: unknown) {
      this.dispatchEvent('message', { data: JSON.stringify(data) });
    }

    // Test helper to simulate an error
    simulateError() {
      this.dispatchEvent('error', new Error('WebSocket error'));
    }
  }

  return {
    MockWebSocket: MockWebSocketInstance as unknown as typeof WebSocket,
    instances,
    getLastInstance: () => instances[instances.length - 1],
  };
}

/**
 * Sample test data
 */
export const testData = {
  readers: [
    { id: 'reader-0', name: 'ACR122U', type: 'picc' },
    { id: 'reader-1', name: 'SCR3310', type: 'picc' },
  ],
  card: {
    uid: 'DEADBEEF',
    atr: '3B8F8001804F0CA0000003060300030000000068',
    type: 'NTAG215',
    size: 504,
    writable: true,
    data: 'Hello NFC',
    dataType: 'text' as const,
  },
  supportedReaders: {
    readers: [
      {
        name: 'ACR122U',
        manufacturer: 'ACS',
        description: 'Popular USB NFC reader',
        supportedTags: ['NTAG', 'MIFARE'],
        capabilities: { read: true, write: true, ndef: true },
      },
    ],
  },
  version: {
    version: '1.0.0',
    build: 'abc123',
    platform: 'darwin',
  },
  health: {
    status: 'ok' as const,
    uptime: 3600,
  },
};
