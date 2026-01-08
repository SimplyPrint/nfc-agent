# @simplyprint/nfc-agent

JavaScript/TypeScript SDK for interacting with NFC readers via the [nfc-agent](https://github.com/SimplyPrint/nfc-agent) local server.

## Features

- Zero dependencies
- Works in browsers and Node.js (18+)
- TypeScript support with full type definitions
- **REST API client** for simple request/response operations
- **WebSocket client** for real-time card events and advanced features
- Card polling with event-based notifications

## Installation

This package is hosted on [GitHub Packages](https://github.com/SimplyPrint/nfc-agent/packages). To install:

1. Create or edit `.npmrc` in your project root:

```
@simplyprint:registry=https://npm.pkg.github.com
```

2. Install the package:

```bash
npm install @simplyprint/nfc-agent
```

## Prerequisites

The [nfc-agent](https://github.com/SimplyPrint/nfc-agent) must be running on the local machine.

- HTTP/REST API: `http://127.0.0.1:32145`
- HTTPS/REST API: `https://127.0.0.1:32145`
- WebSocket: `ws://127.0.0.1:32145/v1/ws`
- Secure WebSocket: `wss://127.0.0.1:32145/v1/ws`

Both HTTP and HTTPS are served on the same port. The agent auto-generates a self-signed TLS certificate on first run.

## Quick Start

### REST API (Simple)

```typescript
import { NFCAgentClient } from '@simplyprint/nfc-agent';

const client = new NFCAgentClient();

const readers = await client.getReaders();
const card = await client.readCard(0);
console.log('Card UID:', card.uid);
```

### WebSocket (Real-time)

```typescript
import { NFCAgentWebSocket } from '@simplyprint/nfc-agent';

const ws = new NFCAgentWebSocket();
await ws.connect();

// Subscribe to real-time card events
await ws.subscribe(0);

ws.on('card_detected', (event) => {
  console.log('Card detected:', event.card.uid);
});

ws.on('card_removed', (event) => {
  console.log('Card removed from reader', event.reader);
});
```

---

## WebSocket API (Recommended)

The WebSocket client provides real-time events and additional features not available via REST.

### Connection

```typescript
import { NFCAgentWebSocket } from '@simplyprint/nfc-agent';

const ws = new NFCAgentWebSocket({
  url: 'ws://127.0.0.1:32145/v1/ws',  // default
  timeout: 5000,                       // request timeout (default)
  autoReconnect: true,                 // auto-reconnect on disconnect (default)
  reconnectInterval: 3000,             // reconnect delay (default)
  secure: false,                       // use wss:// instead of ws:// (default: false)
});

await ws.connect();

// Connection events
ws.on('connected', () => console.log('Connected!'));
ws.on('disconnected', () => console.log('Disconnected'));
ws.on('error', (err) => console.error('Error:', err));

// Disconnect when done
ws.disconnect();
```

#### Using Secure WebSocket (Safari / HTTPS)

Safari blocks insecure WebSocket connections (`ws://`) from pages served over HTTPS. To work around this, use the `secure` option to connect via `wss://`:

```typescript
const ws = new NFCAgentWebSocket({ secure: true });
await ws.connect();
```

**Note:** The NFC Agent uses a **self-signed certificate** generated on first run. This provides encryption for the connection but browsers will show a certificate warning since it's not signed by a trusted CA. This is expected for a localhost-only service.

**Setup:** Before using secure WebSocket, the user must first visit `https://127.0.0.1:32145/` in their browser and accept the certificate warning. This only needs to be done once per browser.

### Reading & Writing Cards

```typescript
// List readers
const readers = await ws.getReaders();

// Read card
const card = await ws.readCard(0);
console.log(card.uid, card.type, card.data);

// Write text
await ws.writeCard(0, { data: 'Hello!', dataType: 'text' });

// Write JSON
await ws.writeCard(0, {
  data: JSON.stringify({ id: 123 }),
  dataType: 'json'
});

// Write URL
await ws.writeCard(0, {
  data: 'https://example.com',
  dataType: 'url'
});

// Write URL + JSON (multi-record)
await ws.writeCard(0, {
  url: 'https://simplyprint.io/spool/123',
  data: JSON.stringify({ id: 123 }),
  dataType: 'json'
});
```

### Real-time Card Events

```typescript
// Subscribe to a reader for real-time events
await ws.subscribe(0);

ws.on('card_detected', (event) => {
  console.log('Reader:', event.reader);
  console.log('Card UID:', event.card.uid);
  console.log('Card Type:', event.card.type);         // e.g., "NTAG213"
  console.log('Protocol:', event.card.protocol);      // e.g., "NFC-A"
  console.log('Protocol ISO:', event.card.protocolISO); // e.g., "ISO 14443-3A"
  console.log('Card Data:', event.card.data);
});

ws.on('card_removed', (event) => {
  console.log('Card removed from reader', event.reader);
});

// Unsubscribe when done
await ws.unsubscribe(0);
```

### Advanced Operations

```typescript
// Erase card data
await ws.eraseCard(0);

// Write multiple NDEF records
await ws.writeRecords(0, [
  { type: 'url', data: 'https://example.com' },
  { type: 'text', data: 'Hello World' },
  { type: 'json', data: '{"key": "value"}' },
]);

// Set password protection (NTAG cards)
await ws.setPassword(0, 'mypassword');

// Remove password protection
await ws.removePassword(0, 'mypassword');

// Lock card permanently (IRREVERSIBLE!)
await ws.lockCard(0);

// Get version info (includes update availability)
const version = await ws.getVersion();
console.log('Agent version:', version.version);
console.log('Build time:', version.buildTime);
if (version.updateAvailable) {
  console.log('Update available:', version.latestVersion);
  console.log('Download:', version.releaseUrl);
}

// Health check
const health = await ws.health();
console.log('Status:', health.status);
```

### MIFARE Classic Raw Block Access

For direct block-level access to MIFARE Classic cards (e.g., proprietary tag formats like QIDI BOX):

```typescript
// Read block 4 (first data block in sector 1)
const block = await ws.readMifareBlock(0, 4);
console.log(block.data); // hex string, e.g. "01120100000000000000000000000000"

// Read with specific authentication key
const block = await ws.readMifareBlock(0, 4, {
  key: 'D3F7D3F7D3F7',
  keyType: 'A'
});

// Write block 4
await ws.writeMifareBlock(0, 4, {
  data: '01120100000000000000000000000000',
  key: 'FFFFFFFFFFFF'
});

// Batch write multiple blocks (more efficient for multiple writes)
const result = await ws.writeMifareBlocks(0, {
  blocks: [
    { block: 4, data: '00112233445566778899AABBCCDDEEFF' },
    { block: 5, data: 'FFEEDDCCBBAA99887766554433221100' },
    { block: 8, data: 'DEADBEEFDEADBEEFDEADBEEFDEADBEEF' }  // Different sector - re-auths automatically
  ],
  key: 'FFFFFFFFFFFF',
  keyType: 'A'
});
console.log(`Wrote ${result.written}/${result.total} blocks`);
for (const r of result.results) {
  if (!r.success) {
    console.error(`Block ${r.block} failed: ${r.error}`);
  }
}
```

**Notes:**
- Block numbers: 0-63 for MIFARE Classic 1K, 0-255 for 4K
- Each block is 16 bytes (32 hex characters)
- Sector trailers (blocks 3, 7, 11, 15, etc.) are blocked for safety
- If no key is provided, common default keys are tried automatically

### MIFARE Ultralight Raw Page Access

For direct page-level access to MIFARE Ultralight cards:

```typescript
// Read page 4 (first user data page)
const page = await ws.readUltralightPage(0, 4);
console.log(page.data); // hex string, e.g. "DEADBEEF"

// Read with password (EV1 cards only)
const page = await ws.readUltralightPage(0, 4, {
  password: '12345678'  // 4 bytes as hex
});

// Write page 4
await ws.writeUltralightPage(0, 4, {
  data: 'DEADBEEF'  // 4 bytes as hex
});

// Write with password
await ws.writeUltralightPage(0, 4, {
  data: 'DEADBEEF',
  password: '12345678'
});
```

**Notes:**
- Pages 0-3 are system pages (blocked for writing)
- Each page is 4 bytes (8 hex characters)
- Password is only needed for EV1 variants with password protection enabled

### AES-Encrypted MIFARE Classic Operations

For MIFARE Classic tags that require AES-encrypted data (e.g., certain filament spool tags):

```typescript
// Derive a 6-byte sector key from the card's UID using AES encryption
const derived = await ws.deriveUIDKeyAES(0, {
  aesKey: '713362755e74316e71665a2870662431'  // 16 bytes as hex (32 chars)
});
console.log('Derived key:', derived.key);  // 6 bytes as hex (12 chars)

// Encrypt data with AES and write to a block
await ws.aesEncryptAndWriteBlock(0, 4, {
  data: '30303030303030303030303030303030',  // 16 bytes plaintext (will be encrypted)
  aesKey: '484043466b526e7a404b4174424a7032',  // AES encryption key
  authKey: 'FFFFFFFFFFFF',                      // MIFARE auth key
  authKeyType: 'A'
});

// Write sector trailer with new keys and optional access bits
await ws.writeMifareSectorTrailer(0, 7, {
  keyA: derived.key,       // New Key A
  keyB: derived.key,       // New Key B
  accessBits: 'FF0780',    // Optional - preserves existing if omitted
  authKey: 'FFFFFFFFFFFF', // Current auth key
  authKeyType: 'A'
});
```

**Notes:**
- AES keys are 16 bytes (32 hex characters)
- The derived key is 6 bytes (12 hex characters) - suitable for MIFARE authentication
- Data is encrypted before being written to the card
- Sector trailers are at blocks 3, 7, 11, 15, etc. (for 1K cards)
- `FF0780` is the standard "transport" access bits configuration

---

## REST API

For simple operations without real-time events, use the REST client.

### Usage

```typescript
import { NFCAgentClient } from '@simplyprint/nfc-agent';

const client = new NFCAgentClient({
  baseUrl: 'http://127.0.0.1:32145',  // default
  timeout: 5000,                       // default
});

// Check connection
const connected = await client.isConnected();

// List readers
const readers = await client.getReaders();

// Read card
const card = await client.readCard(0);

// Write card
await client.writeCard(0, { data: 'Hello!', dataType: 'text' });

// Get supported readers info
const supported = await client.getSupportedReaders();

// Get version info (includes update availability)
const version = await client.getVersion();
console.log('Version:', version.version);
if (version.updateAvailable) {
  console.log('Update available:', version.latestVersion);
}

// MIFARE Classic raw block access
const block = await client.readMifareBlock(0, 4, { key: 'FFFFFFFFFFFF' });
console.log('Block data:', block.data);

await client.writeMifareBlock(0, 4, {
  data: '01120100000000000000000000000000',
  key: 'FFFFFFFFFFFF'
});

// MIFARE Ultralight raw page access
const page = await client.readUltralightPage(0, 4);
console.log('Page data:', page.data);

await client.writeUltralightPage(0, 4, {
  data: 'DEADBEEF'
});
```

### Card Polling (REST)

For polling-based card detection with the REST API:

```typescript
const poller = client.pollCard(0, { interval: 500 });

poller.on('card', (card) => {
  console.log('Card detected:', card.uid);
});

poller.on('removed', () => {
  console.log('Card removed');
});

poller.start();
// poller.stop();
```

---

## API Reference

### `NFCAgentWebSocket`

| Method | Description |
|--------|-------------|
| `connect()` | Connect to WebSocket server |
| `disconnect()` | Disconnect from server |
| `getReaders()` | List available readers |
| `readCard(reader)` | Read card data |
| `writeCard(reader, options)` | Write data to card |
| `eraseCard(reader)` | Erase NDEF data |
| `lockCard(reader)` | Lock card permanently |
| `setPassword(reader, password)` | Set NTAG password |
| `removePassword(reader, password)` | Remove password |
| `writeRecords(reader, records)` | Write multiple NDEF records |
| `subscribe(reader)` | Subscribe to card events |
| `unsubscribe(reader)` | Unsubscribe from events |
| `getSupportedReaders()` | Get supported hardware info |
| `getVersion()` | Get agent version |
| `health()` | Health check |
| `readMifareBlock(reader, block, options?)` | Read raw MIFARE Classic block |
| `writeMifareBlock(reader, block, options)` | Write raw MIFARE Classic block |
| `writeMifareBlocks(reader, options)` | Write multiple MIFARE Classic blocks |
| `readUltralightPage(reader, page, options?)` | Read raw MIFARE Ultralight page |
| `writeUltralightPage(reader, page, options)` | Write raw MIFARE Ultralight page |
| `writeUltralightPages(reader, options)` | Write multiple MIFARE Ultralight pages |
| `deriveUIDKeyAES(reader, options)` | Derive 6-byte key from UID via AES |
| `aesEncryptAndWriteBlock(reader, block, options)` | AES encrypt + write block |
| `writeMifareSectorTrailer(reader, block, options)` | Write sector trailer with keys and access bits |

#### WebSocket Events

| Event | Callback | Description |
|-------|----------|-------------|
| `card_detected` | `(event: CardDetectedEvent) => void` | Card placed on reader |
| `card_removed` | `(event: CardRemovedEvent) => void` | Card removed |
| `connected` | `() => void` | Connected to server |
| `disconnected` | `() => void` | Disconnected |
| `error` | `(error: Error) => void` | Connection error |

### `NFCAgentClient`

| Method | Description |
|--------|-------------|
| `isConnected()` | Check if agent is running |
| `getReaders()` | List available readers |
| `readCard(reader)` | Read card data |
| `writeCard(reader, options)` | Write data to card |
| `getSupportedReaders()` | Get supported hardware info |
| `getVersion()` | Get agent version and update info |
| `readMifareBlock(reader, block, options?)` | Read raw MIFARE Classic block |
| `writeMifareBlock(reader, block, options)` | Write raw MIFARE Classic block |
| `writeMifareBlocks(reader, options)` | Write multiple MIFARE Classic blocks |
| `readUltralightPage(reader, page, options?)` | Read raw MIFARE Ultralight page |
| `writeUltralightPage(reader, page, options)` | Write raw MIFARE Ultralight page |
| `writeUltralightPages(reader, options)` | Write multiple MIFARE Ultralight pages |
| `deriveUIDKeyAES(reader, options)` | Derive 6-byte key from UID via AES |
| `aesEncryptAndWriteBlock(reader, block, options)` | AES encrypt + write block |
| `writeMifareSectorTrailer(reader, block, options)` | Write sector trailer with keys and access bits |
| `pollCard(reader, options)` | Create a CardPoller |

### Types

```typescript
interface Reader {
  id: string;
  name: string;
  type: string;
}

interface Card {
  uid: string;
  atr?: string;
  type?: string;           // e.g., "NTAG213", "MIFARE Classic", "ICode SLIX"
  protocol?: string;       // Short: "NFC-A", "NFC-V"
  protocolISO?: string;    // Full: "ISO 14443-3A", "ISO 15693"
  size?: number;
  writable?: boolean;
  data?: string;
  dataType?: 'text' | 'json' | 'binary' | 'url' | 'unknown';
}

interface NDEFRecord {
  type: 'text' | 'url' | 'json' | 'binary' | 'mime';
  data: string;
  mimeType?: string;
}

interface CardDetectedEvent {
  reader: number;
  card: Card;
}

interface CardRemovedEvent {
  reader: number;
}

interface VersionInfo {
  version: string;
  buildTime: string;
  gitCommit: string;
  updateAvailable?: boolean;  // true if a newer version exists
  latestVersion?: string;     // latest available version
  releaseUrl?: string;        // URL to download the update
}

// MIFARE Classic types
type MifareKeyType = 'A' | 'B';

interface MifareBlockData {
  block: number;
  data: string;  // 32 hex chars = 16 bytes
}

interface MifareReadOptions {
  key?: string;       // 12 hex chars = 6 bytes
  keyType?: MifareKeyType;
}

interface MifareWriteOptions {
  data: string;       // 32 hex chars = 16 bytes
  key?: string;       // 12 hex chars = 6 bytes
  keyType?: MifareKeyType;
}

interface MifareBlockWriteOp {
  block: number;
  data: string;       // 32 hex chars = 16 bytes
}

interface MifareBatchWriteOptions {
  blocks: MifareBlockWriteOp[];
  key?: string;       // 12 hex chars = 6 bytes
  keyType?: MifareKeyType;
}

interface MifareBlockWriteResult {
  block: number;
  success: boolean;
  error?: string;
}

interface MifareBatchWriteResult {
  results: MifareBlockWriteResult[];
  written: number;
  total: number;
}

// MIFARE Ultralight types
interface UltralightPageData {
  page: number;
  data: string;  // 8 hex chars = 4 bytes
}

interface UltralightReadOptions {
  password?: string;  // 8 hex chars = 4 bytes (EV1 only)
}

interface UltralightWriteOptions {
  data: string;       // 8 hex chars = 4 bytes
  password?: string;  // 8 hex chars = 4 bytes (EV1 only)
}

// AES MIFARE Classic types
interface DerivedKeyData {
  key: string;  // 12 hex chars = 6 bytes
}

interface DeriveUIDKeyOptions {
  aesKey: string;  // 32 hex chars = 16 bytes (AES-128 key)
}

interface AESEncryptWriteOptions {
  data: string;       // 32 hex chars = 16 bytes (plaintext to encrypt)
  aesKey: string;     // 32 hex chars = 16 bytes (AES-128 key)
  authKey: string;    // 12 hex chars = 6 bytes (MIFARE auth key)
  authKeyType?: MifareKeyType;
}

interface WriteMifareSectorTrailerOptions {
  keyA: string;        // 12 hex chars = 6 bytes
  keyB: string;        // 12 hex chars = 6 bytes
  accessBits?: string; // 6 or 8 hex chars (optional, preserves existing if omitted)
  authKey: string;     // 12 hex chars = 6 bytes
  authKeyType?: MifareKeyType;
}
```

---

## Error Handling

```typescript
import {
  NFCAgentWebSocket,
  ConnectionError,
  CardError
} from '@simplyprint/nfc-agent';

const ws = new NFCAgentWebSocket();

try {
  await ws.connect();
  const card = await ws.readCard(0);
} catch (error) {
  if (error instanceof ConnectionError) {
    console.error('Agent not running:', error.message);
  } else if (error instanceof CardError) {
    console.error('Card error:', error.message);
  }
}
```

## License

MIT
