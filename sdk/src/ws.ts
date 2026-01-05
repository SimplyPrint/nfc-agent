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
  MifareBlockData,
  MifareReadOptions,
  MifareWriteOptions,
  MifareBatchWriteOptions,
  MifareBatchWriteResult,
  UltralightPageData,
  UltralightReadOptions,
  UltralightWriteOptions,
  UltralightBatchWriteOptions,
  UltralightBatchWriteResult,
  DerivedKeyData,
  DeriveUIDKeyOptions,
  AESEncryptWriteOptions,
  WriteMifareSectorTrailerOptions,
} from './types.js';
import { ConnectionError, CardError, NFCAgentError } from './errors.js';

const DEFAULT_WS_URL = 'ws://127.0.0.1:32145/v1/ws';
const DEFAULT_WSS_URL = 'wss://127.0.0.1:32145/v1/ws';
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
    const defaultUrl = options.secure ? DEFAULT_WSS_URL : DEFAULT_WS_URL;
    this.url = options.url ?? defaultUrl;
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
      return await this.request<Card>('read_card', { readerIndex });
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
        readerIndex,
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
      await this.request('erase_card', { readerIndex });
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
      await this.request('lock_card', { readerIndex, confirm: true });
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
      await this.request('set_password', { readerIndex, password });
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
      await this.request('remove_password', { readerIndex, password });
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
      await this.request('write_records', { readerIndex, records });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Read a raw 16-byte block from a MIFARE Classic card
   * @param readerIndex - Index of the reader (0-based)
   * @param block - Block number to read (0-63 for 1K, 0-255 for 4K)
   * @param options - Optional authentication key and key type
   * @returns Block data (16 bytes as hex string)
   */
  async readMifareBlock(
    readerIndex: number,
    block: number,
    options?: MifareReadOptions
  ): Promise<MifareBlockData> {
    try {
      return await this.request<MifareBlockData>('read_mifare_block', {
        readerIndex,
        block,
        key: options?.key,
        keyType: options?.keyType,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write a raw 16-byte block to a MIFARE Classic card
   * @param readerIndex - Index of the reader (0-based)
   * @param block - Block number to write (0-63 for 1K, 0-255 for 4K)
   * @param options - Write options including data and optional authentication key
   */
  async writeMifareBlock(
    readerIndex: number,
    block: number,
    options: MifareWriteOptions
  ): Promise<void> {
    try {
      await this.request('write_mifare_block', {
        readerIndex,
        block,
        data: options.data,
        key: options.key,
        keyType: options.keyType,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write multiple blocks to a MIFARE Classic card in a single card session.
   * This is more efficient and reliable than multiple individual writeMifareBlock calls.
   * Re-authenticates automatically when crossing sector boundaries.
   * @param readerIndex - Index of the reader (0-based)
   * @param options - Options including array of block writes and optional authentication key
   * @returns Results for each block write operation
   */
  async writeMifareBlocks(
    readerIndex: number,
    options: MifareBatchWriteOptions
  ): Promise<MifareBatchWriteResult> {
    try {
      return await this.request<MifareBatchWriteResult>('write_mifare_blocks', {
        readerIndex,
        blocks: options.blocks,
        key: options.key,
        keyType: options.keyType,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Read a 4-byte page from a MIFARE Ultralight card
   * @param readerIndex - Index of the reader (0-based)
   * @param page - Page number to read (0-based, page 4+ for user data)
   * @param options - Optional password for EV1 cards with password protection
   * @returns Page data (4 bytes as hex string)
   */
  async readUltralightPage(
    readerIndex: number,
    page: number,
    options?: UltralightReadOptions
  ): Promise<UltralightPageData> {
    try {
      return await this.request<UltralightPageData>('read_ultralight_page', {
        readerIndex,
        page,
        password: options?.password,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write a 4-byte page to a MIFARE Ultralight card
   * @param readerIndex - Index of the reader (0-based)
   * @param page - Page number to write (minimum 4 for user data)
   * @param options - Write options including data and optional password
   */
  async writeUltralightPage(
    readerIndex: number,
    page: number,
    options: UltralightWriteOptions
  ): Promise<void> {
    try {
      await this.request('write_ultralight_page', {
        readerIndex,
        page,
        data: options.data,
        password: options?.password,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write multiple pages to a MIFARE Ultralight / NTAG card in a single card session.
   * This is more efficient and reliable than multiple individual writeUltralightPage calls.
   * @param readerIndex - Index of the reader (0-based)
   * @param options - Options including array of page writes and optional password
   * @returns Results for each page write operation
   */
  async writeUltralightPages(
    readerIndex: number,
    options: UltralightBatchWriteOptions
  ): Promise<UltralightBatchWriteResult> {
    try {
      return await this.request<UltralightBatchWriteResult>('write_ultralight_pages', {
        readerIndex,
        pages: options.pages,
        password: options?.password,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Derive a 6-byte MIFARE sector key from the card's UID using AES-128-ECB encryption.
   * @param readerIndex - Index of the reader (0-based)
   * @param options - Options including the AES key
   * @returns Derived 6-byte key as hex string (12 characters)
   */
  async deriveUIDKeyAES(
    readerIndex: number,
    options: DeriveUIDKeyOptions
  ): Promise<DerivedKeyData> {
    try {
      return await this.request<DerivedKeyData>('derive_uid_key_aes', {
        readerIndex,
        aesKey: options.aesKey,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Encrypt data with AES-128-ECB and write to a MIFARE Classic block.
   * @param readerIndex - Index of the reader (0-based)
   * @param block - Block number to write (cannot be a sector trailer)
   * @param options - Options including data, AES key, and authentication key
   */
  async aesEncryptAndWriteBlock(
    readerIndex: number,
    block: number,
    options: AESEncryptWriteOptions
  ): Promise<void> {
    try {
      await this.request('aes_encrypt_and_write_block', {
        readerIndex,
        block,
        data: options.data,
        aesKey: options.aesKey,
        authKey: options.authKey,
        authKeyType: options.authKeyType,
      });
    } catch (error) {
      if (error instanceof NFCAgentError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write a MIFARE Classic sector trailer with new keys and optional access bits.
   * @param readerIndex - Index of the reader (0-based)
   * @param block - Sector trailer block number (3, 7, 11, 15, ... for 1K)
   * @param options - Options including new keys, optional access bits, and authentication key
   */
  async writeMifareSectorTrailer(
    readerIndex: number,
    block: number,
    options: WriteMifareSectorTrailerOptions
  ): Promise<void> {
    try {
      await this.request('write_mifare_sector_trailer', {
        readerIndex,
        block,
        keyA: options.keyA,
        keyB: options.keyB,
        accessBits: options.accessBits,
        authKey: options.authKey,
        authKeyType: options.authKeyType,
      });
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
    await this.request('subscribe', { readerIndex });
  }

  /**
   * Unsubscribe from card events on a reader
   * @param readerIndex - Index of the reader to unsubscribe from
   */
  async unsubscribe(readerIndex: number): Promise<void> {
    await this.request('unsubscribe', { readerIndex });
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
