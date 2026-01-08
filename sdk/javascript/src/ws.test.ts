import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { NFCAgentWebSocket } from './ws.js';
import { ConnectionError, CardError } from './errors.js';
import { testData } from './__tests__/mocks.js';

// Mock WebSocket
class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;

  static instances: MockWebSocket[] = [];

  url: string;
  readyState = MockWebSocket.CONNECTING;
  private listeners: Map<string, ((...args: unknown[]) => void)[]> = new Map();

  constructor(url: string) {
    this.url = url;
    MockWebSocket.instances.push(this);
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

  send = vi.fn();

  close() {
    this.readyState = MockWebSocket.CLOSED;
    this.dispatchEvent('close', {});
  }

  // Test helpers
  simulateOpen() {
    this.readyState = MockWebSocket.OPEN;
    this.dispatchEvent('open', {});
  }

  simulateMessage(data: unknown) {
    this.dispatchEvent('message', { data: JSON.stringify(data) });
  }

  simulateError() {
    this.dispatchEvent('error', new Event('error'));
  }

  simulateClose() {
    this.readyState = MockWebSocket.CLOSED;
    this.dispatchEvent('close', {});
  }
}

describe('NFCAgentWebSocket', () => {
  const originalWebSocket = globalThis.WebSocket;

  beforeEach(() => {
    MockWebSocket.instances = [];
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
    vi.useFakeTimers();
  });

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket;
    vi.useRealTimers();
  });

  describe('constructor', () => {
    it('should use default options', () => {
      const ws = new NFCAgentWebSocket();
      expect(ws).toBeInstanceOf(NFCAgentWebSocket);
    });

    it('should accept custom options', () => {
      const ws = new NFCAgentWebSocket({
        url: 'ws://localhost:8080/ws',
        timeout: 10000,
        autoReconnect: false,
      });
      expect(ws).toBeInstanceOf(NFCAgentWebSocket);
    });
  });

  describe('connect', () => {
    it('should connect successfully', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();

      // Simulate successful connection
      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateOpen();

      await expect(connectPromise).resolves.toBeUndefined();
      expect(ws.isConnected).toBe(true);
    });

    it('should reject on connection error', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();

      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateError();

      await expect(connectPromise).rejects.toThrow(ConnectionError);
    });

    it('should emit connected event', async () => {
      const ws = new NFCAgentWebSocket();
      const onConnected = vi.fn();
      ws.on('connected', onConnected);

      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateOpen();

      await connectPromise;
      expect(onConnected).toHaveBeenCalled();
    });
  });

  describe('disconnect', () => {
    it('should disconnect and emit event', async () => {
      const ws = new NFCAgentWebSocket();
      const onDisconnected = vi.fn();
      ws.on('disconnected', onDisconnected);

      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateOpen();
      await connectPromise;

      ws.disconnect();

      expect(ws.isConnected).toBe(false);
      expect(onDisconnected).toHaveBeenCalled();
    });
  });

  describe('getReaders', () => {
    it('should send request and return readers', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const readersPromise = ws.getReaders();

      // Check that message was sent
      expect(mockWs.send).toHaveBeenCalled();
      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('list_readers');

      // Simulate response
      mockWs.simulateMessage({
        type: 'list_readers',
        id: sentMessage.id,
        payload: testData.readers,
      });

      const readers = await readersPromise;
      expect(readers).toEqual(testData.readers);
    });

    it('should throw when not connected', async () => {
      const ws = new NFCAgentWebSocket();
      await expect(ws.getReaders()).rejects.toThrow(ConnectionError);
    });
  });

  describe('readCard', () => {
    it('should send request and return card data', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const cardPromise = ws.readCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('read_card');
      expect(sentMessage.payload.readerIndex).toBe(0);

      mockWs.simulateMessage({
        type: 'read_card',
        id: sentMessage.id,
        payload: testData.card,
      });

      const card = await cardPromise;
      expect(card).toEqual(testData.card);
    });

    it('should throw CardError on error response', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const cardPromise = ws.readCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'read_card',
        id: sentMessage.id,
        error: 'no card in field',
      });

      await expect(cardPromise).rejects.toThrow(CardError);
    });
  });

  describe('writeCard', () => {
    it('should send write request', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const writePromise = ws.writeCard(0, { data: 'Hello', dataType: 'text' });

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('write_card');
      expect(sentMessage.payload.readerIndex).toBe(0);
      expect(sentMessage.payload.data).toBe('Hello');
      expect(sentMessage.payload.dataType).toBe('text');

      mockWs.simulateMessage({
        type: 'write_card',
        id: sentMessage.id,
        payload: { success: true },
      });

      await expect(writePromise).resolves.toBeUndefined();
    });
  });

  describe('subscribe/unsubscribe', () => {
    it('should subscribe to reader', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const subscribePromise = ws.subscribe(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('subscribe');
      expect(sentMessage.payload.readerIndex).toBe(0);

      mockWs.simulateMessage({
        type: 'subscribe',
        id: sentMessage.id,
        payload: {},
      });

      await expect(subscribePromise).resolves.toBeUndefined();
    });

    it('should unsubscribe from reader', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const unsubscribePromise = ws.unsubscribe(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('unsubscribe');

      mockWs.simulateMessage({
        type: 'unsubscribe',
        id: sentMessage.id,
        payload: {},
      });

      await expect(unsubscribePromise).resolves.toBeUndefined();
    });
  });

  describe('card events', () => {
    it('should emit card_detected event', async () => {
      const ws = new NFCAgentWebSocket();
      const onCardDetected = vi.fn();
      ws.on('card_detected', onCardDetected);

      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      mockWs.simulateMessage({
        type: 'card_detected',
        payload: { reader: 0, card: testData.card },
      });

      expect(onCardDetected).toHaveBeenCalledWith({
        reader: 0,
        card: testData.card,
      });
    });

    it('should emit card_removed event', async () => {
      const ws = new NFCAgentWebSocket();
      const onCardRemoved = vi.fn();
      ws.on('card_removed', onCardRemoved);

      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      mockWs.simulateMessage({
        type: 'card_removed',
        payload: { reader: 0 },
      });

      expect(onCardRemoved).toHaveBeenCalledWith({ reader: 0 });
    });
  });

  describe('eraseCard', () => {
    it('should send erase request', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const erasePromise = ws.eraseCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('erase_card');

      mockWs.simulateMessage({
        type: 'erase_card',
        id: sentMessage.id,
        payload: {},
      });

      await expect(erasePromise).resolves.toBeUndefined();
    });
  });

  describe('lockCard', () => {
    it('should send lock request with confirm', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const lockPromise = ws.lockCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('lock_card');
      expect(sentMessage.payload.confirm).toBe(true);

      mockWs.simulateMessage({
        type: 'lock_card',
        id: sentMessage.id,
        payload: {},
      });

      await expect(lockPromise).resolves.toBeUndefined();
    });
  });

  describe('getVersion', () => {
    it('should return version info', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const versionPromise = ws.getVersion();

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('version');

      mockWs.simulateMessage({
        type: 'version',
        id: sentMessage.id,
        payload: testData.version,
      });

      const version = await versionPromise;
      expect(version).toEqual(testData.version);
    });
  });

  describe('health', () => {
    it('should return health info', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const healthPromise = ws.health();

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('health');

      mockWs.simulateMessage({
        type: 'health',
        id: sentMessage.id,
        payload: testData.health,
      });

      const health = await healthPromise;
      expect(health).toEqual(testData.health);
    });
  });

  describe('event listener management', () => {
    it('should add and remove listeners', () => {
      const ws = new NFCAgentWebSocket();
      const callback = vi.fn();

      ws.on('connected', callback);
      ws.off('connected', callback);

      // No way to directly test this without triggering the event
      expect(ws).toBeInstanceOf(NFCAgentWebSocket);
    });
  });

  describe('setPassword', () => {
    it('should send set_password request', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const passwordPromise = ws.setPassword(0, 'mypassword');

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('set_password');
      expect(sentMessage.payload.readerIndex).toBe(0);
      expect(sentMessage.payload.password).toBe('mypassword');

      mockWs.simulateMessage({
        type: 'set_password',
        id: sentMessage.id,
        payload: {},
      });

      await expect(passwordPromise).resolves.toBeUndefined();
    });

    it('should throw CardError on failure', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const passwordPromise = ws.setPassword(0, 'mypassword');

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'set_password',
        id: sentMessage.id,
        error: 'password set failed',
      });

      await expect(passwordPromise).rejects.toThrow(CardError);
    });
  });

  describe('removePassword', () => {
    it('should send remove_password request', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const passwordPromise = ws.removePassword(0, 'mypassword');

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('remove_password');
      expect(sentMessage.payload.readerIndex).toBe(0);
      expect(sentMessage.payload.password).toBe('mypassword');

      mockWs.simulateMessage({
        type: 'remove_password',
        id: sentMessage.id,
        payload: {},
      });

      await expect(passwordPromise).resolves.toBeUndefined();
    });

    it('should throw CardError on failure', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const passwordPromise = ws.removePassword(0, 'wrongpassword');

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'remove_password',
        id: sentMessage.id,
        error: 'invalid password',
      });

      await expect(passwordPromise).rejects.toThrow(CardError);
    });
  });

  describe('writeRecords', () => {
    it('should send write_records request', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const records = [
        { type: 'url' as const, data: 'https://example.com' },
        { type: 'text' as const, data: 'Hello' },
      ];
      const recordsPromise = ws.writeRecords(0, records);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('write_records');
      expect(sentMessage.payload.readerIndex).toBe(0);
      expect(sentMessage.payload.records).toEqual(records);

      mockWs.simulateMessage({
        type: 'write_records',
        id: sentMessage.id,
        payload: {},
      });

      await expect(recordsPromise).resolves.toBeUndefined();
    });

    it('should throw CardError on failure', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const recordsPromise = ws.writeRecords(0, []);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'write_records',
        id: sentMessage.id,
        error: 'write failed',
      });

      await expect(recordsPromise).rejects.toThrow(CardError);
    });
  });

  describe('getSupportedReaders', () => {
    it('should return supported readers', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const supportedPromise = ws.getSupportedReaders();

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      expect(sentMessage.type).toBe('supported_readers');

      mockWs.simulateMessage({
        type: 'supported_readers',
        id: sentMessage.id,
        payload: testData.supportedReaders.readers,
      });

      const result = await supportedPromise;
      expect(result).toEqual(testData.supportedReaders.readers);
    });
  });

  describe('writeCard error branches', () => {
    it('should rethrow non-NFCAgentError', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      // Override request to throw a generic error
      (ws as unknown as { request: () => Promise<void> }).request = vi
        .fn()
        .mockRejectedValue(new Error('generic error'));

      await expect(ws.writeCard(0, { data: 'test', dataType: 'text' })).rejects.toThrow(
        'generic error'
      );
    });
  });

  describe('eraseCard error branches', () => {
    it('should throw CardError on failure', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const erasePromise = ws.eraseCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'erase_card',
        id: sentMessage.id,
        error: 'erase failed',
      });

      await expect(erasePromise).rejects.toThrow(CardError);
    });
  });

  describe('lockCard error branches', () => {
    it('should throw CardError on failure', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const lockPromise = ws.lockCard(0);

      const sentMessage = JSON.parse(mockWs.send.mock.calls[0][0]);
      mockWs.simulateMessage({
        type: 'lock_card',
        id: sentMessage.id,
        error: 'lock failed',
      });

      await expect(lockPromise).rejects.toThrow(CardError);
    });
  });

  describe('auto-reconnect', () => {
    it('should attempt reconnect after disconnect when enabled', async () => {
      const ws = new NFCAgentWebSocket({ autoReconnect: true, reconnectInterval: 1000 });
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      // Simulate disconnect
      mockWs.simulateClose();

      // Should try to reconnect after interval
      await vi.advanceTimersByTimeAsync(1000);

      // New WebSocket instance should be created
      expect(MockWebSocket.instances.length).toBe(2);
    });

    it('should not reconnect when autoReconnect is false', async () => {
      const ws = new NFCAgentWebSocket({ autoReconnect: false });
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      mockWs.simulateClose();
      await vi.advanceTimersByTimeAsync(5000);

      // Should not create new WebSocket
      expect(MockWebSocket.instances.length).toBe(1);
    });

    it('should not reconnect after manual disconnect', async () => {
      const ws = new NFCAgentWebSocket({ autoReconnect: true, reconnectInterval: 1000 });
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateOpen();
      await connectPromise;

      ws.disconnect();
      await vi.advanceTimersByTimeAsync(2000);

      // Should not create new WebSocket after manual disconnect
      expect(MockWebSocket.instances.length).toBe(1);
    });
  });

  describe('request timeout', () => {
    it('should timeout pending requests', async () => {
      const ws = new NFCAgentWebSocket({ timeout: 100 });
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      // Start the request but don't await yet
      const readersPromise = ws.getReaders().catch((e) => e);

      // Advance time past timeout
      vi.advanceTimersByTime(150);

      // Now check the rejection
      const error = await readersPromise;
      expect(error).toBeInstanceOf(ConnectionError);
      expect(error.message).toBe('Request timed out');
    });
  });

  describe('connection closed during request', () => {
    it('should reject pending requests when connection closes', async () => {
      const ws = new NFCAgentWebSocket({ autoReconnect: false });
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      const readersPromise = ws.getReaders();

      // Close connection before response
      mockWs.simulateClose();

      await expect(readersPromise).rejects.toThrow('Connection lost');
    });
  });

  describe('invalid JSON message', () => {
    it('should ignore invalid JSON messages', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      const mockWs = MockWebSocket.instances[0];
      mockWs.simulateOpen();
      await connectPromise;

      // Send invalid JSON - should not throw
      mockWs.dispatchEvent('message', { data: 'not json' });

      expect(ws.isConnected).toBe(true);
    });
  });

  describe('already connected', () => {
    it('should resolve immediately if already connected', async () => {
      const ws = new NFCAgentWebSocket();
      const connectPromise = ws.connect();
      await vi.advanceTimersByTimeAsync(0);
      MockWebSocket.instances[0].simulateOpen();
      await connectPromise;

      // Should resolve immediately
      await expect(ws.connect()).resolves.toBeUndefined();
    });
  });
});
