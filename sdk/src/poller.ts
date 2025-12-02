import type { Card, PollOptions } from './types.js';
import type { NFCAgentClient } from './client.js';

type CardCallback = (card: Card) => void;
type RemovedCallback = () => void;
type ErrorCallback = (error: Error) => void;

type EventMap = {
  card: CardCallback[];
  removed: RemovedCallback[];
  error: ErrorCallback[];
};

const DEFAULT_INTERVAL = 1000;

/**
 * Polls a reader for card presence and emits events on card detection/removal
 */
export class CardPoller {
  private readonly client: NFCAgentClient;
  private readonly readerIndex: number;
  private readonly interval: number;
  private intervalId: ReturnType<typeof setInterval> | null = null;
  private lastCardUID: string | null = null;
  private listeners: EventMap = {
    card: [],
    removed: [],
    error: [],
  };

  constructor(
    client: NFCAgentClient,
    readerIndex: number,
    options: PollOptions = {}
  ) {
    this.client = client;
    this.readerIndex = readerIndex;
    this.interval = options.interval ?? DEFAULT_INTERVAL;
  }

  /**
   * Start polling for cards
   */
  start(): void {
    if (this.intervalId !== null) {
      return; // Already running
    }

    // Do an immediate poll
    this.poll();

    // Set up interval
    this.intervalId = setInterval(() => {
      this.poll();
    }, this.interval);
  }

  /**
   * Stop polling
   */
  stop(): void {
    if (this.intervalId !== null) {
      clearInterval(this.intervalId);
      this.intervalId = null;
    }
    this.lastCardUID = null;
  }

  /**
   * Check if the poller is currently running
   */
  get isRunning(): boolean {
    return this.intervalId !== null;
  }

  /**
   * Register an event listener
   */
  on(event: 'card', callback: CardCallback): this;
  on(event: 'removed', callback: RemovedCallback): this;
  on(event: 'error', callback: ErrorCallback): this;
  on(
    event: 'card' | 'removed' | 'error',
    callback: CardCallback | RemovedCallback | ErrorCallback
  ): this {
    if (event === 'card') {
      this.listeners.card.push(callback as CardCallback);
    } else if (event === 'removed') {
      this.listeners.removed.push(callback as RemovedCallback);
    } else if (event === 'error') {
      this.listeners.error.push(callback as ErrorCallback);
    }
    return this;
  }

  /**
   * Remove an event listener
   */
  off(event: 'card', callback: CardCallback): this;
  off(event: 'removed', callback: RemovedCallback): this;
  off(event: 'error', callback: ErrorCallback): this;
  off(
    event: 'card' | 'removed' | 'error',
    callback: CardCallback | RemovedCallback | ErrorCallback
  ): this {
    if (event === 'card') {
      this.listeners.card = this.listeners.card.filter((cb) => cb !== callback);
    } else if (event === 'removed') {
      this.listeners.removed = this.listeners.removed.filter(
        (cb) => cb !== callback
      );
    } else if (event === 'error') {
      this.listeners.error = this.listeners.error.filter(
        (cb) => cb !== callback
      );
    }
    return this;
  }

  /**
   * Internal poll method
   */
  private async poll(): Promise<void> {
    try {
      const card = await this.client.readCard(this.readerIndex);

      // New card detected or card changed
      if (this.lastCardUID !== card.uid) {
        this.lastCardUID = card.uid;
        for (const callback of this.listeners.card) {
          try {
            callback(card);
          } catch {
            // Ignore callback errors
          }
        }
      }
    } catch (error) {
      // Card was removed or read failed
      if (this.lastCardUID !== null) {
        this.lastCardUID = null;
        for (const callback of this.listeners.removed) {
          try {
            callback();
          } catch {
            // Ignore callback errors
          }
        }
      }

      // Only emit error for non-card-related errors (connection issues)
      if (
        error instanceof Error &&
        !error.message.includes('no card') &&
        !error.message.includes('No card')
      ) {
        for (const callback of this.listeners.error) {
          try {
            callback(error);
          } catch {
            // Ignore callback errors
          }
        }
      }
    }
  }
}
