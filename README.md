# NFC Agent

A local HTTP/WebSocket server that enables web applications to communicate with NFC card readers.

[![CI](https://github.com/SimplyPrint/nfc-agent/actions/workflows/ci.yml/badge.svg)](https://github.com/SimplyPrint/nfc-agent/actions/workflows/ci.yml)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Windows%20%7C%20macOS%20%7C%20Linux-lightgrey)](https://github.com/SimplyPrint/nfc-agent/releases)

---

Built by [SimplyPrint](https://simplyprint.io) for enabling NFC tag interactions in web-based 3D printing workflows. Designed as a general-purpose NFC gateway that any application can use to read and write NFC cards through a simple API.

## Features

- **Cross-platform** - Works on Windows, macOS, and Linux
- **Dual API** - HTTP REST API for simple operations, WebSocket for real-time events
- **NDEF Support** - Read and write URL, text, JSON, and binary data
- **Multiple Records** - Write multiple NDEF records in a single operation
- **Security** - Password protection for NTAG chips, permanent card locking
- **System Tray** - Native system tray integration with status indicator
- **Auto-start** - Install as a system service for automatic startup
- **JavaScript SDK** - Full-featured TypeScript SDK for browser and Node.js

## Supported Hardware

NFC Agent works with any **PC/SC compatible** contactless smart card reader. Tested and recommended:

| Manufacturer | Models |
|--------------|--------|
| **ACS** | ACR122U, ACR1252U, ACR1255U-J1 (Bluetooth), ACR1552U |
| **SCM Microsystems** | SCR3310 |
| **Identiv** | uTrust series |
| **HID Global** | OMNIKEY series |

### Supported Card Types

- NTAG (213, 215, 216)
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

**Debian/Ubuntu:**

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

**Atomic Linux Distributions (Fedora Silverblue, Kinoite, etc.) with Distrobox:**

For immutable/atomic distributions, you can run NFC Agent inside a distrobox container while applying kernel fixes on the host OS:

1. **On the host OS** - Apply the kernel module blacklist (required for ACR122U and similar readers):
   ```bash
   # Create blacklist file on the host
   echo -e "blacklist pn533_usb\nblacklist pn533\nblacklist nfc" | sudo tee /etc/modprobe.d/blacklist-pn533.conf

   # Unload modules if currently loaded
   sudo modprobe -r pn533_usb pn533 nfc 2>/dev/null || true

   # Restart PC/SC daemon on host
   sudo systemctl restart pcscd
   ```

2. **Inside distrobox** - Install the package:
   ```bash
   # Enter your distrobox container (e.g., Fedora-based)
   distrobox enter fedora

   # Install the RPM package
   sudo dnf install ./NFC-Agent-*.rpm
   ```

3. **Run from distrobox** - Start NFC Agent from within the container:
   ```bash
   distrobox enter fedora
   nfc-agent
   ```

> **Note:** The kernel module blacklist must be applied on the host OS because the kernel is shared between the host and containers. The pcscd service also runs on the host and is accessible from within distrobox.

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
| `GET` | `/v1/version` | Get version info |
| `GET` | `/v1/health` | Health check |

### WebSocket

Connect to `ws://127.0.0.1:32145/v1/ws` for real-time card events.

**Message Types:**
- `list_readers` - Get connected readers
- `read_card` - Read card data
- `write_card` - Write data to card
- `subscribe` / `unsubscribe` - Real-time card detection
- `erase_card`, `lock_card`, `set_password`, `remove_password`

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

**Linux: Kernel NFC modules conflict (ACR122U)**

The Linux kernel's NFC subsystem (`pn533_usb`) may claim ACR122U readers before pcscd can access them. Check with:
```bash
lsmod | grep pn533
```

If modules are loaded, unload them and blacklist:
```bash
# Temporary fix
sudo modprobe -r pn533_usb pn533 nfc
sudo systemctl restart pcscd

# Permanent fix - create blacklist file
echo -e "blacklist pn533_usb\nblacklist pn533\nblacklist nfc" | sudo tee /etc/modprobe.d/blacklist-pn533.conf
```

**Arch Linux with ACS readers:** Install the `acsccid` driver from AUR:
```bash
yay -S acsccid
sudo systemctl restart pcscd
```

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
