#!/usr/bin/env python3
"""
Test script for raw card dump via Python SDK.

Tests:
  1. HTTP: GET /v1/readers/0/dump
  2. WS: dump_card command
  3. WS: subscribe with include_raw=True (tap a card to trigger)
  4. Creality CFS decode from MIFARE Classic raw blocks

Usage:
    cd /path/to/nfc-agent
    pip install -e sdk/python pycryptodome requests
    python3 scripts/test_dump.py
    python3 scripts/test_dump.py --test subscribe   # also tests include_raw subscribe
"""

from __future__ import annotations

import argparse
import asyncio
import sys

try:
    import requests
except ImportError:
    print("ERROR: pip install requests")
    sys.exit(1)

try:
    from nfc_agent import NFCWebSocket, CardDataEvent, CardRawDump
except ImportError:
    print("ERROR: pip install -e sdk/python")
    sys.exit(1)

BASE_URL = "http://127.0.0.1:32145/v1"

# Creality CFS AES keys (from CrealityMaterialStandard.php)
CREALITY_AES_KEY_DATA = b"H@CFkRnz@KAtBJp2"

CREALITY_FILAMENT_NAMES: dict[str, str] = {
    "010101": "PLA",
    "010201": "PETG",
    "010301": "ABS",
    "010401": "TPU",
    "010501": "ASA",
    "010601": "PC",
    "010701": "PA",
    "010801": "PLA-CF",
    "010901": "PETG-CF",
    "010A01": "ABS-CF",
    "010B01": "PA-CF",
    "010C01": "PLA-Silk",
    "010D01": "PLA-Matte",
}

LENGTH_CODE_TO_WEIGHT: dict[str, str] = {
    "0330": "1000g",
    "0247": "750g",
    "0198": "600g",
    "0165": "500g",
    "0082": "250g",
}


def http_dump(reader_index: int = 0) -> dict | None:
    """Call GET /v1/readers/{n}/dump via HTTP."""
    try:
        resp = requests.get(f"{BASE_URL}/readers/{reader_index}/dump", timeout=15)
        if resp.status_code == 200:
            return resp.json()
        print(f"  HTTP dump failed: {resp.status_code} {resp.text[:200]}")
        return None
    except requests.exceptions.ConnectionError:
        print("  ERROR: Cannot connect to NFC Agent")
        return None


def try_decrypt_creality(blocks: dict[str, str]) -> dict | None:
    """Try to decrypt Creality CFS data from MIFARE Classic blocks 4, 5, 6."""
    try:
        from Crypto.Cipher import AES
    except ImportError:
        print("  (pycryptodome not installed, skipping Creality decode)")
        return None

    encrypted_blocks = []
    for block_num in [4, 5, 6]:
        key = str(block_num)
        if key not in blocks:
            print(f"  Block {block_num} not available — Creality decode skipped")
            return None
        encrypted_blocks.append(blocks[key])

    decrypted = ""
    for i, enc_hex in enumerate(encrypted_blocks):
        enc_bytes = bytes.fromhex(enc_hex)
        cipher = AES.new(CREALITY_AES_KEY_DATA, AES.MODE_ECB)
        dec = cipher.decrypt(enc_bytes).decode("ascii", errors="replace")
        print(f"  Block {4+i} decrypted: {repr(dec)}")
        decrypted += dec

    print(f"\n  Full payload (48 chars): {repr(decrypted)}")

    if len(decrypted) < 34:
        print(f"  Payload too short: {len(decrypted)} chars")
        return None

    payload = decrypted.ljust(48, "0")[:48]
    parsed = {
        "date_code": payload[0:6],
        "vendor_id": payload[6:10],
        "batch_code": payload[10:12],
        "filament_id": payload[12:18].upper(),
        "color_hex": payload[18:24].upper(),
        "length_code": payload[24:28],
        "serial_num": payload[28:34],
        "padding": payload[34:48],
    }

    filament_name = CREALITY_FILAMENT_NAMES.get(parsed["filament_id"], "Unknown")
    weight = LENGTH_CODE_TO_WEIGHT.get(parsed["length_code"], "Unknown")

    print("\n  Parsed Creality fields:")
    print(f"    Date Code:     {parsed['date_code']}")
    print(f"    Vendor ID:     {parsed['vendor_id']}")
    print(f"    Batch Code:    {parsed['batch_code']}")
    print(f"    Filament ID:   {parsed['filament_id']} ({filament_name})")
    print(f"    Color (hex):   #{parsed['color_hex']}")
    print(f"    Length Code:   {parsed['length_code']} ({weight})")
    print(f"    Serial Num:    {parsed['serial_num']}")

    return parsed


def print_dump(dump: dict) -> None:
    """Pretty-print a CardRawDump dict."""
    print(f"  UID:  {dump.get('uid', 'N/A')}")
    print(f"  Type: {dump.get('type', 'N/A')}")

    if pages := dump.get("pages"):
        print(f"  Pages ({len(pages)} total):")
        for i, page in enumerate(pages[:8]):
            print(f"    Page {i:3d}: {page}")
        if len(pages) > 8:
            print(f"    ... and {len(pages) - 8} more pages")

    if blocks := dump.get("blocks"):
        print(f"  Blocks ({len(blocks)} readable):")
        for block_num in sorted(blocks.keys(), key=int)[:8]:
            print(f"    Block {int(block_num):3d}: {blocks[block_num]}")
        if len(blocks) > 8:
            print(f"    ... and {len(blocks) - 8} more blocks")

        failed = dump.get("failedBlocks") or []
        if failed:
            print(f"  Failed blocks (unknown keys): {failed}")

        # Try Creality decode if this looks like a MIFARE Classic card
        if "4" in blocks or "5" in blocks or "6" in blocks:
            print("\n  Attempting Creality CFS decode...")
            try_decrypt_creality(blocks)


def print_card_data_event(event: CardDataEvent) -> None:
    """Pretty-print a CardDataEvent."""
    d = {
        "uid": event.uid,
        "type": event.type,
        "pages": event.pages,
        "blocks": event.blocks,
        "failedBlocks": event.failed_blocks,
    }
    # Remove None values
    d = {k: v for k, v in d.items() if v is not None}
    print_dump(d)


async def test_ws_dump(reader_index: int = 0) -> bool:
    """Test dump_card WS command."""
    print("\n[2] WebSocket dump_card command")
    print("-" * 50)

    try:
        async with NFCWebSocket() as ws:
            print("  Connected to NFC Agent WS")
            try:
                dump = await asyncio.wait_for(ws.dump_card(reader_index), timeout=15)
                print("  dump_card response received:")
                print_dump({
                    "uid": dump.uid,
                    "type": dump.type,
                    "pages": dump.pages,
                    "blocks": dump.blocks,
                    "failedBlocks": dump.failed_blocks,
                })
                return True
            except Exception as e:
                print(f"  dump_card failed: {e}")
                return False
    except Exception as e:
        print(f"  WS connection failed: {e}")
        return False


async def test_ws_subscribe_raw(reader_index: int = 0) -> None:
    """Test subscribe with include_raw=True."""
    print("\n[3] WebSocket subscribe with include_raw=True")
    print("-" * 50)
    print("  Tap a card on the reader to see card_detected + card_data events...")
    print("  (waiting up to 30 seconds, Ctrl+C to skip)")

    received_data = asyncio.Event()

    try:
        async with NFCWebSocket() as ws:
            @ws.on_card_detected
            def handle_detected(event):
                print(f"\n  >> card_detected: UID={event.card.uid}, Type={event.card.type}")
                print(f"     (waiting for card_data event...)")

            @ws.on_card_data
            def handle_data(event: CardDataEvent):
                print(f"\n  >> card_data received!")
                print_card_data_event(event)
                received_data.set()

            await ws.subscribe(reader_index, include_raw=True)
            print(f"  Subscribed to reader {reader_index} with include_raw=True")

            try:
                await asyncio.wait_for(received_data.wait(), timeout=30)
                print("\n  Subscribe test PASSED")
            except asyncio.TimeoutError:
                print("\n  No card tapped within 30s — subscribe test skipped")
    except Exception as e:
        print(f"  WS connection failed: {e}")


async def main() -> int:
    parser = argparse.ArgumentParser(description="Test NFC Agent raw card dump")
    parser.add_argument("--reader", type=int, default=0, help="Reader index (default: 0)")
    parser.add_argument(
        "--test",
        choices=["http", "ws", "subscribe", "all"],
        default="all",
        help="Which test to run (default: all, skips subscribe interaction)",
    )
    args = parser.parse_args()

    print("=" * 60)
    print("  NFC Agent Raw Dump Test (Python SDK)")
    print("=" * 60)

    run_http = args.test in ("http", "all")
    run_ws = args.test in ("ws", "all")
    run_subscribe = args.test == "subscribe"

    # -- Test 1: HTTP dump --
    if run_http:
        print("\n[1] HTTP GET /v1/readers/0/dump")
        print("-" * 50)
        dump = http_dump(args.reader)
        if dump:
            print("  HTTP dump response received:")
            print_dump(dump)
        else:
            print("  No card? Place a card on the reader and try again.")

    # -- Test 2: WS dump_card --
    if run_ws:
        await test_ws_dump(args.reader)

    # -- Test 3: WS subscribe with include_raw --
    if run_subscribe:
        await test_ws_subscribe_raw(args.reader)

    print("\n" + "=" * 60)
    print("  Done")
    print("=" * 60)
    return 0


if __name__ == "__main__":
    sys.exit(asyncio.run(main()))
