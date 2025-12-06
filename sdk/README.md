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

- REST API: `http://127.0.0.1:32145`
- WebSocket: `ws://127.0.0.1:32145/v1/ws`

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
});

await ws.connect();

// Connection events
ws.on('connected', () => console.log('Connected!'));
ws.on('disconnected', () => console.log('Disconnected'));
ws.on('error', (err) => console.error('Error:', err));

// Disconnect when done
ws.disconnect();
```

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
  console.log('Card Type:', event.card.type);
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

// Get version info
const version = await ws.getVersion();
console.log('Agent version:', version.version);

// Health check
const health = await ws.health();
console.log('Status:', health.status);
```

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
  type?: string;
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
