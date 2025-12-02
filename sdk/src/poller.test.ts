import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { CardPoller } from './poller.js';
import { NFCAgentClient } from './client.js';
import { testData } from './__tests__/mocks.js';

describe('CardPoller', () => {
  let mockClient: NFCAgentClient;

  beforeEach(() => {
    vi.useFakeTimers();
    mockClient = {
      readCard: vi.fn(),
    } as unknown as NFCAgentClient;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('constructor', () => {
    it('should create a poller with default options', () => {
      const poller = new CardPoller(mockClient, 0);
      expect(poller).toBeInstanceOf(CardPoller);
      expect(poller.isRunning).toBe(false);
    });

    it('should accept custom interval', () => {
      const poller = new CardPoller(mockClient, 0, { interval: 500 });
      expect(poller).toBeInstanceOf(CardPoller);
    });
  });

  describe('start/stop', () => {
    it('should start and stop polling', () => {
      const poller = new CardPoller(mockClient, 0);

      expect(poller.isRunning).toBe(false);
      poller.start();
      expect(poller.isRunning).toBe(true);
      poller.stop();
      expect(poller.isRunning).toBe(false);
    });

    it('should not start twice', () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockResolvedValue(testData.card);
      const poller = new CardPoller(mockClient, 0, { interval: 1000 });

      poller.start();
      poller.start(); // Should be ignored

      // Advance time
      vi.advanceTimersByTime(1000);

      // Should only have been called once for the initial read + one interval
      expect(mockClient.readCard).toHaveBeenCalledTimes(2);
    });
  });

  describe('card detection', () => {
    it('should emit card event when card is detected', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockResolvedValue(testData.card);
      const poller = new CardPoller(mockClient, 0, { interval: 1000 });
      const onCard = vi.fn();

      poller.on('card', onCard);
      poller.start();

      // Wait for the initial poll
      await vi.advanceTimersByTimeAsync(0);

      expect(onCard).toHaveBeenCalledWith(testData.card);
    });

    it('should emit card event only once for same card', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockResolvedValue(testData.card);
      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onCard = vi.fn();

      poller.on('card', onCard);
      poller.start();

      // Wait for initial poll
      await vi.advanceTimersByTimeAsync(0);
      // Wait for next poll
      await vi.advanceTimersByTimeAsync(100);
      // Wait for another poll
      await vi.advanceTimersByTimeAsync(100);

      // Should only emit once since UID hasn't changed
      expect(onCard).toHaveBeenCalledTimes(1);

      poller.stop();
    });

    it('should emit card event when different card is detected', async () => {
      const card1 = { ...testData.card, uid: 'CARD1' };
      const card2 = { ...testData.card, uid: 'CARD2' };

      (mockClient.readCard as ReturnType<typeof vi.fn>)
        .mockResolvedValueOnce(card1)
        .mockResolvedValueOnce(card2);

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onCard = vi.fn();

      poller.on('card', onCard);
      poller.start();

      await vi.advanceTimersByTimeAsync(0);
      expect(onCard).toHaveBeenCalledWith(card1);

      await vi.advanceTimersByTimeAsync(100);
      expect(onCard).toHaveBeenCalledWith(card2);
      expect(onCard).toHaveBeenCalledTimes(2);

      poller.stop();
    });
  });

  describe('card removal', () => {
    it('should emit removed event when card is removed', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>)
        .mockResolvedValueOnce(testData.card)
        .mockRejectedValueOnce(new Error('no card in field'));

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onRemoved = vi.fn();

      poller.on('removed', onRemoved);
      poller.start();

      // First poll - card detected
      await vi.advanceTimersByTimeAsync(0);

      // Second poll - card removed
      await vi.advanceTimersByTimeAsync(100);

      expect(onRemoved).toHaveBeenCalledTimes(1);

      poller.stop();
    });

    it('should not emit removed if no card was present', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockRejectedValue(
        new Error('no card in field')
      );

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onRemoved = vi.fn();

      poller.on('removed', onRemoved);
      poller.start();

      await vi.advanceTimersByTimeAsync(0);
      await vi.advanceTimersByTimeAsync(100);

      expect(onRemoved).not.toHaveBeenCalled();

      poller.stop();
    });
  });

  describe('error handling', () => {
    it('should emit error for connection errors', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockRejectedValue(
        new Error('Connection refused')
      );

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onError = vi.fn();

      poller.on('error', onError);
      poller.start();

      await vi.advanceTimersByTimeAsync(0);

      expect(onError).toHaveBeenCalledWith(expect.any(Error));

      poller.stop();
    });

    it('should not emit error for "no card" errors', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockRejectedValue(
        new Error('no card in field')
      );

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onError = vi.fn();

      poller.on('error', onError);
      poller.start();

      await vi.advanceTimersByTimeAsync(0);

      expect(onError).not.toHaveBeenCalled();

      poller.stop();
    });
  });

  describe('event listener management', () => {
    it('should support off() to remove listeners', async () => {
      (mockClient.readCard as ReturnType<typeof vi.fn>).mockResolvedValue(testData.card);

      const poller = new CardPoller(mockClient, 0, { interval: 100 });
      const onCard = vi.fn();

      poller.on('card', onCard);
      poller.off('card', onCard);
      poller.start();

      await vi.advanceTimersByTimeAsync(0);

      expect(onCard).not.toHaveBeenCalled();

      poller.stop();
    });

    it('should support chaining on() calls', () => {
      const poller = new CardPoller(mockClient, 0);

      const result = poller
        .on('card', () => {})
        .on('removed', () => {})
        .on('error', () => {});

      expect(result).toBe(poller);
    });
  });
});
