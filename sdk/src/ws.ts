import type {
  Reader,
  Card,
  NFCAgentWSOptions,
  WSMessageType,
  WSResponse,
  NDEFRecord,
  SupportedReader,
  VersionInfo,
  HealthInfo,
  CardDetectedEvent,
  CardRemovedEvent,
} from './types.js';
import { ConnectionError, CardError, NFCAgentError } from './errors.js';

const DEFAULT_WS_URL = 'ws://127.0.0.1:32145/v1/ws';
const DEFAULT_TIMEOUT = 5000;
const DEFAULT_RECONNECT_INTERVAL = 3000;

type CardDetectedCallback = (event: CardDetectedEvent) => void;
type CardRemovedCallback = (event: CardRemovedEvent) => void;
type ConnectionCallback = () => void;
type ErrorCallback = (error: Error) => void;

interface PendingRequest {
  resolve: (value: unknown) => void;
  reject: (error: Error) => void;
  timeout: ReturnType<typeof setTimeout>;
}

interface EventListeners {
  card_detected: CardDetectedCallback[];
  card_removed: CardRemovedCallback[];
  connected: ConnectionCallback[];
  disconnected: ConnectionCallback[];
  error: ErrorCallback[];
}

/**
 * WebSocket client for real-time NFC Agent communication
 *
 * Provides persistent connection with automatic reconnection,
 * request/response correlation, and event subscriptions.
 */
export class NFCAgentWebSocket {
  private readonly url: string;
  private readonly timeout: number;
  private readonly autoReconnect: boolean;
  private readonly reconnectInterval: number;

  private ws: WebSocket | null = null;
  private requestId = 0;
  private pendingRequests = new Map<string, PendingRequest>();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private isClosing = false;

  private listeners: EventListeners = {
    card_detected: [],
    card_removed: [],
    connected: [],
    disconnected: [],
    error: [],
  };

  /**
   * Create a new WebSocket client
   * @param options - Configuration options
   */
  constructor(options: NFCAgentWSOptions = {}) {
    this.url = options.url ?? DEFAULT_WS_URL;
    this.timeout = options.timeout ?? DEFAULT_TIMEOUT;
    this.autoReconnect = options.autoReconnect ?? true;
    this.reconnectInterval = options.reconnectInterval ?? DEFAULT_RECONNECT_INTERVAL;
  }

  /**
   * Check if connected to the server
   */
  get isConnected(): boolean {
    return this.ws?.readyState === WebSocket.OPEN;
  }

  /**
   * Connect to the NFC Agent WebSocket server
   */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        resolve();
        return;
      }

      this.isClosing = false;

      try {
        this.ws = new WebSocket(this.url);
      } catch (error) {
        reject(new ConnectionError('Failed to create WebSocket connection'));
        return;
      }

      const onOpen = () => {
        cleanup();
        this.emit('connected');
        resolve();
      };

      const onError = (event: Event) => {
        cleanup();
        const error = new ConnectionError('WebSocket connection failed');
        this.emit('error', error);
        reject(error);
      };

      const onClose = () => {
        cleanup();
        reject(new ConnectionError('WebSocket connection closed'));
      };

      const cleanup = () => {
        this.ws?.removeEventListener('open', onOpen);
        this.ws?.removeEventListener('error', onError);
        this.ws?.removeEventListener('close', onClose);
      };

      this.ws.addEventListener('open', onOpen);
      this.ws.addEventListener('error', onError);
      this.ws.addEventListener('close', onClose);

      // Set up permanent message handler
      this.ws.addEventListener('message', this.handleMessage.bind(this));
      this.ws.addEventListener('close', this.handleClose.bind(this));
    });
  }

  /**
   * Disconnect from the server
   */
  disconnect(): void {
    this.isClosing = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    // Reject all pending requests
    for (const [id, request] of this.pendingRequests) {
      clearTimeout(request.timeout);
      request.reject(new ConnectionError('Connection closed'));
    }
    this.pendingRequests.clear();
  }

  /**
   * Send a request and wait for response
   */
  private async request<T>(type: WSMessageType, payload?: unknown): Promise<T> {
    if (!this.isConnected) {
      throw new ConnectionError('Not connected to NFC Agent');
    }

    const id = `req-${++this.requestId}`;

    return new Promise<T>((resolve, reject) => {
      const timeout = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new ConnectionError('Request timed out'));
      }, this.timeout);

      this.pendingRequests.set(id, {
        resolve: resolve as (value: unknown) => void,
        reject,
        timeout,
      });

      const message = JSON.stringify({ type, id, payload });
      this.ws!.send(message);
    });
  }

  /**
   * Handle incoming WebSocket messages
   */
  private handleMessage(event: MessageEvent): void {
    let data: WSResponse;
    try {
      data = JSON.parse(event.data);
    } catch {
      return; // Ignore invalid JSON
    }

    // Handle server-pushed events
    if (data.type === 'card_detected') {
      const payload = data.payload as CardDetectedEvent;
      for (const callback of this.listeners.card_detected) {
        try {
          callback(payload);
        } catch {
          // Ignore callback errors
        }
      }
      return;
    }

    if (data.type === 'card_removed') {
      const payload = data.payload as CardRemovedEvent;
      for (const callback of this.listeners.card_removed) {
        try {
          callback(payload);
        } catch {
          // Ignore callback errors
        }
      }
      return;
    }

    // Handle request responses
    if (data.id) {
      const pending = this.pendingRequests.get(data.id);
      if (pending) {
        this.pendingRequests.delete(data.id);
        clearTimeout(pending.timeout);

        if (data.error) {
          pending.reject(new NFCAgentError(data.error));
        } else {
          pending.resolve(data.payload);
        }
      }
    }
  }

  /**
   * Handle WebSocket close
   */
  private handleClose(): void {
    this.emit('disconnected');

    // Reject all pending requests
    for (const [id, request] of this.pendingRequests) {
      clearTimeout(request.timeout);
      request.reject(new ConnectionError('Connection lost'));
    }
    this.pendingRequests.clear();

    // Auto-reconnect if enabled
    if (this.autoReconnect && !this.isClosing) {
      this.reconnectTimer = setTimeout(() => {
        this.connect().catch(() => {
          // Reconnection failed, will try again
        });
      }, this.reconnectInterval);
    }
  }

  /**
   * Emit an event to listeners
   */
  private emit(event: 'connected' | 'disconnected'): void;
  private emit(event: 'error', error: Error): void;
  private emit(event: string, ...args: unknown[]): void {
    const callbacks = this.listeners[event as keyof EventListeners];
    if (callbacks) {
      for (const callback of callbacks) {
        try {
          (callback as (...args: unknown[]) => void)(...args);
        } catch {
          // Ignore callback errors
        }
      }
    }
  }

  // ============================================================================
  // Event Listeners
  // ============================================================================

  /**
   * Register an event listener
   */
  on(event: 'card_detected', callback: CardDetectedCallback): this;
  on(event: 'card_removed', callback: CardRemovedCallback): this;
  on(event: 'connected', callback: ConnectionCallback): this;
  on(event: 'disconnected', callback: ConnectionCallback): this;
  on(event: 'error', callback: ErrorCallback): this;
  on(
    event: 'card_detected' | 'card_removed' | 'connected' | 'disconnected' | 'error',
    callback: CardDetectedCallback | CardRemovedCallback | ConnectionCallback | ErrorCallback
  ): this {
    const listeners = this.listeners[event as keyof EventListeners];
    if (listeners) {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (listeners as any[]).push(callback);
    }
    return this;
  }

  /**
   * Remove an event listener
   */
  off(event: 'card_detected', callback: CardDetectedCallback): this;
  off(event: 'card_removed', callback: CardRemovedCallback): this;
  off(event: 'connected', callback: ConnectionCallback): this;
  off(event: 'disconnected', callback: ConnectionCallback): this;
  off(event: 'error', callback: ErrorCallback): this;
  off(
    event: 'card_detected' | 'card_removed' | 'connected' | 'disconnected' | 'error',
    callback: CardDetectedCallback | CardRemovedCallback | ConnectionCallback | ErrorCallback
  ): this {
    const listeners = this.listeners[event as keyof EventListeners];
    if (listeners) {
      const index = listeners.indexOf(callback as never);
      if (index !== -1) {
        listeners.splice(index, 1);
      }
    }
    return this;
  }

  // ============================================================================
  // API Methods
  // ============================================================================

  /**
   * List available NFC readers
   */
  async getReaders(): Promise<Reader[]> {
    return this.request<Reader[]>('list_readers');
  }

  /**
   * Read card from a reader
   * @param readerIndex - Index of the reader (0-based)
   */
  async readCard(readerIndex: number): Promise<Card> {
    try {
      return await this.request<Card>('read_card', { reader: readerIndex });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write data to a card
   * @param readerIndex - Index of the reader
   * @param options - Write options
   */
  async writeCard(
    readerIndex: number,
    options: { data?: string; dataType: 'text' | 'json' | 'binary' | 'url'; url?: string }
  ): Promise<void> {
    try {
      await this.request('write_card', {
        reader: readerIndex,
        data: options.data,
        dataType: options.dataType,
        url: options.url,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Erase NDEF data from a card
   * @param readerIndex - Index of the reader
   */
  async eraseCard(readerIndex: number): Promise<void> {
    try {
      await this.request('erase_card', { reader: readerIndex });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Lock a card permanently (IRREVERSIBLE!)
   * @param readerIndex - Index of the reader
   */
  async lockCard(readerIndex: number): Promise<void> {
    try {
      await this.request('lock_card', { reader: readerIndex, confirm: true });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Set password protection on an NTAG card
   * @param readerIndex - Index of the reader
   * @param password - Password to set (4 bytes / 8 hex chars)
   */
  async setPassword(readerIndex: number, password: string): Promise<void> {
    try {
      await this.request('set_password', { reader: readerIndex, password });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Remove password protection from an NTAG card
   * @param readerIndex - Index of the reader
   * @param password - Current password
   */
  async removePassword(readerIndex: number, password: string): Promise<void> {
    try {
      await this.request('remove_password', { reader: readerIndex, password });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write multiple NDEF records to a card
   * @param readerIndex - Index of the reader
   * @param records - Array of NDEF records to write
   */
  async writeRecords(readerIndex: number, records: NDEFRecord[]): Promise<void> {
    try {
      await this.request('write_records', { reader: readerIndex, records });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Subscribe to card events on a reader
   * @param readerIndex - Index of the reader to subscribe to
   */
  async subscribe(readerIndex: number): Promise<void> {
    await this.request('subscribe', { reader: readerIndex });
  }

  /**
   * Unsubscribe from card events on a reader
   * @param readerIndex - Index of the reader to unsubscribe from
   */
  async unsubscribe(readerIndex: number): Promise<void> {
    await this.request('unsubscribe', { reader: readerIndex });
  }

  /**
   * Get list of supported reader hardware
   */
  async getSupportedReaders(): Promise<SupportedReader[]> {
    return this.request<SupportedReader[]>('supported_readers');
  }

  /**
   * Get agent version information
   */
  async getVersion(): Promise<VersionInfo> {
    return this.request<VersionInfo>('version');
  }

  /**
   * Health check
   */
  async health(): Promise<HealthInfo> {
    return this.request<HealthInfo>('health');
  }
}
