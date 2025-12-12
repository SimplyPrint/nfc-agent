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
- **JavaScript SDK** - Full-featured TypeScript SDK for browser and Node.js

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
- `version` - Get version and update info (same response as HTTP endpoint)

**Events:**
- `card_detected` - Card placed on reader
- `card_removed` - Card removed from reader

See the [SDK documentation](sdk/README.md) for detailed API reference.

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

See the full [SDK documentation](sdk/README.md) for more examples.

## OpenPrintTag Support

NFC Agent has native support for [OpenPrintTag](https://openprinttag.org), an open standard for encoding filament/material information on NFC tags. This enables 3D printers and software to automatically identify materials from spool NFC tags.

### Reading OpenPrintTag Cards

When reading a card with OpenPrintTag data (MIME type `application/vnd.openprinttag`), the response includes parsed filament information:

```json
{
  "uid": "04:A2:B3:C4:D5:E6:07",
  "type": "NTAG215",
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
- [SDK Documentation](sdk/README.md)
- [SimplyPrint](https://simplyprint.io)
- [Report Issues](https://github.com/SimplyPrint/nfc-agent/issues)
