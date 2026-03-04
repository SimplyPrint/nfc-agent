#!/usr/bin/env python3
"""Read and decode Creality CFS RFID tag data."""

import requests
import sys
from Crypto.Cipher import AES

BASE_URL = "http://127.0.0.1:32145/v1"

# Creality AES keys (from CrealityMaterialStandard.php)
AES_KEY_DERIVE = b"q3bu^t1nqfZ(pf$1"  # For UID key derivation
AES_KEY_DATA = b"H@CFkRnz@KAtBJp2"    # For data encryption

def get_card(reader_index=0):
    """Get card info from reader."""
    resp = requests.get(f"{BASE_URL}/readers/{reader_index}/card")
    if resp.status_code == 200:
        return resp.json()
    return None

def derive_key_from_uid(uid_hex: str) -> str:
    """Derive 6-byte MIFARE sector key from UID using Creality's algorithm."""
    # Take first 4 bytes of UID
    uid_bytes = bytes.fromhex(uid_hex[:8])
    # Repeat to fill 16 bytes
    plaintext = uid_bytes * 4
    # AES-128-ECB encrypt with derive key
    cipher = AES.new(AES_KEY_DERIVE, AES.MODE_ECB)
    encrypted = cipher.encrypt(plaintext)
    # Take first 6 bytes as key
    return encrypted[:6].hex().upper()

def decrypt_block(encrypted_hex: str) -> str:
    """Decrypt a 16-byte block using Creality's AES key."""
    encrypted = bytes.fromhex(encrypted_hex)
    cipher = AES.new(AES_KEY_DATA, AES.MODE_ECB)
    decrypted = cipher.decrypt(encrypted)
    return decrypted.decode('ascii', errors='replace')

def read_block(reader_index: int, block: int, key: str, key_type: str = "A") -> dict:
    """Read a MIFARE Classic block."""
    resp = requests.get(
        f"{BASE_URL}/readers/{reader_index}/mifare/{block}",
        params={"key": key, "keyType": key_type}
    )
    return resp.json() if resp.status_code == 200 else {"error": resp.json()}

def read_sector_trailer(reader_index: int, block: int, key: str, key_type: str = "A") -> dict:
    """Read sector trailer to see keys."""
    return read_block(reader_index, block, key, key_type)

def parse_payload(payload: str) -> dict:
    """Parse 48-char Creality payload string."""
    if len(payload) < 34:
        return {"error": f"Payload too short: {len(payload)} chars"}

    # Pad to 48 chars
    payload = payload.ljust(48, '0')[:48]

    return {
        "date_code": payload[0:6],
        "vendor_id": payload[6:10],
        "batch_code": payload[10:12],
        "filament_id": payload[12:18],
        "color_hex": payload[18:24],
        "filament_len_code": payload[24:28],
        "serial_num": payload[28:34],
        "padding": payload[34:48],
    }

def length_code_to_weight(code: str) -> str:
    """Convert length code to weight."""
    mapping = {
        "0330": "1000g",
        "0247": "750g",
        "0198": "600g",
        "0165": "500g",
        "0082": "250g",
    }
    return mapping.get(code, "unknown")

def main():
    print("=" * 60)
    print("Creality CFS RFID Tag Reader")
    print("=" * 60)

    # Get card
    card = get_card(0)
    if not card:
        print("\nNo card detected. Place a MIFARE Classic tag on the reader.")
        return 1

    uid = card.get('uid', '')
    print(f"\nCard detected:")
    print(f"  Type: {card.get('type', 'Unknown')}")
    print(f"  UID:  {uid}")
    print(f"  ATR:  {card.get('atr', 'N/A')}")

    if "Classic" not in card.get('type', ''):
        print(f"\nWarning: Card is not MIFARE Classic")

    # Derive key from UID
    derived_key = derive_key_from_uid(uid)
    factory_key = "FFFFFFFFFFFF"

    print(f"\n  Factory Key:  {factory_key}")
    print(f"  Derived Key:  {derived_key}")

    # Try to read blocks 4, 5, 6 (sector 1 data blocks)
    print("\n" + "-" * 60)
    print("Reading Sector 1 (blocks 4, 5, 6) - Creality Data")
    print("-" * 60)

    encrypted_blocks = []
    working_key = None
    working_key_type = None

    for block in [4, 5, 6]:
        print(f"\nBlock {block}:")

        # Try factory key first
        result = read_block(0, block, factory_key, "A")
        if "error" not in result and result.get("data"):
            print(f"  [Factory Key A] Raw: {result['data']}")
            encrypted_blocks.append(result['data'])
            working_key = factory_key
            working_key_type = "A"
            continue

        # Try derived key A
        result = read_block(0, block, derived_key, "A")
        if "error" not in result and result.get("data"):
            print(f"  [Derived Key A] Raw: {result['data']}")
            encrypted_blocks.append(result['data'])
            working_key = derived_key
            working_key_type = "A"
            continue

        # Try derived key B
        result = read_block(0, block, derived_key, "B")
        if "error" not in result and result.get("data"):
            print(f"  [Derived Key B] Raw: {result['data']}")
            encrypted_blocks.append(result['data'])
            working_key = derived_key
            working_key_type = "B"
            continue

        print(f"  FAILED to read (tried factory and derived keys)")
        encrypted_blocks.append(None)

    # Read sector trailer (block 7)
    print(f"\nBlock 7 (Sector Trailer):")
    for key, ktype, label in [(factory_key, "A", "Factory A"), (derived_key, "A", "Derived A"), (derived_key, "B", "Derived B")]:
        result = read_block(0, 7, key, ktype)
        if "error" not in result and result.get("data"):
            print(f"  [{label}] Raw: {result['data']}")
            # Parse trailer: KeyA (6) + Access (4) + KeyB (6)
            data = result['data']
            if len(data) == 32:
                key_a = data[0:12]
                access = data[12:20]
                key_b = data[20:32]
                print(f"    Key A: {key_a}")
                print(f"    Access: {access}")
                print(f"    Key B: {key_b}")
            break
    else:
        print(f"  FAILED to read sector trailer")

    # Decrypt and decode if we have all blocks
    print("\n" + "-" * 60)
    print("Decrypted Data")
    print("-" * 60)

    if all(encrypted_blocks):
        decrypted = ""
        for i, enc in enumerate(encrypted_blocks):
            dec = decrypt_block(enc)
            print(f"Block {4+i}: {repr(dec)} ({enc})")
            decrypted += dec

        print(f"\nFull payload (48 chars): {repr(decrypted)}")

        # Parse the payload
        print("\n" + "-" * 60)
        print("Parsed Fields")
        print("-" * 60)
        parsed = parse_payload(decrypted)
        if "error" not in parsed:
            print(f"  Date Code:      {parsed['date_code']}")
            print(f"  Vendor ID:      {parsed['vendor_id']}")
            print(f"  Batch Code:     {parsed['batch_code']}")
            print(f"  Filament ID:    {parsed['filament_id']}")
            print(f"  Color (hex):    #{parsed['color_hex']}")
            print(f"  Length Code:    {parsed['filament_len_code']} ({length_code_to_weight(parsed['filament_len_code'])})")
            print(f"  Serial Num:     {parsed['serial_num']}")
            print(f"  Padding:        {repr(parsed['padding'])}")
        else:
            print(f"  Error: {parsed['error']}")
    else:
        print("Could not read all data blocks")

    print("\n" + "=" * 60)

    return 0

if __name__ == "__main__":
    sys.exit(main())
