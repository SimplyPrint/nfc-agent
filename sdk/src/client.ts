import type {
  Reader,
  Card,
  WriteOptions,
  NFCAgentOptions,
  PollOptions,
  SupportedReadersResponse,
  APIErrorResponse,
  VersionInfo,
  MifareBlockData,
  MifareReadOptions,
  MifareWriteOptions,
  UltralightPageData,
  UltralightReadOptions,
  UltralightWriteOptions,
} from './types.js';
import { ConnectionError, CardError, APIError } from './errors.js';
import { CardPoller } from './poller.js';

const DEFAULT_BASE_URL = 'http://127.0.0.1:32145';
const DEFAULT_TIMEOUT = 5000;

/**
 * Client for interacting with the NFC Agent local server
 */
export class NFCAgentClient {
  private readonly baseUrl: string;
  private readonly timeout: number;

  /**
   * Create a new NFC Agent client
   * @param options - Configuration options
   */
  constructor(options: NFCAgentOptions = {}) {
    this.baseUrl = options.baseUrl ?? DEFAULT_BASE_URL;
    this.timeout = options.timeout ?? DEFAULT_TIMEOUT;
  }

  /**
   * Internal fetch wrapper with timeout and error handling
   */
  private async request<T>(
    endpoint: string,
    options: RequestInit = {}
  ): Promise<T> {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), this.timeout);

    try {
      const response = await fetch(`${this.baseUrl}${endpoint}`, {
        ...options,
        signal: controller.signal,
        headers: {
          'Content-Type': 'application/json',
          ...options.headers,
        },
      });

      const data = await response.json();

      if (!response.ok) {
        const errorData = data as APIErrorResponse;
        throw new APIError(
          errorData.error || `HTTP ${response.status}`,
          response.status
        );
      }

      return data as T;
    } catch (error) {
      if (error instanceof APIError) {
        throw error;
      }
      if (error instanceof Error) {
        if (error.name === 'AbortError') {
          throw new ConnectionError('Request timed out');
        }
        if (
          error.message.includes('fetch') ||
          error.message.includes('network') ||
          error.message.includes('Failed to fetch')
        ) {
          throw new ConnectionError(
            'Failed to connect to nfc-agent. Is the agent running?'
          );
        }
      }
      throw new ConnectionError(
        error instanceof Error ? error.message : 'Unknown connection error'
      );
    } finally {
      clearTimeout(timeoutId);
    }
  }

  /**
   * Check if the nfc-agent server is running and accessible
   * @returns true if connected, false otherwise
   */
  async isConnected(): Promise<boolean> {
    try {
      await this.getReaders();
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Get a list of available NFC readers
   * @returns Array of Reader objects
   */
  async getReaders(): Promise<Reader[]> {
    return this.request<Reader[]>('/v1/readers');
  }

  /**
   * Read card data from a specific reader
   * @param readerIndex - Index of the reader (0-based)
   * @returns Card data if a card is present
   * @throws CardError if no card is present or read fails
   */
  async readCard(readerIndex: number): Promise<Card> {
    try {
      return await this.request<Card>(`/v1/readers/${readerIndex}/card`);
    } catch (error) {
      if (error instanceof APIError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Write data to a card on a specific reader
   * @param readerIndex - Index of the reader (0-based)
   * @param options - Write options including data, dataType, and optional URL
   * @throws CardError if write fails
   */
  async writeCard(readerIndex: number, options: WriteOptions): Promise<void> {
    const body: Record<string, string> = {
      dataType: options.dataType,
    };

    if (options.dataType === 'url') {
      body.data = options.data || options.url || '';
    } else {
      if (options.data) {
        body.data = options.data;
      }
      if (options.url) {
        body.url = options.url;
      }
    }

    try {
      await this.request(`/v1/readers/${readerIndex}/card`, {
        method: 'POST',
        body: JSON.stringify(body),
      });
    } catch (error) {
      if (error instanceof APIError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Get information about supported reader hardware
   * @returns List of supported reader models with capabilities
   */
  async getSupportedReaders(): Promise<SupportedReadersResponse> {
    return this.request<SupportedReadersResponse>('/v1/supported-readers');
  }

  /**
   * Get agent version information
   * @returns Version info including build details and update availability
   */
  async getVersion(): Promise<VersionInfo> {
    return this.request<VersionInfo>('/v1/version');
  }

  /**
   * Read a raw 16-byte block from a MIFARE Classic card
   * @param readerIndex - Index of the reader (0-based)
   * @param block - Block number to read (0-63 for 1K, 0-255 for 4K)
   * @param options - Optional authentication key and key type
   * @returns Block data (16 bytes as hex string)
   * @throws CardError if read fails or authentication fails
   */
  async readMifareBlock(
    readerIndex: number,
    block: number,
    options?: MifareReadOptions
  ): Promise<MifareBlockData> {
    const params = new URLSearchParams();
    if (options?.key) {
      params.set('key', options.key);
    }
    if (options?.keyType) {
      params.set('keyType', options.keyType);
    }
    const query = params.toString();
    const url = `/v1/readers/${readerIndex}/mifare/${block}${query ? `?${query}` : ''}`;

    try {
      return await this.request<MifareBlockData>(url);
    } catch (error) {
      if (error instanceof APIError) {
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
   * @throws CardError if write fails or authentication fails
   */
  async writeMifareBlock(
    readerIndex: number,
    block: number,
    options: MifareWriteOptions
  ): Promise<void> {
    try {
      await this.request(`/v1/readers/${readerIndex}/mifare/${block}`, {
        method: 'POST',
        body: JSON.stringify({
          data: options.data,
          key: options.key,
          keyType: options.keyType,
        }),
      });
    } catch (error) {
      if (error instanceof APIError) {
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
   * @throws CardError if read fails or authentication fails
   */
  async readUltralightPage(
    readerIndex: number,
    page: number,
    options?: UltralightReadOptions
  ): Promise<UltralightPageData> {
    const params = new URLSearchParams();
    if (options?.password) {
      params.set('password', options.password);
    }
    const query = params.toString();
    const url = `/v1/readers/${readerIndex}/ultralight/${page}${query ? `?${query}` : ''}`;

    try {
      return await this.request<UltralightPageData>(url);
    } catch (error) {
      if (error instanceof APIError) {
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
   * @throws CardError if write fails or authentication fails
   */
  async writeUltralightPage(
    readerIndex: number,
    page: number,
    options: UltralightWriteOptions
  ): Promise<void> {
    try {
      await this.request(`/v1/readers/${readerIndex}/ultralight/${page}`, {
        method: 'POST',
        body: JSON.stringify({
          data: options.data,
          password: options.password,
        }),
      });
    } catch (error) {
      if (error instanceof APIError) {
        throw new CardError(error.message);
      }
      throw error;
    }
  }

  /**
   * Create a card poller for automatic card detection
   * @param readerIndex - Index of the reader to poll
   * @param options - Polling options
   * @returns CardPoller instance
   */
  pollCard(readerIndex: number, options: PollOptions = {}): CardPoller {
    return new CardPoller(this, readerIndex, options);
  }
}
