# NFC Agent

A local HTTP/WebSocket server that enables web applications to communicate with NFC card readers.

[![CI](https://github.com/SimplyPrint/nfc-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/SimplyPrint/nfc-agent/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)](https://github.com/SimplyPrint/nfc-agent/releases)

---

## What is NFC Agent?

NFC Agent is a **local API service** that bridges your NFC reader hardware to applications via HTTP and WebSocket. It doesn't do anything on its own — it's the bridge that lets software communicate with your NFC hardware.

### For SimplyPrint users

**Just install and keep it running!** [SimplyPrint](https://simplyprint.io) will automatically detect and use NFC Agent to read and write filament NFC tags directly from your browser—no additional configuration required. This enables computers that don't normally support NFC (desktops, laptops without built-in NFC) to work with NFC filament tags through a USB reader.

### For developers

NFC Agent provides a fully-featured HTTP/WebSocket API that you can use from any programming language to interact with NFC readers. Read cards, write NDEF data, handle real-time card events, and more. See the [API documentation](#api-overview) and [SDK](#javascript-sdk) below.

## Features

- **Cross-platform** - Works on Windows, macOS, and Linux
- **Dual API** - HTTP REST API for simple operations, WebSocket for real-time events
- **NDEF Support** - Read and write URL, text, JSON, and binary data
- **OpenPrintTag** - Native support for the [OpenPrintTag](https://openprinttag.org) filament NFC standard
- **Multiple Records** - Write multiple NDEF records in a single operation
- **Security** - Password protection for NTAG chips, permanent card locking
- **System Tray** - Native system tray integration with status indicator
- **Auto-start** - Install as a system service for automatic startup
- **Official SDKs** - Full-featured SDKs for [JavaScript/TypeScript](sdk/javascript/) and [Python](sdk/python/)

## Supported Hardware

NFC Agent works with any **PC/SC compatible** contactless smart card reader. Tested and recommended:

| Manufacturer | Models | Buy |
|--------------|--------|-----|
| **ACS** | ACR122U, ACR1252U, ACR1255U-J1 (Bluetooth), **ACR1552U** | [ACR122U](https://amzn.to/48NzOkF), [ACR1252U](https://amzn.to/48Ne1tb), [ACR1255U-J1](https://amzn.to/3KNB1Aj), [ACR1552U](https://amzn.to/48KvK4v) |
| **SCM Microsystems** | SCR3310 | — |
| **Identiv** | uTrust series | — |
| **HID Global** | OMNIKEY series | — |

> **Recommended: [ACR1552U](https://amzn.to/48KvK4v)** — Supports the widest range of tags including ISO 15693 (NFC-V/SLIX2), which is required for [OpenPrintTag](https://openprinttag.org) filament tags.

<sub>Amazon links are affiliate links.</sub>

### Supported Card Types

- NTAG (213, 215, 216, 424 DNA)
- MIFARE Classic, Ultralight, DESFire
- ISO 14443 Type A/B
- FeliCa

### Proxmark3 Support (Experimental)

NFC Agent also supports the **[Proxmark3](https://github.com/RfidResearchGroup/proxmark3)** RFID research tool as an alternative reader. This is experimental and requires the Proxmark3 Iceman fork client to be installed.

**Prerequisites:**
1. Install the Proxmark3 Iceman fork: https://github.com/RfidResearchGroup/proxmark3
2. Ensure `pm3` is in your PATH (test with `pm3 -c "hw version"`)

**Enable Proxmark3 support:**
```bash
NFC_AGENT_PROXMARK3=1 nfc-agent
```

**Environment Variables:**

| Variable | Description |
|----------|-------------|
| `NFC_AGENT_PROXMARK3` | Set to `1` to enable Proxmark3 detection |
| `NFC_AGENT_PM3_PATH` | Custom path to `pm3` binary |
| `NFC_AGENT_PM3_PORT` | Specific serial port (e.g., `/dev/ttyACM0`) |

**Limitations:**
- ~1-2 second latency per command (pm3 client startup overhead)
- Polling frequency should be reduced for Proxmark3 readers
- Not all operations are supported (write operations may be limited)

**Troubleshooting:**
- Ensure the Proxmark3 device is connected: `pm3 -c "hw version"`
- Check serial port permissions on Linux: `sudo usermod -a -G dialout $USER`
- If device not detected, specify the port: `NFC_AGENT_PM3_PORT=/dev/ttyACM0`

## Installation

### macOS

**Option 1: Download DMG**

Download the latest `.dmg` from [GitHub Releases](https://github.com/SimplyPrint/nfc-agent/releases).

**Option 2: Homebrew**

```bash
brew install simplyprint/tap/nfc-agent
```

### Windows

Download the installer (`.exe`) from [GitHub Releases](https://github.com/SimplyPrint/nfc-agent/releases) and run it.

### Linux

**Debian/Ubuntu/Raspberry Pi OS:**

```bash
# Download the .deb package from GitHub Releases, then:
sudo apt install ./NFC-Agent-*.deb

# Install PC/SC daemon if not already installed
sudo apt install pcscd
sudo systemctl enable --now pcscd
```

**Fedora/RHEL:**

```bash
# Download the .rpm package from GitHub Releases, then:
sudo dnf install ./NFC-Agent-*.rpm

# Install PC/SC daemon
sudo dnf install pcsc-lite
sudo systemctl enable --now pcscd
```

> **Atomic Distributions (Fedora Silverblue, Kinoite, etc.):** You can run NFC Agent inside a distrobox container. Install the `.rpm` package in the container, apply the kernel module fix on the host OS, then run `nfc-agent` from the container.

**Arch Linux / Other:**

```bash
# Download the tar.gz archive from GitHub Releases
tar -xzf nfc-agent_*_linux_amd64.tar.gz
sudo mv nfc-agent /usr/local/bin/

# Install PC/SC daemon and drivers
sudo pacman -S pcsclite ccid

# For ACS readers (ACR122U, ACR1252U, etc.), install the ACS-specific driver from AUR:
yay -S acsccid   # or: paru -S acsccid

sudo systemctl enable --now pcscd
```

> **Note:** Unlike the `.deb` and `.rpm` packages, the tar.gz installation requires manual setup of the PC/SC daemon. If you see "No readers found", ensure `pcscd` is running (`systemctl status pcscd`).

**Important: Kernel Module Fix (All Distributions)**

The Linux kernel's NFC subsystem may claim certain readers (especially ACR122U) before pcscd can access them. Run the following after installation:

```bash
# Blacklist conflicting kernel modules
echo -e "blacklist pn533_usb\nblacklist pn533\nblacklist nfc" | sudo tee /etc/modprobe.d/blacklist-nfc-pn533.conf

# Unload modules if currently loaded
sudo modprobe -r pn533_usb pn533 nfc 2>/dev/null || true

# Restart PC/SC daemon
sudo systemctl restart pcscd
```

## Quick Start

1. **Connect** your NFC reader via USB
2. **Run** the NFC Agent:
   ```bash
   nfc-agent
   ```
3. **Open** http://127.0.0.1:32145 in your browser to see the status page
4. **Place** an NFC card on the reader - the web interface will display card information

## CLI Usage

```bash
nfc-agent              # Run with system tray
nfc-agent --no-tray    # Run in headless mode (servers, scripts)
nfc-agent install      # Install auto-start service
nfc-agent uninstall    # Remove auto-start service
nfc-agent version      # Show version information
```

## Configuration

Configure via environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `NFC_AGENT_PORT` | `32145` | HTTP/WebSocket server port |
| `NFC_AGENT_HOST` | `127.0.0.1` | Server bind address |
| `NFC_AGENT_PROXMARK3` | `0` | Set to `1` to enable Proxmark3 support |
| `NFC_AGENT_PM3_PATH` | `pm3` | Path to Proxmark3 client binary |
| `NFC_AGENT_PM3_PORT` | (auto) | Specific serial port for Proxmark3 |

## API Overview

### HTTP Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/v1/readers` | List connected readers |
| `GET` | `/v1/readers/{n}/card` | Read card on reader N |
| `POST` | `/v1/readers/{n}/card` | Write data to card |
| `POST` | `/v1/readers/{n}/erase` | Erase card data |
| `POST` | `/v1/readers/{n}/lock` | Lock card (permanent!) |
| `POST` | `/v1/readers/{n}/password` | Set password protection |
| `DELETE` | `/v1/readers/{n}/password` | Remove password |
| `POST` | `/v1/readers/{n}/records` | Write multiple NDEF records |
| `GET` | `/v1/readers/{n}/mifare/{block}` | Read MIFARE Classic block |
| `POST` | `/v1/readers/{n}/mifare/{block}` | Write MIFARE Classic block |
| `POST` | `/v1/readers/{n}/mifare/batch` | Write multiple MIFARE Classic blocks |
| `GET` | `/v1/readers/{n}/ultralight/{page}` | Read MIFARE Ultralight page |
| `POST` | `/v1/readers/{n}/ultralight/{page}` | Write MIFARE Ultralight page |
| `POST` | `/v1/readers/{n}/mifare/derive-key` | Derive 6-byte key from UID via AES |
| `POST` | `/v1/readers/{n}/mifare/aes-write/{block}` | AES encrypt + write block |
| `POST` | `/v1/readers/{n}/mifare/sector-trailer/{block}` | Write sector trailer with keys and access bits |
| `GET` | `/v1/supported-readers` | List supported reader models |
| `GET` | `/v1/version` | Get version and update info |
| `GET` | `/v1/health` | Health check |

#### Version Endpoint

The `/v1/version` endpoint returns version information and checks for available updates:

```json
{
  "version": "1.2.3",
  "buildTime": "2025-01-15T10:30:00Z",
  "gitCommit": "abc123def456...",
  "updateAvailable": true,
  "latestVersion": "1.3.0",
  "releaseUrl": "https://github.com/SimplyPrint/nfc-agent/releases/tag/v1.3.0"
}
```

The `updateAvailable`, `latestVersion`, and `releaseUrl` fields are only present when the agent has checked for updates.

### WebSocket

Connect to `ws://127.0.0.1:32145/v1/ws` for real-time card events.

**Message Types:**
- `list_readers` - Get connected readers
- `read_card` - Read card data
- `write_card` - Write data to card
- `subscribe` / `unsubscribe` - Real-time card detection
- `erase_card`, `lock_card`, `set_password`, `remove_password`
- `read_mifare_block`, `write_mifare_block`, `write_mifare_blocks` - Raw MIFARE Classic block access
- `read_ultralight_page`, `write_ultralight_page`, `write_ultralight_pages` - Raw MIFARE Ultralight page access
- `derive_uid_key_aes` - Derive 6-byte key from UID via AES
- `aes_encrypt_and_write_block` - AES encrypt + write MIFARE block
- `write_mifare_sector_trailer` - Write sector trailer with keys and access bits
- `version` - Get version and update info (same response as HTTP endpoint)

**Events:**
- `card_detected` - Card placed on reader
- `card_removed` - Card removed from reader

See the SDK documentation for detailed API reference: [JavaScript](sdk/javascript/README.md) | [Python](sdk/python/README.md)

## JavaScript SDK

Install the official SDK for browser and Node.js:

```bash
# Add to .npmrc (one-time setup)
echo "@simplyprint:registry=https://npm.pkg.github.com" >> .npmrc

# Install
npm install @simplyprint/nfc-agent
```

### Quick Example

```typescript
import { NFCAgentWebSocket } from '@simplyprint/nfc-agent';

const ws = new NFCAgentWebSocket();
await ws.connect();

// Subscribe to real-time card events
await ws.subscribe(0);

ws.on('card_detected', (event) => {
  console.log('Card UID:', event.card.uid);
  console.log('Card Type:', event.card.type);
  console.log('Data:', event.card.data);
});

// Write data to card
await ws.writeCard(0, {
  data: 'https://example.com',
  dataType: 'url'
});
```

See the full [SDK documentation](sdk/javascript/README.md) for more examples.

## Python SDK

Install the official Python SDK:

```bash
pip install nfc-agent
```

### Quick Example

```python
import asyncio
from nfc_agent import NFCClient, NFCWebSocket

async def main():
    # REST API
    async with NFCClient() as client:
        readers = await client.get_readers()
        card = await client.read_card(0)
        print(f"Card UID: {card.uid}")

    # WebSocket for real-time events
    async with NFCWebSocket() as ws:
        await ws.subscribe(0)

        @ws.on_card_detected
        def handle_card(event):
            print(f"Card: {event.card.uid}")

        await asyncio.sleep(60)

asyncio.run(main())
```

See the full [Python SDK documentation](sdk/python/README.md) for more examples.

## MIFARE Classic Raw Block Access

For direct block-level access to MIFARE Classic cards (e.g., for proprietary tag formats like QIDI BOX filament tags):

### HTTP API

**Read block 4:**
```bash
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4"
```

**Read with specific key:**
```bash
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=D3F7D3F7D3F7&keyType=A"
```

**Write block 4:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/4 \
  -H "Content-Type: application/json" \
  -d '{"data": "01120100000000000000000000000000", "key": "D3F7D3F7D3F7"}'
```

### JavaScript SDK

```typescript
// Read block
const block = await client.readMifareBlock(0, 4, { key: 'D3F7D3F7D3F7' });
console.log(block.data); // "01120100000000000000000000000000"

// Write block
await client.writeMifareBlock(0, 4, {
  data: '01120100000000000000000000000000',
  key: 'D3F7D3F7D3F7'
});
```

### Authentication Keys

If no key is provided, the agent tries these default keys in order:
- `FFFFFFFFFFFF` - Default transport key
- `D3F7D3F7D3F7` - NFC Forum default
- `A0A1A2A3A4A5` - MAD key
- `000000000000` - Zero key

### Block Restrictions

- **Sector trailers** (blocks 3, 7, 11, 15, etc.) cannot be read or written - they contain authentication keys
- Block numbers: 0-63 for MIFARE Classic 1K, 0-255 for MIFARE Classic 4K
- Each block is 16 bytes (32 hex characters)

### Batch Write Multiple Blocks

Write multiple blocks efficiently in a single card session. Automatically re-authenticates when crossing sector boundaries.

**HTTP:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/batch \
  -H "Content-Type: application/json" \
  -d '{
    "blocks": [
      {"block": 4, "data": "00112233445566778899AABBCCDDEEFF"},
      {"block": 5, "data": "FFEEDDCCBBAA99887766554433221100"}
    ],
    "key": "FFFFFFFFFFFF",
    "keyType": "A"
  }'
```

**Response:**
```json
{
  "results": [
    {"block": 4, "success": true, "error": ""},
    {"block": 5, "success": true, "error": ""}
  ],
  "written": 2,
  "total": 2
}
```

**JavaScript SDK:**
```typescript
const result = await client.writeMifareBlocks(0, {
  blocks: [
    { block: 4, data: '00112233445566778899AABBCCDDEEFF' },
    { block: 5, data: 'FFEEDDCCBBAA99887766554433221100' }
  ],
  key: 'FFFFFFFFFFFF',
  keyType: 'A'
});
console.log(`Wrote ${result.written}/${result.total} blocks`);
```

## MIFARE Ultralight Raw Page Access

For direct page-level access to MIFARE Ultralight cards:

### HTTP API

**Read page 4 (first user data page):**
```bash
curl "http://127.0.0.1:32145/v1/readers/0/ultralight/4"
```

**Read with password (EV1 cards only):**
```bash
curl "http://127.0.0.1:32145/v1/readers/0/ultralight/4?password=12345678"
```

**Write page 4:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/ultralight/4 \
  -H "Content-Type: application/json" \
  -d '{"data": "DEADBEEF"}'
```

**Write with password:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/ultralight/4 \
  -H "Content-Type: application/json" \
  -d '{"data": "DEADBEEF", "password": "12345678"}'
```

### JavaScript SDK

```typescript
// Read page
const page = await client.readUltralightPage(0, 4);
console.log(page.data); // "DEADBEEF" (8 hex chars = 4 bytes)

// Read with password (EV1)
const page = await client.readUltralightPage(0, 4, { password: '12345678' });

// Write page
await client.writeUltralightPage(0, 4, { data: 'DEADBEEF' });

// Write with password
await client.writeUltralightPage(0, 4, { data: 'DEADBEEF', password: '12345678' });
```

### Memory Layout

| Pages | Contents | Notes |
|-------|----------|-------|
| 0-1 | UID | Read-only |
| 2 | Lock bytes | Writing locks pages permanently! |
| 3 | OTP / Capability Container | OTP bits are irreversible |
| 4+ | User data | Safe to read/write |
| Last 4-5 | Config/Password (EV1) | Varies by variant |

### Page Restrictions

- Pages 0-3 are blocked for writing to prevent accidental damage
- Each page is 4 bytes (8 hex characters)
- Page count varies by variant: Ultralight (16 pages), Ultralight C (48 pages), Ultralight EV1 (varies)

### Password Protection (EV1 only)

MIFARE Ultralight EV1 supports password protection:
- Password is 4 bytes (8 hex characters)
- Use `password` parameter when accessing protected pages

## AES-Encrypted MIFARE Classic Operations

For MIFARE Classic tags that require AES-encrypted data (e.g., certain third-party filament spool tags):

### Derive UID Key via AES

Derives a 6-byte MIFARE sector key from the card's 4-byte UID using AES-128-ECB encryption.

**HTTP:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/derive-key \
  -H "Content-Type: application/json" \
  -d '{"aesKey": "713362755e74316e71665a2870662431"}'
```

**Response:**
```json
{"key": "abc123def456"}
```

**JavaScript SDK:**
```typescript
const derived = await ws.deriveUIDKeyAES(0, {
  aesKey: '713362755e74316e71665a2870662431'  // 16 bytes as hex (32 chars)
});
console.log('Derived key:', derived.key);  // 6 bytes as hex (12 chars)
```

### AES Encrypt and Write Block

Encrypts 16 bytes of data with AES-128-ECB and writes to a MIFARE Classic block.

**HTTP:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/aes-write/4 \
  -H "Content-Type: application/json" \
  -d '{
    "data": "30303030303030303030303030303030",
    "aesKey": "484043466b526e7a404b4174424a7032",
    "authKey": "FFFFFFFFFFFF",
    "authKeyType": "A"
  }'
```

**JavaScript SDK:**
```typescript
await ws.aesEncryptAndWriteBlock(0, 4, {
  data: '30303030303030303030303030303030',  // 16 bytes plaintext (will be encrypted)
  aesKey: '484043466b526e7a404b4174424a7032',  // AES encryption key
  authKey: 'FFFFFFFFFFFF',                      // MIFARE auth key
  authKeyType: 'A'
});
```

### Write Sector Trailer

Writes a MIFARE Classic sector trailer with new keys and optional access bits.

**HTTP:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/sector-trailer/7 \
  -H "Content-Type: application/json" \
  -d '{
    "keyA": "abc123def456",
    "keyB": "abc123def456",
    "accessBits": "FF0780",
    "authKey": "FFFFFFFFFFFF",
    "authKeyType": "A"
  }'
```

**JavaScript SDK:**
```typescript
await ws.writeMifareSectorTrailer(0, 7, {
  keyA: derived.key,       // New Key A
  keyB: derived.key,       // New Key B
  accessBits: 'FF0780',    // Optional - preserves existing if omitted
  authKey: 'FFFFFFFFFFFF', // Current auth key
  authKeyType: 'A'
});
```

### Parameter Reference

| Parameter | Size | Description |
|-----------|------|-------------|
| `aesKey` | 32 hex chars (16 bytes) | AES-128 encryption key |
| `authKey` | 12 hex chars (6 bytes) | MIFARE sector authentication key |
| `keyA`, `keyB` | 12 hex chars (6 bytes) | New sector trailer keys |
| `accessBits` | 6 or 8 hex chars (3-4 bytes) | Access bits (optional, preserves existing if omitted) |
| `data` | 32 hex chars (16 bytes) | Block data to encrypt/write |
| `authKeyType` | `"A"` or `"B"` | Key type for authentication |

**Notes:**
- Sector trailers are at blocks 3, 7, 11, 15, etc. (for MIFARE Classic 1K)
- The derived key from `derive-key` is suitable for MIFARE authentication
- Data is encrypted with AES before being written to the card
- `FF0780` is the standard "transport" access bits configuration

## OpenPrintTag Support

NFC Agent has native support for [OpenPrintTag](https://openprinttag.org), an open standard for encoding filament/material information on NFC tags. This enables 3D printers and software to automatically identify materials from spool NFC tags.

### Reading OpenPrintTag Cards

When reading a card with OpenPrintTag data (MIME type `application/vnd.openprinttag`), the response includes parsed filament information:

```json
{
  "uid": "04:A2:B3:C4:D5:E6:07",
  "type": "NTAG215",
  "protocol": "NFC-A",
  "protocolISO": "ISO 14443-3A",
  "dataType": "openprinttag",
  "data": {
    "materialName": "PLA Galaxy Black",
    "brandName": "Prusament",
    "materialClass": "FFF",
    "materialType": "PLA",
    "primaryColor": "#1A1A1A",
    "nominalWeight": 1000,
    "remainingWeight": 750,
    "filamentDiameter": 1.75,
    "minPrintTemp": 215,
    "maxPrintTemp": 230
  }
}
```

### Writing OpenPrintTag Cards

Use `dataType: "openprinttag"` with JSON material data:

**HTTP:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/card \
  -H "Content-Type: application/json" \
  -d '{
    "dataType": "openprinttag",
    "data": {
      "materialName": "PLA Galaxy Black",
      "brandName": "Prusament",
      "materialClass": 0,
      "materialType": 0,
      "nominalWeight": 1000,
      "primaryColor": "#1A1A1A",
      "minPrintTemp": 215,
      "maxPrintTemp": 230
    }
  }'
```

**WebSocket:**
```json
{
  "type": "write_card",
  "id": "1",
  "reader": 0,
  "dataType": "openprinttag",
  "data": {
    "materialName": "PETG Orange",
    "brandName": "Prusa",
    "materialClass": 0,
    "materialType": 2,
    "nominalWeight": 1000
  }
}
```

### OpenPrintTag Fields

| Field | Type | Description |
|-------|------|-------------|
| `materialName` | string | Material display name (required) |
| `brandName` | string | Brand/manufacturer name (required) |
| `materialClass` | int | 0 = FFF (filament), 1 = SLA (resin) |
| `materialType` | int | Material type (0=PLA, 1=ABS, 2=PETG, etc.) |
| `nominalWeight` | float | Nominal weight in grams (required) |
| `primaryColor` | string | Hex color code (#RRGGBB or #RRGGBBAA) |
| `filamentDiameter` | float | Diameter in mm (default: 1.75) |
| `density` | float | Material density in g/cm³ |
| `minPrintTemp` | int | Minimum print temperature °C |
| `maxPrintTemp` | int | Maximum print temperature °C |
| `manufacturedDate` | int | Unix timestamp |
| `expirationDate` | int | Unix timestamp |

See the [OpenPrintTag specification](https://openprinttag.org) for the complete field reference.

## Building from Source

### Prerequisites

- Go 1.22 or later
- PC/SC development libraries

**macOS:**
```bash
# PC/SC is included in macOS
```

**Linux:**
```bash
sudo apt install libpcsclite-dev  # Debian/Ubuntu
sudo dnf install pcsc-lite-devel  # Fedora/RHEL
```

**Windows:**
```bash
# PC/SC (WinSCard) is included in Windows
```

### Build

```bash
git clone https://github.com/SimplyPrint/nfc-agent.git
cd nfc-agent
go build -o nfc-agent ./cmd/nfc-agent
```

### Run Tests

```bash
go test -v ./...
```

## How It Works

NFC Agent uses the PC/SC (Personal Computer/Smart Card) interface to communicate with NFC readers. When a web application needs to interact with an NFC card:

1. Web app connects to NFC Agent via HTTP or WebSocket
2. NFC Agent sends commands to the reader via PC/SC
3. Reader communicates with the NFC card
4. Response flows back through NFC Agent to the web app

```
┌───────────┐      HTTP/WS       ┌───────────┐      PC/SC       ┌────────┐      NFC       ┌──────┐
│  Web App  │◄─────────────────► │ NFC Agent │◄────────────────►│ Reader │◄──────────────►│ Card │
└───────────┘    localhost:32145 └───────────┘                  └────────┘                └──────┘
```

## Security Considerations

- NFC Agent binds to `127.0.0.1` by default (localhost only)
- No authentication is required for local connections
- For production deployments, consider running behind a reverse proxy with authentication
- Card locking is **permanent and irreversible** - use with caution

## Troubleshooting

### "No readers found"

1. Ensure your NFC reader is connected via USB
2. Check if the PC/SC daemon is running:
   ```bash
   # Linux
   sudo systemctl status pcscd

   # macOS - should work automatically
   ```
3. Try unplugging and reconnecting the reader

**Linux: Kernel NFC modules conflict**

The Linux kernel's NFC subsystem may claim certain readers before pcscd can access them. Check with `lsmod | grep pn533`—if modules are loaded, follow the **Kernel Module Fix** steps in the [Linux installation section](#linux).

**Arch Linux with ACS readers:** Install the `acsccid` driver from AUR (`yay -S acsccid`) and restart pcscd.

### "Failed to connect to card"

1. Ensure the card is placed correctly on the reader
2. Some readers have an LED that indicates card detection
3. Try a different card to rule out card issues

### Linux: "Rejected unauthorized PC/SC client"

On modern Linux distributions (Fedora, Silverblue, etc.), PC/SC access is controlled by Polkit. NFC Agent must run as part of your graphical session to be authorized.

**Solution:** Run `nfc-agent` directly from your terminal or use `nfc-agent install` to set up XDG autostart (starts automatically when you log in to your desktop).

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

This project is licensed under the **GNU Affero General Public License v3.0** - see the [LICENSE](LICENSE) file for details.

## Credits

Built with care by [SimplyPrint](https://simplyprint.io) - Cloud-based 3D print management.

---

**Links:**
- [GitHub Repository](https://github.com/SimplyPrint/nfc-agent)
- [SDK Documentation](sdk/)
- [SimplyPrint](https://simplyprint.io)
- [Report Issues](https://github.com/SimplyPrint/nfc-agent/issues)
