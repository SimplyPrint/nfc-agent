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
  /** Card type (e.g., "NTAG213", "NTAG215", "MIFARE Classic", "ICode SLIX") */
  type?: string;
  /** Short protocol name (e.g., "NFC-A", "NFC-V") */
  protocol?: string;
  /** Full ISO protocol name (e.g., "ISO 14443-3A", "ISO 15693") */
  protocolISO?: string;
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
  | 'health'
  | 'read_mifare_block'
  | 'write_mifare_block'
  | 'write_mifare_blocks'
  | 'read_ultralight_page'
  | 'write_ultralight_page'
  | 'write_ultralight_pages'
  | 'derive_uid_key_aes'
  | 'aes_encrypt_and_write_block'
  | 'write_mifare_sector_trailer';

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
  buildTime: string;
  gitCommit: string;
  /** Whether a newer version is available */
  updateAvailable?: boolean;
  /** Latest available version (if updateAvailable is true) */
  latestVersion?: string;
  /** URL to download the latest release */
  releaseUrl?: string;
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

// ============================================================================
// MIFARE Classic Types
// ============================================================================

/**
 * MIFARE Classic key type for authentication
 */
export type MifareKeyType = 'A' | 'B';

/**
 * Response from reading a MIFARE Classic block
 */
export interface MifareBlockData {
  /** Block number that was read */
  block: number;
  /** Block data as hex string (32 characters = 16 bytes) */
  data: string;
}

/**
 * Options for reading a MIFARE Classic block
 */
export interface MifareReadOptions {
  /** Authentication key as hex string (12 characters = 6 bytes). If not provided, default keys are tried. */
  key?: string;
  /** Key type: 'A' or 'B' (default: 'A') */
  keyType?: MifareKeyType;
}

/**
 * Options for writing a MIFARE Classic block
 */
export interface MifareWriteOptions {
  /** Block data as hex string (32 characters = 16 bytes) */
  data: string;
  /** Authentication key as hex string (12 characters = 6 bytes). If not provided, default keys are tried. */
  key?: string;
  /** Key type: 'A' or 'B' (default: 'A') */
  keyType?: MifareKeyType;
}

/**
 * Payload for read_mifare_block WebSocket request
 */
export interface ReadMifareBlockPayload {
  readerIndex: number;
  block: number;
  key?: string;
  keyType?: MifareKeyType;
}

/**
 * Payload for write_mifare_block WebSocket request
 */
export interface WriteMifareBlockPayload {
  readerIndex: number;
  block: number;
  data: string;
  key?: string;
  keyType?: MifareKeyType;
}

/**
 * A single block write operation for batch writes
 */
export interface MifareBlockWriteOp {
  /** Block number (0-255, excluding sector trailers) */
  block: number;
  /** Block data as hex string (32 characters = 16 bytes) */
  data: string;
}

/**
 * Options for batch writing multiple MIFARE Classic blocks
 */
export interface MifareBatchWriteOptions {
  /** Array of block write operations */
  blocks: MifareBlockWriteOp[];
  /** Authentication key as hex string (12 characters = 6 bytes). If not provided, default keys are tried. */
  key?: string;
  /** Key type: 'A' or 'B' (default: 'A') */
  keyType?: MifareKeyType;
}

/**
 * Result of a single block write in a batch operation
 */
export interface MifareBlockWriteResult {
  /** Block number */
  block: number;
  /** Whether the write succeeded */
  success: boolean;
  /** Error message if write failed */
  error?: string;
}

/**
 * Response from batch writing MIFARE Classic blocks
 */
export interface MifareBatchWriteResult {
  /** Results for each block write */
  results: MifareBlockWriteResult[];
  /** Number of blocks successfully written */
  written: number;
  /** Total number of blocks attempted */
  total: number;
}

/**
 * Payload for write_mifare_blocks WebSocket request
 */
export interface WriteMifareBlocksPayload {
  readerIndex: number;
  blocks: MifareBlockWriteOp[];
  key?: string;
  keyType?: MifareKeyType;
}

// ============================================================================
// MIFARE Ultralight Types
// ============================================================================

/**
 * Response from reading a MIFARE Ultralight page
 */
export interface UltralightPageData {
  /** Page number that was read */
  page: number;
  /** Page data as hex string (8 characters = 4 bytes) */
  data: string;
}

/**
 * Options for reading a MIFARE Ultralight page
 */
export interface UltralightReadOptions {
  /** Authentication password as hex string (8 characters = 4 bytes) for EV1 cards. Optional. */
  password?: string;
}

/**
 * Options for writing a MIFARE Ultralight page
 */
export interface UltralightWriteOptions {
  /** Page data as hex string (8 characters = 4 bytes) */
  data: string;
  /** Authentication password as hex string (8 characters = 4 bytes) for EV1 cards. Optional. */
  password?: string;
}

/**
 * Payload for read_ultralight_page WebSocket request
 */
export interface ReadUltralightPagePayload {
  readerIndex: number;
  page: number;
  password?: string;
}

/**
 * Payload for write_ultralight_page WebSocket request
 */
export interface WriteUltralightPagePayload {
  readerIndex: number;
  page: number;
  data: string;
  password?: string;
}

/**
 * A single page write operation for batch writes
 */
export interface UltralightPageWriteOp {
  /** Page number (4-255 for user data) */
  page: number;
  /** Page data as hex string (8 characters = 4 bytes) */
  data: string;
}

/**
 * Options for batch writing multiple MIFARE Ultralight pages
 */
export interface UltralightBatchWriteOptions {
  /** Array of page write operations */
  pages: UltralightPageWriteOp[];
  /** Authentication password as hex string (8 characters = 4 bytes) for EV1 cards. Optional. */
  password?: string;
}

/**
 * Result of a single page write in a batch operation
 */
export interface UltralightPageWriteResult {
  /** Page number */
  page: number;
  /** Whether the write succeeded */
  success: boolean;
  /** Error message if write failed */
  error?: string;
}

/**
 * Response from batch writing MIFARE Ultralight pages
 */
export interface UltralightBatchWriteResult {
  /** Results for each page write */
  results: UltralightPageWriteResult[];
  /** Number of pages successfully written */
  written: number;
  /** Total number of pages attempted */
  total: number;
}

/**
 * Payload for write_ultralight_pages WebSocket request
 */
export interface WriteUltralightPagesPayload {
  readerIndex: number;
  pages: UltralightPageWriteOp[];
  password?: string;
}

// ============================================================================
// AES MIFARE Classic Types
// ============================================================================

/**
 * Response from deriving a UID key via AES
 */
export interface DerivedKeyData {
  /** Derived 6-byte MIFARE key as hex string (12 characters) */
  key: string;
}

/**
 * Options for deriving a UID key via AES
 */
export interface DeriveUIDKeyOptions {
  /** AES-128 key as hex string (32 characters = 16 bytes) */
  aesKey: string;
}

/**
 * Options for AES encrypt and write block
 */
export interface AESEncryptWriteOptions {
  /** Block data as hex string (32 characters = 16 bytes) - will be AES encrypted before writing */
  data: string;
  /** AES-128 encryption key as hex string (32 characters = 16 bytes) */
  aesKey: string;
  /** MIFARE authentication key as hex string (12 characters = 6 bytes) */
  authKey: string;
  /** Key type: 'A' or 'B' (default: 'A') */
  authKeyType?: MifareKeyType;
}

/**
 * Options for writing a MIFARE Classic sector trailer
 */
export interface WriteMifareSectorTrailerOptions {
  /** New Key A as hex string (12 characters = 6 bytes) */
  keyA: string;
  /** New Key B as hex string (12 characters = 6 bytes) */
  keyB: string;
  /** Access bits as hex string (6 or 8 characters = 3 or 4 bytes). If omitted, existing access bits are preserved. */
  accessBits?: string;
  /** Authentication key as hex string (12 characters = 6 bytes) */
  authKey: string;
  /** Key type for authentication: 'A' or 'B' (default: 'A') */
  authKeyType?: MifareKeyType;
}

/**
 * Payload for derive_uid_key_aes WebSocket request
 */
export interface DeriveUIDKeyPayload {
  readerIndex: number;
  aesKey: string;
}

/**
 * Payload for aes_encrypt_and_write_block WebSocket request
 */
export interface AESEncryptWritePayload {
  readerIndex: number;
  block: number;
  data: string;
  aesKey: string;
  authKey: string;
  authKeyType?: MifareKeyType;
}

/**
 * Payload for write_mifare_sector_trailer WebSocket request
 */
export interface WriteMifareSectorTrailerPayload {
  readerIndex: number;
  block: number;
  keyA: string;
  keyB: string;
  accessBits?: string;
  authKey: string;
  authKeyType?: MifareKeyType;
}
