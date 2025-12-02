/**
 * Represents an NFC reader device
 */
export interface Reader {
  /** Unique identifier for the reader */
  id: string;
  /** Human-readable name of the reader */
  name: string;
  /** Reader type (e.g., "picc" for contactless) */
  type: string;
}

/**
 * Data type for card content
 */
export type CardDataType = 'text' | 'json' | 'binary' | 'url' | 'unknown';

/**
 * Represents an NFC card/tag
 */
export interface Card {
  /** Unique identifier (hex encoded) */
  uid: string;
  /** Answer To Reset (hex encoded) */
  atr?: string;
  /** Card type (e.g., "NTAG213", "NTAG215", "MIFARE Classic") */
  type?: string;
  /** Memory size in bytes */
  size?: number;
  /** Whether the card can be written to */
  writable?: boolean;
  /** Data stored on the card */
  data?: string;
  /** Type of data stored */
  dataType?: CardDataType;
}

/**
 * Options for writing data to a card
 */
export interface WriteOptions {
  /** Data to write (text, JSON string, or base64 for binary) */
  data?: string;
  /** Type of data being written */
  dataType: 'text' | 'json' | 'binary' | 'url';
  /** Optional URL to write as the first NDEF record */
  url?: string;
}

/**
 * Configuration options for the NFC Agent client
 */
export interface NFCAgentOptions {
  /** Base URL of the nfc-agent server (default: "http://127.0.0.1:32145") */
  baseUrl?: string;
  /** Request timeout in milliseconds (default: 5000) */
  timeout?: number;
}

/**
 * Options for card polling
 */
export interface PollOptions {
  /** Polling interval in milliseconds (default: 1000) */
  interval?: number;
}

/**
 * Information about a supported reader model
 */
export interface SupportedReader {
  name: string;
  manufacturer: string;
  description: string;
  supportedTags: string[];
  capabilities: {
    read: boolean;
    write: boolean;
    ndef: boolean;
  };
  limitations?: string[];
}

/**
 * Response from the supported readers endpoint
 */
export interface SupportedReadersResponse {
  readers: SupportedReader[];
}

/**
 * API error response structure
 */
export interface APIErrorResponse {
  error: string;
}

/**
 * API success response for write operations
 */
export interface WriteSuccessResponse {
  success: string;
}

// ============================================================================
// WebSocket Types
// ============================================================================

/**
 * WebSocket message types (requests)
 */
export type WSMessageType =
  | 'list_readers'
  | 'read_card'
  | 'write_card'
  | 'erase_card'
  | 'lock_card'
  | 'set_password'
  | 'remove_password'
  | 'write_records'
  | 'subscribe'
  | 'unsubscribe'
  | 'supported_readers'
  | 'version'
  | 'health';

/**
 * WebSocket event types (server push)
 */
export type WSEventType = 'card_detected' | 'card_removed';

/**
 * Base WebSocket message structure
 */
export interface WSMessage<T = unknown> {
  type: WSMessageType | WSEventType;
  id?: string;
  payload?: T;
}

/**
 * WebSocket response structure
 */
export interface WSResponse<T = unknown> {
  type: string;
  id?: string;
  success?: boolean;
  payload?: T;
  error?: string;
}

/**
 * Configuration options for the WebSocket client
 */
export interface NFCAgentWSOptions {
  /** WebSocket URL (default: "ws://127.0.0.1:32145/v1/ws") */
  url?: string;
  /** Request timeout in milliseconds (default: 5000) */
  timeout?: number;
  /** Auto-reconnect on disconnect (default: true) */
  autoReconnect?: boolean;
  /** Reconnect interval in milliseconds (default: 3000) */
  reconnectInterval?: number;
}

/**
 * Payload for read_card request
 */
export interface ReadCardPayload {
  reader: number;
}

/**
 * Payload for write_card request
 */
export interface WriteCardPayload {
  reader: number;
  data?: string;
  dataType: 'text' | 'json' | 'binary' | 'url';
  url?: string;
}

/**
 * Payload for erase_card request
 */
export interface EraseCardPayload {
  reader: number;
}

/**
 * Payload for lock_card request
 */
export interface LockCardPayload {
  reader: number;
  confirm: true; // Must be true to confirm permanent lock
}

/**
 * Payload for set_password request
 */
export interface SetPasswordPayload {
  reader: number;
  password: string;
}

/**
 * Payload for remove_password request
 */
export interface RemovePasswordPayload {
  reader: number;
  password: string;
}

/**
 * Single NDEF record for write_records
 */
export interface NDEFRecord {
  type: 'text' | 'url' | 'json' | 'binary' | 'mime';
  data: string;
  mimeType?: string; // For 'mime' type
}

/**
 * Payload for write_records request
 */
export interface WriteRecordsPayload {
  reader: number;
  records: NDEFRecord[];
}

/**
 * Payload for subscribe/unsubscribe requests
 */
export interface SubscribePayload {
  reader: number;
}

/**
 * Version info response
 */
export interface VersionInfo {
  version: string;
  build?: string;
  platform?: string;
}

/**
 * Health check response
 */
export interface HealthInfo {
  status: 'ok' | 'degraded' | 'error';
  uptime?: number;
}

/**
 * Card detected event payload
 */
export interface CardDetectedEvent {
  reader: number;
  card: Card;
}

/**
 * Card removed event payload
 */
export interface CardRemovedEvent {
  reader: number;
}
