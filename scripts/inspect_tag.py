#!/usr/bin/env python3
"""
NFC Tag Inspector

Generic "what's on this tag?" tool. Reads all available information from
whatever card is on the reader: UID, type, ATR, NDEF data, and raw pages/blocks.

Usage:
    python3 scripts/inspect_tag.py
    python3 scripts/inspect_tag.py --reader 1
    python3 scripts/inspect_tag.py --pages 20    # read 20 pages (NTAG)
    python3 scripts/inspect_tag.py --blocks 16   # read 16 blocks (MIFARE Classic)

Requirements: pip install requests
"""

import argparse
import json
import sys

try:
    import requests
except ImportError:
    print("Error: requests library required. Install with: pip3 install requests")
    sys.exit(1)

BASE_URL = "http://127.0.0.1:32145/v1"

DEFAULT_MIFARE_KEYS = [
    "FFFFFFFFFFFF",
    "D3F7D3F7D3F7",
    "A0A1A2A3A4A5",
    "000000000000",
]

SECTOR_TRAILERS = {3, 7, 11, 15, 19, 23, 27, 31, 35, 39, 43, 47, 51, 55, 59, 63}


def check_agent() -> bool:
    try:
        resp = requests.get(f"{BASE_URL}/health", timeout=2)
        return resp.status_code == 200
    except requests.exceptions.ConnectionError:
        return False


def get_readers() -> list:
    resp = requests.get(f"{BASE_URL}/readers", timeout=5)
    resp.raise_for_status()
    return resp.json()


def get_card(reader_index: int) -> dict | None:
    resp = requests.get(f"{BASE_URL}/readers/{reader_index}/card", timeout=10)
    if resp.status_code == 200:
        return resp.json()
    return None


def read_mifare_block(reader_index: int, block: int, key: str, key_type: str = "A") -> dict | None:
    resp = requests.get(
        f"{BASE_URL}/readers/{reader_index}/mifare/{block}",
        params={"key": key, "keyType": key_type},
        timeout=5,
    )
    if resp.status_code == 200:
        return resp.json()
    return None


def read_ultralight_page(reader_index: int, page: int) -> dict | None:
    resp = requests.get(f"{BASE_URL}/readers/{reader_index}/ultralight/{page}", timeout=5)
    if resp.status_code == 200:
        return resp.json()
    return None


def format_hex(data: str, group: int = 4) -> str:
    """Format hex string with spaces every N chars."""
    if not data:
        return "(empty)"
    return " ".join(data[i:i + group] for i in range(0, len(data), group))


def print_section(title: str):
    print(f"\n{'─' * 60}")
    print(f"  {title}")
    print(f"{'─' * 60}")


def inspect_mifare_classic(reader_index: int, num_blocks: int = 16):
    """Read MIFARE Classic blocks using default keys."""
    print_section("MIFARE Classic Raw Blocks")

    working_key = None
    working_key_type = "A"

    for block in range(num_blocks):
        if block in SECTOR_TRAILERS:
            print(f"  Block {block:3d}  [SECTOR TRAILER — skipped for safety]")
            continue

        result = None
        used_key = None

        if working_key:
            # Try the key that worked last time first
            result = read_mifare_block(reader_index, block, working_key, working_key_type)
            if result and result.get("data"):
                used_key = working_key

        if not result or not result.get("data"):
            # Try all default keys
            for key in DEFAULT_MIFARE_KEYS:
                result = read_mifare_block(reader_index, block, key, "A")
                if result and result.get("data"):
                    working_key = key
                    working_key_type = "A"
                    used_key = key
                    break

        if result and result.get("data"):
            data = result["data"].upper()
            formatted = format_hex(data, 2)
            ascii_repr = "".join(
                chr(int(data[i:i+2], 16)) if 32 <= int(data[i:i+2], 16) <= 126 else "."
                for i in range(0, len(data), 2)
            )
            key_label = f"[key={used_key}]" if used_key != working_key else ""
            print(f"  Block {block:3d}  {formatted}  |{ascii_repr}| {key_label}")
        else:
            print(f"  Block {block:3d}  [read failed — authentication error or no card]")


def inspect_ultralight(reader_index: int, num_pages: int = 20):
    """Read NTAG / Ultralight pages."""
    print_section("NTAG / Ultralight Pages (4 bytes each)")

    for page in range(num_pages):
        result = read_ultralight_page(reader_index, page)
        if result and result.get("data"):
            data = result["data"].upper()
            formatted = format_hex(data, 2)
            ascii_repr = "".join(
                chr(int(data[i:i+2], 16)) if 32 <= int(data[i:i+2], 16) <= 126 else "."
                for i in range(0, len(data), 2)
            )
            annotation = ""
            if page == 0:
                annotation = "← UID bytes 0-2 + check"
            elif page == 1:
                annotation = "← UID bytes 3-6"
            elif page == 2:
                annotation = "← lock bytes"
            elif page == 3:
                annotation = "← CC (Capability Container)"
            elif page == 4:
                annotation = "← first user data page"
            print(f"  Page {page:3d}  {formatted}  |{ascii_repr}| {annotation}")
        else:
            print(f"  Page {page:3d}  [read failed]")


def main():
    parser = argparse.ArgumentParser(description="NFC Tag Inspector")
    parser.add_argument("--reader", type=int, default=0, help="Reader index (default: 0)")
    parser.add_argument("--pages", type=int, default=20, help="Number of pages to read for NTAG/Ultralight (default: 20)")
    parser.add_argument("--blocks", type=int, default=16, help="Number of blocks to read for MIFARE Classic (default: 16)")
    args = parser.parse_args()

    print("=" * 60)
    print("  NFC Tag Inspector")
    print("=" * 60)

    # Check agent
    if not check_agent():
        print("\nERROR: Cannot connect to NFC Agent at http://127.0.0.1:32145")
        print("Make sure it's running: go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent")
        sys.exit(1)

    # List readers
    try:
        readers = get_readers()
    except Exception as e:
        print(f"\nERROR: Failed to get readers: {e}")
        sys.exit(1)

    if not readers:
        print("\nNo readers found. Connect a USB NFC reader and try again.")
        sys.exit(1)

    print(f"\nAvailable readers:")
    for i, r in enumerate(readers):
        marker = " ←" if i == args.reader else ""
        print(f"  [{i}] {r.get('name', 'Unknown')}{marker}")

    if args.reader >= len(readers):
        print(f"\nERROR: Reader index {args.reader} out of range (0–{len(readers)-1})")
        sys.exit(1)

    reader_name = readers[args.reader].get("name", "Unknown")
    print(f"\nUsing reader [{args.reader}]: {reader_name}")

    # Read card
    print("\nReading card...")
    card = get_card(args.reader)
    if not card:
        print("\nNo card detected. Place an NFC tag on the reader and try again.")
        sys.exit(1)

    # Card summary
    print_section("Card Info")
    print(f"  UID:          {card.get('uid', 'N/A')}")
    print(f"  Type:         {card.get('type', 'Unknown')}")
    print(f"  Protocol:     {card.get('protocol', 'N/A')} ({card.get('protocolISO', 'N/A')})")
    print(f"  ATR:          {card.get('atr', 'N/A')}")
    print(f"  Size:         {card.get('size', 0)} bytes")
    print(f"  Writable:     {card.get('writable', 'Unknown')}")

    if card.get("dataType"):
        print_section("NDEF / Data")
        print(f"  Data type:    {card.get('dataType', 'none')}")
        if card.get("url"):
            print(f"  URL:          {card.get('url')}")
        if card.get("data"):
            data = card.get("data", "")
            if card.get("dataType") == "openprinttag":
                print("  OpenPrintTag data:")
                if isinstance(data, dict):
                    for k, v in data.items():
                        print(f"    {k}: {v}")
                else:
                    print(f"  {data}")
            elif card.get("dataType") == "json":
                try:
                    parsed = json.loads(data) if isinstance(data, str) else data
                    print(f"  JSON data:")
                    print(f"    {json.dumps(parsed, indent=4)}")
                except Exception:
                    print(f"  Data: {data}")
            else:
                # Truncate long data
                display = str(data)[:200] + ("..." if len(str(data)) > 200 else "")
                print(f"  Data:         {display}")

    # Raw page/block dump
    card_type = card.get("type", "").upper()
    if "CLASSIC" in card_type:
        inspect_mifare_classic(args.reader, args.blocks)
    elif any(t in card_type for t in ["NTAG", "ULTRALIGHT", "ISO 15693"]):
        inspect_ultralight(args.reader, args.pages)
    else:
        # Unknown type — try ultralight first
        print_section(f"Raw Data (unknown type: {card.get('type', '?')})")
        print("  Attempting ultralight page read...")
        inspect_ultralight(args.reader, min(args.pages, 16))

    print("\n" + "=" * 60)
    print("  Done")
    print("=" * 60)


if __name__ == "__main__":
    main()
