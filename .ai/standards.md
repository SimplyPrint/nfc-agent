# NFC Standards Guide

## Core Principle

**NFC Agent does NOT parse brand-specific tag data.**

The agent exposes raw block/page access via API. Creality, Anycubic, Qidi, etc. data is parsed by SimplyPrint's backend or mobile app — not here. This avoids duplicating logic across the agent, mobile app, and server.

**Exception:** OpenPrintTag has full native encode/decode in `internal/openprinttag/`. This is intentional because OpenPrintTag is the primary open standard we actively write.

---

## Standard Quick Reference

| Standard | Tag required | iOS | WebNFC | Reader required |
|----------|-------------|-----|--------|-----------------|
| **OpenPrintTag** | ICode SLIX / SLIX2 (ISO 15693) | ✗ | ✓ (Android Chrome) | ACR1552U only |
| **Creality CFS** | MIFARE Classic 1K | ✗ | ✗ | Any (desktop reader or Android app) |
| **Anycubic ACE** | NTAG215 or NTAG216 | ✓ | ✗ | Any |
| **Qidi Tech (Qidi Box)** | MIFARE Classic 1K | ✗ | ✗ | Any (desktop reader or Android app) |
| **OpenSpool** | NTAG215 or NTAG216 | ✓ | ✓ | Any |
| **OpenTag** | NTAG213, 215, or 216 | ✓ | ✓ | Any |
| **SimplyPrint URL** | Any tag | ✓ | ✓ | Any |

**iOS cannot read/write MIFARE Classic** — this is a hardware limitation, not a software one. Creality CFS and Qidi Box tags are therefore iOS-incompatible.

**WebNFC (Android Chrome only)** supports NDEF but not raw MIFARE Classic or MIFARE Ultralight access.

---

## Tag Type Reference

| Tag | Size | Standards |
|-----|------|----------|
| NTAG213 | 144B | OpenTag, SimplyPrint URL |
| NTAG215 | 504B | Anycubic ACE, OpenSpool, OpenTag, SimplyPrint URL |
| NTAG216 | 888B | Anycubic ACE, OpenSpool, OpenTag, SimplyPrint URL |
| MIFARE Classic 1K | 752B usable | Creality CFS, Qidi Tech, SimplyPrint URL |
| MIFARE Ultralight C | 144B | Anycubic ACE, SimplyPrint URL |
| ICode SLIX / SLIX2 | 316B | OpenPrintTag, SimplyPrint URL |

---

## Per-Standard Details

### OpenPrintTag (by Prusa)

- **Tag:** ICode SLIX / SLIX2 — ISO 15693 (NFC-V protocol)
- **Reader:** Requires ACR1552U — it's the only supported reader with ISO 15693 support. ACR122U and ACR1252U do NOT work for this standard.
- **Encoding:** CBOR, with UUIDv5 for brand/material/instance identification
- **Spec:** https://specs.openprinttag.org/#/
- **Agent support:** Full native encode/decode (`internal/openprinttag/`)
- **Read:** `GET /readers/0/card` → response has `"dataType":"openprinttag"` with full parsed JSON
- **Write:** `POST /readers/0/card` with `"dataType":"openprinttag"` and JSON material fields

**Write example:**
```bash
curl -X POST http://127.0.0.1:32145/v1/readers/0/card \
  -H "Content-Type: application/json" \
  -d '{
    "dataType": "openprinttag",
    "data": {
      "materialName": "PLA Galaxy Black",
      "brandName": "Prusament",
      "nominalWeight": 1000,
      "materialClass": 0,
      "materialType": 0,
      "primaryColor": "#1A1A1A",
      "minPrintTemp": 215,
      "maxPrintTemp": 230
    }
  }'
```

**materialType values:** 0=PLA, 1=ABS, 2=PETG, 3=TPU, 4=ASA, 5=PA (Nylon), 6=PC, 7=Carbon fiber composite, 8=HIPS, 9=PVA, 10=Other

---

### Creality CFS

- **Tag:** MIFARE Classic 1K
- **Data location:** Sector 1, blocks 4, 5, 6 (48 bytes total)
- **Authentication:** Custom derived key (see below)
- **Data encryption:** AES-128-ECB

**Key derivation algorithm:**
```python
AES_KEY_DERIVE = b"q3bu^t1nqfZ(pf$1"  # 16 bytes

def derive_key(uid_hex: str) -> str:
    uid_bytes = bytes.fromhex(uid_hex[:8])  # First 4 bytes of UID
    plaintext = uid_bytes * 4               # Repeat to fill 16 bytes
    cipher = AES.new(AES_KEY_DERIVE, AES.MODE_ECB)
    encrypted = cipher.encrypt(plaintext)
    return encrypted[:6].hex().upper()      # First 6 bytes = MIFARE key
```

**Data decryption:**
```python
AES_KEY_DATA = b"H@CFkRnz@KAtBJp2"  # 16 bytes

def decrypt_block(encrypted_hex: str) -> str:
    cipher = AES.new(AES_KEY_DATA, AES.MODE_ECB)
    return cipher.decrypt(bytes.fromhex(encrypted_hex)).decode('ascii', errors='replace')
```

**Plaintext structure (48 ASCII chars):**
```
[0:6]   Date code          e.g. "250115"
[6:10]  Vendor ID          e.g. "0001"
[10:12] Batch code         e.g. "01"
[12:18] Filament ID        e.g. "PLA001"
[18:24] Color (hex RGB)    e.g. "FF0000"
[24:28] Length code        "0330"=1000g, "0247"=750g, "0198"=600g, "0165"=500g, "0082"=250g
[28:34] Serial number      e.g. "SN0001"
[34:48] Padding
```

**API flow for reading Creality tags:**
```bash
# 1. Read card (get UID)
curl http://127.0.0.1:32145/v1/readers/0/card

# 2. Derive sector key from UID
curl -X POST http://127.0.0.1:32145/v1/readers/0/mifare/derive-key \
  -H "Content-Type: application/json" \
  -d '{"aesKey":"713362755e74316e71665a2870662431"}'
# aesKey is AES_KEY_DERIVE above as hex

# 3. Read encrypted blocks 4, 5, 6
curl "http://127.0.0.1:32145/v1/readers/0/mifare/4?key=<derived_key>&keyType=A"
curl "http://127.0.0.1:32145/v1/readers/0/mifare/5?key=<derived_key>&keyType=A"
curl "http://127.0.0.1:32145/v1/readers/0/mifare/6?key=<derived_key>&keyType=A"

# 4. Decrypt each block client-side with AES_KEY_DATA
```

**Script:** `scripts/read_creality.py` — does all of the above end-to-end.

**Parsing:** Decrypted data is sent to SimplyPrint backend for interpretation. Agent does not parse filament type, color name, etc.

---

### Anycubic ACE

- **Tag:** NTAG215 or NTAG216
- **Encoding:** NDEF JSON record (plain JSON, no encryption)
- **Read:** `GET /readers/0/card` → `"dataType":"json"`, `"data"` contains raw JSON string
- **Write:** `POST /readers/0/card` with `"dataType":"json"` and JSON string in `"data"`

**Parsing:** JSON structure is parsed by SimplyPrint backend. Agent just stores/retrieves raw JSON.

**Example read result:**
```json
{
  "uid": "04:A2:B3:C4:D5:E6",
  "type": "NTAG215",
  "dataType": "json",
  "data": "{\"material\":\"PLA\",\"color\":\"#FF0000\",\"weight\":1000}"
}
```

---

### Qidi Tech (Qidi Box)

- **Tag:** MIFARE Classic 1K
- **Encoding:** Custom raw block writes (no encryption)
- **Read:** `GET /readers/0/mifare/{block}` with default keys
- **Parsing:** Entirely server-side — SimplyPrint backend interprets block data

No known public documentation from Qidi at time of writing.

---

### OpenSpool

- **Tag:** NTAG215 or NTAG216
- **Encoding:** NDEF JSON record
- **Spec:** https://openspool.io/
- **Read/write:** Same as Anycubic ACE (NDEF JSON via `/card` endpoint)

---

### OpenTag

- **Tag:** NTAG213, 215, or 216
- **Encoding:** NDEF
- **Spec:** https://opentag3d.info/
- **Read/write:** Via `/card` endpoint

---

### SimplyPrint Simple URL

- **Tag:** Any supported tag
- **Encoding:** NDEF URI record pointing to the spool's SimplyPrint URL
- **Use case:** Lets any NFC-capable phone tap the tag and open the spool in SimplyPrint
- **Write:** `POST /readers/0/card` with `"dataType":"url"` and the spool URL in `"data"`

---

## Platform Compatibility Summary

| Method | Creality CFS | Qidi | Anycubic | OpenPrintTag | OpenSpool | OpenTag |
|--------|-------------|------|----------|--------------|-----------|---------|
| Desktop NFC Agent + ACR1552U | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| Desktop NFC Agent + ACR122U/1252U | ✓ | ✓ | ✓ | ✗ | ✓ | ✓ |
| SimplyPrint mobile app (Android) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| SimplyPrint mobile app (iOS) | ✗ | ✗ | ✓ | ✗ | ✓ | ✓ |
| Web NFC (Android Chrome) | ✗ | ✗ | ✗ | ✓ | ✓ | ✓ |
