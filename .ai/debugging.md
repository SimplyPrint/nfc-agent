# Reader Debugging Guide

## Quick Start — Is the Agent Running?

```bash
# Check health
curl http://127.0.0.1:32145/v1/health
# → {"status":"ok","readerCount":1}

# List readers
curl http://127.0.0.1:32145/v1/readers
# → [{"id":"0","name":"ACS ACR1252U PICC Reader","type":"picc"}]

# Run the comprehensive health-check script
python3 scripts/debug_api.py

# Inspect a tag on the reader
python3 scripts/inspect_tag.py
```

If the agent isn't running:
```bash
go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent
```

---

## Enable Extended APDU Logging

Extended logging records every raw APDU command and response sent to/from the reader. Essential for debugging detection failures and reader-specific issues.

```bash
# Enable
curl -X POST http://127.0.0.1:32145/v1/settings \
  -H "Content-Type: application/json" \
  -d '{"extendedLogging": true}'

# Read logs (tail style)
curl "http://127.0.0.1:32145/v1/logs?limit=100&level=debug"

# Clear logs
curl -X DELETE http://127.0.0.1:32145/v1/logs
```

In code, extended logging is gated by `settings.IsExtendedLoggingEnabled()` in `internal/settings/`.

---

## Reader Comparison

| Reader | ISO 14443 (NTAG/MIFARE) | ISO 15693 (OpenPrintTag ICode SLIX) | Notes |
|--------|------------------------|-------------------------------------|-------|
| ACR122U | ✓ | ✗ | Common, cheap. Sometimes finicky with rapid command sequences (e.g. GET_VERSION flooding). |
| ACR1252U | ✓ | ✗ | Similar to 122U. State corruption risk after GET_VERSION failures — code includes reconnect workaround. |
| ACR1255U-J1 | ✓ | ✗ | Bluetooth version of 1252U. Same capabilities. |
| ACR1552U | ✓ | ✓ | **Recommended.** Best stress tolerance. Supports ISO 15693 for OpenPrintTag. Better NTAG handling. |

**Only the ACR1552U supports OpenPrintTag (ICode SLIX / SLIX2 tags, ISO 15693 / NFC-V).**

---

## Raw APDU Commands

These are the raw APDU commands the agent sends internally. Useful for low-level debugging or verifying reader behaviour.

All commands follow ISO 7816 format: `[CLA] [INS] [P1] [P2] [Lc] [Data...] [Le]`

### Standard Commands (all readers)

```
Get UID:           FF CA 00 00 00
  Response:        <uid bytes> 90 00

GET_VERSION (v1):  FF 00 00 00 02 60 00    (NTAG type detection, standard passthrough)
GET_VERSION (v2):  FF 00 00 00 01 60       (works on ACR1252U where v1 fails)
  Response:        00 <vendor> <productType> <subtype> <major> <minor> <storage> <protocol> 90 00
  productType:     04 = NTAG family
  storage byte:    0F = NTAG213 (48B), 11 = NTAG215 (496B), 13 = NTAG216 (872B)

Read page (NTAG/Ultralight, 4 bytes):  FF B0 00 <page> 04
  Response:        <4 bytes> 90 00

Read block (MIFARE Classic, 16 bytes): FF B0 00 <block> 10
  Response:        <16 bytes> 90 00

Write page (NTAG/Ultralight):          FF D6 00 <page> 04 <4 bytes>
  Response:        90 00

Write block (MIFARE Classic):          FF D6 00 <block> 10 <16 bytes>
  Response:        90 00

Load MIFARE key:   FF 82 00 00 06 <6 byte key>
  Response:        90 00

Authenticate block: FF 86 00 00 05 01 00 <block> <0x60=Key A | 0x61=Key B> 00
  Response:        90 00 (success) or 63 00 (auth failure)
```

### Success/Error Status Words

```
90 00 = Success
63 00 = Authentication failure (wrong key)
6A 82 = File not found / card not present
6F 00 = Unknown error
```

### NTAG Write Methods (multiple fallbacks in code)

The agent tries multiple write methods because not all readers support all variants:

```
Method 1 — Standard UPDATE BINARY:   FF D6 00 <page> 04 <4 bytes>
Method 2 — Raw NTAG WRITE command:   A2 <page> <4 bytes>
Method 3 — ACR122U passthrough:      FF 00 00 00 08 D4 42 A2 <page> <4 bytes>
```

### Default MIFARE Keys (tried in order when no key specified)

```
FFFFFFFFFFFF  ← default transport key
D3F7D3F7D3F7  ← NFC Forum default
A0A1A2A3A4A5  ← MAD (MIFARE Application Directory) key
000000000000  ← zero key
```

---

## Known Reader-Specific Issues

### ACR1252U — State Corruption After GET_VERSION

**Problem:** GET_VERSION can leave the reader in a corrupted state, causing subsequent commands to fail or return garbage.

**Solution (implemented in `internal/core/card.go`):**
1. After GET_VERSION failure, do a full disconnect with `scard.ResetCard`
2. Reconnect with a fresh context
3. This fixes the corruption without requiring reader unplug

### ACR122U — Rapid Command Sequences

**Problem:** The ACR122U doesn't handle rapid APDU sequences well. Card detection (which involves GET_VERSION + CC reads + NDEF parse) can overwhelm it.

**Solution:** Detection results are cached by UID in `cardDetectionCache`. Once a card is detected, all detection info (type, size, NDEF) is cached and reused until the card is removed.

### Per-Reader Mutex

Each reader has its own mutex (`readerMutexes` in `internal/core/card.go`) to prevent concurrent `GetCardUID` calls on the same reader. This is especially important for password-protected NTAG cards where detection takes longer and polling ticks can overlap.

---

## Debugging Card Detection Problems

**Step 1:** Enable extended logging and watch logs while placing a card:
```bash
curl -X POST http://127.0.0.1:32145/v1/settings -H "Content-Type: application/json" -d '{"extendedLogging":true}'
# place card on reader
curl "http://127.0.0.1:32145/v1/logs?limit=100&level=debug"
```

**Step 2:** Check what the agent sees:
```bash
curl http://127.0.0.1:32145/v1/readers/0/card
# If this returns an error, the card may not be detected or reader is having issues
```

**Step 3:** Run the generic inspector:
```bash
python3 scripts/inspect_tag.py
# Shows all available info including raw pages/blocks
```

**Step 4:** For MIFARE Classic issues, try reading with explicit keys:
```bash
# Try default transport key
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=FFFFFFFFFFFF&keyType=A"
# Try NFC Forum key
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=D3F7D3F7D3F7&keyType=A"
```

---

## Existing Debug Scripts

| Script | Purpose |
|--------|---------|
| `scripts/debug_api.py` | Quick health check — reader list, version, health endpoint |
| `scripts/inspect_tag.py` | Generic tag inspector — reads everything available from any card |
| `scripts/read_creality.py` | Reads + decrypts a Creality CFS tag end-to-end |
| `scripts/test_write.py` | Tests NTAG page writes and MIFARE block writes with round-trip verify |
| `scripts/capture_tag_data.py` | Captures APDU data via WebSocket for regression testing |
| `scripts/auto_capture.py` | Continuous monitoring / multi-tag capture |
| `scripts/capture_one.py` | Single-tag capture |

---

## Common Troubleshooting

### "No readers found"
1. Check reader is connected: `lsusb` (Linux) or System Information (macOS)
2. Check PC/SC daemon: `systemctl status pcscd` (Linux)
3. Linux kernel module conflict: `lsmod | grep pn533` — if loaded, blacklist them:
   ```bash
   echo -e "blacklist pn533_usb\nblacklist pn533\nblacklist nfc" | sudo tee /etc/modprobe.d/blacklist-nfc-pn533.conf
   sudo modprobe -r pn533_usb pn533 nfc 2>/dev/null || true
   sudo systemctl restart pcscd
   ```
4. Arch Linux with ACS reader: install `acsccid` from AUR

### "Failed to connect to card"
1. Card not centred on reader — try repositioning
2. Reader LED should light up when card is detected
3. Try a different card to rule out card damage

### "Rejected unauthorized PC/SC client" (Linux)
On modern Fedora/Silverblue, PC/SC access is controlled by Polkit. The agent must run in a graphical session.

**Solution:** Run `./nfc-agent` directly from terminal (not as a system service), or use `./nfc-agent install` to set up XDG autostart (starts with desktop login).

### Card detected but type is "unknown"
The detection pipeline (GET_VERSION + capability container check + memory probe) failed. Enable extended logging to see which APDU commands returned unexpected responses.

### OpenPrintTag not detected
Make sure you're using an ACR1552U. Other readers do not support ISO 15693 (NFC-V) required for ICode SLIX/SLIX2 tags.
