#!/usr/bin/env python3
"""Smoke-test the transparent DESFire WebSocket session.

The DESFire session (desfire_session_open / desfire_transmit /
desfire_transmit_batch / desfire_session_close) is a raw, key-free APDU pipe:
the agent holds the PC/SC connection open across messages and forwards
caller-supplied APDU bytes verbatim. The SimplyPrint backend drives the actual
DESFire crypto over this pipe.

This test is card-agnostic on purpose: it opens a session and transmits the
standard get-UID pseudo-APDU (FF CA 00 00 00) through it, which any ISO 14443-A
card answers. That proves open -> transmit -> close works end-to-end against a
real reader without needing a DESFire card on the bench. Put a DESFire card on
the reader and feed real 0x90-class APDUs to exercise the actual chip.

Usage:
    python3 scripts/test_desfire_session.py [reader_index]
"""

import asyncio
import json
import sys

import websockets

WS_URL = "ws://127.0.0.1:32145/v1/ws"


async def call(ws, msg_type, payload, req_id):
    await ws.send(json.dumps({"type": msg_type, "id": req_id, "payload": payload}))
    while True:
        resp = json.loads(await ws.recv())
        # Ignore unsolicited push events (card_detected, readers_changed, ...).
        if resp.get("id") == req_id or resp.get("type") == "error":
            return resp


async def main(reader_index):
    async with websockets.connect(WS_URL) as ws:
        # 1. Open the session.
        r = await call(ws, "desfire_session_open", {"readerIndex": reader_index}, "open-1")
        print("open     ->", json.dumps(r))
        if not r.get("type") == "desfire_session_opened":
            print("FAIL: could not open session:", r.get("error"))
            return 1
        uid = (r.get("payload") or {}).get("uid", "")

        # 2. Transmit get-UID through the pipe. Expect <uid> 90 00.
        r = await call(ws, "desfire_transmit", {"readerIndex": reader_index, "apdu": "ffca000000"}, "tx-1")
        print("transmit ->", json.dumps(r))
        p = r.get("payload") or {}
        ok = r.get("type") == "desfire_response" and p.get("sw1") == 0x90 and p.get("sw2") == 0x00
        if ok and uid:
            ok = p.get("response", "").lower().startswith(uid.lower())
        print(f"  relayed get-UID matches session UID + 9000: {ok}")

        # 3. Batch transmit (two get-UIDs).
        r = await call(ws, "desfire_transmit_batch",
                       {"readerIndex": reader_index, "apdus": ["ffca000000", "ffca000000"]}, "batch-1")
        print("batch    ->", json.dumps(r))

        # 4. Close.
        r = await call(ws, "desfire_session_close", {"readerIndex": reader_index}, "close-1")
        print("close    ->", json.dumps(r))

        # 5. Transmit after close must fail cleanly.
        r = await call(ws, "desfire_transmit", {"readerIndex": reader_index, "apdu": "ffca000000"}, "tx-2")
        print("tx-after-close ->", json.dumps(r))
        clean = r.get("type") == "error" and "no DESFire session" in (r.get("error") or "")
        print(f"  transmit after close rejected cleanly: {clean}")

        return 0 if ok and clean else 1


if __name__ == "__main__":
    idx = int(sys.argv[1]) if len(sys.argv) > 1 else 0
    sys.exit(asyncio.run(main(idx)))
