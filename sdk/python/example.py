#!/usr/bin/env python3
"""Example script to test the NFC Agent Python SDK locally."""

import asyncio

from nfc_agent import NFCClient, NFCWebSocket, CardError


async def test_rest_client():
    """Test the REST client."""
    print("=" * 60)
    print("Testing REST Client (NFCClient)")
    print("=" * 60)

    async with NFCClient() as client:
        # Check connection
        connected = await client.is_connected()
        print(f"Connected to nfc-agent: {connected}")
        if not connected:
            print("ERROR: nfc-agent is not running!")
            return False

        # Get version
        version = await client.get_version()
        print(f"Agent version: {version.version}")

        # List readers
        readers = await client.get_readers()
        print(f"\nFound {len(readers)} reader(s):")
        for i, reader in enumerate(readers):
            print(f"  [{i}] {reader.name} (type: {reader.type})")

        if not readers:
            print("No readers found!")
            return False

        # Try to read from each reader
        print("\nTrying to read cards from each reader...")
        for i, reader in enumerate(readers):
            try:
                card = await client.read_card(i)
                print(f"\n  Reader {i} ({reader.name}):")
                print(f"    UID: {card.uid}")
                print(f"    Type: {card.type}")
                print(f"    Protocol: {card.protocol}")
                if card.size:
                    print(f"    Size: {card.size} bytes")
                if card.data:
                    print(f"    Data: {card.data[:50]}{'...' if len(card.data) > 50 else ''}")
                if card.data_type:
                    print(f"    Data Type: {card.data_type.value}")
            except CardError as e:
                print(f"\n  Reader {i} ({reader.name}): No card present")

        return True


async def test_websocket():
    """Test the WebSocket client with real-time events."""
    print("\n" + "=" * 60)
    print("Testing WebSocket Client (NFCWebSocket)")
    print("=" * 60)

    async with NFCWebSocket() as ws:
        # Get readers
        readers = await ws.get_readers()
        print(f"Found {len(readers)} reader(s)")

        # Health check
        health = await ws.health()
        print(f"Health: {health.status} (uptime: {health.uptime}s)")

        if not readers:
            print("No readers to subscribe to")
            return

        # Subscribe to all readers
        for i, reader in enumerate(readers):
            await ws.subscribe(i)
            print(f"Subscribed to reader {i}: {reader.name}")

        # Set up event handlers
        @ws.on_card_detected
        def handle_card(event):
            print(f"\n*** CARD DETECTED on reader {event.reader} ***")
            print(f"    UID: {event.card.uid}")
            print(f"    Type: {event.card.type}")
            if event.card.data:
                print(f"    Data: {event.card.data[:50]}")

        @ws.on_card_removed
        def handle_removed(event):
            print(f"\n*** CARD REMOVED from reader {event.reader} ***")

        @ws.on_disconnected
        def handle_disconnect():
            print("\n*** DISCONNECTED ***")

        print("\nListening for card events for 30 seconds...")
        print("Place or remove a card to see events.\n")

        await asyncio.sleep(30)

        print("\nDone listening.")


async def test_polling():
    """Test the card poller."""
    print("\n" + "=" * 60)
    print("Testing Card Poller")
    print("=" * 60)

    async with NFCClient() as client:
        readers = await client.get_readers()
        if not readers:
            print("No readers found!")
            return

        # Use first reader
        print(f"Polling reader 0: {readers[0].name}")

        poller = client.poll_card(0, interval=0.5)

        @poller.on_card
        def handle_card(card):
            print(f"  Card detected: {card.uid} ({card.type})")

        @poller.on_removed
        def handle_removed():
            print("  Card removed")

        @poller.on_error
        def handle_error(e):
            print(f"  Error: {e}")

        await poller.start()
        print("Polling for 15 seconds...")

        await asyncio.sleep(15)

        poller.stop()
        print("Polling stopped.")


async def main():
    """Run all tests."""
    print("\nNFC Agent Python SDK - Local Test")
    print("=" * 60)

    try:
        # Test REST client
        success = await test_rest_client()
        if not success:
            return

        # Ask user if they want to continue with WebSocket test
        print("\n" + "-" * 60)
        response = input("Test WebSocket events? (y/n): ").strip().lower()
        if response == "y":
            await test_websocket()

        # Ask user if they want to test polling
        print("\n" + "-" * 60)
        response = input("Test card polling? (y/n): ").strip().lower()
        if response == "y":
            await test_polling()

    except Exception as e:
        print(f"\nError: {e}")
        raise

    print("\n" + "=" * 60)
    print("Test complete!")
    print("=" * 60)


if __name__ == "__main__":
    asyncio.run(main())
