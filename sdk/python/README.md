# NFC Agent Python SDK

Python SDK for interacting with NFC readers via the [nfc-agent](https://github.com/SimplyPrint/nfc-agent) local server.

## Installation

```bash
pip install nfc-agent
```

## Requirements

- Python 3.9+
- Running [nfc-agent](https://github.com/SimplyPrint/nfc-agent) server

## Quick Start

### REST API (Simple Operations)

```python
import asyncio
from nfc_agent import NFCClient

async def main():
    async with NFCClient() as client:
        # List readers
        readers = await client.get_readers()
        print(f"Found {len(readers)} reader(s)")

        # Read card
        try:
            card = await client.read_card(0)
            print(f"Card UID: {card.uid}")
            print(f"Card Type: {card.type}")
            if card.data:
                print(f"Data: {card.data}")
        except Exception as e:
            print(f"No card present: {e}")

asyncio.run(main())
```

### Synchronous Usage

```python
from nfc_agent import NFCClient

with NFCClient() as client:
    readers = client.get_readers_sync()
    card = client.read_card_sync(0)
    print(f"Card UID: {card.uid}")
```

### WebSocket (Real-time Events)

```python
import asyncio
from nfc_agent import NFCWebSocket

async def main():
    async with NFCWebSocket() as ws:
        # Subscribe to reader events
        await ws.subscribe(0)

        @ws.on_card_detected
        def handle_card(event):
            print(f"Card detected: {event.card.uid}")

        @ws.on_card_removed
        def handle_removed(event):
            print(f"Card removed from reader {event.reader}")

        # Keep running
        await asyncio.sleep(60)

asyncio.run(main())
```

### Card Polling

```python
import asyncio
from nfc_agent import NFCClient

async def main():
    async with NFCClient() as client:
        poller = client.poll_card(0, interval=0.5)

        @poller.on_card
        def handle_card(card):
            print(f"Card: {card.uid}")

        @poller.on_removed
        def handle_removed():
            print("Card removed")

        await poller.start()
        await asyncio.sleep(30)
        poller.stop()

asyncio.run(main())
```

## API Reference

### NFCClient

REST API client for simple request/response operations.

```python
from nfc_agent import NFCClient

# With context manager (recommended)
async with NFCClient(base_url="http://127.0.0.1:32145", timeout=5.0) as client:
    ...

# Or sync
with NFCClient() as client:
    ...
```

#### Methods

| Method | Description |
|--------|-------------|
| `get_readers()` | List available NFC readers |
| `read_card(reader_index)` | Read card data from a reader |
| `write_card(reader_index, *, data, data_type, url)` | Write data to a card |
| `get_version()` | Get agent version information |
| `is_connected()` | Check if agent is running |
| `poll_card(reader_index, *, interval)` | Create a card poller |

##### MIFARE Classic

| Method | Description |
|--------|-------------|
| `read_mifare_block(reader_index, block, *, key, key_type)` | Read 16-byte block |
| `write_mifare_block(reader_index, block, *, data, key, key_type)` | Write 16-byte block |
| `write_mifare_blocks(reader_index, blocks, *, key, key_type)` | Batch write blocks |
| `derive_uid_key_aes(reader_index, aes_key)` | Derive key from UID |
| `aes_encrypt_and_write_block(...)` | AES encrypt and write |
| `write_mifare_sector_trailer(...)` | Write sector trailer |

##### MIFARE Ultralight / NTAG

| Method | Description |
|--------|-------------|
| `read_ultralight_page(reader_index, page, *, password)` | Read 4-byte page |
| `write_ultralight_page(reader_index, page, *, data, password)` | Write 4-byte page |
| `write_ultralight_pages(reader_index, pages, *, password)` | Batch write pages |

### NFCWebSocket

WebSocket client for real-time communication and events.

```python
from nfc_agent import NFCWebSocket

async with NFCWebSocket(
    url="ws://127.0.0.1:32145/v1/ws",
    timeout=5.0,
    auto_reconnect=True,
    reconnect_interval=3.0,
    secure=False  # Use wss:// for HTTPS pages
) as ws:
    ...
```

#### Methods

All methods from NFCClient, plus:

| Method | Description |
|--------|-------------|
| `subscribe(reader_index)` | Subscribe to card events |
| `unsubscribe(reader_index)` | Unsubscribe from events |
| `erase_card(reader_index)` | Erase NDEF data |
| `lock_card(reader_index)` | Permanently lock card |
| `set_password(reader_index, password)` | Set NTAG password |
| `remove_password(reader_index, password)` | Remove NTAG password |
| `write_records(reader_index, records)` | Write multiple NDEF records |
| `health()` | Health check |

#### Events

```python
@ws.on_card_detected
def handle_card(event):
    print(event.card.uid)

@ws.on_card_removed
def handle_removed(event):
    print(f"Removed from reader {event.reader}")

@ws.on_connected
def handle_connected():
    print("Connected")

@ws.on_disconnected
def handle_disconnected():
    print("Disconnected")

@ws.on_error
def handle_error(error):
    print(f"Error: {error}")
```

### CardPoller

Polls a reader for card presence.

```python
poller = client.poll_card(reader_index, interval=1.0)

@poller.on_card
def handle_card(card):
    print(card.uid)

@poller.on_removed
def handle_removed():
    print("Removed")

@poller.on_error
def handle_error(e):
    print(f"Error: {e}")

await poller.start()
# ...
poller.stop()
```

### Types

```python
from nfc_agent import (
    Reader,
    Card,
    CardDataType,
    VersionInfo,
    HealthInfo,
    CardDetectedEvent,
    CardRemovedEvent,
    MifareKeyType,
    MifareBlockData,
    UltralightPageData,
    NDEFRecord,
)
```

### Exceptions

```python
from nfc_agent import (
    NFCAgentError,    # Base exception
    ConnectionError,  # Connection failed
    CardError,        # Card operation failed
    APIError,         # API returned error
    TimeoutError,     # Request timed out
    ReaderError,      # Reader issue
)
```

## Examples

### Write URL to Card

```python
async with NFCClient() as client:
    await client.write_card(0, data="https://example.com", data_type="url")
```

### Write Text to Card

```python
async with NFCClient() as client:
    await client.write_card(0, data="Hello World", data_type="text")
```

### Write JSON to Card

```python
import json

async with NFCClient() as client:
    data = json.dumps({"user_id": 123, "name": "Alice"})
    await client.write_card(0, data=data, data_type="json")
```

### Read MIFARE Classic Block

```python
from nfc_agent import MifareKeyType

async with NFCClient() as client:
    block = await client.read_mifare_block(
        0,
        block=4,
        key="FFFFFFFFFFFF",
        key_type=MifareKeyType.A
    )
    print(f"Block {block.block}: {block.data}")
```

### Monitor Multiple Readers

```python
async with NFCWebSocket() as ws:
    # Subscribe to all readers
    readers = await ws.get_readers()
    for i, reader in enumerate(readers):
        await ws.subscribe(i)
        print(f"Subscribed to {reader.name}")

    @ws.on_card_detected
    def handle(event):
        print(f"Reader {event.reader}: {event.card.uid}")

    await asyncio.sleep(300)
```

## Development

```bash
# Install dev dependencies
pip install -e ".[dev]"

# Run tests
pytest

# Lint
ruff check src tests

# Type check
mypy src
```

## License

MIT
