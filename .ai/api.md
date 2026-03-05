# NFC Agent API Reference

Base URL: `http://127.0.0.1:32145/v1`
WebSocket: `ws://127.0.0.1:32145/v1/ws`

Port is configurable via `NFC_AGENT_PORT` env var. All responses are JSON.

---

## HTTP API

### System Endpoints

```bash
# Health check
curl http://127.0.0.1:32145/v1/health
# → {"status":"ok","readerCount":1}

# Version info (also checks for updates)
curl http://127.0.0.1:32145/v1/version
# → {"version":"1.2.3","buildTime":"2025-01-15T10:30:00Z","gitCommit":"abc1234",
#    "updateAvailable":true,"latestVersion":"1.3.0","releaseUrl":"https://..."}

# List supported reader models
curl http://127.0.0.1:32145/v1/supported-readers

# Get/update user settings
curl http://127.0.0.1:32145/v1/settings
curl -X POST http://127.0.0.1:32145/v1/settings \
  -H "Content-Type: application/json" \
  -d '{"extendedLogging":true,"crashReporting":true}'

# Get recent logs
# Response: {"entries":[{"timestamp":"...","level":"INFO","category":"card","message":"...","data":{...}},...],
#            "stats":{"total_entries":278,"max_entries":1000,"min_level":"DEBUG"}}
curl "http://127.0.0.1:32145/v1/logs?limit=100&level=debug&category=Card"

# Clear logs
curl -X DELETE http://127.0.0.1:32145/v1/logs

# List crash reports
curl http://127.0.0.1:32145/v1/crashes

# Check for updates
curl "http://127.0.0.1:32145/v1/updates?refresh=true"

# Auto-start management
curl http://127.0.0.1:32145/v1/autostart
curl -X POST http://127.0.0.1:32145/v1/autostart    # enable
curl -X DELETE http://127.0.0.1:32145/v1/autostart  # disable

# Graceful shutdown
curl -X POST http://127.0.0.1:32145/v1/shutdown
```

---

### Reader Listing

```bash
# List connected readers
curl http://127.0.0.1:32145/v1/readers
# → [{"id":"0","name":"ACS ACR1252U PICC Reader","type":"picc"}]
```

Reader index `{n}` in all `/readers/{n}/...` routes is the **0-based array index** from this list.

---

### Card (NDEF) Operations

```bash
# Read card — UID, type, protocol, NDEF data. FAST — use for detection/polling.
curl http://127.0.0.1:32145/v1/readers/0/card
# → {
#     "uid": "04:A2:B3:C4:D5:E6:07",
#     "atr": "3b8f8001...",
#     "type": "NTAG215",
#     "protocol": "NFC-A",
#     "protocolISO": "ISO 14443-3A",
#     "size": 504,
#     "writable": true,
#     "url": "https://example.com",
#     "data": "https://example.com",
#     "dataType": "url"
#   }
# dataType values: "url", "text", "json", "openprinttag", "binary", "unknown"

# Write URL
curl -X POST http://127.0.0.1:32145/v1/readers/0/card \
  -H "Content-Type: application/json" \
  -d '{"data":"https://example.com","dataType":"url"}'
# → {"success":"data written successfully"}

# Write text
curl -X POST http://127.0.0.1:32145/v1/readers/0/card \
  -H "Content-Type: application/json" \
  -d '{"data":"Hello World","dataType":"text"}'

# Write JSON
curl -X POST http://127.0.0.1:32145/v1/readers/0/card \
  -H "Content-Type: application/json" \
  -d '{"data":"{\"key\":\"value\"}","dataType":"json"}'

# Write OpenPrintTag
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

# Unified read — metadata + NDEF + full raw memory dump. SLOW (full memory read).
# Call once on demand after detection — do NOT poll with this.
# For NTAG/Ultralight: pages field is populated. For MIFARE Classic: blocks + failedBlocks.
curl http://127.0.0.1:32145/v1/readers/0/read
# NTAG215 response:
# → {"uid":"04484783...","type":"NTAG213","protocol":"NFC-A","size":180,"writable":true,
#    "dataType":"json","data":"{...}","pages":["04484783","8a837280",...]}
# MIFARE Classic response:
# → {"uid":"c34e2820","type":"MIFARE Classic","blocks":{"0":"c34e28...","1":"000000..."},
#    "failedBlocks":[12,16]}

# Raw memory dump only (no NDEF metadata — use /read instead for most cases)
curl http://127.0.0.1:32145/v1/readers/0/dump
# → {"uid":"c34e2820","type":"NTAG215","pages":["04c34e28","20800149","e1100600","03000000",...]}
# → {"uid":"c34e2820","type":"MIFARE Classic","blocks":{"0":"c34e28...","1":"000000..."},"failedBlocks":[12,16]}

# Erase card
curl -X POST http://127.0.0.1:32145/v1/readers/0/erase

# Lock card permanently (IRREVERSIBLE!)
curl -X POST http://127.0.0.1:32145/v1/readers/0/lock \
  -H "Content-Type: application/json" \
  -d '{"confirm":true}'

# Write multiple NDEF records
curl -X POST http://127.0.0.1:32145/v1/readers/0/records \
  -H "Content-Type: application/json" \
  -d '{"records":[
    {"type":"uri","format":"U","data":"https://example.com"},
    {"type":"text","format":"T","data":"Hello"}
  ]}'
```

---

### Password Protection (NTAG EV1 only)

```bash
# Set password (password = 4 bytes = 8 hex chars, pack = 2 bytes = 4 hex chars)
curl -X POST http://127.0.0.1:32145/v1/readers/0/password \
  -H "Content-Type: application/json" \
  -d '{"password":"12345678","pack":"0000","startPage":4}'

# Remove password
curl -X DELETE http://127.0.0.1:32145/v1/readers/0/password \
  -H "Content-Type: application/json" \
  -d '{"password":"12345678"}'
```

---

### MIFARE Classic — Raw Block Access

Blocks are 16 bytes (32 hex chars). Sector trailers (blocks 3, 7, 11, 15...) are restricted.

Default keys tried automatically if no key given: `FFFFFFFFFFFF`, `D3F7D3F7D3F7`, `A0A1A2A3A4A5`, `000000000000`

```bash
# Read block (key is optional — tries defaults if omitted)
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4"
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=FFFFFFFFFFFF&keyType=A"
# → {"block":4,"data":"00112233445566778899AABBCCDDEEFF"}

# Write block
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/4 \
  -H "Content-Type: application/json" \
  -d '{"data":"00112233445566778899AABBCCDDEEFF","key":"FFFFFFFFFFFF","keyType":"A"}'
# → {"success":"block written"}

# Batch write (re-authenticates across sector boundaries automatically)
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/batch \
  -H "Content-Type: application/json" \
  -d '{
    "blocks": [
      {"block": 4, "data": "00112233445566778899AABBCCDDEEFF"},
      {"block": 5, "data": "FFEEDDCCBBAA99887766554433221100"},
      {"block": 8, "data": "DEADBEEFDEADBEEFDEADBEEFDEADBEEF"}
    ],
    "key": "FFFFFFFFFFFF",
    "keyType": "A"
  }'
# → {"results":[{"block":4,"success":true,"error":""},...],"written":3,"total":3}

# Derive 6-byte MIFARE key from UID using AES-128-ECB
# (Used for Creality CFS tag key derivation)
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/derive-key \
  -H "Content-Type: application/json" \
  -d '{"aesKey":"713362755e74316e71665a2870662431"}'
# → {"key":"abc123def456"}  (6 bytes = 12 hex chars)

# AES-128-ECB encrypt plaintext and write to block
# (Used for Creality CFS tag data blocks)
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/aes-write/4 \
  -H "Content-Type: application/json" \
  -d '{
    "data": "30303030303030303030303030303030",
    "aesKey": "484043466b526e7a404b4174424a7032",
    "authKey": "FFFFFFFFFFFF",
    "authKeyType": "A"
  }'

# Write sector trailer (new keys + access bits)
# Sector trailers: block 3 (sector 0), 7 (sector 1), 11 (sector 2), 15 (sector 3)...
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

---

### MIFARE Ultralight / NTAG — Raw Page Access

Pages are 4 bytes (8 hex chars). Pages 0–3 are protected.

```bash
# Read page
curl http://127.0.0.1:32145/v1/readers/0/ultralight/4
curl "http://127.0.0.1:32145/v1/readers/0/ultralight/4?password=12345678"
# → {"page":4,"data":"DEADBEEF"}

# Write page
curl -X POST http://127.0.0.1:32145/v1/readers/0/ultralight/4 \
  -H "Content-Type: application/json" \
  -d '{"data":"DEADBEEF"}'
curl -X POST http://127.0.0.1:32145/v1/readers/0/ultralight/4 \
  -H "Content-Type: application/json" \
  -d '{"data":"DEADBEEF","password":"12345678"}'

# Batch write pages
curl -X POST http://127.0.0.1:32145/v1/readers/0/ultralight/batch \
  -H "Content-Type: application/json" \
  -d '{
    "pages": [
      {"page": 4, "data": "DEADBEEF"},
      {"page": 5, "data": "CAFEBABE"},
      {"page": 6, "data": "12345678"}
    ]
  }'
# → {"results":[{"page":4,"success":true,"error":""},...],"written":3,"total":3}
```

**Ultralight memory layout:**
| Pages | Contents | Notes |
|-------|----------|-------|
| 0–1 | UID | Read-only |
| 2 | Lock bytes | Writing permanently locks pages! |
| 3 | Capability Container (CC) | OTP bits are irreversible |
| 4+ | User data | Safe to read/write |
| Last 4–5 | Config/Password (EV1) | Varies by variant |

---

## WebSocket API

URL: `ws://127.0.0.1:32145/v1/ws`

**Message format:**
```json
{
  "type": "message_type",
  "id": "optional-request-id",
  "payload": { ... }
}
```

**Response format:**
```json
{
  "type": "response_type",
  "id": "matching-request-id",
  "payload": { ... },
  "error": "error message if any"
}
```

---

### System Messages

```json
// List readers
→ {"type":"list_readers","id":"1"}
← {"type":"readers","id":"1","payload":[{"id":"0","name":"ACS ACR1252U PICC Reader","type":"picc"}]}

// Health check
→ {"type":"health","id":"2"}
← {"type":"health","id":"2","payload":{"status":"ok","readerCount":1}}

// Version info
→ {"type":"version","id":"3"}
← {"type":"version","id":"3","payload":{"version":"1.2.3","buildTime":"...","gitCommit":"..."}}

// Supported reader models
→ {"type":"supported_readers","id":"4"}
← {"type":"supported_readers","id":"4","payload":[...]}
```

---

### Card Operations

```json
// Read card
→ {"type":"read_card","id":"r1","payload":{"readerIndex":0}}
← {"type":"card","id":"r1","payload":{"uid":"04:A2:B3:C4","type":"NTAG215","dataType":"url","url":"https://..."}}

// Write card
→ {"type":"write_card","id":"w1","payload":{"readerIndex":0,"data":"https://example.com","dataType":"url"}}
← {"type":"card_written","id":"w1","payload":{"success":"data written successfully"}}

// Erase card
→ {"type":"erase_card","id":"e1","payload":{"readerIndex":0}}
← {"type":"card_erased","id":"e1","payload":{"success":"card erased"}}

// Lock card (permanent!)
→ {"type":"lock_card","id":"l1","payload":{"readerIndex":0,"confirm":true}}
← {"type":"card_locked","id":"l1","payload":{"success":"card locked"}}

// Set password (NTAG EV1 only)
→ {"type":"set_password","id":"p1","payload":{"readerIndex":0,"password":"12345678","pack":"0000","startPage":4}}
← {"type":"password_set","id":"p1","payload":{"success":"password set"}}

// Remove password
→ {"type":"remove_password","id":"p2","payload":{"readerIndex":0,"password":"12345678"}}
← {"type":"password_removed","id":"p2","payload":{"success":"password removed"}}

// Write multiple NDEF records
→ {"type":"write_records","id":"wr1","payload":{
    "readerIndex":0,
    "records":[
      {"type":"uri","format":"U","data":"https://example.com"},
      {"type":"text","format":"T","data":"Hello"}
    ]
  }}
← {"type":"records_written","id":"wr1","payload":{"success":"records written"}}
```

---

### Real-time Card Detection (Subscribe)

```json
// Subscribe to card events (intervalMs = polling interval)
→ {"type":"subscribe","id":"s1","payload":{"readerIndex":0,"intervalMs":500}}
← {"type":"subscribed","id":"s1","payload":null}

// Subscribe with full raw dump — card_detected fires immediately, then card_data fires
// once the full memory read completes (background goroutine, non-blocking)
→ {"type":"subscribe","id":"s1","payload":{"readerIndex":0,"intervalMs":500,"includeRaw":true}}

// Server sends these automatically when card state changes:
← {"type":"card_detected","payload":{
    "readerIndex":0,
    "readerName":"ACS ACR1252U PICC Reader",
    "card":{"uid":"04:A2:B3:C4","type":"NTAG215","dataType":"url","url":"https://..."}
  }}
// (only when subscribed with includeRaw:true) — fires after card_detected
← {"type":"card_data","payload":{
    "readerIndex":0,
    "readerName":"ACS ACR1252U PICC Reader",
    "uid":"c34e2820",
    "type":"NTAG215",
    "pages":["04c34e28","20800149","e1100600","03000000",...]
  }}
// For MIFARE Classic card_data:
← {"type":"card_data","payload":{
    "uid":"c34e2820","type":"MIFARE Classic",
    "blocks":{"0":"c34e28...","1":"000000...","4":"aabbcc..."},
    "failedBlocks":[12,16]
  }}
← {"type":"card_removed","payload":{"readerIndex":0,"readerName":"ACS ACR1252U PICC Reader"}}

// Unified read — metadata + NDEF + full raw memory. SLOW — call once on demand, not in a loop.
→ {"type":"read_card_full","id":"r1","payload":{"readerIndex":0}}
← {"type":"read_card_full","id":"r1","payload":{
    "readerIndex":0,"readerName":"ACS ACR1552...",
    "uid":"04484783...","type":"NTAG213","protocol":"NFC-A","size":180,"writable":true,
    "dataType":"json","data":"{...}","pages":["04484783",...]
  }}

// On-demand raw dump only (no NDEF metadata — use read_card_full instead for most cases)
→ {"type":"dump_card","id":"d1","payload":{"readerIndex":0}}
← {"type":"dump_card","id":"d1","payload":{"uid":"c34e2820","type":"NTAG215","pages":[...]}}

// Unsubscribe
→ {"type":"unsubscribe","id":"u1","payload":{"readerIndex":0}}
← {"type":"unsubscribed","id":"u1","payload":null}
```

---

### MIFARE Classic Block Operations (WebSocket)

```json
// Read block
→ {"type":"read_mifare_block","id":"mb1","payload":{"readerIndex":0,"block":4,"key":"FFFFFFFFFFFF","keyType":"A"}}
← {"type":"mifare_block","id":"mb1","payload":{"block":4,"data":"00112233445566778899AABBCCDDEEFF"}}

// Write block
→ {"type":"write_mifare_block","id":"mb2","payload":{"readerIndex":0,"block":4,"data":"00112233445566778899AABBCCDDEEFF","key":"FFFFFFFFFFFF","keyType":"A"}}
← {"type":"mifare_block_written","id":"mb2","payload":{"success":"block written"}}

// Write multiple blocks
→ {"type":"write_mifare_blocks","id":"mb3","payload":{
    "readerIndex":0,
    "blocks":[{"block":4,"data":"..."},{"block":5,"data":"..."}],
    "key":"FFFFFFFFFFFF",
    "keyType":"A"
  }}
← {"type":"mifare_blocks_written","id":"mb3","payload":{"results":[...],"written":2,"total":2}}

// Derive UID key via AES
→ {"type":"derive_uid_key_aes","id":"dk1","payload":{"readerIndex":0,"aesKey":"713362755e74316e71665a2870662431"}}
← {"type":"uid_key_derived","id":"dk1","payload":{"key":"abc123def456"}}

// AES encrypt and write block
→ {"type":"aes_encrypt_and_write_block","id":"aw1","payload":{
    "readerIndex":0,"block":4,
    "data":"30303030303030303030303030303030",
    "aesKey":"484043466b526e7a404b4174424a7032",
    "authKey":"FFFFFFFFFFFF","authKeyType":"A"
  }}
← {"type":"block_encrypted_and_written","id":"aw1","payload":{"success":"block encrypted and written"}}

// Write sector trailer
→ {"type":"write_mifare_sector_trailer","id":"st1","payload":{
    "readerIndex":0,"block":7,
    "keyA":"abc123def456","keyB":"abc123def456",
    "accessBits":"FF0780",
    "authKey":"FFFFFFFFFFFF","authKeyType":"A"
  }}
← {"type":"sector_trailer_written","id":"st1","payload":{"success":"sector trailer written"}}
```

---

### MIFARE Ultralight / NTAG Page Operations (WebSocket)

```json
// Read page
→ {"type":"read_ultralight_page","id":"up1","payload":{"readerIndex":0,"page":4,"password":"12345678"}}
← {"type":"ultralight_page","id":"up1","payload":{"page":4,"data":"DEADBEEF"}}

// Write page
→ {"type":"write_ultralight_page","id":"up2","payload":{"readerIndex":0,"page":4,"data":"DEADBEEF","password":"12345678"}}
← {"type":"ultralight_page_written","id":"up2","payload":{"success":"page written"}}

// Write multiple pages
→ {"type":"write_ultralight_pages","id":"up3","payload":{
    "readerIndex":0,
    "pages":[{"page":4,"data":"DEADBEEF"},{"page":5,"data":"CAFEBABE"}],
    "password":"12345678"
  }}
← {"type":"ultralight_pages_written","id":"up3","payload":{"results":[...],"written":2,"total":2}}
```

---

## OpenPrintTag Fields Reference

When writing with `dataType:"openprinttag"`, the `data` field is a JSON object:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `materialName` | string | yes | Display name (e.g. "PLA Galaxy Black") |
| `brandName` | string | yes | Brand/manufacturer (e.g. "Prusament") |
| `nominalWeight` | float | yes | Nominal weight in grams (e.g. 1000) |
| `materialClass` | int | no | 0 = FFF/filament (default), 1 = SLA/resin |
| `materialType` | int | no | 0=PLA, 1=ABS, 2=PETG, 3=TPU, 4=ASA, 5=PA, 6=PC, 7=Carbon fiber, 8=HIPS, 9=PVA, 10=Other |
| `primaryColor` | string | no | Hex color (#RRGGBB or #RRGGBBAA) |
| `filamentDiameter` | float | no | Diameter in mm (default: 1.75) |
| `density` | float | no | Material density in g/cm³ |
| `minPrintTemp` | int | no | Minimum print temp °C |
| `maxPrintTemp` | int | no | Maximum print temp °C |
| `manufacturedDate` | int | no | Unix timestamp |
| `expirationDate` | int | no | Unix timestamp |

See full spec: https://specs.openprinttag.org/#/

---

## Parameter Size Reference

| Parameter | Size | Example |
|-----------|------|---------|
| `key` / `keyA` / `keyB` (MIFARE) | 12 hex chars (6 bytes) | `FFFFFFFFFFFF` |
| `aesKey` | 32 hex chars (16 bytes) | `713362755e74316e71665a2870662431` |
| `password` (NTAG EV1) | 8 hex chars (4 bytes) | `12345678` |
| `pack` (NTAG EV1) | 4 hex chars (2 bytes) | `0000` |
| MIFARE block `data` | 32 hex chars (16 bytes) | `00112233445566778899AABBCCDDEEFF` |
| Ultralight page `data` | 8 hex chars (4 bytes) | `DEADBEEF` |
| `accessBits` | 6 or 8 hex chars (3–4 bytes) | `FF0780` |
