# NFC Agent SDKs

Official SDKs for interacting with the NFC Agent local server.

## Available SDKs

| Language | Package | Documentation |
|----------|---------|---------------|
| **JavaScript/TypeScript** | [`@simplyprint/nfc-agent`](https://github.com/SimplyPrint/nfc-agent/pkgs/npm/nfc-agent) | [Documentation](javascript/README.md) |
| **Python** | [`nfc-agent`](https://pypi.org/project/nfc-agent/) | [Documentation](python/README.md) |

## Quick Start

### JavaScript/TypeScript

```bash
npm install @simplyprint/nfc-agent
```

```typescript
import { NFCAgentClient } from '@simplyprint/nfc-agent';

const client = new NFCAgentClient();
const card = await client.readCard(0);
console.log(`Card UID: ${card.uid}`);
```

### Python

```bash
pip install nfc-agent
```

```python
from nfc_agent import NFCClient

async with NFCClient() as client:
    card = await client.read_card(0)
    print(f"Card UID: {card.uid}")
```

## API Overview

Both SDKs provide two client interfaces:

### REST Client
Simple request/response pattern for basic operations.
- `get_readers()` / `getReaders()` - List available readers
- `read_card()` / `readCard()` - Read card data
- `write_card()` / `writeCard()` - Write NDEF data

### WebSocket Client
Real-time event-driven communication with persistent connection.
- Card detection events
- Card removal events
- Auto-reconnection
- All REST operations plus subscriptions

## Features

- **NDEF Support** - Read/write text, URLs, JSON, and binary data
- **MIFARE Classic** - Block-level read/write with authentication
- **MIFARE Ultralight/NTAG** - Page-level operations with password protection
- **AES Encryption** - UID-based key derivation and encrypted writes
- **Real-time Events** - WebSocket subscriptions for card detection

## HTTP API

Don't want to use an SDK? The NFC Agent exposes a full HTTP/WebSocket API:

- **Base URL:** `http://127.0.0.1:32145`
- **WebSocket:** `ws://127.0.0.1:32145/v1/ws`

See the [main documentation](../README.md#api-overview) for the complete API reference.

## Versioning

Both SDKs follow the same version numbering. Use matching SDK versions for compatibility.

| SDK Version | NFC Agent Version |
|-------------|-------------------|
| 0.5.x | 0.5.x+ |
