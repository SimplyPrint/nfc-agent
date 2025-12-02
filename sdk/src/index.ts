// REST API client
export { NFCAgentClient } from './client.js';

// WebSocket client
export { NFCAgentWebSocket } from './ws.js';

// Poller (for REST API)
export { CardPoller } from './poller.js';

// Errors
export {
  NFCAgentError,
  ConnectionError,
  ReaderError,
  CardError,
  APIError,
} from './errors.js';

// Types
export type {
  // Core types
  Reader,
  Card,
  CardDataType,
  WriteOptions,
  NFCAgentOptions,
  PollOptions,
  SupportedReader,
  SupportedReadersResponse,
  // WebSocket types
  NFCAgentWSOptions,
  WSMessageType,
  WSEventType,
  WSMessage,
  WSResponse,
  NDEFRecord,
  VersionInfo,
  HealthInfo,
  CardDetectedEvent,
  CardRemovedEvent,
} from './types.js';

// Convenience aliases for CDN/browser usage
export { NFCAgentClient as Client } from './client.js';
export { NFCAgentWebSocket as WebSocket } from './ws.js';
