import type {
  Reader,
  Card,
  WriteOptions,
  NFCAgentOptions,
  PollOptions,
  SupportedReadersResponse,
  APIErrorResponse,
  VersionInfo,
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
   * Create a card poller for automatic card detection
   * @param readerIndex - Index of the reader to poll
   * @param options - Polling options
   * @returns CardPoller instance
   */
  pollCard(readerIndex: number, options: PollOptions = {}): CardPoller {
    return new CardPoller(this, readerIndex, options);
  }
}
