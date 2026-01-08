import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { NFCAgentClient } from './client.js';
import { ConnectionError, CardError } from './errors.js';
import { testData } from './__tests__/mocks.js';

describe('NFCAgentClient', () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    vi.useFakeTimers();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.useRealTimers();
  });

  describe('constructor', () => {
    it('should use default options', () => {
      const client = new NFCAgentClient();
      expect(client).toBeInstanceOf(NFCAgentClient);
    });

    it('should accept custom options', () => {
      const client = new NFCAgentClient({
        baseUrl: 'http://localhost:8080',
        timeout: 10000,
      });
      expect(client).toBeInstanceOf(NFCAgentClient);
    });
  });

  describe('getReaders', () => {
    it('should return list of readers', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(testData.readers),
      });

      const client = new NFCAgentClient();
      const readers = await client.getReaders();

      expect(readers).toEqual(testData.readers);
      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers',
        expect.objectContaining({
          headers: { 'Content-Type': 'application/json' },
        })
      );
    });

    it('should throw ConnectionError on network failure', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Failed to fetch'));

      const client = new NFCAgentClient();
      await expect(client.getReaders()).rejects.toThrow(ConnectionError);
    });

    it('should throw ConnectionError on abort', async () => {
      // Simulate an aborted request
      const abortError = new Error('AbortError');
      abortError.name = 'AbortError';
      globalThis.fetch = vi.fn().mockRejectedValue(abortError);

      const client = new NFCAgentClient();
      await expect(client.getReaders()).rejects.toThrow(ConnectionError);
      await expect(client.getReaders()).rejects.toThrow('Request timed out');
    });
  });

  describe('readCard', () => {
    it('should return card data', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(testData.card),
      });

      const client = new NFCAgentClient();
      const card = await client.readCard(0);

      expect(card).toEqual(testData.card);
      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers/0/card',
        expect.any(Object)
      );
    });

    it('should throw CardError when no card present', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ error: 'no card in field' }),
      });

      const client = new NFCAgentClient();
      await expect(client.readCard(0)).rejects.toThrow(CardError);
    });

    it('should rethrow non-APIError', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Network error'));

      const client = new NFCAgentClient();
      await expect(client.readCard(0)).rejects.toThrow(ConnectionError);
    });
  });

  describe('writeCard', () => {
    it('should write text data', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: 'data written' }),
      });

      const client = new NFCAgentClient();
      await client.writeCard(0, { data: 'Hello', dataType: 'text' });

      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers/0/card',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ dataType: 'text', data: 'Hello' }),
        })
      );
    });

    it('should write URL data', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: 'data written' }),
      });

      const client = new NFCAgentClient();
      await client.writeCard(0, { data: 'https://example.com', dataType: 'url' });

      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers/0/card',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({ dataType: 'url', data: 'https://example.com' }),
        })
      );
    });

    it('should throw CardError on write failure', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 400,
        json: () => Promise.resolve({ error: 'write failed' }),
      });

      const client = new NFCAgentClient();
      await expect(
        client.writeCard(0, { data: 'test', dataType: 'text' })
      ).rejects.toThrow(CardError);
    });

    it('should write with url option for non-url dataType', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: 'data written' }),
      });

      const client = new NFCAgentClient();
      await client.writeCard(0, {
        data: '{"id": 123}',
        dataType: 'json',
        url: 'https://example.com',
      });

      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers/0/card',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            dataType: 'json',
            data: '{"id": 123}',
            url: 'https://example.com',
          }),
        })
      );
    });

    it('should write url dataType with url option fallback', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve({ success: 'data written' }),
      });

      const client = new NFCAgentClient();
      await client.writeCard(0, {
        dataType: 'url',
        url: 'https://example.com',
      });

      expect(fetch).toHaveBeenCalledWith(
        'http://127.0.0.1:32145/v1/readers/0/card',
        expect.objectContaining({
          method: 'POST',
          body: JSON.stringify({
            dataType: 'url',
            data: 'https://example.com',
          }),
        })
      );
    });

    it('should rethrow non-APIError', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Network error'));

      const client = new NFCAgentClient();
      await expect(
        client.writeCard(0, { data: 'test', dataType: 'text' })
      ).rejects.toThrow(ConnectionError);
    });
  });

  describe('getSupportedReaders', () => {
    it('should return supported readers info', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(testData.supportedReaders),
      });

      const client = new NFCAgentClient();
      const result = await client.getSupportedReaders();

      expect(result).toEqual(testData.supportedReaders);
    });
  });

  describe('isConnected', () => {
    it('should return true when agent is running', async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: () => Promise.resolve([]),
      });

      const client = new NFCAgentClient();
      const connected = await client.isConnected();

      expect(connected).toBe(true);
    });

    it('should return false when agent is not running', async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error('Failed to fetch'));

      const client = new NFCAgentClient();
      const connected = await client.isConnected();

      expect(connected).toBe(false);
    });
  });

  describe('pollCard', () => {
    it('should return a CardPoller instance', () => {
      const client = new NFCAgentClient();
      const poller = client.pollCard(0, { interval: 500 });

      expect(poller).toBeDefined();
      expect(typeof poller.start).toBe('function');
      expect(typeof poller.stop).toBe('function');
      expect(typeof poller.on).toBe('function');
    });
  });
});
