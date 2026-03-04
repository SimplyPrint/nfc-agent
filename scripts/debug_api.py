#!/usr/bin/env python3
"""
NFC Agent API Health Check

Quick verification that the agent is running and all core endpoints work.
No NFC card required for the basic checks.

Usage:
    python3 scripts/debug_api.py
    python3 scripts/debug_api.py --card    # also test card endpoints if reader has a card

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


def ok(msg: str):
    print(f"  OK  {msg}")


def fail(msg: str):
    print(f"  FAIL  {msg}")


def warn(msg: str):
    print(f"  WARN  {msg}")


def check(label: str, fn) -> bool:
    try:
        result = fn()
        ok(f"{label}: {result}")
        return True
    except AssertionError as e:
        fail(f"{label}: {e}")
        return False
    except requests.exceptions.ConnectionError:
        fail(f"{label}: cannot connect to agent at {BASE_URL}")
        return False
    except Exception as e:
        fail(f"{label}: {e}")
        return False


def test_health():
    resp = requests.get(f"{BASE_URL}/health", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    data = resp.json()
    assert data.get("status") == "ok", f"status={data.get('status')}"
    return f"status=ok, readerCount={data.get('readerCount', '?')}"


def test_version():
    resp = requests.get(f"{BASE_URL}/version", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    data = resp.json()
    v = data.get("version", "?")
    build = data.get("buildTime", "?")
    commit = data.get("gitCommit", "?")[:7] if data.get("gitCommit") else "?"
    update = " (update available!)" if data.get("updateAvailable") else ""
    return f"v{v} built={build} commit={commit}{update}"


def test_readers() -> list:
    resp = requests.get(f"{BASE_URL}/readers", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    readers = resp.json()
    assert isinstance(readers, list), "expected list"
    return readers


def test_supported_readers():
    resp = requests.get(f"{BASE_URL}/supported-readers", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    return "ok"


def test_settings():
    resp = requests.get(f"{BASE_URL}/settings", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    data = resp.json()
    return f"crashReporting={data.get('crashReporting')}, extendedLogging={data.get('extendedLogging')}"


def test_logs():
    resp = requests.get(f"{BASE_URL}/logs?limit=5", timeout=5)
    assert resp.status_code == 200, f"HTTP {resp.status_code}"
    data = resp.json()
    # Response: {"entries":[...], "stats":{"total_entries":N,...}}
    count = len(data.get("entries", []))
    total = data.get("stats", {}).get("total_entries", "?")
    return f"{count} recent entries ({total} total)"


def test_card(reader_index: int):
    resp = requests.get(f"{BASE_URL}/readers/{reader_index}/card", timeout=10)
    if resp.status_code == 200:
        card = resp.json()
        uid = card.get("uid", "?")
        card_type = card.get("type", "?")
        data_type = card.get("dataType", "none")
        return f"uid={uid} type={card_type} dataType={data_type}"
    elif resp.status_code in (404, 500):
        # No card on reader
        return "no card on reader"
    else:
        raise AssertionError(f"HTTP {resp.status_code}: {resp.text}")


def main():
    global BASE_URL
    parser = argparse.ArgumentParser(description="NFC Agent API Health Check")
    parser.add_argument("--card", action="store_true", help="Also test card endpoints if reader present")
    parser.add_argument("--url", default=BASE_URL, help=f"API base URL (default: {BASE_URL})")
    args = parser.parse_args()

    if args.url != BASE_URL:
        BASE_URL = args.url

    print("=" * 60)
    print("  NFC Agent API Health Check")
    print(f"  URL: {BASE_URL}")
    print("=" * 60)

    all_ok = True

    # Core endpoints
    print("\nCore endpoints:")
    if not check("Health", test_health):
        print("\n  Cannot connect to agent. Start it with:")
        print("    go build -o nfc-agent ./cmd/nfc-agent && ./nfc-agent")
        sys.exit(1)
        all_ok = False

    check("Version", test_version)
    check("Settings", test_settings)
    check("Logs", test_logs)
    check("Supported readers", test_supported_readers)

    # Readers
    print("\nReaders:")
    readers = []
    try:
        readers = test_readers()
        if readers:
            ok(f"Found {len(readers)} reader(s):")
            for i, r in enumerate(readers):
                print(f"       [{i}] {r.get('name', 'Unknown')} (type: {r.get('type', '?')})")
        else:
            warn("No readers found — connect a USB NFC reader")
    except Exception as e:
        fail(f"Readers: {e}")
        all_ok = False

    # Card endpoints (optional)
    if args.card and readers:
        print("\nCard endpoints:")
        for i in range(len(readers)):
            result = check(f"Reader {i} card", lambda ri=i: test_card(ri))
            if not result:
                all_ok = False

    # Summary
    print("\n" + "=" * 60)
    if all_ok:
        print("  All checks passed!")
    else:
        print("  Some checks failed. See above for details.")
    print("=" * 60)

    return 0 if all_ok else 1


if __name__ == "__main__":
    sys.exit(main())
